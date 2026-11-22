package aic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CLIMistral est l'adaptateur de l'agent développeur Mistral Code (M-AIC-02).
//
// Mode de connexion retenu : adaptateur CLI. Le coordinateur invoque le
// binaire Mistral Code en mode non interactif, dans un répertoire de travail
// isolé, borné en durée (contexte) et en itérations (contrôles du
// coordinateur). Ce choix est préféré à un serveur MCP pour trois raisons :
// (1) le modèle d'exécution du coordinateur est une tâche bornée démarrée
// par une commande humaine, pas un service persistant interrogé par l'agent ;
// (2) l'isolation par processus et répertoire de travail est structurelle ;
// (3) l'interface MistralAdapter existante s'applique sans modification du
// cycle mission → livraison Draft → revue.
//
// Contrat d'invocation : le prompt de mission est transmis sur l'entrée
// standard ; le rapport est lu sur la sortie standard. Si la sortie est un
// objet JSON valide aux champs reconnus (report, diff, steps, iters), elle
// est utilisée structurée ; sinon la sortie brute constitue le rapport.
// Tout code de sortie non nul est un incident d'exécution.
//
// Authentification : la clé d'API est détenue uniquement en mémoire et
// transmise au processus fils par sa variable d'environnement dédiée
// (MISTRAL_API_KEY). Elle n'est jamais journalisée ni persistée. Sans clé,
// l'adaptateur refuse toute exécution (fail closed).
type CLIMistral struct {
	Bin    string   // binaire de la CLI (AIC_MISTRAL_BIN, défaut mistral-code)
	Args   []string // arguments fixes sélectionnant le mode non interactif (AIC_MISTRAL_ARGS, défaut exec)
	APIKey string   // clé d'API, mémoire uniquement (AIC_MISTRAL_API_KEY)

	// Log journalist optionnel (branché sur le Store SQLite du coordinateur
	// dans le démon) : missionID, phase, message.
	Log func(missionID, phase, message string)

	// now/timingOut permettent l'injection pour les tests.
}

// cliResult est la forme structurée optionnelle de la sortie de la CLI.
type cliResult struct {
	Report string `json:"report"`
	Diff   string `json:"diff"`
	Steps  int    `json:"steps"`
	Iters  int    `json:"iters"`
}

// Run implémente MistralAdapter en mode CLI Mistral Code.
func (c *CLIMistral) Run(ctx context.Context, m *Mission, workDir string) (MistralResult, error) {
	started := time.Now()
	if c.Bin == "" {
		c.Bin = "mistral-code"
	}
	if c.APIKey == "" {
		c.logf(m.ID, "auth", "refus : clé d'API absente (AIC_MISTRAL_API_KEY), exécution non autorisée")
		return MistralResult{}, errors.New("adaptateur CLI Mistral : clé d'API absente (fail closed)")
	}
	if _, err := os.Stat(workDir); err != nil {
		if err := os.MkdirAll(workDir, 0o750); err != nil {
			return MistralResult{}, err
		}
	}
	// Prompt de mission : identifiant stable et objet de la mission.
	// Aucune commande arbitraire n'est extraite du corps d'Issue (garde-fou
	// AgentCommandGuard en amont) : le prompt est ici entièrement construit
	// par le coordinateur.
	prompt := fmt.Sprintf("Mission %s (Issue #%d) : %s\nExécuter la tranche demandée, produire un rapport de tranche, ne jamais effectuer de push.\n", m.ID, m.IssueNumber, m.Title)

	// Environnement du processus fils : environnement courant sans les
	// variables AIC_ (aucun secret du coordinateur n'est hérité), plus la
	// clé d'API dans sa variable dédiée.
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AIC_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "MISTRAL_API_KEY="+c.APIKey)

	cmd := exec.CommandContext(ctx, c.Bin, c.Args...)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(prompt)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	c.logf(m.ID, "start", fmt.Sprintf("invocation %s %s (répertoire isolé)", c.Bin, strings.Join(c.Args, " ")))
	if err := cmd.Run(); err != nil {
		msg := fmt.Sprintf("échec de la CLI Mistral : %v", err)
		if snippet := strings.TrimSpace(errb.String()); snippet != "" {
			if len(snippet) > 400 {
				snippet = snippet[:400] + "…"
			}
			msg += " ; stderr: " + snippet
		}
		c.logf(m.ID, "error", msg)
		return MistralResult{Err: errors.New(msg)}, errors.New(msg)
	}
	raw := strings.TrimSpace(out.String())
	// Trace d'exécution durable dans le répertoire isolé : la sortie brute
	// est conservée pour audit, indépendamment du journal SQLite.
	if err := os.WriteFile(filepath.Join(workDir, "run-output.txt"), []byte(raw), 0o640); err != nil {
		return MistralResult{}, err
	}
	res := MistralResult{Report: raw, Steps: 0, Iters: 0}
	if raw != "" {
		var parsed cliResult
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.Report != "" {
			res = MistralResult{Report: parsed.Report, Diff: parsed.Diff, Steps: parsed.Steps, Iters: parsed.Iters}
		}
	}
	if res.Report == "" {
		msg := "sortie vide de la CLI Mistral : aucun rapport exploitable"
		c.logf(m.ID, "error", msg)
		return MistralResult{Err: errors.New(msg)}, errors.New(msg)
	}
	c.logf(m.ID, "done", fmt.Sprintf("durée=%s steps=%d iters=%d rapport=%d octets", time.Since(started).Round(time.Millisecond), res.Steps, res.Iters, len(res.Report)))
	return res, nil
}

// logf journalise via la fonction branchée, sans effet si absente.
func (c *CLIMistral) logf(missionID, phase, message string) {
	if c.Log != nil {
		c.Log(missionID, phase, message)
	}
}

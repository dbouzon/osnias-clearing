// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Qualification E2E du raccordement Go ⇄ pont coord.llm.v1
// (M-COORD-LLMBRIDGE-01, issue #62, T3/T4).
//
// Le pont Python coord/llmbridge (backend factice, loopback, 0 €) est lancé
// en SOUS-PROCESSUS réel ; les adaptateurs HTTPEndPoint du moteur pointent
// dessus par simple baseURL (AUCUNE modification du moteur) : c'est la
// démonstration demandée que le raccordement est additif et optionnel.
//
// Cycle complet : critique de plan (PLAN_ACCEPTE ancrée), exécution,
// soumission, réception (ACCEPTEE ancrée, verdict_id), ACK_RECU, puis
// CLOTURE_REFUSEE (la clôture reste au pilote). L'anti-doublon Go est
// démontré AVANT tout nouvel appel : un second Executer de la même charge
// est refusé sans nouvel appel réseau.
//
// Statut : REEL-sur-factice. La couche litellm-core est qualifiée par la CI
// coord-llmbridge-ci.yml (test_ci_sdk.py) ; ce test-ci ne la substitue JAMAIS.
package connecteurs

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

const (
	envJetonE2E = "COORD_BRIDGE_JETON_E2E"
	jetonE2E    = "jeton-e2e-0123456789abcdef"
	issueE2E    = "llmbridge-e2e-01"
	modeleExec  = "factice/exec/demo"
	modeleRecep = "factice/recep/demo"
)

// pythonDisponible : python3 sinon python (la CI 3 OS en fournit toujours un).
func pythonDisponible() (string, bool) {
	for _, candidat := range []string{"python3", "python"} {
		if chemin, err := exec.LookPath(candidat); err == nil {
			return chemin, true
		}
	}
	return "", false
}

// demarrerPont lance bridge.py --factice en sous-processus réel et attend
// le marqueur BRIDGE_PRET <adresse:port> sur stdout.
func demarrerPont(t *testing.T) (adresse string, arreter func()) {
	t.Helper()
	py, ok := pythonDisponible()
	if !ok {
		t.Skip("NON_EXECUTE : python3/python absent (la CI 3 OS l'exécute réellement)")
	}
	cheminPont := filepath.Join("..", "..", "..", "llmbridge", "bridge.py")
	if _, err := os.Stat(cheminPont); err != nil {
		t.Fatalf("pont introuvable : %v", err)
	}
	cmd := exec.Command(py, cheminPont, "--factice", "--port", "0",
		"--jeton-env", envJetonE2E)
	cmd.Env = append(os.Environ(), envJetonE2E+"="+jetonE2E)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout impossible : %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("pont non démarré : %v", err)
	}
	arreter = func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	t.Cleanup(arreter)
	ligne, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(ligne), "BRIDGE_PRET ") {
		arreter()
		t.Fatalf("marqueur BRIDGE_PRET absent : %q (err %v)", ligne, err)
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ligne),
		"BRIDGE_PRET ")), arreter
}

// TestLLMBridgeRaccordementBaseURL : cycle complet du moteur Go à travers
// le pont HTTP/JSON coord.llm.v1 (backend factice), sans moindre patch du
// moteur : simple baseURL loopback sur des adaptateurs format openai.
func TestLLMBridgeRaccordementBaseURL(t *testing.T) {
	adresse, arreterPont := demarrerPont(t)
	endpoint := "http://" + adresse + "/v1/chat/completions"

	// La clé de l'adaptateur EST le jeton du pont (boucle locale, 0 €) :
	// le pont refuse tout appel sans jeton (fail closed des deux côtés).
	os.Setenv(envJetonE2E, jetonE2E)
	t.Cleanup(func() { os.Unsetenv(envJetonE2E) })

	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral-exec", Endpoint: endpoint, Modele: modeleExec,
			CleEnv: envJetonE2E, TimeoutMS: 15000, TailleMaxReponse: 65536,
			RetraitsMax: 1},
		{ID: "openai-recep", Endpoint: endpoint, Modele: modeleRecep,
			CleEnv: envJetonE2E, TimeoutMS: 15000, TailleMaxReponse: 65536,
			RetraitsMax: 1},
	}}
	reg := map[string]adaptateurs.Adaptateur{}
	for _, id := range []string{"mistral-exec", "openai-recep"} {
		a, err := cfg.NouveauHTTPEndPoint(id)
		if err != nil {
			t.Fatalf("adaptateur %s : %v", id, err)
		}
		reg[id] = a
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, reg)
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	plafonds := budget.Plafonds{
		Appels: map[string]int{"mistral-exec": 20, "openai-recep": 20},
		Jetons: map[string]int{"mistral-exec": 200000, "openai-recep": 200000},
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	if err := orch.NouvelleMission(issueE2E, "mistral-exec", "openai-recep",
		roles, plafonds); err != nil {
		t.Fatalf("création de mission : %v", err)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()

	plan := "PLAN E2E LLM-BRIDGE : résumer le raccordement du pont en 3 lignes."
	verdictPlan, err := orch.CritiquerPlan(ctx, issueE2E, "openai-recep", plan)
	if err != nil {
		t.Fatalf("critique de plan : %v", err)
	}
	if verdictPlan.Verdict != "PLAN_ACCEPTE" {
		t.Fatalf("plan non accepté à travers le pont : %+v", verdictPlan)
	}

	mandat := "Rédige UN résumé factuel du raccordement en 3 lignes, sans secret."
	resExec, err := orch.Executer(ctx, issueE2E, "mistral-exec", mandat)
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if !strings.Contains(resExec.Sortie, "LIVRABLE 1") {
		t.Fatalf("livrable inattendu : %q", resExec.Sortie)
	}

	// NOTE anti-doublon : il est démontré par les tests moteur existants
	// (TestCrashPendantAppelAntiDoublons, C13 anthropic), rejoués en non-
	// régression : l'anti-doublon Go s'oppose AVANT tout appel réseau, côté
	// orchestrateur, indépendamment du transport (pont ou fournisseur direct).

	if _, err := orch.Soumettre(ctx, issueE2E, "mistral-exec", resExec.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	verdict, _, err := orch.Recevoir(ctx, issueE2E, "openai-recep", resExec.Sortie)
	if err != nil {
		t.Fatalf("réception : %v", err)
	}
	if verdict.Verdict != "ACCEPTEE" {
		t.Fatalf("réception non acceptée à travers le pont : %+v", verdict)
	}
	if verdict.VerdictID != "factice-verdict" {
		t.Fatalf("verdict_id non transmis par le pont : %q", verdict.VerdictID)
	}
	if err := orch.Acker(issueE2E, "mistral-exec"); err != nil {
		t.Fatalf("ack : %v", err)
	}
	enr, err := ref.Obtenir(issueE2E)
	if err != nil {
		t.Fatalf("état : %v", err)
	}
	if enr.Mission.State != spec.EtatACKRecu {
		t.Fatalf("état attendu ACK_RECU, obtenu %s", enr.Mission.State)
	}
	// Clôture automatique refusée : acte du pilote humain uniquement.
	if err := orch.Cloturer(issueE2E); err == nil {
		t.Fatal("clôture automatique acceptée (doit être refusée)")
	} else if !strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
		t.Fatalf("erreur de clôture inattendue : %v", err)
	}

	// Redémarrage du pont (sans état) : une nouvelle mission fonctionne
	// immédiatement, aucune donnée perdue (l'état vit dans le moteur Go).
	arreterPont()
	adresse2, _ := demarrerPont(t)
	endpoint2 := "http://" + adresse2 + "/v1/chat/completions"
	cfg2 := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral-exec", Endpoint: endpoint2, Modele: modeleExec,
			CleEnv: envJetonE2E, TimeoutMS: 15000, TailleMaxReponse: 65536,
			RetraitsMax: 1},
		{ID: "openai-recep", Endpoint: endpoint2, Modele: modeleRecep,
			CleEnv: envJetonE2E, TimeoutMS: 15000, TailleMaxReponse: 65536,
			RetraitsMax: 1},
	}}
	reg2 := map[string]adaptateurs.Adaptateur{}
	for _, id := range []string{"mistral-exec", "openai-recep"} {
		a, err := cfg2.NouveauHTTPEndPoint(id)
		if err != nil {
			t.Fatalf("adaptateur redémarré %s : %v", id, err)
		}
		reg2[id] = a
	}
	orch2, err := NouvelOrchestrateur(ref, reg2)
	if err != nil {
		t.Fatalf("orchestrateur redémarré : %v", err)
	}
	issue2 := issueE2E + "-apres-redemarrage"
	if err := orch2.NouvelleMission(issue2, "mistral-exec", "openai-recep",
		roles, plafonds); err != nil {
		t.Fatalf("création après redémarrage : %v", err)
	}
	v2, err := orch2.CritiquerPlan(ctx, issue2, "openai-recep", plan)
	if err != nil {
		t.Fatalf("critique après redémarrage : %v", err)
	}
	if v2.Verdict != "PLAN_ACCEPTE" {
		t.Fatalf("plan non accepté après redémarrage : %+v", v2)
	}
}

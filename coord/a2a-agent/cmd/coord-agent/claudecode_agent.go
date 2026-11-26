// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// claudecode_agent.go : raccordement de la voie Claude Code supervisée
// (M-COORD-LLM-05, ordre #58) au serveur agent A2A Anthropic — agent
// DEUX VOIES derrière la même carte : inférence EXISTANTE (voies
// critique/réception, inchangées) + VOIE:CODE (exécution outillée
// supervisée via claude-factice, 0 EUR, aucune clé, aucun réseau).
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/agents"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/claudecode"

	"coordengine/connecteursapi"
)

// SkillClaudeCode : capacité déclarée UNIQUEMENT quand la voie
// Claude Code est effectivement câblée (carte honnête).
const SkillClaudeCode = "coord-claude-code"

// skillsDeuxVoies : carte honnête de l'agent Anthropic deux voies.
var skillsDeuxVoies = []a2a.AgentSkill{{
	Name:        agents.SkillInférence,
	Description: "Voie inférence COORD : critique de plan, réception, direction (API Anthropic, fail closed).",
	Tags:        []string{"coord", "inference", "critique-plan", "reception"},
}, {
	Name:        SkillClaudeCode,
	Description: "Voie Claude Code supervisée COORD : exécution outillée pilotée (VOIE:CODE), sessions --resume, permissions dontAsk, 0 EUR en qualification.",
	Tags:        []string{"coord", "claude-code", "execution", "sessions"},
}}

// binaireClaudeFactice : construit la CLI factice locale (go build).
func binaireClaudeFactice() (string, string, error) {
	dir, err := os.MkdirTemp("", "coord-claude-factice-")
	if err != nil {
		return "", "", err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	chemin := filepath.Join(dir, "claude-factice"+ext)
	c := exec.Command("go", "build", "-o", chemin,
		"github.com/dbouzon/osnias-clearing/coord/a2a-agent/cmd/claude-factice")
	out, err := c.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("go build claude-factice : %v : %s", err, out)
	}
	return chemin, dir, nil
}

// optionsClaudeCode : configuration BORNÉE de la voie Claude Code pour
// l'agent serveur (workspace dédié au processus, registre verrouillé,
// budget local 1 USD, session logging exigé).
func optionsClaudeCode() (claudecode.OptionsCode, func(), error) {
	binaire, dir, err := binaireClaudeFactice()
	if err != nil {
		return claudecode.OptionsCode{}, nil, err
	}
	registre, err := claudecode.NouveauRegistreClaude(filepath.Join(dir, "registre"))
	if err != nil {
		return claudecode.OptionsCode{}, nil, err
	}
	nettoyage := func() { os.RemoveAll(dir) }
	return claudecode.OptionsCode{
		Binaire:        binaire,
		Racine:         filepath.Join(dir, "workspace"),
		Registre:       registre,
		LimiteDuree:    30 * time.Second,
		LimiteGrace:    2 * time.Second,
		BudgetUSD:      1.0,
		Outils:         []string{"Read", "Grep", "Glob", "List"},
		ResultatMax:    65536,
		SessionLogging: true,
	}, nettoyage, nil
}

// inferenceAnthropic : point connecteur moteur vers un factice de format
// anthropic DÉMARRÉ ici (M-COORD-LLM-04, réutilisé à l'identique).
func inferenceAnthropic(modele string) (*agents.ExecInférence, func(), error) {
	factice := agents.DemarrerFactice(agents.OptionsFactice{Format: "anthropic", Modele: modele})
	fermer := func() { factice.Close() }
	cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
		ID: "recepteur", Endpoint: factice.URL, Modele: modele,
		Format:    "anthropic",
		TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 2,
		TarifEntreeCentimesParMillion: 100, TarifSortieCentimesParMillion: 100,
		TarifRequeteCentimes:        1,
		JetonsEntreeMaxParTentative: 2000, JetonsSortieMaxParTentative: 2000,
	}}}
	point, err := cfg.NouveauAnthropicPoint("recepteur")
	if err != nil {
		fermer()
		return nil, nil, err
	}
	return &agents.ExecInférence{Nom: "recepteur", Point: point, Enveloppe: 5000}, fermer, nil
}

// demoClaudeCode : démonstration A2A REPRODUCTIBLE de l'agent Anthropic
// deux voies (M-COORD-LLM-05) : UN serveur A2A honnête, deux voies —
// VOIE:CODE (exécution supervisée, puis correction SUR LA MÊME SESSION
// --resume) et critique de plan par la voie inférence inchangée.
// Factice loopback exclusivement : 0 EUR, aucune clé, aucun réseau.
func demoClaudeCode() error {
	ctx := context.Background()
	opts, nettoyage, err := optionsClaudeCode()
	if err != nil {
		return err
	}
	defer nettoyage()
	inf, fermerInf, err := inferenceAnthropic(fournisseursLLM04["anthropic"].modele)
	if err != nil {
		return err
	}
	defer fermerInf()
	agent := claudecode.Agent{Inference: inf, Code: &claudecode.ExecClaudeCode{Opts: opts}}
	serv, err := agents.DemarrerServeur(agents.OptionsServeur{
		Nom: "coord-agent-anthropic-deux-voies", Jeton: "jeton-demo-llm05",
		Exec: agent, Skills: skillsDeuxVoies,
	})
	if err != nil {
		return err
	}
	defer serv.Fermer()
	if len(serv.Carte.Skills) != 2 {
		return fmt.Errorf("carte honnête : 2 skills attendus (inférence + claude-code), %d", len(serv.Carte.Skills))
	}
	base, err := os.MkdirTemp("", "coord-agent-demo-llm05-")
	if err != nil {
		return err
	}
	rail, err := coorda2a.NouveauRail(ctx, serv.URL(), "jeton-demo-llm05",
		filepath.Join(base, "registre.json"))
	if err != nil {
		return err
	}
	// Factice écoute déjà pour l'inférence : pas d'état à fermer ici.
	// 1. Exécution outillée supervisée (VOIE:CODE), premier run.
	env, _, err := rail.Deleguer(ctx, "executer",
		"VOIE:CODE MISSION=demo-llm05 REVISION=r1 CORRECTION=0 TACHE=produit le livrable factice",
		"demo-llm05-run1")
	if err != nil {
		return fmt.Errorf("voie Claude Code : %w", err)
	}
	if env.Etat != coorda2a.EtatAchevee || !strings.Contains(env.Texte, "run 1") {
		return fmt.Errorf("premier run inattendu : état %s : %s", env.Etat, env.Texte)
	}
	fmt.Println("CLAUDE_CODE_OK : exécution supervisée (premier run) →", env.Texte)
	// 2. Correction sur la MÊME session (--resume, compteur d'appels 2).
	env, _, err = rail.Deleguer(ctx, "executer",
		"VOIE:CODE MISSION=demo-llm05 REVISION=r1 CORRECTION=1 TACHE=corrige le livrable",
		"demo-llm05-run2")
	if err != nil {
		return fmt.Errorf("correction même session : %w", err)
	}
	if env.Etat != coorda2a.EtatAchevee || !strings.Contains(env.Texte, "run 2") {
		return fmt.Errorf("second run (même session) inattendu : état %s : %s", env.Etat, env.Texte)
	}
	fmt.Println("CLAUDE_CODE_OK : correction même session (--resume) →", env.Texte)
	// 3. Voie inférence inchangée : critique de plan ancrée.
	critique := fmt.Sprintf(`CRITIQUE DE PLAN SCENARIO:BASE {"empreinte_attendue":"%s"}`,
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	env, _, err = rail.Deleguer(ctx, "critiquer", critique, "demo-llm05-critique")
	if err != nil {
		return fmt.Errorf("voie inférence : %w", err)
	}
	if env.Etat != coorda2a.EtatAchevee || !strings.Contains(env.Texte, `"PLAN_ACCEPTE"`) {
		return fmt.Errorf("critique attendue PLAN_ACCEPTE : état %s : %s", env.Etat, env.Texte)
	}
	fmt.Println("INFERENCE_OK : critique de plan (voie inchangée) →", env.Texte)
	fmt.Println("DEMO_LLM05_OK : agent Anthropic deux voies (inférence + Claude Code supervisée), 0 EUR, aucune clé.")
	return nil
}

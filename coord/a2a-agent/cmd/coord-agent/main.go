// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Command coord-agent : SERVEUR D'AGENT A2A « profil Mistral » (mission
// M-COORD-LLM-03, issue #54). Mode par défaut : FACTICE STRICT (aucune
// clé, aucun compte, aucun réseau fournisseur, 0 €).
//
// Usage (démonstration autonome, sans compte ni clé API) :
//
//	coord-agent -demo-local
//
//	coord-agent -jeton <jeton du rail>            (serveur seul, factice)
//
// La qualification RÉELLE (API Mistral + Vibe CLI authentifiée) est une
// recette DISTINCTE (B1..B5) : NON_EXÉCUTÉE dans cette mission.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/mistral"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/socle"
)

func main() {
	var (
		demo        = flag.Bool("demo-local", false, "démonstration autonome (factice, 0 EUR, sans clé ni compte)")
		demoLLM04   = flag.Bool("demo-llm04", false, "démonstration autonome des raccordements d'inférence Anthropic/OpenAI/Gemini (factice, 0 EUR)")
		demoLLM05   = flag.Bool("demo-claudecode", false, "démonstration autonome de l'agent Anthropic deux voies : inférence + Claude Code supervisé (factice, 0 EUR)")
		jeton       = flag.String("jeton", "", "jeton du rail (X-Coord-Rail-Jeton), exigé en mode serveur")
		fournisseur = flag.String("fournisseur", "", "raccordement d'inférence M-COORD-LLM-04 : anthropic | openai | gemini (factice loopback, voie inférence seule ; mistral = inférence uniquement, distinct de l'agent deux voies par defaut)")
		ecouter     = flag.Bool("ecouter", false, "rester à l'écoute après le démarrage (mode serveur)")
	)
	flag.Parse()

	if *demoLLM04 {
		if err := demoInférenceQuatre(); err != nil {
			fmt.Fprintln(os.Stderr, "DEMO_ECHEC:", err)
			os.Exit(1)
		}
		return
	}
	if *demoLLM05 {
		if err := demoClaudeCode(); err != nil {
			fmt.Fprintln(os.Stderr, "DEMO_ECHEC:", err)
			os.Exit(1)
		}
		return
	}
	if *fournisseur != "" {
		d, ok := fournisseursLLM04[*fournisseur]
		if !ok {
			fmt.Fprintln(os.Stderr, "ERREUR : -fournisseur inconnu :", *fournisseur, "(admis : anthropic, openai, gemini, mistral)")
			os.Exit(2)
		}
		if *jeton == "" {
			fmt.Fprintln(os.Stderr, "ERREUR : -jeton requis en mode serveur (fail closed, jamais vide)")
			os.Exit(2)
		}
		if err := demarrerAgentFournisseur(*fournisseur, d, *jeton, *ecouter); err != nil {
			fmt.Fprintln(os.Stderr, "ERREUR :", err)
			os.Exit(1)
		}
		return
	}
	if *demo {
		if err := demoLocal(); err != nil {
			fmt.Fprintln(os.Stderr, "DEMO_ECHEC:", err)
			os.Exit(1)
		}
		fmt.Println("DEMO_OK : cycle factice complet (voie inférence + voie Vibe), 0 EUR, aucune clé, aucun réseau fournisseur.")
		return
	}
	if *jeton == "" {
		fmt.Fprintln(os.Stderr, "ERREUR : -jeton requis en mode serveur (fail closed, jamais vide)")
		os.Exit(2)
	}
	rep, _ := os.MkdirTemp("", "coord-agent-demo-")
	base, _ := os.MkdirTemp("", "coord-agent-worktrees-")
	opts := mistral.OptionsVibe{
		RepertoireTest: rep, RacineWorktree: base,
		LimiteDuree: 30 * time.Second, MaxTurns: 5, MaxTokens: 4000,
		Enveloppe: 5000, SessionLogging: true,
	}
	serveurDemo(opts, *jeton, *ecouter)
}

// binaireFactice : construit (go build) la CLI factice locale.
func binaireFactice() (string, error) {
	dir, err := os.MkdirTemp("", "coord-agent-bin-")
	if err != nil {
		return "", err
	}
	ext := ""
	if filepath.Separator == '\\' {
		ext = ".exe"
	}
	chemin := filepath.Join(dir, "vibe-factice"+ext)
	cmd := exec.Command("go", "build", "-o", chemin,
		"github.com/dbouzon/osnias-clearing/coord/a2a-agent/cmd/vibe-factice")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build vibe-factice : %v : %s", err, out)
	}
	return chemin, nil
}

// serveurDemo : démarre l'agent deux voies et (option) reste à l'écoute.
func serveurDemo(opts mistral.OptionsVibe, jeton string, ecouter bool) {
	reg, err := mistral.NouveauRegistreSessions(filepath.Join(opts.RacineWorktree, "sessions.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "registre sessions :", err)
		os.Exit(2)
	}
	bin, err := binaireFactice()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	opts.Binaire = bin
	agent := mistral.Agent{Vibe: &mistral.ExecVibe{Opts: opts, Registre: reg}}
	s, err := socle.NouveauServeur("coord-agent-mistral", agent, jeton)
	if err != nil {
		fmt.Fprintln(os.Stderr, "serveur :", err)
		os.Exit(2)
	}
	fmt.Println("AGENT_PRET :", s.URL())
	if ecouter {
		select {}
	}
	s.Fermer()
}

// demoLocal : cycle factice complet : critique de plan (voie inférence)
// puis exécution (voie Vibe) et correction (même session, S16).
func demoLocal() error {
	repertoire, err := os.MkdirTemp("", "coord-agent-depot-")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(repertoire, ".git"), 0o700); err != nil {
		return err
	}
	base, err := os.MkdirTemp("", "coord-agent-wt-")
	if err != nil {
		return err
	}
	regChemin := filepath.Join(base, "sessions.json")
	bin, err := binaireFactice()
	if err != nil {
		return err
	}
	opts := mistral.OptionsVibe{
		Binaire: bin, RepertoireTest: repertoire, RacineWorktree: base,
		LimiteDuree: 30 * time.Second, MaxTurns: 5, MaxTokens: 4000,
		Enveloppe: 5000, SessionLogging: true,
	}
	reg, err := mistral.NouveauRegistreSessions(regChemin)
	if err != nil {
		return err
	}
	agent := mistral.Agent{Vibe: &mistral.ExecVibe{Opts: opts, Registre: reg}}
	const jeton = "jeton-demo-llm03"
	s, err := socle.NouveauServeur("coord-agent-mistral", agent, jeton)
	if err != nil {
		return err
	}
	defer s.Fermer()

	ctx := context.Background()
	rail, err := coorda2a.NouveauRail(ctx, s.URL(), jeton, filepath.Join(base, "registre-demo.json"))
	if err != nil {
		return err
	}

	// 1. Voie inférence : critique de plan, verdict ancré.
	emp := shaEmp("plan-demo-v1")
	charge := "CRITIQUE DE PLAN " + `{"empreinte_attendue":"` + emp + `"}`
	env, _, err := rail.Deleguer(ctx, "critiquer", charge, "demo-critique-1")
	if err != nil {
		return err
	}
	if env.Etat != coorda2a.EtatAchevee {
		return fmt.Errorf("voie inférence : état %s, achevee attendue", env.Etat)
	}
	var verdict map[string]any
	if err := json.Unmarshal([]byte(env.Texte), &verdict); err != nil {
		return fmt.Errorf("verdict non JSON : %w", err)
	}
	fmt.Println("CRITIQUE_PLAN :", env.Texte)

	// 2. Voie Vibe : exécution factice.
	env, _, err = rail.Deleguer(ctx, "executer",
		"VOIE:VIBE MISSION=demo REVISION=r1 TACHE=produire un livrable factice",
		"demo-exec-1")
	if err != nil {
		return err
	}
	if env.Etat != coorda2a.EtatAchevee {
		return fmt.Errorf("voie Vibe : état %s, achevee attendue", env.Etat)
	}
	fmt.Println("EXEC_VIBE : OK (jetons", env.Jetons, ")")

	// 3. Correction : même session, contexte neuf (S16).
	env, _, err = rail.Deleguer(ctx, "executer",
		"VOIE:VIBE MISSION=demo REVISION=r1 CORRECTION=1 TACHE=corriger selon la critique",
		"demo-exec-2")
	if err != nil {
		return err
	}
	if env.Etat != coorda2a.EtatAchevee {
		return fmt.Errorf("correction : état %s", env.Etat)
	}
	if s, ok := reg.Chercher("demo", "r1"); !ok || s.Session == "" {
		return fmt.Errorf("session non enregistrée (S16)")
	}
	fmt.Println("CORRECTION : OK (même session, clé neuve)")
	return nil
}

// shaEmp : empreinte 64 hex d'un libellé (démonstration, ancrage).
func shaEmp(s string) string {
	// empreinte déterministe factice (aucune preuve cryptographique de
	// livrable réel : les livrables du run factice sont de la donnée).
	const hexd = "0123456789abcdef"
	var out []byte
	v := fnv64(s)
	for i := 0; i < 64; i++ {
		out = append(out, hexd[int(v%16)])
		v = v*6364136223846793005 + 1442695040888963407
	}
	return string(out)
}

func fnv64(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

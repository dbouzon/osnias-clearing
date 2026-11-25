// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Command vibe-factice : CLI VIBE SIMULÉE pour la qualification de la
// voie Vibe du connecteur Mistral (M-COORD-LLM-03, ordre #54, plan v2).
// Aucune clé, aucun compte, aucun réseau : la qualification réelle de
// la CLI Vibe est une recette DISTINCTE (B4/B5, autorisation écrite du
// pilote requise). Codes de sortie et artefacts reproduisent le contrat
// documenté de la CLI en mode programmatique (note v1.1 §4.1) :
//
//	0     fini (export.json écrit, sortie JSON sur stdout)
//	1     démarrage impossible
//	2     échec API/runtime
//	3     arrêt avant achèvement (SIGTERM coopératif, limite)
//
// Scénarios pilotables par ARGUMENTS (données, jamais secrets) :
//
//	-factice-code N           force le code de sortie (1/2/3)
//	-factice-code-sec          code 0 SANS export.json valide (S12)
//	-factice-duree-ms N        tâche longue interruptible (S14/T-LONG)
//	-factice-export MODE       ok | absent | corrompu | fuite (S12/S19ter)
//	-factice-approbation       appel d'outil exigeant approbation :
//	                          refus TRACÉ dans warnings, code 3, aucun
//	                          effet de bord (S13/ordre T4)
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type exportFactice struct {
	SessionID        string   `json:"session_id"`
	Outcome          string   `json:"outcome"`
	TokensPrompt     int      `json:"tokens_prompt"`
	TokensCompletion int      `json:"tokens_completion"`
	Warnings         []string `json:"warnings"`
}

func main() {
	var (
		promptFichier = flag.String("prompt-file", "", "")
		worktree      = flag.String("worktree", "", "")
		maxTokens     = flag.Int("max-tokens", 1000, "")
		maxTurns      = flag.Int("max-turns", 10, "")
		output        = flag.String("output", "json", "")
		_             = maxTurns
		_             = output
		timeLimit     = flag.Int("time-limit", 60, "")
		outputDir     = flag.String("output-dir", "", "")
		resume        = flag.String("resume", "", "")
		// factices
		fCode    = flag.Int("factice-code", 0, "")
		fCodeSec = flag.Bool("factice-code-sec", false, "")
		fDureeMS = flag.Int("factice-duree-ms", 0, "")
		fExport  = flag.String("factice-export", "ok", "")
		fApprob  = flag.Bool("factice-approbation", false, "")
		fEnviron = flag.Bool("factice-env-dump", false, "")
		fMuet    = flag.Bool("factice-ignore-sigterm", false, "")
		_        = flag.Bool("trust", false, "")
		_        = flag.Bool("p", false, "")
	)
	flag.Parse()

	// Contexte SIGTERM coopératif (S14 : arrêt coopératif, session
	// résumable => export écrit jusqu'à l'arrêt, exit 3). Variantes
	// scénario débranchées : muet => le SIGTERM est IGNORÉ (S15) SANS
	// jamais annuler le contexte courant.
	var ctx context.Context
	if *fMuet {
		signal.Ignore(syscall.SIGTERM, os.Interrupt)
		ctx = context.Background()
	} else {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
	}
	if *fEnviron {
		// Scénario T-ENV/S19bis : l'environnement hérité serait un canal
		// d'exfiltration d'identifiants ; la liste blanche doit rendre
		// ce dump VIDE (aucun GITHUB_TOKEN ne peut traverser).
		for _, kv := range os.Environ() {
			fmt.Fprintln(os.Stderr, kv)
		}
	}

	prompt := ""
	if *promptFichier != "" {
		b, err := os.ReadFile(*promptFichier)
		if err != nil {
			fmt.Fprintln(os.Stderr, "DEMARRAGE_IMPOSSIBLE: prompt illisible")
			os.Exit(1)
		}
		prompt = string(b)
	}
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "DEMARRAGE_IMPOSSIBLE: output-dir requis")
		os.Exit(1)
	}

	// Borne de durée effective : la plus petite de --time-limit et du
	// scénario factice (éventuel).
	limite := time.Duration(*timeLimit) * time.Second
	if *fDureeMS > 0 {
		d := time.Duration(*fDureeMS) * time.Millisecond
		if d < limite {
			limite = d
		}
	}
	deadline := time.Now().Add(limite)
	if *fDureeMS > 0 {
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				ecrireExport(*outputDir, *worktree, *maxTokens, "canceled", nil)
				fmt.Fprintln(os.Stderr, "ARRET_COOPERATIF: SIGTERM reçu")
				os.Exit(3)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	if err := ctx.Err(); err != nil {
		ecrireExport(*outputDir, *worktree, *maxTokens, "canceled", nil)
		os.Exit(3)
	}

	// S13 : appel d'outil exigeant une approbation en programmatique
	// SANS --auto-approve => REFUS TRACÉ, aucun effet de bord, code 3.
	if *fApprob {
		ecrireExportMode(*outputDir, *worktree, *maxTokens, "interrupted",
			[]string{"APPROBATION_REFUSEE: outil shell exigé en mode programmatique sans --auto-approve : refus tracé, aucun effet de bord"},
			*fExport)
		os.Exit(3)
	}

	suffixe := ""
	if *resume != "" {
		suffixe = "CORRECTION_APPLIQUEE: " + *resume + " "
	}
	sortie := map[string]any{"messages": []any{map[string]any{
		"role": "assistant", "content": suffixe + limiter(prompt, 200),
	}}}
	bSortie, _ := json.Marshal(sortie)
	fmt.Println(string(bSortie))

	// Scénario code 0 sans export valide (S12).
	if *fCodeSec {
		os.Exit(0)
	}
	if *fCode != 0 {
		os.Exit(*fCode)
	}

	ecrireExportMode(*outputDir, *worktree, *maxTokens, "success", nil, *fExport)
	os.Exit(0)
}

// ecrireExport : export.json nominal du run.
func ecrireExport(dir, worktree string, maxTokens int, outcome string, warnings []string) {
	ecrireExportMode(dir, worktree, maxTokens, outcome, warnings, "ok")
}

// ecrireExportMode : écrit export.json selon le scénario (ok | absent |
// corrompu | fuite). La FUITE écrit un pseudo-secret : le SUPERVISEUR
// doit l'expurger ou échouer (S19ter, fail closed).
func ecrireExportMode(dir, worktree string, maxTokens int, outcome string, warnings []string, mode string) {
	if mode == "absent" {
		return
	}
	e := exportFactice{
		SessionID:        sessionID(worktree),
		Outcome:          outcome,
		TokensPrompt:     215,
		TokensCompletion: 521,
		Warnings:         warnings,
	}
	if mode == "fuite" {
		e.Warnings = append(e.Warnings,
			"sk-FACTICEfuite1234567890XY : pseudo-secret de test dans export.json (S19ter)")
	}
	b, _ := json.MarshalIndent(e, "", "  ")
	if mode == "corrompu" {
		b = []byte(`{"session_id": "ok", "outcome":`) // JSON tronqué volontairement
	}
	chemin := filepath.Join(dir, "export.json")
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	os.Rename(tmp, chemin)
}

// sessionID : identifiant de session DÉTERMINISTE du worktree (la paire
// (session, worktree) est l'ancre de reprise).
func sessionID(worktree string) string {
	h := sha256.Sum256([]byte(worktree))
	return "sess-" + hex.EncodeToString(h[:8])
}

func limiter(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// garde : usage d'un argument inconnu toléré (args = données).
var _ = fmt.Sprintf

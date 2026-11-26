// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// claudecode_llm05_test.go : qualification E2E de la voie Claude Code
// supervisée (M-COORD-LLM-05, T4/CC15) : mission SIMULÉE où l'agent
// Anthropic DEUX VOIES exécute par VOIE:CODE (claude-factice supervisé)
// et l'agent OpenAI (inférence factice) réceptionne indépendamment.
// Boucle de correction COMPLÈTE : A_CORRIGER piloté PAR LE LIVRABLE →
// correction sur la MÊME session (--resume, run 2) → ACCEPTEE (verdict
// ancré + verdict_id/verdict_url) → ACK_RECU ; clôture REFUSÉE hors
// pilote. Aucune clé, aucun réseau fournisseur, 0 €.
package a2a

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// lancerAgentModule : lance coord-agent avec le répertoire du module
// comme répertoire de travail (build de la CLI factice au démarrage),
// attend AGENT_PRET (borne 60 s), arrête au Cleanup.
func lancerAgentModule(t *testing.T, binaire, dir string, args ...string) string {
	t.Helper()
	c := exec.Command(binaire, args...)
	c.Dir = dir
	stdout, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	sortie := make(chan string, 1)
	echec := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if u, ok := strings.CutPrefix(sc.Text(), "AGENT_PRET : "); ok {
				sortie <- strings.TrimSpace(u)
				return
			}
		}
		if err := sc.Err(); err != nil {
			echec <- err
		} else {
			echec <- errModuleQuitte
		}
	}()
	select {
	case u := <-sortie:
		t.Cleanup(func() {
			if c.Process != nil {
				_ = c.Process.Kill()
				_, _ = c.Process.Wait()
			}
		})
		return u
	case err := <-echec:
		t.Fatalf("démarrage agent (module) : %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("AGENT_PRET attendu depuis 60s (timeout)")
	}
	return ""
}

var errModuleQuitte = errors.New("agents : le sous-processus a quitté avant AGENT_PRET")

// compilerAgent : construit le binaire coord-agent (module séparé,
// sous-processus ; suffixe .exe sous Windows, leçon #55).
func compilerAgent(t *testing.T) string {
	t.Helper()
	goBin := "go"
	if _, err := exec.LookPath("go"); err != nil {
		goBin = "/usr/local/go/bin/go"
	}
	suffixe := ""
	if runtime.GOOS == "windows" {
		suffixe = ".exe"
	}
	binaire := filepath.Join(t.TempDir(), "coord-agent"+suffixe)
	build := exec.Command(goBin, "build", "-o", binaire, "./cmd/coord-agent")
	build.Dir = filepath.Join("..", "..", "..", "a2a-agent")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build coord-agent : %v\n%s", err, out)
	}
	return binaire
}

// TestE2ELLM05ClaudeCodeBoucleCorrection : CC15 — mission simulée
// Claude Code exécutant / OpenAI réceptionnaire, boucle complète.
func TestE2ELLM05ClaudeCodeBoucleCorrection(t *testing.T) {
	ctx := context.Background()
	binaire := compilerAgent(t)
	const jeton = "jeton-banc-llm05"
	// Exécutant : agent Anthropic DEUX VOIES (inférence + VOIE:CODE).
	// Répertoire de travail = module a2a-agent : l'agent construit sa
	// CLI factice (go build cmd/claude-factice) au démarrage.
	urlExec := lancerAgentModule(t, binaire, filepath.Join("..", "..", "..", "a2a-agent"),
		"-fournisseur", "anthropic", "-jeton", jeton, "-ecouter")
	// Réceptionnaire : agent OpenAI (inférence factice, LLM-04).
	urlRec := lancerAgentLLM04(t, binaire, "-fournisseur", "openai",
		"-jeton", jeton, "-ecouter")

	execID, recID := "agent-claude-exec", "agent-openai-rec"
	declare := func(id, url string) connecteurs.ConfigFournisseur {
		return connecteurs.ConfigFournisseur{
			ID: id, Endpoint: url, Modele: "agent-factice",
			Format: "openai", TimeoutMS: 60000, TailleMaxReponse: 1 << 20,
			TarifEntreeCentimesParMillion: 200,
			TarifSortieCentimesParMillion: 1000,
			TarifRequeteCentimes:          1,
			JetonsEntreeMaxParTentative:   20000,
			JetonsSortieMaxParTentative:   20000,
			CleEnv:                        "COORD_JETON_RAIL_TEST",
		}
	}
	regs := map[string]adaptateurs.Adaptateur{}
	for _, f := range []struct{ id, url string }{{execID, urlExec}, {recID, urlRec}} {
		a, err := NouvelAdaptateurA2AJeton(ctx, declare(f.id, f.url), jeton,
			t.TempDir()+"/registre-"+f.id+".json")
		if err != nil {
			t.Fatalf("adaptateur %s : %v", f.id, err)
		}
		regs[f.id] = a
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, regs)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	const issue = "llm05-claude-code-boucle"
	plaf := budget.Plafonds{
		Appels: map[string]int{execID: 12, recID: 12},
		Jetons: map[string]int{execID: 400000, recID: 400000},
	}
	if err := orch.NouvelleMissionExplicit(issue, "cc-exec", "oai-rec",
		execID, recID, roles, plaf); err != nil {
		t.Fatal(err)
	}
	etat := func() string {
		enr, err := ref.Obtenir(issue)
		if err != nil {
			t.Fatal(err)
		}
		return enr.Mission.State
	}

	// 1-2. Exécution VOIE:CODE (run 1) : le livrable porte le marqueur
	// de rétroaction A_CORRIGER (données, aucune instruction exécutée).
	mandat := "VOIE:CODE MISSION=" + issue + " REVISION=r1 CORRECTION=0 TACHE=produit le livrable SCENARIO:LIVRABLE_A_CORRIGER"
	res, err := orch.Executer(ctx, issue, "cc-exec", mandat)
	if err != nil {
		t.Fatalf("exécution VOIE:CODE : %v", err)
	}
	livrable1 := res.Sortie
	if !strings.Contains(livrable1, "run 1") || !strings.Contains(livrable1, "SCENARIO:A_CORRIGER") {
		t.Fatalf("livrable run 1 avec marqueur attendu : %s", livrable1)
	}
	if _, err := orch.Soumettre(ctx, issue, "cc-exec", livrable1); err != nil {
		t.Fatalf("soumission 1 : %v", err)
	}
	// 3. Réception OpenAI : A_CORRIGER → EN_CORRECTION.
	v, _, err := orch.Recevoir(ctx, issue, "oai-rec", livrable1)
	if err != nil {
		t.Fatalf("réception 1 : %v", err)
	}
	if v.Verdict != "A_CORRIGER" || etat() != spec.EtatEnCorrection {
		t.Fatalf("A_CORRIGER/EN_CORRECTION attendus : %s / %s", v.Verdict, etat())
	}
	// 4. Correction : MÊME session --resume (run 2), livrable PROPRE.
	corrige := "VOIE:CODE MISSION=" + issue + " REVISION=r1 CORRECTION=1 TACHE=corrige le livrable CORRECTION_NETTOYEE SCENARIO:BASE"
	res2, err := orch.Executer(ctx, issue, "cc-exec", corrige)
	if err != nil {
		t.Fatalf("correction : %v", err)
	}
	livrable2 := res2.Sortie
	if !strings.Contains(livrable2, "run 2") || strings.Contains(livrable2, "SCENARIO:A_CORRIGER") {
		t.Fatalf("livrable corrigé (run 2, marqueur retiré) attendu : %s", livrable2)
	}
	if _, err := orch.Soumettre(ctx, issue, "cc-exec", livrable2); err != nil {
		t.Fatalf("nouvelle soumission : %v", err)
	}
	v2, _, err := orch.Recevoir(ctx, issue, "oai-rec", livrable2)
	if err != nil {
		t.Fatalf("réception 2 : %v", err)
	}
	if v2.Verdict != "ACCEPTEE" {
		t.Fatalf("ACCEPTEE attendu : %+v", v2)
	}
	if v2.VerdictID == "" || v2.VerdictURL == "" {
		t.Fatalf("verdict_id/verdict_url exigés : %+v", v2)
	}
	// 5. ACK → ACK_RECU ; clôture refusée hors pilote.
	if err := orch.Acker(issue, "cc-exec"); err != nil {
		t.Fatalf("ACK : %v", err)
	}
	if etat() != spec.EtatACKRecu {
		t.Fatalf("ACK_RECU attendu : %s", etat())
	}
	if err := orch.Cloturer(issue); err == nil ||
		!strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
		t.Fatalf("clôture hors pilote interdite : %v", err)
	}
}

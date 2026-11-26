// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// M-COORD-LLM-04, T5 : QUALIFICATION CROISÉE des quatre fournisseurs
// (Mistral, Anthropic, OpenAI, Gemini) — 12 couples ordonnés exéc≠récep,
// UN cycle complet par couple : critique de plan → exécution simulée →
// soumission → A_CORRIGER → correction → ACCEPTEE → ACK_RECU (la clôture
// reste réservée au pilote, vérifiée refusée).
//
// Architecture HONNÊTE : les quatre agents sont de vrais SOUS-PROCESSUS du
// binaire coord-a2a-agent (moteur et agents sont des modules Go distincts :
// internal/a2a ne peut pas les importer), démarrés en écoute loopback.
// L'exécution Mistral de la matrice passe par l'agent D'INFÉRENCE
// (-fournisseur mistral, profil stop-one), distinct de l'agent Mistral
// historique deux voies (VOIE:VIBE), inchangé. Aucune clé, aucun réseau
// fournisseur, 0 EUR : le mode est SIMULÉ de bout en bout.
//
// La boucle A_CORRIGER est pilotée PAR LE LIVRABLE : la première soumission
// contient le marqueur SCENARIO:A_CORRIGER, le factice réception rend donc
// A_CORRIGER ; le livrable corrigé ne le contient plus → ACCEPTEE. Les
// verdicts de réCEPTION émettent verdict_id/verdict_url (FACTICE-LLM04).
package a2a

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/connecteurs"
	"coordengine/internal/missions"
	"coordengine/internal/spec"

	coordbudget "coordengine/internal/budget"
)

// agentLLM04 : un agent serveur A2A démarré en sous-processus.
type agentLLM04 struct {
	url string
	cmd *exec.Cmd
}

// fermer : arrêt du sous-processus (idempotent).
func (a *agentLLM04) fermer() {
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_, _ = a.cmd.Process.Wait()
		a.cmd = nil
	}
}

// lancerAgentLLM04 : démarre un sous-processus coord-agent en écoute et
// attend sa ligne AGENT_PRET <URL> (attente bornée, fail closed).
func lancerAgentLLM04(t *testing.T, binaire string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binaire, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	agent := &agentLLM04{cmd: cmd}
	t.Cleanup(agent.fermer)
	sortie := make(chan string, 1)
	echec := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			l := sc.Text()
			if u, ok := strings.CutPrefix(l, "AGENT_PRET : "); ok {
				sortie <- strings.TrimSpace(u)
				return
			}
		}
		if err := sc.Err(); err != nil {
			echec <- err
		} else {
			echec <- errors.New("agents : le sous-processus a quitté avant AGENT_PRET")
		}
	}()
	select {
	case u := <-sortie:
		return u
	case err := <-echec:
		t.Fatalf("démarrage agent : %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("démarrage agent : AGENT_PRET attendu depuis 60s (timeout)")
	}
	return ""
}

// TestMatriceLLM04DouzeCouples : la qualification croisée T5.
func TestMatriceLLM04DouzeCouples(t *testing.T) {
	ctx := context.Background()

	// 1. Binaire coord-agent (module séparé, build par sous-processus ;
	// suffixe .exe sous Windows, leçon M-BRIDGE/#55).
	goBin := "go"
	if _, err := exec.LookPath("go"); err != nil {
		goBin = "/usr/local/go/bin/go"
	}
	repertoireAgents := filepath.Join("..", "..", "..", "a2a-agent")
	suffixe := ""
	if runtime.GOOS == "windows" {
		suffixe = ".exe"
	}
	binaire := filepath.Join(t.TempDir(), "coord-agent"+suffixe)
	build := exec.Command(goBin, "build", "-o", binaire, "./cmd/coord-agent")
	build.Dir = repertoireAgents
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build coord-agent : %v\n%s", err, out)
	}

	// 2. Quatre agents en écoute loopback, même jeton de rail (banc).
	const jeton = "jeton-banc-llm04"
	fournisseurs := []string{"anthropic", "openai", "gemini", "mistral"}
	urls := map[string]string{}
	for _, f := range fournisseurs {
		urls[f] = lancerAgentLLM04(t, binaire,
			"-fournisseur", f, "-jeton", jeton, "-ecouter")
	}

	// 3. Adaptateurs A2A du moteur vers chaque agent (registresDistincts).
	registres := map[string]adaptateurs.Adaptateur{}
	for _, f := range fournisseurs {
		cfg := connecteurs.ConfigFournisseur{
			ID: "agent-" + f, Endpoint: urls[f], Modele: "agent-factice",
			Format: "openai", TimeoutMS: 5000, TailleMaxReponse: 1 << 20,
			TarifEntreeCentimesParMillion: 200,
			TarifSortieCentimesParMillion: 1000,
			TarifRequeteCentimes:          1,
			JetonsEntreeMaxParTentative:   20000,
			JetonsSortieMaxParTentative:   20000,
			CleEnv:                        "COORD_JETON_RAIL_TEST",
		}
		a, err := NouvelAdaptateurA2AJeton(ctx, cfg, jeton,
			filepath.Join(t.TempDir(), "registre-"+f+".json"))
		if err != nil {
			t.Fatalf("adaptateur %s : %v", f, err)
		}
		registres["agent-"+f] = a
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, registres)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}

	// 4. Les 12 couples ordonnés, cycle complet par couple.
	couples, acceptes := 0, 0
	for _, execF := range fournisseurs {
		for _, recF := range fournisseurs {
			if execF == recF {
				continue
			}
			couples++
			execID, recID := "agent-"+execF, "agent-"+recF
			issue := "llm04-matrice-" + execF + "-" + recF
			plaf := coordbudget.Plafonds{
				Appels: map[string]int{execID: 12, recID: 12},
				Jetons: map[string]int{execID: 400000, recID: 400000},
			}
			if err := orch.NouvelleMissionExplicit(issue, execF+"-exec", recF+"-recep",
				execID, recID, roles, plaf); err != nil {
				t.Fatalf("couple %s : création : %v", issue, err)
			}
			etat := func() string {
				enr, err := ref.Obtenir(issue)
				if err != nil {
					t.Fatal(err)
				}
				return enr.Mission.State
			}

			// Critique de plan par le réceptionnaire (ancrée PLAN_ACCEPTE).
			vp, err := orch.CritiquerPlan(ctx, issue, recF+"-recep",
				"Plan : qualification croisée "+issue)
			if err != nil {
				t.Fatalf("couple %s : critique plan : %v", issue, err)
			}
			if vp.Verdict != "PLAN_ACCEPTE" {
				t.Fatalf("couple %s : PLAN_ACCEPTE attendu : %+v", issue, vp)
			}

			// Exécution simulée + soumission PORTANT le marqueur de
			// rétroaction : la première réception rend A_CORRIGER.
			livrable := "LIVRABLE LLM04 " + issue + " SCENARIO:A_CORRIGER"
			if _, err := orch.Executer(ctx, issue, execF+"-exec", "mandat "+issue); err != nil {
				t.Fatalf("couple %s : exécution : %v", issue, err)
			}
			if _, err := orch.Soumettre(ctx, issue, execF+"-exec", livrable); err != nil {
				t.Fatalf("couple %s : soumission : %v", issue, err)
			}
			v, _, err := orch.Recevoir(ctx, issue, recF+"-recep", livrable)
			if err != nil {
				t.Fatalf("couple %s : réception 1 : %v", issue, err)
			}
			if v.Verdict != "A_CORRIGER" || etat() != spec.EtatEnCorrection {
				t.Fatalf("couple %s : A_CORRIGER/EN_CORRECTION attendus : %s / %s",
					issue, v.Verdict, etat())
			}

			// Correction + nouveau livrable SANS marqueur → ACCEPTEE.
			corrige := "LIVRABLE LLM04 " + issue + " CORRIGE"
			if _, err := orch.Executer(ctx, issue, execF+"-exec", "correction "+issue); err != nil {
				t.Fatalf("couple %s : correction : %v", issue, err)
			}
			if _, err := orch.Soumettre(ctx, issue, execF+"-exec", corrige); err != nil {
				t.Fatalf("couple %s : nouvelle soumission : %v", issue, err)
			}
			v2, _, err := orch.Recevoir(ctx, issue, recF+"-recep", corrige)
			if err != nil {
				t.Fatalf("couple %s : réception 2 : %v", issue, err)
			}
			if v2.Verdict != "ACCEPTEE" {
				t.Fatalf("couple %s : ACCEPTEE attendu : %+v", issue, v2)
			}
			if v2.VerdictID == "" || v2.VerdictURL == "" {
				t.Fatalf("couple %s : verdict_id/verdict_url exigés : %+v", issue, v2)
			}

			// ACK exécutant → ACK_RECU ; clôture REFUSÉE hors pilote.
			if err := orch.Acker(issue, execF+"-exec"); err != nil {
				t.Fatalf("couple %s : ACK : %v", issue, err)
			}
			if etat() != spec.EtatACKRecu {
				t.Fatalf("couple %s : ACK_RECU attendu : %s", issue, etat())
			}
			if err := orch.Cloturer(issue); err == nil ||
				!strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
				t.Fatalf("couple %s : clôture hors pilote interdite : %v", issue, err)
			}
			acceptes++
		}
	}
	if couples != 12 || acceptes != 12 {
		t.Fatalf("12 couples admis attendus : %d créés, %d au cycle complet",
			couples, acceptes)
	}
	fmt.Println("MATRICE_LLM04 : 12/12 couples, cycles complets (critique→A_CORRIGER→ACCEPTEE→ACK_RECU)")
}

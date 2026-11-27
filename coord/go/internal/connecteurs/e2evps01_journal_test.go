// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Reproduction M-COORD-E2E-VPS-01 (issue #74) : deux appels réels DISTINCTS
// du même adaptateur produisant des réponses IDENTIQUES (comportement du
// FacticeBackend coord.llm.v1 sans marqueur : sortie constante) sérialisaient
// en lignes de journal canoniques IDENTIQUES ; la déduplication de
// persist.JournalExecute supprimait alors silencieusement la seconde entrée
// APPEL_REEL. Le journal durable ne reflétait plus la réalité des appels
// (réserve héritée M-COORD-DEP-03/#72, reproduite proprement sur le VPS :
// 8 APPEL_RESERVE pour 6 APPEL_REEL sur le cycle d'incident).
//
// Correctif couvert : les entrées APPEL_REEL et RECEPTION portent leur
// cle_idempotence (unique par appel, RFC3339Nano dans empreinteCharge),
// ce qui distingue tout couple d'événements distincts sans affaiblir la
// déduplication des rejeus exacts.
package connecteurs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/missions"
)

// TestReproAppelsReelsDistinctsJournalises : deux Executer (V1 puis V2 après
// A_CORRIGER) dont les réponses factices sont identiques doivent produire
// DEUX entrées APPEL_REEL distinctes dans le journal durable.
func TestReproAppelsReelsDistinctsJournalises(t *testing.T) {
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}

	// Exécutant factice à réponse CONSTANTE (fidèle au FacticeBackend du
	// pont coord.llm.v1 sans marqueur : la sortie ne dépend pas de la charge).
	appelsExec := 0
	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appelsExec++
		w.Write(reponseChat("LIVRABLE CONSTANT (sans secret)", "exec-modele", 1500))
	}))
	t.Cleanup(exec.Close)

	// Réceptionnaire factice : critique de plan acceptée ; première
	// réception A_CORRIGER avec 2 points, verdict_id CONSTANT.
	appelsRecep := 0
	recep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appelsRecep++
		var corps corpsChat
		_ = json.NewDecoder(r.Body).Decode(&corps)
		contenu := corps.Messages[len(corps.Messages)-1].Content
		if strings.Contains(contenu, "CRITIQUE DE PLAN") {
			out, _ := json.Marshal(map[string]any{
				"verdict": "PLAN_ACCEPTE", "points": []string{"rien de bloquant"},
				"empreinte_attendue": extraireEmpreinte(t, contenu),
			})
			w.Write(reponseChat(string(out), "recep-modele", 300))
			return
		}
		if strings.Contains(contenu, "RECEPTION INDEPENDANTE") {
			out, _ := json.Marshal(map[string]any{
				"verdict": "A_CORRIGER",
				"points": []string{
					"corriger le calcul affiche",
					"preciser l unite monetaire",
				},
				"empreinte_attendue": extraireEmpreinte(t, contenu),
				"verdict_id":         "factice-verdict-a-corriger",
				"verdict_url":        "urn:coord:factice",
			})
			w.Write(reponseChat(string(out), "recep-modele", 300))
			return
		}
		w.Write(reponseChat("SOUMISSION COMPRISE", "recep-modele", 300))
	}))
	t.Cleanup(recep.Close)

	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral", Endpoint: exec.URL, Modele: "exec-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
		{ID: "openai", Endpoint: recep.URL, Modele: "recep-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
	}}
	execA, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur executant : %v", err)
	}
	recepA, err := cfg.NouveauHTTPEndPoint("openai")
	if err != nil {
		t.Fatalf("connecteur réceptionnaire : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{
		"mistral": execA, "openai": recepA,
	})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}

	const issue = "e2evps01-repro-journal"
	if err := orch.NouvelleMission(issue, "mistral", "openai",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()

	// Cycle V1 : exécution, soumission, réception A_CORRIGER.
	if _, err := orch.Executer(ctx, issue, "mistral",
		"MANDAT V1 : livrable de demonstration"); err != nil {
		t.Fatalf("exécution V1 : %v", err)
	}
	if _, err := orch.Soumettre(ctx, issue, "mistral",
		"LIVRABLE CONSTANT (sans secret)"); err != nil {
		t.Fatalf("soumission V1 : %v", err)
	}
	v1, _, err := orch.Recevoir(ctx, issue, "openai",
		"LIVRABLE CONSTANT (sans secret)")
	if err != nil {
		t.Fatalf("réception V1 : %v", err)
	}
	if v1.Verdict != "A_CORRIGER" {
		t.Fatalf("verdict V1 inattendu : %s", v1.Verdict)
	}

	// Cycle V2 : correction, NOUVELLE exécution dont la réponse factice est
	// IDENTIQUE à celle de V1 (sortie constante).
	if _, err := orch.Executer(ctx, issue, "mistral",
		"MANDAT V2 : livrable corrige"); err != nil {
		t.Fatalf("exécution V2 : %v", err)
	}

	if appelsExec != 3 { // executer V1 + soumettre V1 (adaptateur exécutant) + executer V2
		t.Fatalf("appels exécutant : %d (attendu 3)", appelsExec)
	}
	if appelsRecep != 1 { // réception V1 (pas de critique de plan dans ce test)
		t.Fatalf("appels réceptionnaire : %d (attendu 1)", appelsRecep)
	}

	// Le journal durable doit porter DEUX entrées APPEL_REEL d'action
	// executer : deux événements distincts, réponses identiques.
	raw, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	constant := 0
	cles := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l == "" {
			continue
		}
		var j struct {
			Type           string `json:"type"`
			Action         string `json:"action"`
			CleIdempotence string `json:"cle_idempotence"`
		}
		if errJ := json.Unmarshal([]byte(l), &j); errJ != nil {
			t.Fatalf("ligne journal invalide : %v", errJ)
		}
		if j.Type == "APPEL_REEL" && j.Action == "executer" {
			constant++
			if j.CleIdempotence == "" {
				t.Fatalf("APPEL_REEL sans cle_idempotence : %s", l)
			}
			cles[j.CleIdempotence] = true
		}
	}
	if constant != 2 {
		t.Fatalf("REPRO : %d entrée(s) APPEL_REEL executer dans le journal, "+
			"2 appels réels distincts ont eu lieu (déduplication de lignes "+
			"identiques = entrée manquante)", constant)
	}
	if len(cles) != 2 {
		t.Fatalf("clés d'idempotence APPEL_REEL : %d distinctes (attendu 2)", len(cles))
	}
}

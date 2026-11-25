// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
)

// Reproductions de la contre-réception A_CORRIGER id 7154948522 (mission
// M-COORD-PILOT-03) : le plafond monétaire PlafondCentimes est VALIDÉ dans
// connecteurs.go mais jamais TRANSMIS au moteur budgétaire, et le surplus est
// comptabilisé après l'appel, lorsque la dépense est déjà engagée. Le plafond
// monétaire n'est donc PAS démontré opposable.
//
// Ces deux tests, écrits AVANT toute correction, compilent sur le code
// incriminé et échouent : ils matérialisent l'anomalie. Après correction ils
// doivent passer sans être modifiés (garde-fous permanents).

// Repro A : le budget ne porte AUCUN plafond monétaire ni AUCUN compteur de
// réservation en centimes : la dépense réelle n'est bornée par rien.
func TestReproABudgetSansPlafondMonetaire(t *testing.T) {
	if _, ok := reflect.TypeOf(budget.Plafonds{}).FieldByName("Centimes"); !ok {
		t.Fatalf("REPRO A : budget.Plafonds n'expose AUCUN plafond monétaire " +
			"(champ Centimes) : le plafond de l'autorisation pilote ne peut pas " +
			"être rendu opposable — anomalie 7154948522")
	}
	if _, ok := reflect.TypeOf(budget.Etat{}).FieldByName("CentimesReserves"); !ok {
		t.Fatalf("REPRO A : budget.Etat ne compte AUCUN centime réservé : " +
			"aucune réservation monétaire durable avant appel — anomalie 7154948522")
	}
}

// Repro B : une mission dont l'adaptateur est RÉEL (non loopback) doit refuser
// tout appel tant qu'aucun plafond monétaire n'est armé (fail closed). Sur le
// code incriminé, l'appel PART et réussit : le plafond monétaire déclaré dans
// l'autorisation pilote n'est opposé à RIEN.
func TestReproBAppelReelSansPlafondMonetaireArme(t *testing.T) {
	appels := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels++
		w.Write(reponseChat("LIVRABLE FACTICE", "exec-modele", 1500))
	}))
	t.Cleanup(s.Close)
	// White-box : connecteur déclaré RÉEL (simuleLoc=false) pointant vers un
	// serveur factice loopback — AUCUNE donnée, AUCUNE clé, AUCUN réseau
	// sortant, 0 € ; seule la voûte budgétaire monétaire est testée.
	h := &HTTPEndPoint{
		id: "mistral", endpoint: s.URL, modele: "exec-modele",
		client:    &http.Client{Timeout: 5 * time.Second},
		tailleMax: 65536, retraits: 0, simuleLoc: false,
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"mistral": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "repro-monetarie"
	p := plafondsTest() // plafonds d'appels et de jetons armés, AUCUN plafond monétaire
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "monetaire") {
		t.Fatalf("REPRO B : un appel réel sans plafond monétaire armé doit être REFUSÉ "+
			"avant tout octet émis ; ici l'appel est parti (%d requêtes reçues) : %+v",
			appels, err)
	}
	if appels != 0 {
		t.Fatalf("REPRO B : aucun octet n'a dû être émis vers le fournisseur : %d requêtes", appels)
	}
}

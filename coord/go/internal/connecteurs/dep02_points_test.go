// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"strings"
	"testing"

	"coordengine/internal/spec"
)

// ---- M-COORD-DEP-02 (revue PR #71, tour 3) : les points du verdict sont
// validés au niveau JSON BRUT à l'entrée fournisseur, avant tout aplatissage
// ([]string transformerait [null] en [""] sans erreur), et un verdict sans
// champ points reste recevable (compatibilité). ----

func TestVerdictReceptionDeSortiePoints(t *testing.T) {
	emp := empreinteCharge("LIVRABLE-TEMoin")
	base := `{"verdict":"ACCEPTEE","empreinte_attendue":"` + emp + `","verdict_id":"v","verdict_url":"u"`
	cas := []struct {
		nom    string
		points string
		rejet  bool
		motif  string
	}{
		{"absent", base + `}`, false, ""},
		{"liste vide", base + `,"points":[]}`, false, ""},
		{"liste valide", base + `,"points":["corriger X"]}`, false, ""},
		{"null explicite", base + `,"points":null}`, true, "points"},
		{"element null", base + `,"points":[null]}`, true, "points"},
		{"element null en queue", base + `,"points":["corriger X",null]}`, true, "points"},
		// [42] est déjà rejeté par le décodage strict de la structure
		// (fail closed amont) : l'essentiel est qu'il ne passe JAMAIS.
		{"element non chaine", base + `,"points":[42]}`, true, ""},
	}
	for _, c := range cas {
		_, err := VerdictReceptionDeSortie(c.points, emp)
		if c.rejet && err == nil {
			t.Fatalf("%s : points invalides acceptés : %s", c.nom, c.points)
		}
		if !c.rejet && err != nil {
			t.Fatalf("%s : verdict valide rejeté : %v", c.nom, err)
		}
		if c.rejet && c.motif != "" && err != nil && !strings.Contains(err.Error(), c.motif) {
			t.Fatalf("%s : motif de rejet inattendu : %v", c.nom, err)
		}
	}
}

func TestVerdictPlanDeSortiePoints(t *testing.T) {
	emp := empreinteCharge("PLAN-TEMoin")
	base := `{"verdict":"PLAN_ACCEPTE","empreinte_attendue":"` + emp + `"`
	if _, err := VerdictPlanDeSortie(base+`}`, emp); err != nil {
		t.Fatalf("verdict de plan sans champ points doit rester accepté : %v", err)
	}
	if _, err := VerdictPlanDeSortie(base+`,"points":[null]}`, emp); err == nil {
		t.Fatal("points [null] du plan doivent être refusés (fail closed)")
	}
	if _, err := VerdictPlanDeSortie(base+`,"points":["réserve mineure"]}`, emp); err != nil {
		t.Fatalf("points valides du plan rejetés : %v", err)
	}
}

// receptionViaBanc mène la mission jusqu'à la réception : le verdict brut
// du réceptionnaire factice est configurable via pointsReception (JSON brut).
func receptionViaBanc(t *testing.T, pointsBrut, nom string) (*banc, error, string) {
	t.Helper()
	b := nouveauBanc(t)
	b.verdictReception = "ACCEPTEE"
	b.pointsReception = &pointsBrut
	issue := "coord-dep02-points-" + nom
	ctx := context.Background()
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat v1")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	_, _, err = b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie)
	return b, err, issue
}

func TestReceptionPointsNullFournisseurRefuse(t *testing.T) {
	// Revue PR #71, tour 3 : la réponse du fournisseur contenant
	// "points":[null] doit être REFUSÉE à l'entrée (elle se décode en [""]),
	// la soumission reste en attente, aucune décision n'est commitée.
	b, err, issue := receptionViaBanc(t, "[null]", "elementnull")
	if err == nil {
		t.Fatal("un verdict avec points [null] doit être refusé via Recevoir")
	}
	if !strings.Contains(err.Error(), "points") {
		t.Fatalf("motif de rejet inattendu : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("la soumission doit rester en attente, état : %s", b.etat(t, issue))
	}
	enr, err := b.ref.Obtenir(issue)
	if err != nil {
		t.Fatalf("obtenir : %v", err)
	}
	if len(enr.Mission.Decisions) != 0 {
		t.Fatalf("aucune décision ne doit être commitée, obtenu %d", len(enr.Mission.Decisions))
	}
}

func TestReceptionSansChampPointsAcceptee(t *testing.T) {
	// Revue PR #71, tour 3 : un verdict SANS champ points, jusque-là
	// accepté, doit rester recevable (jamais réencodé en null) : la
	// décision est commitée avec une liste de points vide.
	b, err, issue := receptionViaBanc(t, "", "absent")
	if err != nil {
		t.Fatalf("un verdict sans champ points doit rester accepté : %v", err)
	}
	if b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("état attendu ACCEPTEE : %s", b.etat(t, issue))
	}
	enr, err := b.ref.Obtenir(issue)
	if err != nil {
		t.Fatalf("obtenir : %v", err)
	}
	if len(enr.Mission.Decisions) != 1 {
		t.Fatalf("une décision attendue, obtenu %d", len(enr.Mission.Decisions))
	}
	d := enr.Mission.Decisions[0]
	if d.Verdict != "ACCEPTEE" {
		t.Fatalf("verdict attendu ACCEPTEE : %s", d.Verdict)
	}
	if len(d.Points) != 0 {
		t.Fatalf("points doit rester vide quand le champ est absent : %+v", d.Points)
	}
}

func TestReceptionPointsNullExpliciteFournisseurRefuse(t *testing.T) {
	// null explicite = charge tronquée : refusé à l'entrée, comme [null].
	_, err, _ := receptionViaBanc(t, "null", "nullexplicite")
	if err == nil {
		t.Fatal("un verdict avec points null explicite doit être refusé via Recevoir")
	}
	if !strings.Contains(err.Error(), "points") {
		t.Fatalf("motif de rejet inattendu : %v", err)
	}
}

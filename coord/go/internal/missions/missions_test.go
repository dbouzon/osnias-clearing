// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package missions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/budget"
	"coordengine/internal/policy"
)

func configValide() ([]string, map[string][]string, []policy.Identity) {
	adapters := []string{"mistral-simule", "openai-simule"}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	identites := []policy.Identity{
		{ID: "exec-sim", Role: "EXECUTANT", QualifiedBy: "mistral-simule"},
		{ID: "recep-sim", Role: "RECEPTIONNAIRE", QualifiedBy: "openai-simule"},
		{ID: "pilote-local", Role: "PILOTE", QualifiedBy: "mistral-simule"},
	}
	return adapters, roles, identites
}

func TestCreationEtRelecture(t *testing.T) {
	ref, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, r, i := configValide()
	enr, err := ref.Creer("#mvp-01", a, r, i, budget.Plafonds{
		Appels: map[string]int{"mistral-simule": 5, "openai-simule": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if enr.Mission.State != "PLANIFIEE" {
		t.Fatal("etat initial PLANIFIEE attendu, obtenu " + enr.Mission.State)
	}
	lu, err := ref.Obtenir("#mvp-01")
	if err != nil {
		t.Fatal(err)
	}
	if lu.Issue != "#mvp-01" || len(lu.Identites) != 3 {
		t.Fatal("relecture incoherente")
	}
	// Le dossier mission.json est bien ecrit (shard = un dossier par mission).
	// Chemin encodé par nomDir (injectif, C6) : '#' (0x23) devient '~23'.
	if _, err := os.Stat(filepath.Join(ref.Racine(), "~23mvp-01", "mission.json")); err != nil {
		t.Fatalf("enregistrement atomique absent : %v", err)
	}
}

func TestCreationDoublonRefuse(t *testing.T) {
	ref, _ := Ouvrir(t.TempDir())
	a, r, i := configValide()
	if _, err := ref.Creer("#x", a, r, i, budget.Plafonds{Appels: map[string]int{"a": 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ref.Creer("#x", a, r, i, budget.Plafonds{}); err == nil {
		t.Fatal("doublon DOIT etre refuse")
	}
}

func TestHabilitationInvalidesRefusees(t *testing.T) {
	identites := []policy.Identity{
		{ID: "e", Role: "EXECUTANT", QualifiedBy: "mister-malsain"}, // hors liste : N-01
		{ID: "r", Role: "RECEPTIONNAIRE", QualifiedBy: "openai-simule"},
	}
	err := Valider([]string{"mistral-simule"}, map[string][]string{"EXECUTANT": {"executer"}}, identites)
	if err == nil {
		t.Fatal("identite qualifiee par un adaptateur non declare DOIT etre refusee (N-01, fail closed)")
	}
	// N-05 : separation EXECUTANT / RECEPTIONNAIRE obligatoire.
	err = Valider([]string{"m", "o"},
		map[string][]string{"EXECUTANT": {"executer"}, "RECEPTIONNAIRE": {"recevoir"}, "PILOTE": {"cloturer"}},
		[]policy.Identity{{ID: "e", Role: "EXECUTANT", QualifiedBy: "m"}})
	if err == nil {
		t.Fatal("absence de RECEPTIONNAIRE distinct DOIT etre refusee (N-05)")
	}
}

func TestJournalIdempotentParMission(t *testing.T) {
	ref, _ := Ouvrir(t.TempDir())
	a, r, i := configValide()
	_, err := ref.Creer("#j", a, r, i, budget.Plafonds{Appels: map[string]int{"m": 1}})
	if err != nil {
		t.Fatal(err)
	}
	e1 := map[string]any{"type": "TRANSITION", "detail": "EXECUTION par exec-sim"}
	if err := ref.Journaliser("#j", e1); err != nil {
		t.Fatal(err)
	}
	if err := ref.Journaliser("#j", e1); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(JournalPath(ref.Racine(), "#j"))
	if strings.Count(string(raw), "EXECUTION par exec-sim") != 1 {
		t.Fatal("idempotence N-19 violee : entree dupliquee")
	}
}

func TestEstConformeFailClosedAuDemarrage(t *testing.T) {
	racine := t.TempDir()
	ref, _ := Ouvrir(racine)
	a, r, i := configValide()
	_, err := ref.Creer("#ok", a, r, i, budget.Plafonds{Appels: map[string]int{"m": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ref.EstConforme(); err != nil {
		t.Fatalf("referentiel sain refuse au demarrage : %v", err)
	}
	// Sabotage direct d'un enregistrement : le demarrage DOIT refuser (R2/S5).
	// Chemin encodé par nomDir (injectif, C6) : '#' (0x23) devient '~23'.
	chemin := filepath.Join(racine, "~23ok", "mission.json")
	_ = os.WriteFile(chemin, []byte(`{"Issue":"#ok","Roles":{}}`), 0o644)
	ref2, err := Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	if err := ref2.EstConforme(); err == nil {
		t.Fatal("configuration mal formee DOIT refuser le demarrage (R2/S5, fail closed)")
	}
}

func TestIndexReconstruitAuDemarrage(t *testing.T) {
	racine := t.TempDir()
	ref, _ := Ouvrir(racine)
	a, r, i := configValide()
	_, _ = ref.Creer("#list", a, r, i, budget.Plafonds{Appels: map[string]int{"m": 1}})
	ref2, _ := Ouvrir(racine)
	if len(ref2.Lister()) != 1 {
		t.Fatal("index reconstruit au demarrage DOIT retrouver les missions existantes (O(n) documente)")
	}
}

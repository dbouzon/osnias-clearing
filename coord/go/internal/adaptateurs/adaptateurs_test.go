// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package adaptateurs

import (
	"strings"
	"testing"
)

func TestRegistreFailClosed(t *testing.T) {
	if _, err := Registre(nil); err == nil {
		t.Fatal("registre vide DOIT etre refuse (N-01, fail closed)")
	}
	if _, err := Registre([]string{"", "a"}); err == nil {
		t.Fatal("identifiant vide DOIT etre refuse")
	}
	if _, err := Registre([]string{"a", "a"}); err == nil {
		t.Fatal("doublon DOIT etre refuse")
	}
	if _, err := NouveauSimule("adaptateur-reel"); err == nil {
		t.Fatal("un identifiant de mode reel DOIT etre refuse (fail closed, lecon T02)")
	}
	if _, err := NouveauSimule("openai real"); err == nil {
		t.Fatal("mode real DOIT etre refuse (fail closed)")
	}
}

func TestSimuleDeterministeSansReseau(t *testing.T) {
	s, err := NouveauSimule("mistral-simule")
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Executer("mandat A")
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := s.Executer("mandat A")
	if r1 != r2 {
		t.Fatal("l'adaptateur simule DOIT etre deterministe")
	}
	if !r1.Simule {
		t.Fatal("tout resultat DOIT etre marque simule")
	}
	if r1.Jetons != JetonsExecuter {
		t.Fatal("jetons d'execution incoherents avec l'estimation budgetaire")
	}
	rs, _ := s.Soumettre("livrable B")
	if rs.Jetons != JetonsSoumettre {
		t.Fatal("jetons de soumission incoherents")
	}
	if !strings.Contains(r1.Sortie, "SIMULEE") || !strings.Contains(rs.Sortie, "SIMULEE") {
		t.Fatal("les sorties DOIVENT rappeler la simulation explicitement")
	}
}

func TestJetonsEstimes(t *testing.T) {
	if JetonsEstimes("executer") != JetonsExecuter || JetonsEstimes("soumettre") != JetonsSoumettre {
		t.Fatal("estimations budgetaires incoherentes avec les couts simules")
	}
	if JetonsEstimes("recevoir") != 0 {
		t.Fatal("recevoir n'est pas un appel d'adaptateur : estimation nulle attendue")
	}
}

func TestIndependanceFournisseur(t *testing.T) {
	// Deux identifiants opaques quelconques se comportent exactement pareil :
	// aucun privilège vendeur, aucun code special.
	a, _ := NouveauSimule("mistral-simule")
	b, _ := NouveauSimule("openai-simule")
	ra, _ := a.Executer("m")
	rb, _ := b.Executer("m")
	if ra.Jetons != rb.Jetons || ra.Simule != rb.Simule {
		t.Fatal("comportement inegal entre fournisseurs : privilege vendeur interdit")
	}
	if ra.Sortie == rb.Sortie {
		t.Fatal("les sorties doivent identifier l'adaptateur (opaques, distincts)")
	}
}

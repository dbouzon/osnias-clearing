// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package budget

import "testing"

// AjusterJetons (M-COORD-PILOT-03) : le surplus réel mesuré après appel
// s'ajoute AUX jetons, jamais au compteur d'appels (l'appel est déjà
// réservé durablement en TX1), et le compteur ne descend jamais sous zéro.
func TestAjusterJetonsSansAppelSupplementaire(t *testing.T) {
	b := Nouveau(Plafonds{
		Appels: map[string]int{"mistral": 2},
		Jetons: map[string]int{"mistral": 5000},
	})
	b.Consommer("mistral", 1000)
	if b.Etat.AppelsConsommes["mistral"] != 1 || b.Etat.JetonsConsommes["mistral"] != 1000 {
		t.Fatalf("état initial inattendu : %+v", b.Etat)
	}
	b.AjusterJetons("mistral", 500)
	if b.Etat.AppelsConsommes["mistral"] != 1 {
		t.Fatalf("le surplus ne doit PAS compter un appel : %+v", b.Etat)
	}
	if b.Etat.JetonsConsommes["mistral"] != 1500 {
		t.Fatalf("surplus attendu dans les jetons : %+v", b.Etat)
	}
	b.AjusterJetons("mistral", -400)
	if b.Etat.JetonsConsommes["mistral"] != 1100 {
		t.Fatalf("ajustement négatif attendu : %+v", b.Etat)
	}
	b.AjusterJetons("mistral", -99999)
	if b.Etat.JetonsConsommes["mistral"] != 0 {
		t.Fatalf("le compteur ne descend jamais sous zéro : %+v", b.Etat)
	}
	if b.Etat.AppelsConsommes["mistral"] != 1 {
		t.Fatalf("aucun appel supplémentaire compté : %+v", b.Etat)
	}
}

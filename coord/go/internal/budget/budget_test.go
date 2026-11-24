// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package budget

import "testing"

func TestR3FailClosedSansPlafond(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 2}})
	ok, motif := b.VerifierAvantAppel("inconnu", 10)
	if ok {
		t.Fatal("un adaptateur sans plafond DOIT etre refuse (fail closed)")
	}
	if motif == "" {
		t.Fatal("motif explicite attendu")
	}
}

func TestR3PlafondAppelsDepasse(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 1}})
	if ok, motif := b.VerifierAvantAppel("a", 0); !ok {
		t.Fatal("premier appel autorise attendu, motif: " + motif)
	}
	b.Consommer("a", 100)
	ok, motif := b.VerifierAvantAppel("a", 0)
	if ok {
		t.Fatal("depassement de plafond DOIT etre refuse (R3)")
	}
	if motif == "" {
		t.Fatal("motif explicite attendu sur refus budget")
	}
}

func TestR3PlafondJetonsDepasse(t *testing.T) {
	b := Nouveau(Plafonds{
		Appels: map[string]int{"a": 10},
		Jetons: map[string]int{"a": 1000},
	})
	if ok, m := b.VerifierAvantAppel("a", 999); !ok {
		t.Fatal("sous le plafond attendu, motif: " + m)
	}
	if ok, _ := b.VerifierAvantAppel("a", 1001); ok {
		t.Fatal("plafond de jetons DOIT bloquer AVANT l'appel (R3)")
	}
}

func TestConsommerEtCoutSimule(t *testing.T) {
	b := Nouveau(Plafonds{
		Appels:  map[string]int{"a": 10},
		Centime: 2,
	})
	b.Consommer("a", 100)
	if b.Etat.AppelsConsommes["a"] != 1 || b.Etat.JetonsConsommes["a"] != 100 {
		t.Fatal("compteurs apres consommation incoherents")
	}
	if b.CoutSimule("a") != 200 {
		t.Fatal("indicateur monetaire SIMULE incoherent")
	}
	if b.ResumeCompteurs() == "" {
		t.Fatal("resume de compteurs vide inattendu")
	}
}

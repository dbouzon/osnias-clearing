// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package budget

import (
	"strings"
	"testing"
)

// Réservation monétaire durable (correction de la réserve 7154948522,
// M-COORD-PILOT-03) : le plafond monétaire de la mission, en centimes
// d'euro ENTIERS, est désormais OPPOSABLE AVANT toute tentative
// potentiellement facturable, et une réservation n'est JAMAIS libérée.

func TestReserverCentimesSansPlafondArme(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 1}, Jetons: map[string]int{"a": 100}})
	if ok, motif := b.ReserverCentimes("a", 50); ok {
		t.Fatalf("sans plafond arme, la réservation doit être refusée")
	} else if !strings.Contains(motif, "monetaire") {
		t.Fatalf("motif inattendu : %s", motif)
	}
}

func TestReserverCentimesPlafondGlobal(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 5, "b": 5}, Centimes: 100})
	if ok, motif := b.ReserverCentimes("a", 60); !ok {
		t.Fatalf("première réservation attendue : %s", motif)
	}
	// Le plafond est GLOBAL à la mission, tous adaptateurs confondus.
	if ok, motif := b.ReserverCentimes("b", 41); ok {
		t.Fatalf("dépassement du plafond global attendu : %s", motif)
	}
	if ok, motif := b.ReserverCentimes("b", 40); !ok {
		t.Fatalf("réservation exacte au plafond attendue : %s", motif)
	}
	if b.CentimesReservesTotal() != 100 {
		t.Fatalf("total réservé attendu 100 : %d", b.CentimesReservesTotal())
	}
}

func TestReserverCentimesEstimationNonPositive(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 1}, Centimes: 100})
	if ok, _ := b.ReserverCentimes("a", 0); ok {
		t.Fatalf("réservation nulle refusée (tarif/borne inconnu, fail closed)")
	}
	if ok, _ := b.ReserverCentimes("a", -5); ok {
		t.Fatalf("réservation négative refusée")
	}
}

func TestAjusterCentimesNeLibereJamais(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 5}, Centimes: 1000})
	if ok, _ := b.ReserverCentimes("a", 100); !ok {
		t.Fatalf("réservation initiale attendue")
	}
	// CONSERVATEUR PAR CONSTRUCTION : un ajustement négatif ou nul est
	// IGNORÉ — le coût réel constaté ne libère JAMAIS de réservation
	// (réserve 7154948522 : le surplus ne doit plus être le seul garde-fou).
	b.AjusterCentimes("a", -60)
	b.AjusterCentimes("a", 0)
	if b.CentimesReservesTotal() != 100 {
		t.Fatalf("la réservation ne doit jamais être libérée : %d", b.CentimesReservesTotal())
	}
	// Un SURCOÛT réel constaté est enregistré.
	b.AjusterCentimes("a", 25)
	if b.Etat.CentimesReserves["a"] != 125 {
		t.Fatalf("surcoût attendu enregistré : %+v", b.Etat.CentimesReserves)
	}
}

// Un référentiel persisté AVANT la correction (sans champ centimes_reserves)
// se recharge sans erreur : compteur monétaire initialisé à vide.
func TestBudgetAncienFormatRecharge(t *testing.T) {
	b := Nouveau(Plafonds{Appels: map[string]int{"a": 1}, Centimes: 100})
	b.Etat.CentimesReserves = nil
	if ok, motif := b.ReserverCentimes("a", 10); !ok {
		t.Fatalf("réservation après map nil attendue : %s", motif)
	}
	if b.Etat.CentimesReserves["a"] != 10 {
		t.Fatalf("réinitialisation attendue : %+v", b.Etat.CentimesReserves)
	}
}

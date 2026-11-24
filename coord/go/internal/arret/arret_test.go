// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package arret

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func journalTemp(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "journal.jsonl")
}

func TestR4DemanderEtJournalDistinctDesEcart(t *testing.T) {
	j := journalTemp(t)
	e := &Etat{}
	if Actif(e) {
		t.Fatal("etat initial doit etre inactif")
	}
	if err := Demander(e, "pause pilote", "pilote-local", j); err != nil {
		t.Fatal(err)
	}
	if !Actif(e) {
		t.Fatal("arret actif attendu")
	}
	raw, err := os.ReadFile(j)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"type\":\""+TypeArret+"\"") {
		t.Fatal("entree ARRET_PILOTE attendue dans le journal")
	}
	if strings.Contains(string(raw), "ECART") {
		t.Fatal("le journal d'arret ne doit JAMAIS contenir d'entree ECART (distinction N-20)")
	}
}

func TestR4LeverFailClosed(t *testing.T) {
	j := journalTemp(t)
	e := &Etat{}
	if err := Lever(e, "specieux", "x", j); err == nil {
		t.Fatal("lever un arret inactif DOIT echouer (fail closed)")
	}
	_ = Demander(e, "motif", "pilote-local", j)
	if err := Lever(e, "reprise", "pilote-local", j); err != nil {
		t.Fatal(err)
	}
	if Actif(e) {
		t.Fatal("arret doit etre inactif apres levee")
	}
	raw, _ := os.ReadFile(j)
	if !strings.Contains(string(raw), TypeLeveeArret) {
		t.Fatal("entree LEVEE_ARRET_PILOTE attendue")
	}
	// Une nouvelle commande d'arrêt (même motif) est un NOUVEL événement :
	// l'entrée est re-journalisée avec son instant propre, jamais perdue.
	_ = Demander(e, "motif", "pilote-local", j)
	_ = Lever(e, "reprise", "pilote-local", j)
	_ = Demander(e, "motif", "pilote-local", j)
	raw2, _ := os.ReadFile(j)
	if strings.Count(string(raw2), TypeLeveeArret) != 1 && strings.Count(string(raw2), TypeLeveeArret) != 2 {
		t.Fatal("chaque levee DOIT être journalisée (traceabilité des commandes pilote)")
	}
}

func TestJournalRefusExplicite(t *testing.T) {
	j := journalTemp(t)
	if err := JournalRefus("transition X refusée : arrêt demandé par le pilote", "exec", j); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(j)
	if !strings.Contains(string(raw), TypeRefusArret) {
		t.Fatal("entree REFUS_TRANSITION_ARRET attendue")
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package ctxcontrol

import (
	"os"
	"path/filepath"
	"testing"
)

var termes = []string{"normalisation", "moteur go", "vecteurs de conformite"}

func TestScoreEtQualification(t *testing.T) {
	// N-18 : score = fraction de termes couverts ; COHERENTE exige score >= 0.5
	// ET provenance declaree.
	s := Score("_normalisation_ et _moteur go_ pour la mission", termes)
	if s < 0.5 {
		t.Fatalf("score attendu >= 0.5, obtenu %v", s)
	}
	// Le meme texte sans provenance declaree reste un ecart isole.
	quals := Classify([]Instruction{
		{Texte: "normalisation et moteur go pour la mission", Provenance: "issue-20"},
		{Texte: "normalisation et moteur go pour la mission", Provenance: ""},
		{Texte: "achete des.actions maintenant", Provenance: "inconnu"},
	}, termes)
	if quals[0].Qualification != "COHERENTE" {
		t.Fatalf("instruction coherente attendue: %+v", quals[0])
	}
	if quals[1].Qualification != "ECART_ISOLE" {
		t.Fatalf("sans provenance : ecart isole attendu: %+v", quals[1])
	}
	if quals[2].Qualification != "ECART_ISOLE" {
		t.Fatalf("instruction hors mission : ecart isole attendu: %+v", quals[2])
	}
}

func TestJournalEcartIdempotentSansTexte(t *testing.T) {
	// N-19 : journal idempotent, empreinte de l'instruction (jamais le texte).
	dir := t.TempDir()
	journal := filepath.Join(dir, "journal.jsonl")
	ecarts := []Ecart{{TSUTC: "ts", Texte: "instruction sensible"}}
	n1, err := JournalEcart(ecarts, journal)
	if err != nil || n1 != 1 {
		t.Fatalf("1 ligne attendue, obtenu %d (err=%v)", n1, err)
	}
	n2, err := JournalEcart(ecarts, journal)
	if err != nil || n2 != 0 {
		t.Fatalf("rejeu idempotent attendu, obtenu %d (err=%v)", n2, err)
	}
	lu, _ := os.ReadFile(journal)
	if len(lu) == 0 {
		t.Fatal("journal vide")
	}
	for _, marqueur := range []string{"\"instruction sensible\"", "achete"} {
		if string(lu) == marqueur {
			t.Fatal("le texte libre ne doit pas etre journalise comme preuve")
		}
	}
	if len(ecarts[0].Hash) != 64 {
		t.Fatalf("empreinte 64 hex attendue: %s", ecarts[0].Hash)
	}
}

func TestRepriseSansInterruption(t *testing.T) {
	// N-20 : un ecart n'interrompt jamais et ne produit jamais de decision.
	r := RepriseApresEcart([]Qualification{{Qualification: "ECART_ISOLE"}}, "reception")
	if r["interruption"] != false || r["decisions_par_ecart"] != float64(0) {
		t.Fatalf("reprise sans interruption attendue: %v", r)
	}
	if r["etape_reprise"] != "reception" || r["ecarts_isoles"] != float64(1) {
		t.Fatalf("etape et ecarts attendus: %v", r)
	}
	// Etape inconnue : reprise a l'execution par defaut.
	r = RepriseApresEcart(nil, "etape-inconnue")
	if r["etape_reprise"] != "execution" {
		t.Fatalf("etape par defaut attendue: %v", r)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package persist

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomiciteTroisPoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	initial := map[string]any{"etat": float64(1), "mission": "M-A"}
	if err := SaveAtomic(initial, path, ""); err != nil {
		t.Fatal(err)
	}
	nouveau := map[string]any{"etat": float64(2), "mission": "M-B"}

	for _, point := range []string{"ecriture", "avant_remplacement"} {
		err := SaveAtomic(nouveau, path, point)
		var crash CrashSimule
		if !errors.As(err, &crash) || crash.Point != point {
			t.Fatalf("crash %s attendu, obtenu %v", point, err)
		}
		// L'original est INTACT apres le crash.
		lu, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(lu, &m); err != nil {
			t.Fatalf("original corrompu au point %s: %v", point, err)
		}
		if m["mission"] != "M-A" {
			t.Fatalf("original modifie au point %s", point)
		}
		if !NettoyerTemporaire(path) {
			t.Fatalf("temporaire attendu au point %s", point)
		}
	}

	// apres_remplacement : le nouveau contenu EST EN PLACE.
	err := SaveAtomic(nouveau, path, "apres_remplacement")
	var crash CrashSimule
	if !errors.As(err, &crash) {
		t.Fatalf("crash attendu, obtenu %v", err)
	}
	lu, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(lu, &m); err != nil || m["mission"] != "M-B" {
		t.Fatalf("contenu nouveau attendu, obtenu %s (err=%v)", lu, err)
	}
	// La sauvegarde reelle reste sans residue temporaire.
	if err := SaveAtomic(initial, path, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatal("aucun temporaire ne doit rester apres une sauvegarde reelle")
	}
}

func TestJournalIdempotent(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "ecarts.jsonl")
	e := []any{map[string]any{"instruction_hash": "aa", "qualification": "ECART_ISOLE"}}
	n1, err := JournalExecute(e, journal)
	if err != nil || n1 != 1 {
		t.Fatalf("1 ligne attendue, obtenu %d (err=%v)", n1, err)
	}
	n2, err := JournalExecute(e, journal)
	if err != nil || n2 != 0 {
		t.Fatalf("0 ligne attendue au rejeu, obtenu %d (err=%v)", n2, err)
	}
	e2 := []any{map[string]any{"instruction_hash": "bb", "qualification": "ECART_ISOLE"}}
	n3, err := JournalExecute(e2, journal)
	if err != nil || n3 != 1 {
		t.Fatalf("1 nouvelle ligne attendue, obtenu %d (err=%v)", n3, err)
	}
	lu, _ := os.ReadFile(journal)
	if len(lu) == 0 || lu[len(lu)-1] != '\n' {
		t.Fatal("journal JSONL mal forme")
	}
}

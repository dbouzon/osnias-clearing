// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package conduite

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestConduiteS1S9 execute la conduite complète et vérifie les
// critères GO/NO-GO pré-déclarés du plan v2 accepté :
//
//  1. chaque scénario doit exister (S1,S2,S3,S3b,S4,S5,S6,S7,S8,S9) ;
//  2. aucune ligne ne doit être au verdict PERDUE ;
//  3. le streaming (S3b) doit être marqué dégradé (non incrémental) ;
//  4. la sémantique A2A (états finaux) doit être IDENTIQUE par paire.
func TestConduiteS1S9(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	exp, err := Runner(ctx)
	if err != nil {
		t.Fatalf("conduite: %v", err)
	}
	attendus := map[string]bool{
		"S1": false, "S2": false, "S3": false, "S3b": false,
		"S4": false, "S5": false, "S6": false, "S7": false,
		"S8": false, "S9": false,
	}
	for _, l := range exp.Lignes {
		if _, ok := attendus[l.Scenario]; !ok {
			t.Errorf("scenario inattendu %s", l.Scenario)
			continue
		}
		attendus[l.Scenario] = true
		if strings.Contains(l.Verdict, "PERDUE") {
			t.Errorf("%s: verdict %s (sha A %s, sha B %s)", l.Scenario, l.Verdict, l.A.Sha, l.B.Sha)
		}
		if l.A.EtatFinal != l.B.EtatFinal {
			t.Errorf("%s: etats finaux differents: A %s, B %s", l.Scenario, l.A.EtatFinal, l.B.EtatFinal)
		}
	}
	for id, vu := range attendus {
		if !vu {
			t.Errorf("scenario manquant: %s", id)
		}
	}
	// Critère 3 : le streaming doit être documenté comme dégradé en B.
	for _, l := range exp.Lignes {
		if l.Scenario == "S3b" {
			if l.A.Evenements != l.B.Evenements {
				t.Errorf("S3b: nombre d evenements differents A=%d B=%d", l.A.Evenements, l.B.Evenements)
			}
			if !strings.Contains(l.Verdict, "dégradé") && !strings.Contains(string([]rune(l.Verdict)), "degrade") {
				t.Errorf("S3b: verdict attendu degradé, obtenu %s", l.Verdict)
			}
		}
	}
}

// TestMarkdown vérifie que le rapport Markdown rend toutes les lignes.
func TestMarkdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	exp, err := Runner(ctx)
	if err != nil {
		t.Fatalf("conduite: %v", err)
	}
	md := exp.Markdown()
	if !strings.Contains(md, "| S1 |") || !strings.Contains(md, "| S9 |") {
		t.Error("rapport Markdown incomplet")
	}
	if !strings.Contains(md, "conduite comparative") {
		t.Error("titre du rapport absent")
	}
}

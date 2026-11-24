// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package resume

import "testing"

func TestDivergentsGithubFaitFoi(t *testing.T) {
	k := map[string]any{"round": float64(1), "prochaine_action": "action A", "sha_head": "h1"}
	g := map[string]any{"round": float64(2), "prochaine_action": "action B", "sha_head": "h1"}
	e := Arbitrer(k, g, "2026-11-24T22:10:00Z", "test")
	if e.Elu != "github" {
		t.Fatalf("github fait foi, obtenu %s", e.Elu)
	}
	if e.Regle != "github_fait_foi" {
		t.Fatal("regle d'arbitrage attendue")
	}
	if len(e.ChampsDivergents) != 2 {
		t.Fatalf("2 champs divergents attendus, obtenu %v", e.ChampsDivergents)
	}
	if e.Empreintes["knowledge"] == e.Empreintes["github"] {
		t.Fatal("les deux empreintes integrales doivent figurer")
	}
}

func TestConcordants(t *testing.T) {
	k := map[string]any{"round": float64(1)}
	g := map[string]any{"round": float64(1)}
	e := Arbitrer(k, g, "ts", "test")
	if e.Elu != "supports_concordants" || len(e.ChampsDivergents) != 0 {
		t.Fatalf("supports concordants attendus, obtenu %s %v", e.Elu, e.ChampsDivergents)
	}
}

func TestRepriseShaDivergent(t *testing.T) {
	g := map[string]any{"sha_head": "aaa", "prochaine_action": "action A"}
	e := Arbitrer(map[string]any{"sha_head": "bbb"}, g, "ts", "test")
	action, errs := Reprise(e, "zzz")
	if action != "" || len(errs) == 0 {
		t.Fatalf("sha divergent : reprise refusee attendue, obtenu %q %v", action, errs)
	}
	action, errs = Reprise(e, "aaa")
	if action != "action A" || len(errs) != 0 {
		t.Fatalf("reprise attendue, obtenu %q %v", action, errs)
	}
}

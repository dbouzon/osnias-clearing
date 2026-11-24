// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package resume implements COORD Specification 1.0 section 8 (N-16, N-17):
// support arbitration (github makes the decision), journaled with both
// fingerprints, and deterministic post-arbitration resume.
package resume

import (
	"sort"

	"coordengine/internal/jcs"
	"coordengine/internal/spec"
)

// Entree is the normalized arbitration decision, sufficient for a post-mortem
// analysis: divergent fields and the fingerprints of BOTH complete records.
type Entree struct {
	Elu              string         `json:"elu"`
	Record           map[string]any `json:"record"`
	Regle            string         `json:"regle"`
	ChampsDivergents []string       `json:"champs_divergents"`
	Empreintes       map[string]any `json:"empreintes"`
	TSUTC            string         `json:"ts_utc"`
	Arbitre          string         `json:"arbitre"`
}

// ChampsDivergents returns the first-level fields whose canonical form
// diverges between the two records.
func ChampsDivergents(a, b map[string]any) []string {
	canon := func(v any) string {
		c, err := jcs.Canonical(v)
		if err != nil {
			return "<non canonique>"
		}
		return string(c)
	}
	cles := map[string]bool{}
	for k := range a {
		cles[k] = true
	}
	for k := range b {
		cles[k] = true
	}
	names := make([]string, 0, len(cles))
	for k := range cles {
		names = append(names, k)
	}
	sort.Strings(names)
	out := []string{}
	for _, k := range names {
		if canon(a[k]) != canon(b[k]) {
			out = append(out, k)
		}
	}
	return out
}

// Arbitrer decides between the knowledge-base record and the GitHub record:
// on divergence, github fait foi (N-16). The entry carries both fingerprints.
func Arbitrer(recordKnowledge, recordGithub map[string]any, tsUTC, arbitre string) Entree {
	div := ChampsDivergents(recordKnowledge, recordGithub)
	elu := "supports_concordants"
	if len(div) > 0 {
		elu = "github"
	}
	fk, _ := jcs.Fingerprint(recordKnowledge)
	fg, _ := jcs.Fingerprint(recordGithub)
	return Entree{
		Elu:              elu,
		Record:           recordGithub,
		Regle:            spec.RegleArbitrage,
		ChampsDivergents: div,
		Empreintes:       map[string]any{"knowledge": fk, "github": fg},
		TSUTC:            tsUTC,
		Arbitre:          arbitre,
	}
}

// Reprise determines the next action after arbitration: it executes only if
// the verified head matches the record's head (N-17).
func Reprise(entree Entree, shaHeadAttendu string) (string, []string) {
	record := entree.Record
	if record == nil {
		return "", []string{"enregistrement arbitre absent"}
	}
	shaHead, _ := record["sha_head"].(string)
	if shaHead != "" && shaHead != shaHeadAttendu {
		return "", []string{"sha_head divergent : reprise impossible sans re-verification du monde"}
	}
	action, _ := record["prochaine_action"].(string)
	return action, nil
}

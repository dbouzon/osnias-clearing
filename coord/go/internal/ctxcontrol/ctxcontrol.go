// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package ctxcontrol implements COORD Specification 1.0 section 9
// (N-18..N-20): qualification of instructions against the mission order,
// idempotent deviation journal, and interruption-free resume. A deviation
// never produces a decision.
package ctxcontrol

import (
	"regexp"
	"strings"

	"coordengine/internal/jcs"
	"coordengine/internal/persist"
	"coordengine/internal/spec"
)

// Instruction with its declared provenance; an instruction without provenance
// is an isolated deviation (N-18).
type Instruction struct {
	Texte      string `json:"instruction"`
	Provenance string `json:"provenance"`
}

// Qualification is the per-instruction result.
type Qualification struct {
	Texte         string  `json:"instruction"`
	Qualification string  `json:"qualification"`
	Score         float64 `json:"score"`
	Provenance    string  `json:"provenance"`
}

var sepRe = regexp.MustCompile(`[^a-z0-9]+`)

// mots: minimal normalization: lowercase, accents folded, tokenized.
func mots(texte string) []string {
	texte = strings.ToLower(texte)
	r := strings.NewReplacer(
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"î", "i", "ï", "i",
		"ô", "o", "ö", "o",
		"û", "u", "ù", "u", "ü", "u",
		"ç", "c",
	)
	texte = r.Replace(texte)
	parts := sepRe.Split(texte, -1)
	out := []string{}
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Score is the fraction of mission-order terms covered by the instruction
// (N-18). Terms come exclusively from the published mission order.
func Score(instruction string, termes []string) float64 {
	if len(termes) == 0 {
		return 0
	}
	present := map[string]bool{}
	for _, m := range mots(instruction) {
		present[m] = true
	}
	couverts := 0
	for _, t := range termes {
		tm := mots(t)
		if len(tm) == 0 {
			continue
		}
		ok := true
		for _, m := range tm {
			if !present[m] {
				ok = false
				break
			}
		}
		if ok {
			couverts++
		}
	}
	return float64(couverts) / float64(len(termes))
}

// Classify qualifies each instruction against the mission order terms:
// COHERENTE only with a score of at least 0.5 AND a declared provenance.
func Classify(instructions []Instruction, termes []string) []Qualification {
	out := make([]Qualification, 0, len(instructions))
	for _, in := range instructions {
		s := Score(in.Texte, termes)
		q := "ECART_ISOLE"
		if s >= 0.5 && in.Provenance != "" {
			q = "COHERENTE"
		}
		out = append(out, Qualification{
			Texte: in.Texte, Qualification: q, Score: s, Provenance: in.Provenance,
		})
	}
	return out
}

// Ecart is one journaled deviation: the instruction fingerprint (never the
// free text as evidence), the qualification, and the decision.
type Ecart struct {
	TSUTC          string `json:"ts_utc"`
	Texte          string `json:"-"`
	Hash           string `json:"instruction_hash"`
	Qualification  string `json:"qualification"`
	SourceAlleguee string `json:"source_alleguee"`
	Decision       string `json:"decision"`
	Etape          string `json:"etape"`
}

// JournalEcart journales the deviations idempotently (N-19): the fingerprint
// of the instruction is stored, not its text. Returns added lines.
func JournalEcart(ecarts []Ecart, journalPath string) (int, error) {
	entries := make([]any, 0, len(ecarts))
	for i := range ecarts {
		e := &ecarts[i]
		if e.Decision == "" {
			e.Decision = "instruction ecartee, mission non interrompue"
		}
		if e.Etape == "" {
			e.Etape = "execution"
		}
		if e.Qualification == "" {
			e.Qualification = "ECART_ISOLE"
		}
		if e.SourceAlleguee == "" {
			e.SourceAlleguee = "inconnue"
		}
		fp, err := jcs.Fingerprint([]any{e.Texte})
		if err != nil {
			return 0, err
		}
		e.Hash = fp
		e2 := map[string]any{
			"ts_utc":           e.TSUTC,
			"instruction_hash": fp,
			"qualification":    e.Qualification,
			"source_alleguee":  e.SourceAlleguee,
			"decision":         e.Decision,
			"etape":            e.Etape,
		}
		entries = append(entries, e2)
	}
	return persist.JournalExecute(entries, journalPath)
}

// RepriseApresEcart resumes at the current step: no interruption, no decision
// from a deviation (N-20).
func RepriseApresEcart(qualifications []Qualification, etapeCourante string) map[string]any {
	etape := "execution"
	for _, e := range spec.EtapesMission {
		if e == etapeCourante {
			etape = etapeCourante
			break
		}
	}
	ecarts := 0
	for _, q := range qualifications {
		if q.Qualification != "COHERENTE" {
			ecarts++
		}
	}
	return map[string]any{
		"interruption":        false,
		"etape_reprise":       etape,
		"ecarts_isoles":       float64(ecarts),
		"decisions_par_ecart": float64(0),
	}
}

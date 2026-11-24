// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package budget implements the pre-call budget control of COORD Pilot
// (conception §2.2, recette R3): per-mission, per-adapter counters, ceilings
// fixed at mission creation, verification BEFORE any adapter call, and a
// fail-closed default: an adapter without a ceiling is never allowed to call.
// The MVP is simulated: the token counts and the monetary indicator are
// declared by the simulated adapter; no real cost can be incurred here.
package budget

import (
	"fmt"
	"strings"
)

// Plafonds: ceilings fixed at mission creation. Fail-closed: an adapter
// absent from Appels has NO call authorised, whatever the counters say.
type Plafonds struct {
	Appels  map[string]int `json:"appels"`
	Jetons  map[string]int `json:"jetons"`
	Centime int            `json:"centimes_par_jeton_simule"`
}

// Etat: consumed counters, per adapter.
type Etat struct {
	AppelsConsommes map[string]int `json:"appels_consommes"`
	JetonsConsommes map[string]int `json:"jetons_consommes"`
}

// Budget: ceilings plus consumed state, persisted with the mission record.
type Budget struct {
	Plafonds Plafonds `json:"plafonds"`
	Etat     Etat     `json:"etat"`
}

// Nouveau builds a Budget with initialised maps.
func Nouveau(plafonds Plafonds) *Budget {
	if plafonds.Appels == nil {
		plafonds.Appels = map[string]int{}
	}
	if plafonds.Jetons == nil {
		plafonds.Jetons = map[string]int{}
	}
	return &Budget{
		Plafonds: plafonds,
		Etat: Etat{
			AppelsConsommes: map[string]int{},
			JetonsConsommes: map[string]int{},
		},
	}
}

// VerifierAvantAppel checks the budget BEFORE any adapter call (R3).
// Returns (false, motif) on refusal; refusals are explicit, never masked.
func (b *Budget) VerifierAvantAppel(adaptateur string, jetonsEstimes int) (bool, string) {
	plafondAppels, defini := b.Plafonds.Appels[adaptateur]
	if !defini || plafondAppels <= 0 {
		return false, "budget : aucun plafond d'appels defini pour l'adaptateur " +
			adaptateur + " : aucun appel autorise (fail closed)"
	}
	if b.Etat.AppelsConsommes == nil {
		b.Etat.AppelsConsommes = map[string]int{}
	}
	if b.Etat.JetonsConsommes == nil {
		b.Etat.JetonsConsommes = map[string]int{}
	}
	if b.Etat.AppelsConsommes[adaptateur]+1 > plafondAppels {
		return false, fmt.Sprintf("budget : plafond d'appels depasse pour %s (%d/%d)",
			adaptateur, b.Etat.AppelsConsommes[adaptateur], plafondAppels)
	}
	if plafondJetons, ok := b.Plafonds.Jetons[adaptateur]; ok && plafondJetons > 0 {
		if b.Etat.JetonsConsommes[adaptateur]+jetonsEstimes > plafondJetons {
			return false, fmt.Sprintf("budget : plafond de jetons depasse pour %s (%d+%d > %d)",
				adaptateur, b.Etat.JetonsConsommes[adaptateur], jetonsEstimes, plafondJetons)
		}
	}
	return true, ""
}

// Consommer records one adapter call AFTER it happened (never before the
// verification). Called only on successful adapter invocation.
func (b *Budget) Consommer(adaptateur string, jetons int) {
	if b.Etat.AppelsConsommes == nil {
		b.Etat.AppelsConsommes = map[string]int{}
	}
	if b.Etat.JetonsConsommes == nil {
		b.Etat.JetonsConsommes = map[string]int{}
	}
	b.Etat.AppelsConsommes[adaptateur]++
	b.Etat.JetonsConsommes[adaptateur] += jetons
}

// CoutSimule: simulated monetary indicator (cents), display only.
func (b *Budget) CoutSimule(adaptateur string) int {
	return b.Etat.JetonsConsommes[adaptateur] * b.Plafonds.Centime
}

// ResumeCompteurs returns a one-line summary for the UI and journals.
func (b *Budget) ResumeCompteurs() string {
	var parts []string
	for a := range b.Plafonds.Appels {
		parts = append(parts, fmt.Sprintf("%s: %d/%d appels", a,
			b.Etat.AppelsConsommes[a], b.Plafonds.Appels[a]))
	}
	return strings.Join(parts, "; ")
}

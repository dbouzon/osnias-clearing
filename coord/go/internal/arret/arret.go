// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package arret implements the human stop of COORD Pilot (conception §2.2,
// recette R4): the pilot can stop a mission at any time; the service then
// refuses any further transition with an explicit motive, journals the
// request, and keeps the machine state intact. Journal entries are typed
// ARRET_PILOTE / LEVEE_ARRET_PILOTE, DISTINCT from the engine's ECART
// entries (N-20 governs self-declared context deviations and never
// interrupts; the pilot stop is a command of the human authority).
// Resuming after a stop passes exclusively through the normative device
// (resume N-16/N-17), which the service calls, never duplicates.
package arret

import (
	"time"

	"coordengine/internal/persist"
)

// Types de journal : distincts des ECART du moteur (N-20).
const (
	TypeArret      = "ARRET_PILOTE"
	TypeLeveeArret = "LEVEE_ARRET_PILOTE"
	TypeRefusArret = "REFUS_TRANSITION_ARRET"
)

// Etat: the stop flag persisted with the mission record.
type Etat struct {
	Actif bool   `json:"actif"`
	Motif string `json:"motif_si_actif"`
}

// Actif returns whether the stop is currently active.
func Actif(e *Etat) bool { return e != nil && e.Actif }

// nouvelleEntree : ligne de journal à l'instant courant (UTC, RFC 3339),
// sous forme canonique (map) car le sérialiseur normatif n'accepte que
// des valeurs décodées JSON.
func nouvelleEntree(typeEntree, motif, par string) map[string]any {
	return map[string]any{
		"type":        typeEntree,
		"motif":       motif,
		"instant_utc": time.Now().UTC().Format(time.RFC3339),
		"par":         par,
	}
}

// Demander activates the stop and journals ARRET_PILOTE. Idempotent: a stop
// over an active stop re-journals the new motive (the machine state is never
// modified by this package).
func Demander(e *Etat, motif, par, journalPath string) error {
	e.Actif = true
	e.Motif = motif
	_, err := persist.JournalExecute([]any{nouvelleEntree(TypeArret, motif, par)}, journalPath)
	return err
}

// Lever clears the stop flag and journals LEVEE_ARRET_PILOTE. The effective
// resume is performed by the service through the normative device (N-16/N-17).
// Refused (fail closed) if the stop is not active: no silent state change.
func Lever(e *Etat, motif, par, journalPath string) error {
	if !Actif(e) {
		return errPasActif{}
	}
	e.Actif = false
	e.Motif = ""
	_, err := persist.JournalExecute([]any{nouvelleEntree(TypeLeveeArret, motif, par)}, journalPath)
	return err
}

// JournalRefus records a refused transition while stopped (traceability,
// recette R4: every refusal is journaled, never masked).
func JournalRefus(motif, par, journalPath string) error {
	_, err := persist.JournalExecute([]any{nouvelleEntree(TypeRefusArret, motif, par)}, journalPath)
	return err
}

type errPasActif struct{}

func (errPasActif) Error() string {
	return "arret pilote non actif : levee impossible (fail closed)"
}

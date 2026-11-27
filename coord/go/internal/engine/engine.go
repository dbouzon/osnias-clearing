// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package engine implements the COORD Specification 1.0 state machine
// (sections 6.1 to 6.6): closed transition table, binding of the executant,
// no self-reception, referenced verdicts, and staleness by fingerprint.
// The engine is written from the specification, not translated from any
// existing implementation.
package engine

import (
	"encoding/json"
	"regexp"
	"strings"

	"coordengine/internal/policy"
	"coordengine/internal/spec"
)

var revisionRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var empreinteRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Soumission = {revision, proofs fingerprint} presented for reception (6.2).
type Soumission struct {
	Revision  string `json:"revision"`
	Empreinte string `json:"empreinte_preuves"`
}

// Decision: a recorded verdict with its evidence reference (6.4) and the
// proofs fingerprint it applies to (6.5).
type Decision struct {
	Verdict    string `json:"verdict"`
	Revision   string `json:"revision"`
	Empreinte  string `json:"empreinte"`
	Recepteur  string `json:"recepteur"`
	VerdictID  string `json:"verdict_id"`
	VerdictURL string `json:"verdict_url"`
	// Points : detail des points du verdict (corrections demandees ou
	// reserves). Persiste avec la decision pour rester opposable APRES un
	// arret et une reprise (M-COORD-DEP-02) : sans lui, l'executant ne
	// peut pas retrouver le detail des corrections a appliquer.
	Points []string `json:"points,omitempty"`
}

// Mission is the durable state machine instance (spec 6).
type Mission struct {
	State     string
	Issue     string
	Executant *policy.Identity
	EnAttente *Soumission
	Decisions []Decision
}

// New returns a mission in state PLANIFIEE.
func New(issue string) *Mission {
	return &Mission{State: spec.EtatPlanifiee, Issue: issue}
}

// Transition applies one closed-table transition. It returns (false, motif)
// on refusal; refusal motifs are explicit and never masked.
func (m *Mission) Transition(cible string, id policy.Identity,
	adapters []string, roles map[string][]string, donnees map[string]string) (bool, string) {

	errs := policy.ValidateConfig(adapters, roles)
	if len(errs) > 0 {
		return false, "configuration d'habilitations invalide: " + strings.Join(errs[:min(3, len(errs))], "; ")
	}
	if spec.Terminaux[m.State] {
		return false, "mission terminee (etat " + m.State + ")"
	}
	cibles, ok := spec.TableTransitions[m.State]
	if !ok {
		return false, "transition interdite: " + m.State + " -> " + cible
	}
	requis, ok := cibles[cible]
	if !ok {
		return false, "transition interdite: " + m.State + " -> " + cible
	}
	if id.Role != requis.Role {
		return false, "role non autorise pour cette transition (attendu: " + requis.Role + ")"
	}
	if !policy.Authorized(id, requis.Action, adapters, roles) {
		return false, "identite non habilitee pour l'action: " + requis.Action
	}

	switch cible {
	case spec.EtatExecution:
		if m.State == spec.EtatPlanifiee {
			b := id
			m.Executant = &b
			m.State = spec.EtatExecution
			return true, ""
		}
		if m.Executant == nil || id.ID != m.Executant.ID {
			return false, "identite executante non stabilisee sur la mission"
		}
		m.State = spec.EtatExecution
		return true, ""

	case spec.EtatDecisionAttendue:
		rev := donnees["revision"]
		emp := donnees["empreinte_preuves"]
		if !revisionRE.MatchString(rev) {
			return false, "revision invalide (40 hex attendus)"
		}
		if !empreinteRE.MatchString(emp) {
			return false, "empreinte de preuves invalide (sha256, 64 hex)"
		}
		m.EnAttente = &Soumission{Revision: rev, Empreinte: emp}
		m.State = spec.EtatDecisionAttendue
		return true, ""

	case spec.EtatAcceptee, spec.EtatRefusee, spec.EtatEnCorrection:
		if m.EnAttente == nil {
			return false, "aucune soumission en attente"
		}
		if m.Executant != nil && id.ID == m.Executant.ID {
			return false, "auto-reception interdite (executant = recepteur)"
		}
		attendu, ok := spec.CiblesVerdict[cible]
		if !ok {
			return false, "cible inconnue"
		}
		if donnees["verdict"] != "" && donnees["verdict"] != attendu {
			return false, "verdict incoherent avec la cible (attendu: " + attendu + ")"
		}
		if donnees["verdict_id"] == "" || donnees["verdict_url"] == "" {
			return false, "reference de verdict obligatoire (verdict_id + verdict_url)"
		}
		// Points du verdict (DEP-02) : champ additif ; present mais
		// invalide => refus fail-closed (un verdict tronque ne doit
		// jamais ecraser le detail d'une correction precedente).
		points := []string{}
		if raw := donnees["points"]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &points); err != nil {
				return false, "points de verdict invalides (liste JSON de chaines attendue)"
			}
		}
		m.Decisions = append(m.Decisions, Decision{
			Verdict:    attendu,
			Revision:   m.EnAttente.Revision,
			Empreinte:  m.EnAttente.Empreinte,
			Recepteur:  id.ID,
			VerdictID:  donnees["verdict_id"],
			VerdictURL: donnees["verdict_url"],
			Points:     points,
		})
		m.EnAttente = nil
		m.State = cible
		return true, ""

	case spec.EtatACKRecu:
		if len(m.Decisions) == 0 {
			return false, "ACK sans decision"
		}
		d := m.Decisions[len(m.Decisions)-1]
		if donnees["empreinte_courante"] != d.Empreinte ||
			donnees["revision_courante"] != d.Revision {
			return false, "verdict perime (empreinte ou revision divergentes)"
		}
		m.State = spec.EtatACKRecu
		return true, ""

	case spec.EtatCloturee:
		m.State = spec.EtatCloturee
		return true, ""
	}
	return false, "cible inconnue"
}

// Perime implements spec 6.5 staleness: fingerprint and revision comparison,
// never counters.
func Perime(d Decision, empreinteCourante, revisionCourante string) bool {
	return d.Empreinte != empreinteCourante || d.Revision != revisionCourante
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

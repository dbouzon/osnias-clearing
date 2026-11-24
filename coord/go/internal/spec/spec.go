// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package spec carries the normative constants of COORD Specification 1.0:
// states, verdicts, and the closed transition table (section 6.2). The wire
// identifier set is the French mapping used by all published conformance
// vectors.
package spec

// States (spec 6.1). Terminal states refuse every transition (N-09).
const (
	EtatPlanifiee        = "PLANIFIEE"
	EtatExecution        = "EXECUTION"
	EtatDecisionAttendue = "DECISION_ATTENDUE"
	EtatAcceptee         = "ACCEPTEE"
	EtatEnCorrection     = "EN_CORRECTION"
	EtatACKRecu          = "ACK_RECU"
	EtatRefusee          = "REFUSEE"
	EtatCloturee         = "CLOTUREE"
)

var Terminaux = map[string]bool{EtatRefusee: true, EtatCloturee: true}

// Actions and roles (spec 2 and 6.2).
const (
	ActionExecuter  = "executer"
	ActionSoumettre = "soumettre"
	ActionRecevoir  = "recevoir"
	ActionAcker     = "acker"
	ActionCloturer  = "cloturer"
)

var Roles = []string{"EXECUTANT", "RECEPTIONNAIRE", "PILOTE"}
var Actions = []string{ActionExecuter, ActionSoumettre, ActionRecevoir, ActionAcker, ActionCloturer}

// CiblesVerdict: target state -> expected verdict value (spec 6.4).
var CiblesVerdict = map[string]string{
	EtatAcceptee:     "ACCEPTEE",
	EtatRefusee:      "REFUSEE",
	EtatEnCorrection: "A_CORRIGER",
}

type Requis struct {
	Role   string
	Action string
}

// TableTransitions is the closed table of the specification (N-10): any
// (state, target) pair absent here is refused.
var TableTransitions = map[string]map[string]Requis{
	EtatPlanifiee: {EtatExecution: {Role: "EXECUTANT", Action: ActionExecuter}},
	EtatExecution: {EtatDecisionAttendue: {Role: "EXECUTANT", Action: ActionSoumettre}},
	EtatDecisionAttendue: {
		EtatAcceptee:     {Role: "RECEPTIONNAIRE", Action: ActionRecevoir},
		EtatRefusee:      {Role: "RECEPTIONNAIRE", Action: ActionRecevoir},
		EtatEnCorrection: {Role: "RECEPTIONNAIRE", Action: ActionRecevoir},
	},
	EtatEnCorrection: {EtatExecution: {Role: "EXECUTANT", Action: ActionExecuter}},
	EtatAcceptee:     {EtatACKRecu: {Role: "EXECUTANT", Action: ActionAcker}},
	EtatACKRecu:      {EtatCloturee: {Role: "PILOTE", Action: ActionCloturer}},
}

// EtapesMission (spec 9, context control).
var EtapesMission = []string{"critique", "execution", "reception", "ack", "cloture"}

// RegleArbitrage (spec 8, N-16).
const RegleArbitrage = "github_fait_foi"

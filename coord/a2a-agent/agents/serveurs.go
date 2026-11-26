// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// serveurs.go : démarrage d'UN SERVEUR D'AGENT PAR FOURNISSEUR sur le
// socle commun (M-COORD-LLM-04, T2/T4). Chaque instance `coord-agent`
// écoute en loopback avec son PROPRE jeton du rail ; la sélection de
// deux intervenants DISTINCTS se fait côté orchestrateur (rail/URL +
// rôles NouvelleMissionExplicit, inchangés). Les cartes déclarent des
// capacités HONNÊTES : inférence uniquement ici, l'exécution outillée
// reste déclarée par l'agent Mistral uniquement.
package agents

import (
	"errors"

	"github.com/a2aproject/a2a-go/v2/a2a"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/socle"
)

// Executable : alias du contrat d'exécuteur du socle (a2a-go).
type Executable = coorda2a.Executable

// Serveur : serveur d'agent d'inférence d'un fournisseur sur le socle.
type Serveur struct {
	Carte   *a2a.AgentCard // carte effectivement servie (capacités honnêtes)
	interne *socle.Serveur
}

// URL : URL de base du serveur d'agent (résolution client du rail).
func (s *Serveur) URL() string { return s.interne.URL() }

// Fermer : arrêt RÉEL de l'écoute.
func (s *Serveur) Fermer() { s.interne.Fermer() }

// OptionsServeur : déclaration d'un agent d'inférence.
type OptionsServeur struct {
	Nom         string     // nom d'agent, ex. "coord-agent-anthropic"
	Description string     // description de la carte (défaut : DescriptionAgent)
	Jeton       string     // jeton du rail EXIGÉ (fail closed côté socle)
	Exec        Executable // exécuteur (ExecInférence)
	// Skills : capacités additionnelles HONNÊTES de la carte (M-COORD-
	// LLM-05 : un agent DEUX VOIES déclare inférence ET claude-code).
	// Vide = défaut (inférence seule), rétrocompatible.
	Skills []a2a.AgentSkill
}

// DemarrerServeur : carte aux capacités honnêtes (skill d'inférence
// UNIQUEMENT — l'exécution d'outils n'est ni implémentée ni déclarée)
// puis socle commun : loopback, jeton exigé, JSON-RPC A2A standard.
func DemarrerServeur(o OptionsServeur) (*Serveur, error) {
	if o.Exec == nil {
		return nil, errors.New("agents : EXEC_ABSENT : exécuteur exigé (socle, fail closed)")
	}
	if o.Jeton == "" {
		return nil, socle.ErrJetonVide
	}
	desc := o.Description
	if desc == "" {
		desc = DescriptionAgent
	}
	carte := coorda2a.CarteAgent(o.Nom, desc)
	carte.Skills = []a2a.AgentSkill{{
		Name:        SkillInférence,
		Description: "Voie inférence COORD : critique de plan, réception, direction (API fournisseur, fail closed).",
		Tags:        []string{"coord", "inference", "critique-plan", "reception"},
	}}
	if len(o.Skills) > 0 {
		// Surcharge honnête (additive) : chaque skill déclaré correspond à
		// une voie EFFECTIVEMENT implémentée par l'exécuteur passé.
		carte.Skills = o.Skills
	}
	interne, err := socle.NouveauServeurCarte(o.Nom, o.Exec, o.Jeton, carte)
	if err != nil {
		return nil, err
	}
	return &Serveur{Carte: carte, interne: interne}, nil
}

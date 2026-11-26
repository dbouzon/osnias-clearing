// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package connecteursapi : PONT PUBLIC D'ALIASES vers les connecteurs
// fournisseurs du moteur COORD (mission M-COORD-LLM-04, issue #56, ordre
// v1.0, arbitrage C' accepté par la critique OpenAI — session pont
// m-coord-llm-04, T2). AUCUN connecteur n'est déplacé ni dupliqué :
// `coordengine/internal/connecteurs` reste INCHANGÉ octet par octet,
// harnais historiques compris ; ce package expose, dans le MÊME module
// coordengine, les types publics par ALIAS de type Go : les alias
// transportent la MÉTHODE SET RÉELLE des types, donc la sécurité
// fail-closed (bornes, tarifs, expurgation des secrets, idempotence,
// loopback imposé hors triple verrou, MODELE_DIVERGENT,
// JETONS_INCONNUS) est LA MÊME instance de code, jamais une copie.
//
// FINALITÉ : permettre au module `coord/a2a-agent` (socle serveur A2A
// mutualisé) d'instancier les vrais points fournisseur pour la voie
// INFÉRENCE (critique de plan, réception), sans que ce second module
// puisse importer un package `internal` du moteur (règle Go : import
// internal interdit hors du module). Aucune réservation monétaire
// n'est exposée ici-appel : la voûte TX1/TX2 vit uniquement dans
// l'orchestrateur (orchestre.go) ; ce pont ne réexporte ni
// Orchestrateur, ni budget, ni transitions normatives.
package connecteursapi

import (
	"coordengine/internal/adaptateurs"
	"coordengine/internal/connecteurs"
)

// Aliases de types : MÊME identité de type et MÊME méthode set que le
// package interne (le code appelé est celui du moteur, pas une copie).
type (
	// Resultat : résultat d'un appel d'adaptateur (jetons, sortie, simule).
	Resultat = adaptateurs.Resultat
	// Autorisation : bloc d'autorisation explicite du pilote (mission #29).
	Autorisation = connecteurs.Autorisation
	// Config : configuration locale des connecteurs (jamais de secret).
	Config = connecteurs.Config
	// ConfigFournisseur : déclaration d'un fournisseur (endpoint, modèle,
	// format, profil, tarifs, bornes — fail closed au chargement).
	ConfigFournisseur = connecteurs.ConfigFournisseur
	// HTTPEndPoint : point fournisseur format OpenAI (profils gemini et
	// mistral possibles), fail closed, loopback factice autorisé.
	HTTPEndPoint = connecteurs.HTTPEndPoint
	// AnthropicPoint : point fournisseur format Messages API, fail closed.
	AnthropicPoint = connecteurs.AnthropicPoint
)

// ChargerConfig : lit et valide la configuration locale (fail closed).
// Simple délégation : aucune sémantique ajoutée ni retirée.
func ChargerConfig(chemin string) (*Config, error) {
	return connecteurs.ChargerConfig(chemin)
}

// ConnecteurReel : contrat « point réel » utilisé par la voie inférence
// de l'agent (sous-ensemble des actions sans verrou d'état moteur).
// C'est l'interface moteur `connecteurs.ConnecteurReel`, réexportée par
// alias : même contrat, aucune seconde définition.
type ConnecteurReel = connecteurs.ConnecteurReel

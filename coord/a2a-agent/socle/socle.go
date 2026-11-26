// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package socle : SOCLE SERVEUR D'AGENT A2A GÉNÉRIQUE et MUTUALISABLE
// (M-COORD-LLM-03, issue #54, plan v2 PLAN_ACCEPTE). Premier exemplaire
// du « schéma réplicable » de la note v1.1 §4.7 : un serveur par
// fournisseur, fournisseur-NEUTRE par construction.
//
// Réutilisation stricte (aucune duplication) :
//   - l'écoute loopback, l'exigence du jeton du rail (X-Coord-Rail-Jeton),
//     la carte d'agent standard et le handler JSON-RPC A2A viennent du
//     module coord/a2a (DemarrerAgentFactice, qualifié en #48/#51) ;
//   - le contrat d'exécuteur est Executable (alias du SDK a2a-go v2.9.0
//     ré-exporté par coord/a2a) : ExecMistral (inférence) et ExecVibe
//     (sous-processus) sont les MÊMES contrats que les factices ;
//   - aucune connaissance fournisseur dans ce package : l'exécuteur est
//     injecté. Le rail client, le registre durable, la voûte monétaire
//     (TX1/TX2) et les transitions normatives restent côté COORD : ce
//     socle n'est PAS un second orchestrateur.
//
// SÉMANTIQUE D'EXÉCUTION (fait de code découvert à l'inventaire T2) :
// le pont internal/a2a capture l'artefact À L'ENVOI (message/send) et
// REFUSE tout achèvement asynchrone non capturé (RESULTAT_NON_TRANSMIS,
// fail closed). Le serveur exécute donc le travail DANS l'appel
// message/send (le client HTTP reste bloqué pendant le run), la borne de
// 20 s côté pont ne s'appliquant qu'au suivi APRÈS un état non terminal.
// L'annulation passe par tasks/cancel : le contexte d'exécution est
// annulé, l'exécuteur émet l'événement canceled EXIGÉ par le SDK (piège
// documenté : une séquence vide ferait bloquer tasks/cancel).
package socle

import (
	"errors"

	"github.com/a2aproject/a2a-go/v2/a2a"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
)

// Executable : contrat d'exécuteur d'agent (frontière du module socle :
// les exécuteurs fournisseurs n'importent le SDK que pour implémenter).
type Executable = coorda2a.Executable

// ErrJetonVide : un agent sans jeton du rail est interdit (fail closed :
// le rail envoie TOUJOURS X-Coord-Rail-Jeton ; un serveur ouvert serait
// une surface non authentifiée).
var ErrJetonVide = errors.New("socle : jeton du rail exigé (fail closed, jamais vide)")

// Serveur : serveur d'agent A2A loopback (enveloppe du ServeurFactice
// du rail : écoute 127.0.0.1, carte sur /.well-known/agent-card.json,
// JSON-RPC sur /invoke, jeton vérifié sur tout appel d'invocation).
type Serveur struct {
	inner *coorda2a.ServeurFactice
}

// NouveauServeur : démarre un agent A2A loopback pour l'exécuteur donné,
// jeton du rail EXIGÉ. Boucle locale uniquement, par construction (le
// rail impose le loopback client-side, le socle impose le loopback
// server-side ; l'exposition distante serait une mission dédiée avec
// carte signée : arbitrage D1 du plan v2, hors périmètre).
func NouveauServeur(nom string, exec Executable, jeton string) (*Serveur, error) {
	if jeton == "" {
		return nil, ErrJetonVide
	}
	s, err := coorda2a.DemarrerAgentFactice(nom, exec, jeton)
	if err != nil {
		return nil, err
	}
	return &Serveur{inner: s}, nil
}

// URL : URL de BASE de l'agent (résolution de la carte, usage client).
func (s *Serveur) URL() string { return s.inner.URL() }

// NouveauServeurCarte : identique à NouveauServeur avec une carte
// d'agent fournie (M-COORD-LLM-04 #56 : capacités HONNÊTES déclarées
// par fournisseur). Additif ; NouveauServeur inchangé.
func NouveauServeurCarte(nom string, exec Executable, jeton string, carte *a2a.AgentCard) (*Serveur, error) {
	if jeton == "" {
		return nil, ErrJetonVide
	}
	s, err := coorda2a.DemarrerAgentFacticeCarte(nom, exec, jeton, carte)
	if err != nil {
		return nil, err
	}
	return &Serveur{inner: s}, nil
}

// Fermer : arrêt RÉEL (écoute fermée, appels ultérieurs refusés).
func (s *Serveur) Fermer() { s.inner.Fermer() }

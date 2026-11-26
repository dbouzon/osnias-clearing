// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package agents : EXECUTEURS D'INFÉRENCE A2A PAR FOURNISSEUR
// (M-COORD-LLM-04, issue #56). Deuxième exemplaire du schéma réplicable
// du socle (premier : paquet mistral) : Anthropic, OpenAI et Gemini se
// raccordent au MÊME socle serveur A2A que Mistral, par la voie
// INFÉRENCE UNIQUEMENT (critique de plan, réception, direction).
//
// AUCUN connecteur n'est réécrit : le point fournisseur est le VRAI
// connecteur du moteur COORD, importé via le pont public d'aliases
// `coordengine/connecteursapi` (arbitrage C' accepté) — les contrôles
// fail-closed (modèle exact, motif de fin nominal, usage, bornes,
// expurgation des secrets, idempotence, loopback imposé hors triple
// verrou) sont LA MÊME instance de code que l'orchestrateur, jamais une
// copie. La qualification pointe les connecteurs vers des serveurs
// HTTP FACTICES loopback par format : le VRAI chemin
// A2A → agents → connecteur → HTTP factice est exercé, aucun verdict
// préfabriqué.
//
// DISTINCTION INFÉRENCE / EXÉCUTION OUTILLÉE (ordre #56) : ce paquet
// n'exécute AUCUN outil. `VOIE:VIBE` est réservé au paquet mistral
// (runner supervisé) : toute charge VOIE_DE_MONTEE vers un agent
// d'inférence est REFUSÉE explicitement (CAPACITE_NON_IMPLEMENTEE) —
// une capacité non implémentée est refusée, jamais simulée
// silencieusement. Les cartes d'agent annoncent les capacités
// HONNÊTEMENT (Skills : inférence uniquement).
//
// BUDGET : AUCUNE réservation ici (voûte TX1/TX2 = orchestrateur,
// propriétaire unique). L'exécuteur vérifie seulement la cohérence
// locale : enveloppe de jetons allouée EXIGÉE (jamais « inconnu =
// gratuit »), jetons réels de la réponse bornés par cette enveloppe,
// tout écart = échec net journalisé, pas de consommation silencieuse.
package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"coordengine/connecteursapi"
)

// Capabilities honnêtes annoncées dans la carte d'agent (v1 : inférence
// seule pour les trois fournisseurs raccordés ici ; l'exécution
// outillée reste déclarée par le paquet mistral uniquement).
const (
	SkillInférence   = "coord-inference"        // critique de plan / réception (voie API)
	SkillExécution   = "coord-execution-outils" // réservé Mistral, JAMAIS ici
	DescriptionAgent = "Agent A2A COORD, voie inférence uniquement (M-COORD-LLM-04, qualification SIMULEE sur serveurs factices loopback, 0 EUR)"
)

// Marqueurs de charge (données opaques ; aucune instruction de la
// charge n'est exécutée — S9).
const marqueurVibe = "VOIE:VIBE"

// Erreurs fail-closed de l'exécuteur d'inférence.
var (
	ErrPointAbsent            = errors.New("agents : POINT_ABSENT : connecteur fournisseur exigé (pont connecteursapi, fail closed)")
	ErrEnveloppeAbsente       = errors.New("agents : ENVELOPPE_ABSENTE : enveloppe de jetons allouée exigée (voûte COORD, jamais inconnu=gratuit)")
	ErrCapaciteNonImplementee = errors.New("agents : CAPACITE_NON_IMPLEMENTEE : exécution outillée réservée à l'agent Mistral ; cette capacité est refusée, pas simulée")
	ErrChargeVide             = errors.New("agents : CHARGE_VIDE : charge exigée (fail closed)")
)

// ExecInférence : exécuteur A2A de la voie INFÉRENCE d'un fournisseur.
// Point = le vrai connecteur moteur (alias d'interface, aucune copie).
type ExecInférence struct {
	Nom       string // identité déclarée de l'agent (carte, journaux)
	Point     connecteursapi.ConnecteurReel
	Enveloppe int // enveloppe de jetons allouée par la voûte COORD (> 0 exigé)
}

// verifier : cohérence AVANT tout appel réseau (fail closed).
func (e *ExecInférence) verifier() error {
	if e.Point == nil {
		return ErrPointAbsent
	}
	if e.Enveloppe <= 0 {
		return ErrEnveloppeAbsente
	}
	return nil
}

// cleIdempotenceLocale : clé d'idempotence par délégation (contenu de
// la charge) — jamais un retry aveugle : deux délégations distinctes
// d'une même charge portent la même clé côté connecteur, l'anti-doublon
// DURABLE reste celui de l'orchestrateur.
func cleIdempotenceLocale(charge string) string {
	s := sha256.Sum256([]byte(charge))
	return hex.EncodeToString(s[:])
}

// Cancel : annulation CONFORME (l'événement canceled est EXIGÉ par le
// SDK ; une séquence vide ferait bloquer tasks/cancel — piège #25).
func (*ExecInférence) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return annulationConforme(ec)
}

func annulationConforme(ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// Execute : traduit la charge A2A en appel d'inférence via le VRAI
// connecteur, produit l'artefact strict du rail {"jetons", "texte"}.
// Le run a lieu DANS message/send (sémantique du socle, inchangée).
func (e *ExecInférence) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}
		if err := e.verifier(); err != nil {
			yield(echeac(ec, err), nil)
			return
		}
		if strings.HasPrefix(charge, marqueurVibe) {
			// Capacité NON implémentée : REFUS EXPLICITE, jamais une
			// simulation silencieuse (ordre #56, « capacités annoncées
			// honnêtement »).
			yield(echeac(ec, ErrCapaciteNonImplementee), nil)
			return
		}
		if strings.TrimSpace(charge) == "" {
			yield(echeac(ec, ErrChargeVide), nil)
			return
		}
		res, err := e.Point.ExecuterContexte(ctx, "executer", charge, cleIdempotenceLocale(charge))
		if err != nil {
			// Le message d'erreur du connecteur est DÉJÀ expurgé des
			// secrets (commun.secretRE) : aucune fuite possible ici.
			yield(echeac(ec, fmt.Errorf("agents : %s : %w", e.Nom, err)), nil)
			return
		}
		if res.Jetons > e.Enveloppe {
			// Écart d'enveloppe : échec NET (coût engagé mais jamais
			// accepté silencieusement ; la voûte COORD tranche au TX2).
			yield(echeac(ec, fmt.Errorf("agents : ENVELOPPE_DEPASSEE : %d jetons > enveloppe %d (fail closed)", res.Jetons, e.Enveloppe)), nil)
			return
		}
		if !yield(artefact(ec, res.Jetons, res.Sortie), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// Helpers wire (identiques au contrat strict du rail ; répétition
// volontairement minimale, hors logique de sécurité, cf. mistral).

func artefact(ec *a2asrv.ExecutorContext, jetons int, texte string) *a2a.TaskArtifactUpdateEvent {
	payload := fmt.Sprintf(`{"jetons":%d,"texte":%s}`, jetons, mustJSON(texte))
	return a2a.NewArtifactEvent(ec, a2a.NewTextPart(payload))
}

func echeac(ec *a2asrv.ExecutorContext, err error) *a2a.TaskStatusUpdateEvent {
	return a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
		a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error())))
}

func texteDeCharge(ec *a2asrv.ExecutorContext) string {
	if ec.Message == nil || len(ec.Message.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range ec.Message.Parts {
		b.WriteString(p.Text())
	}
	return b.String()
}

func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"?"`
	}
	return string(b)
}

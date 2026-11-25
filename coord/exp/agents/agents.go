// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package agents fournit les deux agents simulés de l'expérimentation
// M-COORD-EXP-01 (issue #38). Les agents sont des a2asrv.AgentExecutor
// du SDK officiel a2a-go : messages, événements, états de tâches et
// artefacts produits sont des objets A2A v1.0 réels, IDENTIQUES dans
// les deux modes (A2A direct et A2A encapsulé dans MCP). Seule
// l'enveloppe de transport diffère.
//
// Langage de tâche déterministe (données opaques, anti-injection) :
//
//	PING          -> complétée, artefact "PONG"
//	ECHO:<texte>  -> complétée, artefact = <texte>
//	COMPTER:n     -> artefact "1,2,...,n"
//	ECHEC         -> TASK_STATE_FAILED avec message d'erreur
//	PAUSE         -> TASK_STATE_INPUT_REQUIRED persistée (annulable)
//	LENT:<ms>     -> WORKING puis complétée (annulation par contexte)
//	GROS:<octets> -> artefact déterministe de <octets> octets
//
// Aucune instruction contenue dans le texte (tentatives d'injection
// S9/S10) n'influence un comportement privilégié : fail-closed.
package agents

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// Exec est l'agent exécutant simulé (AGENT-EXEC).
type Exec struct{}

// Crit est l'agent critique/récepteur simulé (AGENT-CRIT).
type Crit struct{}

// Execute implémente a2asrv.AgentExecutor pour AGENT-EXEC.
func (Exec) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return execute(ctx, ec, false)
}

// Execute implémente a2asrv.AgentExecutor pour AGENT-CRIT.
func (Crit) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return execute(ctx, ec, true)
}

// cancelSeq annule proprement une tâche : l'executor produit l'événement
// d'état CANCELED exigé par le SDK (un retour vide bloquerait le serveur).
func cancelSeq(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// Cancel implémente a2asrv.AgentExecutor pour AGENT-EXEC.
func (Exec) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return cancelSeq(ctx, ec)
}

// Cancel implémente a2asrv.AgentExecutor pour AGENT-CRIT.
func (Crit) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return cancelSeq(ctx, ec)
}

func texteTache(ec *a2asrv.ExecutorContext) string {
	if ec.Message == nil || len(ec.Message.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range ec.Message.Parts {
		b.WriteString(p.Text())
	}
	return b.String()
}

func artefact(ec *a2asrv.ExecutorContext, texte string) *a2a.TaskArtifactUpdateEvent {
	return a2a.NewArtifactEvent(ec, a2a.NewTextPart(texte))
}

func execute(ctx context.Context, ec *a2asrv.ExecutorContext, crit bool) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		tache := texteTache(ec)

		// L'A2A v1.0 exige un premier événement Task (submitted) avant
		// toute mise à jour d'état.
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}

		// AGENT-CRIT : verdict structuré déterministe sur le livrable.
		if crit {
			verdict := "REFUSEE_LIVRABLE_VIDE"
			motif := "livrable vide"
			if strings.TrimSpace(tache) != "" {
				verdict = "ACCEPTEE"
				motif = "livrable non vide, conforme au format attendu"
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("analyse du livrable"))), nil)
			if !yield(artefact(ec, fmt.Sprintf(`{"verdict":"%s","motif":"%s"}`, verdict, motif)), nil) {
				return
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
			return
		}

		// AGENT-EXEC : langage de tâque déterministe.
		champ, valeur, _ := strings.Cut(tache, ":")
		switch champ {
		case "PING":
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil)
			yield(artefact(ec, "PONG"), nil)
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
		case "ECHO":
			yield(artefact(ec, valeur), nil)
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
		case "COMPTER":
			n := 0
			fmt.Sscanf(valeur, "%d", &n)
			var b strings.Builder
			for i := 1; i <= n; i++ {
				if i > 1 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, "%d", i)
				yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil)
			}
			if !yield(artefact(ec, b.String()), nil) {
				return
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
		case "PAUSE":
			// Pause A2A conforme : la tâche passe en TASK_STATE_INPUT_REQUIRED
			// persistée dans le magasin ; le client peut relire (tasks/get)
			// ou annuler (tasks/cancel) la tâche (S5).
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateInputRequired, nil), nil)
		case "ECHEC":
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("ECHEC_SIMULE: entrée en erreur demandée"))), nil)
		case "LENT":
			ms := 0
			fmt.Sscanf(valeur, "%d", &ms)
			if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
				return
			}
			for i := 0; i < ms; i += 25 {
				pas := 25
				if ms-i < pas {
					pas = ms - i
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(pas) * time.Millisecond):
				}
			}
			if !yield(artefact(ec, "FIN_LENT"), nil) {
				return
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
		case "GROS":
			n := 0
			fmt.Sscanf(valeur, "%d", &n)
			b := make([]byte, n)
			for i := range b {
				b[i] = byte('A' + (i % 26))
			}
			if !yield(artefact(ec, string(b)), nil) {
				return
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
		default:
			// Données opaques : aucun comportement privilégié n'est dérivé
			// du contenu (anti-injection, fail-closed).
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("TACHE_INCONNUE: aucune instruction reconnue"))), nil)
		}
	}
}

// Cartes retourne les AgentCard des deux agents (adresses des interfaces).
func Cartes(urlExec, urlCrit string) (exec *a2a.AgentCard, crit *a2a.AgentCard) {
	exec = &a2a.AgentCard{
		Name:        "coord-exp-agent-exec",
		Description: "Agent executant simule (M-COORD-EXP-01)",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(urlExec, a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Skills: []a2a.AgentSkill{{
			ID:          "execution",
			Name:        "Execution de tache simulee",
			Description: "Execute une tache deterministe (PING, ECHO, COMPTER, ECHEC, PAUSE, LENT, GROS)",
		}},
	}
	crit = &a2a.AgentCard{
		Name:        "coord-exp-agent-crit",
		Description: "Agent critique simule (M-COORD-EXP-01)",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(urlCrit, a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Skills: []a2a.AgentSkill{{
			ID:          "critique",
			Name:        "Reception d'un livrable",
			Description: "Rend un verdict structure deterministe",
		}},
	}
	return exec, crit
}

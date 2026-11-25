// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package modea

import (
	"context"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// ResultatSend résume un résultat message/send pour la comparaison
// inter-modes (champ sémantique uniquement : état de tâche et texte).
type ResultatSend struct {
	TaskID string
	Etat   string
	Texte  string
}

// Evenement résume un événement de streaming pour la comparaison
// inter-modes (type, état, extrait de texte).
type Evenement struct {
	Type   string `json:"type"`
	Etat   string `json:"etat,omitempty"`
	Texte  string `json:"texte,omitempty"`
	TaskID string `json:"taskId,omitempty"`
}

// PileClient encapsule un client A2A avec des helpers haut niveau.
type PileClient struct {
	Client *a2aclient.Client
}

// SendMessageBrut envoie message/send et retourne le résultat A2A
// réel (Task ou Message), pour relais fidèle par la passerelle MCP.
func (pc *PileClient) SendMessageBrut(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error) {
	return pc.Client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
}

// SendMessage envoie message/send et résume le résultat.
func (pc *PileClient) SendMessage(ctx context.Context, msg *a2a.Message) (*ResultatSend, error) {
	res, err := pc.Client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, err
	}
	return resumer(res), nil
}

// Stream envoie message/stream et collecte la séquence d'événements.
func (pc *PileClient) Stream(ctx context.Context, msg *a2a.Message) ([]Evenement, error) {
	var seq []Evenement
	for ev, err := range pc.Client.SendStreamingMessage(ctx, &a2a.SendMessageRequest{Message: msg}) {
		if err != nil {
			return seq, err
		}
		seq = append(seq, resumerEvenement(ev))
	}
	return seq, nil
}

// resumer extrait état + texte d'un résultat message/send.
func resumer(res a2a.SendMessageResult) *ResultatSend {
	r := &ResultatSend{}
	switch v := res.(type) {
	case *a2a.Task:
		r.TaskID = string(v.ID)
		r.Etat = string(v.Status.State)
		if len(v.Artifacts) > 0 && len(v.Artifacts[0].Parts) > 0 {
			r.Texte = v.Artifacts[0].Parts[0].Text()
		}
	case *a2a.Message:
		if len(v.Parts) > 0 {
			r.Texte = v.Parts[0].Text()
		}
	}
	return r
}

// resumerEvenement résume un événement A2A pour la comparaison.
func resumerEvenement(ev a2a.Event) Evenement {
	e := Evenement{}
	switch v := ev.(type) {
	case *a2a.Task:
		e.Type = "task"
		e.TaskID = string(v.ID)
		e.Etat = string(v.Status.State)
	case *a2a.TaskStatusUpdateEvent:
		e.Type = "status"
		e.TaskID = string(v.TaskID)
		e.Etat = string(v.Status.State)
		if v.Status.Message != nil && len(v.Status.Message.Parts) > 0 {
			e.Texte = v.Status.Message.Parts[0].Text()
		}
	case *a2a.TaskArtifactUpdateEvent:
		e.Type = "artifact"
		e.TaskID = string(v.TaskID)
		if len(v.Artifact.Parts) > 0 {
			e.Texte = v.Artifact.Parts[0].Text()
		}
	case *a2a.Message:
		e.Type = "message"
		if len(v.Parts) > 0 {
			e.Texte = v.Parts[0].Text()
		}
	default:
		e.Type = "autre"
	}
	return e
}

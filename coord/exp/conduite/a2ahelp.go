// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package conduite

import (
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// a2aEtatInputRequired : état A2A v1.0 attendu par le scénario S5.
const a2aEtatInputRequired = string(a2a.TaskStateInputRequired)

// a2aMsg construit un message A2A utilisateur à partir d un texte.
func a2aMsg(texte string) *a2a.Message {
	return a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(texte))
}

func reqGetTask(id string) *a2a.GetTaskRequest { return &a2a.GetTaskRequest{ID: a2a.TaskID(id)} }

func reqCancel(id string) *a2a.CancelTaskRequest {
	return &a2a.CancelTaskRequest{ID: a2a.TaskID(id)}
}

// verifierEtat échoue si l état constaté n est pas l état attendu.
func verifierEtat(eu, attendu string) error {
	if eu != attendu {
		return errEtat{eu: eu, attendu: attendu}
	}
	return nil
}

type errEtat struct{ eu, attendu string }

func (e errEtat) Error() string {
	return "etat " + e.eu + " attendu " + e.attendu
}

// wireTask est la forme wire d un Task A2A v1.0, pour décoder les
// sorties JSON des outils MCP du mode B.
type wireTask struct {
	ID     string `json:"id"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
	Artifacts []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"artifacts"`
}

func (w wireTask) etat() string { return w.Status.State }

func (w wireTask) taskID() string { return w.ID }

// texte retourne le texte du premier artifact (vide sinon).
func (w wireTask) texte() string {
	if len(w.Artifacts) == 0 || len(w.Artifacts[0].Parts) == 0 {
		return ""
	}
	return w.Artifacts[0].Parts[0].Text
}

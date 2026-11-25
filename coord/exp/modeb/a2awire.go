// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package modeb

import "github.com/a2aproject/a2a-go/v2/a2a"

// Alias wire A2A utilisés par la passerelle MCP.
type (
	a2aMessage      = a2a.Message
	a2aGetTaskReq   = a2a.GetTaskRequest
	a2aCancelReq    = a2a.CancelTaskRequest
	a2aListTasksReq = a2a.ListTasksRequest
)

// a2aNewText construit un Message A2A à partir d'un simple texte.
func a2aNewText(texte string) *a2a.Message {
	return a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(texte))
}

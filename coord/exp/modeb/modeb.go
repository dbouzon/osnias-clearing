// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package modeb implémente le MODE B de M-COORD-EXP-01 : A2A
// ENCAPSULÉ DANS MCP. Une passerelle MCP expérimentale (isolée dans
// coord/exp, hors du moteur COORD et du serveur MCP #37) expose les
// opérations A2A comme outils MCP ; elle dialogue avec les agents par
// le même SDK a2a-go et la même pile serveur que le mode A, de sorte
// que les messages A2A produits sont IDENTIQUES. Seule l'enveloppe
// (frames JSON-RPC MCP, transport en mémoire) est ajoutée.
package modeb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dbouzon/osnias-clearing/coord/exp/agents"
	"github.com/dbouzon/osnias-clearing/coord/exp/modea"
)

// Pile du mode B : couche MCP (outils) encapsulant la couche A2A
// (clients a2a-go en loopback). Mesures.Enveloppe = octets des frames
// MCP ; PileA.Mesures.Octets = octets de la couche A2A interne.
type Pile struct {
	PileA     *modea.Pile
	Serveur   *mcp.Server
	Session   *mcp.ClientSession
	Appels    int64
	Enveloppe int64
}

// argsCarte : arguments de a2a_get_card.
type argsCarte struct {
	Agent string `json:"agent"`
}

// argsMessage : arguments de a2a_send / a2a_send_stream. Message est
// au choix un simple texte ou un objet Message A2A complet.
type argsMessage struct {
	Agent   string `json:"agent"`
	Message any    `json:"message"`
}

// argsID : arguments de a2a_get_task / a2a_cancel.
type argsID struct {
	ID string `json:"id"`
}

func outilTexte(out string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: out}},
	}, nil, nil
}

func (p *Pile) client(agent string) *modea.PileClient {
	if agent == "crit" {
		return &modea.PileClient{Client: p.PileA.ClientCrit()}
	}
	return &modea.PileClient{Client: p.PileA.ClientExec()}
}

// messageA2A transforme l'argument en *a2a.Message.
func messageA2A(v any) (*a2aMessage, error) {
	switch m := v.(type) {
	case string:
		return a2aNewText(m), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var msg a2aMessage
		if err := json.Unmarshal(b, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	}
}

func (p *Pile) carte(agent string) (string, error) {
	exec, crit := agents.Cartes(p.PileA.URLExec(), p.PileA.URLCrit())
	if agent == "crit" {
		b, err := json.Marshal(crit)
		return string(b), err
	}
	b, err := json.Marshal(exec)
	return string(b), err
}

// NouvellePile démarre la couche A2A (loopback) puis la couche MCP
// par-dessus, avec transports MCP en mémoire.
func NouvellePile(ctx context.Context) (*Pile, error) {
	pa, err := modea.NouvellePile(ctx)
	if err != nil {
		return nil, err
	}
	p := &Pile{PileA: pa}

	p.Serveur = mcp.NewServer(
		&mcp.Implementation{Name: "coord-exp-passerelle-mcp", Title: "Passerelle experimentale A2A-over-MCP", Version: "0.1.0"},
		nil,
	)

	// a2a_get_card : découverte de l'AgentCard via l'outil MCP.
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_get_card",
		Description: "Retourne l'AgentCard A2A v1.0 de l'agent demande (mode B : decouverte encapsulee).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in argsCarte) (*mcp.CallToolResult, any, error) {
		out, err := p.carte(in.Agent)
		return outilTexte(out, err)
	})

	// a2a_send : message/send A2A via l'outil MCP.
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_send",
		Description: "Envoie un message A2A (message/send) a l'agent et retourne le resultat A2A (Task ou Message) en JSON.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in argsMessage) (*mcp.CallToolResult, any, error) {
		msg, err := messageA2A(in.Message)
		if err != nil {
			return outilTexte("", err)
		}
		res, err := p.client(in.Agent).SendMessageBrut(ctx, msg)
		if err != nil {
			return outilTexte("", err)
		}
		b, err := json.Marshal(res)
		return outilTexte(string(b), err)
	})

	// a2a_get_task : relit une tâche persistée.
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_get_task",
		Description: "Relit une tâche A2A persistée (tasks/get) par son identifiant.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in argsID) (*mcp.CallToolResult, any, error) {
		res, err := p.PileA.ClientExec().GetTask(ctx, &a2aGetTaskReq{ID: a2a.TaskID(in.ID)})
		if err != nil {
			return outilTexte("", err)
		}
		b, err := json.Marshal(res)
		return outilTexte(string(b), err)
	})

	// a2a_list_tasks : liste les tâches (métadonnées de gouvernance).
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_list_tasks",
		Description: "Liste les tâches A2A persistées (tasks/list).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		res, err := p.PileA.ClientExec().ListTasks(ctx, &a2aListTasksReq{})
		if err != nil {
			return outilTexte("", err)
		}
		b, err := json.Marshal(res)
		return outilTexte(string(b), err)
	})

	// a2a_cancel : annule une tâche persistée.
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_cancel",
		Description: "Annule une tâche A2A persistée (tasks/cancel).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in argsID) (*mcp.CallToolResult, any, error) {
		res, err := p.PileA.ClientExec().CancelTask(ctx, &a2aCancelReq{ID: a2a.TaskID(in.ID)})
		if err != nil {
			return outilTexte("", err)
		}
		b, err := json.Marshal(res)
		return outilTexte(string(b), err)
	})

	// a2a_send_stream : message/stream A2A via l'outil MCP. Envelope
	// MCP unidirectionnelle : la séquence d'événements est retournée
	// APRÈS l'appel (dégradation documentée du mode B).
	mcp.AddTool(p.Serveur, &mcp.Tool{
		Name:        "a2a_send_stream",
		Description: "Envoie un message A2A en streaming (message/stream) et retourne la sequence complete des evenements A2A en JSON.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in argsMessage) (*mcp.CallToolResult, any, error) {
		msg, err := messageA2A(in.Message)
		if err != nil {
			return outilTexte("", err)
		}
		seq, err := p.client(in.Agent).Stream(ctx, msg)
		if err != nil {
			return outilTexte("", err)
		}
		b, err := json.Marshal(seq)
		return outilTexte(string(b), err)
	})

	tS, tC := mcp.NewInMemoryTransports()
	if _, err := p.Serveur.Connect(ctx, tS, nil); err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "coord-exp-client-mcp", Title: "Client MCP experimental", Version: "0.1.0"}, nil)
	if p.Session, err = client.Connect(ctx, tC, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// Appeler invoque un outil MCP et comptabilise l'enveloppe (octets des
// frames aller et retour, transport MCP en mémoire).
func (p *Pile) Appeler(ctx context.Context, outil string, args map[string]any) (*mcp.CallToolResult, error) {
	res, err := p.Session.CallTool(ctx, &mcp.CallToolParams{Name: outil, Arguments: args})
	if err != nil {
		return nil, err
	}
	b1, _ := json.Marshal(&mcp.CallToolParams{Name: outil, Arguments: args})
	b2, _ := json.Marshal(res)
	p.Appels++
	p.Enveloppe += int64(len(b1) + len(b2))
	return res, nil
}

// Fermer libère la session MCP et la pile A2A.
func (p *Pile) Fermer() {
	if p.Session != nil {
		p.Session.Close()
	}
	if p.PileA != nil {
		p.PileA.Fermer()
	}
}

// Texte extrait le texte du premier TextContent du résultat.
func Texte(res *mcp.CallToolResult) (string, error) {
	if res.IsError {
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				return "", fmt.Errorf("outil MCP: %s", tc.Text)
			}
		}
		return "", fmt.Errorf("outil MCP en erreur")
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text, nil
		}
	}
	return "", fmt.Errorf("resultat MCP sans texte")
}

// MesuresA2A retourne les octets de la couche A2A interne du mode B.
func (p *Pile) MesuresA2A() (octets int64, echanges int64) {
	return p.PileA.Mesures.Octets, p.PileA.Mesures.Echanges
}

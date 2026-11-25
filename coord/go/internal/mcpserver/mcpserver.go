// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package mcpserver implémente le serveur MCP commun de COORD (M-COORD-MCP-01,
// issue #35) : lecture seule stricte au-dessus du service COORD Pilot local.
// Aucune mutation n'est exposée ; le paquet ne connaît que le jeton de lecture
// (jamais le token console du pilote) et aucun secret ne figure dans les
// messages d'erreur, les journaux ou les réponses MCP (fail-closed).
//
// Périmètre du transport : ce paquet n'attache AUCUN transport au serveur ;
// cmd/coord-mcp choisit le transport local par défaut et le cœur reste
// utilisable tel quel pour une future architecture client-serveur distante
// (transport diffus HTTP) sans changer le contrat d'outils.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// OutilLister expose l'état résumé des missions (lecture seule).
	OutilLister = "coord_list_missions"
	// OutilDetail renvoie l'enregistrement complet d'une mission (lecture seule).
	OutilDetail = "coord_get_mission"
)

// requeteDetail est le schéma strict d'entrée de coord_get_mission.
type requeteDetail struct {
	Issue string `json:"issue" jsonschema:"identifiant de la mission (obligatoire, non vide)"`
}

// ErreurMCP est un refus explicite, stable et SANS secret : ni jeton, ni URL,
// ni chemin de fichier, ni corps brut du service Pilot.
type ErreurMCP struct {
	Code  string
	Motif string
}

func (e *ErreurMCP) Error() string { return e.Code + " : " + e.Motif }

// Refus renvoie une erreur MCP interprétable côté client sans effet de bord.
func Refus(code, motif string) *ErreurMCP {
	return &ErreurMCP{Code: code, Motif: motif}
}

// Serveur est le cœur lecture seule : il appelle uniquement les routes GET
// de lecture du service Pilot (avec le jeton de lecture) et mappe les erreurs.
type Serveur struct {
	baseURL string
	jeton   string
	client  *http.Client
}

// NouveauServeur valide l'URL (loopback obligatoire, fail-closed), le jeton
// (non vide) et construit le cœur lecture seule. Aucun secret n'est exposé
// dans les erreurs renvoyées.
func NouveauServeur(baseURL, jeton string) (*Serveur, error) {
	if strings.TrimSpace(jeton) == "" {
		return nil, errors.New("mcpserver : jeton de lecture requis (fail closed)")
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return nil, errors.New("mcpserver : URL du service Pilot invalide (fail closed)")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, errors.New("mcpserver : l'URL du service Pilot n'est pas loopback (refus, fail closed)")
	}
	return &Serveur{
		baseURL: strings.TrimRight(u.String(), "/"),
		jeton:   strings.TrimSpace(jeton),
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// appelerGET appelle une route de lecture du service Pilot avec le jeton de
// lecture uniquement. Toute erreur est convertie en refus sans secret.
func (s *Serveur) appelerGET(ctx context.Context, chemin string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+chemin, nil)
	if err != nil {
		return nil, Refus("REQUETE_INVALIDE", "requete impossible a construire")
	}
	req.Header.Set("X-Coord-Pilot-Token", s.jeton)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, Refus("PILOT_INJOIGNABLE", "service COORD Pilot local injoignable")
	}
	defer resp.Body.Close()
	corps, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return nil, Refus("REPONSE_ILLISIBLE", "reponse du service illisible")
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return json.RawMessage(corps), nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, Refus("JETON_REFUSE", "jeton de lecture absent, invalide ou perime")
	case resp.StatusCode == http.StatusNotFound:
		return nil, Refus("MISSION_INCONNUE", "mission inconnue")
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return nil, Refus("ACCES_REFUSE", fmt.Sprintf("acces refuse par le service (HTTP %d)", resp.StatusCode))
	default:
		return nil, Refus("SERVICE_INDISPONIBLE", "service COORD Pilot momentanement indisponible")
	}
}

// ListerMissions renvoie le résumé des missions (données pures du service).
func (s *Serveur) ListerMissions(ctx context.Context) (json.RawMessage, error) {
	return s.appelerGET(ctx, "/api/missions")
}

// ObtenirMission renvoie l'enregistrement complet d'une mission (journal
// inclus, données pures, jamais interprétées).
func (s *Serveur) ObtenirMission(ctx context.Context, issue string) (json.RawMessage, error) {
	issue = strings.TrimSpace(issue)
	if issue == "" {
		return nil, Refus("REQUETE_INVALIDE", "identifiant de mission requis")
	}
	if strings.ContainsAny(issue, " \t\r\n/") || len(issue) > 128 {
		return nil, Refus("REQUETE_INVALIDE", "identifiant de mission invalide")
	}
	return s.appelerGET(ctx, "/api/mission/"+url.PathEscape(issue))
}

// Peupler enregistre les deux outils COORD (lecture seule stricte) sur un
// serveur MCP du SDK. Aucun outil de mutation n'est ajouté, par construction.
func (s *Serveur) Peupler(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        OutilLister,
		Description: "Liste les missions COORD du service Pilot local (lecture seule).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return s.repondreJSON(func() (json.RawMessage, error) { return s.ListerMissions(ctx) })
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        OutilDetail,
		Description: "Renvoie l'enregistrement complet d'une mission COORD (lecture seule).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in requeteDetail) (*mcp.CallToolResult, any, error) {
		return s.repondreJSON(func() (json.RawMessage, error) { return s.ObtenirMission(ctx, in.Issue) })
	})
}

// repondreJSON serialise la réponse en texte JSON pur ; toute erreur devient
// un refus MCP explicite (IsError), sans secret ni effet de bord.
func (s *Serveur) repondreJSON(obtenir func() (json.RawMessage, error)) (*mcp.CallToolResult, any, error) {
	donnees, err := obtenir()
	if err != nil {
		var emcp *ErreurMCP
		if errors.As(err, &emcp) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: emcp.Code + " : " + emcp.Motif}},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "REFUS : requete refusee"}},
		}, nil, nil
	}
	var compact any
	if err := json.Unmarshal(donnees, &compact); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "REPONSE_ILLISIBLE : donnees invalides"}},
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(donnees)}},
	}, compact, nil
}

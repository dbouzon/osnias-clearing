// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// factices.go : SERVEURS HTTP FACTICES PAR FORMAT FOURNISSEUR pour la
// qualification SIMULEE du paquet agents (M-COORD-LLM-04). Loopback
// UNIQUEMENT, aucune clé, aucun compte, 0 EUR, aucun réseau fournisseur :
// ils imitent les FORMATS DE FIL (Anthropic Messages / OpenAI chat
// completions, aussi utilisé par les profils gemini et mistral) pour que
// le VRAI chemin A2A → agents → connecteur moteur → HTTP s'exerce de bout
// en bout — aucun verdict préfabriqué côté agent (l'ordre #56 l'exige).
//
// Rétroaction par marqueurs SCENARIO: insérés dans la CHARGE (données
// opaques, jamais des instructions exécutées) :
//
//	SCENARIO:DIV_MDL     la réponse annonce un modèle divergent (Q3)
//	SCENARIO:TRONQUE     fin non nominale (max_tokens / length) (Q5)
//	SCENARIO:SANS_USAGE  usage absent du corps (Q6)
//	SCENARIO:401         refus d'authentification (Q7)
//	SCENARIO:A_CORRIGER  la réception rend A_CORRIGER (cycles)
//	SCENARIO:5XX         erreur serveur
package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Formats de fil admis pour les factices.
const (
	FormatAnthropic = "anthropic"
	FormatOpenAI    = "openai"
)

// OptionsFactice : déclaration d'un serveur factice de format.
type OptionsFactice struct {
	Format string // "anthropic" ou "openai" (profils gemini/mistral compris)
	Modele string // modèle EXACT que le factice doit échoyer
}

// reponses wire minimales par format (champs analysés par le connecteur).
type reponseFacticeAnthropic struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

type reponseFacticeOpenAI struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// DemarrerFactice : serveur factice loopback du format donné.
func DemarrerFactice(o OptionsFactice) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		charge := ""
		if n := len(req.Messages); n > 0 {
			charge = req.Messages[n-1].Content
		}
		if strings.Contains(charge, "SCENARIO:401") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.Contains(charge, "SCENARIO:5XX") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		modele := o.Modele
		if strings.Contains(charge, "SCENARIO:DIV_MDL") {
			modele = o.Modele + "-divergent"
		}
		texte := verdictFactice(charge)
		in := 1 + len(charge)/4
		out := 1 + len(texte)/4
		w.Header().Set("Content-Type", "application/json")
		switch o.Format {
		case FormatAnthropic:
			rep := reponseFacticeAnthropic{ID: "msg_factice", Model: modele}
			rep.Content = []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: texte}}
			rep.StopReason = "end_turn"
			if strings.Contains(charge, "SCENARIO:TRONQUE") {
				rep.StopReason = "max_tokens"
			}
			if !strings.Contains(charge, "SCENARIO:SANS_USAGE") {
				rep.Usage = &struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				}{InputTokens: in, OutputTokens: out}
			}
			json.NewEncoder(w).Encode(rep)
		default: // openai (profils gemini/mistral)
			rep := reponseFacticeOpenAI{ID: "cmpl_factice", Model: modele}
			rep.Choices = []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{}}
			rep.Choices[0].Message.Content = texte
			rep.Choices[0].FinishReason = "stop"
			if strings.Contains(charge, "SCENARIO:TRONQUE") {
				rep.Choices[0].FinishReason = "length"
			}
			if !strings.Contains(charge, "SCENARIO:SANS_USAGE") {
				rep.Usage = &struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				}{PromptTokens: in, CompletionTokens: out, TotalTokens: in + out}
			}
			json.NewEncoder(w).Encode(rep)
		}
	}))
}

// verdictFactice : contenu d'inférence ANCRÉ à l'empreinte extraite de
// la charge (machine normative COORD, gabarit identique au paquet
// mistral — jamais un verdict préfabriqué côté agent).
func verdictFactice(charge string) string {
	emp := empDeCharge(charge)
	if emp == "" {
		return "FACTICE_ECHO : " + tronque(charge, 200)
	}
	switch {
	case strings.Contains(charge, "CRITIQUE DE PLAN"):
		return fmt.Sprintf(`{"verdict":"PLAN_ACCEPTE","points":[],"empreinte_attendue":%q}`, emp)
	case strings.Contains(charge, "RECEPTION INDEPENDANTE"):
		v := "ACCEPTEE"
		if strings.Contains(charge, "SCENARIO:A_CORRIGER") {
			v = "A_CORRIGER"
		}
		return fmt.Sprintf(`{"verdict":%q,"points":[],"empreinte_attendue":%q,`+
			`"verdict_id":"FACTICE-LLM04","verdict_url":"https://example.invalid/llm04"}`,
			v, emp)
	default:
		return "FACTICE_ECHO : " + tronque(charge, 200)
	}
}

// empDeCharge : empreinte 64 hex attendue, extraite de la charge.
// Deux formes admises (gabarit brut ou échappé dans un gabarit JSON).
func empDeCharge(charge string) string {
	marque := `empreinte_attendue":"` // gabarit BRUT
	e := indexEmp(charge, marque)
	if e == "" {
		marque = `empreinte_attendue\":\"` // gabarit ÉCHAPPÉ (dans JSON)
		e = indexEmp(charge, marque)
	}
	return e
}

func indexEmp(charge, marque string) string {
	i := strings.Index(charge, marque)
	if i < 0 {
		return ""
	}
	reste := charge[i+len(marque):]
	if len(reste) < 64 {
		return ""
	}
	suite := reste[:64]
	for _, c := range suite {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return suite
}

func tronque(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

package aic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RealOpenAI est l'adaptateur réel de l'agent chef de projet OpenAI (M-AIC-04).
// Il réalise Reviewer sur l'API HTTP chat/completions de OpenAI.
//
// Règles de sécurité :
//   - la clé d'API est détenue uniquement en mémoire et transmise par
//     l'en-tête Authorization ; elle n'est JAMAIS journalisée ni persistée ;
//   - l'adaptateur n'effectue AUCUN appel payant tant que la clé est absente
//     (refus de construction, fail closed) ;
//   - l'appel est borné : délai par requête, plafond de jetons de réponse ;
//   - aucune re-soumission automatique : un statut défavorable ou une
//     réponse ambiguë est une erreur, rejouée par la file d'événements si le
//     coordinateur le décide, jamais en boucle dans l'adaptateur ;
//   - la réponse distante doit être structurée (decision ACCEPTED |
//     CORRECTION | BLOCKED, summary) ; une réponse non structurée est une
//     erreur explicite, jamais transformée en verdict par défaut ;
//   - chaque appel journalise phase, modèle, statut, tokens, identifiant de
//     corrélation (en-tête x-request-id) et extrait borné de la réponse ;
//     jamais la clé, jamais les en-têtes d'authentification.

// RealOpenAIOptions regroupe les paramètres de construction de l'adaptateur.
type RealOpenAIOptions struct {
	APIKey    string                                 // clé OpenAI, mémoire uniquement, jamais journalisée
	APIURL    string                                 // racine COMPLÈTE de l'API (vide = https://api.openai.com/v1)
	Model     string                                 // modèle interrogé (défaut gpt-4o-mini)
	Timeout   time.Duration                          // délai par requête (défaut 60 s)
	MaxTokens int                                    // plafond de jetons de réponse (défaut 400)
	Log       func(missionID, phase, message string) // journal SQLite optionnel
}

// RealOpenAI réalise Reviewer via l'API OpenAI.
type RealOpenAI struct {
	apiKey    string
	apiURL    string
	model     string
	client    *http.Client
	maxTokens int
	log       func(missionID, phase, message string)
}

// NewRealOpenAI construit l'adaptateur réel. Fail closed : sans clé, la
// construction échoue et aucun appel n'est possible.
func NewRealOpenAI(opts RealOpenAIOptions) (*RealOpenAI, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("adaptateur API OpenAI : clé absente (AIC_OPENAI_APIKEY), refus (fail closed)")
	}
	apiURL := strings.TrimRight(strings.TrimSpace(opts.APIURL), "/")
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 400
	}
	return &RealOpenAI{
		apiKey:    opts.APIKey,
		apiURL:    apiURL,
		model:     model,
		client:    &http.Client{Timeout: timeout},
		maxTokens: maxTokens,
		log:       opts.Log,
	}, nil
}

// openAIRequest est le corps d'appel chat/completions.
type openAIRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_completion_tokens"`
	Messages  []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse est la réponse chat/completions (champs exploités seulement).
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIVerdict est la réponse structurée attendue de l'agent chef de projet.
type openAIVerdict struct {
	Decision string `json:"decision"`
	Summary  string `json:"summary"`
	Comments []struct {
		Ref  string `json:"ref"`
		Note string `json:"note"`
	} `json:"comments"`
}

// openAISystemPrompt est l'instruction système imposée à l'agent distant :
// la réponse doit être structurée pour le coordinateur, jamais du texte libre.
const openAISystemPrompt = `Tu es l'agent chef de projet OpenAI d'Osnias AI Coordinator.
Tu reçois une mission et une livraison de l'agent développeur Mistral Code.
Réponds STRICTEMENT en JSON valide, sans texte hors JSON, avec les champs :
{"decision":"ACCEPTED|CORRECTION|BLOCKED","summary":"réponse en une phrase adressée à Mistral Code","comments":[{"ref":"référence","note":"remarque"}]}
Le champ summary est ta réponse directe à Mistral Code.`

// Review implémente Reviewer par un appel réel à l'API OpenAI.
func (r *RealOpenAI) Review(in ReviewInput) (ReviewOutput, error) {
	return r.Call(in.MissionID, promptForReview(in))
}

// promptForReview construit le message utilisateur : mission, rapport, diff.
func promptForReview(in ReviewInput) string {
	var b strings.Builder
	b.WriteString("Mission " + in.MissionID + "\n")
	b.WriteString("Rapport de l'agent développeur :\n" + in.Report + "\n")
	if strings.TrimSpace(in.Diff) != "" {
		b.WriteString("Diff (résumé) :\n" + in.Diff + "\n")
	}
	return b.String()
}

// Call transmet un message à l'agent OpenAI et retourne sa réponse structurée.
// C'est le point d'appel unique : journalisation, bornage et contrôles.
func (r *RealOpenAI) Call(missionID, userMessage string) (ReviewOutput, error) {
	started := time.Now()
	body, err := json.Marshal(openAIRequest{
		Model:     r.model,
		MaxTokens: r.maxTokens,
		Messages: []openAIMessage{
			{Role: "system", Content: openAISystemPrompt},
			{Role: "user", Content: userMessage},
		},
	})
	if err != nil {
		return ReviewOutput{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.apiURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ReviewOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey) // jamais journalisé
	req.Header.Set("User-Agent", "osnias-aic-coordinator")
	r.logf(missionID, "call", fmt.Sprintf("appel API OpenAI modèle=%s max_tokens=%d", r.model, r.maxTokens))
	resp, err := r.client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("échec d'appel API OpenAI : %v", err)
		r.logf(missionID, "error", msg)
		return ReviewOutput{}, errors.New(msg)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	correlation := resp.Header.Get("X-Request-Id")
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("statut HTTP %d de l'API OpenAI (corrélation=%s) : %s", resp.StatusCode, correlation, excerpt(raw, 300))
		r.logf(missionID, "error", msg)
		return ReviewOutput{}, errors.New(msg)
	}
	var out openAIResponse
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		msg := fmt.Sprintf("réponse OpenAI illisible ou vide (corrélation=%s) : %s", correlation, excerpt(raw, 300))
		r.logf(missionID, "error", msg)
		return ReviewOutput{}, errors.New(msg)
	}
	content := strings.TrimSpace(out.Choices[0].Message.Content)
	var verdict openAIVerdict
	if err := json.Unmarshal([]byte(stripCodeFence(content)), &verdict); err != nil {
		msg := fmt.Sprintf("réponse OpenAI non structurée (corrélation=%s) : %s", correlation, excerptStr(content, 300))
		r.logf(missionID, "error", msg)
		return ReviewOutput{}, errors.New(msg)
	}
	switch verdict.Decision {
	case VerdictAccepted, VerdictCorrection, VerdictBlocked:
	default:
		msg := fmt.Sprintf("verdict OpenAI inconnu (corrélation=%s) : %s", correlation, verdict.Decision)
		r.logf(missionID, "error", msg)
		return ReviewOutput{}, errors.New(msg)
	}
	res := ReviewOutput{Decision: verdict.Decision, Summary: strings.TrimSpace(verdict.Summary)}
	for _, c := range verdict.Comments {
		if strings.TrimSpace(c.Ref) != "" || strings.TrimSpace(c.Note) != "" {
			res.Comments = append(res.Comments, ReviewComment{Ref: c.Ref, Note: c.Note})
		}
	}
	r.logf(missionID, "response", fmt.Sprintf("statut=200 corrélation=%s tokens=%d décision=%s réponse=%s",
		correlation, out.Usage.TotalTokens, verdict.Decision, excerptStr(content, 300)))
	_ = started
	return res, nil
}

// stripCodeFence enlève une clôture markdown éventuelle autour du JSON.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i > 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

// excerpt borne un texte brut pour le journal (aucun secret n'y figure : le
// corps de réponse OpenAI ne contient jamais la clé).
func excerpt(b []byte, n int) string { return excerptStr(string(b), n) }

func excerptStr(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	if s == "" {
		return "(vide)"
	}
	return s
}

// logf journalise via la fonction branchée, sans effet si absente.
func (r *RealOpenAI) logf(missionID, phase, message string) {
	if r.log != nil {
		r.log(missionID, phase, message)
	}
}

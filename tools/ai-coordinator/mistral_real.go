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

// RealMistral est l'adaptateur réel de l'agent développeur Mistral (M-AIC-05).
// Il réalise MistralAdapter sur l'API HTTP chat/completions de Mistral AI.
//
// Règles de sécurité (identiques à RealOpenAI) :
//   - la clé d'API est détenue uniquement en mémoire, transmise par l'en-tête
//     Authorization, et n'est JAMAIS journalisée ni persistée ;
//   - fail closed : la construction échoue sans clé ;
//   - appels bornés (délai, plafond de jetons), aucune re-soumission
//     automatique en boucle ;
//   - chaque appel journalise phase, modèle, statut, jetons, identifiant de
//     corrélation et extrait borné ; jamais la clé ni les en-têtes privés.

// RealMistralOptions regroupe les paramètres de construction.
type RealMistralOptions struct {
	APIKey    string                                 // clé Mistral, mémoire uniquement
	APIURL    string                                 // racine COMPLÈTE (vide = https://api.mistral.ai/v1)
	Model     string                                 // modèle interrogé (défaut mistral-small-latest)
	Timeout   time.Duration                          // délai par requête
	MaxTokens int                                    // plafond de jetons de réponse (défaut 800)
	Log       func(missionID, phase, message string) // journal SQLite optionnel
}

// RealMistral réalise MistralAdapter via l'API Mistral.
type RealMistral struct {
	apiKey    string
	apiURL    string
	model     string
	client    *http.Client
	maxTokens int
	log       func(missionID, phase, message string)
}

// NewRealMistral construit l'adaptateur réel : fail closed sans clé.
func NewRealMistral(opts RealMistralOptions) (*RealMistral, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("adaptateur API Mistral : clé absente (AIC_MISTRAL_APIKEY), refus (fail closed)")
	}
	apiURL := strings.TrimRight(strings.TrimSpace(opts.APIURL), "/")
	if apiURL == "" {
		apiURL = "https://api.mistral.ai/v1"
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "mistral-small-latest"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 800
	}
	return &RealMistral{
		apiKey:    opts.APIKey,
		apiURL:    apiURL,
		model:     model,
		client:    &http.Client{Timeout: timeout},
		maxTokens: maxTokens,
		log:       opts.Log,
	}, nil
}

// mistralRequest est le corps d'appel chat/completions (format Mistral).
type mistralRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Messages  []mistralMessage `json:"messages"`
}

type mistralMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mistralResponse est la réponse chat/completions (champs exploités seulement).
type mistralResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// mistralRunPrompt est l'instruction imposée pour une exécution de mission :
// la réponse doit être un rapport structuré, jamais du texte libre.
const mistralRunPrompt = `Tu es l'agent développeur Mistral Code d'Osnias AI Coordinator.
On te confie une mission de développement. Tu ne disposes ici d'aucun
environnement d'exécution : produis UNIQUEMENT l'analyse attendue.
Réponds STRICTEMENT en JSON valide : {"report":"rapport de tranche concis"}`

// Call transmet un message à l'agent Mistral et retourne sa réponse libre.
// C'est le point d'appel unique : bornage, journalisation et contrôles.
func (r *RealMistral) Call(missionID, userMessage string) (string, error) {
	body, err := json.Marshal(mistralRequest{
		Model:     r.model,
		MaxTokens: r.maxTokens,
		Messages: []mistralMessage{
			{Role: "user", Content: userMessage},
		},
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.apiURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey) // jamais journalisé
	req.Header.Set("User-Agent", "osnias-aic-coordinator")
	r.logf(missionID, "call", fmt.Sprintf("appel API Mistral modèle=%s max_tokens=%d", r.model, r.maxTokens))
	resp, err := r.client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("échec d'appel API Mistral : %v", err)
		r.logf(missionID, "error", msg)
		return "", errors.New(msg)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	correlation := resp.Header.Get("X-Request-Id")
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("statut HTTP %d de l'API Mistral (corrélation=%s) : %s", resp.StatusCode, correlation, excerpt(raw, 300))
		r.logf(missionID, "error", msg)
		return "", errors.New(msg)
	}
	var out mistralResponse
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		msg := fmt.Sprintf("réponse Mistral illisible ou vide (corrélation=%s) : %s", correlation, excerpt(raw, 300))
		r.logf(missionID, "error", msg)
		return "", errors.New(msg)
	}
	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if content == "" {
		msg := fmt.Sprintf("réponse Mistral vide (corrélation=%s)", correlation)
		r.logf(missionID, "error", msg)
		return "", errors.New(msg)
	}
	r.logf(missionID, "response", fmt.Sprintf("statut=200 corrélation=%s tokens=%d réponse=%s",
		correlation, out.Usage.TotalTokens, excerptStr(content, 300)))
	return content, nil
}

// mistralRunReport est le rapport structuré attendu d'une exécution.
type mistralRunReport struct {
	Report string `json:"report"`
}

// Run implémente MistralAdapter : une exécution de mission correspond à une
// complétion unique produisant un rapport JSON. Aucun accès au système de
// fichiers n'est effectué : le mode api ne touche pas au répertoire de travail.
func (r *RealMistral) Run(ctx context.Context, m *Mission, workDir string) (MistralResult, error) {
	_ = ctx
	_ = workDir
	content, err := r.Call(m.ID, mistralRunPrompt+"\n\nMission "+m.ID+" : "+m.Title)
	if err != nil {
		return MistralResult{Err: err}, err
	}
	var rep mistralRunReport
	if err := json.Unmarshal([]byte(stripCodeFence(content)), &rep); err != nil || strings.TrimSpace(rep.Report) == "" {
		msg := fmt.Sprintf("rapport Mistral non structuré : %s", excerptStr(content, 300))
		r.logf(m.ID, "error", msg)
		err2 := errors.New(msg)
		return MistralResult{Err: err2}, err2
	}
	r.logf(m.ID, "run", excerptStr(rep.Report, 300))
	return MistralResult{Report: rep.Report, Diff: "(mode api : aucun diff local)", Steps: 1, Iters: 1}, nil
}

// logf journalise via la fonction branchée, sans effet si absente.
func (r *RealMistral) logf(missionID, phase, message string) {
	if r.log != nil {
		r.log(missionID, phase, message)
	}
}

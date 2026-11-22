package aic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestMistral monte un adaptateur réel pointant vers srv.URL.
func newTestMistral(t *testing.T, url string, logs *[]string) *RealMistral {
	t.Helper()
	r, err := NewRealMistral(RealMistralOptions{
		APIKey:  "msk-test-never-log",
		APIURL:  url,
		Model:   "mistral-small-test",
		Timeout: 5 * time.Second,
		Log: func(missionID, phase, message string) {
			*logs = append(*logs, phase+": "+message)
		},
	})
	if err != nil {
		t.Fatalf("NewRealMistral : %v", err)
	}
	return r
}

// mistralJSON enveloppe un contenu dans une réponse chat/completions Mistral.
func mistralJSON(content string) string {
	body, _ := json.Marshal(map[string]any{
		"id": "cmpl-test",
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
		"usage": map[string]int{"total_tokens": 21},
	})
	return string(body)
}

func TestNewRealMistralFailClosedSansClé(t *testing.T) {
	if _, err := NewRealMistral(RealMistralOptions{}); err == nil {
		t.Fatal("la construction sans clé doit échouer (fail closed)")
	}
}

func TestRealMistralCallSuccès(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("chemin inattendu : %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer msk-test-never-log" {
			t.Errorf("en-tête Authorization inattendu : %q", got)
		}
		var req mistralRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("corps illisible : %v", err)
		}
		if req.Model != "mistral-small-test" || len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("requête inattendue : %+v", req)
		}
		w.Header().Set("X-Request-Id", "req-mistral-7")
		_, _ = w.Write([]byte(mistralJSON("réception confirmée par Mistral")))
	}))
	defer srv.Close()
	r := newTestMistral(t, srv.URL, &logs)
	out, err := r.Call("aic-05", "message de test")
	if err != nil {
		t.Fatalf("Call : %v", err)
	}
	if out != "réception confirmée par Mistral" {
		t.Errorf("réponse = %q", out)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "call") || !strings.Contains(joined, "response") {
		t.Errorf("journal attendu call+response : %v", logs)
	}
	if !strings.Contains(joined, "req-mistral-7") {
		t.Errorf("identifiant de corrélation absent du journal : %v", logs)
	}
	if strings.Contains(joined, "msk-test-never-log") {
		t.Errorf("la clé d'API est journalisée : %v", logs)
	}
}

func TestRealMistralCallStatutNon200(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-mistral-err")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"clé invalide"}}`))
	}))
	defer srv.Close()
	r := newTestMistral(t, srv.URL, &logs)
	if _, err := r.Call("m1", "msg"); err == nil || !strings.Contains(err.Error(), "statut HTTP 401") {
		t.Fatalf("erreur attendue avec statut 401 : %v", err)
	}
	if strings.Contains(strings.Join(logs, "\n"), "msk-test-never-log") {
		t.Errorf("la clé d'API est journalisée : %v", logs)
	}
}

func TestRealMistralRunSansRapportStructuré(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mistralJSON("je ne réponds pas en JSON")))
	}))
	defer srv.Close()
	r := newTestMistral(t, srv.URL, &logs)
	m := &Mission{ID: "M-RUN", Title: "mission de test"}
	if _, err := r.Run(nil, m, ""); err == nil || !strings.Contains(err.Error(), "non structuré") {
		t.Fatalf("erreur rapport non structuré attendue : %v", err)
	}
}

func TestRealMistralRunRapportJSON(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mistralRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, "M-RUN") || !strings.Contains(req.Messages[0].Content, "mission de test") {
			t.Errorf("le prompt doit contenir la mission : %q", req.Messages[0].Content)
		}
		inner, _ := json.Marshal(map[string]string{"report": "analyse produite pour la mission"})
		_, _ = w.Write([]byte(mistralJSON("```json\n" + string(inner) + "\n```")))
	}))
	defer srv.Close()
	r := newTestMistral(t, srv.URL, &logs)
	m := &Mission{ID: "M-RUN", Title: "mission de test"}
	res, err := r.Run(nil, m, "")
	if err != nil {
		t.Fatalf("Run : %v", err)
	}
	if res.Report != "analyse produite pour la mission" || res.Err != nil {
		t.Errorf("résultat = %+v", res)
	}
}

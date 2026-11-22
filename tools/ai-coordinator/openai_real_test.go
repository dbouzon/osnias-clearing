package aic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestOpenAI monte un adaptateur réel pointant vers srv.URL.
func newTestOpenAI(t *testing.T, url string, logs *[]string) *RealOpenAI {
	t.Helper()
	r, err := NewRealOpenAI(RealOpenAIOptions{
		APIKey: "sk-test-never-log",
		APIURL: url,
		Model:  "gpt-4o-mini-test",
		//nolint:staticcheck // durée volontairement extrême pour être acceptée par go vet
		Timeout: 5 * time.Second,
		Log: func(missionID, phase, message string) {
			*logs = append(*logs, phase+": "+message)
		},
	})
	if err != nil {
		t.Fatalf("NewRealOpenAI : %v", err)
	}
	return r
}

// openAIJSON enveloppe un verdict dans une réponse chat/completions.
func openAIJSON(decision, summary string) string {
	content, _ := json.Marshal(map[string]any{
		"decision": decision,
		"summary":  summary,
		"comments": []map[string]string{{"ref": "main.go", "note": "conforme"}},
	})
	body, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-test",
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": string(content)}},
		},
		"usage": map[string]int{"total_tokens": 42},
	})
	return string(body)
}

func TestNewRealOpenAIFailClosedSansClé(t *testing.T) {
	if _, err := NewRealOpenAI(RealOpenAIOptions{}); err == nil {
		t.Fatal("la construction sans clé doit échouer (fail closed)")
	}
}

func TestRealOpenAICallSuccès(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("chemin inattendu : %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test-never-log" {
			t.Errorf("en-tête Authorization inattendu : %q", got)
		}
		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("corps illisible : %v", err)
		}
		if req.Model != "gpt-4o-mini-test" {
			t.Errorf("modèle inattendu : %s", req.Model)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Errorf("messages inattendus : %+v", req.Messages)
		}
		w.Header().Set("X-Request-Id", "req-test-123")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openAIJSON(VerdictAccepted, "réception confirmée, dialogue possible")))
	}))
	defer srv.Close()
	r := newTestOpenAI(t, srv.URL, &logs)
	out, err := r.Call("aic-04-ping", "premier message de test")
	if err != nil {
		t.Fatalf("Call : %v", err)
	}
	if out.Decision != VerdictAccepted {
		t.Errorf("décision = %q", out.Decision)
	}
	if out.Summary != "réception confirmée, dialogue possible" {
		t.Errorf("résumé = %q", out.Summary)
	}
	if len(out.Comments) != 1 || out.Comments[0].Ref != "main.go" {
		t.Errorf("commentaires = %+v", out.Comments)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "call") || !strings.Contains(joined, "response") {
		t.Errorf("journal attendu call+response : %v", logs)
	}
	if !strings.Contains(joined, "req-test-123") {
		t.Errorf("identifiant de corrélation absent du journal : %v", logs)
	}
	if strings.Contains(joined, "sk-test-never-log") {
		t.Errorf("la clé d'API est journalisée : %v", logs)
	}
}

func TestRealOpenAICallStatutNon200(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-err-1")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"clé invalide"}}`))
	}))
	defer srv.Close()
	r := newTestOpenAI(t, srv.URL, &logs)
	_, err := r.Call("m1", "msg")
	if err == nil || !strings.Contains(err.Error(), "statut HTTP 401") {
		t.Fatalf("erreur attendue avec statut 401 : %v", err)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "req-err-1") || !strings.Contains(joined, "error") {
		t.Errorf("journal d'erreur attendu : %v", logs)
	}
	if strings.Contains(joined, "sk-test-never-log") {
		t.Errorf("la clé d'API est journalisée : %v", logs)
	}
}

func TestRealOpenAICallRéponseNonStructurée(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"bonjour, ceci n'est pas du JSON"}}]}`))
	}))
	defer srv.Close()
	r := newTestOpenAI(t, srv.URL, &logs)
	if _, err := r.Call("m1", "msg"); err == nil {
		t.Fatal("une réponse non structurée doit produire une erreur")
	} else if !strings.Contains(err.Error(), "non structurée") {
		t.Errorf("message d'erreur inattendu : %v", err)
	}
}

func TestRealOpenAICallVerdictInconnu(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"decision\":\"PEUT-ETRE\"}"}}]}`))
	}))
	defer srv.Close()
	r := newTestOpenAI(t, srv.URL, &logs)
	if _, err := r.Call("m1", "msg"); err == nil || !strings.Contains(err.Error(), "verdict OpenAI inconnu") {
		t.Fatalf("erreur verdict inconnu attendue : %v", err)
	}
}

func TestRealOpenAICallClôtureMarkdown(t *testing.T) {
	var logs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner, _ := json.Marshal(map[string]any{
			"decision": VerdictCorrection,
			"summary":  "correction demandée",
		})
		fenced := "```json\n" + string(inner) + "\n```"
		outer, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": fenced}}},
		})
		_, _ = w.Write(outer)
	}))
	defer srv.Close()
	r := newTestOpenAI(t, srv.URL, &logs)
	out, err := r.Call("m1", "msg")
	if err != nil {
		t.Fatalf("la clôture markdown doit être tolérée : %v", err)
	}
	if out.Decision != VerdictCorrection {
		t.Errorf("décision = %q", out.Decision)
	}
}

func TestRealOpenAIReviewConstruitLePrompt(t *testing.T) {
	var_body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		var_body = req.Messages[1].Content
		_, _ = w.Write([]byte(openAIJSON(VerdictAccepted, "ok")))
	}))
	defer srv.Close()
	var logs []string
	r := newTestOpenAI(t, srv.URL, &logs)
	out, err := r.Review(ReviewInput{MissionID: "M-01", Report: "rapport de recette", Diff: "diff technique"})
	if err != nil {
		t.Fatalf("Review : %v", err)
	}
	if out.Decision != VerdictAccepted {
		t.Errorf("décision = %q", out.Decision)
	}
	if !strings.Contains(var_body, "M-01") || !strings.Contains(var_body, "rapport de recette") || !strings.Contains(var_body, "diff technique") {
		t.Errorf("le prompt doit contenir la mission et le rapport : %q", var_body)
	}
}

package aic

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRelay monte un relais complet sur des serveurs Mistral et OpenAI
// simulés (httptest), avec base temporaire.
func newTestRelay(t *testing.T, mistralSrv, openAISrv string) (*Relay, *Store) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "relay.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore : %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var mlogs []string
	mistral, err := NewRealMistral(RealMistralOptions{APIKey: "msk-test", APIURL: mistralSrv,
		Log: func(m, p, msg string) {
			mlogs = append(mlogs, msg)
			_ = store.Log(m, "mistral", p, msg)
		}})
	if err != nil {
		t.Fatalf("NewRealMistral : %v", err)
	}
	var ologs []string
	openai, err := NewRealOpenAI(RealOpenAIOptions{APIKey: "sk-test", APIURL: openAISrv,
		Log: func(m, p, msg string) {
			ologs = append(ologs, msg)
			_ = store.Log(m, "openai", p, msg)
		}})
	if err != nil {
		t.Fatalf("NewRealOpenAI : %v", err)
	}
	relay, err := NewRelay(store, mistral, openai, "token-relu")
	if err != nil {
		t.Fatalf("NewRelay : %v", err)
	}
	return relay, store
}

// fakeUpstreams monte deux API simulées qui enregistrent les messages reçus.
func fakeUpstreams(t *testing.T, mistralGot, openAIGot *string) (string, string) {
	t.Helper()
	mSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mistralRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		*mistralGot = req.Messages[len(req.Messages)-1].Content
		_, _ = w.Write([]byte(mistralJSON("réponse de l'agent Mistral")))
	}))
	oSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		*openAIGot = req.Messages[len(req.Messages)-1].Content
		_, _ = w.Write([]byte(openAIJSON(VerdictAccepted, "réception confirmée par OpenAI")))
	}))
	t.Cleanup(mSrv.Close)
	t.Cleanup(oSrv.Close)
	return mSrv.URL, oSrv.URL
}

func TestNewRelayFailClosed(t *testing.T) {
	store, _ := OpenStore(filepath.Join(t.TempDir(), "r.sqlite"))
	defer store.Close()
	mistral, _ := NewRealMistral(RealMistralOptions{APIKey: "k"})
	openai, _ := NewRealOpenAI(RealOpenAIOptions{APIKey: "k"})
	if _, err := NewRelay(store, mistral, openai, ""); err == nil {
		t.Fatal("sans jeton, le relais doit refuser la construction (fail closed)")
	}
	if _, err := NewRelay(store, nil, openai, "tok"); err == nil {
		t.Fatal("sans adaptateur Mistral, le relais doit refuser la construction")
	}
	if _, err := NewRelay(store, mistral, nil, "tok"); err == nil {
		t.Fatal("sans adaptateur OpenAI, le relais doit refuser la construction")
	}
}

func TestRelayCycleComplet(t *testing.T) {
	var mistralGot, openAIGot string
	mURL, oURL := fakeUpstreams(t, &mistralGot, &openAIGot)
	relay, store := newTestRelay(t, mURL, oURL)
	resp, err := relay.Relay(RelayRequest{
		MissionID: "aic-05",
		MessageID: "msg-001",
		From:      "openai",
		Body:      "premier message du cycle",
	}, store.Log)
	if err != nil {
		t.Fatalf("Relay : %v", err)
	}
	if resp.Status != "ok" || resp.MissionID != "aic-05" || resp.MessageID != "msg-001" {
		t.Errorf("réponse = %+v", resp)
	}
	if resp.Mistral != "réponse de l'agent Mistral" {
		t.Errorf("réponse Mistral = %q", resp.Mistral)
	}
	if resp.OpenAI == nil || resp.OpenAI.Decision != VerdictAccepted || resp.OpenAI.Summary != "réception confirmée par OpenAI" {
		t.Errorf("verdict OpenAI = %+v", resp.OpenAI)
	}
	if !strings.Contains(mistralGot, "premier message du cycle") {
		t.Errorf("Mistral doit recevoir le message : %q", mistralGot)
	}
	if !strings.Contains(openAIGot, "premier message du cycle") || !strings.Contains(openAIGot, "réponse de l'agent Mistral") {
		t.Errorf("OpenAI doit recevoir le message ET la réponse Mistral : %q", openAIGot)
	}
	// Journal SQLite : toutes les étapes sous la mission.
	rows, err := store.db.Query(`SELECT agent, phase FROM agent_logs WHERE mission_id='aic-05' ORDER BY id`)
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	defer rows.Close()
	var seen []string
	for rows.Next() {
		var a, p string
		_ = rows.Scan(&a, &p)
		seen = append(seen, a+"/"+p)
	}
	joined := strings.Join(seen, " ")
	for _, want := range []string{"relay/receive", "mistral/call", "mistral/response", "openai/call", "openai/response", "relay/openai"} {
		if !strings.Contains(joined, want) {
			t.Errorf("journal manquant %q : %v", want, seen)
		}
	}
}

func TestRelayIdempotenceParMessageID(t *testing.T) {
	var mistralGot, openAIGot string
	mURL, oURL := fakeUpstreams(t, &mistralGot, &openAIGot)
	relay, store := newTestRelay(t, mURL, oURL)
	req := RelayRequest{MissionID: "aic-05", MessageID: "msg-dup", From: "humain", Body: "message unique"}
	if _, err := relay.Relay(req, store.Log); err != nil {
		t.Fatalf("premier passage : %v", err)
	}
	resp, err := relay.Relay(req, store.Log)
	if err == nil || resp == nil || resp.Status != "duplicate" {
		t.Fatalf("le doublon doit être signalé : resp=%+v err=%v", resp, err)
	}
	if mistralGot == "" || openAIGot == "" {
		t.Fatal("état incohérent du test")
	}
	// Second appel : aucun nouveau passage d'API ne doit être effectué.
	before := mistralGot
	_, err = relay.Relay(RelayRequest{MissionID: "aic-05", MessageID: "msg-dup", From: "humain", Body: "AUTRE message"}, store.Log)
	if err == nil {
		t.Fatal("le doublon doit être une erreur d'idempotence")
	}
	if mistralGot != before {
		t.Errorf("Mistral ne doit pas être rappelé sur doublon : %q", mistralGot)
	}
}

func TestRelayChampsRequis(t *testing.T) {
	mURL, oURL := fakeUpstreams(t, new(string), new(string))
	relay, store := newTestRelay(t, mURL, oURL)
	if _, err := relay.Relay(RelayRequest{MissionID: "m", MessageID: "x", Body: ""}, store.Log); err == nil {
		t.Fatal("un corps vide doit être refusé")
	}
	if _, err := relay.Relay(RelayRequest{MissionID: "m", MessageID: "x", From: "intrus", Body: "b"}, store.Log); err == nil {
		t.Fatal("un émetteur inconnu doit être refusé")
	}
}

func TestRelayHTTPRefusParDéfaut(t *testing.T) {
	t.Setenv("AIC_WEBHOOK_SECRET", "s")
	cfg := ConfigFromEnv()
	cfg.ListenAddr = "127.0.0.1:0"
	coord := &Coordinator{Config: cfg}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// relais désactivé : la route répond 501, jamais 200 sans jeton.
	srv := NewServer(coord, logger)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("écoute locale impossible : %v", err)
	}
	defer srv.Close()
	go func() { _ = srv.Serve(ln) }()
	addr := ln.Addr().String()
	resp, err := http.Post("http://"+addr+"/aic/message", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("requête : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("sans relais construit, /aic/message doit répondre 501, reçu %d", resp.StatusCode)
	}
}

func TestRelayHTTPAuthentification(t *testing.T) {
	var mistralGot, openAIGot string
	mURL, oURL := fakeUpstreams(t, &mistralGot, &openAIGot)
	relay, _ := newTestRelay(t, mURL, oURL)
	cfg := ConfigFromEnv()
	cfg.ListenAddr = "127.0.0.1:0"
	coord := &Coordinator{Config: cfg, Store: relay.Store}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(coord, logger, relay)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("écoute locale impossible : %v", err)
	}
	addr := ln.Addr().String()
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	body, _ := json.Marshal(RelayRequest{MissionID: "aic-05", MessageID: "http-1", From: "humain", Body: "message http"})

	// sans jeton : 401
	resp, err := http.Post("http://"+addr+"/aic/message", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("requête sans jeton : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sans jeton : attendu 401, reçu %d", resp.StatusCode)
	}

	// mauvaise méthode : 405
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/aic/message", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requête GET : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET : attendu 405, reçu %d", resp.StatusCode)
	}

	// jeton valide : cycle complet en HTTP.
	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/aic/message", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-relu")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requête authentifiée : %v", err)
	}
	defer resp.Body.Close()
	var out RelayResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK || out.Status != "ok" || out.OpenAI == nil {
		t.Errorf("cycle HTTP : statut=%d resp=%+v", resp.StatusCode, out)
	}
	if !strings.Contains(mistralGot, "message http") {
		t.Errorf("Mistral doit recevoir le message HTTP : %q", mistralGot)
	}

	// rejeu du même message_id : réponse idempotente sans nouvel appel.
	beforeMistral := mistralGot
	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/aic/message", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-relu")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rejeu : %v", err)
	}
	var dup RelayResponse
	_ = json.NewDecoder(resp.Body).Decode(&dup)
	resp.Body.Close()
	if dup.Status != "duplicate" {
		t.Errorf("rejeu : attendu duplicate, reçu %+v", dup)
	}
	if mistralGot != beforeMistral {
		t.Errorf("aucun nouvel appel Mistral attendu sur rejeu : %q", mistralGot)
	}
}

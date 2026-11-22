package main

import (
	"bytes"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

func TestBuildPayloadMinimal(t *testing.T) {
	raw, err := buildPayload(900, "dbouzon", "/aic start", "titre")
	if err != nil {
		t.Fatalf("payload : %v", err)
	}
	for _, want := range []string{`"number":900`, `"login":"dbouzon"`, `"body":"/aic start"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("champ manquant %s dans %s", want, raw)
		}
	}
}

func TestSignHexKnownVector(t *testing.T) {
	// vecteur RFC 4231 : HMAC-SHA256 "key"/"The quick brown fox jumps over the lazy dog"
	got := signHex("key", []byte("The quick brown fox jumps over the lazy dog"))
	want := "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"
	if got != want {
		t.Fatalf("signature inattendue : %s", got)
	}
}

func TestRunRejectsNonLoopback(t *testing.T) {
	if err := run("0.0.0.0:8080", "s", 1, "a", "b", "", "t"); err == nil || !strings.Contains(err.Error(), "boucle locale") {
		t.Fatalf("refus attendu hors boucle locale : %v", err)
	}
	if err := run("127.0.0.1:8080", "", 1, "a", "b", "", "t"); err == nil || !strings.Contains(err.Error(), "secret requis") {
		t.Fatalf("refus attendu sans secret : %v", err)
	}
	if err := run("127.0.0.1:8080", "s", 0, "a", "b", "", "t"); err == nil || !strings.Contains(err.Error(), "numéro d'Issue") {
		t.Fatalf("refus attendu sans numéro : %v", err)
	}
}

// TestInjectEndToEndSimulated vérifie l'outil de recette de bout en bout en
// mode simulé : le coordinateur traite le webhook signé et la mission atteint
// AWAITING_APPROVAL (aucune écriture GitHub réelle, boucle locale).
func TestInjectEndToEndSimulated(t *testing.T) {
	dir := t.TempDir()
	dbp := dir + "/recette.sqlite"
	store, err := aic.OpenStore(dbp)
	if err != nil {
		t.Fatalf("store : %v", err)
	}
	defer store.Close()
	cfg := aic.ConfigFromEnv()
	cfg.DBPath = dbp
	cfg.WorkRoot = dir + "/work"
	cfg.WebhookSecret = "secret-recette"
	cfg.AuthorizedLogins = []string{"dbouzon"}
	c := &aic.Coordinator{Store: store, Config: cfg, Mistral: &aic.SimulatedMistral{}, Reviewer: &aic.SimulatedOpenAI{}, GitHub: &aic.SimulatedGitHub{}}
	var buf bytes.Buffer
	srv := aic.NewServer(c, slog.New(slog.NewTextHandler(&buf, nil)))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen : %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	if err := run(ln.Addr().String(), "secret-recette", 900, "dbouzon", "/aic start", "recette-e2e", "Recette"); err != nil {
		t.Fatalf("inject : %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m, _ := store.MissionByID("AIC-900"); m != nil && m.State == aic.StateAwaitingApproval {
			return // recette complète : mission exécutée et résultat livré
		}
		time.Sleep(20 * time.Millisecond)
	}
	m, _ := store.MissionByID("AIC-900")
	t.Fatalf("mission AIC-900 non aboutie : %+v", m)
}

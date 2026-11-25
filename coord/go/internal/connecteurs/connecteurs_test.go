// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// corpsChat : format {model, messages} observé par les serveurs factices.
type corpsChat struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// reponseChat : réponse de type chat-completions émise par les factices.
func reponseChat(content, modele string, jetons int) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": modele,
		"choices": []any{map[string]any{
			"message": map[string]any{"content": content},
		}},
		"usage": map[string]any{"total_tokens": jetons},
	})
	return b
}

// nouveauServeurFactice : serveur factice loopback (HORS RÉSEAU, 0 €).
// Le handler reçoit la requête complete et décide de la réponse.
func nouveauServeurFactice(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(s.Close)
	return s
}

func lireCorps(t *testing.T, r *http.Request) corpsChat {
	t.Helper()
	var c corpsChat
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		t.Fatalf("requête factice non JSON : %v", err)
	}
	return c
}

func configTest(endpoint string) *Config {
	return &Config{
		Mode: ModeSimule,
		Fournisseurs: []ConfigFournisseur{{
			ID: "mistral", Endpoint: endpoint, Modele: "test-modele",
			TimeoutMS: 2000, TailleMaxReponse: 65536, RetraitsMax: 1,
		}},
	}
}

func TestHTTPEndPointSuccesEtIdempotence(t *testing.T) {
	var cleRecue string
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		cleRecue = r.Header.Get("Idempotency-Key")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type manquant")
		}
		w.Write(reponseChat("LIVRABLE 1\nLIVRABLE 2\nLIVRABLE 3", "test-modele", 421))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h.Executer("mandat d'essai")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	if res.Simule {
		t.Fatalf("un connecteur réel ne déclare pas Simule=true")
	}
	if res.Jetons != 421 {
		t.Fatalf("jetons réels attendus : 421, obtenu %d", res.Jetons)
	}
	if res.Sortie == "" || !strings.Contains(res.Sortie, "LIVRABLE 1") {
		t.Fatalf("sortie inattendue : %q", res.Sortie)
	}
	if len(cleRecue) != 64 {
		t.Fatalf("Idempotency-Key sha256 attendu, obtenu %q", cleRecue)
	}
	if h.ID() != "mistral" {
		t.Fatalf("identifiant opaque modifié : %q", h.ID())
	}
}

func TestHTTPEndPointRetrait429PuisSucces(t *testing.T) {
	appels := 0
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		appels++
		if appels == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(reponseChat("ok", "test-modele", 10))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h.Executer("mandat")
	if err != nil {
		t.Fatalf("le premier retrait devait suffire : %v", err)
	}
	if appels != 2 || res.Jetons != 10 {
		t.Fatalf("appels=%d jetons=%d", appels, res.Jetons)
	}
}

func TestHTTPEndPointRetraitsEpuises(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "APPEL_ECHOUE") {
		t.Fatalf("échec attendu après retraits : %v", err)
	}
}

func TestHTTPEndPointDelaiDepasse(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write(reponseChat("trop tard", "test-modele", 10))
	})
	cfg := configTest(s.URL)
	cfg.Fournisseurs[0].TimeoutMS = 100
	cfg.Fournisseurs[0].RetraitsMax = 0
	h, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "APPEL_ECHOUE") {
		t.Fatalf("délai attendu : %v", err)
	}
}

func TestHTTPEndPointReponseTropGrande(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 10000))
	})
	cfg := configTest(s.URL)
	cfg.Fournisseurs[0].TailleMaxReponse = 512
	h, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "REPONSE_TROP_GRANDE") {
		t.Fatalf("refus de taille attendu : %v", err)
	}
}

func TestHTTPEndPointModeleDivergent(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("ok", "AUTRE-MODELE", 10))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "MODELE_DIVERGENT") {
		t.Fatalf("identité répondante non vérifiée : %v", err)
	}
}

func TestHTTPEndPointJetonsInconnus(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("ok", "test-modele", 0))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "JETONS_INCONNUS") {
		t.Fatalf("budget non garanti : %v", err)
	}
}

func TestHTTPEndPointReponseVide(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("", "test-modele", 10))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "REPONSE_VIDE") {
		t.Fatalf("réponse vide attendue : %v", err)
	}
}

func TestHTTPEndPointSortieCompromise(t *testing.T) {
	cle := "CLETEST-AZERTY1234567890"
	t.Setenv("COORD_CLE_FACTICE", cle)
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("voici la cle "+cle, "test-modele", 10))
	})
	cfg := configTest(s.URL)
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_FACTICE"
	h, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "SORTIE_COMPROMISE") {
		t.Fatalf("fuite de clé non détectée : %v", err)
	}
	if strings.Contains(err.Error(), cle) {
		t.Fatalf("l'erreur elle-même fuit la clé")
	}
}

func TestHTTPEndPointExpurgeMotifDeCle(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("cle suspecte sk-abcdefghijklmnopqrstuvwxyz1234", "test-modele", 10))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h.Executer("mandat")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	if !strings.Contains(res.Sortie, "[EXPURGE]") || strings.Contains(res.Sortie, "sk-abc") {
		t.Fatalf("expurgation attendue : %q", res.Sortie)
	}
}

func TestPortesReellesFermees(t *testing.T) {
	// Endpoint non-loopback : AUCUN construction possible sans les portes.
	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{{
		ID: "openai", Endpoint: "https://api.example.com/v1/chat/completions",
		CleEnv: "COORD_CLE_OPENAI", Modele: "m", TimeoutMS: 1000,
		TailleMaxReponse: 1024, RetraitsMax: 0,
	}}}
	if _, err := cfg.NouveauHTTPEndPoint("openai"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("mode SIMULE doit refuser un endpoint réel : %v", err)
	}

	cfg.Mode = ModeReel
	if _, err := cfg.NouveauHTTPEndPoint("openai"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("COORD_MODE_REEL absent doit refuser : %v", err)
	}

	t.Setenv("COORD_MODE_REEL", "1")
	if _, err := cfg.NouveauHTTPEndPoint("openai"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("autorisation pilote absente doit refuser : %v", err)
	}

	cfg.Autorisation = &Autorisation{
		Pilote: "Denis Bouzon", Fournisseurs: []string{"mistral"},
		PlafondCentimes: 500, Date: "2026-11-25",
	}
	if _, err := cfg.NouveauHTTPEndPoint("openai"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("un fournisseur non autorisé doit être refusé : %v", err)
	}

	t.Setenv("COORD_CLE_OPENAI", "")
	cfg.Autorisation.Fournisseurs = []string{"mistral", "openai"}
	if _, err := cfg.NouveauHTTPEndPoint("openai"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("clé d'environnement absente doit refuser : %v", err)
	}

	t.Setenv("COORD_CLE_OPENAI", "cle-de-test-locale-12345678")
	h, err := cfg.NouveauHTTPEndPoint("openai")
	if err != nil {
		t.Fatalf("portes ouvertes : la construction devait réussir SANS appel réseau : %v", err)
	}
	if h == nil {
		t.Fatal("connecteur attendu")
	}
}

func TestConfigFailClosed(t *testing.T) {
	rep := t.TempDir()
	chemin := filepath.Join(rep, "config.json")

	if err := os.WriteFile(chemin, []byte(`{"mode":"INCONNU","fournisseurs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ChargerConfig(chemin); err == nil {
		t.Fatal("mode inconnu doit être refusé")
	}

	if err := os.WriteFile(chemin, []byte(`{"mode":"SIMULE","fournisseurs":[
		{"id":"a","endpoint":"http://127.0.0.1:1/x","timeout_ms":10,"taille_max_reponse":10},
		{"id":"a","endpoint":"http://127.0.0.1:1/x","timeout_ms":10,"taille_max_reponse":10}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ChargerConfig(chemin); err == nil || !strings.Contains(err.Error(), "doublon") {
		t.Fatalf("doublon attendu : %v", err)
	}

	if err := os.WriteFile(chemin, []byte(`{"mode":"REEL","fournisseurs":[
		{"id":"a","endpoint":"https://api.example.com/x","cle_env":"X_K","timeout_ms":10,"taille_max_reponse":10}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ChargerConfig(chemin); err == nil ||
		!strings.Contains(err.Error(), "autorisation") {
		t.Fatalf("REEL sans autorisation pilote doit être refusé : %v", err)
	}
}

func TestContexteAnnule(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.Write(reponseChat("tardif", "test-modele", 10))
	})
	h, err := configTest(s.URL).NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer annuler()
	_, err = h.ExecuterContexte(ctx, "executer", "mandat", "cle-idem")
	if err == nil {
		t.Fatal("annulation attendue")
	}
}

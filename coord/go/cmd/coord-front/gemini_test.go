// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-API-GEMINI-01 : fournisseur Gemini (surface
// OpenAI-compatible), serveurs factices loopback uniquement.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// serveurGeminiFactice : surface OpenAI-compatible de Gemini —
// vérifie le chemin /chat/completions et l'en-tête Authorization
// Bearer (pas x-goog-api-key), corps/réponse chat/completions.
func serveurGeminiFactice(t *testing.T, reponses chan string) *httptest.Server {
	t.Helper()
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("chemin Gemini inattendu : %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer cle-gemini-test" {
			t.Errorf("Authorization Bearer attendu, reçu %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-goog-api-key") != "" {
			t.Errorf("x-goog-api-key ne doit pas être utilisé sur la surface OpenAI-compatible")
		}
		mu.Lock()
		appels++
		n := appels
		mu.Unlock()
		var corps struct {
			Model    string          `json:"model"`
			Messages []messageMoteur `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&corps)
		if corps.Model != "gemini-3.5-flash-lite" {
			t.Errorf("modele attendu gemini-3.5-flash-lite, reçu %q", corps.Model)
		}
		if len(corps.Messages) != 2 || corps.Messages[0].Role != "system" {
			t.Errorf("corps Gemini incomplet : %+v", corps)
		}
		texte := ""
		select {
		case texte = <-reponses:
		default:
			texte = "réponse factice gemini"
		}
		_ = n
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": texte}}},
			"usage":   map[string]any{"prompt_tokens": 13, "completion_tokens": 5},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// configGeminiTest : grille où l'étape cadrage du niveau essentiel
// passe par Gemini (Flash-Lite), fournisseur actif vers le serveur factice.
func configGeminiTest(compte compteConf, url string, plafond int) config {
	cfg := configTest(compte)
	cfg.Moteurs = moteursConf{
		Gemini: fournisseurMoteurConf{Activer: true, CleAPI: "cle-gemini-test", URLAPI: url},
		Niveaux: map[string]niveauMoteurConf{
			"essentiel": {
				Cadrage:   etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.5-flash-lite"},
				Redaction: etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.5-flash-lite"},
				Critique:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.5-flash-lite"},
				Revision:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
			},
			"avance": {
				Cadrage:   etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.5-flash-lite"},
				Redaction: etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
				Critique:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.5-flash-lite"},
				Revision:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
			},
			"premium": {
				Cadrage:   etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
				Redaction: etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
				Critique:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-3.8-flash"},
				Revision:  etapeMoteurConf{Moteur: "gemini", Modele: "gemini-4-hydrogen"},
			},
		},
		PlafondAppels: plafond,
	}
	cfg.Metrics = metricsConf{Activer: true, Token: tokenMetricsTest}
	return cfg
}

// TestGeminiCascadeEtCompteurs : appel réel factice → réponse du moteur,
// journal cohérent (jetons 13/5), compteur Orpheus moteur="gemini".
func TestGeminiCascadeEtCompteurs(t *testing.T) {
	reponses := make(chan string, 4)
	reponses <- "Quelle est la finalité attendue, précisément ?"
	srv := serveurGeminiFactice(t, reponses)
	g, srvGW := gatewayTest(t, configGeminiTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50))
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "gem1", "intention": "mission avec gemini", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/gem1/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/gem1/tour", map[string]any{"entree": "une étude"})
	if s != 200 {
		t.Fatalf("tour gemini : statut %d %v", s, m)
	}
	if m["reponse"].(map[string]any)["contenu"] != "Quelle est la finalité attendue, précisément ?" {
		t.Fatalf("contenu gemini attendu, reçu %v", m["reponse"])
	}
	if m["projet"].(map[string]any)["appels_moteurs"] != float64(1) {
		t.Fatalf("1 appel moteur attendu")
	}
	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 1 || entrees[0].Moteur != "gemini" ||
		entrees[0].JetonsEntree != 13 || entrees[0].JetonsSortie != 5 {
		t.Fatalf("journal gemini incohérent : %+v", entrees)
	}
	_, corps := obtenirMetrics(t, srvGW.URL, tokenMetricsTest)
	if !strings.Contains(corps, `moteur="gemini"`) {
		t.Fatalf("compteur Orpheus moteur=gemini attendu dans l'exposition")
	}
}

// TestGeminiFailClosed : activer sans clé réelle → refus validation ;
// fournisseur désactivé dans un itinéraire → 503 MOTEUR_DESACTIVE
// (compté refuse), aucun appel réseau.
func TestGeminiFailClosed(t *testing.T) {
	cfg := configGeminiTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
		"http://127.0.0.1:1", 50)
	cfg.Moteurs.Gemini = fournisseurMoteurConf{Activer: true, CleAPI: cleMoteurNonRenseignee}
	if err := validerMoteurs(cfg.Moteurs); err == nil {
		t.Fatalf("gemini.activer sans clé réelle doit être refusé")
	}
	// Fournisseur gemini inactif (pas de repli) mais un autre actif :
	// pas de repli script global → 503 + compteur refuse.
	cfg.Moteurs.Gemini = fournisseurMoteurConf{}
	cfg.Moteurs.Anthropic = fournisseurMoteurConf{
		Activer: true, CleAPI: "cle-anthropic-hors-itineraire", URLAPI: "http://127.0.0.1:1"}
	if err := validerMoteurs(cfg.Moteurs); err != nil {
		t.Fatalf("itinéraire gemini valide refusé à tort : %v", err)
	}
	_, srv := gatewayTest(t, cfg)
	c := connecter(t, srv, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "gem2", "intention": "fail closed", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/gem2/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/gem2/tour", map[string]any{"entree": "go"})
	if s != 503 || m["code"] != "MOTEUR_DESACTIVE" {
		t.Fatalf("attendu 503 MOTEUR_DESACTIVE, reçu %d %v", s, m)
	}
	_, corps := obtenirMetrics(t, srv.URL, tokenMetricsTest)
	if !strings.Contains(corps, `moteur="gemini"`) ||
		!strings.Contains(corps, `resultat="refuse"} 1`) {
		t.Fatalf("refus gemini attendu dans les compteurs, exposition :\n%s", corps)
	}
}

// TestGeminiErreurFournisseurGenerique : 429 quota Gemini → un réessai,
// puis 502 ERREUR_MOTEUR, journal resultat=erreur, compteur erreur
// (chemin unique des autres fournisseurs, spec v0.2 §3.3).
func TestGeminiErreurFournisseurGenerique(t *testing.T) {
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		appels++
		mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"code": 429, "message": "RESOURCE_EXHAUSTED"}}`))
	}))
	t.Cleanup(srv.Close)
	cfg := configGeminiTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50)
	g, srvGW := gatewayTest(t, cfg)
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "gem3", "intention": "quota", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/gem3/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/gem3/tour", map[string]any{"entree": "go"})
	if s != 502 || m["code"] != "ERREUR_MOTEUR" {
		t.Fatalf("attendu 502 ERREUR_MOTEUR, reçu %d %v", s, m)
	}
	if appels != 2 {
		t.Fatalf("2 tentatives attendues (essai unique), reçu %d", appels)
	}
	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 1 || entrees[0].Resultat != "erreur" ||
		!strings.Contains(entrees[0].Erreur, "429") {
		t.Fatalf("journal : erreur fournisseur gemini attendue, reçu %+v", entrees)
	}
	raw, _ := os.ReadFile(g.cheminJournal)
	if strings.Contains(string(raw), "cle-gemini-test") {
		t.Fatalf("la clé ne doit jamais figurer dans le journal")
	}
	_, corps := obtenirMetrics(t, srvGW.URL, tokenMetricsTest)
	if !strings.Contains(corps, `resultat="erreur"} 1`) {
		t.Fatalf("compteur erreur gemini attendu, exposition :\n%s", corps)
	}
}

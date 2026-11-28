// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-API-DEEPSEEK-01 : fournisseur DeepSeek (API
// nativement OpenAI-compatible), serveurs factices loopback uniquement.
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

// serveurDeepseekFactice : surface OpenAI-compatible de DeepSeek —
// vérifie le chemin /v1/chat/completions et l'en-tête Authorization
// Bearer, corps/réponse chat/completions.
func serveurDeepseekFactice(t *testing.T, reponses chan string) *httptest.Server {
	t.Helper()
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("chemin DeepSeek inattendu : %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			t.Errorf("chemin DeepSeek sans préfixe /v1 : %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer cle-deepseek-test" {
			t.Errorf("Authorization Bearer attendu, reçu %q", r.Header.Get("Authorization"))
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
		if corps.Model != "deepseek-v4.1-flash" {
			t.Errorf("modèle attendu deepseek-v4.1-flash, reçu %q", corps.Model)
		}
		if len(corps.Messages) != 2 || corps.Messages[0].Role != "system" {
			t.Errorf("corps DeepSeek incomplet : %+v", corps)
		}
		texte := ""
		select {
		case texte = <-reponses:
		default:
			texte = "réponse factice deepseek"
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

// configDeepseekTest : grille entière sur DeepSeek (V4.1 Flash),
// fournisseur actif vers le serveur factice.
func configDeepseekTest(compte compteConf, url string, plafond int) config {
	cfg := configTest(compte)
	cfg.Moteurs = moteursConf{
		Deepseek: fournisseurMoteurConf{Activer: true, CleAPI: "cle-deepseek-test", URLAPI: url},
		Niveaux: map[string]niveauMoteurConf{
			"essentiel": {
				Cadrage:   etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Redaction: etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Critique:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Revision:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
			},
			"avance": {
				Cadrage:   etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Redaction: etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Critique:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Revision:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
			},
			"premium": {
				Cadrage:   etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Redaction: etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Critique:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
				Revision:  etapeMoteurConf{Moteur: "deepseek", Modele: "deepseek-v4.1-flash"},
			},
		},
		PlafondAppels: plafond,
	}
	cfg.Metrics = metricsConf{Activer: true, Token: tokenMetricsTest}
	return cfg
}

// TestDeepseekCascadeEtCompteurs : appel réel factice → réponse du
// moteur, journal cohérent (jetons 13/5), compteur Orpheus moteur="deepseek".
func TestDeepseekCascadeEtCompteurs(t *testing.T) {
	reponses := make(chan string, 4)
	reponses <- "Quelle est la finalité attendue, précisément ?"
	srv := serveurDeepseekFactice(t, reponses)
	g, srvGW := gatewayTest(t, configDeepseekTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50))
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "ds1", "intention": "mission avance deepseek", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/ds1/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/ds1/tour", map[string]any{"entree": "une étude"})
	if s != 200 {
		t.Fatalf("tour deepseek : statut %d %v", s, m)
	}
	if m["reponse"].(map[string]any)["contenu"] != "Quelle est la finalité attendue, précisément ?" {
		t.Fatalf("contenu deepseek attendu, reçu %v", m["reponse"])
	}
	if m["projet"].(map[string]any)["appels_moteurs"] != float64(1) {
		t.Fatalf("1 appel moteur attendu")
	}
	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 1 || entrees[0].Moteur != "deepseek" ||
		entrees[0].JetonsEntree != 13 || entrees[0].JetonsSortie != 5 {
		t.Fatalf("journal deepseek incohérent : %+v", entrees)
	}
	_, corps := obtenirMetrics(t, srvGW.URL, tokenMetricsTest)
	if !strings.Contains(corps, `moteur="deepseek"`) {
		t.Fatalf("compteur Orpheus moteur=deepseek attendu dans l'exposition")
	}
}

// TestDeepseekFailClosed : activer sans clé réelle → refus validation ;
// fournisseur désactivé dans un itinéraire (autre actif) → 503
// MOTEUR_DESACTIVE (compté refuse), aucun appel réseau.
func TestDeepseekFailClosed(t *testing.T) {
	cfg := configDeepseekTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
		"http://127.0.0.1:1", 50)
	cfg.Moteurs.Deepseek = fournisseurMoteurConf{Activer: true, CleAPI: cleMoteurNonRenseignee}
	if err := validerMoteurs(cfg.Moteurs); err == nil {
		t.Fatalf("deepseek.activer sans clé réelle doit être refusé")
	}
	// Fournisseur deepseek inactif (pas de repli) mais un autre actif :
	// pas de repli script global → 503 + compteur refuse.
	cfg.Moteurs.Deepseek = fournisseurMoteurConf{}
	cfg.Moteurs.Anthropic = fournisseurMoteurConf{
		Activer: true, CleAPI: "cle-anthropic-hors-itineraire", URLAPI: "http://127.0.0.1:1"}
	if err := validerMoteurs(cfg.Moteurs); err != nil {
		t.Fatalf("itinéraire deepseek valide à tort : %v", err)
	}
	_, srv := gatewayTest(t, cfg)
	c := connecter(t, srv, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "ds2", "intention": "fail closed", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/ds2/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/ds2/tour", map[string]any{"entree": "go"})
	if s != 503 || m["code"] != "MOTEUR_DESACTIVE" {
		t.Fatalf("attendu 503 MOTEUR_DESACTIVE, reçu %d %v", s, m)
	}
	_, corps := obtenirMetrics(t, srv.URL, tokenMetricsTest)
	if !strings.Contains(corps, `moteur="deepseek"`) ||
		!strings.Contains(corps, `resultat="refuse"} 1`) {
		t.Fatalf("refus deepseek attendu dans les compteurs, exposition :\n%s", corps)
	}
}

// TestDeepseekErreurFournisseurGenerique : 429 quota DeepSeek → un
// réessai, puis 502 ERREUR_MOTEUR, journal resultat=erreur, compteur
// erreur (chemin unique des autres fournisseurs, spec v0.2 §3.3).
func TestDeepseekErreurFournisseurGenerique(t *testing.T) {
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		appels++
		mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota exhausted"}}`))
	}))
	t.Cleanup(srv.Close)
	cfg := configDeepseekTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50)
	g, srvGW := gatewayTest(t, cfg)
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "ds3", "intention": "quota", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/ds3/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/ds3/tour", map[string]any{"entree": "go"})
	if s != 502 || m["code"] != "ERREUR_MOTEUR" {
		t.Fatalf("attendu 502 ERREUR_MOTEUR, reçu %d %v", s, m)
	}
	if appels != 2 {
		t.Fatalf("2 tentatives attendues (essai unique), reçu %d", appels)
	}
	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 1 || entrees[0].Resultat != "erreur" ||
		!strings.Contains(entrees[0].Erreur, "429") {
		t.Fatalf("journal : erreur fournisseur deepseek attendue, reçu %+v", entrees)
	}
	raw, _ := os.ReadFile(g.cheminJournal)
	if strings.Contains(string(raw), "cle-deepseek-test") {
		t.Fatalf("la clé ne doit jamais figurer dans le journal")
	}
	_, corps := obtenirMetrics(t, srvGW.URL, tokenMetricsTest)
	if !strings.Contains(corps, `resultat="erreur"} 1`) {
		t.Fatalf("compteur erreur deepseek attendu, exposition :\n%s", corps)
	}
}

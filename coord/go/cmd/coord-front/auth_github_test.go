// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-SRC-01 : homogénéité de source vérifiée serveur
// (409 SOURCE_HETEROGENE), validation des noms de mission, état
// GitHub fail-closed. Aucun appel réseau réel : GitHub n'est jamais
// contacté dans ces tests.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func configSRCTest() config {
	return config{
		SecretSession: "secret-de-test-0123456789",
		Comptes:       []compteConf{{Nom: "demo", Demo: true}},
		Google: googleConf{
			Activer:       true,
			ClientID:      "cid",
			ClientSecret:  "cs",
			CompteNouveau: "demo",
		},
		GitHub: githubConf{Activer: true, ClientID: "ghid", ClientSecret: "ghs"},
	}
}

// TestSourceHeterogene : une mission Drive qui tente de passer sur
// GitHub (ou l'inverse) est refusée AVANT toute écriture (spec §3.4).
func TestSourceHeterogene(t *testing.T) {
	g := newGateway(configSRCTest(), ".", t.TempDir())
	u, err := g.provisionner("sub-src", "src@example.fr", "S", "img")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	g.poserCookieSession(w, u.Compte, u.Sub)
	cookie := w.Result().Header.Get("Set-Cookie")

	srv := httptest.NewServer(g.routes())
	defer srv.Close()
	post := func(corps string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/missions/repertoire",
			strings.NewReader(corps))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		rep, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return rep.StatusCode
	}
	// Mission créée côté Drive.
	if c := post(`{"mission":"m1","source":"drive","dossier_id":"D1","dossier_nom":"Mission 1"}`); c != http.StatusOK {
		t.Fatalf("association drive refusée : %d", c)
	}
	// Changement de source : refus 409, AUCUNE écriture.
	if c := post(`{"mission":"m1","source":"github","dossier_id":"gh/repo/missions/m1","dossier_nom":"m1"}`); c != http.StatusConflict {
		t.Fatalf("hétérogénéité acceptée : %d", c)
	}
	// Le dossier initial reste intact (pas d'écrasement).
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/missions/repertoire?mission=m1", nil)
	req.Header.Set("Cookie", cookie)
	rep, _ := srv.Client().Do(req)
	if !strings.Contains(rep2corps(rep), `"source":"drive"`) {
		t.Fatalf("association drive altérée par la tentative github")
	}
	// Même source, autre appel : accepté (idempotent).
	if c := post(`{"mission":"m1","source":"drive","dossier_id":"D1","dossier_nom":"Mission 1"}`); c != http.StatusOK {
		t.Fatalf("ré-association drive refusée : %d", c)
	}
	// Source inconnue : 400.
	if c := post(`{"mission":"m2","source":"usb","dossier_id":"X","dossier_nom":"X"}`); c != http.StatusBadRequest {
		t.Fatalf("source inconnue acceptée : %d", c)
	}
}

// TestNomMissionValide : caractères sûrs uniquement pour un chemin
// GitHub (spec §3.3).
func TestNomMissionValide(t *testing.T) {
	for _, ok := range []string{"etude-marche", "Mission.2026", "a_b"} {
		if !nomMissionValide(ok) {
			t.Fatalf("nom refusé à tort : %s", ok)
		}
	}
	for _, ko := range []string{"mission avec espace", "entrée", "a/b", "x;rm", ""} {
		if nomMissionValide(ko) {
			t.Fatalf("nom dangereux accepté : %q", ko)
		}
	}
}

// TestAuthGithubEtatDesactive : fail-closed sans configuration.
func TestAuthGithubEtatDesactive(t *testing.T) {
	cfg := configSRCTest()
	cfg.GitHub = githubConf{}
	g := newGateway(cfg, ".", t.TempDir())
	w := httptest.NewRecorder()
	g.authGithubEtat(w, httptest.NewRequest(http.MethodGet, "/api/auth/github/etat", nil))
	if !strings.Contains(w.Body.String(), `"activer":false`) {
		t.Fatalf("état github actif sans config : %s", w.Body.String())
	}
	w2 := httptest.NewRecorder()
	g.authGithubLogin(w2, httptest.NewRequest(http.MethodGet, "/api/auth/github/login", nil))
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("login github sans config : %d", w2.Code)
	}
}

// TestAuthGithubEtatActifNeutre : activée mais non connectée.
func TestAuthGithubEtatActifNeutre(t *testing.T) {
	g := newGateway(configSRCTest(), ".", t.TempDir())
	w := httptest.NewRecorder()
	g.authGithubEtat(w, httptest.NewRequest(http.MethodGet, "/api/auth/github/etat", nil))
	if !strings.Contains(w.Body.String(), `"activer":true`) ||
		!strings.Contains(w.Body.String(), `"github_connecte":false`) {
		t.Fatalf("état github inattendu : %s", w.Body.String())
	}
}

func rep2corps(rep *http.Response) string {
	b := make([]byte, 4096)
	n, _ := rep.Body.Read(b)
	return string(b[:n])
}

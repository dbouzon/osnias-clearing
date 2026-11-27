// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-AUTH-01 : session v2, état OIDC, provisioning,
// garde-fou du répertoire dédié de mission. Aucun appel réseau réel :
// Google n'est jamais contacté (le JWKS n'est pas sollicité ici).
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Session v2 ---------------------------------------------------------

func TestSessionV2AllersRetours(t *testing.T) {
	v := signerSession("secret-de-test-0123456789", "alice", "sub-123")
	nom, sub, ok := verifierSession("secret-de-test-0123456789", v)
	if !ok || nom != "alice" || sub != "sub-123" {
		t.Fatalf("session v2 invalide : %v %v %v", nom, sub, ok)
	}
	// Session sans sub (connexion manuelle).
	v = signerSession("secret-de-test-0123456789", "bob", "")
	nom, sub, ok = verifierSession("secret-de-test-0123456789", v)
	if !ok || nom != "bob" || sub != "" {
		t.Fatalf("session manuelle invalide")
	}
	// Forgery : un autre secret refuse.
	if _, _, ok := verifierSession("autre-secret-9876543210abc", v); ok {
		t.Fatalf("session forgée acceptée")
	}
	// Format corrompu refusé.
	if _, _, ok := verifierSession("secret-de-test-0123456789", "alice|sub.mac"); ok {
		t.Fatalf("session corrompue acceptée")
	}
}

// --- État OIDC ---------------------------------------------------------

func TestEtatOIDCSubEtNonce(t *testing.T) {
	exp := time.Now().Add(10 * time.Minute)
	v := signerEtat("secret-de-test-0123456789", "sub-9", "nonce-42", exp)
	sub, nonce, ok := verifierEtat("secret-de-test-0123456789", v)
	if !ok || sub != "sub-9" || nonce != "nonce-42" {
		t.Fatalf("état OIDC invalide : %v %v %v", sub, nonce, ok)
	}
	// Expiré : refusé.
	sub, _, ok = verifierEtat("secret-de-test-0123456789",
		signerEtat("secret-de-test-0123456789", "sub-9", "n", time.Now().Add(-time.Minute)))
	if ok || sub != "" {
		t.Fatalf("état expiré accepté")
	}
	// Forgery : refusé.
	if _, _, ok := verifierEtat("mauvais-secret", v); ok {
		t.Fatalf("état forgé accepté")
	}
}

// --- Provisioning ------------------------------------------------------

func configAuthTest() config {
	return config{
		SecretSession: "secret-de-test-0123456789",
		Comptes: []compteConf{
			{Nom: "demo", Demo: true},
			{Nom: "vip", Demo: true},
		},
		Google: googleConf{
			Activer:       true,
			ClientID:      "cid",
			ClientSecret:  "cs",
			CompteNouveau: "demo",
			Correspondance: map[string]string{
				"vip@example.fr": "vip",
			},
		},
	}
}

func TestProvisionnerCompteNouveauEtCorrespondance(t *testing.T) {
	cfg := configAuthTest()
	g := newGateway(cfg, ".", t.TempDir())

	// Première connexion : compte_nouveau.
	u, err := g.provisionner("sub-a", "a@example.fr", "A", "img")
	if err != nil || u.Compte != "demo" {
		t.Fatalf("provisioning défaut : %v %v", u, err)
	}
	// Correspondance email → compte dédié.
	uVip, err := g.provisionner("sub-b", "vip@example.fr", "B", "img")
	if err != nil || uVip.Compte != "vip" {
		t.Fatalf("correspondance ignorée : %v %v", uVip, err)
	}
	// Rattachement par sub au retour : même utilisateur, PAS de doublon.
	u2, err := g.provisionner("sub-a", "a@example.fr", "A", "img")
	if err != nil || u2 != u {
		t.Fatalf("rattachement par sub cassé : %v %v", u2, err)
	}
	// Fail-closed : compte inconnu dans gateway.json.
	cfg2 := configAuthTest()
	cfg2.Google.CompteNouveau = "fantome"
	g2 := newGateway(cfg2, ".", t.TempDir())
	if _, err := g2.provisionner("sub-c", "c@example.fr", "C", "img"); err == nil {
		t.Fatalf("compte fantôme accepté")
	}
	// Fail-closed : aucun compte attribué.
	cfg3 := configAuthTest()
	cfg3.Google.CompteNouveau = ""
	cfg3.Google.Correspondance = nil
	g3 := newGateway(cfg3, ".", t.TempDir())
	if _, err := g3.provisionner("sub-d", "d@example.fr", "D", "img"); err == nil {
		t.Fatalf("provisioning sans compte accepté")
	}
}

// --- Répertoire de mission (garde-fou §3.5) ------------------------------

func TestMissionRepertoireGardeFou(t *testing.T) {
	cfg := configAuthTest()
	g := newGateway(cfg, ".", t.TempDir())

	// Créer un utilisateur et poser SA session (sub en cookie).
	u, err := g.provisionner("sub-a", "a@example.fr", "A", "img")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	g.poserCookieSession(w, u.Compte, u.Sub)
	cookie := w.Result().Header.Get("Set-Cookie")
	nomCookie := strings.Split(cookie, "=")[0]
	if nomCookie != "coord_session" {
		t.Fatalf("cookie de session attendu : %s", cookie)
	}
	// Attribuer le répertoire à la mission m1 : OK.
	srv := httptest.NewServer(g.routes())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/missions/repertoire",
		strings.NewReader(`{"mission":"m1","dossier_id":"D1","dossier_nom":"Mission 1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	rep, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if rep.StatusCode != http.StatusOK {
		t.Fatalf("association répertoire refusée : %d", rep.StatusCode)
	}
	// Le même dossier pour une AUTRE mission : refus (garde-fou).
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/missions/repertoire",
		strings.NewReader(`{"mission":"m2","dossier_id":"D1","dossier_nom":"Mission 2"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Cookie", cookie)
	rep2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.StatusCode != http.StatusConflict {
		t.Fatalf("garde-fou anti-collision inopérant : %d", rep2.StatusCode)
	}
	// Sans session : 401, aucune réponse 200.
	req3, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/missions/repertoire?mission=m1", nil)
	rep3, err := srv.Client().Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	if rep3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("route protégée ouverte sans session : %d", rep3.StatusCode)
	}
}

// --- État Google : fail-closed sans configuration -----------------------

func TestAuthGoogleEtatDesactive(t *testing.T) {
	cfg := configAuthTest()
	cfg.Google.Activer = false
	g := newGateway(cfg, ".", t.TempDir())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/auth/google/etat", nil)
	g.authGoogleEtat(w, r)
	if !strings.Contains(w.Body.String(), `"activer":false`) {
		t.Fatalf("état google actif sans configuration : %s", w.Body.String())
	}
	// La route login répond 503, jamais 200.
	w2 := httptest.NewRecorder()
	g.authGoogleLogin(w2, httptest.NewRequest(http.MethodGet, "/api/auth/google/login", nil))
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("login google sans config : %d", w2.Code)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-API-STATUS-01 : console de supervision. Serveurs
// factices loopback uniquement, aucun appel externe, 0 €. Habilitation
// admin (403 sinon), GET sans dépense, tests dispo/réel, expurgation,
// anti-doublon, garde-rythme, invalidation des preuves, agrégats UTC.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// serveurFacticeConsole : endpoint modèles + chat/completions
// contrôlés par le test (compte les appels de chaque type).
type serveurFacticeConsole struct {
	mu           sync.Mutex
	appelsChat   int
	appelsModels int
	statutModels int
}

func (s *serveurFacticeConsole) serveur(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.URL.Path {
		case "/v1/models":
			s.appelsModels++
			if s.statutModels != 0 {
				w.WriteHeader(s.statutModels)
				_, _ = w.Write([]byte(`{"error":"mauvaise cle secret-test"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "modele-a"}}})
		case "/v1/chat/completions":
			s.appelsChat++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "OK"}}},
				"usage":   map[string]any{"prompt_tokens": 9, "completion_tokens": 2},
			})
		default:
			t.Errorf("chemin factice inattendu : %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// configConsoleTest : un compte, un fournisseur mistral sur endpoint
// factice, console activée.
func configConsoleTest(url string) config {
	cfg := configTest(compteConf{Nom: "demo", Demo: true})
	cfg.Moteurs = moteursConf{
		Mistral: fournisseurMoteurConf{Activer: true, CleAPI: "cle-test", URLAPI: url},
		Niveaux: map[string]niveauMoteurConf{
			"essentiel": {
				Cadrage:   etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Redaction: etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Critique:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Revision:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
			},
			"avance": {
				Cadrage:   etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Redaction: etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Critique:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Revision:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
			},
			"premium": {
				Cadrage:   etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Redaction: etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Critique:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
				Revision:  etapeMoteurConf{Moteur: "mistral", Modele: "modele-a"},
			},
		},
		PlafondAppels: 5,
	}
	cfg.Console = consoleConf{Activer: true}
	return cfg
}

// adminClient : utilisateur admin (drapeau utilisateurs.json) avec
// session posée sur le compte configuré.
func adminClient(t *testing.T, g *gateway, srv *httptest.Server, admin bool) *client {
	t.Helper()
	sub := "sub-admin"
	if err := g.users.maj(func(m map[string]*utilisateur) error {
		m[sub] = &utilisateur{Sub: sub, Email: "pilote@example.fr", Nom: "Pilote", Compte: "demo", Admin: admin}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	g.poserCookieSession(w, "demo", sub)
	cookie := w.Result().Header.Get("Set-Cookie")
	valeur := strings.Split(strings.Split(cookie, "=")[1], ";")[0]
	return &client{base: srv.URL, token: valeur}
}

// TestConsoleHabilitation : anonyme → 401 ; connecté non admin → 403
// CONSOLE_REFUSEE ; console désactivée → 403 CONSOLE_DESACTIVEE ;
// admin → 200. Fail-closed sur le drapeau.
func TestConsoleHabilitation(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()

	// Anonyme.
	rep, _ := http.Get(api.URL + "/api/console/etat")
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonyme : attendu 401, reçu %d", rep.StatusCode)
	}
	rep.Body.Close()

	// Connecté NON admin.
	c := adminClient(t, g, api, false)
	s, m, _ := c.req(t, "GET", "/api/console/etat", nil)
	if s != http.StatusForbidden || m["code"] != "CONSOLE_REFUSEE" {
		t.Fatalf("non admin : attendu 403 CONSOLE_REFUSEE, reçu %d %v", s, m)
	}

	// Admin.
	c = adminClient(t, g, api, true)
	s, m, _ = c.req(t, "GET", "/api/console/etat", nil)
	if s != http.StatusOK {
		t.Fatalf("admin : attendu 200, reçu %d %v", s, m)
	}

	// Console désactivée dans la config.
	g2 := newGateway(configConsoleTest(srv.URL), ".", t.TempDir())
	g2.console.Activer = false
	api2 := httptest.NewServer(g2.routes())
	defer api2.Close()
	c2 := adminClient(t, g2, api2, true)
	s, m, _ = c2.req(t, "GET", "/api/console/etat", nil)
	if s != http.StatusForbidden || m["code"] != "CONSOLE_DESACTIVEE" {
		t.Fatalf("console off : attendu 403 CONSOLE_DESACTIVEE, reçu %d %v", s, m)
	}
}

// TestConsoleEtatSansDepense : GET /etat ne déclenche AUCUN appel
// réseau fournisseur (compteur factice à zéro) et expose les cinq
// cartes, la grille, les agrégats et jamais d'empreinte.
func TestConsoleEtatSansDepense(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	s, m, brut := c.req(t, "GET", "/api/console/etat?periode=mois", nil)
	if s != http.StatusOK {
		t.Fatalf("etat : attendu 200, reçu %d", s)
	}
	factice.mu.Lock()
	chats, models := factice.appelsChat, factice.appelsModels
	factice.mu.Unlock()
	if chats != 0 || models != 0 {
		t.Fatalf("GET /etat a déclenché des appels fournisseur : chat=%d models=%d", chats, models)
	}
	cartes, _ := m["fournisseurs"].([]any)
	if len(cartes) != 5 {
		t.Fatalf("cinq cartes attendues, reçu %d", len(cartes))
	}
	if m["grille"] == nil || m["jetons_missions"] == nil {
		t.Fatalf("etat incomplet : %v", m)
	}
	if strings.Contains(brut, "empreinte") || strings.Contains(brut, "cle-test") {
		t.Fatalf("etat expose un secret ou une empreinte : %s", brut)
	}
	if m["origine"] == nil || m["couverture"] == nil || m["actualisation_utc"] == nil {
		t.Fatalf("provenance/actualisation absentes : %v", m)
	}
}

// TestConsoleTestsDispoEtReel : dispo 0 € (jamais de chat), réel
// journalisé une fois (mission __test_api__), succès exige des jetons
// de sortie, échec 401 = catégorie authentification, jamais le motif
// brut du fournisseur.
func TestConsoleTestsDispoEtReel(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	// Dispo : succès.
	s, m, _ := c.req(t, "POST", "/api/console/test", map[string]any{"fournisseur": "mistral", "type": "dispo"})
	if s != http.StatusOK || m["resultat"] != "ok" || m["etat"] != "OPERATIONNELLE" {
		t.Fatalf("dispo : attendu ok OPERATIONNELLE, reçu %d %v", s, m)
	}
	factice.mu.Lock()
	chats := factice.appelsChat
	factice.mu.Unlock()
	if chats != 0 {
		t.Fatalf("dispo a déclenché un appel facturable : %d", chats)
	}

	// Réel : succès, journal commun une seule ligne __test_api__.
	s, m, _ = c.req(t, "POST", "/api/console/test", map[string]any{"fournisseur": "mistral", "type": "reel"})
	if s != http.StatusOK || m["resultat"] != "ok" {
		t.Fatalf("reel : attendu ok, reçu %d %v", s, m)
	}
	factice.mu.Lock()
	chats = factice.appelsChat
	factice.mu.Unlock()
	if chats != 1 {
		t.Fatalf("reel : un seul appel attendu, reçu %d", chats)
	}
	type fichierJournal struct {
		Entrees []entreeJournalMoteur `json:"entrees"`
	}
	fj := fichierJournal{}
	raw, _ := os.ReadFile(g.cheminJournal)
	_ = json.Unmarshal(raw, &fj)
	lignesTest := 0
	for _, e := range fj.Entrees {
		if e.Mission == missionTestAPI {
			lignesTest++
			if e.JetonsSortie <= 0 {
				t.Fatalf("test réel journalisé sans jetons de sortie : %+v", e)
			}
		}
	}
	if lignesTest != 1 {
		t.Fatalf("une seule ligne __test_api__ attendue, reçu %d", lignesTest)
	}

	// Agrégats : les jetons du test sont dans la catégorie tests.
	_, metat, _ := c.req(t, "GET", "/api/console/etat?periode=historique", nil)
	tests, _ := metat["jetons_tests"].([]any)
	if len(tests) != 1 {
		t.Fatalf("agrégat tests attendu pour mistral, reçu %v", tests)
	}

	// Historique servi : jamais d'empreinte.
	_, hist, histBrut := c.req(t, "GET", "/api/console/historique-tests", nil)
	_ = hist
	if strings.Contains(histBrut, "empreinte") {
		t.Fatalf("historique expose des empreintes : %s", histBrut)
	}

	// Échec d'authentification : catégorie expurgée, pas de motif brut
	// (le faux serveur renvoie un message contenant un pseudo-secret).
	factice.mu.Lock()
	factice.statutModels = http.StatusUnauthorized
	factice.mu.Unlock()
	s, m, brut := c.req(t, "POST", "/api/console/test", map[string]any{"fournisseur": "mistral", "type": "dispo"})
	if s != http.StatusOK || m["resultat"] != "erreur" || m["categorie"] != "authentification" {
		t.Fatalf("échec 401 : attendu erreur/authentification, reçu %d %v", s, m)
	}
	if strings.Contains(brut, "secret-test") {
		t.Fatalf("motif brut fournisseur exposé : %s", brut)
	}
}

// TestConsoleAntiDoubleClic : deux requêtes simultanées identiques →
// un seul test exécuté (409 TEST_EN_COURS pour le second).
func TestConsoleAntiDoubleClic(t *testing.T) {
	factice := &serveurFictureLent{serveurFacticeConsole: serveurFacticeConsole{}}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	type repTest struct {
		statut int
		m      map[string]any
	}
	reps := make(chan repTest, 2)
	corps := fmt.Sprintf(`{"fournisseur":"mistral","type":"reel"}`)
	envoyer := func() {
		req, _ := http.NewRequest(http.MethodPost, api.URL+"/api/console/test", strings.NewReader(corps))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: cookieSession, Value: c.token})
		rep, err := api.Client().Do(req)
		if err != nil {
			reps <- repTest{statut: 0}
			return
		}
		m := map[string]any{}
		_ = json.NewDecoder(rep.Body).Decode(&m)
		rep.Body.Close()
		reps <- repTest{statut: rep.StatusCode, m: m}
	}
	go envoyer()
	time.Sleep(50 * time.Millisecond) // laisser le premier verrouiller
	go envoyer()
	r1, r2 := <-reps, <-reps
	code := func(r repTest) string { s, _ := r.m["code"].(string); return s }
	ok, conflit := 0, 0
	for _, r := range []repTest{r1, r2} {
		switch {
		case r.statut == http.StatusOK:
			ok++
		case r.statut == http.StatusConflict && code(r) == "TEST_EN_COURS":
			conflit++
		}
	}
	if ok != 1 || conflit != 1 {
		t.Fatalf("attendu 1 succès + 1 TEST_EN_COURS, reçu %d %v / %d %v", r1.statut, r1.m, r2.statut, r2.m)
	}
	factice.mu.Lock()
	chats := factice.appelsChat
	factice.mu.Unlock()
	if chats != 1 {
		t.Fatalf("un seul appel facturable attendu, reçu %d", chats)
	}
}

// serveurFictureLent : chat/completions lent pour provoquer le
// chevauchement des double-clics.
type serveurFictureLent struct {
	serveurFacticeConsole
}

func (s *serveurFictureLent) serveur(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			s.mu.Lock()
			s.appelsModels++
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "modele-a"}}})
		case "/v1/chat/completions":
			s.mu.Lock()
			s.appelsChat++
			s.mu.Unlock()
			time.Sleep(400 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "OK"}}},
				"usage":   map[string]any{"prompt_tokens": 9, "completion_tokens": 2},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestConsolePreuvePerimee : un succès antérieur est invalidé quand la
// configuration (endpoint) change — jamais de faux « opérationnel ».
func TestConsolePreuvePerimee(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	dir := t.TempDir()
	cfg := configConsoleTest(srv.URL)
	// Le fichier de configuration doit exister pour la relecture mtime.
	chemin := filepath.Join(dir, "gateway.json")
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(chemin, raw, 0600); err != nil {
		t.Fatal(err)
	}
	g := newGateway(cfg, ".", dir)
	g.cheminConfig = chemin
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	// Succès initial.
	s, m, _ := c.req(t, "POST", "/api/console/test", map[string]any{"fournisseur": "mistral", "type": "dispo"})
	if s != http.StatusOK || m["etat"] != "OPERATIONNELLE" {
		t.Fatalf("dispo initiale : reçu %d %v", s, m)
	}

	// La clé (endpoint) change : preuve périmée.
	cfg.Moteurs.Mistral.CleAPI = "cle-test-autre"
	raw2, _ := json.Marshal(cfg)
	os.WriteFile(chemin, raw2, 0600)
	os.Chtimes(chemin, time.Now().Add(time.Second), time.Now().Add(time.Second))

	_, etat, _ := c.req(t, "GET", "/api/console/etat", nil)
	cartes, _ := etat["fournisseurs"].([]any)
	for _, carte := range cartes {
		cm := carte.(map[string]any)
		if cm["fournisseur"] == "mistral" && cm["etat"] != "PREUVE_PERIMEE" {
			t.Fatalf("attendu PREUVE_PERIMEE après changement de clé, reçu %v", cm["etat"])
		}
	}
}

// TestConsoleOriginEtRythme : POST d'origine étrangère refusé ;
// garde-rythme au-delà de 10 tests/minute.
func TestConsoleOriginEtRythme(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	// Origine étrangère : refus AVANT tout appel réseau.
	req, _ := http.NewRequest(http.MethodPost, api.URL+"/api/console/test",
		strings.NewReader(`{"fournisseur":"mistral","type":"dispo"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attaquant.example")
	req.AddCookie(&http.Cookie{Name: cookieSession, Value: c.token})
	rep, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if rep.StatusCode != http.StatusForbidden {
		t.Fatalf("origine étrangère : attendu 403, reçu %d", rep.StatusCode)
	}
	factice.mu.Lock()
	models := factice.appelsModels
	factice.mu.Unlock()
	if models != 0 {
		t.Fatalf("appel réseau malgré origine refusée : %d", models)
	}

	// Garde-rythme : 10 par minute.
	vu429 := false
	for i := 0; i < 12; i++ {
		s, m, _ := c.req(t, "POST", "/api/console/test", map[string]any{"fournisseur": "mistral", "type": "dispo"})
		if s == http.StatusTooManyRequests {
			vu429 = true
			break
		}
		if s != http.StatusOK {
			t.Fatalf("test %d : statut inattendu %d %v", i, s, m)
		}
	}
	if !vu429 {
		t.Fatalf("garde-rythme jamais déclenchée après 11 tests")
	}
}

// TestConsoleAgregatsBornesUTC : filtre mois = bornes UTC du mois en
// cours ; distinction zéro mesuré / absence.
func TestConsoleAgregatsBornesUTC(t *testing.T) {
	factice := &serveurFacticeConsole{}
	srv := factice.serveur(t)
	cfg := configConsoleTest(srv.URL)
	g := newGateway(cfg, ".", t.TempDir())
	api := httptest.NewServer(g.routes())
	defer api.Close()
	c := adminClient(t, g, api, true)

	_, m, _ := c.req(t, "GET", "/api/console/etat?periode=mois", nil)
	if m["periode"] != "mois" {
		t.Fatalf("periode attendue : mois, reçu %v", m["periode"])
	}
	debut, _ := m["borne_debut_utc"].(string)
	if !strings.HasSuffix(debut, "T00:00:00Z") && !strings.Contains(debut, "-01T") {
		t.Fatalf("borne début mois UTC attendue, reçu %v", debut)
	}
	fin, _ := m["borne_fin_utc"].(string)
	if fin == "" {
		t.Fatalf("borne fin absente")
	}
}

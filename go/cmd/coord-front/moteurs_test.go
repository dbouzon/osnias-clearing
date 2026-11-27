// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-API-MISTRAL-01 : grille de niveaux, adaptateurs
// (serveurs factices loopback uniquement), fail-closed, plafond,
// journal de coût, escalade Premium bornée, relecture à chaud.
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

// serveurAnthropicFactice : endpoint /v1/messages qui vérifie les
// en-têtes Anthropic (x-api-key, anthropic-version) et renvoie une
// réponse Messages contrôlée par le test.
func serveurAnthropicFactice(t *testing.T, reponses chan string) *httptest.Server {
	t.Helper()
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("chemin inattendu : %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "cle-test" {
			t.Errorf("en-tête x-api-key manquant ou faux")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("en-tête anthropic-version manquant")
		}
		mu.Lock()
		appels++
		curant := appels
		mu.Unlock()
		var corps struct {
			System   string          `json:"system"`
			Model    string          `json:"model"`
			Messages []messageMoteur `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&corps)
		if corps.System == "" || len(corps.Messages) == 0 {
			t.Errorf("corps Anthropic incomplet : %+v", corps)
		}
		var texte string
		select {
		case texte = <-reponses:
		default:
			texte = fmt.Sprintf("réponse factice n°%d", curant)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": texte}},
			"usage":   map[string]any{"input_tokens": 11, "output_tokens": 7},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// configMoteursTest : grille complète sur un seul fournisseur
// (anthropic, endpoint factice), niveaux et étapes explicites.
func configMoteursTest(compte compteConf, url string, plafond int) config {
	cfg := configTest(compte)
	cfg.Moteurs = moteursConf{
		Anthropic: fournisseurMoteurConf{Activer: true, CleAPI: "cle-test", URLAPI: url},
		Niveaux: map[string]niveauMoteurConf{
			"essentiel": {
				Cadrage:   etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Redaction: etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Critique:  etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Revision:  etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
			},
			"avance": {
				Cadrage:   etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Redaction: etapeMoteurConf{Moteur: "anthropic", Modele: "sonnet-test"},
				Critique:  etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Revision:  etapeMoteurConf{Moteur: "anthropic", Modele: "sonnet-test"},
			},
			"premium": {
				Cadrage:   etapeMoteurConf{Moteur: "anthropic", Modele: "haiku-test"},
				Redaction: etapeMoteurConf{Moteur: "anthropic", Modele: "sonnet-test"},
				Critique:  etapeMoteurConf{Moteur: "anthropic", Modele: "sonnet-test"},
				Revision:  etapeMoteurConf{Moteur: "anthropic", Modele: "opus-test"},
			},
		},
		PlafondAppels: plafond,
	}
	return cfg
}

func lireJournalMoteurs(t *testing.T, g *gateway) []entreeJournalMoteur {
	t.Helper()
	raw, err := os.ReadFile(g.cheminJournal)
	if err != nil {
		t.Fatalf("journal moteurs illisible : %v", err)
	}
	var f struct {
		Entrees []entreeJournalMoteur `json:"entrees"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("journal moteurs invalide : %v", err)
	}
	return f.Entrees
}

// TestMoteursSansCleRepliScript : SANS AUCUNE clé, repli intégral sur le
// comportement actuel (script déterministe), niveau stocké, 0 appel.
func TestMoteursSansCleRepliScript(t *testing.T) {
	g, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	c := connecter(t, srv, "alice")
	s, m, _ := c.req(t, "POST", "/api/projets",
		map[string]any{"id": "m1", "intention": "étudier le marché", "niveau": "essentiel"})
	if s != 201 {
		t.Fatalf("création : statut %d", s)
	}
	if m["niveau"] != "essentiel" || m["plafond_appels"] != float64(50) {
		t.Fatalf("niveau/plafond non stockés : %v", m)
	}
	s, _, _ = c.req(t, "POST", "/api/projets/m1/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	if s != 200 {
		t.Fatalf("dialogue : statut %d", s)
	}
	s, m, _ = c.req(t, "POST", "/api/projets/m1/tour", map[string]any{"entree": "un livrable"})
	if s != 200 {
		t.Fatalf("tour : statut %d", s)
	}
	rep := m["reponse"].(map[string]any)
	if rep["action"] != "QUESTION" || !strings.Contains(rep["contenu"].(string), "finalité") {
		t.Fatalf("repli script attendu (QUESTION scriptée), reçu : %v", rep)
	}
	projetRepli := m["projet"].(map[string]any)
	if projetRepli["appels_moteurs"] != float64(0) {
		t.Fatalf("aucun appel moteur attendu en repli, reçu %v", projetRepli["appels_moteurs"])
	}
	if _, err := os.Stat(g.cheminJournal); !os.IsNotExist(err) {
		t.Fatalf("journal moteurs ne doit exister qu'après un appel réel")
	}
}

// TestNiveauInconnuRefuse : niveau hors grille → 400 NIVEAU_INCONNU.
func TestNiveauInconnuRefuse(t *testing.T) {
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	c := connecter(t, srv, "alice")
	s, m, _ := c.req(t, "POST", "/api/projets",
		map[string]any{"id": "x", "intention": "i", "niveau": "or"})
	if s != 400 || m["code"] != "NIVEAU_INCONNU" {
		t.Fatalf("attendu 400 NIVEAU_INCONNU, reçu %d %v", s, m)
	}
	// Grille restreinte : un niveau connu mais absent de la config → refus.
	cfg := configMoteursTest(compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir()},
		"http://127.0.0.1:1", 50)
	delete(cfg.Moteurs.Niveaux, "premium")
	_, srv2 := gatewayTest(t, cfg)
	c2 := connecter(t, srv2, "bob")
	s, m, _ = c2.req(t, "POST", "/api/projets",
		map[string]any{"id": "x", "intention": "i", "niveau": "premium"})
	if s != 400 || m["code"] != "NIVEAU_INCONNU" {
		t.Fatalf("niveau absent de la grille : attendu 400, reçu %d %v", s, m)
	}
}

// TestCascadeCadrageRedaction : trois tours → QUESTION/CLARIFICATION
// produits par le moteur, puis PROPOSITION_ORDRE dont les champs
// rédigeables viennent de l'étape rédaction ; compteur et journal.
func TestCascadeCadrageRedaction(t *testing.T) {
	reponses := make(chan string, 8)
	reponses <- "Quelle est la finalité précise attendue ?"
	reponses <- "Quel périmètre suffit pour un premier livrable ?"
	reponses <- `{"etat_des_lieux":"rien n'existe","objectif":"produire l'étude","etapes":[{"nom":"cadrage","controle_intermediaire":"revue"},{"nom":"rédaction","controle_intermediaire":"contrôle"}],"limites":["pas de hors-sujet"],"livrable":"etude.md","criteres_reception":["clair","vérifiable"]}`
	srv := serveurAnthropicFactice(t, reponses)
	g, srvGW := gatewayTest(t, configMoteursTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50))
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas1", "intention": "étudier le marché", "niveau": "avance"})
	c.req(t, "POST", "/api/projets/cas1/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})

	s, m, _ := c.req(t, "POST", "/api/projets/cas1/tour", map[string]any{"entree": "une étude"})
	if s != 200 || m["reponse"].(map[string]any)["contenu"] != "Quelle est la finalité précise attendue ?" {
		t.Fatalf("tour 1 : contenu moteur attendu, reçu %d %v", s, m)
	}
	s, m, _ = c.req(t, "POST", "/api/projets/cas1/tour", map[string]any{"entree": "une étude de marché"})
	if s != 200 || !strings.Contains(m["reponse"].(map[string]any)["contenu"].(string), "périmètre") {
		t.Fatalf("tour 2 : contenu moteur attendu, reçu %d %v", s, m)
	}
	s, m, _ = c.req(t, "POST", "/api/projets/cas1/tour", map[string]any{"entree": "un rapport"})
	if s != 200 {
		t.Fatalf("tour 3 : statut %d %v", s, m)
	}
	projetTour := m["projet"].(map[string]any)
	if projetTour["appels_moteurs"] != float64(3) {
		t.Fatalf("3 appels moteur attendus, reçu %v", projetTour["appels_moteurs"])
	}
	ordre := m["projet"].(map[string]any)["ordre"].(map[string]any)
	if ordre["objectif"] != "produire l'étude" || ordre["livrable"] != "etude.md" {
		t.Fatalf("ordre non enrichi par la rédaction : %v", ordre)
	}

	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 3 {
		t.Fatalf("3 entrées de journal attendues, reçu %d", len(entrees))
	}
	raw, _ := os.ReadFile(g.cheminJournal)
	if strings.Contains(string(raw), "cle-test") {
		t.Fatalf("clé API trouvée dans le journal de coût")
	}
	fi, _ := os.Stat(g.cheminJournal)
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("journal moteurs : permissions %v attendu 0600", fi.Mode().Perm())
	}
}

// TestCritiqueEtEscaladePremiumBornee : critique insuffisante en niveau
// premium → UNE escalade (étape revision, version +1) ; jamais deux.
func TestCritiqueEtEscaladePremiumBornee(t *testing.T) {
	reponses := make(chan string, 8)
	for i := 0; i < 3; i++ {
		reponses <- "réponse de cadrage"
	}
	reponses <- `{"suffisant": false, "remarques": ["objectif flou"]}`
	reponses <- `{"etat_des_lieux":"corrigé","objectif":"objectif clarifié","etapes":[{"nom":"cadrage","controle_intermediaire":"revue"}],"limites":[],"livrable":"l.md","criteres_reception":["clair"]}`
	srv := serveurAnthropicFactice(t, reponses)
	g, srvGW := gatewayTest(t, configMoteursTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50))
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas2", "intention": "mission premium", "niveau": "premium"})
	c.req(t, "POST", "/api/projets/cas2/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	for i := 0; i < 3; i++ {
		c.req(t, "POST", "/api/projets/cas2/tour", map[string]any{"entree": "avancez"})
	}
	s, m, _ := c.req(t, "POST", "/api/projets/cas2/qualifier", nil)
	if s != 200 {
		t.Fatalf("qualifier : statut %d %v", s, m)
	}
	p := m
	if p["critique"].(map[string]any)["suffisant"] != false {
		t.Fatalf("critique attendue insuffisante : %v", p["critique"])
	}
	if p["version"] != float64(2) {
		t.Fatalf("escalade premium attendue (v2 après qualification v1 + révision), reçu %v", p["version"])
	}
	ordre := p["ordre"].(map[string]any)
	if ordre["objectif"] != "objectif clarifié" {
		t.Fatalf("ordre non révisé par l'escalade : %v", ordre)
	}
	nbEscalades := 0
	for _, e := range lireJournalMoteurs(t, g) {
		if e.Etape == "revision" {
			nbEscalades++
		}
	}
	if nbEscalades != 1 {
		t.Fatalf("exactement UNE escalade attendue, reçu %d", nbEscalades)
	}
}

// TestPlafondAppels : plafond 2 → 3e appel refusé 429 PLAFOND_ATTEINT.
func TestPlafondAppels(t *testing.T) {
	reponses := make(chan string, 8)
	srv := serveurAnthropicFactice(t, reponses)
	_, srvGW := gatewayTest(t, configMoteursTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 2))
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas3", "intention": "mission bornée", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/cas3/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	c.req(t, "POST", "/api/projets/cas3/tour", map[string]any{"entree": "a"})
	c.req(t, "POST", "/api/projets/cas3/tour", map[string]any{"entree": "b"})
	s, m, _ := c.req(t, "POST", "/api/projets/cas3/tour", map[string]any{"entree": "c"})
	if s != 429 || m["code"] != "PLAFOND_ATTEINT" {
		t.Fatalf("attendu 429 PLAFOND_ATTEINT, reçu %d %v", s, m)
	}
}

// TestReessaiUnique : erreur réseau puis succès → UN nouvel essai,
// réponse OK, essai=2 journalisé ; puis échec persistant → 502.
func TestReessaiUnique(t *testing.T) {
	mu := sync.Mutex{}
	appels := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		appels++
		n := appels
		mu.Unlock()
		if n == 1 || n >= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "après réessai"}}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3},
		})
	}))
	t.Cleanup(srv.Close)
	cfg := configMoteursTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, "", 50)
	cfg.Moteurs.Mistral = fournisseurMoteurConf{Activer: true, CleAPI: "cle-test", URLAPI: srv.URL}
	cfg.Moteurs.Anthropic = fournisseurMoteurConf{}
	for niv := range cfg.Moteurs.Niveaux {
		n := cfg.Moteurs.Niveaux[niv]
		n.Cadrage = etapeMoteurConf{Moteur: "mistral", Modele: "small-test"}
		n.Redaction = etapeMoteurConf{Moteur: "mistral", Modele: "small-test"}
		n.Critique = etapeMoteurConf{Moteur: "mistral", Modele: "small-test"}
		n.Revision = etapeMoteurConf{Moteur: "mistral", Modele: "small-test"}
		cfg.Moteurs.Niveaux[niv] = n
	}
	g, srvGW := gatewayTest(t, cfg)
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas4", "intention": "réessai", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/cas4/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/cas4/tour", map[string]any{"entree": "go"})
	if s != 200 || m["reponse"].(map[string]any)["contenu"] != "après réessai" {
		t.Fatalf("réessai attendu, reçu %d %v", s, m)
	}
	if appels != 2 {
		t.Fatalf("2 appels fournisseur attendus (1 échec + 1 essai), reçu %d", appels)
	}
	if e := lireJournalMoteurs(t, g)[0]; e.Essai != 2 || e.Resultat != "ok" {
		t.Fatalf("journal : essai=2 ok attendu, reçu %+v", e)
	}
	// Échec persistant : deux tentatives puis 502 ERREUR_MOTEUR.
	for i := 0; i < 2; i++ {
		s, m, _ = c.req(t, "POST", "/api/projets/cas4/tour", map[string]any{"entree": "ko"})
	}
	if s != 502 || m["code"] != "ERREUR_MOTEUR" {
		t.Fatalf("attendu 502 ERREUR_MOTEUR, reçu %d %v", s, m)
	}
}

// TestEtapeFournisseurInactif : étape pointant un fournisseur sans clé
// → 503 MOTEUR_DESACTIVE (fail-closed), repli configuré sinon.
func TestEtapeFournisseurInactif(t *testing.T) {
	srv := serveurAnthropicFactice(t, make(chan string, 4))
	cfg := configMoteursTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srv.URL, 50)
	// Mistral déclaré pour l'étape cadrage MAIS sans clé, repli anthropic.
	n := cfg.Moteurs.Niveaux["essentiel"]
	n.Cadrage = etapeMoteurConf{Moteur: "mistral", Modele: "small-test", Repli: "anthropic"}
	cfg.Moteurs.Niveaux["essentiel"] = n
	cfg.Moteurs.Mistral = fournisseurMoteurConf{Activer: true, CleAPI: cleMoteurNonRenseignee}
	if err := validerMoteurs(cfg.Moteurs); err == nil {
		t.Fatalf("activer sans clé réelle doit être refusé au démarrage")
	}
	cfg.Moteurs.Mistral = fournisseurMoteurConf{}
	if err := validerMoteurs(cfg.Moteurs); err != nil {
		t.Fatalf("repli valide refusé à tort : %v", err)
	}
	_, srvGW := gatewayTest(t, cfg)
	c := connecter(t, srvGW, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas5", "intention": "repli", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/cas5/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/cas5/tour", map[string]any{"entree": "go"})
	if s != 200 {
		t.Fatalf("repli anthropic attendu, reçu %d %v", s, m)
	}
	// Sans repli : 503 MOTEUR_DESACTIVE.
	n2 := cfg.Moteurs.Niveaux["essentiel"]
	n2.Cadrage = etapeMoteurConf{Moteur: "openai", Modele: "gpt-test"}
	cfg.Moteurs.Niveaux["essentiel"] = n2
	_, srvGW2 := gatewayTest(t, cfg)
	c2 := connecter(t, srvGW2, "alice")
	c2.req(t, "POST", "/api/projets",
		map[string]any{"id": "cas6", "intention": "ko", "niveau": "essentiel"})
	c2.req(t, "POST", "/api/projets/cas6/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ = c2.req(t, "POST", "/api/projets/cas6/tour", map[string]any{"entree": "go"})
	if s != 503 || m["code"] != "MOTEUR_DESACTIVE" {
		t.Fatalf("attendu 503 MOTEUR_DESACTIVE, reçu %d %v", s, m)
	}
}

// TestRelectureAChaud : mtime de gateway.json modifié → nouveau régime
// pris en compte sans redéploiement ; section invalide = ancien régime.
func TestRelectureAChaud(t *testing.T) {
	dossier := t.TempDir()
	chemin := filepath.Join(dossier, "gateway.json")
	serveur := serveurAnthropicFactice(t, make(chan string, 4))
	cfg := configTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()})
	g := newGateway(cfg, ".", dossier)
	g.cheminConfig = chemin
	g.mtimeMoteurs = time.Now().Add(-time.Hour)

	ecrire := func(activer bool) {
		cle := "A_RENSEIGNER_PAR_LE_PILOTE"
		if activer {
			cle = "cle-test"
		}
		raw, _ := json.Marshal(map[string]any{"moteurs": map[string]any{
			"anthropic": map[string]any{"activer": activer, "cle_api": cle},
			"niveaux": map[string]any{"avance": map[string]any{
				"cadrage":   map[string]any{"moteur": "anthropic", "modele": "haiku"},
				"redaction": map[string]any{"moteur": "anthropic", "modele": "haiku"},
				"critique":  map[string]any{"moteur": "anthropic", "modele": "haiku"},
				"revision":  map[string]any{"moteur": "anthropic", "modele": "haiku"},
			}},
		}})
		if err := os.WriteFile(chemin, raw, 0640); err != nil {
			t.Fatal(err)
		}
		futur := time.Now().Add(2 * time.Hour)
		os.Chtimes(chemin, futur, futur)
	}

	ecrire(false)
	if g.moteursActifs() {
		t.Fatalf("moteurs ne doivent pas être actifs")
	}
	ecrire(true)
	if !g.moteursActifs() {
		t.Fatalf("relecture à chaud : moteurs doivent devenir actifs")
	}
	if g.regimeMoteurs().Anthropic.URLAPI != "" {
		t.Fatalf("url par défaut attendue")
	}
	// Section invalide (activer sans clé) : l'ancien régime est conservé.
	raw, _ := json.Marshal(map[string]any{"moteurs": map[string]any{
		"anthropic": map[string]any{"activer": true, "cle_api": ""},
	}})
	os.WriteFile(chemin, raw, 0640)
	futur := time.Now().Add(3 * time.Hour)
	os.Chtimes(chemin, futur, futur)
	if !g.moteursActifs() {
		t.Fatalf("section invalide : l'ancien régime (actif) doit être conservé")
	}
	_ = serveur
}

// TestValiderMoteursFailClosed : grilles invalides refusées au démarrage.
func TestValiderMoteursFailClosed(t *testing.T) {
	cas := []moteursConf{
		{Anthropic: fournisseurMoteurConf{Activer: true}},
		{Anthropic: fournisseurMoteurConf{Activer: true, CleAPI: cleMoteurNonRenseignee}},
		{Niveaux: map[string]niveauMoteurConf{"or": {}}},
		{Niveaux: map[string]niveauMoteurConf{"avance": {
			Cadrage: etapeMoteurConf{Moteur: "inconnu", Modele: "m"}}}},
		{Niveaux: map[string]niveauMoteurConf{"avance": {
			Cadrage: etapeMoteurConf{Moteur: "mistral"}}}},
		{Niveaux: map[string]niveauMoteurConf{"avance": {
			Cadrage: etapeMoteurConf{Moteur: "mistral", Modele: "m", Repli: "mistral"}}}},
	}
	for i, mc := range cas {
		if err := validerMoteurs(mc); err == nil {
			t.Fatalf("cas %d : grille invalide acceptée", i)
		}
	}
}

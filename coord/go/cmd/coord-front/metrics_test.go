// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests M-COORD-ORPHEUS-01 : compteurs Prometheus du chemin moteurs,
// exposition /metrics fail-closed (token), cohérence compteur/journal.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const tokenMetricsTest = "token-metrics-abcdef0123"

func tempsPasse() time.Time { return time.Now().Add(-time.Hour) }

func ecrireConfigTemp(t *testing.T, chemin string, m map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chemin, raw, 0640); err != nil {
		t.Fatal(err)
	}
	futur := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(chemin, futur, futur); err != nil {
		t.Fatal(err)
	}
	return chemin
}

func configMetricsTest(cfg config) config {
	cfg.Metrics = metricsConf{Activer: true, Token: tokenMetricsTest}
	return cfg
}

func obtenirMetrics(t *testing.T, srvURL, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srvURL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	raw, _ := io.ReadAll(rep.Body)
	return rep.StatusCode, string(raw)
}

// TestMetricsFailClosed : désactivée → 404 ; activée sans token ou
// mauvais token → 404 (pas de divulgation d'existence) ; bon token →
// 200 au format texte Prometheus avec HELP/TYPE.
func TestMetricsFailClosed(t *testing.T) {
	// Désactivée : 404 même avec un token.
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	s, corps := obtenirMetrics(t, srv.URL, "nimporte")
	if s != 404 || strings.Contains(corps, "osnias_coord") {
		t.Fatalf("metrics désactivée : attendu 404 sans exposition, reçu %d %q", s, corps)
	}
	// Activée : sans token → 404, mauvais token → 404.
	cfg := configMetricsTest(configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	_, srv2 := gatewayTest(t, cfg)
	s, _ = obtenirMetrics(t, srv2.URL, "")
	if s != 404 {
		t.Fatalf("metrics sans token : attendu 404, reçu %d", s)
	}
	s, _ = obtenirMetrics(t, srv2.URL, "faux-token-de-test-16")
	if s != 404 {
		t.Fatalf("metrics mauvais token : attendu 404, reçu %d", s)
	}
	// Bon token : 200, format texte, familles présentes.
	s, corps = obtenirMetrics(t, srv2.URL, tokenMetricsTest)
	if s != 200 {
		t.Fatalf("metrics bon token : attendu 200, reçu %d", s)
	}
	for _, attendu := range []string{
		"# HELP coord_moteurs_appels_total",
		"# TYPE coord_moteurs_appels_total counter",
		"coord_moteurs_jetons_entree_total",
		"coord_moteurs_jetons_sortie_total",
		"# TYPE coord_moteurs_duree_seconds histogram",
	} {
		if !strings.Contains(corps, attendu) {
			t.Fatalf("exposition : %q absent du rendu", attendu)
		}
	}
	if strings.Contains(corps, tokenMetricsTest) {
		t.Fatalf("le token ne doit jamais apparaître dans l'exposition")
	}
}

// TestMetricsCompteurAppelReel : un appel moteur réel (serveur factice)
// incrémente appels_total ok, jetons, durée ; cohérence compteur/journal.
func TestMetricsCompteurAppelReel(t *testing.T) {
	reponses := make(chan string, 8)
	reponses <- "Quelle finalité précise ?"
	srvAPI := serveurAnthropicFactice(t, reponses)
	cfg := configMetricsTest(configMoteursTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srvAPI.URL, 50))
	g, srv := gatewayTest(t, cfg)
	c := connecter(t, srv, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "met1", "intention": "test orpheus", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/met1/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, m, _ := c.req(t, "POST", "/api/projets/met1/tour", map[string]any{"entree": "une étude"})
	if s != 200 {
		t.Fatalf("tour : statut %d", s)
	}
	if m["projet"].(map[string]any)["appels_moteurs"] != float64(1) {
		t.Fatalf("1 appel moteur attendu")
	}
	_, corps := obtenirMetrics(t, srv.URL, tokenMetricsTest)
	ligne := `coord_moteurs_appels_total{etape="cadrage",modele="haiku-test",moteur="anthropic",niveau="essentiel",resultat="ok"} 1`
	if !strings.Contains(corps, ligne) {
		t.Fatalf("compteur appels ok absent ou faux :\n%s", corps)
	}
	if !strings.Contains(corps, `coord_moteurs_jetons_entree_total{modele="haiku-test",moteur="anthropic"} 11`) {
		t.Fatalf("jetons entrée attendus (11), exposition :\n%s", corps)
	}
	if !strings.Contains(corps, `coord_moteurs_jetons_sortie_total{modele="haiku-test",moteur="anthropic"} 7`) {
		t.Fatalf("jetons sortie attendus (7), exposition :\n%s", corps)
	}
	if !strings.Contains(corps, `coord_moteurs_duree_seconds_count{etape="cadrage",moteur="anthropic"} 1`) {
		t.Fatalf("histogramme durée attendu (_count 1), exposition :\n%s", corps)
	}
	// Cohérence compteur/journal : la même source, mêmes valeurs.
	entrees := lireJournalMoteurs(t, g)
	if len(entrees) != 1 || entrees[0].Resultat != "ok" ||
		entrees[0].JetonsEntree != 11 || entrees[0].JetonsSortie != 7 {
		t.Fatalf("journal incohérent avec les compteurs : %+v", entrees)
	}
}

// TestMetricsRefusComptes : plafond atteint et fournisseur désactivé
// comptés avec resultat=refuse (spec §3.1), sans appel réseau.
func TestMetricsRefusComptes(t *testing.T) {
	reponses := make(chan string, 8)
	srvAPI := serveurAnthropicFactice(t, reponses)
	cfg := configMetricsTest(configMoteursTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()}, srvAPI.URL, 1))
	_, srv := gatewayTest(t, cfg)
	c := connecter(t, srv, "alice")
	c.req(t, "POST", "/api/projets",
		map[string]any{"id": "met2", "intention": "borné", "niveau": "essentiel"})
	c.req(t, "POST", "/api/projets/met2/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	c.req(t, "POST", "/api/projets/met2/tour", map[string]any{"entree": "a"}) // ok
	s, _, _ := c.req(t, "POST", "/api/projets/met2/tour", map[string]any{"entree": "b"})
	if s != 429 {
		t.Fatalf("plafond : attendu 429, reçu %d", s)
	}
	_, corps := obtenirMetrics(t, srv.URL, tokenMetricsTest)
	if !strings.Contains(corps, `resultat="refuse"} 1`) {
		t.Fatalf("refus plafond attendu dans appels_total (resultat=refuse), exposition :\n%s", corps)
	}

	// Fournisseur désactivé sur l'étape (sans repli) : 503 + refuse.
	cfg2 := configMetricsTest(configMoteursTest(
		compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir()}, srvAPI.URL, 50))
	n := cfg2.Moteurs.Niveaux["essentiel"]
	n.Cadrage = etapeMoteurConf{Moteur: "openai", Modele: "gpt-test"}
	cfg2.Moteurs.Niveaux["essentiel"] = n
	_, srv2 := gatewayTest(t, cfg2)
	c2 := connecter(t, srv2, "bob")
	c2.req(t, "POST", "/api/projets",
		map[string]any{"id": "met3", "intention": "ko", "niveau": "essentiel"})
	c2.req(t, "POST", "/api/projets/met3/dialogue",
		map[string]any{"directeur": "d", "executant": "e"})
	s, _, _ = c2.req(t, "POST", "/api/projets/met3/tour", map[string]any{"entree": "go"})
	if s != 503 {
		t.Fatalf("fournisseur désactivé : attendu 503, reçu %d", s)
	}
	_, corps2 := obtenirMetrics(t, srv2.URL, tokenMetricsTest)
	if !strings.Contains(corps2, `moteur="openai"`) ||
		!strings.Contains(corps2, `resultat="refuse"} 1`) {
		t.Fatalf("refus fournisseur désactivé attendu, exposition :\n%s", corps2)
	}
}

// TestMetricsRelectureAChaud : section metrics relue par mtime
// (activation sans redéploiement), activer sans token = conservé.
func TestMetricsRelectureAChaud(t *testing.T) {
	dossier := t.TempDir()
	chemin := dossier + "/gateway.json"
	cfg := configTest(compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()})
	g := newGateway(cfg, ".", dossier)
	g.cheminConfig = chemin
	g.mtimeMoteurs = tempsPasse()

	ecrire := func(m map[string]any) string {
		return ecrireConfigTemp(t, chemin, m)
	}
	ecrire(map[string]any{"metrics": map[string]any{"activer": false, "token": ""}})
	if g.metrics.Activer {
		t.Fatalf("metrics doivent être inactives")
	}
	ecrire(map[string]any{"metrics": map[string]any{"activer": true, "token": "nouveau-token-16car-min"}})
	g.regimeMoteurs()
	if !g.metrics.Activer || g.metrics.Token != "nouveau-token-16car-min" {
		t.Fatalf("relecture à chaud : metrics attendues actives, reçu %+v", g.metrics)
	}
	// activer sans token : l'ancien régime est conservé (fail-closed).
	ecrire(map[string]any{"metrics": map[string]any{"activer": true, "token": ""}})
	g.regimeMoteurs()
	if g.metrics.Token != "nouveau-token-16car-min" {
		t.Fatalf("activer sans token : l'ancien token devait être conservé, reçu %+v", g.metrics)
	}
}

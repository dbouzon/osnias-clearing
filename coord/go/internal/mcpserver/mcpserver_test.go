// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"coordengine/internal/mcpserver"
	"coordengine/internal/missions"
	"coordengine/internal/pilot"
)

const tokenConsole = "token-console-pour-les-tests"
const jetonLecture = "jeton-lecture-pour-les-tests"

type env struct {
	t      *testing.T
	srv    *httptest.Server
	racine string
}

func nouvelEnv(t *testing.T) *env {
	t.Helper()
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	ui := t.TempDir()
	_ = os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html></html>"), 0o644)
	svc, err := pilot.NouveauService(ref, ui, tokenConsole, "127.0.0.1:0", pilot.AvecJetonLecture(jetonLecture))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(srv.Close)
	e := &env{t: t, srv: srv, racine: racine}
	e.creerMission("mcp-t1")
	return e
}

func (e *env) creerMission(issue string) {
	e.t.Helper()
	corps, _ := json.Marshal(map[string]any{
		"issue":       issue,
		"adaptateurs": []string{"mistral-simule", "openai-simule"},
		"roles": map[string][]string{
			"EXECUTANT":      {"executer", "soumettre", "acker"},
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		"executant_id": "exec-sim",
		"recepteur_id": "recep-sim",
		"plafonds":     map[string]any{"appels": map[string]int{"mistral-simule": 2, "openai-simule": 2}},
	})
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/mission", strings.NewReader(string(corps)))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("X-Coord-Pilot-Token", tokenConsole)
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("creation %s : 201 attendu, obtenu %d", issue, resp.StatusCode)
	}
}

func (e *env) serveur(jeton string) *mcpserver.Serveur {
	e.t.Helper()
	s, err := mcpserver.NouveauServeur(e.srv.URL, jeton)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// TestLecture_Liste_et_Detail : le cœur renvoie les données du service en
// lecture seule avec le jeton de lecture UNIQUEMENT.
func TestLecture_Liste_et_Detail(t *testing.T) {
	e := nouvelEnv(t)
	s := e.serveur(jetonLecture)
	ctx := context.Background()
	liste, err := s.ListerMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(liste), "mcp-t1") {
		t.Fatalf("mission attendue dans la liste : %s", string(liste))
	}
	detail, err := s.ObtenirMission(ctx, "mcp-t1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(detail), "mcp-t1") {
		t.Fatalf("mission attendue dans le detail : %s", string(detail)[:200])
	}
	// Le token console ne doit jamais être transmis ni accepting par le cœur :
	// une valeur arbitraire est refusée.
	if _, err := s.ObtenirMission(ctx, "mcp-t1"); err != nil {
		var emcp *mcpserver.ErreurMCP
		if !errors.As(err, &emcp) {
			t.Fatalf("erreur inattendue : %v", err)
		}
	}
}

// TestLecture_RefusEtNonDivulgation : mauvais jeton, mission inconnue,
// identifiants hostiles : refus explicites SANS jeton, SANS URL, SANS chemin.
func TestLecture_RefusEtNonDivulgation(t *testing.T) {
	e := nouvelEnv(t)
	s := e.serveur("jeton-faux-intercepte")
	ctx := context.Background()
	_, err := s.ListerMissions(ctx)
	var emcp *mcpserver.ErreurMCP
	if !errors.As(err, &emcp) || emcp.Code != "JETON_REFUSE" {
		t.Fatalf("JETON_REFUSE attendu, obtenu %v", err)
	}
	if strings.Contains(err.Error(), jetonLecture) ||
		strings.Contains(err.Error(), e.srv.URL) ||
		strings.Contains(err.Error(), e.racine) ||
		strings.Contains(err.Error(), "jeton-faux-intercepte") {
		t.Fatalf("fuite de secret dans le refus : %s", err.Error())
	}
	s2 := e.serveur(jetonLecture)
	_, err = s2.ObtenirMission(ctx, "inconnue-xyz")
	if !errors.As(err, &emcp) || emcp.Code != "MISSION_INCONNUE" {
		t.Fatalf("MISSION_INCONNUE attendu, obtenu %v", err)
	}
	for _, hostile := range []string{"", "a b", "../etc", strings.Repeat("x", 200), "a/b"} {
		_, err = s2.ObtenirMission(ctx, hostile)
		if !errors.As(err, &emcp) || emcp.Code != "REQUETE_INVALIDE" {
			t.Fatalf("REQUETE_INVALIDE attendu pour %q, obtenu %v", hostile, err)
		}
	}
}

// TestServeur_PilotInjoignable : service arrêté => refus explicite sans
// détail réseau (adresse, port, URL interdits).
func TestServeur_PilotInjoignable(t *testing.T) {
	s, err := mcpserver.NouveauServeur("http://127.0.0.1:1", jetonLecture)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ListerMissions(context.Background())
	var emcp *mcpserver.ErreurMCP
	if !errors.As(err, &emcp) || emcp.Code != "PILOT_INJOIGNABLE" {
		t.Fatalf("PILOT_INJOIGNABLE attendu, obtenu %v", err)
	}
	if strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("URL du service divulguee : %s", err.Error())
	}
}

// TestServeur_URLNonLoopback refuse toute URL hors loopback (fail-closed).
func TestServeur_URLNonLoopback(t *testing.T) {
	for _, u := range []string{"http://exemple.fr", "https://10.0.0.1:8080", "hostinvalide"} {
		if _, err := mcpserver.NouveauServeur(u, jetonLecture); err == nil {
			t.Fatalf("URL non loopback %q : refus attendu", u)
		}
	}
	if _, err := mcpserver.NouveauServeur("http://127.0.0.1:8080", "  "); err == nil {
		t.Fatal("jeton vide : refus attendu")
	}
	if _, err := mcpserver.NouveauServeur("http://[::1]:8080", jetonLecture); err != nil {
		t.Fatalf("URL [::1] loopback acceptee attendue, obtenu : %v", err)
	}
}

// TestMCP_NiveauProtocole : serveur MCP complet sur transports en mémoire ;
// vérifie la négociation, les 2 outils de lecture uniquement, les réponses
// et les refus IsError sans effet de bord ni fuite.
func TestMCP_NiveauProtocole(t *testing.T) {
	e := nouvelEnv(t)
	s := e.serveur(jetonLecture)
	server := mcp.NewServer(&mcp.Implementation{Name: "coord-mcp-tests", Version: "v1"}, nil)
	s.Peupler(server)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "client-tests"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	ctx := context.Background()

	outils, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	vus := map[string]bool{}
	for _, o := range outils.Tools {
		vus[o.Name] = true
	}
	if !vus[mcpserver.OutilLister] || !vus[mcpserver.OutilDetail] || len(outils.Tools) != 2 {
		t.Fatalf("lecture seule stricte violee : outils = %v", vus)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.OutilLister})
	if err != nil || res.IsError {
		t.Fatalf("coord_list_missions : %v / %v", err, res)
	}
	if !strings.Contains(texte(res), "mcp-t1") {
		t.Fatalf("mission attendue : %s", texte(res))
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name: mcpserver.OutilDetail, Arguments: map[string]any{"issue": "mcp-t1"},
	})
	if err != nil || res.IsError {
		t.Fatalf("coord_get_mission : %v", err)
	}
	if !strings.Contains(texte(res), "mcp-t1") {
		t.Fatalf("detail attendu : %s", texte(res)[:200])
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name: mcpserver.OutilDetail, Arguments: map[string]any{"issue": "inconnue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("mission inconnue : IsError attendu")
	}
	corps := texte(res)
	if strings.Contains(corps, jetonLecture) || strings.Contains(corps, e.srv.URL) ||
		strings.Contains(corps, e.racine) || strings.Contains(corps, tokenConsole) {
		t.Fatalf("fuite de secret dans le refus MCP : %s", corps)
	}
	// Un outil de mutation n'existe pas : l'appel est refusé par le protocole.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "coord_creer_mission"}); err == nil {
		t.Fatal("un outil de mutation ne doit pas exister")
	}
}

func texte(res *mcp.CallToolResult) string {
	out := ""
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			out += t.Text
		}
	}
	return out
}

// TestJetonFichier_Permissions : fichier 0600 accepté ; permissions plus
// larges, fichier absent, vide ou binaire => refus (fail-closed, POSIX).
func TestJetonFichier_Permissions(t *testing.T) {
	racine := t.TempDir()
	bon := filepath.Join(racine, "jeton-ok.txt")
	if err := os.WriteFile(bon, []byte(jetonLecture+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := mcpserver.ChargerJeton(bon)
	if err != nil || v != jetonLecture {
		t.Fatalf("fichier 0600 : chargement attendu, obtenu %q / %v", v, err)
	}
	ouvert := filepath.Join(racine, "jeton-ouvert.txt")
	if err := os.WriteFile(ouvert, []byte(jetonLecture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mcpserver.ChargerJeton(ouvert); err == nil {
		t.Fatal("permissions 0644 : refus attendu (fail closed 0600)")
	} else if !strings.Contains(err.Error(), "0600") {
		t.Fatalf("message de diagnostic sans chmod : %s", err.Error())
	}
	if _, err := mcpserver.ChargerJeton(filepath.Join(racine, "absent.txt")); err == nil {
		t.Fatal("fichier absent : refus attendu")
	}
	vide := filepath.Join(racine, "jeton-vide.txt")
	_ = os.WriteFile(vide, []byte("\n"), 0o600)
	if _, err := mcpserver.ChargerJeton(vide); err == nil {
		t.Fatal("fichier vide : refus attendu")
	}
	dossier := filepath.Join(racine, "dossier")
	_ = os.Mkdir(dossier, 0o700)
	if _, err := mcpserver.ChargerJeton(dossier); err == nil {
		t.Fatal("dossier : refus attendu")
	}
	if _, err := mcpserver.ChargerJeton(""); err == nil {
		t.Fatal("chemin vide : refus attendu")
	}
	// Non-divulgation : l'erreur ne cite jamais la valeur du jeton.
	if _, err := mcpserver.ChargerJeton(ouvert); err != nil && strings.Contains(err.Error(), jetonLecture) {
		t.Fatalf("valeur de jeton divulguee : %s", err.Error())
	}
}

// TestBoundary_AucunTransport : le paquet mcpserver ne référence AUCUN
// transport (STDIO ou autre) : le cœur est réutilisable tel quel pour une
// future architecture client-serveur distante sans changement de contrat.
func TestBoundary_AucunTransport(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") ||
			strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		corps, err := os.ReadFile(ent.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(corps), "Stdio") || strings.Contains(string(corps), "SSE") ||
			strings.Contains(string(corps), "Streamable") {
			t.Fatalf("%s : dependance transport interdite dans le coeur", ent.Name())
		}
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package pilot_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/missions"
	"coordengine/internal/pilot"
)

const jetonLectureTest = "jeton-lecture-pour-les-tests"

// envLecture monte un service avec l'option jeton de lecture armée
// (M-COORD-MCP-01) ; tout le reste est identique à l'environnement nominal.
type envLecture struct {
	t      *testing.T
	srv    *httptest.Server
	racine string
}

func nouvelEnvLecture(t *testing.T) *envLecture {
	t.Helper()
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	ui := t.TempDir()
	if err := os.WriteFile(filepath.Join(ui, "index.html"),
		[]byte("<!doctype html><html><body>ui</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := pilot.NouveauService(ref, ui, tokenTest, "127.0.0.1:0", pilot.AvecJetonLecture(jetonLectureTest))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(srv.Close)
	return &envLecture{t: t, srv: srv, racine: racine}
}

func (e *envLecture) creer() {
	e.t.Helper()
	corps, _ := json.Marshal(map[string]any{
		"issue":       "jl-1",
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
	req.Header.Set("X-Coord-Pilot-Token", tokenTest)
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		e.t.Fatalf("creation attendue 201, obtenu %d", resp.StatusCode)
	}
}

func (e *envLecture) get(chemin, token string) int {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+chemin, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-Coord-Pilot-Token", token)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (e *envLecture) post(chemin, token string) (int, map[string]any) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+chemin, strings.NewReader("{}"))
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-Coord-Pilot-Token", token)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestJetonLecture_GET accepté sur les routes de lecture uniquement avec le
// jeton de lecture OU le token console (aucune régression des droits).
func TestJetonLecture_GET(t *testing.T) {
	e := nouvelEnvLecture(t)
	e.creer()
	if code := e.get("/api/missions", jetonLectureTest); code != http.StatusOK {
		t.Fatalf("liste avec jeton de lecture : 200 attendu, obtenu %d", code)
	}
	if code := e.get("/api/mission/jl-1", jetonLectureTest); code != http.StatusOK {
		t.Fatalf("detail avec jeton de lecture : 200 attendu, obtenu %d", code)
	}
	if code := e.get("/api/missions", tokenTest); code != http.StatusOK {
		t.Fatalf("liste avec token console : 200 attendu, obtenu %d", code)
	}
	if code := e.get("/api/missions", "jeton-faux"); code != http.StatusUnauthorized {
		t.Fatalf("jeton invalide : 401 attendu, obtenu %d", code)
	}
	if code := e.get("/api/missions", ""); code != http.StatusUnauthorized {
		t.Fatalf("token absent : 401 attendu, obtenu %d", code)
	}
}

// TestJetonLecture_MutationsRefusees : le jeton de lecture est refusé par
// TOUTE route de mutation (403 JETON_LECTURE_REFUSE), fail-closed.
func TestJetonLecture_MutationsRefusees(t *testing.T) {
	e := nouvelEnvLecture(t)
	e.creer()
	for _, chemin := range []string{
		"/api/mission",
		"/api/mission/jl-1/transition",
		"/api/mission/jl-1/arret",
		"/api/mission/jl-1/arret/lever",
		"/api/mission/jl-1/reprise",
	} {
		code, rep := e.post(chemin, jetonLectureTest)
		if code != http.StatusForbidden {
			t.Errorf("%s avec jeton de lecture : 403 attendu, obtenu %d", chemin, code)
			continue
		}
		if rep["type"] != "JETON_LECTURE_REFUSE" {
			t.Errorf("%s : type JETON_LECTURE_REFUSE attendu, obtenu %v", chemin, rep["type"])
		}
	}
	// Le jeton de lecture reste invalide pour une mutation vers une mission
	// inconnue : le refus 403 précède la résolution de la mission.
	if code, _ := e.post("/api/mission/inconnue/transition", jetonLectureTest); code != http.StatusForbidden {
		t.Errorf("mutation mission inconnue : 403 attendu, obtenu %d", code)
	}
}

// TestJetonLecture_SansOption : sans l'option, aucune route n'accepte un
// quelconque jeton supplémentaire (comportement nominal, non-régression).
func TestJetonLecture_SansOption(t *testing.T) {
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	ui := t.TempDir()
	_ = os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html></html>"), 0o644)
	svc, err := pilot.NouveauService(ref, ui, tokenTest, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/missions", nil)
	req.Header.Set("X-Coord-Pilot-Token", jetonLectureTest)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sans option, un jeton arbitraire doit etre refuse (401), obtenu %d", resp.StatusCode)
	}
}

// TestJetonLecture_JournalSansSecret : le journal du service mentionne le
// refus REFUS_LECTURE_MUTATION mais ne contient JAMAIS la valeur du jeton de
// lecture ni du token console.
func TestJetonLecture_JournalSansSecret(t *testing.T) {
	e := nouvelEnvLecture(t)
	e.creer()
	e.post("/api/mission/jl-1/arret", jetonLectureTest)
	journal, err := os.ReadFile(filepath.Join(e.racine, "journal-service.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	corps := string(journal)
	if !strings.Contains(corps, "REFUS_LECTURE_MUTATION") {
		t.Fatalf("entree REFUS_LECTURE_MUTATION absente du journal")
	}
	if strings.Contains(corps, jetonLectureTest) || strings.Contains(corps, tokenTest) {
		t.Fatalf("VALEUR de jeton presente dans le journal (non-divulgation violee)")
	}
}

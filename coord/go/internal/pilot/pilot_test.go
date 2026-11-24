// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package pilot_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/missions"
	"coordengine/internal/pilot"
)

const tokenTest = "token-local-pour-les-tests"

const rev40 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const emp64 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const empPurge = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

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
	if err := os.WriteFile(filepath.Join(ui, "index.html"),
		[]byte("<!doctype html><html><body>COORD Pilot UI</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := pilot.NouveauService(ref, ui, tokenTest, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, racine: racine}
}

// appel executes one API request and decodes the JSON response.
func (e *env) appel(methode, chemin, token string, corps any) (int, map[string]any) {
	e.t.Helper()
	var body io.Reader
	if corps != nil {
		raw, err := json.Marshal(corps)
		if err != nil {
			e.t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(methode, e.srv.URL+chemin, body)
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
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *env) creerMission(issue string, plafondsAppels map[string]int) (int, map[string]any) {
	e.t.Helper()
	return e.appel("POST", "/api/mission", tokenTest, map[string]any{
		"issue":       issue,
		"adaptateurs": []string{"mistral-simule", "openai-simule"},
		"roles": map[string][]string{
			"EXECUTANT":      {"executer", "soumettre", "acker"},
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		"executant_id": "exec-sim",
		"recepteur_id": "recep-sim",
		"plafonds":     map[string]any{"appels": plafondsAppels},
	})
}

func (e *env) transition(issue, cible, acteur string, donnees map[string]any) (int, map[string]any) {
	e.t.Helper()
	corps := map[string]any{"cible": cible, "acteur": acteur}
	if donnees != nil {
		corps["donnees"] = donnees
	}
	return e.appel("POST", "/api/mission/"+issue+"/transition", tokenTest, corps)
}

func (e *env) journalMissionContient(issue, attendu string) bool {
	e.t.Helper()
	raw, err := os.ReadFile(filepath.Join(e.racine, issue, "journal.jsonl"))
	if err != nil {
		e.t.Fatalf("journal manquant pour %s : %v", issue, err)
	}
	return strings.Contains(string(raw), attendu)
}

func (e *env) journalServiceContient(attendu string) bool {
	e.t.Helper()
	raw, err := os.ReadFile(filepath.Join(e.racine, "journal-service.jsonl"))
	if err != nil {
		e.t.Fatalf("journal du service manquant : %v", err)
	}
	return strings.Contains(string(raw), attendu)
}

func etatMission(e *env, issue string) string {
	e.t.Helper()
	_, detail := e.appel("GET", "/api/mission/"+issue, tokenTest, nil)
	enr := detail["enregistrement"].(map[string]any)
	return enr["Mission"].(map[string]any)["State"].(string)
}

// ---- S1/S2 : token local ----

func TestS1SansTokenRefus401EtJournal(t *testing.T) {
	e := nouvelEnv(t)
	code, rep := e.appel("GET", "/api/missions", "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("401 attendu sans token, obtenu %d", code)
	}
	if _, ok := rep["trace"]; !ok {
		t.Fatal("chaque refus DOIT avoir un identifiant de trace")
	}
	if !e.journalServiceContient("REFUS_AUTH") {
		t.Fatal("le refus d'authentification DOIT être journalisé (S1)")
	}
}

func TestS2TokenInvalideRefuse401(t *testing.T) {
	e := nouvelEnv(t)
	code, _ := e.appel("GET", "/api/missions", "mauvais-token", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("401 attendu avec token invalide, obtenu %d", code)
	}
	if !e.journalServiceContient("invalide ou perime") {
		t.Fatal("motif journalisé attendu (S2)")
	}
}

// ---- S3 : loopback uniquement ----

func TestS3EcouteNonLoopbackRefuseeAuDemarrage(t *testing.T) {
	racine := t.TempDir()
	ref, _ := missions.Ouvrir(racine)
	for _, adresse := range []string{"0.0.0.0:8080", "192.168.1.10:8080", "example.com:443", ":8080"} {
		if _, err := pilot.NouveauService(ref, "", tokenTest, adresse); err == nil {
			t.Fatalf("adresse %s DOIT être refusée au démarrage (S3, fail closed)", adresse)
		}
	}
	if _, err := pilot.NouveauService(ref, "", tokenTest, "127.0.0.1:0"); err != nil {
		t.Fatalf("loopback DOIT être accepté : %v", err)
	}
	if _, err := pilot.NouveauService(ref, "", "", "127.0.0.1:0"); err == nil {
		t.Fatal("token vide DOIT être refusé")
	}
}

// ---- S5/R2 : habilitations fail-closed au démarrage ----

func TestS5EtR2HabilitationMalsaineRefuseDemarrage(t *testing.T) {
	racine := t.TempDir()
	ref, _ := missions.Ouvrir(racine)
	if _, err := pilot.NouveauService(ref, "", tokenTest, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	dossier := filepath.Join(racine, "-x")
	if err := os.MkdirAll(dossier, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossier, "mission.json"),
		[]byte(`{"Issue":"#x","Roles":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ref2, _ := missions.Ouvrir(racine)
	if _, err := pilot.NouveauService(ref2, "", tokenTest, "127.0.0.1:0"); err == nil {
		t.Fatal("démarrage DOIT être refusé si une habilitation est vide ou mal formée (R2/S5, N-05)")
	}
}

// ---- E2E1 : mission complète de bout en bout ----

func TestE2E1MissionCompleteNominal(t *testing.T) {
	e := nouvelEnv(t)
	if code, _ := e.creerMission("e2e-1", map[string]int{"mistral-simule": 10, "openai-simule": 10}); code != http.StatusCreated {
		t.Fatal("création attendue en 201")
	}

	code, rep := e.transition("e2e-1", "EXECUTION", "exec-sim", map[string]any{"mandat": "produire le plan"})
	if code != http.StatusOK {
		t.Fatalf("EXECUTION attendue, obtenu %d : %v", code, rep)
	}
	appel1, ok := rep["appel_adaptateur"].(map[string]any)
	if !ok || appel1["simule"] != true {
		t.Fatal("tout appel DOIT être explicitement marqué simulé")
	}
	if appel1["jetons"].(float64) <= 0 {
		t.Fatal("des jetons simulés DOIVENT être déclarés par l'adaptateur")
	}

	code, rep = e.transition("e2e-1", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusOK {
		t.Fatalf("soumission attendue, obtenu %d : %v", code, rep)
	}

	code, rep = e.transition("e2e-1", "ACCEPTEE", "recep-sim",
		map[string]any{"verdict_id": "v-001", "verdict_url": "http://127.0.0.1/verdict-v-001"})
	if code != http.StatusOK {
		t.Fatalf("verdict ACCEPTEE attendu, obtenu %d : %v", code, rep)
	}
	if _, avecAppel := rep["appel_adaptateur"]; avecAppel {
		t.Fatal("recevoir n'est pas un appel d'adaptateur : aucun appel attendu")
	}

	// ACK sur empreinte INCHANGÉE (N-13 n'est pas déclenché).
	code, rep = e.transition("e2e-1", "ACK_RECU", "exec-sim",
		map[string]any{"empreinte_courante": emp64, "revision_courante": rev40})
	if code != http.StatusOK {
		t.Fatalf("ACK attendu, obtenu %d : %v", code, rep)
	}

	// Clôture : autorité humaine UNIQUEMENT (N-14).
	code, rep = e.transition("e2e-1", "CLOTUREE", pilot.ActeurPiloteLocal, nil)
	if code != http.StatusOK {
		t.Fatalf("clôture par le pilote attendue, obtenu %d : %v", code, rep)
	}
	if rep["etat"] != "CLOTUREE" {
		t.Fatal("état final CLOTUREE attendu")
	}
	if !e.journalMissionContient("e2e-1", "TRANSITION") {
		t.Fatal("le journal de mission DOIT tracer chaque transition")
	}
}

// ---- R1 : transitions hors table fermée refusées avec motif ----

func TestR1TransitionHorsTableRefusee(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("r1", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	// PLANIFIEE -> CLOTUREE n'existe pas dans la table fermée.
	code, rep := e.transition("r1", "CLOTUREE", pilot.ActeurPiloteLocal, nil)
	if code != http.StatusConflict {
		t.Fatalf("refus attendu, obtenu %d", code)
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "transition interdite") {
		t.Fatalf("motif explicite attendu, obtenu %v", rep["motif"])
	}
	if !e.journalMissionContient("r1", "REFUS_TRANSITION") {
		t.Fatal("le refus DOIT être journalisé (R1)")
	}
	// 404 : mission inconnue.
	code, _ = e.transition("inexistante", "EXECUTION", "exec-sim", nil)
	if code != http.StatusNotFound {
		t.Fatal("404 attendu pour mission inconnue")
	}
}

// ---- R3 : budget bloqué AVANT tout appel ----

func TestR3PlafondBloqueAvantAppel(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("r3", map[string]int{"mistral-simule": 1, "openai-simule": 10})
	code, _ := e.transition("r3", "EXECUTION", "exec-sim", nil)
	if code != http.StatusOK {
		t.Fatal("premier appel autorisé sous le plafond")
	}
	// 2e appel d'adaptateur : plafond d'appels mistral-simule = 1 → refus AVANT l'appel.
	code, rep := e.transition("r3", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusConflict {
		t.Fatalf("refus budget attendu, obtenu %d", code)
	}
	if rep["type"] != "BUDGET_DEPASSE" {
		t.Fatalf("type BUDGET_DEPASSE attendu, obtenu %v", rep["type"])
	}
	if !e.journalMissionContient("r3", "REFUS_BUDGET") {
		t.Fatal("refus budgétaire DOIT être journalisé")
	}
	// État machine inchangé, aucun appel consommé.
	if etatMission(e, "r3") != "EXECUTION" {
		t.Fatal("état DOIT rester EXECUTION après refus budgétaire (aucune action externe)")
	}
}

// ---- R4 : arrêt humain effectif, état intact, reprise après levée ----

func TestR4ArretPiloteEffectif(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("r4", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	_, _ = e.transition("r4", "EXECUTION", "exec-sim", nil)

	code, rep := e.appel("POST", "/api/mission/r4/arret", tokenTest,
		map[string]any{"motif": "vérification humaine nécessaire"})
	if code != http.StatusOK || rep["arret"] != true {
		t.Fatal("arrêt demandé attendu")
	}
	if !e.journalMissionContient("r4", "ARRET_PILOTE") {
		t.Fatal("journal ARRET_PILOTE distinct attendu")
	}

	// Toute transition est refusée avec motif explicite, état intact.
	code, rep = e.transition("r4", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusConflict || rep["type"] != "ARRET_PILOTE_ACTIF" {
		t.Fatalf("refus d'arrêt attendu, obtenu %d %v", code, rep)
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "arrêt demandé par le pilote") {
		t.Fatalf("motif explicite attendu, obtenu %v", rep["motif"])
	}
	if !e.journalMissionContient("r4", "REFUS_TRANSITION_ARRET") {
		t.Fatal("le refus pendant l'arrêt DOIT être journalisé (typé ARRET)")
	}
	if etatMission(e, "r4") != "EXECUTION" {
		t.Fatal("état machine DOIT rester intact pendant l'arrêt")
	}

	// Levée : le cycle reprend.
	code, rep = e.appel("POST", "/api/mission/r4/arret/lever", tokenTest,
		map[string]any{"motif": "reprise après vérification"})
	if code != http.StatusOK || rep["arret"] != false {
		t.Fatal("levée d'arrêt attendue")
	}
	code, _ = e.transition("r4", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusOK {
		t.Fatal("après levée, la transition DOIT passer")
	}
}

// ---- R5 : divergence, arbitrage github_fait_foi, reprise conditionnée au head ----

func TestR5DivergenceArbitrageReprise(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("r5", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	_, _ = e.transition("r5", "EXECUTION", "exec-sim", nil)

	_, detail := e.appel("GET", "/api/mission/r5", tokenTest, nil)
	empreinte := detail["empreinte"].(string)
	if len(empreinte) != 64 {
		t.Fatalf("empreinte sha256 attendue, obtenu %v", empreinte)
	}

	// Copie de connaissance divergente (sha_head faux, état faux).
	code, rep := e.appel("POST", "/api/mission/r5/reprise", tokenTest, map[string]any{
		"copie_knowledge": map[string]any{
			"sha_head":         "deadbeef" + strings.Repeat("0", 56),
			"prochaine_action": "cloturer",
			"etat":             "ACCEPTEE",
		},
		"sha_head_attendu": empreinte,
	})
	if code != http.StatusOK {
		t.Fatalf("reprise attendue, obtenu %d : %v", code, rep)
	}
	if rep["elu"] != "github" {
		t.Fatal("arbitrage N-16 : github DOIT faire foi en cas de divergence")
	}
	if rep["regle"] != "github_fait_foi" {
		t.Fatal("règle normative github_fait_foi attendue")
	}
	champs := fmt.Sprint(rep["champs_divergents"])
	if !strings.Contains(champs, "sha_head") {
		t.Fatal("les champs divergents DOIVENT être listés")
	}
	if rep["prochaine_action"] != "soumettre" {
		t.Fatal("prochaine action normative attendue depuis l'état EXECUTION")
	}
	if !e.journalMissionContient("r5", "ARBITRATION") || !e.journalMissionContient("r5", "DIVERGENCE") {
		t.Fatal("arbitrage et divergence DOIVENT être journalisés avec les empreintes des deux copies")
	}

	// N-17 : sha_head divergent → reprise refusée, fail closed.
	code, rep = e.appel("POST", "/api/mission/r5/reprise", tokenTest, map[string]any{
		"copie_knowledge":  map[string]any{"sha_head": empreinte, "prochaine_action": "soumettre"},
		"sha_head_attendu": "beef" + strings.Repeat("0", 60),
	})
	if code != http.StatusConflict {
		t.Fatal("reprise DOIT être refusée si le head vérifié ne correspond pas (N-17)")
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "sha_head divergent") {
		t.Fatal("motif N-17 attendu")
	}
	// Fail closed : pas de sha_head attendu fourni.
	code, _ = e.appel("POST", "/api/mission/r5/reprise", tokenTest,
		map[string]any{"copie_knowledge": map[string]any{}})
	if code != http.StatusBadRequest {
		t.Fatal("sha_head attendu DOIT être obligatoire (N-17, fail closed)")
	}
}

// ---- R6 : verdict périmé refusé (N-13) ----

func TestR6VerdictPerimeRefuse(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("r6", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	_, _ = e.transition("r6", "EXECUTION", "exec-sim", nil)
	_, _ = e.transition("r6", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	_, _ = e.transition("r6", "EN_CORRECTION", "recep-sim",
		map[string]any{"verdict_id": "v-002", "verdict_url": "http://127.0.0.1/v2"})
	_, _ = e.transition("r6", "EXECUTION", "exec-sim", nil)
	_, _ = e.transition("r6", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": strings.Repeat("d", 40), "empreinte_preuves": empPurge})
	_, _ = e.transition("r6", "ACCEPTEE", "recep-sim",
		map[string]any{"verdict_id": "v-003", "verdict_url": "http://127.0.0.1/v3"})

	// ACK sur des preuves MODIFIÉES (l'exécutant tente d'acker un verdict périmé).
	code, rep := e.transition("r6", "ACK_RECU", "exec-sim",
		map[string]any{"empreinte_courante": emp64, "revision_courante": rev40})
	if code != http.StatusConflict {
		t.Fatalf("refus N-13 attendu, obtenu %d", code)
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "verdict perime") {
		t.Fatalf("motif verdict périmé attendu, obtenu %v", rep["motif"])
	}
	if !e.journalMissionContient("r6", "REFUS_TRANSITION") {
		t.Fatal("refus N-13 DOIT être journalisé")
	}
	if etatMission(e, "r6") != "ACCEPTEE" {
		t.Fatal("état DOIT rester ACCEPTEE après refus N-13")
	}
	// L'ACK sur les preuves COURANTES passe (distingue refus N-13 d'un blocage).
	code, _ = e.transition("r6", "ACK_RECU", "exec-sim",
		map[string]any{"empreinte_courante": empPurge, "revision_courante": strings.Repeat("d", 40)})
	if code != http.StatusOK {
		t.Fatal("l'ACK sur les preuves courantes DOIT passer")
	}
}

// ---- E2E2 : séparation stricte EXECUTANT / RECEPTIONNAIRE (N-05) ----

func TestE2E2SeparationN05ExecutantNePeutPasRecevoir(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("auto", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	_, _ = e.transition("auto", "EXECUTION", "exec-sim", nil)
	_, _ = e.transition("auto", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	// L'exécutant tente de rendre lui-même le verdict : refusé par le rôle,
	// résolu côté serveur depuis l'enregistrement (N-05, séparation stricte ;
	// le refus moteur « auto-reception interdite » reste en défense en
	// profondeur pour les configurations non liées, couvert par la démo
	// 19 contrôles et les 22 vecteurs de conformité).
	code, rep := e.transition("auto", "ACCEPTEE", "exec-sim",
		map[string]any{"verdict_id": "v-x", "verdict_url": "http://127.0.0.1/x"})
	if code != http.StatusConflict {
		t.Fatal("l'exécutant ne DOIT pas pouvoir recevoir (N-05)")
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "attendu: RECEPTIONNAIRE") {
		t.Fatalf("motif de séparation des rôles attendu, obtenu %v", rep["motif"])
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "RECEPTIONNAIRE") {
		t.Fatalf("le motif DOIT nommer le rôle attendu, obtenu %v", rep["motif"])
	}
	if etatMission(e, "auto") != "DECISION_ATTENDUE" {
		t.Fatal("état DOIT rester DECISION_ATTENDUE après refus N-05")
	}
	if !e.journalMissionContient("auto", "REFUS_TRANSITION") {
		t.Fatal("refus N-05 DOIT être journalisé")
	}
}

// ---- E2E3 : S4, rôle PILOTE non dérivable d'un corps de requête ----

func TestE2E3RolePILOTENonDerivableDuCorps(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("s4", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	// Acteur inconnu :
	code, rep := e.appel("POST", "/api/mission/s4/transition", tokenTest, map[string]any{
		"cible": "EXECUTION", "acteur": "intrus-inconnu", "role": "PILOTE",
	})
	if code != http.StatusForbidden {
		t.Fatal("acteur non enregistré DOIT être refusé (403)")
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "S4") {
		t.Fatalf("motif renvoyant à S4 attendu, obtenu %v", rep["motif"])
	}
	// Forgery : exécutant qui déclare « role: PILOTE » dans son corps.
	code, _ = e.appel("POST", "/api/mission/s4/transition", tokenTest, map[string]any{
		"cible": "EXECUTION", "acteur": "exec-sim", "role": "PILOTE",
	})
	if code != http.StatusOK {
		t.Fatal("l'exécutant peut exécuter (le role déclaré est IGNORÉ, résolu côté serveur)")
	}
	_, _ = e.transition("s4", "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	_, _ = e.transition("s4", "ACCEPTEE", "recep-sim",
		map[string]any{"verdict_id": "v-9", "verdict_url": "http://127.0.0.1/9"})
	_, _ = e.transition("s4", "ACK_RECU", "exec-sim",
		map[string]any{"empreinte_courante": emp64, "revision_courante": rev40})
	// Tentative de clôture par exec-sim DECLARE PILOTE : refusée.
	code, rep = e.appel("POST", "/api/mission/s4/transition", tokenTest, map[string]any{
		"cible": "CLOTUREE", "acteur": "exec-sim", "role": "PILOTE",
	})
	if code != http.StatusConflict {
		t.Fatal("le rôle déclaré dans le corps NE DOIT PAS accorder le rôle PILOTE (S4)")
	}
	if !strings.Contains(fmt.Sprint(rep["motif"]), "role non autorise") {
		t.Fatalf("refus du moteur attendu, obtenu %v", rep["motif"])
	}
	// La clôture passe UNIQUEMENT par la session locale.
	code, _ = e.transition("s4", "CLOTUREE", pilot.ActeurPiloteLocal, nil)
	if code != http.StatusOK {
		t.Fatal("clôture par la session locale attendue")
	}
}

// ---- E2E4 : persistance à travers un « redémarrage » ----

func TestE2E4RepriseApresRedemarrage(t *testing.T) {
	e := nouvelEnv(t)
	_, _ = e.creerMission("reprise", map[string]int{"mistral-simule": 10, "openai-simule": 10})
	_, _ = e.transition("reprise", "EXECUTION", "exec-sim", nil)

	// « Redémarrage » : le référentiel est réouvert sur le MÊME dossier.
	ref, err := missions.Ouvrir(e.racine)
	if err != nil {
		t.Fatal(err)
	}
	if err := ref.EstConforme(); err != nil {
		t.Fatalf("après redémarrage, le référentiel DOIT être conforme : %v", err)
	}
	// L'état est durable : la mission est lisible telle quelle.
	if etatMission(e, "reprise") != "EXECUTION" {
		t.Fatal("persistance N-15 : l'état doit survivre au service")
	}
	// Arrêt demandé PUIS redémarrage : l'arrêt doit survivre aussi (état persisté).
	_, _ = e.appel("POST", "/api/mission/reprise/arret", tokenTest, map[string]any{"motif": "pause"})
	code, _ := e.transition("reprise", "DECISION_ATTENDUE", "exec-sim", nil)
	if code != http.StatusConflict {
		t.Fatal("l'arrêt DOIT survivre aussi (état persisté)")
	}
}

// ---- Santé et UI statique ----

func TestHealthEtUI(t *testing.T) {
	e := nouvelEnv(t)
	code, rep := e.appel("GET", "/health", "", nil)
	if code != http.StatusOK || rep["adaptateurs"] != "SIMULES" {
		t.Fatal("/health DOIT rappeler la simulation sans token")
	}
	resp, err := e.srv.Client().Get(e.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "COORD Pilot UI") {
		t.Fatal("l'interface DOIT être servie par le service local")
	}
}

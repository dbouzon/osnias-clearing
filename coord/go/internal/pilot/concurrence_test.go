// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Reproductions expérimentales des anomalies C1..C6 relevées par la revue
// statique de réception complémentaire sur c4c4b4c6 (issue #27, PR #28),
// converties en invariants permanents après correction :
//
//   - C1 : Obtenir puis Sauver séparés ne sérialisent PAS la transaction :
//     deux requêtes concurrentes peuvent se perdre leurs mises à jour et
//     laisser passer plus d'appels que le plafond (R3) ;
//   - C2 : l'appel d'adaptateur avait lieu AVANT la validation normative :
//     une transition interdite consommait du budget et passait un appel ;
//   - C3 : une transition concurrente pouvait écraser un arrêt humain ;
//   - C4 : la levée d'arrêt permettait de reprendre sans reprise normative ;
//   - C5 : les erreurs de journal étaient ignorées (masquées) ;
//   - C6 : des identifiants distincts pouvaient collisionner vers un même
//     dossier (nomDir non injectif).
//
// Les tests REPRO_ documentent l'entrelacement fautif tel qu'exécuté par les
// gestionnaires du service (mêmes primitives Obtenir/Sauver) ; les tests
// INVARIANT_ verrouillent le comportement corrigé, y compris en concurrence
// réelle sous `go test -race`.
package pilot_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"coordengine/internal/arret"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/pilot"
	"coordengine/internal/policy"
)

// configTest : configuration d'habilitations équivalente à celle de
// creerMission (mêmes adaptateurs simulés, mêmes rôles).
func configTest() ([]string, map[string][]string, []policy.Identity) {
	adaptateurs := []string{"mistral-simule", "openai-simule"}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	identites := []policy.Identity{
		{ID: "exec-sim", Role: "EXECUTANT", QualifiedBy: "mistral-simule"},
		{ID: "recep-sim", Role: "RECEPTIONNAIRE", QualifiedBy: "openai-simule"},
		{ID: pilot.ActeurPiloteLocal, Role: "PILOTE", QualifiedBy: "mistral-simule"},
	}
	return adaptateurs, roles, identites
}

func plafondsTest() budget.Plafonds {
	return budget.Plafonds{Appels: map[string]int{"mistral-simule": 10, "openai-simule": 10}}
}

// lireJournalMission : contenu brut du journal de mission.
func lireJournalMission(t *testing.T, racine, issue string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(racine, issue, "journal.jsonl"))
	if err != nil {
		t.Fatalf("journal manquant pour %s : %v", issue, err)
	}
	return string(raw)
}

// occurrences : nombre de lignes contenant un marqueur.
func occurrences(texte, marqueur string) int {
	n := 0
	for _, l := range strings.Split(texte, "\n") {
		if strings.Contains(l, marqueur) {
			n++
		}
	}
	return n
}

// appelsConsommes : compteur d'appels consommés d'un adaptateur, lu par
// l'API (0 si aucune consommation : map vide, clés absentes).
func appelsConsommes(e *env, issue, adaptateur string) int {
	e.t.Helper()
	_, detail := e.appel("GET", "/api/mission/"+issue, tokenTest, nil)
	etat, _ := detail["enregistrement"].(map[string]any)["Budget"].(map[string]any)["etat"].(map[string]any)
	if etat == nil {
		e.t.Fatal("état budgétaire illisible dans la réponse")
	}
	cons, _ := etat["appels_consommes"].(map[string]any)
	if cons == nil {
		return 0
	}
	v, _ := cons[adaptateur].(float64)
	return int(v)
}

// ---- C1 : la transaction doit être sérialisée (plafond respecté) ----

// TestC1InvariantPlafondSousConcurrence : N soumissions concurrentes avec un
// plafond de DEUX appels ne doivent compter QUE les appels réellement passés
// (aucune mise à jour perdue) et jamais dépasser le plafond.
func TestC1InvariantPlafondSousConcurrence(t *testing.T) {
	e := nouvelEnv(t)
	issue := "c1"
	if code, _ := e.creerMission(issue, map[string]int{"mistral-simule": 2}); code != http.StatusCreated {
		t.Fatal("mission c1 attendue en 201")
	}
	if code, _ := e.transition(issue, "EXECUTION", "exec-sim", nil); code != http.StatusOK {
		t.Fatal("EXECUTION attendue (1er appel du plafond de 2)")
	}

	const N = 12
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.transition(issue, "DECISION_ATTENDUE", "exec-sim",
				map[string]any{"revision": rev40, "empreinte_preuves": emp64})
		}()
	}
	wg.Wait()

	appelsPasses := occurrences(lireJournalMission(t, e.racine, issue), `"APPEL_SIMULE"`)
	comptes := appelsConsommes(e, issue, "mistral-simule")

	// CHAQUE appel passé doit être comptabilisé : sinon perte de mise à jour (C1).
	if comptes < appelsPasses {
		t.Fatalf("C1 : %d appel(s) passé(s) mais %d comptabilisé(s) : perte de mise à jour",
			appelsPasses, comptes)
	}
	// Le plafond de DEUX appels ne doit jamais être dépassé.
	if comptes > 2 {
		t.Fatalf("C1/R3 : plafond d'appels violé : %d consommés pour 2 autorisés", comptes)
	}
}

// TestC1TransactionSerieArrEtBudget : l'entrelacement déterministe qui
// écrasait l'arrêt du pilote (documenté dans la trace de reproduction
// pré-correction : Obtenir / Sauver séparés, cf. preuves-mvp) devient
// impossible avec Transaction : deux transactions concurrentes sur la même
// mission (B : arrêt humain ; A : consommation budgétaire) aboutissent
// SANS perte de mise à jour, l'arrêt reste durable (C1 + C3).
func TestC1TransactionSerieArrEtBudget(t *testing.T) {
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	adaptateurs, roles, identites := configTest()
	if _, err := ref.Creer("c1r", adaptateurs, roles, identites, plafondsTest()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // B : arrêt humain, via transaction.
		defer wg.Done()
		journal := missions.JournalPath(racine, "c1r")
		errB := ref.Transaction("c1r", func(ctx *missions.ContexteMission) error {
			return arret.Demander(&ctx.Enr.Arret, "pause humaine", pilot.ActeurPiloteLocal, journal)
		}, nil)
		if errB != nil {
			t.Error("transaction d'arrêt : ", errB)
		}
	}()
	go func() { // A : consommation budgétaire, via transaction.
		defer wg.Done()
		errA := ref.Transaction("c1r", func(ctx *missions.ContexteMission) error {
			ctx.Enr.Budget.Consommer("mistral-simule", 10)
			return nil
		}, nil)
		if errA != nil {
			t.Error("transaction de budget : ", errA)
		}
	}()
	wg.Wait()

	final, err := ref.Obtenir("c1r")
	if err != nil {
		t.Fatal(err)
	}
	if !arret.Actif(&final.Arret) {
		t.Fatal("C3 : l'arrêt du pilote DOIT être durable : aucune transaction " +
			"concurrente ne peut l'écraser")
	}
	if got := final.Budget.Etat.AppelsConsommes["mistral-simule"]; got != 1 {
		t.Fatalf("C1 : la consommation budgétaire DOIT être conservée : %d attendu, %d trouvé", 1, got)
	}
}

// TestC3InvariantArretDurableSousConcurrence : orage de requêtes (transitions
// + arrêt humain) : à l'issue, l'arrêt demandé par le pilote DOIT être actif
// (aucune requête concurrente ne peut l'écraser).
func TestC3InvariantArretDurableSousConcurrence(t *testing.T) {
	e := nouvelEnv(t)
	issue := "c3"
	if code, _ := e.creerMission(issue, map[string]int{"mistral-simule": 50}); code != http.StatusCreated {
		t.Fatal("mission c3 attendue en 201")
	}
	_, _ = e.transition(issue, "EXECUTION", "exec-sim", nil)

	var wg sync.WaitGroup
	codes := make([]int, 8)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = e.appel("POST", "/api/mission/"+issue+"/arret", tokenTest,
			map[string]any{"motif": "arrêt orage"})
	}()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.transition(issue, "DECISION_ATTENDUE", "exec-sim",
				map[string]any{"revision": rev40, "empreinte_preuves": emp64})
		}(i)
	}
	wg.Wait()

	succes := 0
	for _, c := range codes {
		if c == http.StatusOK {
			succes++
		}
	}
	if succes > 1 {
		t.Fatalf("C3 : au plus UNE soumission peut aboutir (les autres sont refusées "+
			"par la table d'états ou l'arrêt), %d réussies", succes)
	}
	_, detail := e.appel("GET", "/api/mission/"+issue, tokenTest, nil)
	enr := detail["enregistrement"].(map[string]any)
	if enr["Arret"].(map[string]any)["actif"] != true {
		t.Fatal("C3 : l'arrêt du pilote DOIT rester actif après l'orage de requêtes")
	}
	if etat := etatMission(e, issue); etat != "EXECUTION" && etat != "DECISION_ATTENDUE" {
		t.Fatalf("C3 : état incohérent après l'orage : %s", etat)
	}
}

// ---- C2 : aucun appel (ni consommation) pour une transition interdite ----

// TestC2TransitionInterditeSansAppelNiConsommation : depuis PLANIFIEE, la
// soumission est interdite par la table fermée ; AUCUN appel, AUCUNE
// consommation ne doit avoir lieu. Idem pour un acteur non autorisé.
func TestC2TransitionInterditeSansAppelNiConsommation(t *testing.T) {
	e := nouvelEnv(t)
	issue := "c2"
	if code, _ := e.creerMission(issue, map[string]int{"mistral-simule": 5, "openai-simule": 5}); code != http.StatusCreated {
		t.Fatal("mission c2 attendue en 201")
	}

	// Transition interdite par la table (PLANIFIEE -> DECISION_ATTENDUE).
	code, rep := e.transition(issue, "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusConflict {
		t.Fatalf("transition interdite attendue en 409, obtenu %d : %v", code, rep)
	}
	if etatMission(e, issue) != "PLANIFIEE" {
		t.Fatal("l'état DOIT rester intact")
	}
	if n := occurrences(lireJournalMission(t, e.racine, issue), `"APPEL_SIMULE"`); n != 0 {
		t.Fatalf("C2 : %d appel(s) passé(s) pour une transition interdite", n)
	}
	if c := appelsConsommes(e, issue, "mistral-simule"); c != 0 {
		t.Fatalf("C2 : %d consommation(s) pour une transition interdite", c)
	}

	// Acteur non autorisé : le RÉCEPTIONNAIRE ne peut pas démarrer l'exécution.
	code, rep = e.transition(issue, "EXECUTION", "recep-sim", nil)
	if code != http.StatusConflict {
		t.Fatalf("refus de rôle attendu en 409, obtenu %d : %v", code, rep)
	}
	if n := occurrences(lireJournalMission(t, e.racine, issue), `"APPEL_SIMULE"`); n != 0 {
		t.Fatalf("C2 : %d appel(s) passé(s) pour un acteur non autorisé", n)
	}
	if c := appelsConsommes(e, issue, "openai-simule"); c != 0 {
		t.Fatalf("C2 : %d consommation(s) pour un acteur non autorisé", c)
	}
}

// ---- C4 : la levée d'arrêt exige une reprise normative préalable ----

// TestC4LeveeExigeRepriseNormative : après la levée d'un arrêt, toute
// transition DOIT être refusée jusqu'à la reprise normative (N-16/N-17) ;
// la reprise réussie débloque la mission.
func TestC4LeveeExigeRepriseNormative(t *testing.T) {
	e := nouvelEnv(t)
	issue := "c4"
	if code, _ := e.creerMission(issue, map[string]int{"mistral-simule": 10, "openai-simule": 10}); code != http.StatusCreated {
		t.Fatal("mission c4 attendue en 201")
	}
	if code, _ := e.transition(issue, "EXECUTION", "exec-sim", nil); code != http.StatusOK {
		t.Fatal("EXECUTION attendue")
	}
	if code, _ := e.appel("POST", "/api/mission/"+issue+"/arret", tokenTest,
		map[string]any{"motif": "vérification"}); code != http.StatusOK {
		t.Fatal("arrêt attendu en 200")
	}
	if code, _ := e.appel("POST", "/api/mission/"+issue+"/arret/lever", tokenTest,
		map[string]any{"motif": "reprise contrôlée"}); code != http.StatusOK {
		t.Fatal("levée attendue en 200")
	}

	// Sans reprise normative : la transition DOIT être refusée (C4).
	code, rep := e.transition(issue, "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusConflict || fmt.Sprint(rep["type"]) != "REPRISE_ATTENDUE" {
		t.Fatalf("C4 : reprise normative attendue avant toute transition, obtenu %d %v", code, rep)
	}

	// Reprise normative (dispositif N-16/N-17) : débloque la mission.
	_, detail := e.appel("GET", "/api/mission/"+issue, tokenTest, nil)
	code, _ = e.appel("POST", "/api/mission/"+issue+"/reprise", tokenTest, map[string]any{
		"copie_knowledge": map[string]any{
			"sha_head":         detail["empreinte"],
			"prochaine_action": "soumettre",
			"etat":             "EXECUTION",
		},
		"sha_head_attendu": detail["empreinte"],
	})
	if code != http.StatusOK {
		t.Fatalf("reprise normative attendue en 200, obtenu %d", code)
	}

	code, _ = e.transition(issue, "DECISION_ATTENDUE", "exec-sim",
		map[string]any{"revision": rev40, "empreinte_preuves": emp64})
	if code != http.StatusOK {
		t.Fatal("après reprise normative, la transition DOIT passer")
	}
}

// ---- C5 : aucune erreur de journal n'est masquée ----

// TestC5ErreurJournalExplicite : si le journal de mission devient
// indisponible, l'erreur DOIT être explicite et typée (jamais ignorée) et
// aucun état ne doit être persisté (fail closed).
func TestC5ErreurJournalExplicite(t *testing.T) {
	e := nouvelEnv(t)
	issue := "c5"
	if code, _ := e.creerMission(issue, map[string]int{"mistral-simule": 10}); code != http.StatusCreated {
		t.Fatal("mission c5 attendue en 201")
	}
	// Sabotage : un RÉPERTOIRE à la place du fichier journal rend toute
	// écriture impossible (erreur isolée, reproductible, même pour root).
	journal := filepath.Join(e.racine, issue, "journal.jsonl")
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0o755); err != nil {
		t.Fatal(err)
	}

	code, rep := e.transition(issue, "EXECUTION", "inconnu", nil)
	if code != http.StatusInternalServerError {
		t.Fatalf("C5 : une erreur de journal DOIT être explicite (500), obtenu %d %v", code, rep)
	}
	if fmt.Sprint(rep["type"]) != "ERREUR_JOURNAL" {
		t.Fatalf("C5 : type ERREUR_JOURNAL attendu, obtenu %v", rep["type"])
	}
	if etatMission(e, issue) != "PLANIFIEE" {
		t.Fatal("aucun état ne DOIT être persisté après une erreur de journal")
	}
}

// ---- C6 : identifiants distincts = dossiers distincts ----

// TestC6IdentifiantsDistinctsDossiersDistincts : nomDir DOIT être injectif :
// des identifiants distincts ne doivent JAMAIS partager un dossier.
func TestC6IdentifiantsDistinctsDossiersDistincts(t *testing.T) {
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	adaptateurs, roles, identites := configTest()
	issues := []string{"a b", "a-b", "a~b", "a~20b", "é", "a-é"}
	for _, issue := range issues {
		if _, err := ref.Creer(issue, adaptateurs, roles, identites, plafondsTest()); err != nil {
			t.Fatalf("C6 : la création de %q DOIT aboutir (identifiant distinct) : %v", issue, err)
		}
	}
	for _, issue := range issues {
		enr, err := ref.Obtenir(issue)
		if err != nil || enr.Issue != issue {
			t.Fatalf("C6 : %q DOIT être relisible avec son identifiant exact : %v", issue, err)
		}
	}
	// Toutes les missions restent distinctes après redémarrage.
	ref2, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ref2.Lister()); n != len(issues) {
		t.Fatalf("C6 : %d missions attendues après réouverture, %d trouvées", len(issues), n)
	}
}

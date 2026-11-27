// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"strings"
	"testing"
)

// TestAnalyserReponseQuestion : QUESTION valide exige un contenu.
func TestAnalyserReponseQuestion(t *testing.T) {
	r, err := analyserReponseTour(reponseQuestion("Quel est le livrable ?"))
	if err != nil || r.Action != ReponseQuestion {
		t.Fatalf("question : %v / %s", err, r.Action)
	}
	if _, err := analyserReponseTour(`{"action":"QUESTION","contenu":"  "}`); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("contenu vide attendu REPONSE_INVALIDE, reçu : %v", err)
	}
}

// TestAnalyserReponseChampInconnu : fail closed, champs inconnus refusés
// (aucune interprétation, D4).
func TestAnalyserReponseChampInconnu(t *testing.T) {
	brute := `{"action":"QUESTION","contenu":"...","champ_inconnu":"x"}`
	if _, err := analyserReponseTour(brute); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("champ inconnu attendu REPONSE_INVALIDE, reçu : %v", err)
	}
}

// TestAnalyserReponseVerdict : VERDICT exige verdict admis + points
// détaillés avec sévérités admises.
func TestAnalyserReponseVerdict(t *testing.T) {
	r, err := analyserReponseTour(reponseVerdict("PLAN_ACCEPTE",
		[2]string{"RESERVE_NON_BLOQUANTE", "documentation à compléter"}))
	if err != nil || r.Action != ReponseVerdict || r.Verdict != "PLAN_ACCEPTE" {
		t.Fatalf("verdict : %v / %+v", err, r)
	}
	if _, err := analyserReponseTour(`{"action":"VERDICT","verdict":"INCONNU","points_detail":[]}`); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("verdict inconnu attendu, reçu : %v", err)
	}
	if _, err := analyserReponseTour(reponseVerdict("A_CORRIGER")); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("points absents attendus REPONSE_INVALIDE, reçu : %v", err)
	}
	if _, err := analyserReponseTour(reponseVerdict("A_CORRIGER",
		[2]string{"FATALE", "sévérité inventée"})); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("sévérité inconnue attendue REPONSE_INVALIDE, reçu : %v", err)
	}
}

// TestAnalyserReponseProposition : PROPOSITION_ORDRE exige l'objet ordre ;
// la FORME de l'ordre n'est PAS validée ici (rejet par evaluerProposition).
func TestAnalyserReponseProposition(t *testing.T) {
	r, err := analyserReponseTour(reponsePropositionOrdre(ordreComplet()))
	if err != nil || r.Action != ReponsePropositionOrdre {
		t.Fatalf("proposition : %v / %s", err, r.Action)
	}
	if _, err := analyserReponseTour(`{"action":"PROPOSITION_ORDRE"}`); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("ordre absent attendu REPONSE_INVALIDE, reçu : %v", err)
	}
	// Un ordre NON CONFORME reste ANALYSABLE (forme) : le rejet déterministe
	// vient d'evaluerProposition, avec ses codes stables.
	brut := `{"action":"PROPOSITION_ORDRE","ordre":{"objectif":"vide"}}`
	if _, err := analyserReponseTour(brut); err != nil {
		t.Fatalf("l'analyse ne rejette PAS la forme de l'ordre : %v", err)
	}
}

// TestAnalyserReponseActionInconnue : action hors ensemble admis = refus.
func TestAnalyserReponseActionInconnue(t *testing.T) {
	if _, err := analyserReponseTour(`{"action":"TRANSMETTRE_TOUT_SUITE"}`); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("action inconnue attendue REPONSE_INVALIDE, reçu : %v", err)
	}
	if _, err := analyserReponseTour("ceci n'est pas du JSON"); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("non-JSON attendu REPONSE_INVALIDE, reçu : %v", err)
	}
}

// TestFenetreDialogue : la fenêtre de contexte est BORNÉE en octets
// (ContexteJetonsMax*4/2), les tours les plus récents sont conservés.
func TestFenetreDialogue(t *testing.T) {
	store, _ := magasinTest(t)
	store.cfg.ContexteJetonsMax = 100 // budget réduit pour le test
	projetDialoguer(t, store, "intention fenêtre")
	p, _ := store.Obtenir("temoin")
	ancien := strings.Repeat("a", 300)
	recent := "tour récent court"
	_ = store.journaliserTour(p, map[string]any{"type": "TOUR_DIRECTEUR", "contenu": ancien})
	_ = store.journaliserTour(p, map[string]any{"type": "TOUR_DIRECTEUR", "contenu": recent})
	fen, err := store.fenetreDialogue(p)
	if err != nil {
		t.Fatalf("fenêtre : %v", err)
	}
	budget := store.cfg.ContexteJetonsMax * 4 / 2
	if len(fen) > budget {
		t.Fatalf("fenêtre hors budget : %d > %d", len(fen), budget)
	}
	if !strings.Contains(fen, recent) {
		t.Fatalf("le tour récent doit être conservé : %s", fen)
	}
	if strings.Contains(fen, ancien) {
		t.Fatalf("le tour ancien hors budget doit être évacué")
	}
}

// TestMethodeEmbarquee : la méthode v1 est embarquée dans le binaire.
func TestMethodeEmbarquee(t *testing.T) {
	if !strings.Contains(MethodeV1, "maïeutique") ||
		!strings.Contains(MethodeV1, "PROPOSITION_ORDRE") ||
		!strings.Contains(MethodeV1, "DONNÉE") {
		t.Fatalf("méthode v1 incomplète")
	}
}

// TestReponseInvalideBorne : deux réponses INVALIDES de suite mènent à
// ATTENTE_PILOTE (S14 du plan v2 §3).
func TestReponseInvalideBorne(t *testing.T) {
	store, f := magasinTest(t)
	projetDialoguer(t, store, "intention")
	f.ajouter("pas du json", "toujours pas du json")
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("1re réponse invalide attendue, reçu : %v", err)
	}
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); !estErreurCode(err, CodeReponseInvalide) {
		t.Fatalf("2e réponse invalide attendue, reçu : %v", err)
	}
	p, err := store.Obtenir("temoin")
	if err != nil || p.Etat != EtatAttentePilote {
		t.Fatalf("ATTENTE_PILOTE attendu après 2 réponses invalides : %v / %s", err, p.Etat)
	}
}

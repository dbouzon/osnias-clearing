// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"coordengine/internal/budget"
	"coordengine/internal/engine"
	"coordengine/internal/missions"
	"coordengine/internal/policy"
	"coordengine/internal/spec"
)

// ---- M-COORD-DEP-02 (revue PR #71) : consultation lecture seule et
// reprise sur le référentiel réel ----

var adaptateursConsulter = []string{"mistral", "openai"}

var rolesConsulter = map[string][]string{
	"EXECUTANT":      {"executer", "soumettre", "acker"},
	"RECEPTIONNAIRE": {"recevoir"},
	"PILOTE":         {"cloturer"},
}

var execConsulter = policy.Identity{ID: "mistral-exec", Role: "EXECUTANT", QualifiedBy: "mistral"}
var recepConsulter = policy.Identity{ID: "openai-recep", Role: "RECEPTIONNAIRE", QualifiedBy: "openai"}

const r1Consulter = "1111111111111111111111111111111111111111"
const r2Consulter = "3333333333333333333333333333333333333333"
const empConsulter = "2222222222222222222222222222222222222222222222222222222222222222"

func transitionConsulter(t *testing.T, ref *missions.Referentiel, issue, cible string,
	id policy.Identity, donnees map[string]string) {
	t.Helper()
	err := ref.Transaction(issue, func(c *missions.ContexteMission) error {
		ok, motif := c.Enr.Mission.Transition(cible, id, adaptateursConsulter, rolesConsulter, donnees)
		if !ok {
			return errors.New(motif)
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("transition %s : %v", cible, err)
	}
}

func consulterCapture(t *testing.T, racine, issue string) (string, int) {
	t.Helper()
	sauve := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe : %v", err)
	}
	os.Stdout = w
	rc := consulterRun(racine, issue)
	w.Close()
	os.Stdout = sauve
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("lecture sortie : %v", err)
	}
	return string(out), rc
}

func TestConsulterNeCreeRien(t *testing.T) {
	// Revue PR #71, point 1 : une consultation ne doit JAMAIS écrire.
	// Une racine absente doit faire échouer -consulter SANS créer le
	// répertoire (missions.Ouvrir fait MkdirAll ; -consulter n'y passe plus).
	dir := filepath.Join(t.TempDir(), "absent")
	if rc := consulterRun(dir, "#1"); rc == 0 {
		t.Fatal("la consultation d'une racine absente doit échouer")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("la consultation a créé la racine : écriture interdite en lecture seule")
	}
}

func TestConsulterReprisePointsHistorique(t *testing.T) {
	// Contrôle décisif DEP-02 (revue PR #71, point 3) : une décision
	// A_CORRIGER avec points est enregistrée dans le RÉFÉRENTIEL RÉEL,
	// relue par -consulter après un « arrêt » (le coordinateur est tombé :
	// toute la relecture vient du disque), et les points restent attachés
	// à la bonne révision. Après r2 ACCEPTEE, les points de r1 restent
	// consultables comme historique.
	racine := t.TempDir()
	ref, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	issue := "coord-dep-02-reprise"
	if _, err := ref.Creer(issue, adaptateursConsulter, rolesConsulter,
		[]policy.Identity{execConsulter, recepConsulter},
		budget.Plafonds{
			Appels: map[string]int{"mistral": 20, "openai": 20},
			Jetons: map[string]int{"mistral": 200000, "openai": 200000},
		}); err != nil {
		t.Fatalf("création : %v", err)
	}

	// r1 : exécution -> soumission -> réception A_CORRIGER avec points.
	transitionConsulter(t, ref, issue, spec.EtatExecution, execConsulter, nil)
	transitionConsulter(t, ref, issue, spec.EtatDecisionAttendue, execConsulter,
		map[string]string{"revision": r1Consulter, "empreinte_preuves": empConsulter})
	transitionConsulter(t, ref, issue, spec.EtatEnCorrection, recepConsulter, map[string]string{
		"verdict_id": "v1", "verdict_url": "urn:test:v1",
		"points": `["corriger la conversion 200 centimes","corriger le cout 0,006 c"]`,
	})

	// « SIGKILL » du coordinateur : plus aucune référence mémoire ; la
	// consultation suivante ne peut venir que du disque.
	ref = nil

	sortie, rc := consulterCapture(t, racine, issue)
	if rc != 0 {
		t.Fatalf("consulter après arrêt rc=%d sortie=%s", rc, sortie)
	}
	var m1 engine.Mission
	if err := json.Unmarshal([]byte(sortie), &m1); err != nil {
		t.Fatalf("sortie consulter non JSON : %v", err)
	}
	if m1.State != spec.EtatEnCorrection {
		t.Fatalf("état attendu EN_CORRECTION : %s", m1.State)
	}
	if len(m1.Decisions) != 1 {
		t.Fatalf("une décision attendue après r1, obtenu %d", len(m1.Decisions))
	}
	d1 := m1.Decisions[0]
	if d1.Verdict != "A_CORRIGER" || d1.Revision != r1Consulter {
		t.Fatalf("points r1 doivent rester attachés à la révision r1 : %+v", d1)
	}
	if len(d1.Points) != 2 ||
		d1.Points[0] != "corriger la conversion 200 centimes" ||
		d1.Points[1] != "corriger le cout 0,006 c" {
		t.Fatalf("détail des corrections perdu à la relecture : %+v", d1.Points)
	}

	// r2 : correction -> nouvelle soumission -> réception ACCEPTEE.
	ref2, err := missions.Ouvrir(racine)
	if err != nil {
		t.Fatalf("réouverture après arrêt : %v", err)
	}
	transitionConsulter(t, ref2, issue, spec.EtatExecution, execConsulter, nil)
	transitionConsulter(t, ref2, issue, spec.EtatDecisionAttendue, execConsulter,
		map[string]string{"revision": r2Consulter, "empreinte_preuves": empConsulter})
	transitionConsulter(t, ref2, issue, spec.EtatAcceptee, recepConsulter, map[string]string{
		"verdict_id": "v2", "verdict_url": "urn:test:v2", "points": `[]`,
	})

	// Historique : après r2, les points de r1 restent consultables.
	sortie2, rc2 := consulterCapture(t, racine, issue)
	if rc2 != 0 {
		t.Fatalf("consulter après r2 rc=%d sortie=%s", rc2, sortie2)
	}
	var m2 engine.Mission
	if err := json.Unmarshal([]byte(sortie2), &m2); err != nil {
		t.Fatalf("sortie consulter non JSON : %v", err)
	}
	if m2.State != spec.EtatAcceptee {
		t.Fatalf("état attendu ACCEPTEE : %s", m2.State)
	}
	if len(m2.Decisions) != 2 {
		t.Fatalf("deux décisions attendues (historique complet), obtenu %d", len(m2.Decisions))
	}
	h1, h2 := m2.Decisions[0], m2.Decisions[1]
	if h1.Verdict != "A_CORRIGER" || h1.Revision != r1Consulter || len(h1.Points) != 2 ||
		h1.Points[0] != "corriger la conversion 200 centimes" ||
		h1.Points[1] != "corriger le cout 0,006 c" {
		t.Fatalf("points r1 disparus de l'historique après r2 : %+v", h1)
	}
	if h2.Verdict != "ACCEPTEE" || h2.Revision != r2Consulter {
		t.Fatalf("décision r2 inattendue : %+v", h2)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"coordengine/internal/policy"
)

var adaptateurs = []string{"adapteur_a", "adapteur_b"}
var roles = map[string][]string{
	"EXECUTANT":      {"executer", "soumettre", "acker"},
	"RECEPTIONNAIRE": {"recevoir"},
	"PILOTE":         {"cloturer"},
}

var exec = policy.Identity{ID: "agent_exec", Role: "EXECUTANT", QualifiedBy: "adapteur_a"}
var recep = policy.Identity{ID: "agent_recep", Role: "RECEPTIONNAIRE", QualifiedBy: "adapteur_b"}
var pilote = policy.Identity{ID: "agent_pilote", Role: "PILOTE", QualifiedBy: "adapteur_a"}

const rev40 = "1111111111111111111111111111111111111111"
const emp64 = "2222222222222222222222222222222222222222222222222222222222222222"

func TestScenarioNominal(t *testing.T) {
	m := New("#1")
	if m.State != "PLANIFIEE" {
		t.Fatal("etat initial")
	}
	steps := []struct {
		cible string
		id    policy.Identity
		data  map[string]string
	}{
		{"EXECUTION", exec, nil},
		{"DECISION_ATTENDUE", exec, map[string]string{"revision": rev40, "empreinte_preuves": emp64}},
		{"ACCEPTEE", recep, map[string]string{"verdict_id": "v1", "verdict_url": "https://x/1"}},
		{"ACK_RECU", exec, map[string]string{"revision_courante": rev40, "empreinte_courante": emp64}},
		{"CLOTUREE", pilote, nil},
	}
	for _, s := range steps {
		ok, motif := m.Transition(s.cible, s.id, adaptateurs, roles, s.data)
		if !ok {
			t.Fatalf("transition %s refusee: %s", s.cible, motif)
		}
	}
	if m.State != "CLOTUREE" {
		t.Fatal("etat final attendu CLOTUREE")
	}
}

func refus(t *testing.T, cible string, id policy.Identity, data map[string]string, contient string) {
	t.Helper()
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	ok, motif := m.Transition(cible, id, adaptateurs, roles, data)
	if ok || !strings.Contains(motif, contient) {
		t.Fatalf("refus attendu contenant %q, obtenu ok=%v motif=%s", contient, ok, motif)
	}
}

func TestAutoReception(t *testing.T) {
	// L'executant declare RECEPTIONNAIRE par son propre adaptateur ne peut pas
	// recevoir sa mission (N-11).
	self := policy.Identity{ID: "agent_exec", Role: "RECEPTIONNAIRE", QualifiedBy: "adapteur_a"}
	refus(t, "ACCEPTEE", self, map[string]string{"verdict_id": "v", "verdict_url": "u"}, "auto-reception interdite")
}

func TestVerdictSansReference(t *testing.T) {
	refus(t, "REFUSEE", recep, nil, "reference de verdict obligatoire")
}

func TestHorsTableFermee(t *testing.T) {
	refus(t, "CLOTUREE", pilote, nil, "transition interdite")
}

func TestRoleNonAutorise(t *testing.T) {
	// Le PILOTE ne soumet pas a la place de l'executant.
	p := policy.Identity{ID: "agent_pilote", Role: "PILOTE", QualifiedBy: "adapteur_a"}
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	ok, motif := m.Transition("DECISION_ATTENDUE", p, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	if ok || !strings.Contains(motif, "role non autorise") {
		t.Fatalf("role refuse attendu, obtenu %v %s", ok, motif)
	}
}

func TestIdentiteHorsAdaptateurEnregistre(t *testing.T) {
	inconnu := policy.Identity{ID: "agent_exec", Role: "RECEPTIONNAIRE", QualifiedBy: "adapteur_inconnu"}
	refus(t, "ACCEPTEE", inconnu, map[string]string{"verdict_id": "v", "verdict_url": "u"}, "identite non habilitee")
}

func TestVerdictPerime(t *testing.T) {
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	m.Transition("ACCEPTEE", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v", "verdict_url": "u"})
	// L'empreinte des preuves a change depuis la decision : ACK refuse (N-13).
	ok, motif := m.Transition("ACK_RECU", exec, adaptateurs, roles,
		map[string]string{"revision_courante": rev40, "empreinte_courante": strings.Repeat("3", 64)})
	if ok || !strings.Contains(motif, "verdict perime") {
		t.Fatalf("ACK perime attendu, obtenu %v %s", ok, motif)
	}
}

func TestExecutantNonStable(t *testing.T) {
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	m.Transition("EN_CORRECTION", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v", "verdict_url": "u"})
	autre := policy.Identity{ID: "agent_exec_2", Role: "EXECUTANT", QualifiedBy: "adapteur_a"}
	ok, motif := m.Transition("EXECUTION", autre, adaptateurs, roles, nil)
	if ok || !strings.Contains(motif, "non stabilisee") {
		t.Fatalf("executant non stable attendu, obtenu %v %s", ok, motif)
	}
}

func TestTerminalRefuseTout(t *testing.T) {
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	m.Transition("REFUSEE", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v", "verdict_url": "u"})
	ok, motif := m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	if ok || !strings.Contains(motif, "mission terminee") {
		t.Fatalf("terminal attendu, obtenu %v %s", ok, motif)
	}
}

func TestConfigInvalideFermeTout(t *testing.T) {
	m := New("#1")
	mal := map[string][]string{"EXECUTANT": {"executer", "*"}}
	ok, motif := m.Transition("EXECUTION", exec, adaptateurs, mal, nil)
	if ok || !strings.Contains(motif, "configuration d'habilitations invalide") {
		t.Fatalf("fail closed attendu, obtenu %v %s", ok, motif)
	}
}

func TestBoucleCorrectionRefus(t *testing.T) {
	// A_CORRIGER par recepteur distinct, meme executant reprend, puis refus motive.
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	if ok, motif := m.Transition("EN_CORRECTION", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v1", "verdict_url": "u1"}); !ok {
		t.Fatal(motif)
	}
	if ok, motif := m.Transition("EXECUTION", exec, adaptateurs, roles, nil); !ok {
		t.Fatal(motif)
	}
	if d := m.Decisions[0]; d.Verdict != "A_CORRIGER" {
		t.Fatalf("verdict A_CORRIGER attendu, obtenu %s", d.Verdict)
	}
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	if ok, motif := m.Transition("REFUSEE", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v2", "verdict_url": "u2"}); !ok {
		t.Fatal(motif)
	}
	if m.State != "REFUSEE" || m.Decisions[1].Verdict != "REFUSEE" {
		t.Fatal("refus motive attendu")
	}
}

// ---- M-COORD-DEP-02 : points du verdict persistes avec la decision ----

func TestVerdictPointsPersistes(t *testing.T) {
	// Controle decisif DEP-02 : au stade EN_CORRECTION, le DETAIL des
	// corrections doit rester attache a la decision persistee.
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	ok, motif := m.Transition("EN_CORRECTION", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v", "verdict_url": "u",
			"points": `["corriger la conversion 200 centimes","corriger le cout 0,006 c"]`})
	if !ok {
		t.Fatalf("transition EN_CORRECTION refusee: %s", motif)
	}
	if len(m.Decisions) != 1 {
		t.Fatalf("une decision attendue, obtenu %d", len(m.Decisions))
	}
	d := m.Decisions[0]
	if d.Verdict != "A_CORRIGER" || len(d.Points) != 2 {
		t.Fatalf("verdict A_CORRIGER avec 2 points attendu, obtenu %+v", d)
	}
	if d.Points[0] != "corriger la conversion 200 centimes" ||
		d.Points[1] != "corriger le cout 0,006 c" {
		t.Fatalf("points alters: %+v", d.Points)
	}
}

func TestVerdictPointsInvalidesRefuses(t *testing.T) {
	// Fail closed : un champ points present mais invalide refuse la
	// transition (un verdict tronque n'ecrase jamais le detail).
	refus(t, "EN_CORRECTION", recep, map[string]string{
		"verdict_id": "v", "verdict_url": "u", "points": `{invalide}`,
	}, "points de verdict invalides")
	refus(t, "ACCEPTEE", recep, map[string]string{
		"verdict_id": "v", "verdict_url": "u", "points": `[42]`,
	}, "points de verdict invalides")
	// Revue PR #71 : null et [null] sont acceptes silencieusement par
	// json.Unmarshal dans un []string ; le moteur doit les refuser.
	refus(t, "EN_CORRECTION", recep, map[string]string{
		"verdict_id": "v", "verdict_url": "u", "points": `null`,
	}, "points de verdict invalides")
	refus(t, "ACCEPTEE", recep, map[string]string{
		"verdict_id": "v", "verdict_url": "u", "points": `[null]`,
	}, "points de verdict invalides")
	refus(t, "EN_CORRECTION", recep, map[string]string{
		"verdict_id": "v", "verdict_url": "u", "points": `["corriger",null]`,
	}, "points de verdict invalides")
}

func TestVerdictPointsRelectureDisque(t *testing.T) {
	// Relecture APRES arret/reprise : la persistance JSON de la mission
	// restitue integralement les points (controle decisif DEP-02).
	m := New("#1")
	m.Transition("EXECUTION", exec, adaptateurs, roles, nil)
	m.Transition("DECISION_ATTENDUE", exec, adaptateurs, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	m.Transition("EN_CORRECTION", recep, adaptateurs, roles,
		map[string]string{"verdict_id": "v9", "verdict_url": "u9",
			"points": `["corriger la conversion 200 centimes","corriger le cout 0,006 c"]`})
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var relue Mission
	if err := json.Unmarshal(raw, &relue); err != nil {
		t.Fatal(err)
	}
	if relue.State != "EN_CORRECTION" || len(relue.Decisions) != 1 {
		t.Fatalf("etat/decisions non restitues: %+v", relue)
	}
	d := relue.Decisions[0]
	if len(d.Points) != 2 || d.Points[1] != "corriger le cout 0,006 c" {
		t.Fatalf("points perdus apres relecture disque: %+v", d.Points)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette de l'affectation EXPLICITE des rôles (M-COORD-RAIL-01 T2,
// arbitrage pilote) : banc multi-fournisseurs à PLUS DE DEUX adaptateurs.
// Additif et rétrocompatible : la chaîne alphabétique (NouvelleMission)
// reste inchangée et son comportement est revérifié ici en parallèle.
package connecteurs

import (
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/policy"
)

// bancTroisAdaptateurs : orchestrateur avec TROIS adaptateurs simulés
// enregistrés (au-delà de deux, le tri alphabétique ne peut plus exprimer
// l'intention du banc : le couple choisi ici est INVERSE du couple trié).
func bancTroisAdaptateurs(t *testing.T) (*Orchestrateur, *missions.Referentiel) {
	t.Helper()
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	nouveau := func(id string) adaptateurs.Adaptateur {
		adaptateur, err := adaptateurs.NouveauSimule(id)
		if err != nil {
			t.Fatal(err)
		}
		return adaptateur
	}
	reg := map[string]adaptateurs.Adaptateur{
		"alpha-factice": nouveau("alpha-factice"),
		"beta-factice":  nouveau("beta-factice"),
		"gamma-factice": nouveau("gamma-factice"),
	}
	orch, err := NouvelOrchestrateur(ref, reg)
	if err != nil {
		t.Fatal(err)
	}
	return orch, ref
}

var rolesBanc = map[string][]string{
	"EXECUTANT":      {"executer", "soumettre", "acker"},
	"RECEPTIONNAIRE": {"recevoir"},
	"PILOTE":         {"cloturer"},
}

func plafondsBanc() budget.Plafonds {
	return budget.Plafonds{
		Appels: map[string]int{"alpha-factice": 10, "beta-factice": 10, "gamma-factice": 10},
		Jetons: map[string]int{"alpha-factice": 1000, "beta-factice": 1000, "gamma-factice": 1000},
	}
}

// identiteBanc : identité enregistrée sous un ID donné.
func identiteBanc(t *testing.T, enr *missions.Enregistrement, id string) policy.Identity {
	t.Helper()
	for _, i := range enr.Identites {
		if i.ID == id {
			return i
		}
	}
	t.Fatalf("identité %s attendue : %+v", id, enr.Identites)
	return policy.Identity{}
}

// R1 : affectation explicite — l'exécutant est qualifié par l'adaptateur
// CHOISI (pas le premier trié) ; couple INVERSE du couple alphabétique pour
// prouver que le choix est bien explicite.
func TestAffectationExpliciteTroisAdaptateurs(t *testing.T) {
	orch, ref := bancTroisAdaptateurs(t)
	if err := orch.NouvelleMissionExplicit("banc-r1", "exec-beta", "recep-alpha",
		"beta-factice", "alpha-factice", rolesBanc, plafondsBanc()); err != nil {
		t.Fatalf("création explicite : %v", err)
	}
	enr, err := ref.Obtenir("banc-r1")
	if err != nil {
		t.Fatal(err)
	}
	if i := identiteBanc(t, enr, "exec-beta"); i.QualifiedBy != "beta-factice" {
		t.Fatalf("exec qualifié par beta attendu : %s", i.QualifiedBy)
	}
	if i := identiteBanc(t, enr, "recep-alpha"); i.QualifiedBy != "alpha-factice" {
		t.Fatalf("récep qualifié par alpha attendu : %s", i.QualifiedBy)
	}
	if i := identiteBanc(t, enr, ActeurPilote); i.QualifiedBy != "beta-factice" {
		t.Fatalf("pilote qualifié par l'adaptateur exécutant attendu : %s", i.QualifiedBy)
	}
}

// R2 : fail closed — adaptateur qualifiant INCONNU du registre : refus,
// aucune mission créée.
func TestAffectationExpliciteAdaptateurInconnu(t *testing.T) {
	orch, ref := bancTroisAdaptateurs(t)
	if err := orch.NouvelleMissionExplicit("banc-r2", "exec-x", "recep-y",
		"inexistant-factice", "alpha-factice", rolesBanc, plafondsBanc()); err == nil ||
		!strings.Contains(err.Error(), "ADAPTATEUR_INCONNU") {
		t.Fatalf("ADAPTATEUR_INCONNU attendu : %v", err)
	}
	if err := orch.NouvelleMissionExplicit("banc-r2b", "exec-x", "recep-y",
		"alpha-factice", "inexistant-factice", rolesBanc, plafondsBanc()); err == nil ||
		!strings.Contains(err.Error(), "ADAPTATEUR_INCONNU") {
		t.Fatalf("ADAPTATEUR_INCONNU attendu (récep) : %v", err)
	}
	if _, err := ref.Obtenir("banc-r2"); err == nil {
		t.Fatal("aucune mission ne doit exister après refus (fail closed)")
	}
}

// R3 : rétrocompatibilité — NouvelleMission conserve SON comportement
// historique TEL QUEL sur le même banc de trois : troncature à ids[:2]
// puis premier/dernier. Comportement constaté au SHA f51bc8a5 : avec trois
// adaptateurs, gamma-factice est EXCLU, l'exécutant est qualifié par
// alpha (premier trié) et le réceptionnaire par beta (second trié, car
// ids[len-1] désigne la FIN DE LA LISTE TRONQUÉE, pas le dernier du
// registre). Ce comportement surprenant EST le défaut que
// NouvelleMissionExplicit corrige ; le test le FIGE pour garantir la
// non-régression : aucun changement introduit par l'ajout.
func TestRetrocompatibiliteTriAlphabetique(t *testing.T) {
	orch, ref := bancTroisAdaptateurs(t)
	if err := orch.NouvelleMission("banc-r3", "exec-defaut", "recep-defaut",
		rolesBanc, plafondsBanc()); err != nil {
		t.Fatalf("création par tri : %v", err)
	}
	enr, err := ref.Obtenir("banc-r3")
	if err != nil {
		t.Fatal(err)
	}
	if i := identiteBanc(t, enr, "exec-defaut"); i.QualifiedBy != "alpha-factice" {
		t.Fatalf("tri : exécutant qualifié par le PREMIER trié : %s", i.QualifiedBy)
	}
	if i := identiteBanc(t, enr, "recep-defaut"); i.QualifiedBy != "beta-factice" {
		t.Fatalf("tri historique : réceptionnaire qualifié par le SECOND trié : %s", i.QualifiedBy)
	}
}

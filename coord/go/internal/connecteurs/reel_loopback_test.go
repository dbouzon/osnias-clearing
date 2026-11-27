// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/missions"
)

// M-COORD-MISTRAL-VPS-01, issue #78 : réserve du pilote du 27/11/2026.
// Sur l'essai réel, la voûte monétaire avait réservé 0 centime pour un
// appel FACTURABLE passé derrière un pont loopback (moteur ->
// HTTPEndPoint -> pont coord.llm.v1 -> api.mistral.ai) : SimuleLocal
// qualifiait tout endpoint loopback de « banc d'essai 0 € », même en mode
// REEL déclaré par le pilote. Ces tests prouvent la correction : une
// configuration déclarée REEL n'est JAMAIS « simulée locale », même en
// loopback. Ils construisent les connecteurs par le chemin de production
// (Config -> NouveauHTTPEndPoint -> adopter, triple verrou), pas en
// white-box. Serveurs factices loopback : HORS RÉSEAU, AUCUNE CLÉ VRAIE,
// 0 €.

// configReelDerrierePont : configuration REEL déclarée par le pilote dont
// l'endpoint est un pont loopback, tarifs/bornes/retraits déclarés.
// Enveloppe conservatrice (retraits_max = 0) : entrée 1000×15/1e6 = 1 c,
// sortie 800×60/1e6 = 1 c, requête 1 c => 3 c par tentative.
func configReelDerrierePont() *Config {
	return &Config{
		Mode: ModeReel,
		Autorisation: &Autorisation{
			Pilote:          "pilote-test",
			Fournisseurs:    []string{"mistral"},
			PlafondCentimes: 3, PlafondAppels: 1, PlafondJetons: 1800,
			Date: "2026-11-27",
		},
		Fournisseurs: []ConfigFournisseur{{
			ID: "mistral", Endpoint: "", CleEnv: "COORD_BRIDGE_JETON",
			Modele: "exec-modele", TimeoutMS: 5000, TailleMaxReponse: 65536,
			RetraitsMax:                    0,
			TarifEntreeCentimesParMillion: 15,
			TarifSortieCentimesParMillion: 60,
			TarifRequeteCentimes:          1,
			JetonsEntreeMaxParTentative:   1000,
			JetonsSortieMaxParTentative:   800,
		}},
	}
}

// connecteurReelDerrierePont : connecteur REEL construit par le chemin de
// production, dont l'endpoint est le serveur factice loopback (le « pont
// »). Retourne le connecteur et le compteur de requêtes du témoin.
func connecteurReelDerrierePont(t *testing.T, cfg *Config) (*HTTPEndPoint, *int) {
	t.Helper()
	appels := 0
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		appels++
		w.Write(reponseChat("LIVRABLE FACTICE", "exec-modele", 100))
	})
	cfg.Fournisseurs[0].Endpoint = s.URL
	t.Setenv("COORD_MODE_REEL", "1")
	t.Setenv("COORD_BRIDGE_JETON", "jeton-de-pont-local-1234")
	h, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur REEL derrière pont : %v", err)
	}
	return h, &appels
}

// TestReelDerrierePontPlafondInsuffisant : REPRODUCTION de la réserve du
// pilote (#78). Plafond monétaire de mission INSUFFISANT (2 c < enveloppe
// conservatrice 3 c) => refus BUDGET_DEPASSE AVANT le réseau, témoin à
// ZÉRO requête, AUCUNE réservation conservée. Ce test échoue sur main
// (la voûte restait inerte derrière un pont loopback : l'appel partait),
// il passe avec le correctif.
func TestReelDerrierePontPlafondInsuffisant(t *testing.T) {
	h, appels := connecteurReelDerrierePont(t, configReelDerrierePont())
	if h.SimuleLocal() {
		t.Fatalf("REEL derrière pont : l'adaptateur ne doit plus être " +
			"qualifié de simulé local (issue #78)")
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"mistral": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "reel-pont-1"
	p := plafondsTest()
	p.Centimes = 2 // enveloppe conservatrice 3 c : dépassée
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("refus monétaire BUDGET_DEPASSE attendu AVANT l'appel : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("le témoin ne doit compter AUCUNE requête : %d", *appels)
	}
	enr, _ := ref.Obtenir(issue)
	if enr.Budget.CentimesReservesTotal() != 0 {
		t.Fatalf("aucune réservation ne devait être conservée : %d",
			enr.Budget.CentimesReservesTotal())
	}
}

// TestReelDerrierePontPlafondSuffisantReserveDurable : plafond SUFFISANT
// (3 c = enveloppe) => l'appel passe et la réservation DURABLE de
// l'enveloppe complète est consignée : état budgétaire + journal
// APPEL_RESERVE avec centimes_reserves (preuve demandée par le pilote).
func TestReelDerrierePontPlafondSuffisantReserveDurable(t *testing.T) {
	h, appels := connecteurReelDerrierePont(t, configReelDerrierePont())
	if h.SimuleLocal() {
		t.Fatalf("REEL derrière pont : l'adaptateur ne doit plus être " +
			"qualifié de simulé local (issue #78)")
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"mistral": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "reel-pont-2"
	p := plafondsTest()
	p.Centimes = 3 // enveloppe conservatrice exacte
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("appel autorisé : %v", err)
	}
	if *appels != 1 {
		t.Fatalf("exactement 1 requête attendue : %d", *appels)
	}
	enr, _ := ref.Obtenir(issue)
	if enr.Budget.Etat.CentimesReserves["mistral"] != 3 {
		t.Fatalf("réservation durable attendue (3 centimes) : %+v",
			enr.Budget.Etat.CentimesReserves)
	}
	j, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	if !strings.Contains(string(j), "APPEL_RESERVE") ||
		!strings.Contains(string(j), "centimes_reserves") {
		t.Fatalf("le journal doit tracer la réservation monétaire")
	}
	if !strings.Contains(string(j), `"centimes_reserves":3`) {
		t.Fatalf("le journal doit montrer la réservation prévue de 3 centimes : %s", j)
	}
}

// TestSimuleLoopbackResteZeroCentime : rétrocompatibilité. Une
// configuration SIMULE (mode par défaut) pointant un serveur factice
// loopback reste 0 € : aucune réservation, aucune exigence de triple
// verrou. Le correctif n'arme la voûte QUE pour le mode REEL déclaré.
func TestSimuleLoopbackResteZeroCentime(t *testing.T) {
	cfg := configReelDerrierePont()
	cfg.Mode = ModeSimule
	cfg.Autorisation = nil // SIMULE : aucune autorisation exigée
	h, appels := connecteurReelDerrierePont(t, cfg)
	if !h.SimuleLocal() {
		t.Fatalf("SIMULE loopback : doit rester qualifié de simulé local (0 €)")
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"mistral": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "simule-pont-3"
	p := plafondsTest()
	p.Centimes = 3
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("appel simulé : %v", err)
	}
	if *appels != 1 {
		t.Fatalf("exactement 1 requête attendue : %d", *appels)
	}
	enr, _ := ref.Obtenir(issue)
	if enr.Budget.CentimesReservesTotal() != 0 {
		t.Fatalf("SIMULE loopback : aucune réservation attendue : %d",
			enr.Budget.CentimesReservesTotal())
	}
}

// TestReelDerrierePontExigeTripleVerrou : une configuration REEL derrière
// un pont loopback passe par le triple verrou du chemin de production,
// comme tout appel réel : COORD_MODE_REEL=1 exigé, clé d'environnement
// exigée (fail closed), même en loopback.
func TestReelDerrierePontExigeTripleVerrou(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseChat("LIVRABLE FACTICE", "exec-modele", 100))
	})

	// Sans COORD_MODE_REEL=1 : construction refusée, fail closed.
	cfg := configReelDerrierePont()
	cfg.Fournisseurs[0].Endpoint = s.URL
	t.Setenv("COORD_MODE_REEL", "")
	t.Setenv("COORD_BRIDGE_JETON", "jeton-de-pont-local-1234")
	_, err := cfg.NouveauHTTPEndPoint("mistral")
	if err == nil || !strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("COORD_MODE_REEL absent : construction REEL devait être refusée : %v", err)
	}

	// COORD_MODE_REEL=1 mais clé absente : refusée aussi (fail closed).
	t.Setenv("COORD_MODE_REEL", "1")
	t.Setenv("COORD_BRIDGE_JETON", "")
	_, err = cfg.NouveauHTTPEndPoint("mistral")
	if err == nil || !strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("clé absente : construction REEL devait être refusée : %v", err)
	}
}

// TestReelDerrierePontBorneEmission : le bornage à l'émission
// (CHARGE_HORS_BORNE) s'applique aussi en REEL derrière un pont loopback :
// AUCUN octet émis quand la charge dépasse la borne déclarée.
func TestReelDerrierePontBorneEmission(t *testing.T) {
	cfg := configReelDerrierePont()
	cfg.Fournisseurs[0].JetonsEntreeMaxParTentative = 10 // borne très basse
	h, appels := connecteurReelDerrierePont(t, cfg)
	if h.SimuleLocal() {
		t.Fatalf("REEL derrière pont : l'adaptateur ne doit plus être " +
			"qualifié de simulé local (issue #78)")
	}
	_, err := h.ExecuterContexte(context.Background(), "executer",
		"mandat délibérément long pour dépasser la borne de dix jetons d'entrée déclarée", "")
	if err == nil || !strings.Contains(err.Error(), "CHARGE_HORS_BORNE") {
		t.Fatalf("refus CHARGE_HORS_BORNE attendu AVANT l'émission : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("AUCUNE requête ne devait partir : %d", *appels)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/missions"
)

// Tests monétaires de la correction de la réserve 7154948522
// (M-COORD-PILOT-03) : réservation conservatrice AVANT le réseau, plafond
// opposable, retraits comptés intégralement, surplus jamais libérateur,
// bornage à l'émission. Tous sur serveurs factices loopback : HORS RÉSEAU,
// AUCUNE CLÉ, 0 € ; les connecteurs sont déclarés RÉELS en white-box pour
// exercer la voûte monétaire uniquement.

// nouveauConnecteurReelFactice : HTTPEndPoint déclaré réel (simuleLoc=false)
// pointant vers un serveur factice loopback, avec tarifs et bornes déclarés.
func nouveauConnecteurReelFactice(t *testing.T, handler http.HandlerFunc,
	tarifEntree, tarifSortie, tarifRequete, borneEntree, borneSortie, retraits int) (*HTTPEndPoint, *int) {
	t.Helper()
	appels := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels++
		if handler != nil {
			handler(w, r)
			return
		}
		w.Write(reponseChat("LIVRABLE FACTICE", "exec-modele", 1500))
	}))
	t.Cleanup(s.Close)
	h := &HTTPEndPoint{
		id: "mistral", endpoint: s.URL, modele: "exec-modele",
		client:    &http.Client{Timeout: 5 * time.Second},
		tailleMax: 65536, retraits: retraits, simuleLoc: false,
		tarifEntreeMillion: tarifEntree, tarifSortieMillion: tarifSortie,
		tarifRequete:    tarifRequete,
		jetonsEntreeMax: borneEntree, jetonsSortieMax: borneSortie,
	}
	return h, &appels
}

func orchestreAvecReel(t *testing.T, h *HTTPEndPoint) (*Orchestrateur, *missions.Referentiel) {
	t.Helper()
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"mistral": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	return orch, ref
}

// Budget monétaire insuffisant => refus AVANT réseau : AUCUNE requête émise.
func TestPlafondMonetaireInsuffisantAvantReseau(t *testing.T) {
	h, appels := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 4000, 1000, 0)
	orch, ref := orchestreAvecReel(t, h)
	issue := "monetaire-1"
	p := plafondsTest()
	p.Centimes = 1 // 1 centime : enveloppe (bien plus grande) dépassée
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("refus monétaire attendu AVANT l'appel : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("AUCUNE requête n'a dû partir : %d", *appels)
	}
	enr, _ := ref.Obtenir(issue)
	if enr.Budget.CentimesReservesTotal() != 0 {
		t.Fatalf("aucune réservation ne devait être conservée : %d", enr.Budget.CentimesReservesTotal())
	}
}

// L'enveloppe réservée compte (1 + retraits) tentatives INTÉGRALES : chaque
// retrait 429/5xx peut être facturé en totalité par le fournisseur.
func TestEnveloppeCompteLesRetraits(t *testing.T) {
	h0, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 4000, 1000, 0)
	e0, err := h0.EstimationCentimesSur()
	if err != nil {
		t.Fatalf("estimation : %v", err)
	}
	h1, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 4000, 1000, 1)
	e1, err := h1.EstimationCentimesSur()
	if err != nil {
		t.Fatalf("estimation retraits : %v", err)
	}
	if e1 != 2*e0 {
		t.Fatalf("retraits_max=1 doit doubler l'enveloppe : %d vs %d", e1, e0)
	}
	// Décomposition exacte (arrondi vers le haut, entiers) :
	// entrée 4000×100/1e6 => 1c, sortie 1000×300/1e6 => 1c, requête 1c,
	// tentative = 3c, enveloppe sans retrait = 3c.
	if e0 != 3 {
		t.Fatalf("décomposition entière attendue 3c : %d", e0)
	}
}

// Tarif ou borne inconnu (<=0) => refus AVANT réseau, fail closed.
func TestTarifInconnuRefuseAvantReseau(t *testing.T) {
	h, appels := nouveauConnecteurReelFactice(t, nil, 100, 0, 1, 4000, 1000, 0) // tarif sortie absent
	orch, _ := orchestreAvecReel(t, h)
	issue := "monetaire-tarif"
	p := plafondsTest()
	p.Centimes = 100000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "TARIF_INCONNU") {
		t.Fatalf("TARIF_INCONNU attendu avant réseau : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("aucune requête n'a dû partir : %d", *appels)
	}
}

// Cycle monétaire nominal : réservation AVANT appel, conservée après succès
// (CONSERVATRICE : jamais libérée), journal enrichi ; chaque nouvelle
// tentative facturable réserve À NOUVEAU durablement.
func TestReservationMonetaireDurableSurCycle(t *testing.T) {
	h, appels := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 4000, 1000, 0)
	orch, ref := orchestreAvecReel(t, h)
	issue := "monetaire-cycle"
	p := plafondsTest()
	p.Centimes = 100000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if *appels != 1 {
		t.Fatalf("exactement une requête attendue : %d", *appels)
	}
	enr, _ := ref.Obtenir(issue)
	if enr.Budget.Etat.CentimesReserves["mistral"] != 3 {
		t.Fatalf("réservation durable attendue (3 centimes) : %+v", enr.Budget.Etat.CentimesReserves)
	}
	j, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	if !strings.Contains(string(j), "centimes_reserves") {
		t.Fatalf("le journal doit tracer la réservation monétaire")
	}
	// Nouvelle tentative facturable : nouvelle réservation CONSERVÉE en
	// plus de la précédente (jamais libérée).
	if _, err := orch.Soumettre(context.Background(), issue, "mistral-exec", "LIVRABLE FACTICE"); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	enr, _ = ref.Obtenir(issue)
	if enr.Budget.Etat.CentimesReserves["mistral"] != 6 {
		t.Fatalf("deux réservations durables attendues (6 centimes) : %+v", enr.Budget.Etat.CentimesReserves)
	}
}

// La requête elle-même est refusée avant tout octet si la charge dépasse la
// borne d'entrée déclarée (bornage à L'ÉMISSION, heuristique 4 octets/jeton).
func TestChargeHorsBorneRefuseAvantEmission(t *testing.T) {
	h, appels := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 10, 1000, 0) // borne entrée 10 jetons
	_, err := h.Executer("mandat très long qui dépasse largement la borne déclarée de dix jetons d'entrée")
	if err == nil || !strings.Contains(err.Error(), "CHARGE_HORS_BORNE") {
		t.Fatalf("CHARGE_HORS_BORNE attendu : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("AUCUN octet n'a dû être émis : %d", *appels)
	}
}

// ChargerConfig : mode REEL sans tarifs/bornes => refus (fail closed).
func TestConfigReelSansTarifsRefusee(t *testing.T) {
	rep := t.TempDir()
	chemin := filepath.Join(rep, "config.json")
	config := `{"mode":"REEL","autorisation_pilote":{"pilote":"p","fournisseurs":["a"],` +
		`"plafond_centimes":500,"plafond_appels_par_fournisseur":5,` +
		`"plafond_jetons_par_fournisseur":5000,"date":"2026-11-25"},` +
		`"fournisseurs":[{"id":"a","endpoint":"https://api.example.com/v1/chat/completions",` +
		`"cle_env":"X_K","modele":"m","timeout_ms":5000,"taille_max_reponse":65536}]}`
	if err := os.WriteFile(chemin, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ChargerConfig(chemin); err == nil ||
		!strings.Contains(err.Error(), "tarifs") {
		t.Fatalf("config REEL sans tarifs doit être refusée : %v", err)
	}
}

// L'exemple de configuration REEL livré (coord/pilot/connecteurs/) doit
// rester ACCEPTÉ par ChargerConfig : il ne doit jamais diverger des
// validations (autorisation complète + tarifs et bornes explicites).
func TestExempleConfigReelLivreValide(t *testing.T) {
	if _, err := ChargerConfig("../../../pilot/connecteurs/exemple-config-reel.json"); err != nil {
		t.Fatalf("l'exemple de configuration REEL livré doit être ACCEPTÉ : %v", err)
	}
}

// CoutReelCentimes : tarification conservatrice après coup (tarif maximum
// des deux appliqué, arrondi vers le haut, prix de requête inclus).
func TestCoutReelCentimesConservateur(t *testing.T) {
	h, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 4000, 1000, 0)
	c, err := h.CoutReelCentimes(1500)
	if err != nil {
		t.Fatalf("coût réel : %v", err)
	}
	// 1500 jetons au tarif le plus élevé (300c/M) => 1c, + 1c de requête.
	if c != 2 {
		t.Fatalf("coût conservateur attendu 2c : %d", c)
	}
	_, err = (&HTTPEndPoint{id: "x"}).CoutReelCentimes(10)
	if err == nil || !strings.Contains(err.Error(), "TARIF_INCONNU") {
		t.Fatalf("tarif inconnu attendu : %v", err)
	}
}

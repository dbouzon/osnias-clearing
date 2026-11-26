// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package agents_test

// inference_test.go : recette QUALIFIÉE (SIMULÉE, 0 EUR, serveurs
// factices loopback uniquement) du raccordement Anthropic au socle
// M-COORD-LLM-04 T2 : le VRAI chemin A2A → ExecInférence → connecteur
// moteur (format Messages, aucune copie) → HTTP factice est exercé.
// Correspondance exigée : chaque test porte l'étiquette Qx du plan v1.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/agents"

	"coordengine/connecteursapi"
)

const (
	modeleAnthropic = "claude-3-5-sonnet-20241022"
	jetonsEnveloppe = 5000
)

// Fixture d'un agent d'inférence raccordé à un factice du format.
type fixture struct {
	t       *testing.T
	serveur *agents.Serveur
	rail    *coorda2a.Rail
}

// demarrerAgent : factice format + point connecteur MOTEUR (alias) +
// socle + rail. Aucune clé, aucun secret, loopback seul.
func demarrerAgent(t *testing.T, format, profil string, modele string) *fixture {
	t.Helper()
	f := agents.DemarrerFactice(agents.OptionsFactice{Format: format, Modele: modele})
	t.Cleanup(f.Close)
	cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
		ID:                            "recepteur",
		Endpoint:                      f.URL,
		Modele:                        modele,
		Format:                        format,
		Profil:                        profil,
		TimeoutMS:                     5000,
		TailleMaxReponse:              1 << 20,
		RetraitsMax:                   2,
		TarifEntreeCentimesParMillion: 100,
		TarifSortieCentimesParMillion: 100,
		TarifRequeteCentimes:          1,
		JetonsEntreeMaxParTentative:   2000,
		JetonsSortieMaxParTentative:   2000,
	}}}
	var point connecteursapi.ConnecteurReel
	var err error
	if format == "anthropic" {
		point, err = cfg.NouveauAnthropicPoint("recepteur")
	} else {
		point, err = cfg.NouveauHTTPEndPoint("recepteur")
	}
	if err != nil {
		t.Fatalf("point fournisseur : %v", err)
	}
	serv, err := agents.DemarrerServeur(agents.OptionsServeur{
		Nom: "coord-agent-" + format, Jeton: "jeton-test-llm04",
		Exec: &agents.ExecInférence{Nom: "recepteur", Point: point, Enveloppe: jetonsEnveloppe},
	})
	if err != nil {
		t.Fatalf("serveur : %v", err)
	}
	t.Cleanup(serv.Fermer)
	reg := filepath.Join(t.TempDir(), "registre-rail.json")
	rail, err := coorda2a.NouveauRail(context.Background(), serv.URL(), "jeton-test-llm04", reg)
	if err != nil {
		t.Fatalf("rail : %v", err)
	}
	return &fixture{t: t, serveur: serv, rail: rail}
}

const empTete = "CRITIQUE DE PLAN "
const empVal = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func chargeEmp(marque, scenario string) string {
	c := fmt.Sprintf(`%s SCENARIO:BASE {"empreinte_attendue":"%s"}`, marque, empVal)
	if scenario != "" {
		c = fmt.Sprintf(`%s %s SCENARIO:BASE {"empreinte_attendue":"%s"}`, marque, scenario, empVal)
	}
	return c
}

// TestQ3ModeleDivergent : la réponse annonçant un modèle autre que le
// modèle EXACT déclaré est refusée fail-closed (Q3).
func TestQ3ModeleDivergent(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:DIV_MDL"), "cle-q3")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (modèle divergent : fail closed côté connecteur moteur)", env.Etat)
	}
}

// TestQ5TroncationRefusee : stop_reason max_tokens = fin non nominale =
// refus (Q5).
func TestQ5TroncationRefusee(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:TRONQUE"), "cle-q5")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (troncation)", env.Etat)
	}
}

// TestQ6UsageAbsent : usage absent du corps = JETONS_INCONNUS, jamais
// « inconnu = gratuit » (Q6).
func TestQ6UsageAbsent(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:SANS_USAGE"), "cle-q6")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (usage absent : JETONS_INCONNUS côté connecteur moteur)", env.Etat)
	}
}

// TestQ7Refus401 : refus d'authentification = appel échoué borné (Q7).
func TestQ7Refus401(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:401"), "cle-q7")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (401)", env.Etat)
	}
}

// TestQ10JetonRailExige : invocation HTTP SANS jeton du rail = 401
// sur le serveur d'agent (Q10, même contrôle que le rail M5).
func TestQ10JetonRailExige(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	rep, err := http.Post(fx.serveur.URL()+"/invoke", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"message/send","id":1,"params":{}}`))
	if err != nil {
		t.Fatalf("post : %v", err)
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("401 attendu sans jeton, obtenu %d", rep.StatusCode)
	}
}

// TestCycleAnthropicComplet : cycle nominal critique + réception +
// réception A_CORRIGER injectée par le factice (T2, Q-réception).
func TestCycleAnthropicComplet(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	ctx := context.Background()

	env, _, err := fx.rail.Deleguer(ctx, "critiquer", chargeEmp("CRITIQUE DE PLAN", ""), "cle-1")
	if err != nil {
		t.Fatalf("critique : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee {
		t.Fatalf("état %s, achevée attendue : %s", env.Etat, env.Texte)
	}
	var verdict map[string]any
	if err := json.Unmarshal([]byte(env.Texte), &verdict); err != nil {
		t.Fatalf("verdict non JSON : %v (%s)", err, env.Texte)
	}
	if verdict["verdict"] != "PLAN_ACCEPTE" || verdict["empreinte_attendue"] != empVal {
		t.Fatalf("verdict inattendu : %s", env.Texte)
	}
	if env.Jetons <= 0 {
		t.Fatalf("jetons %d, usage réel attendu", env.Jetons)
	}

	// Réception ACCEPTEE.
	env, _, err = fx.rail.Deleguer(ctx, "recevoir", chargeEmp("RECEPTION INDEPENDANTE", ""), "cle-2")
	if err != nil {
		t.Fatalf("réception : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee {
		t.Fatalf("réception : état %s", env.Etat)
	}
	if !strings.Contains(env.Texte, `"verdict":"ACCEPTEE"`) {
		t.Fatalf("verdict ACCEPTEE attendu : %s", env.Texte)
	}

	// Réception A_CORRIGER (rétroaction factice).
	env, _, err = fx.rail.Deleguer(ctx, "recevoir",
		chargeEmp("RECEPTION INDEPENDANTE", "SCENARIO:A_CORRIGER"), "cle-3")
	if err != nil {
		t.Fatalf("réception : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee || !strings.Contains(env.Texte, `"verdict":"A_CORRIGER"`) {
		t.Fatalf("A_CORRIGER attendu : %s", env.Texte)
	}
}

// TestQ16VoieVibeRefusee : VOIE:VIBE sur agent d'inférence = refus
// CAPACITE_NON_IMPLEMENTEE, jamais une simulation silencieuse (Q16).
func TestQ16VoieVibeRefusee(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "executer",
		"VOIE:VIBE MISSION=x REVISION=r1 TACHE=interdit", "cle-q16")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (CAPACITE_NON_IMPLEMENTEE)", env.Etat)
	}
}

// TestQ4ChargeSansEmpreinte : charge sans gabarit d'empreinte : la
// sortie factice n'est PAS un verdict ancré (aucun verdict fabriqué
// sans ancrage, C15 côté orchestrateur).
func TestQ4ChargeSansEmpreinte(t *testing.T) {
	fx := demarrerAgent(t, "anthropic", "", modeleAnthropic)
	env, _, err := fx.rail.Deleguer(context.Background(), "executer",
		"charge quelconque sans empreinte SCENARIO:BASE", "cle-q4")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee {
		t.Fatalf("état %s : %s", env.Etat, env.Texte)
	}
	if strings.Contains(env.Texte, "verdict") {
		t.Fatalf("aucun verdict ancré attendu sans empreinte : %s", env.Texte)
	}
}

// TestEnveloppeDepassee : jetons réels > enveloppe locale = échec NET
// (cohérence locale, pas de réservation ici).
func TestEnveloppeDepassee(t *testing.T) {
	f := agents.DemarrerFactice(agents.OptionsFactice{Format: "anthropic", Modele: modeleAnthropic})
	t.Cleanup(f.Close)
	cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
		ID: "recepteur", Endpoint: f.URL, Modele: modeleAnthropic, Format: "anthropic",
		TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 2,
		TarifEntreeCentimesParMillion: 100, TarifSortieCentimesParMillion: 100,
		TarifRequeteCentimes: 1, JetonsEntreeMaxParTentative: 2000, JetonsSortieMaxParTentative: 2000,
	}}}
	point, err := cfg.NouveauAnthropicPoint("recepteur")
	if err != nil {
		t.Fatalf("point : %v", err)
	}
	serv, err := agents.DemarrerServeur(agents.OptionsServeur{
		Nom: "coord-agent-anthropic-petit", Jeton: "jeton-test-llm04",
		Exec: &agents.ExecInférence{Nom: "recepteur", Point: point, Enveloppe: 1},
	})
	if err != nil {
		t.Fatalf("serveur : %v", err)
	}
	t.Cleanup(serv.Fermer)
	rail, err := coorda2a.NouveauRail(context.Background(), serv.URL(), "jeton-test-llm04",
		filepath.Join(t.TempDir(), "r.json"))
	if err != nil {
		t.Fatalf("rail : %v", err)
	}
	env, _, err := rail.Deleguer(context.Background(), "critiquer", chargeEmp("CRITIQUE DE PLAN", ""), "cle-env")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (ENVELOPPE_DEPASSEE)", env.Etat)
	}
}

// TestConfigurationsSansSecret : les configurations d'exemple publiées
// (coord/connecteur/<Fournisseur>) ne contiennent AUCUN secret (Q9,
// contrôle de surface des fichiers sources).
func TestConfigurationsSansSecret(t *testing.T) {
	chemins := []string{
		"../../connecteur/Claude/exemple-config-claude.json",
		"../../connecteur/OpenAI/exemple-config-openai.json",
	}
	for _, chemin := range chemins {
		b, err := os.ReadFile(chemin)
		if err != nil {
			t.Fatalf("config d'exemple illisible (%s) : %v", chemin, err)
		}
		s := string(b)
		if strings.Contains(s, "sk-") || strings.Contains(s, "AIza") {
			t.Fatalf("SECRET DÉTECTÉ dans %s : %s", chemin, s)
		}
	}
}

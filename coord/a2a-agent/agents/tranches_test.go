// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package agents_test

// tranches_test.go : T4 de M-COORD-LLM-04 — qualification DÉDIÉE des
// raccordements OpenAI (format par défaut) puis Gemini (profil stop-only,
// raisonnement borné), sur les MÊMES helpers que la tranche Anthropic :
// le socle est unique, seuls les contrôles de fin de réponse diffèrent.
// Tout est SIMULÉ : serveurs factices loopback, 0 EUR, aucune clé.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/agents"

	"coordengine/connecteursapi"
)

const (
	modeleOpenAI = "gpt-4o-mini-2024-07-18"
	modeleGemini = "gemini-2.0-flash"
)

// sousTemp : chemin de registre jetable (helper local aux tests T4).
func sousTemp(nom string) string {
	d, _ := os.MkdirTemp("", "coord-t4-*")
	return d + string(os.PathSeparator) + nom
}

// matriceT4 : couples (format, profil, modèle) qualifiés tranche par
// tranche (T2 anthropic, T4 openai puis gemini).
var matriceT4 = []struct {
	nom    string
	format string
	profil string
	modele string
}{
	{"openai", "openai", "", modeleOpenAI},
	{"gemini", "openai", "gemini", modeleGemini},
}

// TestT4CycleNominalParTranche : cycle critique+réception nominal pour
// chaque raccordement T4, verdicts ancrés empreinte, jetons réels.
func TestT4CycleNominalParTranche(t *testing.T) {
	for _, m := range matriceT4 {
		t.Run(m.nom, func(t *testing.T) {
			fx := demarrerAgent(t, m.format, m.profil, m.modele)
			ctx := context.Background()
			env, _, err := fx.rail.Deleguer(ctx, "critiquer", chargeEmp("CRITIQUE DE PLAN", ""), "cle-t4-"+m.nom)
			if err != nil {
				t.Fatalf("critique : %v", err)
			}
			if env.Etat != coorda2a.EtatAchevee {
				t.Fatalf("état %s : %s", env.Etat, env.Texte)
			}
			var verdict map[string]any
			if err := json.Unmarshal([]byte(env.Texte), &verdict); err != nil {
				t.Fatalf("verdict non JSON : %s", env.Texte)
			}
			if verdict["verdict"] != "PLAN_ACCEPTE" || verdict["empreinte_attendue"] != empVal {
				t.Fatalf("verdict inattendu : %s", env.Texte)
			}
			env, _, err = fx.rail.Deleguer(ctx, "recevoir", chargeEmp("RECEPTION INDEPENDANTE", "SCENARIO:A_CORRIGER"), "cle-t4r-"+m.nom)
			if err != nil {
				t.Fatalf("réception : %v", err)
			}
			if env.Etat != coorda2a.EtatAchevee || !strings.Contains(env.Texte, `"verdict":"A_CORRIGER"`) {
				t.Fatalf("A_CORRIGER attendu : %s", env.Texte)
			}
		})
	}
}

// TestT4FinNonNominaleParProfil : profil gemini (stop-only) = troncature
// (finish_reason length) REFUSÉE fail-closed (Q5). Format openai SANS
// profil : STATU QUO du format historique (aucun contrôle de fin, aucune
// régression — limite documentée dans le rapport, un profil stop-only
// openai serait un changement moteur hors périmètre de réutilisation).
func TestT4FinNonNominaleParProfil(t *testing.T) {
	fx := demarrerAgent(t, "openai", "gemini", modeleGemini)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:TRONQUE"), "cle-t4t-gemini")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("gemini : état %s, échouée attendue (fin non nominale, stop-only)", env.Etat)
	}
	// Statu quo openai (profil vide) : PAS de contrôle de fin au format
	// historique — le test FIGE le comportement existant du moteur.
	fx = demarrerAgent(t, "openai", "", modeleOpenAI)
	env, _, err = fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", "SCENARIO:TRONQUE"), "cle-t4t-openai")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee {
		t.Fatalf("openai (sans profil) : état %s, achevée attendue (statu quo documenté)", env.Etat)
	}
}

// TestT4ModeleDivergentRefuse : identité exacte exigée (Q3).
func TestT4ModeleDivergentRefuse(t *testing.T) {
	for _, m := range matriceT4 {
		t.Run(m.nom, func(t *testing.T) {
			fx := demarrerAgent(t, m.format, m.profil, m.modele)
			env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
				chargeEmp("CRITIQUE DE PLAN", "SCENARIO:DIV_MDL"), "cle-t4d-"+m.nom)
			if err != nil {
				t.Fatalf("deleguer : %v", err)
			}
			if env.Etat != coorda2a.EtatEchouee {
				t.Fatalf("état %s, échouée attendue (modèle divergent)", env.Etat)
			}
		})
	}
}

// TestT4UsageAbsentRefuse : usage absent = JETONS_INCONNUS (Q6).
func TestT4UsageAbsentRefuse(t *testing.T) {
	for _, m := range matriceT4 {
		t.Run(m.nom, func(t *testing.T) {
			fx := demarrerAgent(t, m.format, m.profil, m.modele)
			env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
				chargeEmp("CRITIQUE DE PLAN", "SCENARIO:SANS_USAGE"), "cle-t4u-"+m.nom)
			if err != nil {
				t.Fatalf("deleguer : %v", err)
			}
			if env.Etat != coorda2a.EtatEchouee {
				t.Fatalf("état %s, échouée attendue (usage absent)", env.Etat)
			}
		})
	}
}

// TestT4GeminiRaisonnementInconnuRefuse : profil gemini, raisonnement
// INCONNU : refus fail-closed. La validation au CHARGEMENT vit dans
// ChargerConfig (moteur, fail closed) ; ici la Config est construite
// en mémoire pour pointer le factice, la défense en profondeur du
// moteur refuse À L'APPEL (RAISONNEMENT_INCONNU, jamais de réseau).
func TestT4GeminiRaisonnementInconnuRefuse(t *testing.T) {
	f := agents.DemarrerFactice(agents.OptionsFactice{Format: "openai", Modele: modeleGemini})
	t.Cleanup(f.Close)
	cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
		ID: "gemini", Endpoint: f.URL, Modele: modeleGemini, Format: "openai",
		Profil: "gemini", Raisonnement: "xhigh-hors-bornes",
		TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 2,
		TarifEntreeCentimesParMillion: 100, TarifSortieCentimesParMillion: 100,
		TarifRequeteCentimes: 1, JetonsEntreeMaxParTentative: 2000,
		JetonsSortieMaxParTentative: 2000,
	}}}
	point, err := cfg.NouveauHTTPEndPoint("gemini")
	if err != nil {
		t.Fatalf("point : %v", err)
	}
	serv, err := agents.DemarrerServeur(agents.OptionsServeur{
		Nom: "coord-agent-gemini-ko", Jeton: "jeton-test-llm04",
		Exec: &agents.ExecInférence{Nom: "gemini", Point: point, Enveloppe: 5000},
	})
	if err != nil {
		t.Fatalf("serveur : %v", err)
	}
	t.Cleanup(serv.Fermer)
	rail, err := coorda2a.NouveauRail(context.Background(), serv.URL(), "jeton-test-llm04",
		sousTemp("reg-t4-rn.json"))
	if err != nil {
		t.Fatalf("rail : %v", err)
	}
	env, _, err := rail.Deleguer(context.Background(), "critiquer", chargeEmp("CRITIQUE DE PLAN", ""), "cle-t4rn")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatEchouee {
		t.Fatalf("état %s, échouée attendue (RAISONNEMENT_INCONNU, fail closed à l'appel)", env.Etat)
	}
}

// TestT4GeminiRaisonnementDefautJournalise : profil gemini sans
// raisonnement déclaré : défaut low ACCEPTÉ (journalisé côté moteur),
// appel nominal.
func TestT4GeminiRaisonnementDefautJournalise(t *testing.T) {
	fx := demarrerAgent(t, "openai", "gemini", modeleGemini)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", ""), "cle-t4g")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if env.Etat != coorda2a.EtatAchevee {
		t.Fatalf("état %s : %s", env.Etat, env.Texte)
	}
}

// TestT4CartesHonnêtes : chaque carte d'agent T4 déclare le skill
// d'inférence et AUCUN skill d'exécution d'outils (Q16 côté carte).
func TestT4CartesHonnêtes(t *testing.T) {
	for _, m := range matriceT4 {
		t.Run(m.nom, func(t *testing.T) {
			fx := demarrerAgent(t, m.format, m.profil, m.modele)
			c := fx.serveur.Carte
			if len(c.Skills) != 1 {
				t.Fatalf("1 skill attendu (inférence), %d déclarés", len(c.Skills))
			}
			if c.Skills[0].Name != "coord-inference" {
				t.Fatalf("skill inattendu : %s", c.Skills[0].Name)
			}
			if strings.Contains(strings.ToLower(c.Description), "execution outillee") {
				t.Fatal("la carte ne doit PAS annoncer l'exécution outillée")
			}
		})
	}
}

// TestT4SecretsJamaisTransmis : les marqueurs de SCENARIO ne
// transportent AUCUN secret ; contrôle du registre du rail (0 sk-/AIza).
func TestT4SecretsJamaisTransmis(t *testing.T) {
	fx := demarrerAgent(t, "openai", "", modeleOpenAI)
	env, _, err := fx.rail.Deleguer(context.Background(), "critiquer",
		chargeEmp("CRITIQUE DE PLAN", ""), "cle-t4s")
	if err != nil {
		t.Fatalf("deleguer : %v", err)
	}
	if strings.Contains(env.Texte, "sk-") || strings.Contains(env.Texte, "AIza") {
		t.Fatalf("secret détecté dans le résultat : %s", env.Texte)
	}
	b, err := os.ReadFile(fx.rail.Chemin())
	if err != nil {
		t.Fatalf("registre illisible : %v", err)
	}
	if strings.Contains(string(b), "sk-") || strings.Contains(string(b), "AIza") {
		t.Fatal("secret détecté dans le registre durable du rail")
	}
}

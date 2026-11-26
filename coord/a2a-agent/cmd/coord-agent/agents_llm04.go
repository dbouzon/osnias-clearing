// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// agents_llm04.go : raccordement Anthropic / OpenAI / Gemini au MÊME
// socle serveur A2A (M-COORD-LLM-04, issue #56). Voie INFÉRENCE
// uniquement : aucune clé, aucun compte, aucun réseau fournisseur en
// qualification — le point connecteur MOTEUR (via connecteursapi,
// arbitrage C') pointe vers un FACTICE de format loopback démarré par
// ce même binaire. La qualification RÉELLE est une recette DISTINCTE
// (triple verrou, autorisation écrite du pilote).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/agents"

	"coordengine/connecteursapi"
)

// declarationFournisseur : paramètres de qualification simulée par
// fournisseur (modèle exact échoyé par le factice, enveloppe locale).
type declarationFournisseur struct {
	format string
	profil string
	modele string
}

// fournisseursLLM04 : les trois raccordements de la mission. Mistral
// (déjà raccordé en #54) reste servi par le code historique de main.go :
// ces déclarations ne le remplacent PAS.
var fournisseursLLM04 = map[string]declarationFournisseur{
	"anthropic": {format: "anthropic", profil: "", modele: "claude-3-5-sonnet-20241022"},
	"openai":    {format: "openai", profil: "", modele: "gpt-4o-mini-2024-07-18"},
	"gemini":    {format: "openai", profil: "gemini", modele: "gemini-2.0-flash"},
}

// demarrerAgentFournisseur : factice de format + point connecteur
// MOTEUR + socle + (option) écoute. Loopback seul, jeton du rail exigé.
func demarrerAgentFournisseur(nom string, d declarationFournisseur, jeton string, ecouter bool) error {
	if jeton == "" {
		return fmt.Errorf("jeton du rail requis (fail closed)")
	}
	factice := agents.DemarrerFactice(agents.OptionsFactice{Format: d.format, Modele: d.modele})
	defer factice.Close()
	cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
		ID:                            "recepteur",
		Endpoint:                      factice.URL,
		Modele:                        d.modele,
		Format:                        d.format,
		Profil:                        d.profil,
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
	if d.format == "anthropic" {
		point, err = cfg.NouveauAnthropicPoint("recepteur")
	} else {
		point, err = cfg.NouveauHTTPEndPoint("recepteur")
	}
	if err != nil {
		return err
	}
	serv, err := agents.DemarrerServeur(agents.OptionsServeur{
		Nom: "coord-agent-" + nom, Jeton: jeton,
		Exec: &agents.ExecInférence{Nom: "recepteur", Point: point, Enveloppe: 5000},
	})
	if err != nil {
		return err
	}
	fmt.Println("AGENT_PRET :", serv.URL())
	if ecouter {
		select {}
	}
	serv.Fermer()
	return nil
}

// demoInférenceQuatre : démonstration autonome des raccordements
// d'inférence : UN cycle critique complet par fournisseur (factice de
// format loopback, vraie route A2A → agents → connecteur moteur → HTTP,
// 0 EUR). Ne touche PAS au comportement historique de -demo-local.
func demoInférenceQuatre() error {
	ctx := context.Background()
	const emp = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, nom := range []string{"anthropic", "openai", "gemini"} {
		d := fournisseursLLM04[nom]
		factice := agents.DemarrerFactice(agents.OptionsFactice{Format: d.format, Modele: d.modele})
		cfg := connecteursapi.Config{Fournisseurs: []connecteursapi.ConfigFournisseur{{
			ID: "recepteur", Endpoint: factice.URL, Modele: d.modele,
			Format: d.format, Profil: d.profil,
			TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 2,
			TarifEntreeCentimesParMillion: 100, TarifSortieCentimesParMillion: 100,
			TarifRequeteCentimes: 1, JetonsEntreeMaxParTentative: 2000,
			JetonsSortieMaxParTentative: 2000,
		}}}
		var point connecteursapi.ConnecteurReel
		var err error
		if d.format == "anthropic" {
			point, err = cfg.NouveauAnthropicPoint("recepteur")
		} else {
			point, err = cfg.NouveauHTTPEndPoint("recepteur")
		}
		if err != nil {
			factice.Close()
			return fmt.Errorf("%s : %w", nom, err)
		}
		serv, err := agents.DemarrerServeur(agents.OptionsServeur{
			Nom: "coord-agent-" + nom, Jeton: "jeton-demo-llm04",
			Exec: &agents.ExecInférence{Nom: "recepteur", Point: point, Enveloppe: 5000},
		})
		if err != nil {
			factice.Close()
			return err
		}
		reg := filepath.Join(os.TempDir(), fmt.Sprintf("coord-agent-demo-%s.json", nom))
		rail, err := coorda2a.NouveauRail(ctx, serv.URL(), "jeton-demo-llm04", reg)
		if err != nil {
			serv.Fermer()
			factice.Close()
			return err
		}
		charge := fmt.Sprintf("CRITIQUE DE PLAN SCENARIO:BASE {\"empreinte_attendue\":\"%s\"}", emp)
		env, _, err := rail.Deleguer(ctx, "critiquer", charge, fmt.Sprintf("demo-llm04-%s", nom))
		if err != nil {
			serv.Fermer()
			factice.Close()
			return err
		}
		serv.Fermer()
		factice.Close()
		if env.Etat != coorda2a.EtatAchevee {
			return fmt.Errorf("%s : état %s : %s", nom, env.Etat, env.Texte)
		}
		var verdict map[string]any
		if err := json.Unmarshal([]byte(env.Texte), &verdict); err != nil {
			return fmt.Errorf("%s : verdict non JSON : %s", nom, env.Texte)
		}
		if verdict["verdict"] != "PLAN_ACCEPTE" {
			return fmt.Errorf("%s : verdict inattendu : %s", nom, env.Texte)
		}
		if !strings.HasPrefix(verdict["empreinte_attendue"].(string), "0123456") {
			return fmt.Errorf("%s : ancrage empreinte inattendu", nom)
		}
		fmt.Printf("INFERENCE_OK : %s (jetons %d, verdict %s)\n", nom, env.Jetons, env.Texte)
	}
	fmt.Println("DEMO_LLM04_OK : trois raccordements d'inférence exercés de bout en bout, 0 EUR, aucune clé.")
	return nil
}

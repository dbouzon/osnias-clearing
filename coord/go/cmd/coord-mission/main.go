// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-mission conduit une mission inter-IA COORD sur connecteurs réels
// (M-COORD-PILOT-03, issue #29). Deux usages :
//
//	-demo-local         : démo AUTONOME sur serveurs factices loopback,
//	                      HORS RÉSEAU, aucune clé, 0 €. Rejouable partout.
//	-config <fichier>   : mission réelle contre les fournisseurs déclarés.
//	                      Entrée exclusivement possible si le triple verrou
//	                      tient (mode REEL + COORD_MODE_REEL=1 + clés
//	                      d'environnement + autorisation pilote explicite).
//	                      Sinon : NON_EXECUTE + procédure (exit 3), jamais
//	                      une simulation de succès réel.
//
// Aucune clôture automatique : la mission s'arrête à ACK_RECU ; la
// clôture reste un acte du pilote humain.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

func main() {
	demoLocal := flag.Bool("demo-local", false, "démo autonome sur serveurs factices loopback (0 €, hors réseau)")
	configChemin := flag.String("config", "", "fichier de configuration locale (jamais avec un secret dedans)")
	dataDir := flag.String("data", "coord-mission-data", "dossier du référentiel missions")
	issue := flag.String("issue", "", "référence de la mission (ex. M-COORD-XX-01)")
	executant := flag.String("executant", "mistral-exec", "identité exécutante")
	recepteur := flag.String("recepteur", "openai-recep", "identité réceptionnaire")
	planChemin := flag.String("plan", "", "fichier du plan à critiquer")
	mandatChemin := flag.String("mandat", "", "fichier du mandat d'exécution")
	livrableChemin := flag.String("livrable", "", "fichier du livrable soumis (réception)")
	flag.Parse()

	if *demoLocal {
		os.Exit(demoLocalRun(*dataDir))
	}
	if *configChemin == "" {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : -config ou -demo-local requis")
		os.Exit(3)
	}
	// Le cycle réel n'est JAMAIS un succès simulé : sans porte ouverte il
	// refuse et imprime la procédure locale (exit 3).
	os.Exit(reelRun(*configChemin, *dataDir, *issue, *executant, *recepteur,
		*planChemin, *mandatChemin, *livrableChemin))
}

// ---- démo locale : serveurs factices loopback ----

func demoLocalRun(dataDir string) int {
	rep, err := os.MkdirTemp("", "coord-mission-demo")
	if err != nil {
		fmt.Fprintln(os.Stderr, "dossier temporaire impossible : "+err.Error())
		return 1
	}
	_ = os.MkdirAll(dataDir, 0o755)
	ref, err := missions.Ouvrir(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "référentiel impossible : "+err.Error())
		return 1
	}

	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repondreFactice(w, "LIVRABLE 1\nLIVRABLE 2\nLIVRABLE 3 (démo locale, serveur factice loopback)")
	}))
	defer exec.Close()
	recep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var corps struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&corps)
		contenu := ""
		if len(corps.Messages) > 0 {
			contenu = corps.Messages[len(corps.Messages)-1].Content
		}
		emp := extraireEmpreinte(contenu)
		var out []byte
		if strings.Contains(contenu, "CRITIQUE DE PLAN") {
			out, _ = json.Marshal(map[string]any{
				"verdict": "PLAN_ACCEPTE", "points": []string{"démo : plan simple, rien de bloquant"},
				"empreinte_attendue": emp,
			})
		} else {
			out, _ = json.Marshal(map[string]any{
				"verdict": "ACCEPTEE", "points": []string{"démo : 3 lignes présentes"},
				"empreinte_attendue": emp,
				"verdict_id":         "demo-verdict", "verdict_url": "urn:coord:demo",
			})
		}
		repondreFactice(w, string(out))
	}))
	defer recep.Close()

	// Défaut corrigé en cycle M-COORD-PILOT-03 (découvert à la reproduction
	// locale) : les serveurs factices répondent "model":"demo" ; un modèle
	// DIVERGENT serait refusé MODELE_DIVERGENT avant la première étape. Les
	// modèles DÉCLARÉS valent donc exactement le modèle répondu : le contrôle
	// fail-closed reste actif, la démo déroule son cycle complet.
	cfg := &connecteurs.Config{Mode: connecteurs.ModeSimule, Fournisseurs: []connecteurs.ConfigFournisseur{
		{ID: "mistral", Endpoint: exec.URL, Modele: "demo", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 1},
		{ID: "openai", Endpoint: recep.URL, Modele: "demo", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 1},
	}}
	reg, err := registreDepuisConfig(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "registre factice impossible : "+err.Error())
		return 1
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orchestrateur impossible : "+err.Error())
		return 1
	}
	issue := "demo-local-" + fmt.Sprint(time.Now().UTC().Unix())
	plafonds := budget.Plafonds{
		Appels: map[string]int{"mistral": 20, "openai": 20},
		Jetons: map[string]int{"mistral": 200000, "openai": 200000},
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", roles, plafonds); err != nil {
		fmt.Fprintln(os.Stderr, "création impossible : "+err.Error())
		return 1
	}
	ctx := context.Background()
	fmt.Println("[DEMO-LOCAL] issue : " + issue)
	fmt.Println("[DEMO-LOCAL] serveurs factices loopback, hors réseau, aucune clé, 0 €.")
	fmt.Println("[DEMO-LOCAL] état : " + etatToString(ref, issue))

	plan := "PLAN DE DEMO : rédiger un résumé en 3 lignes, critères : 3 lignes, pas de secret, factuel."
	v, err := orch.CritiquerPlan(ctx, issue, "openai-recep", plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, "critique de plan échouée : "+err.Error())
		return 1
	}
	fmt.Println("[DEMO-LOCAL] critique de plan : " + v.Verdict)
	if v.Verdict != "PLAN_ACCEPTE" {
		fmt.Fprintln(os.Stderr, "plan non accepté en démo : boucle de correction laissée au pilote")
		return 2
	}
	resExec, err := orch.Executer(ctx, issue, "mistral-exec",
		"Rédige UN résumé factuel du protocole COORD en exactement 3 lignes, sans secret.")
	if err != nil {
		fmt.Fprintln(os.Stderr, "exécution échouée : "+err.Error())
		return 1
	}
	fmt.Println("[DEMO-LOCAL] état : " + etatToString(ref, issue))
	if _, err := orch.Soumettre(ctx, issue, "mistral-exec", resExec.Sortie); err != nil {
		fmt.Fprintln(os.Stderr, "soumission échouée : "+err.Error())
		return 1
	}
	fmt.Println("[DEMO-LOCAL] état : " + etatToString(ref, issue))
	verdict, _, err := orch.Recevoir(ctx, issue, "openai-recep", resExec.Sortie)
	if err != nil {
		fmt.Fprintln(os.Stderr, "réception échouée : "+err.Error())
		return 1
	}
	fmt.Println("[DEMO-LOCAL] réception : " + verdict.Verdict + " (" + verdict.VerdictID + ")")
	if verdict.Verdict != "ACCEPTEE" {
		fmt.Println("[DEMO-LOCAL] verdict non accepté : correction attendue (pas de clôture automatique)")
		return 2
	}
	if err := orch.Acker(issue, "mistral-exec"); err != nil {
		fmt.Fprintln(os.Stderr, "ack échoué : "+err.Error())
		return 1
	}
	fmt.Println("[DEMO-LOCAL] état : " + etatToString(ref, issue))
	fmt.Println("[DEMO-LOCAL] journal : " + missions.JournalPath(dataDir, issue))
	fmt.Println("[DEMO-LOCAL] dossier de travail temporaire : " + rep)
	fmt.Println("[DEMO-LOCAL] BILAN : cycle complet REEL-sur-connecteurs-factices, état attendu ACK_RECU. La clôture reste réservée au pilote.")
	if etatToString(ref, issue) != spec.EtatACKRecu {
		return 1
	}
	return 0
}

func repondreFactice(w http.ResponseWriter, content string) {
	b, _ := json.Marshal(map[string]any{
		"model": "demo",
		"choices": []any{map[string]any{
			"message": map[string]any{"content": content},
		}},
		"usage": map[string]any{"total_tokens": 421},
	})
	w.Write(b)
}

func extraireEmpreinte(contenu string) string {
	idx := strings.Index(contenu, `empreinte_attendue":"`)
	if idx < 0 {
		return ""
	}
	reste := contenu[idx+len(`empreinte_attendue":"`):]
	fin := strings.Index(reste, `"`)
	if fin != 64 || len(reste) < 64 {
		return ""
	}
	return reste[:64]
}

func etatToString(ref *missions.Referentiel, issue string) string {
	enr, err := ref.Obtenir(issue)
	if err != nil {
		return "INCONNU"
	}
	return enr.Mission.State
}

// ---- cycle réel, verrouillé par décisions du pilote ----

func registreDepuisConfig(cfg *connecteurs.Config) (map[string]adaptateurs.Adaptateur, error) {
	ids := make([]string, 0, len(cfg.Fournisseurs))
	for _, f := range cfg.Fournisseurs {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	reg := map[string]adaptateurs.Adaptateur{}
	for _, id := range ids {
		f, errF := cfg.Fournisseur(id)
		if errF != nil {
			return nil, errF
		}
		var adapt adaptateurs.Adaptateur
		switch f.FormatEffectif() {
		case connecteurs.FormatAnthropic:
			ap, errA := cfg.NouveauAnthropicPoint(id)
			if errA != nil {
				return nil, errA
			}
			adapt = ap
		default: // "openai" (valeur implicite, plan v2 D4)
			hp, errH := cfg.NouveauHTTPEndPoint(id)
			if errH != nil {
				return nil, errH
			}
			adapt = hp
		}
		reg[id] = adapt
	}
	return reg, nil
}

func reelRun(configChemin, dataDir, issue, executant, recepteur,
	planChemin, mandatChemin, livrableChemin string) int {

	cfg, err := connecteurs.ChargerConfig(configChemin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : configuration refusée : "+err.Error())
		return 3
	}
	if cfg.Mode != connecteurs.ModeReel {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : mode REEL requis pour une mission réelle "+
			"(une configuration SIMULE ne conduit jamais de mission présentée comme réelle, fail closed)")
		return 3
	}
	reg, err := registreDepuisConfig(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : connecteurs refusés (verrous réels) : "+err.Error())
		fmt.Fprintln(os.Stderr, "Procédure locale : 1) placer les clés dans les variables "+
			"indiquées par cle_env (jamais dans un fichier), 2) exporter "+connecteurs.EnvModeReel+
			"=1, 3) renseigner l'autorisation du pilote dans la configuration, 4) relancer.")
		return 3
	}
	if issue == "" {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : -issue requis")
		return 3
	}
	ref, err := missions.Ouvrir(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "référentiel impossible : "+err.Error())
		return 1
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orchestrateur impossible : "+err.Error())
		return 1
	}
	ctx := context.Background()
	plafonds := budget.Plafonds{
		Appels: map[string]int{},
		Jetons: map[string]int{},
		// Voûte monétaire (réserve 7154948522) : le plafond monétaire de
		// l'autorisation pilote est TRANSMIS au moteur budgétaire et opposé
		// AVANT chaque tentative facturable. Sans lui aucune mission réelle
		// ne peut démarrer (fail closed côté orchestrateur).
		Centimes: cfg.Autorisation.PlafondCentimes,
	}
	for id := range reg {
		plafonds.Appels[id] = cfg.Autorisation.PlafondAppels
		plafonds.Jetons[id] = cfg.Autorisation.PlafondJetons
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	if err := orch.NouvelleMission(issue, executant, recepteur, roles, plafonds); err != nil {
		if !strings.Contains(err.Error(), "deja existante") {
			fmt.Fprintln(os.Stderr, "création de mission refusée : "+err.Error())
			return 1
		}
		fmt.Println("Note : mission déjà existante, étapes poursuivies sur l'état courant.")
	}
	etapeSuivante := planChemin != ""
	if etapeSuivante {
		plan, err := os.ReadFile(planChemin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "plan illisible : "+err.Error())
			return 3
		}
		v, err := orch.CritiquerPlan(ctx, issue, recepteur, string(plan))
		if err != nil {
			fmt.Fprintln(os.Stderr, "critique de plan échouée : "+err.Error())
			return 1
		}
		fmt.Println("CRITIQUE_PLAN : " + v.Verdict)
		if v.Verdict != "PLAN_ACCEPTE" {
			fmt.Println("Plan non accepté : appliquer les points et relancer (A_CORRIGER).")
			return 2
		}
	}
	if mandatChemin != "" {
		mandat, err := os.ReadFile(mandatChemin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mandat illisible : "+err.Error())
			return 3
		}
		if _, err := orch.Executer(ctx, issue, executant, string(mandat)); err != nil {
			fmt.Fprintln(os.Stderr, "exécution échouée : "+err.Error())
			return 1
		}
		fmt.Println("ETAT : " + etatToString(ref, issue))
	}
	if livrableChemin != "" {
		livrable, err := os.ReadFile(livrableChemin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "livrable illisible : "+err.Error())
			return 3
		}
		if _, err := orch.Soumettre(ctx, issue, executant, string(livrable)); err != nil {
			fmt.Fprintln(os.Stderr, "soumission échouée : "+err.Error())
			return 1
		}
		fmt.Println("ETAT : " + etatToString(ref, issue))
		v, _, err := orch.Recevoir(ctx, issue, recepteur, string(livrable))
		if err != nil {
			fmt.Fprintln(os.Stderr, "réception échouée : "+err.Error())
			return 1
		}
		fmt.Println("RECEPTION : " + v.Verdict + " (" + v.VerdictID + ")")
		if v.Verdict != "ACCEPTEE" {
			fmt.Println("Verdict non accepté : corriger puis relancer (A_CORRIGER).")
			return 2
		}
		if err := orch.Acker(issue, executant); err != nil {
			fmt.Fprintln(os.Stderr, "ack échoué : "+err.Error())
			return 1
		}
		fmt.Println("ETAT : " + etatToString(ref, issue))
	}
	if err := orch.Cloturer(issue); err != nil {
		fmt.Println("CLOTURE : " + err.Error())
	}
	return 0
}

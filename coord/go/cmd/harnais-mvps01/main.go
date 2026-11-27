// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// harnais-mvps01 : harnais DE RECETTE M-COORD-MISTRAL-VPS-01 (issue #78).
// Pilote de séquence minimal conduisant le moteur coordengine (main
// 0a869f4e) via ses packages réels, UNE TRANSITION PAR PROCESSUS : chaque
// invocation relit l'état persisté du référentiel. Intervenants = le pont
// coord.llm.v1 en mode factice (bridge.py --factice, loopback 127.0.0.1
// EXPLICITE) : exécutant factice/exec/mvps01, réceptionnaire
// factice/recep/mvps01. Mode SIMULE, HORS RÉSEAU, aucune clé réelle, 0 €.
//
//	-mode creer     : NouvelleMission (note « déjà existante » en reprise)
//	-mode critiquer : CritiquerPlan (-plan fichier), PLAN_ACCEPTE attendu
//	-mode executer  : Executer (-mandat fichier), livrable vers -sortie
//	-mode soumettre : Soumettre (-livrable fichier)
//	-mode recevoir  : Recevoir (-livrable fichier), verdict + points
//	-mode acker     : Acker
//	-mode cloturer  : tentative de clôture (CLOTURE_REFUSEE attendue)
//	-mode resoudre  : résolution pilote d'un appel incertain (-resolution,
//	                  -motif) via ResoudreAppelEnCours
//	-mode rejeu     : rejeu EXPLICITE de la dernière opération (AvecRejeu
//	                  « derniere »), même action et même charge : la réponse
//	                  est re-délivrée par le registre SANS nouvel appel
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/missions"
)

func etatToString(ref *missions.Referentiel, issue string) string {
	enr, err := ref.Obtenir(issue)
	if err != nil {
		return "INCONNU"
	}
	return enr.Mission.State
}

func lireFichier(chemin string) (string, int) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return "", 3
	}
	return string(b), 0
}

func main() {
	data := flag.String("data", "", "dossier du référentiel missions (state/mvps01)")
	issue := flag.String("issue", "", "identifiant de la mission")
	mode := flag.String("mode", "", "creer|critiquer|executer|soumettre|recevoir|acker|cloturer|resoudre|rejeu")
	plan := flag.String("plan", "", "fichier du plan à critiquer")
	mandat := flag.String("mandat", "", "fichier du mandat d'exécution")
	livrable := flag.String("livrable", "", "fichier du livrable soumis")
	sortie := flag.String("sortie", "", "fichier où écrire le livrable produit (executer, rejeu)")
	resolution := flag.String("resolution", "", "résolution pilote (resoudre)")
	motif := flag.String("motif", "", "motif de la résolution (resoudre)")
	pontEP := flag.String("pont", "http://127.0.0.1:8790/v1/chat/completions", "pont coord.llm.v1 (loopback explicite)")
	modeleExec := flag.String("modele-exec", "factice/exec/mvps01", "modèle déclaré de l'exécutant (identité)")
	modeleRecep := flag.String("modele-recep", "factice/recep/mvps01", "modèle déclaré du réceptionnaire (identité)")
	profilExec := flag.String("profil-exec", "", "profil applicatif de l'exécutant (ex. mistral, stop-only)")
	retraits := flag.Int("retraits", 1, "retraits_max de l'exécutant (bornes de reprise côté moteur)")
	rejeuCible := flag.String("rejeu-cible", "derniere", "cible du rejeu : id_operation explicite ou 'derniere'")
	plafondAppels := flag.Int("plafond-appels", 20, "plafond d'appels par fournisseur (creer)")
	plafondJetons := flag.Int("plafond-jetons", 200000, "plafond de jetons par fournisseur (creer)")
	flag.Parse()
	if *data == "" || *issue == "" || *mode == "" {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : -data, -issue et -mode requis")
		os.Exit(3)
	}
	ref, err := missions.Ouvrir(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "référentiel impossible : "+err.Error())
		os.Exit(1)
	}
	cfg := &connecteurs.Config{Mode: connecteurs.ModeSimule, Fournisseurs: []connecteurs.ConfigFournisseur{
		{ID: "pont-exec", Endpoint: *pontEP, Format: "openai",
			Modele: *modeleExec, CleEnv: "COORD_BRIDGE_JETON", Profil: *profilExec,
			TimeoutMS: 5000, TailleMaxReponse: 65536, RetraitsMax: *retraits},
		{ID: "pont-recep", Endpoint: *pontEP, Format: "openai",
			Modele: *modeleRecep, CleEnv: "COORD_BRIDGE_JETON",
			TimeoutMS: 5000, TailleMaxReponse: 65536, RetraitsMax: 1},
	}}
	ids := []string{"pont-exec", "pont-recep"}
	sort.Strings(ids)
	reg := map[string]adaptateurs.Adaptateur{}
	for _, id := range ids {
		hp, errH := cfg.NouveauHTTPEndPoint(id)
		if errH != nil {
			fmt.Fprintln(os.Stderr, "connecteur impossible : "+errH.Error())
			os.Exit(1)
		}
		reg[id] = hp
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orchestrateur impossible : "+err.Error())
		os.Exit(1)
	}
	ctx := context.Background()
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}

	switch *mode {
	case "creer":
		plafonds := budget.Plafonds{
			Appels: map[string]int{"pont-exec": *plafondAppels, "pont-recep": *plafondAppels},
			Jetons: map[string]int{"pont-exec": *plafondJetons, "pont-recep": *plafondJetons},
		}
		if err := orch.NouvelleMission(*issue, "mvps01-exec", "mvps01-recep", roles, plafonds); err != nil {
			if strings.Contains(err.Error(), "deja existante") {
				fmt.Println("Note : mission déjà existante, étapes poursuivies sur l'état courant.")
				fmt.Println("ETAT : " + etatToString(ref, *issue))
				return
			}
			fmt.Fprintln(os.Stderr, "création de mission refusée : "+err.Error())
			os.Exit(1)
		}
		fmt.Println("CREATION : " + *issue)
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "critiquer":
		contenu, rc := lireFichier(*plan)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -plan requis et lisible")
			os.Exit(3)
		}
		v, errC := orch.CritiquerPlan(ctx, *issue, "mvps01-recep", contenu)
		if errC != nil {
			fmt.Fprintln(os.Stderr, "critique de plan échouée : "+errC.Error())
			os.Exit(1)
		}
		fmt.Println("CRITIQUE_PLAN : " + v.Verdict)
		if v.Verdict != "PLAN_ACCEPTE" {
			os.Exit(2)
		}
	case "executer":
		contenu, rc := lireFichier(*mandat)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -mandat requis et lisible")
			os.Exit(3)
		}
		res, errE := orch.Executer(ctx, *issue, "mvps01-exec", contenu)
		if errE != nil {
			fmt.Fprintln(os.Stderr, "exécution échouée : "+errE.Error())
			os.Exit(1)
		}
		if *sortie != "" {
			if errW := os.WriteFile(*sortie, []byte(res.Sortie), 0o640); errW != nil {
				fmt.Fprintln(os.Stderr, "écriture du livrable impossible : "+errW.Error())
				os.Exit(1)
			}
		}
		fmt.Println("EXECUTION : livrable produit")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "soumettre":
		contenu, rc := lireFichier(*livrable)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -livrable requis et lisible")
			os.Exit(3)
		}
		if _, errS := orch.Soumettre(ctx, *issue, "mvps01-exec", contenu); errS != nil {
			fmt.Fprintln(os.Stderr, "soumission échouée : "+errS.Error())
			os.Exit(1)
		}
		fmt.Println("SOUMISSION : révision commitée")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "recevoir":
		contenu, rc := lireFichier(*livrable)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -livrable requis et lisible")
			os.Exit(3)
		}
		v, _, errR := orch.Recevoir(ctx, *issue, "mvps01-recep", contenu)
		if errR != nil {
			fmt.Fprintln(os.Stderr, "réception échouée : "+errR.Error())
			os.Exit(1)
		}
		fmt.Println("RECEPTION : " + v.Verdict + " (" + v.VerdictID + ")")
		fmt.Println("POINTS : " + strings.Join(v.Points, " | "))
		if v.Verdict != "ACCEPTEE" {
			os.Exit(2)
		}
	case "acker":
		if errA := orch.Acker(*issue, "mvps01-exec"); errA != nil {
			fmt.Fprintln(os.Stderr, "ack échoué : "+errA.Error())
			os.Exit(1)
		}
		fmt.Println("ACK : acquitté par l'exécutant")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "cloturer":
		errCl := orch.Cloturer(*issue)
		if errCl == nil {
			fmt.Fprintln(os.Stderr, "ANOMALIE : clôture acceptée par l'automatisme")
			os.Exit(1)
		}
		fmt.Println("CLOTURE : " + errCl.Error())
		os.Exit(2)
	case "resoudre":
		if errRes := orch.ResoudreAppelEnCours(*issue, *resolution, *motif, "pilote-mvps01"); errRes != nil {
			fmt.Fprintln(os.Stderr, "résolution échouée : "+errRes.Error())
			os.Exit(1)
		}
		fmt.Println("RESOLUTION : " + *resolution + " (" + *motif + ")")
	case "rejeu":
		contenu, rc := lireFichier(*mandat)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -mandat requis et lisible (même charge que l'opération rejouée)")
			os.Exit(3)
		}
		orchR := orch.AvecRejeu(*rejeuCible)
		res, errRj := orchR.Executer(ctx, *issue, "mvps01-exec", contenu)
		if errRj != nil {
			fmt.Fprintln(os.Stderr, "rejeu échoué : "+errRj.Error())
			os.Exit(1)
		}
		if *sortie != "" {
			if errW := os.WriteFile(*sortie, []byte(res.Sortie), 0o640); errW != nil {
				fmt.Fprintln(os.Stderr, "écriture du livrable impossible : "+errW.Error())
				os.Exit(1)
			}
		}
		fmt.Println("REJEU : réponse re-délivrée par le registre (opération terminée)")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	default:
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : mode inconnu "+*mode)
		os.Exit(3)
	}
}

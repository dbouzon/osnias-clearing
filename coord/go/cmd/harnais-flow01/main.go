// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// harnais-flow01 : harnais DE RECETTE M-COORD-FLOW-01 (issue #76, plan v2).
// Pilote de séquence minimal conduisant le moteur coord-mission via ses
// packages réels, UNE TRANSITION PAR PROCESSUS : chaque invocation relit
// l'état persisté du référentiel. Intervenants = deux serveurs factices
// loopback format openai (adresses 127.0.0.1 EXPLICITES, démarrés séparément
// comme processus coord, JAMAIS tués avec le coordinateur). Mode SIMULE,
// HORS RÉSEAU, aucune clé, 0 €.
//
// T3 (plan v2) : -temoin <fichier> inscrit AVANT toute action le PID et le
// PGID RÉELS du processus dans un fichier témoin ; le lanceur de recette
// vérifie l'appartenance (/proc/<pid>/stat, PGID, ligne de commande) AVANT
// tout signal, et l'arrêt est TOUJOURS ciblé par PID (jamais de groupe).
//
//	-mode creer     : NouvelleMission (note « déjà existante » en reprise)
//	-mode critiquer : CritiquerPlan (-plan fichier), PLAN_ACCEPTE attendu
//	-mode executer  : Executer (-mandat fichier), livrable vers -sortie
//	-mode soumettre : Soumettre (-livrable fichier)
//	-mode recevoir  : Recevoir (-livrable fichier), verdict + points
//	-mode acker     : Acker
//	-mode cloturer  : tentative de clôture (CLOTURE_REFUSEE attendue)
//	-mode resoudre  : décision pilote sur appel non résolu (-resolution
//	                  RECU|PERDU|ECARTE|ECHOUE -motif texte)
//	-mode consulter : JSON mission sur stdout, opérations sur stderr
//	-mode compter   : comptage des opérations (JSON sur stdout)
//	-mode derniere  : id_operation de la dernière opération ouverte
//	-mode vivant    : processus coordinateur vivant (cible de SIGKILL),
//	                  dort -secondes (défaut 900)
//
//	-rejeu <id|derniere> : rejeu explicite (contrat D5 validé AVANT tout
//	                       appel ; refus traçable, zéro effet si terminée).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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

// ecrireTemoin : inscrit PID et PGID RÉELS du processus (T3 : le lanceur
// vérifie l'appartenance avant tout signal, arrêt ciblé par PID).
func ecrireTemoin(chemin, mode string) error {
	pid := os.Getpid()
	pgid, err := pgidProcessus(pid)
	if err != nil {
		return err
	}
	ligne := fmt.Sprintf("PID=%d\nPGID=%d\nMODE=%s\nINSTANT=%s\n",
		pid, pgid, mode, time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(chemin, []byte(ligne), 0o640)
}

func main() {
	data := flag.String("data", "", "dossier du référentiel missions (state/flow01)")
	issue := flag.String("issue", "", "identifiant de la mission")
	mode := flag.String("mode", "", "creer|critiquer|executer|soumettre|recevoir|acker|cloturer|resoudre|consulter|compter|derniere|vivant")
	plan := flag.String("plan", "", "fichier du plan à critiquer")
	mandat := flag.String("mandat", "", "fichier du mandat d'exécution")
	livrable := flag.String("livrable", "", "fichier du livrable soumis")
	sortie := flag.String("sortie", "", "fichier où écrire le livrable produit (executer)")
	resolution := flag.String("resolution", "", "décision pilote : RECU|PERDU|ECARTE|ECHOUE (mode resoudre)")
	motif := flag.String("motif", "", "motif de la décision pilote (mode resoudre)")
	rejeu := flag.String("rejeu", "", "rejeu explicite : id_operation ou derniere")
	temoin := flag.String("temoin", "", "fichier témoin PID/PGID (T3, avant toute action)")
	secondes := flag.Int("secondes", 900, "durée de vie du mode vivant")
	execEP := flag.String("exec-endpoint", "http://127.0.0.1:8093/", "serveur factice exécutant (loopback explicite)")
	recepEP := flag.String("recep-endpoint", "http://127.0.0.1:8094/", "serveur factice réceptionnaire (loopback explicite)")
	flag.Parse()
	if *temoin != "" {
		if err := ecrireTemoin(*temoin, *mode); err != nil {
			fmt.Fprintln(os.Stderr, "témoin impossible : "+err.Error())
			os.Exit(1)
		}
	}
	if *data == "" || *issue == "" || *mode == "" {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : -data, -issue et -mode requis")
		os.Exit(3)
	}
	ref, err := missions.Ouvrir(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "référentiel impossible : "+err.Error())
		os.Exit(1)
	}
	switch *mode {
	case "consulter":
		// Lecture seule (DEP-02) : JSON pur sur stdout, opérations sur stderr.
		enr, errC := ref.Obtenir(*issue)
		if errC != nil {
			fmt.Fprintln(os.Stderr, "mission introuvable : "+errC.Error())
			os.Exit(1)
		}
		out, errM := json.MarshalIndent(enr.Mission, "", "  ")
		if errM != nil {
			fmt.Fprintln(os.Stderr, "lecture impossible : "+errM.Error())
			os.Exit(1)
		}
		fmt.Println(string(out))
		st, errO := connecteurs.ResumeOperations(*data, *issue)
		if errO != nil {
			fmt.Fprintln(os.Stderr, "OPERATIONS : "+errO.Error())
			os.Exit(1)
		}
		outOps, _ := json.MarshalIndent(st, "", "  ")
		fmt.Fprintln(os.Stderr, "--- OPERATIONS ---")
		fmt.Fprintln(os.Stderr, string(outOps))
		return
	case "compter":
		st, errO := connecteurs.ResumeOperations(*data, *issue)
		if errO != nil {
			fmt.Fprintln(os.Stderr, "OPERATIONS : "+errO.Error())
			os.Exit(1)
		}
		out, _ := json.Marshal(st)
		fmt.Println(string(out))
		return
	case "derniere":
		evs, errL := connecteurs.LireOperations(*data, *issue)
		if errL != nil {
			fmt.Fprintln(os.Stderr, "OPERATIONS : "+errL.Error())
			os.Exit(1)
		}
		derniere := ""
		for _, ev := range evs {
			if ev.Type == connecteurs.TypeOperationOuverte {
				derniere = ev.IdOperation
			}
		}
		if derniere == "" {
			fmt.Fprintln(os.Stderr, "AUCUNE_OPERATION")
			os.Exit(1)
		}
		fmt.Println(derniere)
		return
	}
	cfg := &connecteurs.Config{Mode: connecteurs.ModeSimule, Fournisseurs: []connecteurs.ConfigFournisseur{
		{ID: "mistral", Endpoint: *execEP, Modele: "flow01-demo", TimeoutMS: 30000,
			TailleMaxReponse: 65536, RetraitsMax: 1},
		{ID: "openai", Endpoint: *recepEP, Modele: "flow01-demo", TimeoutMS: 30000,
			TailleMaxReponse: 65536, RetraitsMax: 1},
	}}
	ids := []string{"mistral", "openai"}
	sort.Strings(ids)
	reg := map[string]adaptateurs.Adaptateur{}
	for _, id := range ids {
		hp, errH := cfg.NouveauHTTPEndPoint(id)
		if errH != nil {
			fmt.Fprintln(os.Stderr, "connecteur factice impossible : "+errH.Error())
			os.Exit(1)
		}
		reg[id] = hp
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orchestrateur impossible : "+err.Error())
		os.Exit(1)
	}
	if *rejeu != "" {
		orch = orch.AvecRejeu(*rejeu)
		fmt.Println("REJEU : cible " + *rejeu)
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
			Appels: map[string]int{"mistral": 40, "openai": 40},
			Jetons: map[string]int{"mistral": 200000, "openai": 200000},
		}
		if err := orch.NouvelleMission(*issue, "mistral-exec", "openai-recep", roles, plafonds); err != nil {
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
		v, errC := orch.CritiquerPlan(ctx, *issue, "openai-recep", contenu)
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
		res, errE := orch.Executer(ctx, *issue, "mistral-exec", contenu)
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
		if _, errS := orch.Soumettre(ctx, *issue, "mistral-exec", contenu); errS != nil {
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
		v, _, errR := orch.Recevoir(ctx, *issue, "openai-recep", contenu)
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
		if errA := orch.Acker(*issue, "mistral-exec"); errA != nil {
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
		if *resolution == "" || *motif == "" {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -resolution et -motif requis (donnée de recette : décision pilote SIMULÉE)")
			os.Exit(3)
		}
		if errR := orch.ResoudreAppelEnCours(*issue, *resolution, *motif, connecteurs.ActeurPilote); errR != nil {
			fmt.Fprintln(os.Stderr, "résolution échouée : "+errR.Error())
			os.Exit(1)
		}
		fmt.Println("RESOLUTION : " + *resolution + " (" + *motif + ")")
	case "vivant":
		// Coordinateur VIVANT (cible du SIGKILL de la recette) : le processus
		// ne fait RIEN d'autre que dormir ; le lanceur le tue par PID après
		// vérification d'appartenance (T3).
		fmt.Println("VIVANT : coordinateur flow01 en attente (PID " + fmt.Sprint(os.Getpid()) + ")")
		time.Sleep(time.Duration(*secondes) * time.Second)
		fmt.Println("ANOMALIE DE RECETTE : vivant terminé sans kill")
		os.Exit(9)
	default:
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : mode inconnu "+*mode)
		os.Exit(3)
	}
}

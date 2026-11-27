// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// harnais-temoin : harnais de recette de la mission FILLE MAIEUTIQUE-TEMOIN-01
// (mission M-COORD-MAIEUTIQUE-01, plan v2 §5). Dérivé des harnais existants
// (harnais-flow01, harnais-mvps01) : UNE TRANSITION PAR PROCESSUS, chaque
// invocation relit l'état persisté du référentiel. La mission fille est
// créée par la TRANSMISSION du service coord-maieutique (NouvelleMissionExplicit,
// binôme openai-exec / openai-recep) ; ce harnais conduit ensuite le cycle
// COORD complet : executer → soumettre → recevoir → acker → cloturer.
//
// Intervenants = serveurs factices loopback format openai (adresses
// 127.0.0.1 EXPLICITES, processus coord séparés, JAMAIS tués avec le
// coordinateur). Mode SIMULÉ, HORS RÉSEAU, aucune clé, 0 €.
//
//	-mode consulter : JSON mission sur stdout
//	-mode compter   : comptage des opérations (JSON sur stdout)
//	-mode executer  : Executer (-mandat fichier), livrable vers -sortie
//	-mode soumettre : Soumettre (-livrable fichier)
//	-mode recevoir  : Recevoir (-livrable fichier), verdict + points,
//	                  ACCEPTEE attendu (exit 0) sinon exit 2
//	-mode acker     : Acker
//	-mode cloturer  : tentative de clôture ; la clôture est RÉSERVÉE au
//	                  pilote humain : l'automatisme est TOUJOURS refusé
//	                  (exit 2, CLOTURE_REFUSEE attendue, garde démontrée)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"coordengine/internal/adaptateurs"
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
	data := flag.String("data", "", "dossier du référentiel missions de la mission fille")
	issue := flag.String("issue", "MAIEUTIQUE-TEMOIN-01", "identifiant de la mission fille")
	mode := flag.String("mode", "", "consulter|compter|executer|soumettre|recevoir|acker|cloturer")
	mandat := flag.String("mandat", "", "fichier du mandat d'exécution (executer)")
	livrable := flag.String("livrable", "", "fichier du livrable soumis (soumettre, recevoir)")
	sortie := flag.String("sortie", "", "fichier où écrire le livrable produit (executer)")
	execEP := flag.String("exec-endpoint", "http://127.0.0.1:8293/", "serveur factice exécutant (loopback explicite)")
	recepEP := flag.String("recep-endpoint", "http://127.0.0.1:8294/", "serveur factice réceptionnaire (loopback explicite)")
	flag.Parse()
	if *data == "" || *mode == "" {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : -data et -mode requis")
		os.Exit(3)
	}
	ref, err := missions.Ouvrir(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "référentiel impossible : "+err.Error())
		os.Exit(1)
	}
	switch *mode {
	case "consulter":
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
	}
	cfg := &connecteurs.Config{Mode: connecteurs.ModeSimule, Fournisseurs: []connecteurs.ConfigFournisseur{
		{ID: "openai-exec", Endpoint: *execEP, Modele: "modele-openai-exec", TimeoutMS: 30000,
			TailleMaxReponse: 4 << 20, RetraitsMax: 1},
		{ID: "openai-recep", Endpoint: *recepEP, Modele: "modele-openai-recep", TimeoutMS: 30000,
			TailleMaxReponse: 4 << 20, RetraitsMax: 1},
	}}
	ids := []string{"openai-exec", "openai-recep"}
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
	ctx := context.Background()

	switch *mode {
	case "executer":
		contenu, rc := lireFichier(*mandat)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -mandat requis et lisible")
			os.Exit(3)
		}
		res, errE := orch.Executer(ctx, *issue, "openai-exec", contenu)
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
		if _, errS := orch.Soumettre(ctx, *issue, "openai-exec", contenu); errS != nil {
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
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "acker":
		if errA := orch.Acker(*issue, "openai-exec"); errA != nil {
			fmt.Fprintln(os.Stderr, "ack échoué : "+errA.Error())
			os.Exit(1)
		}
		fmt.Println("ACK : acquitté par l'exécutant")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
	case "cloturer":
		errCl := orch.Cloturer(*issue)
		// La clôture est réservée au pilote humain : l'automatisme est
		// TOUJOURS refusé (aucun texte de modèle ne vaut autorisation).
		_ = errCl
		fmt.Println("CLOTURE_REFUSEE : la clôture est réservée au pilote humain")
		fmt.Println("ETAT : " + etatToString(ref, *issue))
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : mode inconnu "+*mode)
		os.Exit(3)
	}
}

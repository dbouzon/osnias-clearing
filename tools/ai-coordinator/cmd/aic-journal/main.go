// Command aic-journal fournit une preuve de recette en lecture seule :
// missions, états, livraisons et journal des_agents depuis la base SQLite du
// coordinateur. Aucune écriture, aucune clé, aucun réseau (M-AIC-02 T02).
package main

import (
	"flag"
	"fmt"
	"os"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

func main() {
	dbPath := flag.String("db", "ai-coordinator.sqlite", "chemin de la base SQLite du coordinateur")
	mission := flag.String("mission", "", "identifiant de mission (vide : toutes)")
	flag.Parse()
	store, err := aic.OpenStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
	defer store.Close()
	pending, err := store.PendingMissions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
	fmt.Printf("# missions non terminées : %d\n", len(pending))
	for _, m := range pending {
		fmt.Printf("mission %s issue=%d état=%s corrections=%d\n", m.ID, m.IssueNumber, m.State, m.Corrections)
	}
	if *mission != "" {
		m, err := store.MissionByID(*mission)
		if err != nil || m == nil {
			fmt.Fprintln(os.Stderr, "mission introuvable:", *mission, err)
			os.Exit(1)
		}
		tr, del, err := store.Counts(m.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur:", err)
			os.Exit(1)
		}
		ref, err := store.DraftForMission(m.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur:", err)
			os.Exit(1)
		}
		fmt.Printf("mission %s état=%s transitions=%d livraisons=%d livraison=%q\n", m.ID, m.State, tr, del, ref)
	}
	logs, err := store.Journal(*mission)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
	fmt.Printf("# journal (%d entrées)\n", len(logs))
	for _, l := range logs {
		fmt.Printf("%s [%s/%s] %s : %s\n", l.At, l.MissionID, l.Agent, l.Phase, l.Message)
	}
}

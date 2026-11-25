// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-exp exécute la conduite comparative M-COORD-EXP-01 (issue #38) :
// scénarios S1..S9 en mode A (A2A direct) et mode B (A2A encapsulé
// dans MCP), mesures et tableau comparatif. Aucune ressource externe,
// budget 0 EUR, boucles locales uniquement.
//
// Usage :
//
//	coord-exp           rend le rapport Markdown (FR)
//	coord-exp -json     rend les lignes en JSON (qualification)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dbouzon/osnias-clearing/coord/exp/conduite"
)

func main() {
	jsonMode := flag.Bool("json", false, "sortie JSON des lignes de conduite")
	s10 := flag.Bool("s10", false, "scenario S10 seul (gouvernance COORD : Pilot reel + serveur MCP lecture)")
	flag.Parse()

	if *s10 {
		res, err := conduite.LigneS10(context.Background())
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur S10:", err)
			os.Exit(1)
		}
		if *jsonMode {
			b, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				fmt.Fprintln(os.Stderr, "erreur d encodage JSON:", err)
				os.Exit(1)
			}
			fmt.Println(string(b))
			return
		}
		fmt.Println(conduite.MarkdownS10(res))
		return
	}

	exp, err := conduite.Runner(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur de conduite:", err)
		os.Exit(1)
	}
	if *jsonMode {
		b, err := json.MarshalIndent(exp.Lignes, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur d encodage JSON:", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}
	fmt.Println(exp.Markdown())
}

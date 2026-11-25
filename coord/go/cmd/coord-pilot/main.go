// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-pilot is the COORD Pilot MVP launcher (M-COORD-PILOT-02): a local,
// autonomous, zero-cost service. It listens on the loopback interface only,
// generates a fresh session token displayed on the console (and never
// journaled), and refuses to start on any non-loopback address (S3) or on a
// non-conforming repository (R2/S5). Simulated adapters only: no account, no
// API key, no outgoing network call.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"coordengine/internal/missions"
	"coordengine/internal/pilot"
)

func main() {
	adresse := flag.String("addr", "127.0.0.1:0", "adresse d'ecoute (loopback uniquement, S3)")
	donnees := flag.String("data", "coord-pilot-data", "dossier du referentiel missions")
	ui := flag.String("ui", "ui", "dossier de l'interface web (index.html requis, fail closed)")
	flag.Parse()

	token := genererToken()
	jetonLecture := genererToken()
	jetonFichier := filepath.Join(os.TempDir(), "coord-pilot-jeton-lecture.txt")
	if err := os.WriteFile(jetonFichier, []byte(jetonLecture+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "ecriture du fichier de jeton de lecture impossible : "+filepath.Base(jetonFichier))
		os.Exit(1)
	}

	ref, err := missions.Ouvrir(*donnees)
	if err != nil {
		fmt.Fprintln(os.Stderr, "referentiel impossible : "+err.Error())
		os.Exit(1)
	}

	svc, err := pilot.NouveauService(ref, *ui, token, *adresse, pilot.AvecJetonLecture(jetonLecture))
	if err != nil {
		fmt.Fprintln(os.Stderr, "demarrage refuse : "+err.Error())
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", *adresse)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ecoute impossible : "+err.Error())
		os.Exit(1)
	}
	absUI, _ := filepath.Abs(*ui)
	fmt.Println("COORD Pilot (MVP) — adaptateurs SIMULES, aucune cle, aucun appel reseau.")
	fmt.Println("Referentiel : " + *donnees)
	fmt.Println("Interface   : " + absUI)
	fmt.Println("URL         : http://" + ln.Addr().String() + "/")
	fmt.Println("Token local : " + token)
	fmt.Println("Ce token n'est pas journalise ; il est regenere a chaque demarrage.")
	fmt.Println("Jeton de lecture (PDF/MCP pages) : " + filepath.Base(jetonFichier) + " (permissions 0600, dossier temporaire)")
	fmt.Println("Ce jeton est LECTURE SEULE : liste et detail des missions ; il est refuse sur toute mutation.")
	fmt.Println("Il n'est pas journalise et est regenere a chaque demarrage (rotation : les anciens jetons deviennent invalides).")
	if err := http.Serve(ln, svc.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "service interrompu : "+err.Error())
		os.Exit(1)
	}
}

func genererToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, "generateur aleatoire indisponible : "+err.Error())
		os.Exit(1)
	}
	return hex.EncodeToString(b)
}

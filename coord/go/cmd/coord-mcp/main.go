// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-mcp is the COORD common MCP server (M-COORD-MCP-01, issue #35):
// a strictly read-only MCP facade above the local COORD Pilot service. The
// pilot console token is NEVER known to this program: it only holds the
// separate read-only token, read from a local 0600 file owned by the pilot.
// The transport (STDIO) is chosen here only; the mcpserver package carries no
// transport dependency, so a future remote client-server architecture can
// reuse the same contract without change.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"coordengine/internal/mcpserver"
	"coordengine/internal/missions"
	"coordengine/internal/pilot"
)

const codeErrJeton = 2
const codeErrDemarrage = 3

func usage() {
	fmt.Fprintln(os.Stderr, "coord-mcp — serveur MCP commun COORD (lecture seule stricte)")
	fmt.Fprintln(os.Stderr, "usage : coord-mcp -pilot http://127.0.0.1:PORT -jeton /chemin/du/jeton-lecture (0600)")
	fmt.Fprintln(os.Stderr, "        coord-mcp -demo-local   (demonstration locale sans cle ni compte, adaptateurs SIMULES)")
}

func main() {
	pilotURL := flag.String("pilot", "", "URL loopback du service COORD Pilot local")
	jeton := flag.String("jeton", "", "chemin du fichier de jeton de lecture (permissions 0600)")
	demo := flag.Bool("demo-local", false, "demonstration locale autonome (aucune cle, aucun compte, aucun reseau sortant)")
	flag.Parse()

	if *demo {
		if err := demoLocal(); err != nil {
			fmt.Fprintln(os.Stderr, "demo locale ECHEC : "+err.Error())
			os.Exit(codeErrDemarrage)
		}
		return
	}
	if *pilotURL == "" || *jeton == "" {
		usage()
		os.Exit(codeErrDemarrage)
	}

	valeur, err := mcpserver.ChargerJeton(*jeton)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error()+" (aucune valeur de jeton affichee ; corriger puis relancer)")
		os.Exit(codeErrJeton)
	}
	coeur, err := mcpserver.NouveauServeur(*pilotURL, valeur)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(codeErrDemarrage)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "coord-mcp", Version: "v1"}, nil)
	coeur.Peupler(server)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "serveur MCP interrompu : "+err.Error())
		os.Exit(codeErrDemarrage)
	}
}

// demoLocal enchaîne : service Pilot factice (adaptateurs SIMULES, loopback)
// + fichier jeton 0600 + serveur MCP + client MCP factice ; vérifie la
// négociation, la liste, le détail, le refus explicite d'une mission inconnue
// sans fuite de secret, et l'ABSENCE de tout outil de mutation. Aucune clé,
// aucun compte, aucun réseau sortant : 0 €.
func demoLocal() error {
	racine, err := os.MkdirTemp("", "coord-mcp-demo")
	if err != nil {
		return err
	}
	defer os.RemoveAll(racine)
	ui, err := os.MkdirTemp("", "coord-mcp-demo-ui")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ui)
	if err := os.WriteFile(filepath.Join(ui, "index.html"),
		[]byte("<!doctype html><html><body>demo</body></html>"), 0o600); err != nil {
		return err
	}
	ref, err := missions.Ouvrir(filepath.Join(racine, "missions"))
	if err != nil {
		return err
	}
	tokenConsole := generer()
	jetonLecture := generer()
	svc, err := pilot.NouveauService(ref, ui, tokenConsole, "127.0.0.1:0", pilot.AvecJetonLecture(jetonLecture))
	if err != nil {
		return err
	}
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	if err := creerMissionsDemo(srv.URL, tokenConsole); err != nil {
		return err
	}
	cheminJeton := filepath.Join(racine, "jeton-lecture.txt")
	if err := os.WriteFile(cheminJeton, []byte(jetonLecture+"\n"), 0o600); err != nil {
		return err
	}
	valeur, err := mcpserver.ChargerJeton(cheminJeton)
	if err != nil {
		return fmt.Errorf("chargement du jeton refuse en demo : %w", err)
	}
	coeur, err := mcpserver.NouveauServeur(srv.URL, valeur)
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "coord-mcp", Version: "v1"}, nil)
	coeur.Peupler(server)

	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		return err
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "client-factice"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		return err
	}
	defer cs.Close()

	ctx := context.Background()
	outils, err := cs.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	noms := []string{}
	for _, o := range outils.Tools {
		noms = append(noms, o.Name)
	}
	fmt.Println("outils exposes :", strings.Join(noms, ", "))
	if len(noms) != 2 {
		return fmt.Errorf("nombre d'outils inattendu : %d (attendu : lecture seule stricte, 2 outils)", len(noms))
	}
	vus := map[string]bool{}
	for _, n := range noms {
		vus[n] = true
	}
	if !vus[mcpserver.OutilLister] || !vus[mcpserver.OutilDetail] {
		return fmt.Errorf("outils inattendus : %v", noms)
	}
	resListe, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.OutilLister})
	if err != nil {
		return err
	}
	if resListe.IsError {
		return fmt.Errorf("coord_list_missions en echec : %s", texte(resListe))
	}
	fmt.Println("coord_list_missions -> OK :", texte(resListe))
	resDetail, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      mcpserver.OutilDetail,
		Arguments: map[string]any{"issue": "mcp-demo-1"},
	})
	if err != nil {
		return err
	}
	if resDetail.IsError {
		return fmt.Errorf("coord_get_mission en echec : %s", texte(resDetail))
	}
	fmt.Println("coord_get_mission  -> OK :", texte(resDetail)[:120], "...")
	resInconnu, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      mcpserver.OutilDetail,
		Arguments: map[string]any{"issue": "inexistante-xyz"},
	})
	if err != nil {
		return err
	}
	if !resInconnu.IsError {
		return fmt.Errorf("mission inconnue : refus attendu (IsError)")
	}
	fmt.Println("mission inconnue   -> refus explicite :", texte(resInconnu))
	if contenu := texte(resInconnu); strings.Contains(contenu, jetonLecture) ||
		strings.Contains(contenu, srv.URL) || strings.Contains(contenu, racine) {
		return fmt.Errorf("fuite de secret dans un refus (garde-fou non-divulgation)")
	}
	fmt.Println("DEMO CONFORME : lecture seule stricte, refus sans fuite, aucun outil de mutation, 0 euro.")
	return nil
}

func texte(res *mcp.CallToolResult) string {
	out := ""
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			out += t.Text
		}
	}
	return out
}

func generer() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// creerMissionsDemo crée deux missions factices via l'API du service avec le
// token console (jamais connu du serveur MCP). Adaptateurs SIMULES uniquement.
func creerMissionsDemo(base, tokenConsole string) error {
	for _, issue := range []string{"mcp-demo-1", "mcp-demo-2"} {
		corps, err := json.Marshal(map[string]any{
			"issue":       issue,
			"adaptateurs": []string{"mistral-simule", "openai-simule"},
			"roles": map[string][]string{
				"EXECUTANT":      {"executer", "soumettre", "acker"},
				"RECEPTIONNAIRE": {"recevoir"},
				"PILOTE":         {"cloturer"},
			},
			"executant_id": "exec-sim",
			"recepteur_id": "recep-sim",
			"plafonds":     map[string]any{"appels": map[string]int{"mistral-simule": 3, "openai-simule": 3}},
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodPost, base+"/api/mission", bytes.NewReader(corps))
		if err != nil {
			return err
		}
		req.Header.Set("X-Coord-Pilot-Token", tokenConsole)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return fmt.Errorf("creation %s refusee (HTTP %d) : %s", issue, resp.StatusCode, string(raw))
		}
	}
	return nil
}

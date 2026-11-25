// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package conduite

// Scenario S10 de M-COORD-EXP-01 (plan v2 accepte) : gouvernance COORD.
// Le service Pilot reel cmd/coord-pilot (boucle locale, adaptateurs
// SIMULES, token console + jeton de lecture distincts) est lance comme
// processus, une mission experimentale derivee de COORD est enregistree
// via son API authentifiee, puis le serveur MCP de #37 (cmd/coord-mcp,
// lecture seule stricte) est sollicite en lecture par un client MCP sur
// STDIO. Les tests negatifs anti-mandat verifient qu un agent (mode A ou
// mode B) ne peut creer, modifier ni clore une mission, et qu aucune
// credential COORD n atteint un agent.
//
// Ce fichier ne modifie AUCUN composant du moteur : les binaires sont
// construits depuis coord/go INCHANGE et pilotes comme un utilisateur
// reel le ferait. 0 EUR, aucune cle, aucun reseau sortant.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dbouzon/osnias-clearing/coord/exp/modea"
	"github.com/dbouzon/osnias-clearing/coord/exp/modeb"
)

// issueS10 : identifiant de la mission experimentale enregistree dans le
// Pilot reel. Le contenu est derive de COORD : agents A2A de la pile
// experimentale en EXECUTANT/RECEPTIONNAIRE, plafonds explicites, roles
// standards de la spec.
const issueS10 = "M-COORD-EXP-01-S10"

// ResultatS10 porte la ligne de rapport et les preuves d invariants.
type ResultatS10 struct {
	Ligne      Ligne
	Invariants []string
	Negatifs   []string
}

// racineCoord localise le dossier coord/ du depot depuis le repertoire
// d execution (test ou binaire cmd) : on remonte jusqu au dossier qui
// contient a la fois coord/go/go.mod et coord/exp/go.mod.
func racineCoord() (string, error) {
	courant, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(courant, "go", "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(courant, "exp", "go.mod")); err == nil {
				return courant, nil
			}
		}
		parent := filepath.Dir(courant)
		if parent == courant {
			break
		}
		courant = parent
	}
	return "", fmt.Errorf("dossier coord/ du depot introuvable depuis %s", courant)
}

// construireBinaire compile un binaire du module coord/go vers tmp.
func construireBinaire(racine, paquet, sortie string) error {
	cmd := exec.Command("go", "build", "-o", sortie, paquet)
	cmd.Dir = filepath.Join(racine, "go")
	var sortieErr bytes.Buffer
	cmd.Stderr = &sortieErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("construction %s : %w : %s", paquet, err, sortieErr.String())
	}
	return nil
}

// piloteLance est le service Pilot reel en cours d execution.
type piloteLance struct {
	URL     string
	Token   string
	JetonCh string
	process *exec.Cmd
	arreter func()
}

// lancerPilot demarre cmd/coord-pilot (adaptateurs SIMULES) sur une boucle
// locale a port ephemere et releve dans sa sortie console : l URL, le
// token du pilote et le nom du fichier de jeton de lecture (jamais
// journalise, rotation a chaque demarrage).
func lancerPilot(binaire, donnees, ui string) (*piloteLance, error) {
	cmd := exec.Command(binaire, "-addr", "127.0.0.1:0", "-data", donnees, "-ui", ui)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var journal bytes.Buffer
	cmd.Stderr = &journal
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("demarrage coord-pilot : %w : %s", err, journal.String())
	}
	p := &piloteLance{process: cmd}
	p.arreter = func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	lu := make(chan struct{}, 1)
	go func() {
		defer func() { lu <- struct{}{} }()
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			l := sc.Text()
			switch {
			case strings.HasPrefix(l, "URL         : http://"):
				p.URL = "http://" + strings.TrimSpace(strings.TrimPrefix(l, "URL         : http://"))
			case strings.HasPrefix(l, "Token local : "):
				p.Token = strings.TrimSpace(strings.TrimPrefix(l, "Token local : "))
			case strings.HasPrefix(l, "Jeton de lecture (PDF/MCP pages) : "):
				nom := strings.TrimSpace(strings.TrimPrefix(l, "Jeton de lecture (PDF/MCP pages) : "))
				if i := strings.Index(nom, " ("); i > 0 {
					nom = nom[:i]
				}
				p.JetonCh = filepath.Join(os.TempDir(), nom)
			}
			if p.URL != "" && p.Token != "" && p.JetonCh != "" {
				return
			}
		}
	}()
	select {
	case <-lu:
	case <-time.After(20 * time.Second):
		p.arreter()
		return nil, fmt.Errorf("coord-pilot muet (URL/token/jeton non releves) : %s", journal.String())
	}
	if p.URL == "" || p.Token == "" || p.JetonCh == "" {
		p.arreter()
		return nil, fmt.Errorf("coord-pilot : releve incomplet (URL=%q token=%d octets jeton=%q)",
			p.URL, len(p.Token), p.JetonCh)
	}
	return p, nil
}

// posteMission cree une mission via l API authentifiee du Pilot.
func (p *piloteLance) posteMission(corps map[string]any, token string) (int, map[string]any, error) {
	raw, err := json.Marshal(corps)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, p.URL+"/api/mission", bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Coord-Pilot-Token", token)
	}
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer rep.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&out)
	return rep.StatusCode, out, nil
}

// corpsMissionS10 : mission derivee de COORD : agents A2A de la pile
// experimentale en EXECUTANT/RECEPTIONNAIRE, pilote local, plafonds bas.
func corpsMissionS10(rolesExec []string) map[string]any {
	return map[string]any{
		"issue":       issueS10,
		"adaptateurs": []string{"mistral-simule", "openai-simule"},
		"roles": map[string][]string{
			"EXECUTANT":      rolesExec,
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		"executant_id": "AGENT-EXEC-EXP01",
		"recepteur_id": "AGENT-CRIT-EXP01",
		"plafonds":     map[string]any{"appels": map[string]int{"mistral-simule": 3, "openai-simule": 3}},
	}
}

// LigneS10 execute le scenario de gouvernance : Pilot reel, serveur MCP
// reel en lecture, mission enregistree, preuves negatives anti-mandat,
// puis verifie que les DEUX modes (A2A direct et A2A-over-MCP) operent
// leurs agents SANS aucune credential COORD, la gouvernance restant
// portee par l etoile Pilot (aucun agent ne fait autorite COORD).
func LigneS10(ctx context.Context) (*ResultatS10, error) {
	res := &ResultatS10{}
	racine, err := racineCoord()
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "coord-exp-s10-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	binPilot := filepath.Join(tmp, "coord-pilot")
	binMcp := filepath.Join(tmp, "coord-mcp")
	if err := construireBinaire(racine, "./cmd/coord-pilot", binPilot); err != nil {
		return nil, err
	}
	if err := construireBinaire(racine, "./cmd/coord-mcp", binMcp); err != nil {
		return nil, err
	}
	res.Invariants = append(res.Invariants, "binaires coord-pilot et coord-mcp construits depuis coord/go INCHANGE (go build reussi)")

	donnees := filepath.Join(tmp, "missions")
	ui := filepath.Join(racine, "pilot", "ui")
	if _, err := os.Stat(filepath.Join(ui, "index.html")); err != nil {
		return nil, fmt.Errorf("interface Pilot (coord/pilot/ui) introuvable : %w", err)
	}
	p, err := lancerPilot(binPilot, donnees, ui)
	if err != nil {
		return nil, err
	}
	defer p.arreter()
	res.Invariants = append(res.Invariants, "Pilot reel demarre sur boucle locale a port ephemere (adaptateurs SIMULES, token console genere)")

	// Creation governee : la mission experimentale derivee de COORD.
	code, corps, err := p.posteMission(corpsMissionS10([]string{"executer", "soumettre", "acker"}), p.Token)
	if err != nil {
		return nil, err
	}
	if code != http.StatusCreated {
		return nil, fmt.Errorf("creation mission %s : HTTP %d (%v)", issueS10, code, corps)
	}
	etatInitial, _ := corps["etat"].(string)
	res.Invariants = append(res.Invariants,
		fmt.Sprintf("mission %s enregistree via l API authentifiee du Pilot (POST /api/mission, X-Coord-Pilot-Token), etat %s",
			issueS10, etatInitial))

	// Preuves negatives anti-mandat cote HTTP.
	code, corps, err = p.posteMission(corpsMissionS10(nil), "")
	if err != nil {
		return nil, err
	}
	res.Negatifs = append(res.Negatifs,
		fmt.Sprintf("creation SANS token : HTTP %d%s", code, verdictNegatif(code, http.StatusUnauthorized)))

	jetonValeur, err := os.ReadFile(p.JetonCh)
	if err != nil {
		return nil, fmt.Errorf("jeton de lecture illisible : %w", err)
	}
	code, corps, err = p.posteMission(corpsMissionS10(nil), strings.TrimSpace(string(jetonValeur)))
	if err != nil {
		return nil, err
	}
	res.Negatifs = append(res.Negatifs,
		fmt.Sprintf("creation avec jeton de LECTURE : HTTP %d (type %v)%s", code, corps["type"], verdictNegatif(code, http.StatusForbidden)))

	// Auto-mandat tente : un agent ne peut s attribuer la cloture.
	code, corps, err = p.posteMission(corpsMissionS10([]string{"executer", "soumettre", "acker", "cloturer"}), p.Token)
	if err != nil {
		return nil, err
	}
	if code == http.StatusCreated {
		res.Negatifs = append(res.Negatifs, "habilitation EXECUTANT+cloturer : ACCEPTEE (defaut de gouvernance !)")
	} else {
		res.Negatifs = append(res.Negatifs,
			fmt.Sprintf("habilitation EXECUTANT+cloturer : REFUSEE (HTTP %d, type %v)", code, corps["type"]))
	}

	// Serveur MCP reel en lecture seule, dialogue STDIO.
	cmdMcp := exec.Command(binMcp, "-pilot", p.URL, "-jeton", p.JetonCh)
	mcpIn, err := cmdMcp.StdinPipe()
	if err != nil {
		return nil, err
	}
	mcpOut, err := cmdMcp.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var journalMcp bytes.Buffer
	cmdMcp.Stderr = &journalMcp
	if err := cmdMcp.Start(); err != nil {
		return nil, fmt.Errorf("demarrage coord-mcp : %w", err)
	}
	arreterMcp := func() {
		_ = cmdMcp.Process.Kill()
		_, _ = cmdMcp.Process.Wait()
	}
	defer arreterMcp()
	client := mcp.NewClient(&mcp.Implementation{Name: "coord-exp-s10", Version: "v1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: mcpOut, Writer: mcpIn}, nil)
	if err != nil {
		return nil, fmt.Errorf("connexion MCP (STDIO) : %w : %s", err, journalMcp.String())
	}
	res.Invariants = append(res.Invariants, "serveur MCP #37 joint en STDIO par un client MCP (jeton de lecture uniquement)")

	// Inventaire des outils : exactement les deux lectures, rien d autre.
	outils, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list : %w", err)
	}
	noms := map[string]bool{}
	for _, t := range outils.Tools {
		noms[t.Name] = true
	}
	if noms["coord_list_missions"] && noms["coord_get_mission"] && len(noms) == 2 {
		res.Invariants = append(res.Invariants, "inventaire MCP : exactement 2 outils (coord_list_missions, coord_get_mission), aucun outil de mutation")
	} else {
		res.Negatifs = append(res.Negatifs, fmt.Sprintf("inventaire MCP inattendu : %v", noms))
	}

	// Lecture de la liste puis du detail : la mission governee est lisible.
	listeRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "coord_list_missions", Arguments: map[string]any{}})
	if err != nil {
		return nil, fmt.Errorf("coord_list_missions : %w", err)
	}
	listeTxt, err := modeb.Texte(listeRes)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(listeTxt, issueS10) {
		res.Negatifs = append(res.Negatifs, "coord_list_missions ne mentionne pas la mission experimentale")
	} else {
		res.Invariants = append(res.Invariants, "coord_list_missions (jeton de lecture) expose la mission experimentale")
	}
	detailRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "coord_get_mission", Arguments: map[string]any{"issue": issueS10}})
	if err != nil {
		return nil, fmt.Errorf("coord_get_mission : %w", err)
	}
	detailTxt, err := modeb.Texte(detailRes)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(detailTxt, "AGENT-EXEC-EXP01") {
		res.Negatifs = append(res.Negatifs, "coord_get_mission ne porte pas le contenu derive de COORD")
	} else {
		res.Invariants = append(res.Invariants, "coord_get_mission porte le contenu derive de COORD (agents A2A experimentaux, habilitations, plafonds)")
	}
	if strings.Contains(detailTxt, p.Token) || strings.Contains(detailTxt, string(jetonValeur)) {
		res.Negatifs = append(res.Negatifs, "FUITE : les donnees de mission exposent le token pilote ou le jeton de lecture")
	} else {
		res.Invariants = append(res.Invariants, "aucune credential COORD dans les donnees de mission lues par les agents")
	}

	// Mutation tente par le canal MCP : outil inexistant, refus interprétable.
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "coord_create_mission", Arguments: map[string]any{}}); err != nil {
		res.Negatifs = append(res.Negatifs, "creation via MCP (outil inexistant coord_create_mission) : REFUSEE par le serveur lecture seule")
	} else {
		res.Negatifs = append(res.Negatifs, "creation via MCP : ACCEPTEE (defaut de lecture seule !)")
	}

	// Gouvernance identique dans les DEUX modes : les agents travaillent
	// (aller-retour minimal) sans aucune credential COORD ; aucune voie de
	// mutation quelle que soit l enveloppe de transport.
	l := Ligne{Scenario: "S10", Objet: "gouvernance COORD (Pilot reel + MCP lecture seule)"}

	pa, err := modea.NouvellePile(ctx)
	if err != nil {
		return nil, fmt.Errorf("mode A : %w", err)
	}
	envoye, errA := (&modea.PileClient{Client: pa.ClientExec()}).SendMessage(ctx, a2aMsg("PING"))
	pa.Fermer()
	if errA != nil {
		return nil, fmt.Errorf("mode A : %w", errA)
	}
	manifest := manifestGouvernance(res)
	l.A = EtatLigne{EtatFinal: envoye.Etat, Sha: shaSemantique(envoye.Etat, manifest), Octets: pa.Mesures.Octets, Echanges: pa.Mesures.Echanges}

	pb, err := modeb.NouvellePile(ctx)
	if err != nil {
		return nil, fmt.Errorf("mode B : %w", err)
	}
	rout, errB := pb.Appeler(ctx, "a2a_send", map[string]any{"agent": "exec", "message": "PING"})
	pb.Fermer()
	if errB != nil {
		return nil, fmt.Errorf("mode B : %w", errB)
	}
	sortie, err := modeb.Texte(rout)
	if err != nil {
		return nil, err
	}
	var wb wireTask
	if err := json.Unmarshal([]byte(sortie), &wb); err != nil {
		return nil, fmt.Errorf("mode B : decodage : %w", err)
	}
	octA2A, echA2A := pb.MesuresA2A()
	l.B = EtatLigne{EtatFinal: wb.etat(), Sha: shaSemantique(wb.etat(), manifest), Octets: pb.Enveloppe + octA2A, Echanges: pb.Appels + echA2A}

	if gouvernanceConforme(res) && l.A.EtatFinal == l.B.EtatFinal {
		l.Verdict = "conservée"
		l.A.Note = "agent A2A sans credential COORD"
		l.B.Note = "agent A2A sans credential COORD (enveloppe MCP sans privilege)"
	} else {
		l.Verdict = "PERDUE"
	}
	res.Ligne = l
	return res, nil
}

// verdictNegatif marque la conformite d un refus attendu (chaine vide si
// conforme, motif d ecart sinon).
func verdictNegatif(code, attendu int) string {
	if code == attendu {
		return " : REFUS CONFORME"
	}
	return " : ECART (defaut)"
}

// gouvernanceConforme est vrai si aucun ecart n a ete releve.
func gouvernanceConforme(r *ResultatS10) bool {
	for _, n := range r.Negatifs {
		if strings.Contains(n, "defaut") || strings.Contains(n, "ECART") || strings.Contains(n, "FUITE") ||
			strings.Contains(n, "inattendu") || strings.Contains(n, "ACCEPTEE") {
			return false
		}
	}
	return true
}

// manifestGouvernance condense les preuves relevees : identique par mode
// (la gouvernance est portee par l etoile Pilot, pas par le transport).
func manifestGouvernance(r *ResultatS10) string {
	return strings.Join(r.Invariants, ";") + "|" + strings.Join(r.Negatifs, ";")
}

// MarkdownS10 rend la section S10 du rapport.
func MarkdownS10(r *ResultatS10) string {
	var b strings.Builder
	b.WriteString("## S10 - Gouvernance COORD (Pilot reel + serveur MCP lecture seule)\n\n")
	b.WriteString("Invariants :\n\n")
	for _, i := range r.Invariants {
		fmt.Fprintf(&b, "- [OK] %s\n", i)
	}
	b.WriteString("\nTests negatifs (anti-mandat) :\n\n")
	for _, n := range r.Negatifs {
		motif := "[REFUS ATTENDU]"
		if strings.Contains(n, "defaut") || strings.Contains(n, "FUITE") || strings.Contains(n, "inattendu") || strings.Contains(n, "ACCEPTEE") {
			motif = "[ECART]"
		}
		fmt.Fprintf(&b, "- %s %s\n", motif, n)
	}
	return b.String()
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Command claude-factice : CLI Claude Code FACTICE pour la recette de
// la voie Claude Code supervisée (M-COORD-LLM-05, T2). AUCUN réseau
// fournisseur, aucune clé, aucune écriture GitHub : 0 €. Elle imite le
// CONTRAT documenté de `claude -p --output-format json` :
//
//	sortie JSON {result, session_id, is_error, total_cost_usd, num_turns,
//	environment} ; code 0 succès, 1/2/3 échecs (documentés) ; SIGTERM
//	coopératif sauf scénario HANGUE (test de l'arrêt forcé du groupe).
//
// Rétroaction pilotée PAR LE PROMPT (marqueurs SCENARIO:, données de
// test, jamais des instructions exécutées) :
//
//	SCENARIO:BASE         nominal (défaut)
//	SCENARIO:ERR_IS_ERROR is_error vrai
//	SCENARIO:JSON_INVALIDE sortie non JSON
//	SCENARIO:VIDE        result vide
//	SCENARIO:SESSION_ABS session_id omis
//	SCENARIO:PERM_REFUS  refus de permission (code 2)
//	SCENARIO:BUDGET      dépasse --max-budget-usd (code 3)
//	SCENARIO:FUIE        secret dans le résultat (prouve l'expurgation)
//	SCENARIO:HANGUE      ignore SIGTERM (prouve le SIGKILL du groupe)
//	SCENARIO:HOSTILE    tente d'exécuter des instructions du prompt
//	OUTIL:<nom>          demande d'outil hors liste (→ PERM_REFUS)
//	SCENARIO:QUITTE1/2/3 codes de sortie bruts
//
// Sessions : état local dans $HOME/.claude-factice/etat.json ; le
// compteur d'appels PAR SESSION prouve la correction dans la MÊME
// session (reprise --resume) et l'absence de double exécution.
// Contrôles EMBARQUÉS (triple contrôle n°3) : mode de permission
// dontAsk EXIGÉ, et settings.json exigé avec deny non vides ; toute
// divergence → échec net (code 2), jamais une exécution silencieuse.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type etatFactice struct {
	SessionID string `json:"session_id"`
	Appels    int    `json:"appels"`
}

const sessionParDefaut = "cc-factice-session-0001"

func main() {
	code := executer(os.Args[1:])
	if code != 0 {
		fmt.Fprintf(os.Stderr, "claude-factice : échec code %d\n", code)
	}
	os.Exit(code)
}

// executer : analyse les arguments, applique les contrôles embarqués,
// produit la sortie JSON du scénario demandé.
func executer(args []string) int {
	prompt := ""
	resume := ""
	outils := ""
	mode := ""
	parametres := ""
	sortieJSON := true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p":
			if i+1 >= len(args) {
				return 2
			}
			prompt = args[i+1]
			i++
		case "--resume":
			if i+1 >= len(args) {
				return 2
			}
			resume = args[i+1]
			i++
		case "--allowedTools":
			if i+1 >= len(args) {
				return 2
			}
			outils = args[i+1]
			i++
		case "--permission-mode":
			if i+1 >= len(args) {
				return 2
			}
			mode = args[i+1]
			i++
		case "--settings":
			if i+1 >= len(args) {
				return 2
			}
			parametres = args[i+1]
			i++
		case "--max-budget-usd":
			if i+1 >= len(args) {
				return 2
			}
			i++ // borne acceptée ; le scénario BUDGET simule le franchissement
		case "--output-format":
			if i+1 >= len(args) || args[i+1] != "json" {
				return 2
			}
			i++
		default:
			fmt.Fprintf(os.Stderr, "claude-factice : argument inconnu : %s\n", args[i])
			return 2
		}
	}
	// Contrôles EMBARQUÉS (n°3 du triple contrôle) : mode verrouillé,
	// settings exigé avec deny NON VIDE. Jamais une exécution silencieuse
	// si le superviseur a contourné ses propres contrôles.
	if mode != "dontAsk" {
		fmt.Fprintf(os.Stderr, "claude-factice : MODE_PERMISSION_REFUSE : dontAsk exigé, reçu %q\n", mode)
		return 2
	}
	if !verifierParametres(parametres) {
		fmt.Fprintf(os.Stderr, "claude-factice : SETTINGS_INVALIDE : deny exigés non vides\n")
		return 2
	}
	outilsAdmis := map[string]bool{}
	for _, o := range strings.Split(outils, ",") {
		if o != "" {
			outilsAdmis[o] = true
		}
	}
	// Demande d'outil hors liste : REFUS DE PERMISSION (code 2, contrat
	// documenté : en mode dontAsk tout ce qui demanderait est refusé).
	if j := strings.Index(prompt, "OUTIL:"); j >= 0 {
		demande := prompt[j+len("OUTIL:"):]
		if k := strings.IndexAny(demande, " \t\n"); k >= 0 {
			demande = demande[:k]
		}
		if !outilsAdmis[demande] {
			fmt.Fprintf(os.Stderr, "claude-factice : permission refusée pour l'outil %s (dontAsk)\n", demande)
			return 2
		}
	}
	// Instructions hostiles dans le prompt : ENDIGUÉES par construction
	// (aucune instruction du prompt n'est exécutée ; réponse neutre).
	if strings.Contains(prompt, "SCENARIO:HOSTILE") {
		prompt = "SCENARIO:BASE " + prompt // neutralisé : données, jamais exécuté
	}
	switch {
	case strings.Contains(prompt, "SCENARIO:QUITTE1"):
		return 1
	case strings.Contains(prompt, "SCENARIO:QUITTE2"):
		return 2
	case strings.Contains(prompt, "SCENARIO:QUITTE3"):
		return 3
	case strings.Contains(prompt, "SCENARIO:BUDGET"):
		fmt.Fprintln(os.Stderr, "claude-factice : budget dépassé (--max-budget-usd)")
		return 3
	case strings.Contains(prompt, "SCENARIO:PERM_REFUS"):
		fmt.Fprintln(os.Stderr, "claude-factice : permission refusée (dontAsk)")
		return 2
	}
	// LENTE : run de 2 s pour la recette de l'annulation (CC9).
	if strings.Contains(prompt, "SCENARIO:LENTE") {
		time.Sleep(2 * time.Second)
	}
	// HANGUE : ignore SIGTERM jusqu'au SIGKILL du groupe (le superviseur
	// doit expulser). POSIX uniquement ; sous Windows le test est écarté.
	if strings.Contains(prompt, "SCENARIO:HANGUE") {
		ignorerTerminaison()
		for {
			bloque()
		}
	}
	etat, err := chargerEtat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-factice : %v\n", err)
		return 2
	}
	if resume != "" {
		// Reprise : l'ID demandé DOIT correspondre à la session locale,
		// sinon SESSION_INCOHERENTE (aucune double exécution possible).
		if etat.SessionID == "" || etat.SessionID != resume {
			fmt.Fprintf(os.Stderr, "claude-factice : SESSION_INCOHERENTE : %q != %q\n", resume, etat.SessionID)
			return 2
		}
	}
	etat.Appels++
	etat.SessionID = premierNonVide(etat.SessionID, sessionParDefaut)
	if err := sauverEtat(etat); err != nil {
		fmt.Fprintf(os.Stderr, "claude-factice : %v\n", err)
		return 2
	}
	sortie := map[string]any{
		"result":         resultat(prompt, etat.Appels, etat.SessionID),
		"session_id":     etat.SessionID,
		"is_error":       false,
		"total_cost_usd": 0.05,
		"num_turns":      1,
		"environment":    envRecue(),
	}
	if strings.Contains(prompt, "SCENARIO:SESSION_ABS") {
		delete(sortie, "session_id")
	}
	if strings.Contains(prompt, "SCENARIO:ERR_IS_ERROR") {
		sortie["is_error"] = true
		sortie["result"] = "échec simulé du run (factice)"
	}
	if strings.Contains(prompt, "SCENARIO:VIDE") {
		sortie["result"] = ""
	}
	corps, err := json.Marshal(sortie)
	if err != nil {
		return 2
	}
	if strings.Contains(prompt, "SCENARIO:JSON_INVALIDE") {
		fmt.Println("ceci n'est pas du JSON {result:,")
		return 0
	}
	if !sortieJSON {
		return 2
	}
	fmt.Println(string(corps))
	return 0
}

// resultat : texte du champ result par scénario.
func resultat(prompt string, appels int, session string) string {
	switch {
	case strings.Contains(prompt, "SCENARIO:FUIE"):
		return "lua resultat contient sk-ant-api03-EXEMPLESECRET123456789012 et xoxb-EXEMPLESECRET12345678900AAABBBB volontairement pour prouver l'expurgation (session " + session + ")"
	default:
		return fmt.Sprintf("LIVRABLE CLAUDE CODE FACTICE (run %d, session %s)", appels, session)
	}
}

// verifierParametres : le settings.json doit être lisible, JSON, et
// contenir des règles deny NON VIDES (contrôle embarqué n°3).
func verifierParametres(chemin string) bool {
	if chemin == "" {
		return false
	}
	data, err := os.ReadFile(chemin)
	if err != nil {
		return false
	}
	var s struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if json.Unmarshal(data, &s) != nil {
		return false
	}
	return len(s.Permissions.Deny) > 0
}

// chargerEtat/sauverEtat : sessions locales sous le HOME RÉÉCRIT.
func cheminEtat() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("HOME absent (isolation attendue)")
	}
	dir := filepath.Join(home, ".claude-factice")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "etat.json"), nil
}

func chargerEtat() (etatFactice, error) {
	var e etatFactice
	p, err := cheminEtat()
	if err != nil {
		return e, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return e, err
	}
	if json.Unmarshal(data, &e) != nil {
		return e, fmt.Errorf("SESSION_INCOHERENTE : etat.json corrompu")
	}
	return e, nil
}

func sauverEtat(e etatFactice) error {
	p, err := cheminEtat()
	if err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// envRecue : déclare EXACTEMENT l'environnement reçu (preuve de liste
// blanche : toute divulgation est détectée par le superviseur).
func envRecue() []string {
	cles := []string{}
	for _, kv := range os.Environ() {
		if k := strings.SplitN(kv, "=", 2)[0]; k != "" {
			cles = append(cles, k)
		}
	}
	sort.Strings(cles)
	return cles
}

func premierNonVide(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

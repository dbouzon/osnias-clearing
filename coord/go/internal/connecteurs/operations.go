// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coordengine/internal/missions"
	"coordengine/internal/persist"
)

// M-COORD-FLOW-01 : registre durable des OPÉRATIONS (plan v2 D2). Une
// opération est un aller-retour adaptateur complet RÉSERVER -> APPELER ->
// COMMITTER, identifié par id_operation propre à CHAQUE opération (D1).
// appel_en_cours.json ne conserve que le DERNIER appel : c'est le registre
// append-only operations.jsonl qui permet de RETROUVER les opérations
// terminées (ajustement pilote c). Fail closed : une ligne illisible bloque
// TOUTE nouvelle opération (OPERATIONS_ILLISIBLES) ; un registre ABSENT
// (mission antérieure à FLOW-01) est vide : cycle inchangé (D6).

// Types d'événements d'opération (journal du registre operations.jsonl).
const (
	TypeOperationOuverte     = "OPERATION_OUVERTE"
	TypeOperationTerminee    = "OPERATION_TERMINEE"
	TypeOperationEchouee     = "OPERATION_ECHOUEE"
	TypeOperationInterrompue = "OPERATION_INTERROMPUE"
	TypeOperationResolue     = "OPERATION_RESOLUE"
	TypeOperationRejetee     = "OPERATION_REJETEE"
)

// États dérivés d'une opération.
const (
	OperationEnCours     = "EN_COURS"
	OperationTerminee    = "TERMINEE"
	OperationEchouee     = "ECHOUEE"
	OperationInterrompue = "INTERROMPUE"
)

// EvenementOperation : une ligne du registre d'opérations.
type EvenementOperation struct {
	Type        string `json:"type"`
	IdOperation string `json:"id_operation"`
	Issue       string `json:"issue"`
	Action      string `json:"action"`
	Empreinte   string `json:"empreinte_charge,omitempty"`
	Acteur      string `json:"acteur,omitempty"`
	InstantUTC  string `json:"instant_utc"`
	Detail      string `json:"detail,omitempty"`
}

// EtatOperation : état dérivé d'une opération et données de compatibilité
// du rejeu (D5 : mission/action/charge/acteur doivent coïncider).
type EtatOperation struct {
	Etat      string `json:"etat"`
	Action    string `json:"action"`
	Empreinte string `json:"empreinte_charge"`
	Acteur    string `json:"acteur"`
}

// StatutOperations : comptage séparé exposé par -consulter (D7).
type StatutOperations struct {
	Terminees    int `json:"terminees"`
	Echouees     int `json:"echouees"`
	Interrompues int `json:"interrompues"`
	Resolues     int `json:"resolues"`
	EnCours      int `json:"en_cours"`
}

// cheminOperations : registre d'opérations, à côté du journal de mission.
func cheminOperations(racine, issue string) string {
	return filepath.Join(filepath.Dir(missions.JournalPath(racine, issue)), "operations.jsonl")
}

// journaliserOperation : append canonique (N-19) d'un événement d'opération.
// Le sérialiseur normatif n'accepte que des map[string]any : conversion
// explicite ici, jamais de struct directe.
func journaliserOperation(racine, issue string, ev EvenementOperation) error {
	if ev.InstantUTC == "" {
		ev.InstantUTC = time.Now().UTC().Format(time.RFC3339)
	}
	ligne := map[string]any{
		"type":         ev.Type,
		"id_operation": ev.IdOperation,
		"issue":        ev.Issue,
		"action":       ev.Action,
		"instant_utc":  ev.InstantUTC,
	}
	if ev.Empreinte != "" {
		ligne["empreinte_charge"] = ev.Empreinte
	}
	if ev.Acteur != "" {
		ligne["acteur"] = ev.Acteur
	}
	if ev.Detail != "" {
		ligne["detail"] = ev.Detail
	}
	_, err := persist.JournalExecute([]any{ligne}, cheminOperations(racine, issue))
	return err
}

// LireOperations : lecture fail closed du registre. Absent = vide (D6) ;
// toute ligne illisible = erreur OPERATIONS_ILLISIBLES (D2).
func LireOperations(racine, issue string) ([]EvenementOperation, error) {
	raw, err := os.ReadFile(cheminOperations(racine, issue))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.New("OPERATIONS_ILLISIBLES : " + err.Error())
	}
	var evs []EvenementOperation
	// compteur de lignes non vides pour un diagnostic précis.
	n := 0
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		n++
		var ev EvenementOperation
		if err := json.Unmarshal([]byte(l), &ev); err != nil ||
			ev.Type == "" || ev.IdOperation == "" {
			return nil, fmt.Errorf(
				"OPERATIONS_ILLISIBLES : registre des opérations de %s corrompu (ligne %d)",
				issue, n)
		}
		evs = append(evs, ev)
	}
	return evs, nil
}

// etatsOperations : dérive l'état de chaque opération depuis les événements,
// dans l'ordre d'écriture (le dernier événement d'état l'emporte ;
// OPERATION_REJETEE est informationnel : il ne change JAMAIS l'état).
func etatsOperations(evs []EvenementOperation) map[string]EtatOperation {
	etats := map[string]EtatOperation{}
	for _, ev := range evs {
		st, ok := etats[ev.IdOperation]
		if !ok {
			st = EtatOperation{Etat: OperationEnCours, Action: ev.Action,
				Empreinte: ev.Empreinte, Acteur: ev.Acteur}
		}
		switch ev.Type {
		case TypeOperationOuverte:
			st.Etat = OperationEnCours
		case TypeOperationTerminee:
			st.Etat = OperationTerminee
		case TypeOperationEchouee:
			st.Etat = OperationEchouee
		case TypeOperationInterrompue:
			st.Etat = OperationInterrompue
		case TypeOperationResolue:
			// décision pilote sur un appel non résolu : la résolution
			// (RECU/PERDU/ECARTE/ECHOUE) devient l'état, en préfixe
			// RESOLU_ (contrat D5 : rejouable sauf RESOLU_RECU).
			st.Etat = "RESOLU_" + ev.Detail
		case TypeOperationRejetee:
			// informationnel : aucun changement d'état (D5).
		}
		etats[ev.IdOperation] = st
	}
	return etats
}

// ResumeOperations : comptage séparé des opérations par état final (D7),
// exposé par coord-mission -consulter. Fail closed sur registre illisible.
func ResumeOperations(racine, issue string) (StatutOperations, error) {
	var st StatutOperations
	evs, err := LireOperations(racine, issue)
	if err != nil {
		return st, err
	}
	for _, e := range etatsOperations(evs) {
		switch e.Etat {
		case OperationTerminee:
			st.Terminees++
		case OperationEchouee:
			st.Echouees++
		case OperationInterrompue:
			st.Interrompues++
		case OperationEnCours:
			st.EnCours++
		default:
			st.Resolues++
		}
	}
	return st, nil
}

// trouverOperationRejeu : résout la cible d'un rejeu explicite.
// "derniere" désigne la dernière opération OUVERTE du registre ;
// toute autre valeur est un id_operation littéral.
func trouverOperationRejeu(evs []EvenementOperation, cible string) (string, bool) {
	if cible != "" && cible != "derniere" {
		return cible, true
	}
	derniere := ""
	for _, ev := range evs {
		if ev.Type == TypeOperationOuverte {
			derniere = ev.IdOperation
		}
	}
	return derniere, derniere != ""
}

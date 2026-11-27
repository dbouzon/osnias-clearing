// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package maieutique implements the pre-mission maieutic dialogue of COORD
// (mission M-COORD-MAIEUTIQUE-01, issue #42, plan v2 D2) : a pilot and a
// director (an AI connector) co-construct a QUALIFIED mission order through
// a bounded, journaled state machine, before any normative mission exists.
//
// Fail-closed design inherited from the plan v2 :
//   - STRICTLY ADDITIVE : this package only consumes PUBLIC APIs of the
//     engine (adaptateurs.Adaptateur, budget, connecteurs, missions,
//     persist, jcs) and never modifies normative state on its own.
//   - every state change goes through the transition table (D2) : any
//     unlisted action is refused (ACTION_INTERDITE).
//   - no permanent deadlock : repeated invalid answers or director
//     unavailability lead to ATTENTE_PILOTE, from which the pilot has
//     three explicit exits (relancer, rectifier, abandonner).
//   - persistence is atomic and canonical (persist.SaveAtomic over
//     jcs.Canonical maps) ; every transition is journaled (D10) with
//     actor, action, UTC instant and call/context counters.
package maieutique

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// États du projet maïeutique (plan v2 D2).
const (
	EtatIntention     = "INTENTION"
	EtatDialogue      = "DIALOGUE"
	EtatOrdrePropose  = "ORDRE_PROPOSE"
	EtatOrdreQualifie = "ORDRE_QUALIFIE"
	EtatValide        = "VALIDE"
	EtatPublie        = "PUBLIE"
	EtatTransmis      = "TRANSMIS"
	EtatAttentePilote = "ATTENTE_PILOTE"
	EtatInvalide      = "INVALIDE"
	EtatAbandonne     = "ABANDONNE"
	EtatSansProjet    = "" // état source de la création
)

// Actions de la table des transitions (plan v2 D2).
const (
	ActionDeclarerIntention = "declarer_intention"
	ActionOuvrirDialogue    = "ouvrir_dialogue"
	ActionTourDirecteur     = "tour_directeur"
	ActionProposerOrdre     = "proposer_ordre"
	ActionRetourDialogue    = "retour_dialogue"
	ActionQualifier         = "qualifier"
	ActionValiderPilote     = "valider_pilote"
	ActionPublier           = "publier"
	ActionTransmettre       = "transmettre"
	ActionInvalider         = "invalider"
	ActionRelancer          = "relancer"
	ActionRectifier         = "rectifier"
	ActionAbandonner        = "abandonner"
)

// Codes d'erreur STABLES du module (jamais reformulés : le service HTTP et
// les tests s'appuient sur ces identifiants).
const (
	CodeActionInterdit      = "ACTION_INTERDITE"
	CodeProjetInconnu       = "PROJET_INCONNU"
	CodeProjetExistant      = "PROJET_EXISTANT"
	CodeIntentionVide       = "INTENTION_VIDE"
	CodeBinomeIdentique     = "BINOME_IDENTIQUE"
	CodeDirecteurInconnu    = "DIRECTEUR_INCONNU"
	CodeExecutantInconnu    = "EXECUTANT_INCONNU"
	CodeOperationDejaFait   = "OPERATION_DEJA_EFFECTUEE"
	CodeOrdreAbsent         = "ORDRE_ABSENT"
	CodeOrdreNonConforme    = "ORDRE_NON_CONFORME"
	CodeObjectifNonUnique   = "OBJECTIF_NON_UNIQUE"
	CodeSourcesVides        = "SOURCES_VIDES"
	CodeSourceNonDeclaree   = "SOURCE_NON_DECLAREE"
	CodeSourcesNonVerifiees = "SOURCES_NON_VERIFIEES"
	CodePasDeControle       = "PAS_DE_CONTROLE_INTERMEDIAIRE"
	CodeLivrableImprecis    = "LIVRABLE_IMPRECIS"
	CodeEtapesIncoherentes  = "ETAPES_INCOHERENTES"
	CodeCritiqueBloquante   = "CRITIQUE_BLOQUANTE"
	CodeReponseInvalide     = "REPONSE_INVALIDE"
	CodeIndisponibilite     = "INDISPONIBILITE_DIRECTEUR"
	CodeEmpreinteDivergente = "EMPREINTE_DIVERGENTE"
	CodeJetonPiloteAbsent   = "JETON_PILOTE_ABSENT"
	CodeDocumentInvalide    = "DOCUMENT_INVALIDE"
	CodeProjetTerminal      = "PROJET_TERMINAL"
)

// Erreur est l'erreur structurée du module : un code STABLE plus un motif.
type Erreur struct {
	Code  string `json:"code"`
	Motif string `json:"motif"`
}

func (e Erreur) Error() string { return e.Code + " : " + e.Motif }

func erreur(code, motif string) error { return Erreur{Code: code, Motif: motif} }

// estErreurCode dit si err porte le code stable donné.
func estErreurCode(err error, code string) bool {
	var e Erreur
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// tableTransitions : plan v2 D2. Toute action non listée est REFUSÉE.
// Les boucles propres PUBLIE→publier et TRANSMIS→transmettre existent
// uniquement pour que l'opération idempotente puisse S'EXÉCUTER et rendre
// son refus explicite OPERATION_DEJA_EFFECTUEE (zéro second appel).
var tableTransitions = map[string]map[string]string{
	EtatSansProjet: {ActionDeclarerIntention: EtatIntention},
	EtatIntention:  {ActionOuvrirDialogue: EtatDialogue},
	EtatDialogue: {
		ActionTourDirecteur: EtatDialogue,
		ActionProposerOrdre: EtatOrdrePropose,
	},
	EtatOrdrePropose: {
		ActionQualifier:      EtatOrdreQualifie,
		ActionRetourDialogue: EtatDialogue,
	},
	EtatOrdreQualifie: {
		ActionValiderPilote:  EtatValide,
		ActionRetourDialogue: EtatDialogue,
	},
	EtatValide: {
		ActionPublier:   EtatPublie,
		ActionInvalider: EtatInvalide,
	},
	EtatPublie: {
		ActionTransmettre: EtatTransmis,
		ActionPublier:     EtatPublie, // boucle propre : idempotence
		ActionInvalider:   EtatInvalide,
	},
	EtatTransmis: {
		ActionTransmettre: EtatTransmis, // boucle propre : idempotence
	},
	EtatAttentePilote: {
		ActionRelancer:   EtatDialogue,
		ActionRectifier:  EtatDialogue,
		ActionAbandonner: EtatAbandonne,
	},
}

// Projet est l'état durable complet d'un projet maïeutique (D6).
type Projet struct {
	ID               string          `json:"id"`
	Etat             string          `json:"etat"`
	Intention        string          `json:"intention"`
	DirecteurID      string          `json:"directeur_id"`
	ExecutantID      string          `json:"executant_id"`
	Version          int             `json:"version"`         // version figée (vN)
	EmpreinteOrdre   string          `json:"empreinte_ordre"` // sha256 de l'ordre figé
	Ordre            *Ordre          `json:"ordre,omitempty"` // ordre courant
	SourcesDeclarees []string        `json:"sources_declarees"`
	UrlsAcceptees    map[string]bool `json:"urls_acceptees"`
	Cadrage          map[string]any  `json:"cadrage,omitempty"`
	Devis            map[string]any  `json:"devis,omitempty"`
	AccordDevis      bool            `json:"accord_devis"`
	EchecsDirecteur  int             `json:"echecs_directeur"` // indispo OU réponses invalides
	Appels           int             `json:"appels"`           // compteur d'appels directeur (D10)
	ContexteOctets   int             `json:"contexte_octets"`  // volume consommé (D10)
	InstantUTC       string          `json:"instant_utc"`
}

// maintenantUTC : horodatage UTC isolé (tests reproductibles).
func maintenantUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// Store est le magasin de projets maïeutiques, racine D6 :
// state/maieutique/<projet>/{intention.md, projet.json, maieutique.jsonl,
// documents/, ordres/, livrable/}.
type Store struct {
	cfg     Config
	mu      sync.Mutex
	verrous map[string]*sync.Mutex
}

// OuvrirStore ouvre (ou crée) la racine des projets maïeutiques.
func OuvrirStore(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Racine) == "" {
		return nil, errors.New("maieutique : racine vide interdite")
	}
	if err := ConfigParDefautAppliquee(&cfg); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Racine, 0o755); err != nil {
		return nil, err
	}
	return &Store{cfg: cfg, verrous: map[string]*sync.Mutex{}}, nil
}

// Config retourne la configuration effective du magasin (lecture seule).
func (s *Store) Config() Config { return s.cfg }

func (s *Store) verrou(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.verrous[id]
	if !ok {
		v = &sync.Mutex{}
		s.verrous[id] = v
	}
	return v
}

func (s *Store) dir(id string) string { return filepath.Join(s.cfg.Racine, id) }

func (s *Store) cheminProjet(id string) string {
	return filepath.Join(s.dir(id), "projet.json")
}

func (s *Store) cheminJournal(id string) string {
	return filepath.Join(s.dir(id), "maieutique.jsonl")
}

func (s *Store) cheminDialogue(id string) string {
	return filepath.Join(s.dir(id), "dialogue.jsonl")
}

// charger relit l'état durable (D6 : reprise de processus).
func (s *Store) charger(id string) (*Projet, error) {
	raw, err := os.ReadFile(s.cheminProjet(id))
	if err != nil {
		return nil, erreur(CodeProjetInconnu, "projet illisible ou absent : "+id)
	}
	var p Projet
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, erreur(CodeProjetInconnu, "projet non JSON : "+id)
	}
	return &p, nil
}

// sauver écrit l'état de façon atomique et CANONIQUE (persist.SaveAtomic
// exige une forme JCS : les structs Go passent par enMap).
func (s *Store) sauver(p *Projet) error {
	m, err := enMap(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir(p.ID), 0o755); err != nil {
		return err
	}
	p.InstantUTC = maintenantUTC()
	m, err = enMap(p)
	if err != nil {
		return err
	}
	return persistSaveAtomic(m, s.cheminProjet(p.ID))
}

// versJSONGenerique : convertit une valeur Go quelconque (struct, slices
// typées comme []string) en valeur JSON générique canonisable par JCS.
func versJSONGenerique(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// journaliser ajoute des entrées au journal du projet (D10, idempotent).
// Chaque entrée est convertie en forme générique : les slices typées
// ([]string) ne sont PAS canonisables par JCS telles quelles.
func (s *Store) journaliser(id string, entrees ...any) error {
	if err := os.MkdirAll(s.dir(id), 0o755); err != nil {
		return err
	}
	generiques := make([]any, 0, len(entrees))
	for _, e := range entrees {
		g, err := versJSONGenerique(e)
		if err != nil {
			return err
		}
		generiques = append(generiques, g)
	}
	_, err := persistJournal(generiques, s.cheminJournal(id))
	return err
}

// entreeJournal : forme normalisée d'une transition journalisée (D10).
func entreeJournal(acteur, action, avant, apres string, appels, contexte int, detail map[string]any) map[string]any {
	e := map[string]any{
		"type":             "TRANSITION_MAIEUTIQUE",
		"acteur":           acteur,
		"action":           action,
		"etat_avant":       avant,
		"etat_apres":       apres,
		"instant_utc":      maintenantUTC(),
		"appels_directeur": appels,
		"contexte_octets":  contexte,
	}
	for k, v := range detail {
		e[k] = v
	}
	return e
}

// transition applique la table D2 : garde, changement d'état, sauvegarde
// atomique, journal. Toute action non listée = ACTION_INTERDITE.
func (s *Store) transition(p *Projet, action, acteur string, detail map[string]any) error {
	cibles, ok := tableTransitions[p.Etat]
	if !ok {
		return erreur(CodeProjetTerminal, "état "+p.Etat+" sans action possible (terminal)")
	}
	apres, ok := cibles[action]
	if !ok {
		return erreur(CodeActionInterdit, action+" interdit depuis "+p.Etat)
	}
	avant := p.Etat
	p.Etat = apres
	if err := s.sauver(p); err != nil {
		return err
	}
	if detail == nil {
		detail = map[string]any{}
	}
	return s.journaliser(p.ID, entreeJournal(acteur, action, avant, apres, p.Appels, p.ContexteOctets, detail))
}

// enMap convertit une valeur Go en map JSON générique, forme acceptée par
// jcs.Canonical (qui ne sérialise PAS les structs Go directement).
func enMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("maieutique : conversion canonique impossible : %w", err)
	}
	return m, nil
}

// enListe convertit une valeur Go en slice JSON générique (journaux).
func enListe(v any) ([]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var l []any
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return l, nil
}

// EmpreinteMap : sha256 hexadécimal de la forme canonique JCS d'une valeur.
func EmpreinteMap(v any) (string, error) {
	b, err := canonique(v)
	if err != nil {
		return "", err
	}
	return sha256Hex(b), nil
}

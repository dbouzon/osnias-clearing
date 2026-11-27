// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package missions implements the multi-mission repository of COORD Pilot
// on atomic JSON files (conception §2.2, §2.5): one directory per mission
// (simple sharding, no global growing file), records written through
// persist.SaveAtomic (N-15), mission journal through persist.JournalExecute
// (N-19), per-mission write lock, index rebuilt at startup (documented O(n)).
// The documented migration threshold (~1000 missions or frequent
// multi-connection reads) towards SQLite is an explicit design datum
// (conception §2.5); it is NOT a hidden bottleneck.
package missions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"coordengine/internal/arret"
	"coordengine/internal/budget"
	"coordengine/internal/engine"
	"coordengine/internal/jcs"
	"coordengine/internal/persist"
	"coordengine/internal/policy"
)

// Enregistrement: the complete persisted record of one mission: the
// normative state machine instance, its habilitation configuration, its
// identities (EXECUTANT and RECEPTIONNAIRE, always distinct, N-05), its
// budget, its stop state, and the reprise-expected flag (correction C4:
// after a stop is lifted, the mission stays blocked until the normative
// resume device N-16/N-17 has been executed).
type Enregistrement struct {
	Issue           string              `json:"Issue"`
	Mission         engine.Mission      `json:"Mission"`
	Adaptateurs     []string            `json:"Adaptateurs"`
	Roles           map[string][]string `json:"Roles"`
	Identites       []policy.Identity   `json:"Identites"`
	Budget          budget.Budget       `json:"Budget"`
	Arret           arret.Etat          `json:"Arret"`
	RepriseAttendue bool                `json:"RepriseAttendue"`
}

// JournalPath names the mission journal file of an issue.
func JournalPath(racine, issue string) string {
	return filepath.Join(racine, nomDir(issue), "journal.jsonl")
}

// nomDir mappe un identifiant de mission vers un nom de dossier. Le codage
// est INJECTIF octet par octet (correction C6, réception complémentaire
// issue #27) : les octets sûrs [a-zA-Z0-9_-] sont conservés, tout autre
// octet est encodé « ~xx » en hexadécimal. '~' (0x7E) n'appartenant pas au
// jeu conservé, chaque séquence d'échappement est non ambiguë : deux
// identifiants distincts ne peuvent JAMAIS partager un dossier.
func nomDir(issue string) string {
	var b strings.Builder
	for i := 0; i < len(issue); i++ {
		c := issue[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			b.WriteByte(c)
		default:
			const hexa = "0123456789abcdef"
			b.WriteByte('~')
			b.WriteByte(hexa[c>>4])
			b.WriteByte(hexa[c&0x0f])
		}
	}
	out := b.String()
	if out == "" {
		out = "sans-nom"
	}
	return out
}

// Referentiel: the multi-mission repository rooted at racine.
type Referentiel struct {
	racine  string
	mu      sync.Mutex
	verrous map[string]*sync.Mutex
	index   map[string]bool // rebuilt at startup (conception §2.5)
}

// Ouvrir opens (or bootstraps) the repository and rebuilds the index.
func Ouvrir(racine string) (*Referentiel, error) {
	if strings.TrimSpace(racine) == "" {
		return nil, errors.New("referentiel : racine vide interdite")
	}
	if err := os.MkdirAll(racine, 0o755); err != nil {
		return nil, err
	}
	r := &Referentiel{
		racine:  racine,
		verrous: map[string]*sync.Mutex{},
		index:   map[string]bool{},
	}
	if err := r.reconstruireIndex(); err != nil {
		return nil, err
	}
	return r, nil
}

// OuvrirLectureSeule ouvre le référentiel SANS ÉCRIRE NI CRÉER QUOI QUE CE
// SOIT (revue PR #71) : la racine doit exister, sinon erreur. Usage :
// consultation (coord-mission -consulter), qui ne doit jamais créer de
// répertoire sur un chemin absent ou mal orthographié.
func OuvrirLectureSeule(racine string) (*Referentiel, error) {
	if strings.TrimSpace(racine) == "" {
		return nil, errors.New("referentiel : racine vide interdite")
	}
	info, err := os.Stat(racine)
	if err != nil {
		return nil, fmt.Errorf("referentiel : racine introuvable (lecture seule) : %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("referentiel : la racine n'est pas un dossier")
	}
	r := &Referentiel{
		racine:  racine,
		verrous: map[string]*sync.Mutex{},
		index:   map[string]bool{},
	}
	if err := r.reconstruireIndex(); err != nil {
		return nil, err
	}
	return r, nil
}

// Racine exposes the repository root (used by the service to compute
// journal paths without guessing).
func (r *Referentiel) Racine() string { return r.racine }

func (r *Referentiel) reconstruireIndex() error {
	entries, err := os.ReadDir(r.racine)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(r.racine, e.Name(), "mission.json")); err == nil {
			r.index[e.Name()] = true
		}
	}
	return nil
}

func (r *Referentiel) verrou(issue string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.verrous[issue]
	if !ok {
		v = &sync.Mutex{}
		r.verrous[issue] = v
	}
	return v
}

// Valider applies the fail-closed habilitation validation (N-01..N-05) to a
// candidate mission: engine policy config must be valid, and the identities
// must be qualified by declared adapters with distinct EXECUTANT and
// RECEPTIONNAIRE (no self-reception, N-05). An empty or malformed
// configuration makes every operation refused (fail closed).
func Valider(adaptateurs []string, roles map[string][]string, identites []policy.Identity) error {
	if errs := policy.ValidateConfig(adaptateurs, roles); len(errs) > 0 {
		return fmt.Errorf("habilitations invalides (N-01..N-05) : %s", strings.Join(errs, " ; "))
	}
	var exec, recep bool
	vus := map[string]bool{}
	for _, id := range identites {
		if !policy.Qualified(id, adaptateurs) {
			return fmt.Errorf("identite %q non qualifiee (adapteur %q hors liste declaree, N-01)",
				id.ID, id.QualifiedBy)
		}
		if vus[id.ID] {
			return fmt.Errorf("identite dupliquee : %s", id.ID)
		}
		vus[id.ID] = true
		if id.Role == "EXECUTANT" {
			exec = true
		}
		if id.Role == "RECEPTIONNAIRE" {
			recep = true
		}
	}
	if !exec || !recep {
		return errors.New("identites EXECUTANT et RECEPTIONNAIRE requises et distinctes (N-05, separation stricte)")
	}
	return nil
}

// Creer creates a mission record; refuses duplicates and invalid
// configurations. Writes atomically (N-15) and journals the creation.
func (r *Referentiel) Creer(issue string, adaptateurs []string, roles map[string][]string,
	identites []policy.Identity, plafonds budget.Plafonds) (*Enregistrement, error) {
	if strings.TrimSpace(issue) == "" {
		return nil, errors.New("creation : reference d'issue vide interdite")
	}
	if err := Valider(adaptateurs, roles, identites); err != nil {
		return nil, err
	}
	v := r.verrou(issue)
	v.Lock()
	defer v.Unlock()
	if _, err := r.charger(issue); err == nil {
		return nil, fmt.Errorf("creation : mission %s deja existante", issue)
	}
	enr := &Enregistrement{
		Issue:       issue,
		Mission:     *engine.New(issue),
		Adaptateurs: adaptateurs,
		Roles:       roles,
		Identites:   identites,
		Budget:      *budget.Nouveau(plafonds),
		Arret:       arret.Etat{},
	}
	if err := r.sauverSansVerrou(enr); err != nil {
		return nil, err
	}
	if err := r.journaliserSansVerrou(issue, []any{map[string]any{
		"type": "CREATION", "issue": issue, "etat": enr.Mission.State,
	}}); err != nil {
		return nil, err
	}
	return enr, nil
}

// Sauver persists the record atomically under the per-mission lock (N-15).
func (r *Referentiel) Sauver(enr *Enregistrement) error {
	v := r.verrou(enr.Issue)
	v.Lock()
	defer v.Unlock()
	return r.sauverSansVerrou(enr)
}

func (r *Referentiel) sauverSansVerrou(enr *Enregistrement) error {
	dir := filepath.Join(r.racine, nomDir(enr.Issue))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m, err := EnMap(enr)
	if err != nil {
		return err
	}
	if err := persist.SaveAtomic(m, filepath.Join(dir, "mission.json"), ""); err != nil {
		return err
	}
	r.mu.Lock()
	r.index[nomDir(enr.Issue)] = true
	r.mu.Unlock()
	return nil
}

// EnMap converts a record to JSON-decoded map values, the only form accepted
// by the normative canonical serializer (jcs, RFC 8785).
func EnMap(enr *Enregistrement) (map[string]any, error) {
	raw, err := json.Marshal(enr)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Empreinte returns the canonical SHA-256 fingerprint of the record (spec 5.3).
func Empreinte(enr *Enregistrement) (string, error) {
	m, err := EnMap(enr)
	if err != nil {
		return "", err
	}
	return jcs.Fingerprint(m)
}

// ErrIntrouvable : la mission n'existe pas dans le référentiel.
var ErrIntrouvable = errors.New("mission introuvable")

var errIntrouvable = ErrIntrouvable

// Copie retourne une copie indépendante (deep copy) de l'enregistrement,
// pour valider une transition SANS muter l'original (correction C2,
// réception complémentaire : la validation normative a lieu sur cette copie
// AVANT tout appel d'adaptateur).
func Copie(enr *Enregistrement) (*Enregistrement, error) {
	raw, err := json.Marshal(enr)
	if err != nil {
		return nil, err
	}
	var c Enregistrement
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ContexteMission : vue verrouillée d'une mission pendant une transaction.
// Le verrou de mission est DÉTENU du début à la fin de la transaction
// (lecture, validation, appel, mutation, journalisation, sauvegarde).
type ContexteMission struct {
	Issue string
	Enr   *Enregistrement
}

// Transaction sérialise l'intégralité d'une opération par mission
// (corrections C1/C3, réception complémentaire issue #27) : verrou de
// mission détenu du chargement à la sauvegarde, aucune requête concurrente
// ne peut ni perdre une mise à jour ni écraser un arrêt humain.
//
// fn replie la logique métier (validations, appels, mutations) sur
// ctx.Enr ; si fn échoue, l'enregistrement n'est PAS sauvegardé (état
// intact sur disque, rejet explicite) ; les journaux déjà écrits par fn
// restent tracés (refus explicites). Si fn réussit, l'enregistrement est
// sauvegardé (N-15) PUIS apres est exécuté sous le même verrou
// (journalisation post-persistance : une erreur de journal y est
// EXPLICITE et n'annule pas la sauvegarde, elle est retournée à l'appelant).
func (r *Referentiel) Transaction(issue string, fn func(*ContexteMission) error, apres func(*ContexteMission) error) error {
	v := r.verrou(issue)
	v.Lock()
	defer v.Unlock()
	enr, err := r.charger(issue)
	if err != nil {
		return err
	}
	ctx := &ContexteMission{Issue: issue, Enr: enr}
	if err := fn(ctx); err != nil {
		return err
	}
	if err := r.sauverSansVerrou(ctx.Enr); err != nil {
		return err
	}
	if apres == nil {
		return nil
	}
	return apres(ctx)
}

// Obtenir loads one mission record.
func (r *Referentiel) Obtenir(issue string) (*Enregistrement, error) {
	r.mu.Lock()
	if !r.index[nomDir(issue)] {
		r.mu.Unlock()
		return nil, errIntrouvable
	}
	r.mu.Unlock()
	return r.charger(issue)
}

func (r *Referentiel) charger(issue string) (*Enregistrement, error) {
	return r.chargerDossier(nomDir(issue))
}

// chargerDossier charge un enregistrement par son nom de dossier ENCODÉ
// (tel que retourné par Lister), SANS ré-encodage (C6 : l'encodage injectif
// n'est idempotent que sur les identifiants, pas sur les noms encodés).
func (r *Referentiel) chargerDossier(dir string) (*Enregistrement, error) {
	raw, err := os.ReadFile(filepath.Join(r.racine, dir, "mission.json"))
	if err != nil {
		return nil, errIntrouvable
	}
	var enr Enregistrement
	if err := json.Unmarshal(raw, &enr); err != nil {
		return nil, fmt.Errorf("mission %s : enregistrement illisible : %w", dir, err)
	}
	return &enr, nil
}

// ParDossier charge un enregistrement par son nom de dossier encodé (retourné
// par Lister), sans ré-encodage.
func (r *Referentiel) ParDossier(dir string) (*Enregistrement, error) {
	return r.chargerDossier(dir)
}

// Journaliser appends idempotent typed lines to the mission journal (N-19).
func (r *Referentiel) Journaliser(issue string, entrees ...any) error {
	v := r.verrou(issue)
	v.Lock()
	defer v.Unlock()
	return r.journaliserSansVerrou(issue, entrees)
}

// journaliserSansVerrou : à n'appeler qu'en tenant déjà le verrou de mission.
func (r *Referentiel) journaliserSansVerrou(issue string, entrees []any) error {
	_, err := persist.JournalExecute(entrees, JournalPath(r.racine, issue))
	return err
}

// Lister returns the sorted issue list from the index.
func (r *Referentiel) Lister() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for d := range r.index {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// EstConforme checks every mission configuration at service startup: a
// malformed or empty configuration anywhere makes the startup fail (R2/S5).
// Le parcours se fait PAR DOSSIER (noms encodés, sans ré-encodage, C6).
func (r *Referentiel) EstConforme() error {
	for _, dir := range r.Lister() {
		enr, err := r.chargerDossier(dir)
		if err != nil {
			return fmt.Errorf("demarrage : mission %s illisible : %w", dir, err)
		}
		if err := Valider(enr.Adaptateurs, enr.Roles, enr.Identites); err != nil {
			return fmt.Errorf("demarrage : mission %s non conforme (fail closed) : %w", dir, err)
		}
	}
	return nil
}

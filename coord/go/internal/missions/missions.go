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
// budget, and its stop state.
type Enregistrement struct {
	Issue       string              `json:"Issue"`
	Mission     engine.Mission      `json:"Mission"`
	Adaptateurs []string            `json:"Adaptateurs"`
	Roles       map[string][]string `json:"Roles"`
	Identites   []policy.Identity   `json:"Identites"`
	Budget      budget.Budget       `json:"Budget"`
	Arret       arret.Etat          `json:"Arret"`
}

// JournalPath names the mission journal file of an issue.
func JournalPath(racine, issue string) string {
	return filepath.Join(racine, nomDir(issue), "journal.jsonl")
}

func nomDir(issue string) string {
	b := strings.Builder{}
	for _, r := range issue {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
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

var errIntrouvable = errors.New("mission introuvable")

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
	raw, err := os.ReadFile(filepath.Join(r.racine, nomDir(issue), "mission.json"))
	if err != nil {
		return nil, errIntrouvable
	}
	var enr Enregistrement
	if err := json.Unmarshal(raw, &enr); err != nil {
		return nil, fmt.Errorf("mission %s : enregistrement illisible : %w", issue, err)
	}
	return &enr, nil
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
func (r *Referentiel) EstConforme() error {
	for _, dir := range r.Lister() {
		enr, err := r.charger(dir)
		if err != nil {
			return fmt.Errorf("demarrage : mission %s illisible : %w", dir, err)
		}
		if err := Valider(enr.Adaptateurs, enr.Roles, enr.Identites); err != nil {
			return fmt.Errorf("demarrage : mission %s non conforme (fail closed) : %w", dir, err)
		}
	}
	return nil
}

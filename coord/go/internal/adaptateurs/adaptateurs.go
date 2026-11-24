// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package adaptateurs defines the COORD Pilot adapter interface,
// INDEPENDENT of A2A and of any provider (mission #27, step 2), plus the
// ONLY implementation of this MVP: Simule, a simulated adapter which makes
// NO network call, holds NO credential, and refuses any attempt to run in
// a "real" mode (fail closed, lesson of tools/ai-coordinator T02).
// Providers are identified by opaque IDs (mistral, openai, ...) with no
// vendor privilege: any configured ID behaves exactly the same.
package adaptateurs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Jetons facturés (déclarés, simulés) par appel d'adaptateur simulé. Ce sont
// aussi les estimations utilisées par le contrôle budgétaire AVANT appel.
const (
	JetonsExecuter  = 1000
	JetonsSoumettre = 250
)

// JetonsEstimes returns the simulated token estimate for an action.
func JetonsEstimes(action string) int {
	switch action {
	case "executer":
		return JetonsExecuter
	case "soumettre":
		return JetonsSoumettre
	}
	return 0
}

// Resultat is the outcome of one simulated adapter call.
type Resultat struct {
	Jetons int    `json:"jetons"`
	Sortie string `json:"sortie"`
	Simule bool   `json:"simule"`
}

// Adaptateur is the provider- and transport-independent interface of COORD
// Pilot (accepted plan §3.3). "recevoir" is deliberately absent: receiving a
// verdict is a decision recorded in COORD, not an adapter call.
type Adaptateur interface {
	ID() string
	Executer(mandat string) (Resultat, error)
	Soumettre(livrable string) (Resultat, error)
}

// Simule: deterministic, in-memory only. By construction it performs no I/O
// beyond a pure computation; there is no code path that could reach the
// network, no credential field, no configuration of an endpoint.
type Simule struct {
	id string
}

// NouveauSimule builds a simulated adapter. Fail closed: an identifier that
// tries to switch to a "real" mode (any occurrence of "reel"/"real") is
// refused at construction time.
func NouveauSimule(id string) (*Simule, error) {
	bas := strings.ToLower(strings.Join(strings.Fields(id), ""))
	if id == "" || strings.Contains(bas, "reel") || strings.Contains(bas, "real") {
		return nil, fmt.Errorf("adaptateur %q interdit : le MVP accepte des adaptateurs SIMULES uniquement (fail closed)", id)
	}
	return &Simule{id: id}, nil
}

// ID returns the opaque adapter identifier.
func (s *Simule) ID() string { return s.id }

// Executer simulates the execution of a mandate (transition executer).
func (s *Simule) Executer(mandat string) (Resultat, error) {
	h := sha256.Sum256([]byte("executer|" + s.id + "|" + mandat))
	sortie := "EXECUTION SIMULEE par [" + s.id + "] empreinte provisoire " +
		hex.EncodeToString(h[:])[:16] + " (aucun appel reel, aucun reseau)"
	return Resultat{Jetons: JetonsExecuter, Sortie: sortie, Simule: true}, nil
}

// Soumettre simulates the submission of a deliverable (transition soumettre).
func (s *Simule) Soumettre(livrable string) (Resultat, error) {
	h := sha256.Sum256([]byte("soumettre|" + s.id + "|" + livrable))
	sortie := "SOUMISSION SIMULEE par [" + s.id + "] empreinte " +
		hex.EncodeToString(h[:])[:16] + " (aucune cle, aucun reseau)"
	return Resultat{Jetons: JetonsSoumettre, Sortie: sortie, Simule: true}, nil
}

// Registre builds the registry of simulated adapters from a configuration
// list (N-01: non-empty, no duplicates). Fail closed on invalid entries.
func Registre(ids []string) (map[string]Adaptateur, error) {
	vus := map[string]bool{}
	reg := map[string]Adaptateur{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("registre : identifiant d'adaptateur vide")
		}
		if vus[id] {
			return nil, errors.New("registre : doublon d'adaptateur " + id)
		}
		s, err := NouveauSimule(id)
		if err != nil {
			return nil, err
		}
		vus[id] = true
		reg[id] = s
	}
	if len(reg) == 0 {
		return nil, errors.New("registre : liste d'adaptateurs vide (N-01)")
	}
	return reg, nil
}

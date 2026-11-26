// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// registre.go : registre de sessions Claude Code VERROUILLÉ par
// fichier exclusif (M-COORD-LLM-05, plan v2, réponse au point bloquant
// n°1 de la critique OpenAI). TOUT accès — lecture comme écriture —
// passe par le verrou ; conflit de verrou = REFUS fail closed, jamais
// une attente indéterminée. Écriture STRICTEMENT postérieure à la
// validation du résultat (un run raté N'OUVRE PAS de session), via
// temporaire + renommage : un fichier partiel n'est jamais visible.
package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Erreurs du registre (fail closed, motifs constants documentés).
var (
	ErrRegistreVerrouIndisponible = errors.New("claudecode : REGISTRE_VERROU_INDISPONIBLE : registre déjà tenu (accès refusé, fail closed, jamais une attente indéterminée)")
	ErrRegistreCorrompu           = errors.New("claudecode : REGISTRE_CORROMPU : fichier de sessions non JSON valide (refus explicite au chargement, fail closed)")
	ErrRegistreEcriture           = errors.New("claudecode : REGISTRE_ECRITURE : écriture du registre impossible (fail closed)")
)

// SessionCC : lien durable (mission, révision) → session Claude Code.
// La révision corrélée est conservée pour audit ; la clé de reprise est
// la PAIRE (mission, révision), reprise = `--resume` de l'ID stocké.
type SessionCC struct {
	Mission   string `json:"mission"`
	Revision  string `json:"revision"`
	SessionID string `json:"session_id"`
}

// RegistreClaude : registre sur fichier JSON protégé par verrou exclusif.
type RegistreClaude struct {
	chemin string // fichier sessions
	verrou string // fichier verrou (chemin distinct)
}

// NouveauRegistreClaude : crée (si absent) le registre dans le répertoire
// donné (0700, fichiers 0600). Le verrou vit à côté (fichier distinct).
func NouveauRegistreClaude(repertoire string) (*RegistreClaude, error) {
	if err := os.MkdirAll(repertoire, 0o700); err != nil {
		return nil, err
	}
	r := &RegistreClaude{
		chemin: filepath.Join(repertoire, "sessions-claudecode.json"),
		verrou: filepath.Join(repertoire, "sessions-claudecode.lock"),
	}
	if _, err := os.Stat(r.chemin); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(r.chemin, []byte("{}"), 0o600); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// accéder : exécute f SOUS VERROU exclusif non bloquant, avec REPRISES
// BORNÉES (200 essais × 5 ms = 1 s max : la contention normale des
// écritures concurrentes passe, un verrou durablement tenu reste un
// REFUS net, jamais une attente indéterminée — CC-CONCUR1/CC-CONCUR2).
func (r *RegistreClaude) accéder(f func() error) error {
	v, err := os.OpenFile(r.verrou, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("%w : %s", ErrRegistreVerrouIndisponible, err)
	}
	defer v.Close()
	var dernier error
	for i := 0; i < 200; i++ {
		if dernier = verrouiller(v); dernier == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if dernier != nil {
		return ErrRegistreVerrouIndisponible
	}
	defer deverrouiller(v)
	return f()
}

// charger : lecture des sessions SOUS VERROU. Fichier absent = vide ;
// fichier corrompu (trongué, non JSON) = REFUS explicite.
func (r *RegistreClaude) charger() (map[string]SessionCC, error) {
	out := map[string]SessionCC{}
	lire := func() error {
		data, err := os.ReadFile(r.chemin)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return ErrRegistreCorrompu
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return ErrRegistreCorrompu // CC-REINTEG
		}
		return nil
	}
	if err := r.accéder(lire); err != nil {
		return nil, err
	}
	return out, nil
}

// Chercher : session de la paire (mission, révision), SOUS VERROU.
func (r *RegistreClaude) Chercher(mission, revision string) (SessionCC, bool, error) {
	sessions, err := r.charger()
	if err != nil {
		return SessionCC{}, false, err
	}
	s, ok := sessions[mission+"/"+revision]
	return s, ok, nil
}

// Enregistrer : écrit la session SOUS VERROU, via écrireAtomique
// (temporaire + renommage). Appelé UNIQUEMENT après validation du
// résultat du run (garanti par l'exécuteur, jamais ici).
func (r *RegistreClaude) Enregistrer(s SessionCC) error {
	ecrire := func() error {
		sessions, err := r.chargerSansVerrou()
		if err != nil {
			return err
		}
		sessions[s.Mission+"/"+s.Revision] = s
		data, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			return ErrRegistreCorrompu
		}
		tmp := r.chemin + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return fmt.Errorf("%w : %s", ErrRegistreEcriture, err)
		}
		if err := os.Rename(tmp, r.chemin); err != nil {
			return fmt.Errorf("%w : %s", ErrRegistreEcriture, err)
		}
		return nil
	}
	return r.accéder(ecrire)
}

// chargerSansVerrou : lecture brute (le verrou est DÉJÀ tenu par
// accéder ; jamais appelé hors de ce cadre).
func (r *RegistreClaude) chargerSansVerrou() (map[string]SessionCC, error) {
	data, err := os.ReadFile(r.chemin)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]SessionCC{}, nil
		}
		return nil, err
	}
	out := map[string]SessionCC{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, ErrRegistreCorrompu
	}
	return out, nil
}

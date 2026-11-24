// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package persist implements COORD Specification 1.0 section 7 (N-15):
// atomic persistence with normalized crash points for testing. A crash never
// leaves a partial record: either the previous complete record or the new one.
package persist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coordengine/internal/jcs"
)

// CrashSimule is the injected crash (tests and demonstration).
type CrashSimule struct{ Point string }

func (c CrashSimule) Error() string { return "crash simule: " + c.Point }

// PointsArret: normalized crash points (spec 7).
var PointsArret = []string{"ecriture", "avant_remplacement", "apres_remplacement"}

// SaveAtomic writes record in canonical form atomically: temporary file,
// fsync, os.Rename over the target, directory fsync. crashAt injects a crash
// at a named point (empty string = no injection):
//   - ecriture: temp truncated, original INTACT;
//   - avant_remplacement: temp complete, original INTACT;
//   - apres_remplacement: new content IN PLACE.
func SaveAtomic(record any, path string, crashAt string) error {
	if crashAt != "" && !inPoints(crashAt) {
		return fmt.Errorf("point d'arret inconnu: %s", crashAt)
	}
	contenu, err := jcs.Canonical(record)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if crashAt == "ecriture" {
		_, _ = f.Write(contenu[:len(contenu)/2])
		_ = f.Sync()
		_ = f.Close()
		return CrashSimule{Point: crashAt}
	}
	if _, err = f.Write(contenu); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if crashAt == "avant_remplacement" {
		return CrashSimule{Point: crashAt}
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	if dir, derr := os.Open(filepath.Dir(abs(path))); derr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	if crashAt == "apres_remplacement" {
		return CrashSimule{Point: crashAt}
	}
	return nil
}

// JournalExecute appends canonical JSON lines to journalPath, idempotently:
// an identical line already present is not duplicated (spec 9, N-19; also the
// conflicts journal of section 8). Returns the number of added lines.
func JournalExecute(entries []any, journalPath string) (int, error) {
	existants := map[string]bool{}
	if b, err := os.ReadFile(journalPath); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				existants[l] = true
			}
		}
	}
	added := 0
	f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	for _, e := range entries {
		l, err := jcs.Canonical(e)
		if err != nil {
			return added, err
		}
		if existants[string(l)] {
			continue
		}
		if _, err = f.Write(append(l, '\n')); err != nil {
			return added, err
		}
		if err = f.Sync(); err != nil {
			return added, err
		}
		existants[string(l)] = true
		added++
	}
	return added, nil
}

// NettoyerTemporaire removes an orphaned temporary file after a crash.
func NettoyerTemporaire(path string) bool {
	if _, err := os.Stat(path + ".tmp"); err == nil {
		return os.Remove(path+".tmp") == nil
	}
	return false
}

func inPoints(p string) bool {
	for _, x := range PointsArret {
		if x == p {
			return true
		}
	}
	return false
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package claudecode

import (
	"os"
	"syscall"
)

// verrouiller : verrou fichier EXCLUSIF NON BLOQUANT (flock POSIX) :
// un registre déjà tenu est REFUSÉ (fail closed), jamais une attente
// indéterminée (CC-CONCUR2).
func verrouiller(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// deverrouiller : relâche le verrou (défermé par l'appelant).
func deverrouiller(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package claudecode

import (
	"os"

	"golang.org/x/sys/windows"
)

// verrouiller : verrou fichier EXCLUSIF NON BLOQUANT (LockFileEx,
// échec immédiat LOCKFILE_FAIL_IMMEDIATELY) : un registre déjà tenu est
// REFUSÉ (fail closed), jamais une attente indéterminée (CC-CONCUR2).
func verrouiller(f *os.File) error {
	h := windows.Handle(f.Fd())
	var ol windows.Overlapped
	return windows.LockFileEx(h,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol)
}

// deverrouiller : relâche le verrou (défermé par l'appelant).
func deverrouiller(f *os.File) error {
	h := windows.Handle(f.Fd())
	var ol windows.Overlapped
	return windows.UnlockFileEx(h, 0, 1, 0, &ol)
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8892-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package mcpserver

import (
	"fmt"
	"os"
	"syscall"
)

// uidFichier extrait l'UID propriétaire du fichier (POSIX), ^uint64(0) si
// le type système est indisponible (contrôle propriétaire alors sans objet).
func uidFichier(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Uid)
	}
	return ^uint64(0)
}

// verifierProtectionJeton (POSIX, SEC-MCP-01/02) : permissions
// strictement 0600 et propriétaire = utilisateur courant (contrôle
// sans objet si l'UID ne peut pas être lu).
func verifierProtectionJeton(chemin string, info os.FileInfo) error {
	if info.Mode().Perm() != 0o600 {
		return erreurJeton(chemin, fmt.Sprintf("permissions %o invalides : exiger 0600 (chmod 0600)", info.Mode().Perm()))
	}
	if os.Getuid() >= 0 && uint64(os.Getuid()) != uidFichier(info) {
		return erreurJeton(chemin, "le fichier n'appartient pas a l'utilisateur courant")
	}
	return nil
}

// protegerFichierJeton (POSIX) : chmod explicite 0600. La création est déjà
// restrictive (fichier temporaire O_EXCL 0600, l'umask ne peut pas élargir).
func protegerFichierJeton(chemin string) error {
	if err := os.Chmod(chemin, 0o600); err != nil {
		return erreurJeton(chemin, "chmod 0600 impossible")
	}
	return nil
}

// remplacerFichierJeton (POSIX) : renommage atomique. Un lien symbolique
// éventuel serait remplacé comme entrée de répertoire, jamais suivi.
func remplacerFichierJeton(src, dst string) error {
	return os.Rename(src, dst)
}

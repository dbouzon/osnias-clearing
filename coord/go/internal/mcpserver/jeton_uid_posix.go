// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package mcpserver

import (
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

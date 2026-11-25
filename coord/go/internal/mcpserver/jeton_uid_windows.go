// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package mcpserver

import "os"

// uidFichier : sous Windows, la notion d'UID POSIX n'existe pas ; le contrôle
// propriétaire est sans objet (les permissions du mode sont également
// ignorées) et la protection repose sur le système de fichiers local.
func uidFichier(info os.FileInfo) uint64 { return ^uint64(0) }

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import "syscall"

// pgidProcessus retourne le PGID réel du processus (Unix, T3).
func pgidProcessus(pid int) (int, error) {
	return syscall.Getpgid(pid)
}

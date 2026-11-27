// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

// pgidProcessus : Windows n'a pas de PGID ; le témoin porte le PID seul et
// le lanceur de recette (bash/Unix) n'est pas utilisé sous Windows.
func pgidProcessus(pid int) (int, error) {
	return pid, nil
}

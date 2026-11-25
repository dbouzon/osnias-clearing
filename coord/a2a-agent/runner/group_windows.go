// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package runner

import (
	"os/exec"
	"strconv"
)

// sigTermine/sigTue : sous Windows il n'existe pas de signaux POSIX ni
// de groupes ; la destruction est ARBORESCENTE via taskkill (/T = tout
// l'arbre du PID, /F = forcé). Sémantique équivalente documentée : le
// CLI ET ses sous-processus sont atteints (S14/S15).
func sigTermine() int { return 0 }
func sigTue() int     { return 0 }

// configurerGroupe : pas d'attribut de groupe sous Windows (no-op ;
// la destruction passe par taskkill sur le PID du leader).
func configurerGroupe(cmd *exec.Cmd) {}

// tuerProcessus : taskkill /PID <pid> /T /F (arbre complet). La
// distinction SIGTERM/SIGKILL n'existe pas : arrêt immédiat forcé,
// "session résumable" non garantie sous Windows (limite documentée
// du plan v2 §3 T4).
func tuerProcessus(pid, sig int) {
	// taskkill exige des arguments SEPARES : "/PID <pid>" en un seul
	// argument est rejeté ("option - '/PID 1234' invalide") et le kill
	// échouait silencieusement (S15 : le processus muet vivait jusqu'à
	// sa durée naturelle). Correctif M-COORD-LLM-03, CI windows.
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

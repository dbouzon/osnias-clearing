// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package runner

import (
	"os/exec"
	"strconv"
)

// sigTermine/sigTue : sous Windows il n'existe pas de signaux POSIX ni de
// groupes. La phase gracieuse (sigTermine) est donc SANS EFFET : le
// sous-processus reçoit sa grâce entière, puis la destruction est
// ARBORESCENTE et forcée via taskkill (/T = tout l'arbre, /F = forcé).
// Sémantique équivalente documentée : le CLI ET ses sous-processus sont
// atteints (S15), Expulse n'est marqué qu'après la grâce écoulée.
func sigTermine() int { return 0 }
func sigTue() int     { return 1 }

// configurerGroupe : pas d'attribut de groupe sous Windows (no-op ;
// la destruction passe par taskkill sur le PID du leader).
func configurerGroupe(cmd *exec.Cmd) {}

// tuerProcessus : SEULE la phase sigTue (expulsion) détruit l'arbre :
// la phase gracieuse (sigTermine) est un no-op documenté (pas de SIGTERM
// sous Windows). taskkill exige des arguments SEPARES : "/PID <pid>" en
// un seul argument est rejeté ("option - '/PID 1234' invalide") et le
// kill échouait silencieusement, tout comme tuer dès la phase gracieuse
// court-circuitait la grâce (Expulse jamais marqué). Correctifs
// M-COORD-LLM-03, CI windows (S15).
func tuerProcessus(pid, sig int) {
	if sig == sigTermine() {
		return // pas d'arrêt coopératif sous Windows (limite documentée)
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

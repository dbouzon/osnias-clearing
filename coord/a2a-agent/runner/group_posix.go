// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// sigTermine/sigTue : signaux POSIX réels (S14 : arrêt coopératif ;
// S15 : destruction du GROUPe).
func sigTermine() int { return int(syscall.SIGTERM) }
func sigTue() int     { return int(syscall.SIGKILL) }

// configurerGroupe : le lancement crée un GROUPE de processus distinct
// (Setpgid) : le SIGTERM/SIGKILL du groupe atteint le CLI ET ses
// sous-agents, sans léser le superviseur.
func configurerGroupe(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// tuerProcessus : signal au GROUPE (pid négatif), pas au seul leader.
// Sous Windows, voir group_windows.go.
func tuerProcessus(pid, sig int) {
	syscall.Syscall(syscall.SYS_KILL, uintptr(-pid), uintptr(sig), 0)
}

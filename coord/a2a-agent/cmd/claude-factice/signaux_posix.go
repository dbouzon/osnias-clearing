// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"os/signal"
	"syscall"
	"time"
)

// ignorerTerminaison : le scénario HANGUE ignore SIGTERM (prouve que le
// superviseur expulse via SIGKILL du GROUPE, jamais une attente infinie).
func ignorerTerminaison() { signal.Ignore(syscall.SIGTERM) }

// bloque : boucle ininterrompue jusqu'à l'expulsion du groupe.
func bloque() { time.Sleep(time.Hour) }

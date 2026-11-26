// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import "time"

// ignorerTerminaison/bloque : sous Windows, le scénario HANGUE est
// écarté par la recette (le groupe est détruit par taskkill /T, S15) ;
// ces talons conservent la compilation multi-OS.
func ignorerTerminaison() {}
func bloque()             { time.Sleep(time.Hour) }

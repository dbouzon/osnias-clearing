# REPORT — M-COORD-LLM-02: Gemini connector + direct A2A rail

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX: Apache-2.0.

Mission: issue #48 (Osnias Clearing repo). Base: `main` @ `161cb304` (pinned codeload tarball, sha256 `afaca4dd…`). Operational reference: COORD v1.5. Independent reception: OpenAI (ChatGPT).

## 1. Deliverable

- **`gemini` profile** (`coord/go/internal/connecteurs/gemini.go`): OpenAI-compatible request format, bounded `raisonnement` {none, minimal, low, medium, high} (default `low` logged, unknown value rejected at load), **stop-only finish** (any other reason: `FIN_NON_NOMINAL`/`GEMINI_SANS_CANDIDAT`), exact model required (`MODELE_DIVERGENT` unchanged), shared monetary vault (`EnveloppeCentimes`/`CoutConservateurJetons`).
- **Direct A2A rail** (`coord/go/internal/a2a/`): `rail.go` (delegation capturing the result at send time, atomic durable JSON registry 0600, `X-Coord-Rail-Jeton` token, loopback enforced, pilot-exclusive `ResoudrePilote` for uncertain task resolution), `adaptateur.go` (bounded 20 s wait loop, errors `RESULTAT_NON_TRANSMIS`/`TACHE_TERMINALE_NEGATIVE`/`SAISIE_ATTENDUE`/`BORNE_ATTEINTE`), `factices.go` (loopback mock A2A agents, standard A2A card, 401 without token, artifact contract `{"jetons":N,"texte":"…"}`).
- **Test suite**: 11 Gemini connector tests (C1–C9, C14 pre-network ceiling, C15 trapped verdicts never committed) + 12 rail/demo tests (E1–E10 + full real COORD cycle: plan critique via A2A → execution → A_CORRIGER → correction → ACCEPTEE → ACK → pilot closure).
- **CI**: race job extended to `./internal/a2a/`; `github.com/a2aproject/a2a-go/v2 v2.9.0` as a direct dependency.
- **Docs**: `connecteur/Gemini/` (README, SECRET-FREE config example, NOT-EXECUTED real-world acceptance procedure, this FR/EN report).

## 2. Arbitrations (plan v2, PLAN_ACCEPTE)

A1: `openai` format + `gemini` profile, stop-only; native API documented, NOT EXECUTED. A2: bounded reasoning. A3: exact model + `MODELE_DIVERGENT`. A4: CLI/MCP/frontend out of scope. Q3: rail in `coord/go/internal/a2a` (a2a-go v2.9.0). Q4: uncertain task resolution is pilot-exclusive (SEC-01), no automation.

## 3. Simulated qualification (0 EUR)

- Go: `go vet ./...` clean; `go test ./... -count=1` **all packages ok** (63 legacy connector tests + 11 Gemini + 12 a2a).
- Non-regression: `conformite-v15` vectors **22/22**, summary sha `8c097f76…`; `coord-demo` **19 CONFORME / 0 ECART**, sha `e1f1cdbe…` (identical to baseline); Python suites **135 PASS** (24+10+13+11+13+17+47); `coord/exp` tests + conduite **PASS**.
- Measurements (sandbox, Go 1.27.4): full build ~2.8 s; demo binary ~4.8 MB; demo max RSS ~10.6 MB; a2a suite ~1.6 s. Fits a 2 GB VPS target.

## 4. Reservation

**Real model/echo acceptance NOT EXECUTED** (0 EUR budget): procedure and conditions in `recette-reelle-gemini.md` (written pilot authorization, armed ceiling, fingerprinted requests). Fine-grained version check (`modelVersion`) belongs to the native API, out of scope.

## 5. Decision

Merging branch `mission/coord-llm-02` and closing issue #48: **reserved to the pilot**.

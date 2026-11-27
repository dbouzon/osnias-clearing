# Report M-COORD-FLOW-01 — Durable operation identity and reproducible acceptance run

Order of 27/11/2026 (issue #76, dbouzon/osnias-clearing): durable operation identity, duplicate-free resume, correlated journals, reproducible VPS acceptance. Executor Mistral, director/receiver OpenAI, pilot Denis Bouzon. Pilot's explicit adjustments integrated: (1) mission+action+revision do not identify an operation; (2) a legacy state without identity stays readable but never triggers a new automatic call while a prior result is uncertain; (3) document how to find completed operations. Budget €0, loopback fakes only, no keys. Base: main `de1ef681`. Branch: `mission/coord-flow-01` (head `fd420507306f4cb229535bfb7489995d49405105`).

## 1. Delivered

| Deliverable | Content |
|---|---|
| `internal/connecteurs/operations.go` | append-only registry `operations.jsonl`, events OPENED/COMPLETED/FAILED/INTERRUPTED/RESOLVED/REJECTED, per-state aggregation |
| `internal/connecteurs/orchestre.go` | crypto/rand `id_operation` persisted before the call, journal correlation, explicit replay `AvecRejeu`, fail-closed TX1/TX2 writes |
| `cmd/coord-mission/main.go` | `-rejeu` flag, operation counting in `-consulter` |
| `internal/connecteurs/flow01_operations_test.go` | 8 Go tests of the replay contract |
| `cmd/harnais-flow01/` + `recette/flow01/` | acceptance harness and reproducible launcher (T3) |

## 2. Replay contract (validated)

TERMINEE → `OPERATION_DEJA_TERMINEE` (zero calls, zero budget); ECHOUEE/RESOLU_PERDU/ECARTE/ECHOUE → relaunch SAME identity, FRESH key; EN_COURS → `ErrAppelEnCours`; INTERROMPUE/RESOLU_RECU → `OPERATION_NON_REJOUABLE`; unknown → `OPERATION_INCONNUE`; divergent action/payload/actor → `OPERATION_INCOMPATIBLE` before any call. Refusals journaled `OPERATION_REJETEE` (informational).

## 3. Verification

- Go tests: 16 packages green on the pushed tarball, TestFlow01 8/8, gofmt/vet clean, `GOOS=windows go vet` OK (PGID isolated via build tags).
- FAIL reproductions on main documented (the API does not exist there: tests do not compile), then PASS on the branch.
- Local acceptance run (sandbox): **PASS=37 FAIL=0**, opposed controls OK.
- VPS acceptance run (coord, 200.234.70.137) with kit pinned at `fd420507`: **PASS=37 FAIL=0**, V/N+P/E/X/R/I scenarios conformant, SIGKILL strictly PID-targeted (witness PID/PGID/cmdline verified before signalling), fakes intact, nginx active, no residual process. Trace archive sha256 `2850220642…` kept on the VPS.
- Opposed controls (`controles-flow01.py`): full journal↔registry correlation (16/16+3/3+3/3), zero duplicates (N-19), readable registries, one OPENED per operation, refusals traced.

## 4. Decisions and limits

- Separate counting completed/failed/interrupted/in-progress exposed by `-consulter` (pilot adjustment 3).
- Missing registry = empty (legacy missions readable); corrupt registry = full block `OPERATIONS_ILLISIBLES` (fail closed).
- Merge and closure remain reserved to the pilot (order of 27/11).

## 5. Traceability

Issue #76: ACK 7208328839; plan v1 A_CORRIGER; plan v2 (sha256 `e8ae4805c10f2be91163f5152448e2521aeaf8c2c30a4d38b5b87f9cf58e5de8`) PLAN_ACCEPTE, recorded 7208348372; T2 7208458373/7212222845; branch pushed 7212413389; T3 7213453330; T4 7213482396. OpenAI receptions (bridge session `m-coord-flow-01`): RECEPTION_INTERMEDIAIRE_ACCEPTEE, T2 revalidated without reservation, T3 ACCEPTEE, T4 ACCEPTEE. Commits: `18f10eda` (identity, registry, replay), `d7ed21d5` (gofmt), `fd420507` (harness + acceptance kit).

Documentation: `docs/identites-operations-et-rejeu.md` (FR) and `docs/operation-identities-and-replay.en.md` (EN).

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

# Mission report M-COORD-RAIL-01 — Shared API/A2A multi-LLM rail

- Mission: M-COORD-RAIL-01 (issue #50, mission order comment 7175583376)
- Executor: Mistral (Vibe). Independent reviewer and acceptor: OpenAI (ChatGPT).
- Pilot: Denis Bouzon. Merge and closure are reserved to the pilot.
- Base: `main` f51bc8a5… Branch: `mission/coord-rail-01`.
- Budget: €0. No paid API call, no real qualification of the four providers is assumed.

## 1. Inventory and architecture decision (T1)

Inventory of the A2A rail from mission #48: the rail lived inside `coord/go/internal/a2a` (rail.go, factories.go, adaptateur.go), with the external SDK a2a-go v2.9.0 as a direct dependency of the orchestrator.

Pilot decision (recorded, arbitration comment 7175791250): **Option B, standalone module**. The rail is extracted into a separate, standalone Go module `coord/a2a` (go.mod `github.com/dbouzon/osnias-clearing/coord/a2a`), whose only direct dependency is a2a-go v2.9.0. The orchestrator `coord/go` depends on the SDK only indirectly, through a local `replace` to the module. The bridge `coord/go/internal/a2a` is reduced to one adapter plus its test battery, with no SDK import outside that adapter.

Declared limit: the isolation achieved is a source and CI perimeter isolation (module boundary, dedicated go.mod/go.sum, dedicated CI workflow), **not** a binary isolation: orchestrator binaries still embed the same rail code through usual static linking in Go.

## 2. Corrections and evolutions (T2)

- Byte-exact migration of rail.go and factories into `coord/a2a/rail.go` and `factices.go`, plus purely additive public access: `Chemin()`, `EtatDepuisA2A()`, `JetonsFactices()`, alias `Executable = a2asrv.AgentExecutor`, decorator `CompteurEnvois`.
- secretRE check extended: `(sk-[A-Za-z0-9_\-]{8,}|AIza[0-9A-Za-z_\-]{35})`. FAIL reproduced before the fix, PASS after (reproduction-before-correction honoured).
- NouvelleMissionExplicit made additive and fail-closed: unknown adapter refused with `ADAPTATEUR_INCONNU`; historical behaviour unchanged otherwise.
- Explicit roles: tests `roles_test.go` (R1..R3). R3 locks the historical behaviour: with three adapters or more, the receiver was qualified by the second sorted adapter (default `ids[:2]`), and R3 pins that documented behaviour.
- Dedicated CI `.github/workflows/coord-a2a-ci.yml`: Linux/macOS/Windows, with `-race`.

## 3. Interoperability matrix (T4) — SIMULATED

`coord/go/internal/connecteurs/interop_test.go`, `TestMatriceInteropDouzeCouples`. Four simulated providers on loopback: bench-anthropic (Messages API, end_turn), bench-gemini (gemini profile, stop-only), bench-mistral, bench-openai (openai format, finish_reason "stop"). All 12 ordered pairs pass a complete cycle: NouvelleMissionExplicit → Executer → Soumettre → Recevoir → anchored ACCEPTEE qualification → Acker → ACK_RECU, closure refused after ack. PASS (0.41 s).

Qualification note: the stop-only gemini profile requires finish_reason "stop" in the openai-format benches, otherwise the simulated exchange is refused. Trap encountered and fixed during T4.

## 4. Resilience (T4) — REAL (sandbox)

- TI-06b (`coord/a2a/ti06b_test.go`): real process interruption. The child test binary re-executes itself (COORD_A2A_PROBE=1), delegates, marks its stdout "PROBE_DELEGE", then receives SIGKILL. On registry reopening: uncertain task, duplicate refused by `ErrDoublon` without any network, n==1 sends, `ResoudrePilote` succeeds. PASS.
- TI-07: already covered on the engine side by `TestReponseTardiveArretPendantAppel` (orchestre_test.go:314) — response produced after stopping the context during the call. Cited, not duplicated.

## 5. Replay evidence — REAL (sandbox)

- Module `coord/a2a`: 8 tests ok (including M1..M6 and TI-06b).
- Engine `coord/go`: 13 packages ok, build `./...` 2.66 s.
- Python v1.5 batteries: 47/47 (M1 PASS).
- Interop matrix 12 pairs: PASS.
- Fidelity: every pushed state is verified by a codeload tarball pinned to the SHA (sha256 fingerprint) then batteries replayed from the distributed code.

## 6. Measurements (T4) — REAL (sandbox), NOT_EXECUTED on VPS

Throwaway harness (measurement module outside the deliverable, loopback, €0), 50 complete delegation-exchanges against a simulated agent:

| Measurement | Value |
| --- | --- |
| Median latency | 3.02 ms |
| p95 latency | 3.82 ms |
| Min / max latency | 2.30 ms / 5.03 ms |
| Client RSS | ~13.2 MiB |
| Registry size after 50 exchanges | 10,771 bytes (sha256 e75c2d8f…) |

Binary sizes: coord-demo 4,832,097 B; coord-mission 11,202,474 B; coord-pilot 12,867,974 B; coord-mcp 14,155,352 B.

Status: measurements REAL but obtained in the sandbox; any measurement on a production VPS is NOT_EXECUTED and must be qualified before use.

## 7. Statuses and boundaries

- REAL: Go and Python battery replays, TI-06b (real SIGKILL), sandbox measurements, tarball fingerprints.
- SIMULATED: 12-pair matrix (simulated providers, loopback), E2E demonstrations.
- NOT_EXECUTED: real qualification of the four providers, VPS measurements. No paid call.

## 8. Delivery

Branch `mission/coord-rail-01`, PR to `main` with SHA fingerprints and full history (T1 ACK, baseline, PLAN_ACCEPTE, arbitrations, T3 CONTINUE, T4/T5). Merging, closing #50 and any arbitration remain reserved to the pilot Denis Bouzon.

---

Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX: Apache-2.0

# Mission report M-COORD-HARDENING-01 — Transversal hardening and observability

- Mission: issue #52, order v1.0 of 25/11/2026. Executor: Mistral. Independent reviewer and acceptor: OpenAI. Pilot: Denis Bouzon.
- Base: main `ea510ac9e24b12a853241c530a35a19ff40fed18` (tarball sha256 `b87358cf275cedd175ee468c899e94320a901fe94f12f2bf370d7ac36fe1affc`). Branch: `mission/coord-hardening-01-v2` (the homonymous `mission/coord-hardening-01` branch is an already-merged leftover of the old mission #19, head `c2db74d5`, not deletable with the available tooling; recreation under the -v2 name is documented).
- Budget: €0. No paid API call, loopback only. Module `coord/a2a` UNTOUCHED (zero bytes changed).

## Matrix of the five Gemini recommendations (verified, never presumed)

| # | Gemini finding | Verification | Treatment |
| --- | --- | --- | --- |
| 1 | Bytes limit vs token estimate; 4 B/token not a guarantee | REAL DEFECT: the emission-bound divisor 4 under-counts multi-byte UTF-8 tokens | FIXED: divisor 2 (local OVER-estimate), reproduction before fix, comments reworded |
| 2 | Secret expurgation (Bearer, traces, fingerprints preserved) | ALREADY COVERED (literal key, sk-/AIza, transport/read/excerpt, SORTIE_COMPROMISE) | PROOFS ONLY (fingerprints intact, partial key documented) |
| 3 | Missing vs invalid/zero usage | COVERED (JETONS_INCONNUS fail-closed refusal) | REFINED DIAGNOSTIC (distinct message), public code unchanged |
| 4 | Idempotence, TX1/TX2, reserves | ALREADY COVERED (appel_en_cours.json before network, durable reservation, surcharges only, SEC-01) | PROOFS (marker persists after transport failure) |
| 5 | coord-snapshot.json, EcrireAtomique, HTML console, TC-COORD-* | ARTEFACTS ABSENT (exhaustive search); coord/pilot/ui console exists without consuming any snapshot | CONTRACT PROPOSED, AMBIGUOUS PILOT ARBITRATION (answer "Coord V1,5"): implemented as a purely additive read-only, reversible projection |

## Corrections and proofs (T2, REAL sandbox)

- **P1** (`commun.go`): emission bound `len(corps) > jetonsEntreeMax * 2`; conservative local OVER-estimate, NOT a provider guarantee, defence in depth, fail closed. Reproduction (`hardening_test.go`): a 1500-byte CJK payload under a 500-token bound WAS emitted on main (observed FAIL); refused with CHARGE_HORS_BORNE, zero bytes emitted after the fix (PASS). Non-regression: legitimate payload admitted.
- **P3** (`connecteurs.go`): usage PRESENT but zero/invalid → `JETONS_INCONNUS: usage present but null/invalid (fail closed)`; usage ABSENT → common-control message. Public code `JETONS_INCONNUS` unchanged, no secret in diagnostics. Test: 3 families refused with the right diagnostic.
- **P2** (proof): sha256 (hex 64) and Git SHA (hex 40) cross `expurger` UNCHANGED; `sk-…`/`AIza…` replaced; partial/truncated or too-short (<8) key not replaced (non-authenticating, documented).
- **P4** (proof): after a transport failure, `appel_en_cours.json` persists (marker written BEFORE the network, temp + rename).

## Observability (T4)

`coord-snapshot.json` (read-only projection, `schema_version "1.0"`):
- regenerated at every durable point (committer TX2, ACK, in-flight call resolution);
- fields: persisted machine state, sha256 fingerprint of the journal, entry count, unresolved in-flight call (action), generation instant;
- NEVER a second source of truth: no normative code reads it; a write failure is journalised (`SNAPSHOT_ERREUR`) and never fails the durable transition (tested);
- atomic temp + rename 0600, no leftover temporary (tested);
- no text (payload/verdict) is ever copied: no secret can reach it (tested with a trapped sk-/AIza/mandate);
- explicit conservation note "NOT a replay journal; may be stale" (tested).

## Qualification (T5, REAL sandbox / SIMULATED providers / NOT_EXECUTED VPS and real APIs)

- Engine `coord/go`: vet + build 2.87 s + 13 packages green; windows/darwin cross-build OK. coord-mission binary 11,226,595 B (+24,121 B for the projection).
- Module `coord/a2a`: vet + tests green (untouched, replayed).
- Hardening tests: 9 green (P1/P3 reproductions, P2/P4 proofs, 3 snapshot tests; the 12-pair matrix included in the package).
- Python v1.5: 47/47 PASS (M1 PASS); vectors 22/22 Go and 22/22 Python byte-identical.
- `-race`: NOT_EXECUTED locally (no gcc in the sandbox), covered by the CI Race detector job.
- VPS and real providers: NOT_EXECUTED (€0, mandate honoured).

## Process

- T1: ACK 7176547292; inventory + matrix + plan v1 (sha256 `62f1756d…`); OpenAI review **PLAN_ACCEPTE round 1** (recorded 7176625804).
- T2/T3: corrections + proofs; OpenAI intermediate check **CONFORME** (recorded 7176696034).
- T4: projection delivered (ambiguous pilot arbitration, strictly additive reversible implementation).
- T6: PR at the exact SHA (sha256 fingerprint of every file in the PR), fidelity verified by codeload tarball pinned to the SHA then byte-exact diff and replay from the distributed code, FR/EN reports, independent OpenAI reception.

## Boundaries

Merging PR and closing #52 are reserved to the pilot. #42 suspended. v1.5 statuses unchanged (v1.5 = operational reference). No real provider qualification is presumed by this mission.

---

Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX: Apache-2.0

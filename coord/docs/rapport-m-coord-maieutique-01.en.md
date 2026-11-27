# Report M-COORD-MAIEUTIQUE-01 — Maieutic module: from ambiguous intent to a transmitted mission order

Mission M-COORD-MAIEUTIQUE-01 (issue #42, dbouzon/osnias-clearing, order v1.4). The maieutic module takes COORD from a pilot-declared intent to a qualified, validated, published and transmitted mission order, without any model output ever counting as a decision. Qualified director (OpenAI or an interchangeable profile), distinct executor and receiver, pilot Denis Bouzon as the only holder of decisive transitions (validation, closure). Plan v2 PLAN_ACCEPTE (fingerprint `e54eb7a1…`), mandate sha256 `1cab85cd…`. Budget 0 EUR, loopback fake servers only, no keys, no public listener. Base: main `f6852813`. Branch: `mission/coord-maieutique-01` (head `649cd86d`).

## 1. What was delivered

| Deliverable | Content |
|---|---|
| `coord/go/internal/maieutique/` | state machine INTENTION→DIALOGUE→ORDRE_PROPOSE→ORDRE_QUALIFIE→VALIDE→PUBLIE→TRANSMIS (+ ATTENTE_PILOTE/INVALIDE/ABANDONNE), ten-field order, deterministic validation, sources, framed documents, two-party dialogue, D10 journal |
| `coord/go/cmd/coord-maieutique/` | loopback REST service (pilot token, ~26 routes), daughter-mission test harness, witness acceptance kit (4 Python scripts), T5 bench |
| `coord/schemas/ordre.schema.json` | JSON Schema 2020-12 of the order (ten required fields) |
| `.github/workflows/coord-maieutique-ci.yml` | CI: gofmt/vet/build/test on ubuntu/windows/macos, race detector, engine non-regression (new files only) |
| `coord/go/cmd/coord-maieutique/banc/` | T5 bench: scenarios S1 to S16 + interchangeability, RAM/CPU/latency/call measurements |

## 2. Demonstrated contract (D2-D10)

- Ambiguous intent → QUESTION/CLARIFICATION, never an order; an order leaves the dialogue only after deterministic validation THEN a mandatory semantic critique by the director.
- Deterministic rejections at a single call (OBJECTIF_NON_UNIQUE, SOURCES_VIDES, LIVRABLE_IMPRECIS, ETAPES_INCOHERENTES): no second call, no cost.
- BLOQUANT critique → back to dialogue (422 CRITIQUE_BLOQUANTE); director failures ×2 → ATTENTE_PILOTE, only licit exits: relancer/rectifier/abandonner.
- Explicit pilot validation with a token (never journaled); qualification freezes the version and the JCS sha256 fingerprint.
- Strict idempotence: publication and transmission = exactly one effect, then 409 OPERATION_DEJA_EFFECTUEE; recovery after a real SIGKILL with no double publication, no double transmission, no second call.
- The three fingerprints (validated, published, transmitted) equal the frozen file and the deliverable (sha256 verified).
- Quote derived from the order (never typed by hand), agreement journaled `paiement_reel:false`; closure ALWAYS refused to automation (mother and daughter missions end at ACK_RECU).

## 3. Verifications

- Go tests: full module green (16 packages), gofmt/vet clean, `GOOS=windows/darwin` vet OK, race detector green in CI.
- GitHub CI: 5/5 jobs green at every state (3 OS, race, engine non-regression from `f6852813`).
- T3 OpenAI (bridge session `m-coord-maieutique-01`): favourable verdict after re-tracing the rejections.
- T5 bench 17/17 PASS in the sandbox AND on the VPS (S1-S16 + director-profile interchangeability, criterion 5); VPS measurements: 2,481 ms total, max RSS 14 MB, 28 cumulative director calls.
- Witness acceptance on the VPS (`coord` account, loopback only, kit pinned at `649cd86d`):
  - run A (openai profile, 3 real SIGKILLs: dialogue, publication, transmission): **38/38 proofs**, clean recovery at each phase;
  - run B (anthropic profile, no SIGKILL, distinct ports): **34/34 proofs**;
  - identical frozen-order fingerprint `6b3ddb905f60268e53310e693bd51995c37f7faba163b3d343006645a83f3469` across both runs and the sandbox: provider interchangeability demonstrated;
  - measurements: service VmHWM 7.8-9.3 MB, CPU ≤ 0.04 s, REST latencies ≤ 60 ms — the 1 vCPU / 2 GB VPS target met with two orders of magnitude of headroom.

## 4. Decisions, incidents and limits

- No existing engine file modified (fail-closed D1, verified by diff `f6852813…HEAD`: new scopes only).
- Incident: a sandbox reset destroyed the working tree before any push; T2 was fully rebuilt, then a transcription drift of the acceptance driver was detected and fixed (commits `8e17be54`, `649cd86d`, pushed file verified byte-exact against the proven version).
- Production bug avoided: transmission roles expressed as normative actions (EXECUTANT{executer,soumettre,acker}, RECEPTIONNAIRE{recevoir}, PILOTE{cloturer}) instead of identifiers (commit `5cb4951f` + regression test).
- Mission closure reserved for the human pilot in all circumstances; the witness daughter mission MAIEUTIQUE-TEMOIN-01 stays at ACK_RECU.
- SIMULATED mode only: no keys, no payment, no outbound network call from the VPS.

## 5. Traceability

Issue #42 (order v1.4). Plan v1 A_CORRIGER; plan v2 PLAN_ACCEPTE, fingerprint `e54eb7a1…`. Commits (18): `1d87302c` module, `8a107cf6` documents+dialogue, `c77fe2d5` actions+operations, `67f5b322` fake director+core tests, `c3be9150` documents/dialogue/operations tests, `7ece8a3d` service, `92b544ce` service tests, `6cc1d3a7` schema+CI, `fdc8dc27`/`71455bf8` Windows CI, `c0b5d67d` T4 lock, `63932fc1` T5 bench, `5cb4951f` transmission roles, `2111de7a` daughter harness, `a12feb5e`/`a851944d` acceptance kit, `8e17be54`/`649cd86d` acceptance-driver alignment. OpenAI bridge sessions `m-coord-maieutique-01` (T3 favourable; final post-PR reception journaled on issue #42). VPS acceptance: `/srv/coord/private/recette/m-coord-maieutique-01/` (reports `run-a`, `run-b`), documented cleanup after acceptance.

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

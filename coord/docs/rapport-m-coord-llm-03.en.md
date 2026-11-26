# Mission report — M-COORD-LLM-03: A2A agent server and Mistral API/Vibe connector

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · License Apache-2.0

Mission: issue [#54](https://github.com/dbouzon/osnias-clearing/issues/54)
(COORD v1.5). French report (normative): `rapport-m-coord-llm-03.md`.

## 1. Summary

A reusable **A2A agent server foundation** (no duplication of the existing
rail) and a **two-way Mistral connector**: VOIE API (`mistral` stop-only
profile, strictly additive to the LLM-02 gemini model, shared `baseHTTP.appel`
safety loop) and VOIE VIBE (supervised Vibe CLI subprocess: environment
allow-list, secret scrubbing, exit codes 0/1/2/3, fail-closed `export.json`).
Qualification **SIMULATED, 0 EUR**: real qualification (VPS, real binaries,
provider network, B1..B5) remains **NOT EXECUTED**, pending separate pilot
authorization.

## 2. Deliverables (branch `mission/coord-llm-03`)

New standalone Go 1.27 module `coord/a2a-agent`: `socle/` (rail wrapper,
loopback + token + card reused, `ErrJetonVide`), `runner/` (subprocess
supervisor, strict env allow-list, `sk-`/`AIza`/`gh?_`/`xoxb-` scrubbing,
SIGTERM→SIGKILL process group, fail-closed export, posix/windows groups),
`mistral/` (A2A agent, `VOIE:API`/`VOIE:VIBE` routing, anchored verdicts,
0600 atomic session registry, `--resume` same-session S16), CLI `vibe-factice`
(simulated, exit codes, leaks, `signal.Ignore` without `stop()`), CLI
`coord-agent -demo-local` (full simulated cycle, `DEMO_OK`), module
`README.md`.

Engine side (closed list): `coord/go/internal/connecteurs/mistral.go`
(stop-only profile: only `finish_reason:"stop"` accepted; truncation,
`tool_calls`, missing/unknown = refused with exact reason, never repaired),
`mistral_test.go` (C1..C9 on loopback fake servers), additive changes to
`connecteurs.go` (load-time validation + dispatch). Docs & tooling:
`coord/connecteur/Mistral/exemple-config-mistral.json` (no secret),
Mistral section in `coord/pilot/connecteurs/README.md`, CI
`coord-agent-ci.yml` (ubuntu/windows/macos matrix + race).

## 3. Noted deviation from plan v2

The adapter captures the artifact **at send time** (`message/send`) and
**refuses any asynchronous completion** (`RESULTAT_NON_TRANSMIS`); the 20 s
bound is a post-send follow-up limit. Work therefore executes **within the
`message/send` call** (client blocked). Plan v2 said "immediate reply +
async follow-up"; async would have required an **A2A contract extension**
to be validated by both AIs. Implemented synchronously in-send, with no
contract extension. OpenAI intermediate review T3: deviation **ACCEPTED**.

## 4. Security

No secrets in the repo (env vars only); loopback-only socket, mandatory
token; strict subprocess environment allow-list (full inheritance
impossible); secret-pattern scrubbing before durable writes, secret in
export ⇒ fail-closed verdict; exit codes 0/1/2/3 enforced even under
`signal.Ignore`; atomic 0600 session registry.

## 5. Simulated acceptance

`coord/a2a-agent`: vet clean, full test suite GREEN; `coord/go`: vet+tests
OK; `coord/a2a`: unchanged OK; v1.5 conformance vectors **22/22** (sha
`8c097f76…156` unchanged); hardened Python v1.5 suite **47/47 PASS**;
demo `-demo-local`: **DEMO_OK**.

## 6. Measurements (Linux sandbox, Go 1.27.4, simulated)

`coord-agent` binary 12,370,335 bytes (~11.8 MB); `vibe-factice` binary
4,413,994 bytes (~4.2 MB); module test suite ~3.8 s; full demo cycle
~0.53 s. Performance follow-up of the synchronous approach (latency,
server load): recorded as a T3 watch item, to be reassessed at real
qualification.

## 7. Real qualification: NOT EXECUTED

No real-infrastructure qualification (VPS, networked binaries, B1..B5) was
executed, per COORD v1.5 and mission order #54. It requires a separate
pilot authorization. Merge and closure are reserved to the pilot.

## 8. Reviews

Plan v1 → A_CORRIGER (3 blockers); plan v2 → **PLAN_ACCEPTE**. Intermediate
review T3 (bridge session `m-coord-llm-03`): deviation accepted, foundation
conforming, no A_CORRIGER (logged: comment 7177918226 in #54). Final
independent OpenAI acceptance: anchored to the head SHA, logged in the PR
and #54.

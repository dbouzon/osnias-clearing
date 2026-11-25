# M-COORD-EXP-01 Report — A2A-over-MCP experiment

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

Issue #38 — Pilot order of 25/11/2026. Executor: Mistral. Plan critic and independent receiver: OpenAI. GO/NO-GO decision: reserved to the pilot. This English report summarizes the French reference report (`RAPPORT.md`); in case of divergence the French version prevails.

**Hypothesis tested**: an OSI-inspired stack COORD (governance) → A2A (inter-agent semantics) → MCP (application envelope) → HTTP/STDIO, where "A2A-over-MCP" means A2A messages encapsulated in MCP calls/results. No gain was presupposed; a negative conclusion is admissible.

## 1. Scope and guarantees

Isolated POC (`coord/exp`, own go.mod; SDKs `a2a-go/v2` v2.9.0 and MCP `go-sdk` v1.8.0, both Apache-2.0). The `coordengine` engine, Pilot and MCP server #37 are used **unchanged**. Two simulated agents, loopback only, offline, 0 EUR, no key, no provider call, no COORD credential ever handed to an agent, no mutation channel exposed.

## 2. Method

Each scenario runs in both modes with identical inputs; outcomes are compared by semantic sha256 (final state + artifact) and byte/exchange measurements. Verdicts per pair: "conservée" (identical), "conservée (service dégradé…)" or "PERDUE". Pre-declared GO/NO-GO criteria: (a) semantics preserved everywhere; (b) no governance/security regression; (c) measurable, acceptable overhead; (d) an identified functional gain over the direct API.

## 3. Results (real loopback executions)

| Scenario | Object | Bytes A | Calls A | Bytes B | Calls B | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| S1 | AgentCard discovery | 0 | 0 | 840 | 1 | preserved |
| S2 | minimal round-trip (PING) | 665 | 1 | 1369 | 2 | preserved |
| S3 | multi-step task | 680 | 1 | 1399 | 2 | preserved |
| S4 | large artifact (1 MiB) | 1 049 253 | 1 | 2 098 545 | 2 | preserved |
| S6 | error EF | 716 | 1 | 1475 | 2 | preserved |
| S7 | verbatim copy | 804 | 1 | 1647 | 2 | preserved |
| S8 | critic verdict | 927 | 1 | 1923 | 2 | preserved |
| S3b | streaming | 2832 | 1 | 3966 | 1 | **preserved (degraded: non-incremental)** |
| S5 | cancellation | 1575 | 3 | 3412 | 6 | preserved |
| S9 | prompt injection | 1002 | 1 | 2047 | 2 | preserved (fails on both sides as expected) |
| S10 | COORD governance | 665 | 1 | 1369 | 2 | preserved |

Key facts: semantics fully preserved (no PERDUE anywhere); mode B overhead ≈ **×2 bytes** (payload carried in the MCP envelope and again in the internal A2A call) plus one extra exchange per call; streaming is degraded in mode B (events rendered after the call, single result); S1 has no direct equivalent (card relayed by a dedicated gateway tool). Traces: `preuves/` (sha256 recorded in `preuves/README.md`); assertions automated (`TestConduiteS1S9`, `TestS10Gouvernance`).

## 4. S10 — Governance (real Pilot + read-only MCP)

Real `coord-pilot` started on loopback; mission `M-COORD-EXP-01-S10` created through the authenticated API; MCP server #37 reached over STDIO with the **read-only token only**: exactly 2 tools (`coord_list_missions`, `coord_get_mission`), no mutation tool; the mission is readable, content derived from COORD, **no COORD credential present in agent-visible data**. Negative anti-mandate tests, all conforming: creation without token → 401; creation with the read token on a mutation route → 403 `JETON_LECTURE_REFUSE`; self-granted closure right (`cloturer` hoisted into EXECUTANT) → 409 `CREATION_REFUSEE`; creation via a non-existent MCP tool → refused. Agents operate in both modes without any COORD credential: governance stays with the Pilot star; the MCP envelope grants no privilege.

## 5. Limits

Simulated agents and gateway; loopback; no real provider (real Gemini/Claude qualifications are separate missions requiring authorization); race detector and multi-OS covered by CI `coord-exp-ci.yml` (not run locally — no gcc in the dev sandbox); latency not used as a criterion; mode-B streaming is an implementation choice of the gateway (single result), not an MCP standard limit, but it reflects the cost of the single-result path.

## 6. GO/NO-GO (executor's recommendation)

(a) semantics preserved everywhere (streaming degraded, documented); (b) no governance/security regression; (c) real overhead — ×2 bytes, one extra exchange, plus a gateway to write and maintain; (d) **no functional gain identified** for the current goal — provider qualification (Gemini, Claude) already rides the direct API behind `adaptateurs.Adaptateur` (path qualified in #29/#30).

**Recommendation: NO-GO for a short-term real A2A-over-MCP qualification with Gemini.** The direct path is simpler, cheaper and already qualified; the experiment nonetheless establishes that encapsulation is semantically viable and governance-safe, and remains a documented option (phase 4 of Claude's proposal / analysis #27, A2A facade in a separate module) should a real need emerge — third-party agents, multi-standard ecosystems, standardized transport. A negative conclusion is admissible per the order. **Final decision reserved to the pilot.**

## 7. Reproducibility

`go vet ./... && go build ./... && go test ./... -timeout 300s` from `coord/exp` (~10 s, 0 EUR, no key). Outputs: `go run ./cmd/coord-exp`, `go run ./cmd/coord-exp -s10`, `-json`. CI: `coord-exp-ci.yml` (ubuntu/windows/macos + race).

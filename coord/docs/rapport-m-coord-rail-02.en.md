# M-COORD-RAIL-02 Report — Selecting a shared LLM access library

Order v1.0 of 26 Nov 2026 (rail mutualization study), refocused by order v1.1 (library selection), pilot decision of 26 Nov 2026 (comment 7195050454): HTTP/JSON as the target internal transport. Protocol: COORD v1.5 / Specification 1.0. Executor Mistral, director/receiver OpenAI, pilot Denis Bouzon. Base: main 5e1d172eb761b6749737c8067a1b1fb04edb5baf. 0 EUR spent; no paid calls; no keys; coord/go UNCHANGED. The French report is authoritative; this is its English rendition.

## 1. Target architecture (pilot decision)

Go + SQLite = COORD engine (unchanged) · HTTP/JSON = standard internal transport · LiteLLM Core (local, no proxy) = inference API mutualization (subject to qualification) · official SDKs/runners = per-environment agent execution · A2A = remains operational during the study; keeping it as an external interface is an option, not an authorized migration.

## 2. Method

Sourced statuses (NATIVE/MUTUALIZABLE/SPECIFIC/UNAVAILABLE/UNVERIFIED), objective criteria C1..C6 (SPDX licenses read from repository LICENSE files), strict inference/agent-execution separation, sourced exclusion registry, evidence statuses REAL-documentary / REAL-sandbox / SIMULATED / NOT_EXECUTED. Full v1.5 loop: plan v1.1 PLAN_APPROVED (round 1), E3 check ACCEPTED (round 1), pilot decision folded into E5.

## 3. Results

T1 (kept): green REAL baseline at 5e1d172e (Python 24+10+13+11+13+17+47 PASS, Go 13 packages + modules green, 22/22 vectors, 19 CONFORME/0 DEVIATION demo). Inventory: mutualized (rail, orchestrator, monetary vault, templates, runner); specific (520 LoC of provider profiles: anthropic 208, gemini 182, mistral 130, openai 0); redundancies documented.

Selection (E2, ACCEPTED): LiteLLM SDK via litellm-core v1.111.0 (25 Nov 2026), 24 direct runtime dependencies (all permissive), MIT, signed PyPI provenance; 7/8 API environments. The proxy is EXCLUDED (sourced exclusion registry: network service contrary to fail-closed; CVE-2026-35029/59823 and OIDC bypass reported by Check Point/OpenCVE on the proxy component, fixed). Go alternatives documented as fallback (any-llm-go Apache-2.0, 8 months without release; sashabaranov/go-openai Apache-2.0, OpenAI-format only). Agents: NO mutualized library exists; official per-environment surfaces: Claude Agent SDK (MIT), first-party cursor-sdk Python + SDK Bridge (MIT, sdk.v1 protocol over loopback HTTP/1.1), codex exec, COORD Vibe runner, Antigravity (UNVERIFIED).

Eight-environment matrix (issue #60, comment 7194584835): inference NATIVE/MUTUALIZABLE everywhere except Cursor (no standalone inference API documented, UNVERIFIED); agents fragmented (Claude Code and Cursor CLI complete and documented; Codex exec complete; Vibe supervised by the COORD runner; Gemini CLI unavailable to individual accounts since 18 June 2026, successor Antigravity UNVERIFIED; DeepSeek/Llama UNAVAILABLE for agents; xAI UNVERIFIED).

## 4. HTTP/JSON standard hookup (design, not developed)

Local bridge `coord-litellm-bridge`: HTTP/1.1 server bound to 127.0.0.1 only, versioned contract coord.llm.v1 (GET /ping, POST /v1/chat/completions), OpenAI-compatible response format -> NO engine change (existing HTTPEndPoint baseURL config; stop-only profile kept). Documented properties (required by the pilot decision): loopback authentication (local 0600 token file on both sides, fail-closed; secrets via launch-time environment only); timeouts (existing Go-side request bounds; the bridge implements NO retry, nor does the underlying SDK — no second resumption authority, per precedent D2 #40); errors and non-masking (exact finish_reason, provider error message, actually-served model identity and detailed per-category token usage propagated; normalization hides nothing needed by COORD controls: authenticated identity != declared model kept, stop-only checked on the exact reason, truncation refused); sessions/resume (stateless bridge restarted by the existing Go subprocess supervisor; in-flight calls handled by the orchestrator, LATE_RESPONSE, uncertain recovery pilot-only); idempotence (pre-call reservation and in-flight dedup UNCHANGED, Go side; the bridge is naturally idempotent); usage and fail-closed budget on the Go side (the monetary vault remains the sole budget authority; tariffs stay in Go config, never in the bridge; litellm-core is a TRANSPORT, not an authority).

## 5. Gaps and explicit limits

Real compatibility of litellm-core with each provider: NOT_EXECUTED (documentary only); real qualification is a separate recipe, triple lock, written pilot authorization (precedents #29/#47/#49). Demonstrator: NOT_EXECUTED in the working environment (package installation forbidden by the infrastructure); to be run on an authorized machine if development is decided. Antigravity (Gemini agents), xAI agents, Cursor inference: UNVERIFIED. Resources: Python runtime + bridge estimated ~100-200 MB RSS (ESTIMATED, NOT_EXECUTED) versus 13.2 MiB for the current Go rail; fits a 2 GB VPS but must be measured before commitment. The "one or two mutualized pivots" goal is met for inference (LiteLLM Core = 1), NOT for agents (per-environment official surfaces) — a market limit, documented.

## 6. Recommendations

Keep the Go status quo (520 LoC of profiles) as the operational reference until the bridge is fully qualified. If the pilot approves development: dedicated branch, isolated coord/llmbridge package, litellm-core pinned to an exact version, loopback mock tests (pattern #49), simulated then real qualification under triple lock. Treat contract coord.llm.v1 as the internal HTTP/JSON standard for future hookups (Cursor SDK Bridge as the architectural model). A2A: unchanged during qualification; keeping it as an external interface is a pilot decision after field experience.

## 7. Traceability

Issue #60: ACK 7193860597; T1 7194141872; plan v4 PLAN_APPROVED 7194149511; T2/T3 7194584835; T4 CONTINUE 7194603014; order v1.1: ACK+plan 7194787709 (PLAN_APPROVED, recorded 7194831290); E2 selection 7194812143, E3 ACCEPTED recorded 7194832815; E4 7194834314; pilot decision 7195050454. Documentary sources S1..S31 cited in comments T2/T3 and E2.

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

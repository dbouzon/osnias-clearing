# COORD LLM Bridge — local HTTP/JSON bridge (`coord.llm.v1` contract)

Mission **M-COORD-LLMBRIDGE-01** (issue #62) — COORD v1.5 / Specification 1.0.
Copyright © 2026 Denis Bouzon — ORCID: <https://orcid.org/0009-0007-8894-8902> — SPDX Apache-2.0. **The French version prevails.**

Local HTTP/JSON bridge that pools multi-provider inference through **LiteLLM
Core** (Python SDK, MIT, `litellm-core` distribution, no proxy) and plugs
into the existing Go engine **via a plain baseURL**, with no engine rewrite.
The Go engine (transitions, monetary vault, deduplication, tariffs, budget,
cancellation, resume) remains the **sole authority**: the bridge is a
**stateless** transport, restartable without data loss. A2A remains
operational and untouched; this bridge is an additive option.

Contract `coord.llm.v1` (versioned, JSON Schema 2020-12): `GET /ping`,
`GET /v1/models`, `POST /v1/chat/completions` (OpenAI-compatible strict
subset; `usage` and `finish_reason` are passed through as-is, never
invented). Closed list of allowed destinations (prefix routing), mandatory
Bearer token (constant-time comparison), bounded body and timeouts,
structured JSON errors, no retry, no fallback, no streaming in v1, secrets
only via environment variables and scrubbed from every log and error.

Reproducible install (outside the execution sandbox):
`pip install "litellm-core==1.111.0"`, then
`python3 coord/llmbridge/bridge.py --config coord/llmbridge/config.exemple.json`.

Qualification statuses are traced everywhere (REEL / CI / NON_EXECUTE):
local contract tests (21, fake backend), Go E2E test (full mission cycle
through the bridge, closure refused, bridge restart), CI-only real-SDK tests
against four native fake providers (OpenAI, Mistral, Anthropic, Gemini —
package installation is forbidden in the executor sandbox), real providers
and the 72-hour 2-GB VPS acceptance run both explicitly NON_EXÉCUTÉ in this
mission (distinct qualifications under pilot authorization). No exactly-once
claim: the Go-side deduplication applies before any network call.

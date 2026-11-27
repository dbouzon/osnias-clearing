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

## Marker-driven fake scenarios (M-COORD-DEP-03)

The `FacticeBackend` supports scenarios driven by markers carried by the
prompt (prompts are composed by the Go engine; markers never affect
routing, authentication or bounds).

| Role | Marker in prompt | Output |
|---|---|---|
| `factice/exec/…` | `MANDAT V1` | flawed V1 deliverable, contains `SCENARIO:LIVRABLE_A_CORRIGER` |
| `factice/exec/…` | `MANDAT V2` | corrected V2 deliverable (no scenario marker) |
| `factice/exec/…` | (none) | historical constant deliverable (frozen by test) |
| `factice/recep/…` | prompt contains `SCENARIO:LIVRABLE_A_CORRIGER` | `A_CORRIGER` verdict + 2 explicit points, anchored digest |
| `factice/recep/…` | `CRITIQUE DE PLAN` (no scenario) | `PLAN_ACCEPTE` (historical) |
| `factice/recep/…` | (none) | `ACCEPTEE` + `verdict_id` (historical) |
| both | `MANDAT V1` AND `MANDAT V2` together | explicit refusal `MARQUEURS_CONTRADICTOIRES` (400) |

Rules: exact, injective markers (none is a substring of another);
contradiction => refusal, never an arbitrary choice; without marker =>
unchanged historical behavior. Any future change: table + tests + this
section in the same PR.

### Implications of each marker

- `MANDAT V1`: the caller receives a deliberately flawed deliverable to
  exercise a full iteration cycle (delivery -> A_CORRIGER -> fix ->
  acceptance); no other output is affected.
- `MANDAT V2`: the caller receives the corrected deliverable, an output with
  no scenario marker; this is phase 2 of the same mission identifier.
- `SCENARIO:LIVRABLE_A_CORRIGER` (receiver): the receiver's response is an
  A_CORRIGER verdict with exactly 2 explicit points, anchored to the digest
  of the received deliverable, referenced by a `verdict_id` (the Go engine
  requires a reference for every receivable decision); any other prompt
  content is ignored.
- `MANDAT V1` + `MANDAT V2`: immediate 400 MARQUEURS_CONTRADICTOIRES
  refusal, no inference produced (defence against a contradictory prompt).
- No marker: strictly unchanged historical behavior (frozen by
  non-regression tests).

### Request/response examples (coord.llm.v1 protocol)

`MANDAT V1` (executor):

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V1 ..."}]}
    -> 200 {"choices": [{"message": {"content":
         "... SCENARIO:LIVRABLE_A_CORRIGER ..."}}]}

`MANDAT V2` (executor):

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V2 ..."}]}
    -> 200 {"choices": [{"message": {"content": "corrected V2 deliverable"}}]}

`SCENARIO:LIVRABLE_A_CORRIGER` (receiver):

    POST /v1/chat/completions
    {"model": "factice/recep/openai-dep03", "messages": [
      {"role": "user", "content": "... SCENARIO:LIVRABLE_A_CORRIGER ..."}]}
    -> 200 {"choices": [{"message": {"content":
         "verdict: A_CORRIGER; points: [2 explicit points];
          digest: <sha256 of the deliverable>"}}]}

`MANDAT V1` + `MANDAT V2` together (refusal):

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V1 ... MANDAT V2 ..."}]}
    -> 400 {"error": {"type": "MARQUEURS_CONTRADICTOIRES",
         "message": "..."}}

## Optional bridge journal

Environment variable `COORD_BRIDGE_JOURNAL` = path of a JSONL journal: one
line per inference {timestamp, model, destination, status, error_type,
duration_ms}. NEVER any prompt payload, NEVER any token (model value is
scrubbed); write failures are reported on stderr with no client impact.
Purpose: engine ⇄ bridge trace correlation (M-COORD-DEP-03).

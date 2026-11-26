# Report M-COORD-LLMBRIDGE-01 — coord.llm.v1 HTTP/JSON bridge

Issue #62 — order v1.0, 2026-11-26. COORD v1.5 / Specification 1.0.
Executor: Mistral. Director/independent receiver: OpenAI. Pilot: Denis Bouzon.
Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX Apache-2.0. **The French version prevails.**

Summary of the delivered work (full detail in the French report):

- **T1**: HEAD `ea37d4ea` recorded, baseline replayed (Python 135 PASS incl.
  M1, Go engine + a2a + a2a-agent green, 22/22 vectors sha `8c097f76…`
  byte-identical), plan v1 sha `fd71e8d6…` → PLAN_ACCEPTE round 1 by OpenAI.
  Key facts: loopback `HTTPEndPoint` accepts the bridge by plain baseURL
  (no engine change); litellm-core 1.111.0 (MIT, signed wheel, 24 direct
  deps, no proxy); package installation forbidden in the executor sandbox →
  the SDK is really installed and executed in CI only.
- **T2**: new isolated module `coord/llmbridge/`: stdlib loopback HTTP/1.1
  bridge (mandatory constant-time Bearer token, closed destination list,
  bounded body/timeouts, structured stable error types, stateless, no
  retry/fallback, secret scrubbing), injectable backends
  (`FacticeBackend`, `LiteLLMBackend` with deferred real import:
  `SDK_ABSENT`/`CLE_ABSENTE` fail-closed, `num_retries=0`, usage and
  finish_reason passed through as-is), versioned JSON Schema 2020-12
  contract (`coord.llm.v1`), example config with secrets forbidden,
  21 local contract tests, 4 native fake provider servers (OpenAI,
  Mistral, Anthropic, Gemini) with scenarios, 12 CI-only real-SDK tests
  (explicit NON_EXECUTE skips if the SDK is absent).
- **T3**: additive Go connection with **zero engine change**, demonstrated
  by a new E2E test: real bridge subprocess, baseURL-loopback adaptateurs,
  full mission cycle (PLAN_ACCEPTE → execution → submission → ACCEPTEE
  anchored verdict → ACK_RECU → CLOTURE_REFUSEE) then bridge restart with a
  new mission working immediately (statelessness).
- **T4**: traced qualification — REEL local (21/21 contract tests, E2E Go
  PASS, full non-regression, cross-OS vet), CI (3-OS matrix installing
  litellm-core==1.111.0 and running the real SDK against the four fake
  providers), NON_EXÉCUTÉ (real providers, DeepSeek/Meta/Grok, 72 h VPS run).
- **T5**: sandbox measures (factice backend, 50 calls): startup 127 ms,
  RSS 22.8 MB, median latency 0.81 ms / p95 0.91 ms, vs the A2A rail at
  3.02 ms / 13.2 MB under identical loopback conditions; 2-GB/72 h
  acceptance procedure prepared with pre-declared GO/NO-GO criteria,
  NON_EXÉCUTÉ without an authorized environment.
- **T6**: dedicated branch `mission/coord-llmbridge-01` from `ea37d4ea`,
  PR with exact SHA and per-file sha256 fingerprints, independent OpenAI
  reception anchored to the SHA.

Limits: no exactly-once claim (stateless transport; Go-side deduplication
applies before any network call; uncertain-call resolution remains a pilot
act), no streaming in v1, no provider compatibility claimed without a
test, 0 €, no real key, no provider network call outside loopback fakes.

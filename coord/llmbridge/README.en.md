# COORD LLM Bridge â local HTTP/JSON bridge (`coord.llm.v1` contract)

Mission **M-COORD-LLMBRIDGE-01** (issue #62) â COORD v1.5 / Specification 1.0.
Copyright Â© 2026 Denis Bouzon â ORCID: <https://orcid.org/0009-0007-8894-8902> â SPDX Apache-2.0. **The French version prevails.**

Local HTTP/JSON bridge that polls multi-provider inference through **LiteLLM
Core** (Python SDK, MIT, `litellm-core` distribution, no proxy) and plugs
into the existing Go engine **via a plain baseURL**, with no engine rewrite.
The Go engine (transitions, monetary vault, deduplication, tariffs, budget,
cancellation, resume) remains the **sole authority**: the bridge is
a**stateless** transport, restartable without data loss. A2A remains
operational and untouched; this bridge is an additive option.

Contract `coord.llm.v1` (versioned, JSON Schema 2020-12): `GET /ping`,
aGET /v1/models`, `POST /v1/chat/completions` (OpenAI-compatible strict
subset; `usage` and `finish_reason` are passed through as-is, never
invented). Closed list of allowed destinations (prefix routing), mandatory
Bearer token (constant-time comparison), bounded body and timeouts,
structured JSON errors, no retry, no fallback, no streaming in v1, secrets
only via environment variables and scrubbed from every log and error.

Reproducible install (outside the execution sandbox):
`pip install "litellm-core==1.111.0``, then `python3 coord/llmbridge/bridge.py --config coord/llmbridge/config.example.json`.

Qualification status: local contract tests (21, fake backend), Go E2E test (full mission
cycle through the bridge, closure refuse, bridge restart), CI-only real-SDK tests
against four native fake proviers (OpenAI, Mistal, Anthropic, Gemini â
package installation is forbidden in the executor sandbox), real providers and the 72-hour 2-GB VPS acceptance run
both explicitly NON_EXIST  in this mission (distinct qualifications under pilot
authorization). No exactly-once claim: the Go-side deduplication applies before any network call.

## Marker-driven fake scenarios (M-COORD-DEP-03)

The `FacticeBackend` supports scenarios driven by markers carried by the
prompt (prompts are composed by the Go engine; markers never affect
routing, authentication or bounds).

| Role | Marker in prompt | Output |
|---|---|---|
| `factice/exec/â¦` | `MANDAT V1` | flawed V1 deliverable, contains `SCENARIO:LIVRABLE_A_CORRIGER` |
| `factice/exec/â¦` | `MANDAT V2` | corrected V2 deliverable (no scenario marker) |
| `factice/exec/â¦` | (none) | historical constant deliverable (frozen by test) |
| `factice/recep/â¦` | prompt contains `SCENARIO:LIVRABLE_A_CORRIGER` | `A_CORRIGER` verdict + 2 explicit points, anchored digest |
| `factice/recep/â¦` | `CRITIQUE DE PLAN` (no scenario) | `PLAN_ACCEPTE` (historique) |
| `factice/recep/â¦` | (none) | `ACCEPTEE` + `verdict_id` (historical) |
| both | `MANDAT V1` AND `MANDAT V2` together | explicit refusal `MARQUEURS_CONTRADICTOIRES` (400) |

Rules: exact, injective markers (none is a substring of another);
contradiction => refusal, never an arbitrary choice; without marker =>
unchanged historical behavior. Any future change: table + tests + this
section in the same PR.

### Implications of each marker

- `MANDAT V1`: the caller receives a deliberately flawed deliverable to exercise
  a full iteration cyclle (delivery -> A_CORRIGER -> fix ->
  acceptance); no other output is affected.
- `MANDAT V2`: the caller receives the corrected deliverable, an output with
  no scenario marker; this is phase 2 of the same mission identifier.
- `SCENARIO:LIVRABLE_A_CORRIGER` (receiver): the receiver's response is an
  A_CORRIGER verdict with exactly 2 explicit points, anchored to the digest
  of the received deliverable; any other prompt content is ignored.
- `MANDAT V1` + `MANDAT V2`: immediate 400 `MARQUEURS_CONTRADICTOIRES`
  refusal, no inference produced (defence against a contradictory prompt).
- No marker: strictly unchanged historical behavior (frozen by non-regression tests).

### Request/response examples (coord.llm.v1 protocol)

ÀM9PXÅ¡áÕÑ½È¤è((A=MP½ØÄ½¡Ð½½µÁ±Ñ¥½¹Ì(ìµ½°èÑ¥½á½µ¥ÍÑÉ°µÀ°µÍÍÌèl(ìÉ½±èÕÍÈ°½¹Ñ¹Ðè59PXÄ¸¸¸õuô(´øÈÀÀì¡½¥ÌèmìµÍÍèì½¹Ñ¹Ðè(¸¸¸M9I%<é1%YI	1}}=II%H¸¸¸õõuô()59PXÉ¡áÕÑ½È¤è((A=MP½ØÄ½¡Ð½½µÁ±Ñ¥½¹Ì(ìµ½°èÑ¥½á½µ¥ÍÑÉ°µÀÀÌ°µÍÍÌèl(ìÉ½±èÕÍÈ°½¹Ñ¹Ðè59PXÈ¸¸¸õuô(´øÈÀÀì¡½¥ÌèmìµÍÍèì½¹Ñ¹Ðè½ÉÉÑXÈ[]\XH_W_B°ÐÑSTSÎUPWÐWÐÓÔQÑT
XÙZ]\NÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÜXÙ\ÛÜ[ZKY\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[ÐÑSTSÎUPWÐWÐÓÔQÑTW_BOÈÚÚXÙ\ÈÞÈY\ÜØYÙHÈÛÛ[\XÝWÐÓÔQÑTÈÚ[ÎÌ^XÚ]Ú[×NÂYÙ\ÝÚLMÙH[]\XO_W_BPSUX
ÈPSUÙÙ]\
Y\Ø[
NÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÙ^XËÛZ\Ý[Y\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[PSUHPSUW_BOÈ\ÜÈ\HPTUQUT×ÐÓÓQPÕÒTTÈY\ÜØYÙH_BÈÈÜ[Û[YÙHÝ\[[\ÛY[\XXHÓÓÔÐQÑWÒÕTSH]ÙHÓÓÝ\[ÛB[H\[\[ÙHÝ[Y\Ý[\[Ù[\Ý[][ÛÝ]\Ë\ÜÝ\K\][ÛÛ\ßKUT[HÛ\^[ØYUT[HÚÙ[
[Ù[[YH\ÂØÜXY
NÈÜ]HZ[\\È\H\ÜYÛÝ\Ú]ÈÛY[[\XÝ\ÜÙN[Ú[H x§p¨YÙHXÙHÛÜ[][Û
KPÓÓÔQTLÊK
# Mission report M-COORD-LLM-01 — Anthropic Claude connector (reception and plan-critic roles)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · License Apache-2.0

Mission [#40](https://github.com/dbouzon/osnias-clearing/issues/40) ·
Protocol: **COORD v1.5 / COORD Specification 1.0** (resumption order V1.1,
comment 7173095544) · Executor: Mistral · Independent critic and receiver:
OpenAI · Pilot: Denis Bouzon.
The French report `rapport-m-coord-llm-01.md` prevails in case of divergence.

## 1. Base and scope

- Development base: main `771696d6cfe89697125a2e39225bdcca096e7110`
  (T1, 2026-11-25, after merges #45/#46), branch `mission/coord-llm-01`
  created from that SHA (it did not exist).
- No competing mission; #42 remains suspended; M1/manifest untouched;
  `coord/index.html` untouched; no normative change.
- Scope: additive files (`commun.go`, `anthropic.go`,
  `anthropic_test.go`, `exemple-anthropic.json`,
  `recette-anthropic-reelle.md`) plus minimal, justified edits
  (`connecteurs.go`: D1 helper extraction + D4 `format` field;
  `orchestre.go`: type assertions → `ConnecteurReel` interface;
  `cmd/coord-mission/main.go`: format dispatch) plus documentation
  (connectors README, reports FR/EN).

## 2. Applied decisions (plan v2, no re-design)

- **D1**: COMMON security bricks extracted into `commun.go`
  (`baseHTTP`: guarded call loop with emission bounding, bounded 0..5
  retries, response cap, context cancellation, secret expurgation
  `SORTIE_COMPROMISE`/`[EXPURGE]`, `REPONSE_VIDE`/`JETONS_INCONNUS`/
  `MODELE_DIVERGENT` checks, monetary vault
  `EstimationCentimesSur`/`CoutReelCentimes`, the `adopter` triple lock,
  idempotence key). `HTTPEndPoint` and `AnthropicPoint` differ ONLY in
  request/response format.
- **D2**: stdlib HTTP client; `anthropic-sdk-go` v2.0.0 REJECTED
  (its retry policy is a second reprise authority bypassing the monetary
  vault; recent breaking API change 2026-10-21; minimal POST /v1/messages
  scope; uniformity with the accepted connector of #29/#30).
  `go.mod`/`go.sum` UNCHANGED (zero added dependency).
  `anthropic-version` pinned to `2023-06-01`: any version bump is an
  explicit pilot commit.
- **D3** (Messages API, fail closed): `x-api-key` + pinned
  `anthropic-version` headers (never `Bearer`); COORD template as
  top-level `system` (anti-injection template kept word for word);
  `max_tokens` = declared `jetons_sortie_max_par_tentative` bound; only
  `text` content blocks concatenated (any other block → integral refusal);
  usage = `input_tokens + output_tokens` (missing/zero → `JETONS_INCONNUS`);
  `stop_reason=max_tokens` → `SORTIE_TRONQUEE` integral refusal; unknown
  stop_reason → refusal; unexpected non-zero prompt-caching fields →
  `JETONS_INCONNUS` (COORD never emits `cache_control`);
  `MODELE_DIVERGENT` unchanged; duplicate prevention stays with the
  orchestrator; SHA-256 idempotence key still computed and logged.
- **D4**: additive `"format"` field (`openai` implicit | `anthropic`),
  backward compatible; unknown value refused at load; conductor dispatch
  at the single existing wiring point.
- **`ConnecteurReel` interface** (orchestre.go): the three
  `a.(*HTTPEndPoint)` type assertions become `a.(ConnecteurReel)`
  (monetary vault TX1, out-of-lock call, TX2 overshoot). NO semantic
  change: simulated MVP adapters (#27) do not implement the interface and
  keep their unchanged local path. Justification: a distinct
  `AnthropicPoint` type could not otherwise reach the monetary vault —
  the common change anticipated by order §4, motivated before execution.

## 3. Qualification executed (local REEL, SIMULÉ, 0 €)

Recipe C1..C15 (`anthropic_test.go`, loopback fake servers): **16/16 PASS**
(nominal success with concatenated text blocks and full usage; missing/zero
usage → `JETONS_INCONNUS`; non-text block → integral fail-closed refusal;
`max_tokens` stop → `SORTIE_TRONQUEE`; divergent model → `MODELE_DIVERGENT`;
bounded retries then `APPEL_ECHOUE`; timeout and context cancellation;
oversized response → `REPONSE_TROP_GRANDE`; emission bounding →
`CHARGE_HORS_BORNE` with zero bytes sent; key echo → `SORTIE_COMPROMISE`
with expurgated error; exact headers (`x-api-key`, pinned
`anthropic-version`, no `Bearer`); `max_tokens` equals the declared bound;
duplicate call impossible while in flight (`APPEL_EN_COURS_NON_RESOLU`);
monetary vault reserves BEFORE the network, `BUDGET_DEPASSE` refuses with
zero requests, durable reservation journaled; trapped verdicts —
non-JSON, non-anchored `ACCEPTEE`, anchored `ACCEPTEE` without
`verdict_id` — never commit any transition; plus cache-field and
unknown-stop_reason fail-closed refusals; plus a full simulated E2E:
Mistral executor / Claude receiver, anchored plan critique `PLAN_ACCEPTE`,
execution, submission, `A_CORRIGER` reception, correction, resubmission,
`ACCEPTEE` reception, `ACK_RECU`).

### Non-regression (entry and exit condition), REEL

- Go 1.27.4 linux/amd64: `go vet`/`go build` OK; `go test ./...`
  12 packages OK; 63 connector tests PASS / 0 FAIL.
- Normative vectors: 22/22, output byte-identical to the baseline
  (sha256 `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96`).
- Standalone demo: 19 CONFORME / 0 ECART; `coord-mission -demo-local`
  full cycle to `ACK_RECU`; `coord-mcp -demo-local` conforming.
- Python: 24 + 13 + 11 + 13 + 10 + 47 PASS, FAIL=0 (M1 PASS).
- `go test -race`: NOT EXECUTED locally (no gcc in the sandbox), covered
  by CI (multi-OS Race detector job).

## 4. Execution status (required distinction)

| Evidence | Status |
| --- | --- |
| C1..C15 tests, E2E, non-regression, demos | **REEL** (executed locally) |
| "Provider" calls | **SIMULATED** (loopback fake servers, 0 €) |
| Real Anthropic API qualification | **NOT EXECUTED** (no key, no call; procedure ready: `recette-anthropic-reelle.md`) |

Merge and closure remain with the pilot.

## 5. Reception request

Independent reception by OpenAI at the exact SHA of the PR, per
COORD v1.5: verdict anchored to the SHA and to the imprint, with the
identity and durable reference of the verdict, recorded in issue #40 and
on the PR. On `A_CORRIGER`, the correction loop resumes without undue
pause.

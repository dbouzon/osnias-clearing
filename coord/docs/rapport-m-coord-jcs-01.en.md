# Report M-COORD-JCS-01 — RFC 8785 canonical JSON fingerprint alignment, Python ⇄ Go

Issue #85 — ordered on 27/11/2026. COORD protocol v1.5 / Specification 1.0.
Executor: Mistral. Critic/receiver: OpenAI. Pilot: Denis Bouzon.
Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX Apache-2.0.
The French version is authoritative (translation of rapport-m-coord-jcs-01.md).

## Context and reproduction (T2, PRISTINE tree at starting SHA 3844d6a9)

Specification 1.0 (§5.2) mandates the RFC 8785 canonical form: object keys
sorted by **UTF-16 code units**, minimal escaping, integer-only numbers;
§5.3: fingerprint = SHA-256 over the canonical form, lowercase hex. Finding:
the Go engine (`coord/go/internal/jcs`, `utf16Less`) is conformant; the Python
v1.5 fingerprint path (`coord06_v15.py` `_empreinte_contenu`,
`champs_divergents`) used `coord06.canonical_json` = `json.dumps(sort_keys=True)`
= **code point** ordering, and serialized non-integers as non-normative repr.

Reproduction at SHA `3844d6a9` (cross-checked bytes and sha256, before any fix):

| Case | Python (before) | Go (RFC-expected) |
| --- | --- | --- |
| keys U+10000/U+E000 | `{"\ue000":2,"\U00010000":1}` sha256 `daec79d8…` | `{"\U00010000":1,"\ue000":2}` sha256 `4045c21a23c8ae8f8d9add81f54bd506bee65885099876fb4afb378b1f2c3516` |
| nested object in array | nested keys sorted by code points, sha256 `bed65c85…` | nested keys sorted UTF-16, sha256 `5b77a91a0ccdfcf6444e2d76237b1749f165f943240e464a139335e5b42c96a1`; array element order preserved by both |
| non-integer 1.5 | `{"valeur":1.5}` (non-normative repr) | fail-closed refusal "nombre non canonique (entier attendu)" (§5.3) |

Anchored trace: `knowledge/coord-projet/m-coord-jcs-01/reproduction-sha-3844d6a9.txt`
(sha256 `41b39b4d…`). In practice production keys are ASCII: no known
historical artifact with astral keys. `coord06.py` (v1.0..v1.4) is frozen by
manifest M1; `coord06_v15.py` and `tests_coord06_v15.py` are not (active
development line). Plan v1 submitted to the bridge (session
`m-coord-jcs-01`): **PLAN_ACCEPTE** round 1, three compatibility decisions
arbitrated (Python non-integer refusal aligned with Go/spec; public vector
rather than private fixture; spec unchanged).

## Fix (minimal, Python only; Go INTACT)

- `coord06_v15.py`: new `_canonique_v15(obj)` function — recursive canonical
  serialization in exact parity with `go/internal/jcs`: keys sorted by UTF-16
  units (sort key = `s.encode("utf-16-be")`, exact equivalent of code-unit
  comparison), compact separators, `ensure_ascii=False` (identical minimal
  escaping), arrays order-preserved, booleans/null handled before integers,
  non-integer numbers REFUSED (`ValueError`, motif
  "nombre non canonique (entier attendu)", never approximated; 2^53 bound
  = Go parity). `_empreinte_contenu` and `champs_divergents` use it.
  In `champs_divergents`, a non-canonical value is marked `<non canonique>`
  without failing the arbitration: exact parity with Go's
  `resume.ChampsDivergents`.
- `coord06.py` and every manifest-frozen file: INTACT. The STORAGE path
  (`ecriture_atomique`, JSONL journals) keeps using `coord06.canonical_json`
  (frozen): storage bytes unchanged; for ASCII keys both forms are identical
  byte for byte (verified).
- No change to the protocol, the storage format, the proofs or the historical
  manifests. `docs/spec/coord-spec-1.0.md` INTACT (l.15: historical finding
  "22/22 conformance vectors" of the original qualification).

## Shared public vector (consumed by BOTH engines)

- `coord/docs/conformite-v15/ordre_cles_utf16.json`, format
  **`coord-conformite-jcs-v1`**, family `serialisation_canonique`, 3 vectors:
  JCS-1 UTF-16 key order (canonical bytes + expected sha256 fixed
  independently by the conformant Go engine before the Python fix), JCS-2
  recursive key sorting with array order preserved, JCS-3 non-integer →
  fail-closed refusal with motif.
- BOTH runners replay the family: additive `empreinte` branch in
  `runner_conformite.py` and in `go/cmd/vector-runner` (operation
  `{"operation":"empreinte","objet":…}`, expected `{octets, sha256}` or
  `{erreur: true, contient}`).
- `go/internal/jcs/jcs_test.go`: `TestVecteursPartagesJCS` consumes THE SAME
  file (parity proof against the same artifact).
- `schemas/vector-jcs.schema.json` (JSON Schema 2020-12, additive) + additive
  branch in `.github/scripts/coord-validate-schemas.py`: the new family is
  schema-validated like the v1 family (22 vectors/4 v1 files INTACT,
  + 3 vectors/1 jcs file).
- Public counter: **22 → 25 vectors** (5 files). Fidelity note to the plan:
  plan v1 said "22 → 23"; since 3 new cases are added, the arithmetic total is
  25. Doc-only delta, reported to the receiver.

## Tests and non-regression (REAL, replayed after the fix)

- `tests_coord06_v15.py`: 6 new tests (JCSa..JCSf: UTF-16 order, recursive
  sorting + array order, non-integer refusal, base types and minimal
  escaping, H5 fingerprints through the normative form + `<non canonique>`
  parity, byte-for-byte compatibility with `coord06.canonical_json` on ASCII
  keys). v1.5 battery: **53/53 PASS** (47 + 6), M1 PASS (historical files
  unchanged against main `323d8b9`).
- Historical: 24+13+11+13+10+17 = **88/88 PASS**; robustness included.
- Vectors: **25/25** on the Python module AND on the Go engine;
  Python ⇄ Go cross-check: the 25 [OK]/[KO] verdicts are IDENTICAL line by
  line (both runner outputs compared byte for byte).
- Go: `go vet ./...`, `go build ./...`, `go test ./...` green (jcs included).
- Schemas: the CI validator (extended section 6) replayed with jsonschema
  2020-12 on an equivalent harness (package installation is forbidden in the
  executor's sandbox): 22/4 v1 family + 3/1 jcs family, `vector-jcs` schema
  valid.

## Read-only compatibility

Old vs new Python code fingerprint recomputation over: every `arbitrer`
record of the existing vectors, a sample of real v1.5 records (resume,
journal, habilitations, large-integer cursor) → **IDENTICAL fingerprints**
(ASCII keys). Storage bytes (`coord06.canonical_json`, frozen) unchanged.
Historical proofs and archives keep their bytes and fingerprints; they read
back normally. Non-verifiable cases: none identified.

## Documentation (living docs only)

Counters updated 47→53 tests, 22→25 vectors, 5→6 schemas:
`README.md`, `index.html`, `implementations/index.html`,
`CALL-FOR-IMPLEMENTATIONS.md`, `docs/coord-guides.md`,
`docs/proto-coord06-v15.md` (jcs family description added),
`.github/workflows/coord-go-ci.yml` (step names). Historical reports and
qualifications INTACT.

## Delivery

Branch `mission/coord-jcs-01`, PR at the exact SHA, CI (Python engine
modified + schema validator extended), independent OpenAI reception,
records (issue #85, Drive, KNOWLEDGE.md). Merge and closure reserved to the
pilot. 0 €, no keys, no billable calls, loopback only.

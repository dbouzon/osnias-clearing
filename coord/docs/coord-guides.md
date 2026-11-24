# COORD Implementer Guide

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

How to implement COORD Specification 1.0 without reading the reference code.

## 1. Read the normative pieces only

1. `coord/docs/spec/coord-spec-1.0.md` — the specification (English, RFC 2119).
2. `coord/schemas/*.schema.json` — the wire formats (JSON Schema 2020-12):
   message envelope, identity, verdict payload, habilitation configuration,
   conformance vector collection.
3. `coord/docs/conformite-v15/` — the 22 published conformance vectors
   (`coord-conformite-v1`): 3 positive transition scenarios, 9 negative
   transition scenarios, 8 habilitation configurations, 2 resume arbitrations.

An implementation that passes all 22 vectors conforms to the specification
(spec 10). The reference runners (Go `cmd/vector-runner`, Python
`runner_conformite.py`) are adapters with no normative role; the Python runner
accepts an alternative engine as its second argument.

## 2. Mandatory semantics checklist

- 8 states, closed transition table (spec 6.2): refuse any pair not listed.
- Identity: qualified only by a registered adapter; the engine never
  authenticates (spec 3).
- Fail-closed habilitations: invalid configuration refuses everything
  (spec 4, N-01..N-05); no wildcards, no duplicates.
- No self-reception (N-11): executant and receiver differ by `id`.
- Verdict requires `verdict_id` + `verdict_url`; verdict value matches the
  target state (N-12).
- Staleness by fingerprint and revision, never by counters (N-13).
- Atomic persistence: temp file, fsync, rename, directory fsync (N-15).
- Resume arbitration: divergence => the reference support makes the decision;
  journal both fingerprints (N-16); resume only on a matching verified head
  (N-17).
- Context control: score >= 0.5 AND declared provenance; idempotent journal
  storing the instruction fingerprint, never its text; no interruption, no
  decision from a deviation (N-18..N-20).

## 3. Wire identifiers

States `PLANIFIEE..CLOTUREE`, actions `executer, soumettre, recevoir, acker,
cloturer`, roles `EXECUTANT, RECEPTIONNAIRE, PILOTE`, data keys `revision`
(40 hex), `empreinte_preuves` (64 hex), `verdict_id`, `verdict_url`,
`revision_courante`, `empreinte_courante`. Timestamps RFC 3339 UTC (`Z`).
Canonical form RFC 8785; proofs SHA-256 over the canonical form.

## 4. Testing your implementation

- Replay the vectors: every `attendu` must match your outcome exactly,
  including refusal motifs (they carry the keywords asserted by the vectors).
- Add unit tests for the three crash points of persistence (original intact,
  then new record in place) and for journal idempotence.
- Cross-check with the reference engines: both render the same OK/KO per
  vector on the 22 published vectors.

## 5. Declaring conformance

Publish: your results per vector, your environment, the qualification
(REAL/SIMULATED/NOT EXECUTED) of each test family, and the divergences you
documented. Do not claim authentication security: adapters carry the trust
boundary; simulated adapters prove nothing about it (spec 11).

# Call for Independent Implementations — COORD Specification 1.0

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**COORD Specification 1.0 — An Open Standard for Human-Governed LLM
Collaboration** structures the cooperation of software agents (typically
LLM-backed, from any vendor) on a shared mission under the authority of a
human pilot. The specification is language-neutral, vendor-neutral and fully
public. Conformance is claimed only by an implementation that passes all
published conformance vectors.

We invite independent implementers to build a third (or fourth, or n-th)
implementation, in any language, and to publish their results.

## What to implement

Read the normative pieces only (this is enough, no reference code needed):

1. `coord/docs/spec/coord-spec-1.0.md` — the specification (English, RFC 2119).
2. `coord/schemas/*.schema.json` — the wire formats (JSON Schema 2020-12).
3. `coord/docs/conformite-v15/` — the 25 published conformance vectors:
   3 positive transition scenarios, 9 negative transition scenarios,
   8 authorization configurations, 2 resume arbitrations, and 3 canonical
   serialization scenarios (`coord-conformite-jcs-v1`: UTF-16 key ordering,
   recursive key sorting with array order preserved, refusal of non-integer
   numbers).
4. `coord/docs/coord-guides.md` — the implementer guide.

## The rules

- Replay every vector: each `attendu` must match your outcome exactly,
  including refusal motifs.
- A closed transition table, fail-closed authorizations, referenced verdicts,
  staling by fingerprint (never counters), atomic persistence, resume
  arbitration where the reference support makes the decision, context control
  that never interrupts and never emits decisions from a deviation.
- The engine never authenticates: adapters carry the trust boundary (spec 11).
  Simulated adapters prove nothing about authentication security. Say so.

## Publishing your results

Publish, per vector: your outcome (OK/KO), your refusal motifs where a vector
asserts them; your environment (language, version, OS); the qualification of
each test family (REAL / SIMULATED / NOT EXECUTED); and your documented
divergences. Open an issue in the public repository
[dbouzon/osnias-clearing](https://github.com/dbouzon/osnias-clearing) with the
`[COORD-IMPL]` prefix, or publish in your own repository and reference it.
Cross-check against the reference engines (Go `coord/go/cmd/vector-runner`,
Python `coord/docs/conformite-v15/runner_conformite.py`): both render the same
OK/KO per vector on the 25 published vectors.

The standard, the vectors, the schemas and the reference engines are published
under the Apache-2.0 license. COORD Specification 1.0 is an open candidacy: no
official standard recognition is claimed, and implementers should not claim it
either.

## Résumé en français

Appel à implémentations indépendantes de COORD Specification 1.0 : relisez la
norme (`coord/docs/spec/coord-spec-1.0.md`, anglais, RFC 2119), les 6 schémas
JSON 2020-12 et les 25 vecteurs publics (`coord/docs/conformite-v15/`),
implémentez la machine à états fermée dans le langage de votre choix, rejouez
les 25 vecteurs (chaque `attendu` doit correspondre exactement, motifs de refus
compris), puis publiez vos résultats, environnement et qualifications
(REEL / SIMULE / NON_EXECUTE) dans un ticket `[COORD-IMPL]` du dépôt public
dbouzon/osnias-clearing. Le moteur n'authentifie jamais : les adaptateurs
portent la frontière de confiance. Licence Apache-2.0, aucune reconnaissance
officielle revendiquée. Page publique :
https://www.osnias-clearing.com/coord/implementations/

# COORD Specification 1.0

Language-neutral normative specification of the COORD protocol for collaboration
between language-model agents under human authority.

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

Status: CANDIDATE, NOT ACTIVATED. Implemented reference engines: Go
(`coord/go`, this specification's reference implementation) and Python
(`coord06_v15.py`, hardened candidate of mission M-COORD-HARDENING-01).

The key words MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT,
and MAY are to be interpreted as described in RFC 2119 and RFC 8174.

## 1. Purpose and model

COORD structures the cooperation of software agents (typically LLM-backed, from
any vendor) on a shared mission under the authority of a human pilot. The
protocol is independent of any vendor, transport, or programming language:
every normative artifact (state machine, envelope, verdicts, proofs,
conformance vectors) is defined here and published as JSON.

Three roles exist (Section 2). A mission is a unit of work with an issue
reference, carried by a finite state machine (Section 6) whose every transition
is authorized (Section 4) and evidenced (Section 7). Reception is always
performed by an agent distinct from the executant (Section 6.3). The pilot
alone closes a mission (Section 6.6).

## 2. Roles

- EXECUTANT: performs the mission work, submits proofs for reception, and
  acknowledges the accepted verdict.
- RECEPTIONNAIRE (receiver): reviews a submission and issues a verdict.
- PILOTE (pilot): the human authority; closes a mission. The pilot is the only
  role that may merge, activate, or close (Sections 6.6, 12).

A role is a capability profile, never an identity: the same agent may hold
different roles on different missions, but NOT simultaneously acting as
executant and receiver on the same mission (Section 6.3, Invariant N-05).

## 3. Identities and adapters

The engine MUST NOT authenticate. An identity is a JSON object:

```json
{"id": "agent-1", "role": "EXECUTANT", "qualified_by": "adapter-x"}
```

- `id`: opaque non-empty string.
- `role`: one of the three roles (Section 2).
- `qualified_by`: identifier of the adapter that qualified the identity at the
  trust boundary.

An identity is engine-qualified if and only if `qualified_by` belongs to the
configured adapter list (Section 4). The engine MUST NOT verify the channel,
the vendor, or any credential: channel verification is the adapter's
responsibility outside the specification. An adapter identifier is a
capability-neutral string; the specification MUST NOT require a specific
adapter implementation.

## 4. Authorization (fail-closed habilitations)

Authorization configuration:

```json
{
  "adapters": ["adapter-x", "adapter-y"],
  "roles": {"EXECUTANT": ["execute", "submit", "ack"],
            "RECEPTIONNAIRE": ["receive"],
            "PILOTE": ["close"]}
}
```

Validation rules (all MUST hold for the configuration to be valid):
- N-01: `adapters` MUST be a list of non-empty distinct strings. An empty or
  absent list is valid and refuses everything.
- N-02: role names MUST be known; action names MUST be known.
- N-03: wildcard actions MUST be refused.
- N-04: duplicate actions for a role MUST be refused.
- N-05*: any invalid configuration MUST close the engine: every operation is
  refused (fail closed). Validity is checked before every operation.

Authorization check (all MUST hold):
1. configuration valid (N-01..N-04),
2. identity engine-qualified (Section 3),
3. action belongs to `roles[identity.role]`.

## 5. Envelope, canonical form, proofs

### 5.1 Message envelope

Every inter-agent COORD message carries:

```json
{
  "type": "verdict|submission|ack|control",
  "mission": "M-XXX-01",
  "issue": "#20",
  "actor": {"id": "...", "role": "...", "qualified_by": "..."},
  "state": "EXECUTION",
  "data": {},
  "ts": "2026-11-24T22:00:00Z"
}
```

- Encoding UTF-8, JSON per RFC 8259.
- `ts` MUST be an RFC 3339 timestamp in UTC with the `Z` suffix.
- `actor` is a Section 3 identity.
- `data` is payload typed by `type`; verdict payloads are normative
  (Section 6.4), control payloads are local policy.

### 5.2 Canonical form

For fingerprinting, any COORD JSON data MUST be serialized per RFC 8785 (JSON
Canonicalization Scheme): lexicographic ordering of object keys by UTF-16 code
units, no insignificant whitespace, minimal string escaping, and numbers either
integers or non-exponent original shortest form. An implementation that cannot
serialize a data item canonically MUST refuse the operation rather than emit a
non-canonical form.

### 5.3 Proofs

- Fingerprint: SHA-256 over the canonical form, hex lowercase (64 chars).
- Revision: 40 lowercase hex characters (git commit SHA-1 compatible).
- Integrity MUST be anchored to fingerprints, never to counters (Invariant
  N-08).

## 6. State machine

### 6.1 States

`PLANNED, EXECUTING, DECISION_PENDING, ACCEPTED, IN_CORRECTION, ACK_RECEIVED,
REFUSED, CLOSED`

(Reference French mapping of the Python candidate: PLANIFIEE, EXECUTION,
DECISION_ATTENDUE, ACCEPTEE, EN_CORRECTION, ACK_RECU, REFUSEE, CLOTUREE.)
The published conformance vectors use the French identifier set for states,
actions and data keys; an implementation MUST accept exactly those
identifiers. The English labels above are the normative names; the French set
is the normative wire mapping used by all published artifacts.

`REFUSED` and `CLOSED` are terminal; N-09: no transition leaves a terminal
state. Actions: `execute`, `submit`, `receive`, `ack`, `close` map to the wire
set `executer`, `soumettre`, `recevoir`, `acker`, `cloturer`.

### 6.2 Transition table (closed)

| From | Target | Role | Action | Required data |
| --- | --- | --- | --- | --- |
| PLANNED | EXECUTING | EXECUTANT | execute | — (binds the executant identity, N-06) |
| EXECUTING | DECISION_PENDING | EXECUTANT | submit | `revision`, `proofs_fingerprint` |
| DECISION_PENDING | ACCEPTED | RECEPTIONNAIRE | receive | `verdict`, `verdict_id`, `verdict_url` |
| DECISION_PENDING | REFUSED | RECEPTIONNAIRE | receive | `verdict`, `verdict_id`, `verdict_url` |
| DECISION_PENDING | IN_CORRECTION | RECEPTIONNAIRE | receive | `verdict`, `verdict_id`, `verdict_url` |
| IN_CORRECTION | EXECUTING | EXECUTANT | execute | — (same executant only, N-06) |
| ACCEPTED | ACK_RECEIVED | EXECUTANT | ack | `revision_current`, `fingerprint_current` |
| ACK_RECEIVED | CLOSED | PILOTE | close | — |

N-10: the table is closed: any (state, target) pair not listed is refused.
Each transition MUST also hold a valid authorization (Section 4) for its
stated role and action.

### 6.3 Reception

N-11 (no self-reception): the receiver of a verdict MUST be an agent whose
`id` differs from the bound executant's `id`; a mission where the executant
submits and receives its own submission is invalid and MUST be refused.

### 6.4 Verdicts

Verdict values: `ACCEPTED`, `A_CORRECT` (leads to IN_CORRECTION), `REFUSED`.
N-12: any verdict result requires a non-empty `verdict_id` and `verdict_url`
(evidence reference). A refusal without verdict reference is invalid (thesis:
a refusal without grounds is not actionable). The `verdict` value MUST be
coherent with the target state.

### 6.5 Staleness

N-13: an accepted verdict becomes stale as soon as the current proofs
fingerprint or revision diverges from the values submitted at decision time;
an ack on stale proofs MUST be refused. The engine MUST compare fingerprints,
never counts.

### 6.6 Closure

N-14: only the PILOTE may move ACK_RECEIVED to CLOSED.

## 7. Persistence

N-15: persistence of the mission record MUST be atomic: write a complete
temporary file, fsync, rename over the target, fsync the directory. A crash at
any point MUST leave either the previous complete record or the new complete
record, never a partial one. Implementation-defined crash points are allowed
for testing. A record MUST be stored in canonical form (Section 5.2).

## 8. Resume and arbitration

On restart, engines compare the resume record available in the local knowledge
base with the one published on the reference support. N-16: on divergence, the
reference support makes the decision (`github_makes_foi`); the arbitration
decision MUST be journaled with: chosen support, divergent field names, SHA-256
fingerprints of both complete records, UTC timestamp, and arbiter identifier.
N-17: after arbitration, the next action executes only if the verified head
(SHA) matches the record; a divergent head makes re-verification necessary.

## 9. Context control

An instruction received during a mission MUST be qualified before use:

- N-18: qualification score = fraction of mission-order terms covered by the
  instruction; an instruction is COHERENT only if the score is at least 0.5 AND
  the declared provenance is non-empty; otherwise it is an ISOLATED_DEVIATION.
- N-19: deviations MUST be journaled idempotently (fingerprint of the
  instruction, never its free text beyond operational need: the journal stores
  the hash, the qualification, the willingness-declared source, and the
  decision). A replay of the same deviation MUST NOT duplicate the entry.
- N-20: a deviation MUST never interrupt the mission, never produces system
  decisions, and the mission resumes at the current step (`no interruption, no
  decision from a deviation`).

## 10. Conformance and vectors

Conformance to COORD Specification 1.0 is claimed only by an implementation
that passes ALL published conformance vectors of the `coord-conformite-v1`
format (directory `coord/docs/conformite-v15/`), including the complete replay
results: 3 positive transition scenarios, 9 negative transition scenarios,
8 habilitation configurations, 2 resume arbitrations. Vectors are
language-neutral JSON, normative, and published with the specification. The
runner is an adapter with no normative role.

## 11. Limits

- The engine never authenticates; adapters carry the trust boundary.
- Simulated adapters in tests and demonstrations are not evidence of
  authentication security.
- No specific vendor, model, or transport is required or excluded.
- The schema family lives in `coord/schemas/` (dialect 2020-12); verdict
  payloads and habilitation configurations follow the schemas; schemas are
  versioned alongside the specification.

## 12. Authority

Merge, activation, closure: the pilot only. Reception of the Go reference
engine per the v1.4 cycle: independent OpenAI reception, document trail on
GitHub, activation decided by the pilot.

# Operation identities and explicit replay (M-COORD-FLOW-01)

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

## 1. Operation identity

Every call to a participant (critique, execution, submission, reception) is an **operation** identified by a random `id_operation` (16 crypto/rand bytes, hex), specific to the operation and independent of the payload: two identical volunteers produce two distinct identities.

- The identity is persisted **before** the call in `appel_en_cours.json` (additive field `id_operation,omitempty`) and in the append-only registry `operations.jsonl` stored next to the mission journal.
- Every call journal line (APPEL_RESERVE, APPEL_REEL, APPEL_ECHOUE, RECEPTION, RESOLUTION_APPEL) carries `id_operation`: journals read per operation, the registry reads per mission.

Registry events: `OPERATION_OUVERTE`, `OPERATION_TERMINEE`, `OPERATION_ECHOUEE`, `OPERATION_INTERROMPUE`, `OPERATION_RESOLUE`, `OPERATION_REJETEE` (informational: a replay refusal never changes an operation state).

## 2. Replay contract

Replay is **explicit**: `AvecRejeu(<id|"derniere">)` on the orchestrator, `-rejeu` flag on the CLI. The contract is validated **before any call** (fail closed):

| Target operation state | Replay outcome |
|---|---|
| TERMINEE | `OPERATION_DEJA_TERMINEE`: zero calls, zero budget, no effect |
| ECHOUEE, RESOLU_PERDU, RESOLU_ECARTE, RESOLU_ECHOUE | relaunch with the SAME `id_operation`, a FRESH attempt key |
| EN_COURS | `ErrAppelEnCours`: a call is already reserved |
| INTERROMPUE, RESOLU_RECU | `OPERATION_NON_REJOUABLE`: a NEW voluntary operation is required |
| unknown | `OPERATION_INCONNUE` |
| divergent action/payload/actor | `OPERATION_INCOMPATIBLE`: refused before any call |

Every refusal is journaled (`OPERATION_REJETEE`, informational) and traceable without changing anything.

## 3. Resuming after a brutal stop

An old state **without identity** remains readable (backward compatibility) but never triggers a new automatic call when a prior result is uncertain: the engine blocks with `APPEL_EN_COURS_NON_RESOLU` and the arbitration belongs to the pilot.

D7 procedures (pilot decision, never automatic):

- **a) certain result absent**: the call did not reach the participant. The pilot may replay the operation (`-rejeu <id>`: fresh attempt key, identity kept) or abandon it (`-resolution PERDU|ECARTE|ECHOUE`).
- **b) uncertain result** (process killed mid-call): the response may have landed. The pilot arbitrates: `RECU` if the response actually arrived (the operation becomes non-replayable), otherwise `PERDU`/`ECARTE`/`ECHOUE` (the operation becomes replayable).

## 4. Registry integrity (fail closed)

The registry is re-read **on every** call preparation: a corrupt `operations.jsonl` blocks all new operations (`OPERATIONS_ILLISIBLES`); a missing registry is treated as empty (legacy missions compatible, no read block). The `OPERATION_OUVERTE` write happens in transaction TX1 (after reservations): its failure cancels the call; decisions (TERMINEE/INTERROMPUE) are written in TX2 before commit.

## 5. Finding completed operations

`coord-mission -consulter <mission>` exposes per-state counting (`terminees`, `echouees`, `interrompues`, `en cours`) on STDERR (stdout stays pure JSON). `ResumeOperations(racine, issue)` provides the same aggregation to the API.

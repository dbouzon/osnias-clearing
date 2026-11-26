# Mission report M-COORD-LLM-05 — Wiring Claude Code into the shared A2A agent server

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · License Apache-2.0

Mission: issue [#58](https://github.com/dbouzon/osnias-clearing/issues/58) (COORD v1.5)
Executor: Mistral · Technical direction, review and acceptance: OpenAI · Pilot: Denis Bouzon
Branch: `mission/coord-llm-05` · Base: main `34baff2c` (direct child of the PR #57 merge, pinned tarball `c07af05a…`)

## 1. Summary

The Anthropic connector became TWO-WAY: the EXISTING inference API path
is UNCHANGED byte for byte (no regression, per the order) and a new
SUPERVISED Claude Code path is wired into the same A2A agent server,
selected by the payload marker `VOIE:CODE`. The supervisor REUSES the
generic runner of M-COORD-LLM-03 AS IS (environment whitelist, secret
scrubbing, cooperative SIGTERM then process-group SIGKILL, exit codes
0/1/2/3): no duplication. Qualification is fully SIMULATED through the
`claude-factice` stub CLI (mimicking the documented `claude -p
--output-format json` contract): 0 EUR, no key, no provider network, no
GitHub writes. The three blocking points from OpenAI's plan review
(session locking, explicit validation errors, permission checks) are
handled with named, error-constant behaviour and tested. REAL
qualification remains NOT_EXECUTED (separate route, triple lock).

## 2. Decisions and reviews

- **Plan**: round 1 A_CORRIGER (3 blockers), consolidated v2,
  **round 2 PLAN_ACCEPTED** (bridge session `m-coord-llm-05`, pinned
  plan sha256 `0682005d…`). T3 intermediate review: **COMPLIANT**.
- **Honest cards**: the anthropic agent advertises `coord-inference`
  AND `coord-claude-code`; other providers still advertise inference
  only. `OptionsServeur.Skills` is ADDITIVE (empty = default,
  retrocompatible).
- **Parity with Mistral Vibe without artificial alignment**: same
  two-way agent skeleton and same runner, but Claude Code's own
  contract is respected (sessions = CLI `--resume <session_id>`,
  permissions = `dontAsk` + `--allowedTools` + deny `--settings`,
  spend = client-side `total_cost_usd` estimate).

## 3. Deliverables (branch `mission/coord-llm-05`)

| File | Kind | Content |
| --- | --- | --- |
| `coord/a2a-agent/claudecode/claudecode.go` | NEW | `ExecClaudeCode` (VOIE:CODE path; MISSION/REVISION/CORRECTION/TACHE), two-way `Agent` (data routing), strict CLI output validation, settings.json written then validated, HOME rewritten, local `--max-budget-usd` cap |
| `claudecode/registre.go` | NEW | LOCKED session registry: exclusive non-blocking file lock (bounded retries 200×5 ms), atomic write (temp+rename) STRICTLY after validation, fail-closed `REGISTRE_VERROU_INDISPONIBLE` / `SESSION_INCOHERENTE` / `REGISTRE_CORROMPU` |
| `claudecode/verrou_posix.go` / `verrou_windows.go` | NEW | POSIX flock / Windows LockFileEx (build tags, go.sum completed) |
| `claudecode/claudecode_test.go` | NEW | CC1..CC21 acceptance + CC-CONCUR1/2 + CC-REINTEG (detail §5) |
| `cmd/claude-factice/main.go` (+ `signaux_posix.go`/`signaux_windows.go`) | NEW | Stub CLI mimicking `claude -p --output-format json`: result/session_id/is_error/total_cost_usd/num_turns/environment fields; SCENARIO: markers in the prompt (BASE, ERR_IS_ERROR, JSON_INVALIDE, VIDE, SESSION_ABS, PERM_REFUS, BUDGET, FUIE, HANGUE, LENTE, HOSTILE, QUITTE1/2/3, OUTIL:, LIVRABLE_A_CORRIGER); per-session state and call counter under the rewritten HOME; on-board checks (dontAsk required, deny required, off-list tool refused) |
| `cmd/coord-agent/claudecode_agent.go` | NEW | `optionsClaudeCode` (stub CLI build, dedicated workspace, registry), honest skills, reproducible `-demo-claudecode` A2A demo |
| `cmd/coord-agent/agents_llm04.go` | MODIFIED (additive) | `-fournisseur anthropic` becomes two-way (unchanged inference + VOIE:CODE); other providers unchanged |
| `cmd/coord-agent/main.go` | MODIFIED (additive) | `-demo-claudecode` flag; existing demos unchanged |
| `agents/serveurs.go` | MODIFIED (additive) | Optional `OptionsServeur.Skills` (honest parametrisable card, retrocompatible) |
| `coord/go/internal/a2a/claudecode_llm05_test.go` | NEW (engine) | CC15: simulated mission Claude Code executor / OpenAI recipient, full A_CORRIGER loop, on REAL subprocess servers |
| `coord/go/internal/a2a/agents_llm04_test.go` | MODIFIED | Launcher: working dir = agent module (the anthropic agent builds its stub CLI at startup) |
| `coord/connecteur/Claude/exemple-config-claudecode.json` | NEW | Example configuration WITHOUT secrets, SIMULATED mode, triple lock documented |

## 4. Security (fail closed, named errors)

1. **Permissions, triple check**: supervisor BEFORE launch (only
   `dontAsk` accepted, `MODE_PERMISSION_REFUSE`, bypassPermissions
   forbidden by the order); settings.json WRITTEN then VALIDATED by
   re-reading (mandatory network denies `WebFetch`/`WebSearch`/
   `WebRequest`, allow ⊆ whitelist else `REGLE_PERMISSION_INCONNUE`);
   on-board stub check (refuses any other mode, an off-list tool, or a
   settings file without denies).
2. **Output validation, one error constant per case**:
   `RESULTAT_NON_JSON`, `RESULTAT_ECHEC:<code>`, `IS_ERROR_VRAI:<reason>`,
   `SESSION_ID_ABSENT`, `RESULT_VIDE`, `COUT_INCONNU` (never
   unknown=free), `RESULTAT_COUT_HORS_BORNE`, `RESULTAT_TROP_GRAND`,
   `ENV_HORS_LISTE_BLANCHE`. Any validation failure opens NO session
   (a failed run can never be resumed).
3. **Isolation**: one dedicated workspace per mission, HOME rewritten
   (no access to the real `~/.claude`, no credentials), EXACT
   environment whitelist [HOME, PATH] checked against the environment
   declaration returned by the CLI (any divergence is refused with
   `ENV_HORS_LISTE_BLANCHE`).
4. **Secrets**: runner scrubbing kept (sk-, xoxb- etc.); no secret in
   deliverables, registry, settings or example configurations (checked
   by the acceptance suite).
5. **Sessions**: exclusive non-blocking lock with bounded retries;
   atomic post-validation write; named failures
   (`REGISTRE_VERROU_INDISPONIBLE`, `SESSION_INCOHERENTE`,
   `REGISTRE_CORROMPU`).
6. **GitHub**: no writes possible (no remote, no network argument,
   network denies); self-acceptance forbidden by the normative machine
   (unchanged), checked by CC15 (the recipient is a DISTINCT agent and
   the verdict is anchored).

## 5. Executed acceptance (sandbox, Go 1.27.4, Linux)

Package `claudecode` (5/5 module packages ok): CC1 nominal (session
recorded, counter 1); CC2 correction in the SAME session (`--resume`,
counter 2) + resume without a session REFUSED; CC3 `IS_ERROR_VRAI`
with no session opened; CC4 `RESULTAT_NON_JSON`; CC5 `RESULT_VIDE`;
CC6 `SESSION_ID_ABSENT`; CC7 permission refusal (`PERM_REFUS` and
`OUTIL:WebFetch`, code 2); CC8 budget (code 3) + side-options
`RESULTAT_COUT_HORS_BORNE`; CC9 cancellation `ANNULE` (no session) +
forced group stop on `HANGUE` (`ARRET_FORCE`, bounded, POSIX); CC10
hostile payload contained (data, never executed); CC11/CC12 sk-/xoxb-
secrets scrubbed (deliverable, registry and settings clean); CC13
network denies present; CC14 exact ENV whitelist, `GH_TOKEN` leak
REFUSED; CC19 the stub refuses `bypassPermissions`; CC20 off-whitelist
allow REFUSED; CC-CONCUR1 (2 goroutines × 20 writes: 40/40, no loss);
CC-CONCUR2 (held lock: bounded REFUSAL, never blocked); CC-REINTEG
(corrupted registry: explicit REFUSAL).
CC15 (engine, E2E): simulated mission **Claude Code executor / OpenAI
recipient** on real subprocess servers — VOIE:CODE execution (run 1,
deliverable carrying the A_CORRIGER marker) → A_CORRIGER reception →
EN_CORRECTION → correction in the SAME session (run 2, marker removed)
→ ACCEPTED (anchored verdict, verdict_id/verdict_url) → ACK_RECU;
closure `CLOTURE_REFUSEE` outside the pilot. PASS.
Non-regressions: LLM-04 12-couple matrix 12/12 PASS (launcher fixed),
engine 13/13 packages, agent module 5/5, coord/a2a green, conformance
vectors 22/22, `-demo-local`, `-demo-llm04` and `-demo-claudecode`
identical and REPLAYED (2×). Multi-OS: `GOOS=windows go vet` + build
OK (go.sum completed for golang.org/x/sys/windows).

## 6. Mistral / Anthropic comparative table (supervised CLI paths)

| Aspect | Mistral — Vibe CLI path | Anthropic — Claude Code path |
| --- | --- | --- |
| Payload | `VOIE:VIBE MISSION= REVISION= CORRECTION= TACHE=` | `VOIE:CODE MISSION= REVISION= CORRECTION= TACHE=` (same template) |
| Execution example | deterministic worktree, `--prompt-file`, `--max-tokens` bounding the envelope BEFORE launch | dedicated workspace + rewritten HOME, `-p`, `--max-budget-usd` local cap |
| Sessions | one session per revision, Vibe `--resume` | same key (mission, revision), CLI `--resume <session_id>`, LOCKED registry |
| Error example | `EXPORT_MANQUANT` → FULL envelope (never unknown=free) | `COUT_INCONNU` → REFUS, no session opened (same principle, distinct contracts) |
| Permissions | env whitelist + codes 0/1/2/3 + no remote | same + dontAsk triple check (mode, settings, on-board) |
| Cancellation | SIGTERM → group SIGKILL (shared runner) | same, shared runner reused as is |
| Accounting | tokens read from export.json | declared tokens (800/simulated run) + CLI-estimated `total_cost_usd` cross-checked by `--max-budget-usd`; the COORD vault (TX1/TX2) remains the AUTHORITY |

The package README and this report document every error constant
(cause, remedy) and every SCENARIO: marker.

## 7. Measurements

CC15 E2E: 1.9–2.2 s (binary build + 2 agents + full loop). `coord-agent`
binary 12.98 MB; `claude-factice` 3.98 MB; listening two-way agent ≈ 8.1
MB RSS. Synchronous behaviour unchanged (execution inside
`message/send`): A2A async remains out of scope (contract extension to
be validated); resuming an uncertain task stays with the PILOT.

## 8. Documented limits

1. **Deliberately minimal stub contract**: the stub CLI mimics the
   documented Claude Code contract (`-p`, JSON, sessions, permissions,
   budget) but is NOT Claude Code; real qualification will follow the
   DISTINCT recipe: triple lock (pilot's written authorisation,
   `ANTHROPIC_API_KEY` from the process vault never committed,
   dedicated account), low ceilings, B1..B5 as for Mistral.
   NOT_EXECUTED.
2. **If a budget limit or sufficient isolation cannot be guaranteed in
   production, the real path stays disabled** (order §4).
3. **Windows/HANGUE**: the signal-based forced-stop test is skipped on
   Windows (compilation stubs); Windows remains covered by
   `taskkill /T` (S15, shared runner).
4. `stream-json` not required while synchronous; CLI spend is a
   client-side estimate, cross-checked by the COORD vault (authority).

## 9. Final state and next steps

CC1..CC21 + concurrency + E2E: ALL PASS locally (Linux); 3-OS + race CI
confirmed on the PR (T5). A single PR to `main`, FR/EN reports,
independent OpenAI acceptance at the exact SHA recorded in issue #58.
Merge and closure reserved to the pilot (Denis Bouzon). Real
qualification remains a DISTINCT route subject to the pilot's written
authorisation.

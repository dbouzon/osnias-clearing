# M-COORD-EXP-01 — Expérimentation A2A-over-MCP (issue #38)

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

POC Go **isolé** (module `coord/exp`, `go.mod` propre, aucune dépendance au moteur `coordengine`) comparant **A2A direct** (mode A) et **A2A encapsulé dans MCP** (mode B) sur les mêmes tâches et conditions. Hors ligne, boucles locales uniquement, agents simulés, 0 €, aucune clé, aucun changement normatif, aucune modification du moteur ou du serveur MCP #37.

## Contenu

| Emplacement | Rôle |
| --- | --- |
| `agents/` | Les deux agents A2A simulés (AGENT-EXEC, AGENT-CRIT) |
| `modea/` | Mode A : pile A2A direct (SDK `a2a-go/v2` v2.9.0) |
| `modeb/` | Mode B : passerelle MCP expérimentale (`go-sdk` v1.8.0) encapsulant la même couche A2A |
| `conduite/` | Scénarios S1..S9 + S10 (gouvernance) comparés par paire (sha256 sémantique) |
| `cmd/coord-exp/` | CLI : `coord-exp` (rapport S1..S9), `coord-exp -s10` (gouvernance), `-json` |
| `RAPPORT.md` / `RAPPORT.en.md` | Rapport FR/EN : mesures réelles et conclusion GO/NO-GO |
| `preuves/` | Traces de qualification (S1..S9, S10) |

## Reproduire

```bash
cd coord/exp
go vet ./... && go build ./...
go test ./...            # S1..S9 + S10, boucles locales, ~10 s, 0 EUR
go run ./cmd/coord-exp        # tableau comparatif S1..S9 (Markdown)
go run ./cmd/coord-exp -s10   # scénario de gouvernance seul
go run ./cmd/coord-exp -json  # sorties JSON (qualification)
```

CI : `.github/workflows/coord-exp-ci.yml` (Go 3 OS + race detector), déclenchée sur `coord/exp/**`.

## Statut

Expérimentation M-COORD-EXP-01. Le moteur COORD (`coord/go`) et le serveur MCP #37 sont utilisés **inchangés**. Conclusion et décision de qualification réelle : voir `RAPPORT.md` (décision réservée au pilote).

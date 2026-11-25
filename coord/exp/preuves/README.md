# Preuves de qualification — M-COORD-EXP-01 (issue #38)

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

Toutes les exécutions sont REELLES en boucle locale (loopback), hors ligne, sans compte ni clé, 0 EUR. Qualification des composants : binaire coord-pilot et coord-mcp SIMULES (adaptateurs factices, jamais de fournisseur réel).

| Fichier | Contenu | sha256 |
| --- | --- | --- |
| `conduite-s1-s9.md` | Tableau comparatif et détails des scénarios S1, S2, S3, S3b, S4, S5, S6, S7, S8, S9 (modes A et B) | `5cbfb159f96917ac73987e5c4b7334b1c8f82405849c19f5261e4874beb29d57` |
| `gouvernance-s10.md` | Scénario S10 : Pilot réel + serveur MCP #37 lecture seule + tests négatifs anti-mandat | `c3e92cd208ddcbe2ef07d312d866f6480451b16b9fffa4ad04501f5937c5030e` |

## Régénérer

```bash
cd coord/exp
go run ./cmd/coord-exp       > preuves/conduite-s1-s9.md
go run ./cmd/coord-exp -s10  > preuves/gouvernance-s10.md
go test ./...                # assertions GO/NO-GO (TestConduiteS1S9, TestS10Gouvernance)
```

Les empreintes sémantiques internes (colonne sha du tableau) sont déterministes ; les traces contiennent par ailleurs des identifiants de processus et ports éphémères non déterministes, sans incidence sur les verdicts.

Le race detector et l'exécution multi-OS sont couverts par la CI `coord-exp-ci.yml` (locale : NON_EXÉCUTÉ par absence de gcc dans le bac à sable de développement).

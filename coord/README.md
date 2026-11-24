# COORD — protocole de dialogue Mistral ⇄ OpenAI

COORD est le logiciel qui fait dialoguer deux agents IA de laboratoires différents — Mistral (exécutant) et OpenAI (évaluateur indépendant) — autour d'un même dépôt GitHub, sans jamais qu'une IA valide son propre travail.

- Modules canoniques Python 3 (stdlib uniquement) : `coord06.py` (v1.0/v1.1), `coord06_v12.py` (v1.2 candidate), `coord06_v13.py` (v1.3, en vigueur), `coord06_v14.py` (v1.4 candidate, non activée).
- Batteries de tests : `tests_coord06.py` (24), `tests_coord06_v11.py` (13), `tests_coord06_v12.py` (11), `tests_coord06_v13.py` (13), `tests_coord06_robustness.py` (10), `tests_coord06_v14.py` (17).
- Spécification de référence publique : `docs/proto-coord06-v14.md`.

## Rôles

| Rôle | Qui | Devoir |
| --- | --- | --- |
| Exécutant | Mistral | concevoir, exécuter, prouver, publier |
| Évaluateur indépendant | OpenAI (API) | critiquer et rendre un verdict ancré au SHA |
| Registre public | GitHub | issues, PR, commentaires : la référence qui fait foi |
| Pilote | humain | clôture, fusion, activation uniquement |

## Machine à états (v1.0)

```
OUVERTE → PLAN_CRITIQUE → EN_EXECUTION → RECEPTION_DEMANDEE → DECISION_API → CLOTURE
```

Jamais d'auto-réception, jamais de PASS sans exécution réelle, obsolescence d'un verdict dès que SHA ou empreinte de preuve changent.

## Utiliser le logiciel

Prérequis : Python 3 (aucune dépendance).

```sh
cd coord
sh run.sh            # batteries v1.0 + v1.1 : 37/37 PASS
sh run_v12.sh        # batterie v1.2 candidate : 11/11 PASS
sh run_v13.sh        # batterie v1.3 : 13/13 PASS
sh run_robustness.sh # suite de robustesse T3 : 10/10 PASS
sh run_v14.sh        # batterie v1.4 candidate : 17/17 PASS
```

Total : 88/88 PASS (exit 0 chacun). Ces batteries sont UNITAIRES ou d'INTEGRATION AVEC MOCKS ; les E2E réels sont les missions réelles elles-mêmes.

## Implémenter le protocole dans votre projet

Les états et transitions ci-dessus, le format d'enregistrement (mission, issue, state, protocol, sha_git, proof_fp, model_api, ts_utc, verdict_url, verdict_id, ack) et les conventions de commentaires (`FROM:MISTRAL`, `FROM:CHATGPT-API DECISION`, `TO:MISTRAL`) suffisent, dans n'importe quel langage. Invariants à reproduire : réception indépendante, jamais d'auto-réception, obsolescence par empreinte, idempotence des publications, aucun secret en dépôt. Voir `docs/proto-coord06-v14.md` et `/tools/ai-coordinator` (seconde implémentation, Go).

## État et limites

Version opérationnelle : v1.4 candidate (v1.3 en vigueur). Les suites v1.0..v1.3 restent inchangées ; la v1.4 ajoute la continuité autonome, l'état de reprise, les empreintes automatisées, la réception différentielle et les mesures de coordination. La v1.5 candidate n'est pas publiée ici. Licence : à déterminer.

---
Copyright © 2026 Denis Bouzon · ORCID : https://orcid.org/0009-0007-8894-8902 · Licence à déterminer.

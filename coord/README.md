# COORD — protocole de dialogue Mistral ⇄ OpenAI

COORD est le logiciel qui fait dialoguer deux agents IA de laboratoires différents — Mistral (exécutant) et OpenAI (évaluateur indépendant) — autour d'un même dépôt GitHub, sans jamais qu'une IA valide son propre travail.

- Modules canoniques Python 3 (stdlib uniquement) : `coord06.py` (v1.0/v1.1), `coord06_v12.py` (v1.2), `coord06_v13.py` (v1.3), `coord06_v14.py` (v1.4, opérationnelle), `coord06_v15.py` (v1.5 candidate durcie, non activée).
- Batteries de tests : `tests_coord06.py` (24), `tests_coord06_v11.py` (13), `tests_coord06_v12.py` (11), `tests_coord06_v13.py` (13), `tests_coord06_robustness.py` (10), `tests_coord06_v14.py` (17), `tests_coord06_v15.py` (47).
- Spécification de référence du logiciel : `docs/proto-coord06-v14.md`. Norme ouverte indépendante du langage : `docs/spec/coord-spec-1.0.md` (anglais, RFC 2119), implémentée par le moteur de référence Go `go/` (stdlib uniquement) et le moteur Python v1.5 durci.
- Vecteurs de conformité publics : `docs/conformite-v15/` (22 vecteurs au format `coord-conformite-v1`), schémas d'échange : `schemas/*.schema.json` (JSON Schema 2020-12). Appel à implémentations indépendantes : `CALL-FOR-IMPLEMENTATIONS.md` et https://www.osnias-clearing.com/coord/implementations/.
- **Téléchargement direct** : [coord-v1.0-v1.5.zip](https://www.osnias-clearing.com/coord-v1.0-v1.5.zip) — archive complète du dossier, reconstruite automatiquement à chaque publication. Ancien lien conservé : [coord-v1.0-v1.4.zip](https://www.osnias-clearing.com/coord-v1.0-v1.4.zip) (même contenu).

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
sh run_v12.sh        # batterie v1.2 : 11/11 PASS
sh run_v13.sh        # batterie v1.3 : 13/13 PASS
sh run_robustness.sh # suite de robustesse T3 : 10/10 PASS
sh run_v14.sh        # batterie v1.4 : 17/17 PASS
sh run_v15.sh        # batterie v1.5 candidate durcie : 47/47 PASS
python3 docs/conformite-v15/runner_conformite.py docs/conformite-v15  # 22/22 vecteurs
```

Moteur de référence Go (Go 1.27, stdlib uniquement) :

```sh
cd go
go run ./cmd/vector-runner -dir ../docs/conformite-v15  # 22/22 vecteurs
go run ./cmd/coord-demo                                  # 19 CONFORME / 0 ECART attendus
```

Total Python : 88/88 PASS ; v1.5 : 47/47 PASS ; vecteurs : 22/22 sur les deux moteurs (comparaison croisée conforme). Qualification Go multi-OS (Linux, Windows, macOS) et validation externe des schémas exécutées par CI (`.github/workflows/coord-go-ci.yml`). Ces batteries sont UNITAIRES ou d'INTEGRATION AVEC MOCKS ; les E2E réels sont les missions réelles elles-mêmes.

## Implémenter le protocole dans votre projet

Les états et transitions ci-dessus, le format d'enregistrement (mission, issue, state, protocol, sha_git, proof_fp, model_api, ts_utc, verdict_url, verdict_id, ack) et les conventions de commentaires (`FROM:MISTRAL`, `FROM:CHATGPT-API DECISION`, `TO:MISTRAL`) suffisent, dans n'importe quel langage. Invariants à reproduire : réception indépendante, jamais d'auto-réception, obsolescence par empreinte, idempotence des publications, aucun secret en dépôt. Voir `docs/proto-coord06-v14.md`, la norme `docs/spec/coord-spec-1.0.md` (moteur de référence Go `go/`) et, à titre documentaire, `/tools/ai-coordinator` (infrastructure Go historique, divergence d'états documentée).

## État et limites

Version opérationnelle : v1.4 (référence). Candidatures non activées : v1.5 durcie et COORD Specification 1.0 avec son moteur de référence Go ; l'activation reste réservée au pilote humain. Les suites v1.0..v1.4 restent inchangées ; la v1.5 ajoute les habilitations fail-closed, les verdicts référencés, l'obsolescence par empreinte, la persistance atomique, l'arbitrage de reprise et le contrôle du contexte ; la Specification 1.0 les formalise de façon indépendante du langage. Licence : Apache-2.0 (texte complet dans `LICENSE`).


## Licence

Le logiciel COORD est distribué sous licence [Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0) — texte complet dans [`LICENSE`](LICENSE). Toute redistribution doit conserver les notices de copyright.

---
Copyright © 2026 Denis Bouzon · ORCID : https://orcid.org/0009-0007-8894-8902 · Licence : Apache-2.0.

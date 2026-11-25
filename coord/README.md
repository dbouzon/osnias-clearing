# COORD — protocole de dialogue Mistral ⇄ OpenAI

COORD est le logiciel qui fait dialoguer deux agents IA de laboratoires différents — Mistral (exécutant) et OpenAI (évaluateur indépendant) — autour d'un même dépôt GitHub, sans jamais qu'une IA valide son propre travail.

- Modules canoniques Python 3 (stdlib uniquement) : `coord06.py` (v1.0/v1.1), `coord06_v12.py` (v1.2), `coord06_v13.py` (v1.3), `coord06_v14.py` (v1.4), `coord06_v15.py` (v1.5, version opérationnelle activée).
- Batteries de tests : `tests_coord06.py` (24), `tests_coord06_v11.py` (13), `tests_coord06_v12.py` (11), `tests_coord06_v13.py` (13), `tests_coord06_robustness.py` (10), `tests_coord06_v14.py` (17).
- Spécification de référence publique : `docs/proto-coord06-v15.md` (activée) ; norme découplée : `docs/spec/coord-spec-1.0.md`.
- **Téléchargement direct** : [coord-v1.0-v1.5.zip](https://www.osnias-clearing.com/coord-v1.0-v1.5.zip) — archive complète du dossier, reconstruite automatiquement à chaque publication (lien historique `coord-v1.0-v1.4.zip` maintenu vivant, contenu équivalent).

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

Version opérationnelle : **v1.5, activée le 25 novembre 2026** par décision du pilote (mission M-COORD-ACTIVATION-01, issue #44, SHA de référence `2abdebc943e424a11790dd8a24cd6f43c7d914ee`). Les versions v1.0..v1.4 deviennent historiques : leurs suites et enregistrements restent inchangés et rejouables. La v1.5 ajoute le durcissement des transitions, la voûte budgétaire monétaire, les habilitations serveur et les vecteurs de conformité (47 tests + 22 vecteurs, concordance Python/Go). Relation avec **COORD Specification 1.0** (`docs/spec/coord-spec-1.0.md`) : la norme est découplée des versions d'implémentation ; v1.5 en est l'implémentation conforme de référence (concordance 22/22 vecteurs, matrice dans `docs/qualification-m-coord-activation-01.md`). Licence : Apache-2.0 (texte complet dans `LICENSE`).


## Licence

Le logiciel COORD est distribué sous licence [Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0) — texte complet dans [`LICENSE`](LICENSE). Toute redistribution doit conserver les notices de copyright.

---
Copyright © 2026 Denis Bouzon · ORCID : https://orcid.org/0009-0007-8894-8902 · Licence : Apache-2.0.

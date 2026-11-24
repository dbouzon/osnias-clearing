# COORD Pilot — dossier de conception (M-COORD-PILOT-01, issue #25)

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Périmètre** : conception et maquette uniquement (décision pilote, issue #25). Aucun
déploiement VPS, aucun achat, aucun appel payant, aucun secret publié. v1.4 reste la
référence opérationnelle ; v1.5 (candidate, non activée) est éprouvée **à titre
expérimental** sur le cycle de cette mission. Fusions et activations réservées au pilote.

## Contenu du dossier

| Fichier | Rôle |
| --- | --- |
| `conception.md` | Dossier de conception : inventaire de l'existant, architecture, arbitrages. Plan accepté par la critique OpenAI (verdict `PLAN_ACCEPTE`, session pont `m-coord-pilot-01`). |
| `maquette/` | Maquette HTML navigable locale, données simulées explicites (8 écrans, FR). `README.md` = guide de lecture ; `test-maquette.py` = contrôles statiques exécutables. |
| `recette-mvp.md` | Proposition de MVP et critères de recette mesurables (R1..R8, S1..S5, T1..T12). |
| `rapport-experimentation-v15.md` | Rapport d'expérimentation du protocole v1.5 sur le cycle de cette mission : transitions, preuves, SHA, corrections, réserves. |

## En résumé

COORD Pilot est la **couche d'orchestration et de pilotage humain** autour du noyau
normatif `coordengine` (COORD Specification 1.0), conçue pour rester dans le module Go
existant par **fichiers nouveaux uniquement** : service HTTP local (loopback), référentiel
multi-missions sur fichiers JSON atomiques, adaptateurs simulés fail-closed, habilitations
fail-closed (N-01..N-05), budgets vérifiés avant tout appel, arrêt humain intercepté au
service (journal `ARRET_PILOTE`, distinct des `ECART` N-20), reprise exclusive par le
dispositif normatif (N-16/N-17). Le pilote humain reste la seule autorité de fermeture
(N-14) et d'arbitrage. Indépendance fournisseurs par design (identifiants d'adaptateurs
opaques, aucun privilège vendeur).

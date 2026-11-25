# Recette MVP — résultats d'exécution (M-COORD-PILOT-02)

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Base** : main `ec7dc4f1` (post-consolidation `8dbcefaf`) + fichiers nouveaux
uniquement de la branche `mission/coord-pilot-02`. **Exécutions REEL** sur code
distribué téléchargé, toolchain Go 1.27.4 linux/amd64 officielle installée en
séance, Python 3 stdlib. **Adaptateurs SIMULÉS** : aucun réseau sortant, aucune
clé, 0 €. Verdicts consignés ci-dessous, extraits complets dans `preuves-mvp/`.

## Moteur, habilitations, budget, arrêt

| Critère | Verdict | Preuve |
| --- | --- | --- |
| R1 transitions hors table fermée refusées avec motif | **OK** | tests `pilot_test.go` + scénario E2E R1.a/R1.b (409, motifs explicites, journalisés) |
| R2 habilitations mal formées → démarrage refusé (N-05) | **OK** | `TestS5EtR2HabilitationMalsaineRefuseDemarrage` |
| R3 dépassement plafond → blocage AVANT tout appel, motif journalisé | **OK** | `budget_test.go`, `TestR3PlafondBloqueAvantAppel`, scénario E2E (409 BUDGET_DEPASSE, état intact, aucun appel) |
| R4 arrêt pilote → transitions refusées, état intact ; levée → reprise normative exclusive (C4 : `REPRISE_ATTENDUE` jusqu'à N-16/N-17) | **OK** | `arret_test.go`, `TestR4ArretPiloteEffectif`, `concurrence_test.go` (C3, C4), scénario E2E R4.1..R4.3 |
| R5 divergence → arbitrage `github_fait_foi` (N-16), reprise conditionnée au head (N-17) | **OK** | `TestR5DivergenceArbitrageReprise`, scénario E2E R5 (refus « sha_head divergent » en cas de mismatch, sha_head attendu obligatoire) |
| R6 verdict sur preuves modifiées → refus (N-13) | **OK** | `TestR6VerdictPerimeRefuse`, scénario E2E (« verdict perime », ACK sur preuves courantes passe) |

## Non-régression (R7)

| Batterie | Verdict |
| --- | --- |
| Python v1.0..v1.4 : 24+13+11+13+17 = 78/78, robustesse 10/10 (= 88/88) | **OK** |
| Python v1.5 durcie : 47/47 | **OK** |
| Vecteurs de conformité Python : 22/22 | **OK** |
| Démo v1.5 : exit 0 | **OK** |
| `go vet ./...`, `go build ./...` | **OK** (aucun diagnostic) |
| `go test ./...` module étendu (11 paquets testés) | **OK** (dont 5 nouveaux : adaptateurs, arret, budget, missions, pilot) |
| Vecteurs Go : 22/22, sortie sha256 `8c097f76…` **byte-identique** à la ligne de base #23 | **OK** |
| Démo Go : 19 CONFORME / 0 ECART, sha256 `e1f1cdbe…` **byte-identique** | **OK** |
| Maquette #25 T1..T12 (artefact intact) | **OK** 12/12 |
| Manifeste d'intégrité des fichiers historiques (vérifié par la batterie v1.5) | **OK**, aucun fichier existant modifié (contrôle sha256 main.zip à la livraison initiale ⇒ 0 modifié, 0 supprimé, 20 fichiers présents dans l'arbre de travail et absents de main, le présent fichier de recette et le rapport de mission étant les deux derniers ajoutés ; périmètre inchangé par la révision corrective, voir plus bas) |

## Sécurité locale (S1..S5, REEL, prix nul)

| Critère | Verdict |
| --- | --- |
| S1 sans token → 401 journalisé, aucune transition | **OK** |
| S2 token invalide/périmé → 401, aucune transition | **OK** |
| S3 écoute non loopback → démarrage refusé (0.0.0.0, LAN, :8080 testés) ; token vide refusé | **OK** |
| S4 rôle PILOTE non dérivable du corps (champ `role` ignoré, acteur résolu côté serveur, clôture forgée refusée) | **OK** |
| S5 habilitations vides/mal formées → démarrage refusé, motif journalisé | **OK** |

## Interface

| Critère | Verdict |
| --- | --- |
| T1..T12 contrôles statiques maquette | **OK** (artefact #25 intact, 12/12) |
| T13 aucune ressource externe (CSP self) | **OK** |
| T14 bandeau ADAPTATEURS SIMULÉS permanent + `<title>` | **OK** |
| T15 appels fetch typés `/api/*` et `/health` uniquement | **OK** |
| T16 guide Prise en main complet (6 étapes, rôles, N-16/N-17) | **OK** |
| E2E1..E2E6 scénario complet sur le vrai service (binaire + loopback + journal) | **OK** 25/25 |
| Rendu visuel complet dans un navigateur du poste pilote | **NON_EXECUTE** (aucun navigateur dans le sandbox de la mission ; contrôles dynamiques REEL via HTTP, rendu à la discrétion du pilote local) |

## Empreintes des preuves (REEL, révision corrigée)

- `preuves-mvp/trace-recette-python.txt` sha256 `9849bd09984e4ef778b860708f9620d839995614975cfc60b39fa3f3fc21d682`
- `preuves-mvp/trace-recette-go.txt` sha256 `0368685bde1618267e352b69d20a22bb60d535f8e75f4d3e82797a739a20dd21`
- `preuves-mvp/trace-scenario-e2e.txt` sha256 `d47ef8c8c7f3cdb3e973f354d20120aaade9ab46e80ba3287d38170fa511137c` (BILAN SCENARIO E2E: OK=25 KO=0, byte-identique à la révision initiale)
- `preuves-mvp/trace-reproductions.txt` sha256 `f41627b48456352dd8e2d670e7f077c82dcd6fb6da46a7a4d57ba23e638f804f` (reproductions C1..C6 AVANT correction + invariants verrouillés APRÈS)

## Réception complémentaire et corrections C1..C6 (issue #27, PR #28)

Verdict A_CORRIGER rendu sur `c4c4b4c6` par révision statique (commentaires 7147798524 et 7147799137) : il remplace, pour ce SHA, le verdict favorable du premier tour conformément à H3/N-13. Chaque anomalie a été REPRODUITE expérimentalement avant correction, corrigée, puis verrouillée par un test d'invariant permanent (`coord/go/internal/pilot/concurrence_test.go`) :

| Anomalie | Reproduction avant correction | Correction | Verdict |
| --- | --- | --- | --- |
| C1 : Obtenir puis Sauver séparés, mises à jour perdues (plafond violé sous concurrence) | 13 appels passés / 2 comptabilisés | `Referentiel.Transaction` : verrou de mission détenu du chargement à la sauvegarde, budget, transition, arrêt et reprise inclus | **OK** |
| C2 : appel d'adaptateur AVANT validation normative (consommation pour transition interdite, acteur non autorisé) | 1 appel passé + jetons consommés pour une transition de table interdite | validation sur COPIE INDÉPENDANTE via `missions.Copie` avant tout appel ; refus = AUCUN appel, AUCUNE consommation | **OK** |
| C3 : arrêt humain écrasé par une transition concurrente | arrêt du pilote effacé (orage de requêtes) | l'arrêt est contrôlé DANS la transaction sérialisée : ordre linéarisable, arrêt durable | **OK** |
| C4 : levée d'arrêt sans reprise normative préalable | transition acceptée directement après levée (200) | indicateur `RepriseAttendue` : toute transition est refusée (`REPRISE_ATTENDUE`, 409) jusqu'à la reprise N-16/N-17, seule voie de déblocage (conforme à la conception : « à la levée de l'arrêt, la reprise passe exclusivement par le dispositif normatif ») | **OK** |
| C5 : erreurs de journal ignorées (masquées) | refus d'acteur répondu 403 alors que le journal était indisponible | toute erreur de journal est propagée : réponse typée `ERREUR_JOURNAL` (500), état NON persisté pour une mutation non journalisée ; défaillance post-persistance explicite dans le motif | **OK** |
| C6 : collisions de nomDir (identifiants distincts vers un même dossier) | « a b » et « a-b » collisionnaient | codage INJECTIF octet par octet (échappement `~xx` hexadécimal) ; parcours par dossier encodé sans ré-encodage | **OK** |

Incidence de la correction C6 sur les chemins : l'identifiant `#x` correspond désormais au dossier `~23x` (échappement de `#`), ce qui rend impossibles les collisions ; les tests concernés ont été mis à jour.

Périmètre de la révision corrective : par rapport à la livraison initiale, elle ne modifie que des fichiers introduits par la présente mission (code Go du MVP, tests, preuves, recette, rapport) plus un fichier de qualification (`.github/workflows/coord-go-ci.yml`, ajout du job `Race detector`) ; aucun fichier du noyau historique couvert par le manifeste d'intégrité (ligne « Non-régression » ci-dessus) n'est touché, ce que confirment les empreintes byte-identiques des vecteurs et de la démo Go.

Le race detector de Go n'est pas exécutable dans le sandbox (compilateur C absent) : **NON_EXECUTE local** ; un job CI dédié `Race detector` (`go test -race -count=2 ./internal/pilot/ ./internal/missions/`) est ajouté au workflow de qualification.


## NON_EXECUTE / SIMULE (réserves explicites)

- Adaptateurs SIMULÉS : aucune interopérabilité réelle n'est revendiquée.
- Rendu navigateur complet : NON_EXECUTE dans le sandbox (documenté ci-dessus).
- Go Windows/macOS : NON_EXECUTE en séance ; couverts par la CI existante
  (COORD qualification Go multi-OS verte à `2a19993`, non dépendante de cette PR) ;
  à confirmer par la CI sur la branche.
- Race detector : NON_EXECUTE en local (compilateur C absent), couvert par le
  job CI dédié ajouté au workflow.
- Aucun appel A2A/MCP : l'analyse `docs/analyse-a2a.md` est documentaire (REEL sur
  sources publiques, sans dépendance de code).

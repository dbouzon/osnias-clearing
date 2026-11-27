# Identités d'opération et rejeu explicite (M-COORD-FLOW-01)

Copyright © 2026 Denis Bouzon
ORCID : https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier : Apache-2.0

## 1. Identité d'opération

Chaque appel à un intervenant (critique, exécution, soumission, réception) est une **opération** identifiée par un `id_operation` aléatoire (16 octets crypto/rand, hex), propre à l'opération et indépendant du contenu envoyé : deux volontaires au contenu identique produisent deux identités distinctes.

- L'identité est persistée **avant** l'appel dans `appel_en_cours.json` (champ additif `id_operation,omitempty`) et dans le registre append-only `operations.jsonl` déposé à côté du journal de mission.
- Toutes les lignes de journal d'appel (APPEL_RESERVE, APPEL_REEL, APPEL_ECHOUE, RECEPTION, RESOLUTION_APPEL) portent le champ `id_operation` : un journal se lit par opération, un registre se lit par mission.

Événements du registre : `OPERATION_OUVERTE`, `OPERATION_TERMINEE`, `OPERATION_ECHOUEE`, `OPERATION_INTERROMPUE`, `OPERATION_RESOLUE`, `OPERATION_REJETEE` (informationnel : un refus de rejeu ne change jamais l'état d'une opération).

## 2. Contrat de rejeu

Le rejeu est **explicite** : `AvecRejeu(<id|"derniere">)` côté orchestrateur, flag `-rejeu` côté CLI. Contrat validé **avant tout appel** (fail closed) :

| État de l'opération cible | Résultat du rejeu |
|---|---|
| TERMINEE | `OPERATION_DEJA_TERMINEE` : zéro appel, zéro budget, aucun effet |
| ECHOUEE, RESOLU_PERDU, RESOLU_ECARTE, RESOLU_ECHOUE | relance avec le MÊME `id_operation`, clé de tentative NEUVE |
| EN_COURS | `ErrAppelEnCours` : un appel est déjà réservé |
| INTERROMPUE, RESOLU_RECU | `OPERATION_NON_REJOUABLE` : une NOUVELLE opération volontaire est requise |
| inconnue | `OPERATION_INCONNUE` |
| action/charge/acteur divergents | `OPERATION_INCOMPATIBLE` : refus avant tout appel |

Tout refus est journalisé (`OPERATION_REJETEE`, informationnel) et reste traçable sans rien changer.

## 3. Reprise après arrêt brutal

Un état ancien **sans identité** reste lisible (compatibilité ascendante) mais ne déclenche jamais de nouvel appel automatique si un résultat antérieur est incertain : le moteur bloque par `APPEL_EN_COURS_NON_RESOLU` et l'arbitrage appartient au pilote.

Procédures D7 (décision pilote, jamais automatique) :

- **a) résultat certain absent** : l'appel n'a pas abouti côté intervenant. Le pilote peut rejouer l'opération (`-rejeu <id>` : la clé de tentative est neuve, l'identité conservée) ou l'abandonner (`-resolution PERDU|ECARTE|ECHOUE`).
- **b) résultat incertain** (processus tué pendant l'appel) : la réponse a pu aboutir. Le pilote arbitre : `RECU` si la réponse est effectivement arrivée (l'opération devient NON rejouable), sinon `PERDU`/`ECARTE`/`ECHOUE` (l'opération devient rejouable).

## 4. Intégrité du registre (fail closed)

Le registre est relu **à chaque** préparation d'appel : un `operations.jsonl` corrompu bloque toute nouvelle opération (`OPERATIONS_ILLISIBLES`) ; un registre absent est traité comme vide (missions anciennes compatibles, aucun blocage de lecture). L'écriture `OPERATION_OUVERTE` a lieu dans la transaction TX1 (après réservations) : son échec annule l'appel ; les décisions (TERMINEE/INTERROMPUE) sont écrites dans TX2 avant commit.

## 5. Retrouver les opérations terminées

`coord-mission -consulter <mission>` expose le comptage par état (`terminées`, `échouées`, `interrompues`, `en cours`) sur STDERR (stdout reste du JSON pur). `ResumeOperations(racine, issue)` fournit la même agrégation à l'API.

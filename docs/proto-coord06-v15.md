<!-- Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# Protocole COORD v1.5 (CANDIDATURE, NON ACTIVEE) — spécification consolidée autonome

Mission : M-COORD-HARDENING-01, issue #19 du dépôt `dbouzon/osnias-clearing`. Branche de candidature : `mission/coord-hardening-01`. Auteure : Mistral (Vibe), plan v2 accepté par critique préalable OpenAI (verdict PLAN_ACCEPTE, session m-coord-hardening-01). **Aucune fusion vers main et aucune activation sans décision expresse du pilote.**

## 0. Statut de ce document

Ce document est **autonome** : il contient l'intégralité des règles normatives du protocole COORD applicables à une mission v1.5 consolidée ; il ne renvoie à aucun document v1.0–v1.4 pour ses définitions normatives. Les versions antérieures restent en vigueur pour leurs propres enregistrements (§7, coexistence).

Normalisation des statuts de version (corrigeant la mention historique obsolète) :
- **OFFICIELLE** : version activée par décision expresse du pilote ; à la date de ce document, **v1.4 est la version OFFICIELLE de référence opérationnelle** ;
- **CANDIDATURE** : version spécifiée, implémentée, testée, réceptionnée mais non activée ; **v1.5 (ce document) est une CANDIDATURE**.

Les constantes d'enregistrement des versions antérieures (`PROTOCOL_VERSION*`) demeurent inchangées dans le code distribué : elles identifient les enregistrements historiques, pas le statut opérationnel (défaut de mention documenté, constat complémentaire de l'issue #19).

## 1. Objet, acteurs et vocabulaire

COORD régit la conduite d'une mission exécutée par un agent et réceptionnée par un agent indépendant, sous l'autorité d'un pilote humain.

Trois rôles normatifs, aucun fournisseur d'IA codé en dur :
- **EXECUTANT** : conçoit, exécute, soumet, accuse réception d'un verdict ;
- **RECEPTIONNAIRE** : rend les verdicts de réception ; il est **toujours distinct de l'EXECUTANT de la même mission** ;
- **PILOTE** : autorité humaine ; seule identité habilitée à clôturer ; seule autorité d'activation, de fusion et de publication.

Étapes canoniques d'une mission : `critique` (examen préalable d'un plan ou d'une conception), `execution`, `reception`, `ack`, `cloture`.

Une **identité** est un couple (identifiant stable, rôle) qualifié par un **adaptateur** à la frontière du système (canal de transport vérifié : dépôt, API, jeton...). La règle de séparation des responsabilités :
- l'adaptateur, et lui seul, qualifie l'identité (authentification au sens fort) ;
- **le moteur n'authentifie jamais** : il vérifie structurellement que chaque opération critique porte une identité dont l'adaptateur d'émission est *enregistré dans la configuration*, et refuse toute identité « déclarée sauvagement » (non liée à un adaptateur enregistré) ;
- en conséquence, une identité déclarée hors adaptateur enregistré n'est **jamais présentée comme authentifiée** ; cette séparation est vérifiée par des vecteurs de conformité (cf. §6).

## 2. Habilitations (règle H1, fail closed)

La configuration décrit les adaptateurs enregistrés (`adaptateurs`) et les actions autorisées par rôle (`roles`). Les actions normalisées sont : `executer`, `soumettre`, `recevoir`, `acker`, `cloturer`.

Règles de validation de la configuration (toute violation = configuration invalide, toutes les opérations sont refusées) :
- la liste d'adaptateurs est une liste non vide d'identifiants uniques ;
- les rôles sont connus, les actions connues, **aucun joker**, **aucun doublon** ;
- un rôle sans action est légal (aucun droit) ; une configuration vide de rôles signifie : personne n'est habilité.

Habilitation d'une opération : identité qualifiée par un adaptateur enregistré **ET** action autorisée pour le rôle de l'identité. Toute erreur de configuration ferme tout.

## 3. Machine d'états durcie (règle H2)

États : `PLANIFIEE`, `EXECUTION`, `DECISION_ATTENDUE`, `ACCEPTEE`, `EN_CORRECTION`, `ACK_RECU`, `REFUSEE` (terminal), `CLOTUREE` (terminal).

Table exhaustive et fermée, toute paire absente est interdite :

| État de départ | Cible | Rôle requis | Action | Données obligatoires |
|---|---|---|---|---|
| PLANIFIEE | EXECUTION | EXECUTANT | executer | (liaison stable de l'identité exécutante) |
| EXECUTION | DECISION_ATTENDUE | EXECUTANT | soumettre | `revision` (40 hex), `empreinte_preuves` (sha256, 64 hex) |
| DECISION_ATTENDUE | ACCEPTEE | RECEPTIONNAIRE | recevoir | `verdict`="ACCEPTEE", `verdict_id`, `verdict_url` |
| DECISION_ATTENDUE | REFUSEE | RECEPTIONNAIRE | recevoir | `verdict`="REFUSEE", `verdict_id`, `verdict_url` |
| DECISION_ATTENDUE | EN_CORRECTION | RECEPTIONNAIRE | recevoir | `verdict`="A_CORRIGER", `verdict_id`, `verdict_url` |
| EN_CORRECTION | EXECUTION | EXECUTANT | executer | (identité exécutante stable) |
| ACCEPTEE | ACK_RECU | EXECUTANT | acker | `revision_courante`, `empreinte_courante` (H3) |
| ACK_RECU | CLOTUREE | PILOTE | cloturer | — |

Verdicts normalisés : `ACCEPTEE`, `A_CORRIGER`, `REFUSEE`.

Règles invariantes :
- **R1 (anti auto-réception)** : toute transition de verdict exige une identité dont l'identifiant diffère de celui de l'EXECUTANT lié à la mission ; refus sinon, avec trace ;
- **R2 (référence obligatoire)** : aucun verdict (y compris REFUSEE) n'est accepté sans `verdict_id` et `verdict_url` non vides ; le verdict est ajouté au journal des décisions avec l'empreinte et la révision soumises, l'identité du récepteur et sa référence ;
- **R3 (états non ambigus)** : après le verdict d'acceptation, la mission est dans l'état `ACCEPTEE` (décision rendue), distinct de `DECISION_ATTENDUE` ; aucun retour à `EXECUTION` après acceptation (plus de rollback silencieux) ;
- **R4 (terminaux)** : depuis `REFUSEE` et `CLOTUREE`, aucune transition ;
- **R5 (exécutant stable)** : seule l'identité exécutante liée à la mission relance une correction.

## 4. Obsolescence par empreinte et révision (règle H3)

L'obsolescence d'une décision est calculée **par l'empreinte sha256 des preuves et par la révision (40 hex)** présentées au moment de la décision — **jamais par un compteur de preuves**. L'ACK (`ACCEPTEE → ACK_RECU`) est refusé si `empreinte_courante` ou `revision_courante` divergent de celles de la dernière décision, avec motif `verdict perime`. Fonction normative : `verdict_perime(decision, empreinte_courante, revision_courante)`.

La réception différentielle des versions antérieures (conservation des critères dont les fichiers sont inchangés, réexamen explicite après correction) demeure la règle de conduite des réceptions multi-rounds ; son format d'ancre est formalisé aux §7 (formats d'enregistrement).

## 5. Persistance atomique et durabilité qualifiée (règle H4)

Toute écriture d'état suit la séquence : (1) calcul complet de la valeur **avant toute ouverture de fichier** (un enregistrement non sérialisable ne touche rien) ; (2) écriture complète d'un fichier temporaire + fsync ; (3) renommage atomique ; (4) fsync du répertoire.

Niveaux de garantie normalisés (avec pannes injectées aux points d'arrêt `ecriture`, `avant_remplacement`, `apres_remplacement`) :
- crash pendant l'écriture : le contenu **précédent** est intact ; le temporaire orphelin est purgé à la relecture ;
- crash avant le renommage : contenu précédent intact, temporaire complet purgable ;
- crash après le renommage : le nouveau contenu complet est en place.

Limites explicites : atomicité garantie sur un système de fichiers local (sémantique POSIX `rename`), **pas de réplication, pas de cohérence multi-machines** ; la fsync répertoire est best effort (plateformes sans fsync de répertoire).

## 6. Reprise déterministe et arbitrage des supports (règle H5) + conformité vérifiable

Chaque mission publie un enregistrement de reprise normalisé sur deux supports durables : stockage local persistant (« knowledge ») et dépôt public (« github »). En cas de divergence, **le support dépôt public fait foi**.

Arbitrage : `arbitrer_reprise(record_knowledge, record_github, ts_utc)` retourne l'élu, la règle (`github_fait_foi`), la liste des champs divergents, les empreintes sha256 des **deux versions intégrales** (post-mortem reconstituable). Chaque arbitrage est journalisé (JSONL canonique, idempotent). La reprise n'exécute la prochaine action qu'après vérification de l'état du monde (`sha_head` attendu) ; un SHA divergent refuse la reprise sans ré-exécution.

**Vecteurs de conformité ouverts** (`docs/conformite-v15/`) : fichiers JSON, positifs et négatifs, décrivant scénarios, données et résultats attendus (format documenté `coord-conformite-v1`). Ils couvrent la machine (3 vecteurs positifs, 9 négatifs), les habilitations (8), l'arbitrage de reprise (2) — 22 au total. **Toute implémentation indépendante du protocole, quel que soit son langage, doit produire exactement ces résultats.** Le lanceur de référence (`runner_conformite.py`) admet en argument un moteur alternatif ; un manifeste d'intégrité des fichiers historiques (`manifeste-historique-main.json`) verrouille la non-régression par empreintes de blobs.

## 7. Enregistrements et coexistence des versions

La chaîne d'enregistrement v1.0 → v1.5 est cumulative par délegation stricte : un enregistrement v1.5 porte les champs des enregistrements v1.1–v1.4 (identité de mission, issue, état, version de protocole, SHA Git, empreinte des preuves, modèle, horodatage, référence de verdict, ACK ; résumé de reprise, faisabilité, ancre différentielle, mesures, continuité) **plus** deux blocs propres :
- `durcissement` : `executant`, `receptionnaires`, `regle_arbitrage` = `github_fait_foi`, `empreinte_decision` ;
- `controle_contexte` (C7–C9 ci-dessous).

Validation v1.5 (fail closed) : validation v1.4 par délégation sur copie de version v1.4, puis : identité exécutante non vide ; récepteurs listés ; règle d'arbitrage exacte ; **référence de verdict obligatoire pour tout état de décision, en particulier REFUSEE** ; empreinte de décision requis pour un ACK ; bloc de contrôle de contexte complet.

Coexistence (règle A-1, arbitrée) : les enregistrements v1.1–v1.4 restent validés par leurs validateurs propres ; la machine durcie est exigée des **seuls** enregistrements et missions v1.5 ; aucune rétro-validation. Un **outil de migration optionnel et non destructif** (rejouer les vérifications v1.5 sur un enregistrement historique et produire un rapport de conformité) est prévu en mission ultérieure si le pilote le demande.

## 8. Cohérence du contexte avant exécution (règles C7–C9, consolidation de la candidature issue #160)

- **C7** : chaque instruction reçue est qualifiée par cohérence avec les termes de l'ordre de mission publié et par sa provenance déclarée ; sans provenance déclarée ou sous le seuil de cohérence, elle est `ECART_ISOLE` et **ne peut jamais produire de décision, publication ou modification** ;
- **C8** : tout écart est journalisé (JSONL canonique idempotent : horodatage UTC, empreinte calculée de l'instruction, qualification, source alléguée, décision, étape) — aucune transcription manuelle ;
- **C9** : l'exécution reprend automatiquement à l'étape concernée, aucune interruption systématique, aucune décision issue d'un écart.

## 9. Exigences normatives et matrice de couverture

Chaque exigence est couverte par au moins un test positif et un test négatif de la batterie v1.5 (47 tests) et/ou un vecteur de conformité.

| Exigence | Énoncé (résumé) | Tests batterie | Vecteurs |
|---|---|---|---|
| N-01 | identités qualifiées par adaptateurs enregistrés, moteur n'authentifie jamais | H1i, H1j | V-NEG-5 |
| N-02 | habilitations configurables fail closed (joker/doublon/absence = refus) | H1a–H1k | V-HAB-1..8 |
| N-03 | séparation exécutant/réceptionnaire, auto-réception interdite | H3a | V-NEG-1 |
| N-04 | table de transitions exhaustive et fermée | H2g, H2h | V-NEG-3 |
| N-05 | référence de verdict obligatoire (y compris REFUSEE) | H2d, H2e, Z2 | V-NEG-2 |
| N-06 | états non ambigus, pas de rollback post-acceptation | H2b, H2c | — |
| N-06b | terminaux: aucune transition | H2f | V-NEG-8 |
| N-07 | exécutant stable entre rounds | H2g (paire), vecteur | V-NEG-7 |
| N-08 | obsolescence par empreinte/révision, jamais par compteur | H3c–H3f | V-NEG-6 |
| N-09 | persistance atomique, échec sans perte (3 points d'arrêt) | H4a–H4e | — |
| N-10 | arbitrage des supports, github fait foi, journal post-mortem | H5a–H5e | V-REP-1, V-REP-2 |
| N-11 | reprise déterministe après vérification SHA | H5f, H5g | — |
| N-12 | qualification du contexte, écarts isolés, aucune décision issue d'un écart | C7a, C8, C9 | — |
| N-13 | enregistrement v1.5 par délégation stricte, validation durcie | Z1–Z4 | — |
| N-14 | non-régression des fichiers historiques (manifeste recalculé) | M1 | — |
| N-15 | aucun secret dans les sources | S7 | — |

## 10. Séparation protocole / moteur / adaptateurs

- **Protocole** (ce document) : règles normatives, indépendantes du langage ;
- **Moteur** (`coord06_v15.py`, Python stdlib) : implémentation de référence ; aucune dépendance à GitHub, OpenAI, Mistral ou à un réseau ; aucun fournisseur codé en dur ;
- **Adaptateurs** : qualification des identités à la frontière (dépôt, API, pont) ; relais de messages ; hors périmètre du moteur. L'authentification réelle est une responsabilité d'adaptateur.

## 11. Compatibilité avec le démon Go (règle H6, sans réécriture)

`tools/ai-coordinator` (Go) est une **infrastructure** (démon webhook + SQLite + verrous d'exclusivité), pas un moteur du protocole COORD ; il n'est ni modifié ni réexigé par la présente candidature. Correspondance indicative des états :

| COORD v1.5 | Go (`statemachine.go`) | Note |
|---|---|---|
| PLANIFIEE | open | mission enregistrée, non démarrée |
| EXECUTION | running | agent au travail |
| DECISION_ATTENDUE | review | livraison soumise, examen |
| EN_CORRECTION | correction | corrections demandées |
| ACCEPTEE | awaiting_approval | réception favorable, validation humaine en attente |
| ACK_RECU → CLOTUREE | accepted | validation humaine / clôture |
| REFUSEE / incidents | blocked | terminaux distincts assumés |

Différences assumées documentées : plafond de corrections, verrou d'exclusivité, validation humaine intermédiaire côté Go. `go test` est qualifié **NON_EXECUTE** dans cette mission (toolchain Go absente de l'environnement d'exécution) ; l'analyse du démon est statique.

## 12. Limites explicites

- Authentification : **à la frontière des adaptateurs** ; le moteur n'authentifie pas et les identités simulées (démonstration `run_demo_v15.sh`) ne prouvent **aucune sécurité de bout en bout**, seulement la conformité du moteur aux règles H1–H6 ;
- Durabilité : fsync local mono-machine (§5), pas de réplication ;
- Réception différentielle multi-rounds : conduite documentée, outillage ultérieur ;
- Interopérabilité MCP/A2A : volontairement hors périmètre (mission ultérieure) ;
- La batterie historique v1.0 embarque une copie locale de sa machine (divergence documentée, constat C7 de l'issue #19) ; elle n'est pas modifiée (interdit de toucher aux versions historiques) ; la couverture du code distribué est apportée par la batterie v1.5 (47 tests) sur modules importés réels.

## 13. Références de mise en œuvre

- Module : `coord06_v15.py` ; batterie : `tests_coord06_v15.py` (47 tests) ; lanceur : `run_v15.sh` ;
- Vecteurs : `docs/conformite-v15/` (4 fichiers, 22 vecteurs) ; lanceur : `runner_conformite.py` ; manifeste : `manifeste-historique-main.json` ;
- Démonstration : `demo/demo_v15.py`, lanceur `run_demo_v15.sh` (18 conformités, exit 0 contractuel) ;
- Licence : Apache-2.0 ; mentions « Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 » en tête de chaque source propre à COORD.

Fin de la spécification.

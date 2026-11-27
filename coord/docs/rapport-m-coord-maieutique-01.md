# Rapport M-COORD-MAIEUTIQUE-01 — Module maïeutique : de l'intention ambiguë à l'ordre transmis

Mission M-COORD-MAIEUTIQUE-01 (issue #42, dbouzon/osnias-clearing, ordre v1.4). Le module maïeutique fait passer COORD d'une intention déclarée par le pilote à un ordre de mission qualifié, validé, publié puis transmis à une mission fille, sans jamais qu'un texte de modèle ne vaille décision. Directeur qualifié (OpenAI ou profil interchangeable), exécutant et réceptionnaire distincts, pilote Denis Bouzon seul détenteur des transitions décisives (validation, clôture). Plan v2 PLAN_ACCEPTE (empreinte `e54eb7a1…`), mandat sha256 `1cab85cd…`. Budget 0 €, serveurs factices loopback uniquement, aucune clé, aucun écouteur public. Base : main `f6852813`. Branche : `mission/coord-maieutique-01`.

## 1. Ce qui a été livré

| Livrable | Contenu |
|---|---|
| `coord/go/internal/maieutique/` | machine à états INTENTION→DIALOGUE→ORDRE_PROPOSE→ORDRE_QUALIFIE→VALIDE→PUBLIE→TRANSMIS (+ ATTENTE_PILOTE/INVALIDE/ABANDONNE), ordre à dix champs, validation déterministe, sources, documents encadrés, dialogue binômé, journal D10 |
| `coord/go/cmd/coord-maieutique/` | service REST loopback (jeton pilote, ~26 routes), harnais de recette de la mission fille, kit de recette témoin (4 scripts Python) |
| `coord/schemas/ordre.schema.json` | schéma JSON 2020-12 de l'ordre (dix champs requis) |
| `.github/workflows/coord-maieutique-ci.yml` | CI : gofmt/vet/build/test sur ubuntu/windows/macos, race detector, non-régression du moteur (fichiers nouveaux uniquement) |
| `coord/go/internal/maieutique/scenarios_test.go` | banc T5 : scénarios S1 à S16 + interchangeabilité (critère 5), mesures RAM/CPU/latences/appels/contexte (rapport JSON via `COORD_SCENARIOS_RAPPORT`) |

## 2. Contrat démontré (D2-D10)

- Intention ambiguë → QUESTION/CLARIFICATION, jamais un ordre ; l'ordre ne sort du dialogue qu'après validation déterministe PUIS critique sémantique obligatoire du directeur.
- Rejets déterministes à un seul appel (OBJECTIF_NON_UNIQUE, SOURCES_VIDES, LIVRABLE_IMPRECIS, ETAPES_INCOHERENTES) : aucun second appel, aucun coût.
- Critique BLOQUANTE → retour au dialogue (422 CRITIQUE_BLOQUANTE) ; échecs directeur ×2 → ATTENTE_PILOTE, seules issues licites : relancer/rectifier/abandonner.
- Validation explicite du pilote par jeton (jamais journalisé) ; qualification fige la version et l'empreinte sha256 JCS.
- Idempotence stricte : publication et transmission = exactement un effet, puis 409 OPERATION_DEJA_EFFECTUEE ; reprise après SIGKILL réel sans double publication, sans double transmission, sans second appel.
- Les trois empreintes (validée, publiée, transmise) sont identiques au fichier figé et au livrable (sha256 vérifiés).
- Devis dérivé de l'ordre (jamais saisi à la main), accord consigné `paiement_reel:false` ; clôture TOUJOURS refusée à l'automatisme (mère et fille finissent à ACK_RECU).

## 3. Vérifications

- Tests Go : module complet vert (16 paquets), gofmt/vet propres, `GOOS=windows/darwin` vet OK, race detector vert en CI.
- CI GitHub : 5/5 jobs verts à chaque état (3 OS, race, non-régression moteur depuis `f6852813`).
- T3 OpenAI (session pont `m-coord-maieutique-01`) : verdict FAVORABLE après retracage des rejets.
- T5 banc 17/17 PASS en sandbox ET sur VPS (S1-S16 + interchangeabilité des profils directeurs, critère 5) ; mesures VPS : 2 481 ms tout compris, RSS max 14 Mo, 28 appels directeur cumulés.
- Recette témoin VPS (compte `coord`, loopback seul, kit épinglé au sommet de la branche) :
  - run A (profil openai, 3 SIGKILL réels : dialogue, publication, transmission) : **38/38 preuves**, reprise propre à chaque phase ;
  - run B (profil anthropic, sans SIGKILL, ports distincts) : **34/34 preuves** ;
  - même empreinte d'ordre figé `6b3ddb905f60268e53310e693bd51995c37f7faba163b3d343006645a83f3469` sur les deux runs et sur le sandbox : fournisseurs interchangeables démontré ;
  - mesures : service VmHWM 7,8-9,3 Mo, CPU ≤ 0,04 s, latences REST ≤ 60 ms — cible VPS 1 vCPU / 2 Go satisfaite avec deux ordres de grandeur de marge.

## 4. Décisions, incidents et limites

- Aucun fichier du moteur existant modifié (D1 fail-closed vérifié par diff `f6852813…HEAD` : périmètres nouveaux uniquement).
- Incident : réinitialisation du sandbox a détruit l'arbre de travail avant tout push ; T2 intégralement reconstruit, puis un écart de transcription du pilote de recette a été détecté et corrigé (poussé vérifié byte-exact contre la version éprouvée).
- Bug de production évité : rôles de la transmission exprimés en actions normatives (EXECUTANT{executer,soumettre,acker}, RECEPTIONNAIRE{recevoir}, PILOTE{cloturer}) au lieu d'identifiants (commit `5cb4951f` + test de régression).
- Clôture de mission réservée au pilote humain en toutes circonstances ; la mission fille témoin MAIEUTIQUE-TEMOIN-01 reste à ACK_RECU.
- Mode SIMULÉ uniquement : aucune clé, aucun paiement, aucun appel réseau sortant du VPS.

## 5. Traçabilité

Issue #42 (ordre v1.4). Plan v1 A_CORRIGER ; plan v2 PLAN_ACCEPTE, empreinte `e54eb7a1…`. Commits (18) : `1d87302c` module, `8a107cf6` documents+dialogue, `c77fe2d5` actions+opérations, `67f5b322` directeur factice+tests noyau, `c3be9150` tests documents/dialogue/opérations, `7ece8a3d` service, `92b544ce` tests service, `6cc1d3a7` schéma+CI, `fdc8dc27`/`71455bf8` CI Windows, `c0b5d67d` verrou T4, `63932fc1` banc T5, `5cb4951f` rôles transmission, `2111de7a` harnais fille, `a12feb5e`/`a851944d` kit recette, `8e17be54`/`649cd86d` alignement pilote de recette, puis rapport et consigne de réception. Sessions pont OpenAI `m-coord-maieutique-01` : T3 FAVORABLE ; réception finale T6 ACCEPTEE sans réserve majeure (verdict_id `m-coord-maieutique-01-reception-t6`), servie à la mission mère par le factice de verdicts et consignée sur l'issue #42. Recette VPS : `/srv/coord/private/recette/m-coord-maieutique-01/` (rapports `run-a`, `run-b`), purge documentée après recette.

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

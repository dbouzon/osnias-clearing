# Rapport M-COORD-FLOW-01 — Identité durable des opérations et recette reproductible

Ordre du 27/11/2026 (issue #76, dbouzon/osnias-clearing) : identité durable des opérations, reprise sans doublon, journaux corrélés, recette VPS reproductible. Exécutant Mistral, directeur/réceptionnaire OpenAI, pilote Denis Bouzon. Ajustements exprès du pilote intégrés : (1) mission+action+révision ne suffisent pas à identifier une opération ; (2) un état ancien sans identité est lisible mais ne déclenche jamais de nouvel appel automatique si un résultat antérieur est incertain ; (3) préciser comment retrouver les opérations terminées. Budget 0 €, factices loopback uniquement, aucune clé. Base : main `de1ef681`. Branche : `mission/coord-flow-01` (tête `fd420507306f4cb229535bfb7489995d49405105`).

## 1. Ce qui a été livré

| Livrable | Contenu |
|---|---|
| `internal/connecteurs/operations.go` | registre append-only `operations.jsonl`, événements OUVERTE/TERMINEE/ECHOUEE/INTERROMPUE/RESOLUE/REJETEE, lecture et agrégation par état |
| `internal/connecteurs/orchestre.go` | `id_operation` crypto/rand persisté avant l'appel, corrélation des lignes de journal, rejeu explicite `AvecRejeu`, écritures fail closed TX1/TX2 |
| `cmd/coord-mission/main.go` | flag `-rejeu`, comptage des opérations dans `-consulter` |
| `internal/connecteurs/flow01_operations_test.go` | 8 tests Go du contrat de rejeu |
| `cmd/harnais-flow01/` + `recette/flow01/` | harnais de recette et lanceur reproductible (T3) |

## 2. Contrat de rejeu (D5, validé)

TERMINEE → `OPERATION_DEJA_TERMINEE` (zéro appel, zéro budget) ; ECHOUEE/RESOLU_PERDU/ECARTE/ECHOUE → relance MÊME identité, clé NEUVE ; EN_COURS → `ErrAppelEnCours` ; INTERROMPUE/RESOLU_RECU → `OPERATION_NON_REJOUABLE` ; inconnue → `OPERATION_INCONNUE` ; action/charge/acteur divergents → `OPERATION_INCOMPATIBLE` avant tout appel. Refus journalisés `OPERATION_REJETEE` (informationnel).

## 3. Vérifications

- Tests Go : 16 paquets verts sur le tarball poussé, TestFlow01 8/8, gofmt/vet propres, `GOOS=windows go vet` OK (PGID isolé par build tags).
- Reproductions FAIL sur main documentées (l'API n'y existe pas : les tests ne compilent pas), puis PASS sur la branche.
- Recette locale (sandbox) : **PASS=37 FAIL=0**, contrôles opposés OK.
- Recette VPS (coord, 200.234.70.137) avec le kit épinglé `fd420507` : **PASS=37 FAIL=0**, scénarios V/N+P/E/X/R/I conformes, SIGKILL exclusivement ciblés par PID (témoin PID/PGID/cmdline vérifiés avant signal), factices intacts, nginx actif, aucun processus résiduel. Archive des traces sha256 `2850220642…` conservée sur le VPS.
- Contrôles opposés (`controles-flow01.py`) : corrélation journal↔registre complète (16/16+3/3+3/3), zéro doublon (N-19), registres lisibles, une OUVERTE par opération, refus tracés.

## 4. Décisions et limites

- Comptage séparé terminées/échouées/interrompues/en cours exposé par `-consulter` (ajustement exprès 3).
- Registre absent = vide (compat D6, missions anciennes lisibles) ; registre corrompu = blocage total `OPERATIONS_ILLISIBLES` (D2 fail closed).
- Clôture et fusion restent réservées au pilote (ordre du 27/11).

## 5. Traçabilité

Issue #76 : ACK 7208328839 ; plan v1 A_CORRIGER ; plan v2 (sha256 `e8ae4805c10f2be91163f5152448e2521aeaf8c2c30a4d38b5b87f9cf58e5de8`) PLAN_ACCEPTE, consigné 7208348372 ; T2 7208458373/7212222845 ; branche poussée 7212413389 ; T3 7213453330 ; T4 7213482396. Réceptions OpenAI (session pont `m-coord-flow-01`) : RECEPTION_INTERMEDIAIRE_ACCEPTEE, revalidation T2 sans réserve, T3 ACCEPTEE, T4 ACCEPTEE. Commits : `18f10eda` (identité, registre, rejeu), `d7ed21d5` (gofmt), `fd420507` (harnais + recette).

Documentation : `docs/identites-operations-et-rejeu.md` (FR) et `docs/operation-identities-and-replay.en.md` (EN).

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

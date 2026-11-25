<!-- Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# Dossier de qualification — M-COORD-ACTIVATION-01 — activation de COORD v1.5

Mission : issue #44 (ordre V1.0). Pilote : Denis Bouzon. Exécutant : Mistral (Vibe). Réceptionnaire indépendant : OpenAI. Date de qualification : 26/11/2026. SHA de référence : main `2abdebc943e424a11790dd8a24cd6f43c7d914ee` (tarball codeload épinglé, sha256 archive `1d5e03d8f36b35bc5b08159108d46be91e72b37e0ae5512631901812dce8d184`).

## Décision du pilote

Activation de COORD v1.5 comme référence opérationnelle autorisée par décision explicite publiée dans #44 (commentaire 7171803458, 25/11/2026) : « Exécuter T4 selon le plan documentaire contrôlé : préparer et publier la PR, vérifier les preuves, soumettre au contrôle OpenAI au SHA exact avant fusion. » v1.4 et antérieures deviennent historiques.

## Matrice exigences → résultats (ordre #44, rejeu REEL du 26/11/2026)

| # | Exigence | Résultat | Statut |
|---|---|---|---|
| E1 | 88 tests historiques Python | run.sh (24+13), run_v12.sh (11), run_v13.sh (13), run_v14.sh (17) : FAIL=0, exit 0 | REEL, PASS |
| E2 | 47 tests v1.5 dont M1 | run_v15.sh : PASS=47 FAIL=0, M1 PASS (exception PR #43 : contenu de coord/index.html exempté, présence obligatoire vérifiée) | REEL, PASS |
| E3 | 22 vecteurs communs Python et Go | Python OK=22 KO=0 ; Go OK=22 KO=0, sortie sha256 `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96` (ligne de base #23) ; cross-check 22/22 verdicts identiques Python⇄Go | REEL, PASS |
| E4 | Compilation/tests/démos Go | go vet/build/test : 14 paquets OK (moteur) ; coord/exp conduite PASS incl. S10 ; coord-demo : 19 CONFORME / 0 écart, sortie sha256 `e1f1cdbed291de8e03e8f5ebe3ae20a452b600f87275d92eb95269d3d671a6f6` (byte-identique #23) ; coord-mission -demo-local ACK_RECU ; coord-mcp -demo-local lecture seule stricte, 0 € | REEL/SIMULE, PASS |
| E5 | Séparation des rôles et habilitations | Harnais SEC-01 verts (15+7, PR #34 pérennisés en CI) ; mcpserver lecture seule (jeton distinct, refus mutations) ; auto-réception refusée 409 (rôle résolu serveur) ; clôture contrôleur refusée 409 | REEL/SIMULE, PASS |
| E6 | Cycle critique/correction/reprise/réception/clôture | REFUSEE→ACCEPTEE (verdict ancré empreinte+révision)→ACK_RECU ; CLOTURE_REFUSEE par construction ; vecteurs transitions valides/invalides conformes | REEL/SIMULE, PASS |
| E7 | Invalidation des verdicts obsolètes | Ancrage (révision, empreinte) : la réception d'une révision antérieure ne satisfait jamais l'ancrage courant ; vecteur `reprise_conflit.json` conforme ; tests resume/missions verts | REEL, PASS |
| E8 | Persistance et reprise après interruption | Écriture atomique (temporaire+rename) ; REPRISE_ATTENDUE 409 jusqu'à reprise (N-16/N-17) ; anti-doublon appel_en_cours.json résolu par le seul pilote | REEL, PASS |
| E9 | Cohérence normative des moteurs | N-01..N-20 de COORD Specification 1.0 couverts sans écart (matrice rapport-conformite-std01.md + preuves E1..E8) ; divergence JCS documentée hors vecteurs publiés, arbitrage pilote | REEL, PASS |
| E10 | Robustesse | run_robustness.sh : 10/10 (entrées hostiles, FAIL=0) | REEL, PASS |
| E11 | CI au SHA courant | Check-runs pages success ; workflows qualifs filtrés par chemins non déclenchés par le commit « Rebuild archive » ; dernière exécution verte aux fusions #41 (4/4) et #43 (dérogation consignée) ; compensé par le rejeu local REEL intégral ci-dessus ; -race et multi-OS locaux NON_EXECUTE | REEL/NON_EXECUTE, PARTIEL non bloquant |

## Cycle de réception (protocole v1.4)

- Critique du plan de recette (T2) : **PLAN_ACCEPTE tour 1** (6 points mineurs intégrés).
- Réception indépendante T2 (OpenAI, session pont m-coord-activation-01) : **RECEPTION_CONFORME** ancré au SHA, 5 points non bloquants.
- Contrôle T3 (modifications documentaires) : **SANS OBJECTION**, 5 points de sévérité faible.

## Commandes vérifiées (rejouables, aucune dépendance hors stdlib Python 3)

```sh
cd coord
sh run.sh             # 24+13 PASS (batteries v1.0 + v1.1)
sh run_v12.sh         # 11 PASS
sh run_v13.sh         # 13 PASS
sh run_v14.sh         # 17 PASS
sh run_robustness.sh  # 10 PASS
sh run_v15.sh         # 47 PASS (v1.5 durcie)
python3 docs/conformite-v15/runner_conformite.py          # vecteurs OK=22 KO=0
# Moteur Go (Go 1.27, module coord/go)
go test ./...
go run ./cmd/vector-runner -dir ../docs/conformite-v15    # OK=22 KO=0
go run ./cmd/coord-demo                                    # 19 CONFORME / 0 écart
go run ./cmd/coord-mission -demo-local                     # ACK_RECU (0 €)
go run ./cmd/coord-mcp -demo-local                         # lecture seule (0 €)
```

## Annexe — exception M1 étendue au README (constat d'exécution T4)

Le plan T3 prévoyait la mise à jour des statuts dans `coord/README.md`, or M1 figeait ce fichier (seule `coord/index.html` était exemptée, PR #43). Conformément au précédent PR #43 (page publique évolutive), l'exemption a été étendue au README — fichier de présentation portant les statuts de versions — dans `tests_coord06_v15.py` : présence obligatoire inchangée, 17 autres fichiers historiques contrôlés octet par octet, manifeste 323d8b9 intact. Modification de précaution, signalée au réceptionnaire ; rejeu : v1.5 47/47 PASS (M1 PASS), historiques 88/88, vecteurs 22/22.

## Réserves permanentes (documentées, non bloquantes)

Adaptateurs fournisseurs SIMULÉS hors missions réelles ; qualification fournisseur réelle périmètre #29 ; -race et multi-OS couverts par CI, non exécutables en environnement local restreint ; aucun appel facturable engagé sur cette mission (0 €).

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

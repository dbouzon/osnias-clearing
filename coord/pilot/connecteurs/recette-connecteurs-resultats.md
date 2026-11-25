# Recette M-COORD-PILOT-03 — résultats (connecteurs IA réels)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Exécution REEL en CI (run 45519393828, commit 69f9d0e6, multi-OS + race
detector), sur serveurs factices loopback uniquement : aucune clé, aucun
réseau sortant, aucune dépense. Les 23 tests Go du périmètre (13 connecteur + 9 orchestrateur + 1 budget) sont tous verts, y compris `go test -race -count=2`.

## Connecteur (`connecteurs_test.go`, 13 tests)

| # | Test | Garantie vérifiée |
| --- | --- | --- |
| 1 | TestHTTPEndPointSuccesEtIdempotence | appel complet, jetons réels, `Idempotency-Key` sha256, `Simule=false` |
| 2 | TestHTTPEndPointRetrait429PuisSucces | retrait borné après 429 |
| 3 | TestHTTPEndPointRetraitsEpuises | échec explicite APPEL_ECHOUE |
| 4 | TestHTTPEndPointDelaiDepasse | délai borné (timeout) |
| 5 | TestHTTPEndPointReponseTropGrande | taille plafonnée, refus |
| 6 | TestHTTPEndPointModeleDivergent | identité répondante ≠ déclaré → refus |
| 7 | TestHTTPEndPointJetonsInconnus | usage absent → budget non garanti → refus |
| 8 | TestHTTPEndPointReponseVide | contenu vide → refus |
| 9 | TestHTTPEndPointSortieCompromise | la clé échoée dans la sortie → rejet intégral |
| 10 | TestHTTPEndPointExpurgeMotifDeCle | motif de clé expurgé (`[EXPURGE]`) |
| 11 | TestPortesReellesFermees | chaque porte manquante (mode, COORD_MODE_REEL, autorisation fournisseur, clé) refuse AVANT tout octet émis |
| 12 | TestConfigFailClosed | configuration invalide (mode, doublon, REEL sans autorisation) refusée |
| 13 | TestContexteAnnule | annulation par contexte pendant l'appel |

## Orchestrateur (`orchestre_test.go`, 9 tests)

| # | Test | Garantie vérifiée |
| --- | --- | --- |
| 1 | TestCycleCompletCritiqueExecutionReceptionACK | plan/critique ancrée → exécution → soumission → réception ACCEPTEE → ACK ; ACK sans appel réseau ; clôture refusée ; budget : 2 appels par fournisseur, aucune fuite de clé au journal |
| 2 | TestBoucleDeCorrection | A_CORRIGER → EN_CORRECTION → ré-exécution → nouvelle soumission → ACCEPTEE |
| 3 | TestReponseTardiveArretPendantAppel | arrêt pendant l'appel : état PLANIFIEE inchangé, journal REPONSE_TARDIVE, appel ORPHELIN, anti-doublon puis résolution pilote ; arrêt bloque ensuite toute exécution |
| 4 | TestCrashPendantAppelAntiDoublons | crash après TX1 : reprise refusée APPEL_EN_COURS_NON_RESOLU après redémarrage, résolution pilote, nouvelle tentative autorisée, budget : 2 tentatives comptées (réservation non remboursée) |
| 5 | TestActeurNonHabiliteAucunAppel | réceptionnaire tente d'exécuter : refus normatif, aucun appel, aucun budget |
| 6 | TestPlafondAppelsAvantToutAppel | plafond d'appels : refus BUDGET_DEPASSE avant dépense ; adaptateur sans plafond jamais appelable |
| 7 | TestReceptionVerdictForgeRejete | verdict « CLOTUREE » injecté par le texte du modèle : rejet, soumission toujours en attente, réception ancrée ensuite acceptée |
| 8 | TestReceptionLivrableDivergentRejete | livrable altéré : empreinte divergente → refus AVANT tout appel |
| 9 | TestVerdictPlanNonAncreRejete | verdict de plan non ancré rejeté |

## Budget (`ajuster_test.go`, 1 test)

| # | Test | Garantie vérifiée |
| --- | --- | --- |
| 1 | TestAjusterJetonsSansAppelSupplementaire | le surplus réel ne compte PAS d'appel ; ajustement négatif borné à zéro |

## Non exécuté

- Appels facturés réels : NON_EXÉCUTÉ (autorisation budgétaire du pilote
  requise, conformément à l'ordre de mission).
- Toolchain Go locale (sandbox de mission) : NON_EXÉCUTÉ, couvert par la CI.

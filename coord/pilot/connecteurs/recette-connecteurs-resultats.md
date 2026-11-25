# Recette M-COORD-PILOT-03 — résultats (connecteurs IA réels)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Exécution REEL : Go 1.27.4 officiel linux/amd64 installé dans le sandbox de
correction (mise à jour du 25/11/2026 : `go vet`/`build`/`test` REEL sur les
42 tests Go du périmètre, 32 connecteurs + 10 budget, dont 15 nouveaux pour la
voûte monétaire ; les 4 autres tests du paquet budget sont antérieurs au MVP)
+ CI multi-OS avec race detector (run 45519393828 sur le code initial,
nouveau run au SHA corrigé), sur serveurs factices loopback uniquement :
aucune clé, aucun réseau sortant, aucune dépense.

Correctif de la réserve 7154948522 (contre-réception A_CORRIGER) :
reproductions échouées AVANT correction, puis voûte monétaire implémentée
(réservation durable conservatrice en centimes AVANT chaque tentative
facturable, tarifs/bornes déclarés, refus avant réseau, réservation jamais
libérée). Les 15 tests nouveaux sont détaillés ci-dessous (2 reproductions
converties en garde-fous + 10 tests de voûte + 5 tests budget monétaire).

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

## Budget (`ajuster_test.go` + `reservation_test.go`, 6 tests)

| # | Test | Garantie vérifiée |
| --- | --- | --- |
| 1 | TestAjusterJetonsSansAppelSupplementaire | le surplus réel ne compte PAS d'appel ; ajustement négatif borné à zéro |
| 2 | TestReserverCentimesSansPlafondArme | plafond monétaire non armé => refus, fail closed |
| 3 | TestReserverCentimesPlafondGlobal | le plafond (centimes) est GLOBAL à la mission, tous adaptateurs confondus |
| 4 | TestReserverCentimesEstimationNonPositive | réservation nulle/négative refusée (tarif ou borne inconnu) |
| 5 | TestAjusterCentimesNeLibereJamais | un ajustement négatif ou nul est IGNORÉ : une réservation n'est jamais libérée ; un surcoût est enregistré |
| 6 | TestBudgetAncienFormatRecharge | compteur monétaire absent d'un enregistrement antérieur initialisé sans erreur |

## Voûte monétaire (`reservation_monetaire_test.go` + `reproduction-monetaire_test.go`, 10 tests)

| # | Test | Garantie vérifiée |
| --- | --- | --- |
| 1 | TestReproABudgetSansPlafondMonetaire | REPRO 7154948522 devenue garde-fou : le budget PORTE un plafond et des réservations monétaires |
| 2 | TestReproBAppelReelSansPlafondMonetaireArme | REPRO 7154948522 : appel réel sans plafond armé => refus, aucun octet émis |
| 3 | TestPlafondMonetaireInsuffisantAvantReseau | plafond monétaire insuffisant => `BUDGET_DEPASSE` AVANT réseau, aucune requête, aucune réservation conservée |
| 4 | TestEnveloppeCompteLesRetraits | l'enveloppe réserve (1 + retraits_max) tentatives INTÉGRALES ; décomposition entière exacte (arrondi vers le haut) |
| 5 | TestTarifInconnuRefuseAvantReseau | tarif/borne inconnu => `TARIF_INCONNU` avant réseau, fail closed |
| 6 | TestReservationMonetaireDurableSurCycle | réservation AVANT appel, conservée après succès, journal `centimes_reserves`, nouvelle tentative => nouvelle réservation conservée |
| 7 | TestChargeHorsBorneRefuseAvantEmission | requête hors borne d'entrée => `CHARGE_HORS_BORNE`, aucun octet émis (bornage à l'émission) |
| 8 | TestConfigReelSansTarifsRefusee | configuration REEL sans tarifs/bornes complets refusée au chargement |
| 9 | TestCoutReelCentimesConservateur | coût après coup : tarif le plus élevé appliqué, arrondi vers le haut, requête incluse ; tarif inconnu => erreur |
| 10 | TestExempleConfigReelLivreValide | l'exemple livré `exemple-config-reel.json` reste ACCEPTÉ par `ChargerConfig` (jamais de divergence) |

## Conduite (`-demo-local`, défaut corrigé en cycle)

- Défaut découvert à la reproduction locale sur la tête initiale : la démo
  `-demo-local` échouait `MODELE_DIVERGENT` (les serveurs factices répondent
  `model: demo` alors que la configuration déclarait `demo-exec`/`demo-recep`).
  Corrigé de façon minimale (modèle déclaré = modèle répondu, le contrôle
  fail-closed reste actif) et couvert désormais par la CI (job Go) :
  cycle complet jusqu'à `ACK_RECU`, exit 0, vérifié localement et en CI.

## Non exécuté

- Appels facturés réels : NON_EXÉCUTÉ (autorisation budgétaire du pilote
  requise, conformément à l'ordre de mission).
- Race detector local : NON_EXÉCUTÉ (pas de gcc dans le sandbox de mission),
  couvert par la CI (`go test -race -count=2` incluant `./internal/connecteurs/`).

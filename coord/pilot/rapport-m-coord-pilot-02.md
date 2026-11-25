# Rapport de mission M-COORD-PILOT-02 — MVP autonome Go et architecture adaptateurs

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Mission** : M-COORD-PILOT-02, issue #27 (dbouzon/osnias-clearing). Rapport du 24/11/2026.
**Objet** : livrer le MVP COORD Pilot : service Go local autonome exposant le
cycle de mission sur HTTP loopback, interface d'adaptateurs IA à deux méthodes,
budgets et arrêt pilote fail-closed, UI fonctionnelle, scénario de preuve REEL.
Le protocole v1.5 (candidate NON activée ; v1.4 référence opérationnelle) a été
appliqué au cycle de cette mission même : ACK, vérifications préalables, critique
préalable du plan, développement, recette, publication des preuves.

## 1. Le cycle de mission, étape par étape (traçable dans #27)

| Étape | Référence réelle |
| --- | --- |
| Prérequis levés par le pilote (base de consolidation `8dbcefaf`, vérification HEAD + non-régression à rejouer avant développement) | commentaire 7147078586 |
| ACK MISTRAL : périmètre et limites acceptés | commentaire 7147110086 |
| Bilan post-fusion : main = `ec7dc4f1` (1 commit après consolidation, ne touche que les archives), non-régression Python 88/88, v1.5 47/47, vecteurs 22/22, Go vet/build/test OK, sorties sha256 byte-identiques à la ligne de base #23, CI verte | commentaire 7147146283 |
| Étude REEL du SDK A2A Go (`github.com/a2aproject/a2a-go/v2` v2.9.0, Apache-2.0) : interface interne indépendante retenue, intégration future en module séparé, analyse documentaire isolée sans dépendance normative | `docs/analyse-a2a.md` |
| Plan v1 soumis à critique préalable OpenAI (session pont `m-coord-pilot-02`) puis développement, uniquement sur fichiers nouveaux, branche `mission/coord-pilot-02` depuis `ec7dc4f1` | plan verdict : PLAN_ACCEPTE au premier tour, sans réserve, consigné au commentaire 7147182732 |
| Livraison initiale (PR #28, tête `c4c4b4c6`, CI 5/5 verte) preuves + empreintes | commentaires 7147680424 (issue) et 7147689745 (PR) |
| Réception indépendante OpenAI tour 1 : ACCEPTEE sur `c4c4b4c6` | commentaire 7147689745 |
| Réception complémentaire du pilote : verdict `A_CORRIGER` sur `c4c4b4c6` (six anomalies C1..C6 issues de revue statique, reproductions expérimentales demandées), il remplace pour ce SHA le verdict favorable du tour 1 conformément à H3/N-13 ; corrections exigées sur la MÊME branche/PR | commentaires 7147798524 (issue #27) et 7147799137 (PR #28) |
| ACK du verdict par MISTRAL, engagements acceptés (reproductions, corrections sur la même branche, rejeu recette, tests de concurrence, publication des preuves) | commentaires 7147819323 (issue) et 7147820344 (PR) |

**Position v1.5 rappelée** : expérimentale, NON activée. Aucune activation générale
n'est effectuée ni revendiquée par ce MVP. Les adaptateurs IA sont SIMULÉS :
aucune interopérabilité réelle n'est revendiquée, aucun réseau sortant, aucune clé,
aucun appel payant.

## 2. Livrables

Contrôle d'intégrité contre l'archive de `main` `ec7dc4f1` à la livraison
initiale : **0 fichier modifié, 0 supprimé, uniquement des fichiers nouveaux**
(présent rapport compris). La révision corrective ne touche ensuite QUE des
fichiers introduits par la mission, plus un fichier de qualification
(`.github/workflows/coord-go-ci.yml`, ajout du job `Race detector` justifié par
le verdict A_CORRIGER) ; aucun fichier du noyau historique n'est modifié,
confirmé par les empreintes byte-identiques des vecteurs et de la démo Go.

| Périmètre | Contenu |
| --- | --- |
| `coord/go/cmd/coord-pilot/main.go` | binaire du service : écoute loopback refusée ailleurs (S3), token généré et journalisé en console uniquement |
| `coord/go/internal/adaptateurs/` | interface `Executer`/`Soumettre` (2 méthodes), adaptateur `Simule` fail-closed : jetons `JetonsExecuter=1000`, `JetonsSoumettre=250`, refus des identités réelles (« reel »/« real ») |
| `coord/go/internal/budget/` | plafonds fail-closed : dépassement bloqué AVANT tout appel, motif journalisé (R3) |
| `coord/go/internal/arret/` | états `ARRET_PILOTE`/`LEVEE_ARRET_PILOTE`, refus `REFUS_TRANSITION_ARRET` avec distinction du traitement N-20 (R4) |
| `coord/go/internal/missions/` | référentiel multi-missions, verrou par mission, transaction sérialisée (`Transaction`, verrou du chargement à la sauvegarde), copie indépendante pour validation (`Copie`), contrôle d'unicité d'empreinte, codage de dossier injectif |
| `coord/go/internal/pilot/` | service HTTP loopback : cycles de mission, rôles résolus côté serveur (S4), sécurité S1..S5, scénario E2E du recette ; validation normative AVANT tout appel d'adaptateur, arrêt durable, erreurs de journal explicites |
| `coord/pilot/ui/` | UI fonctionnelle 8 écrans : sans ressource externe (CSP self), bandeau ADAPTATEURS SIMULÉS permanent, appels typés `/api/*` et `/health`, guide Prise en main |
| `coord/pilot/preuves-mvp/` | scénario REEL exécutable de bout en bout + traces d'exécution horodatées, dont `trace-reproductions.txt` (reproductions C1..C6 avant correction, invariants après) |
| `coord/go/internal/pilot/concurrence_test.go` | tests d'invariants permanents C1..C6 (concurrence, plafond, arrêt durable, reprise exclusive, journal explicite, dossiers injectifs) |
| `coord/pilot/docs/analyse-a2a.md` | étude A2A et mapping conceptuel vers l'interface interne |
| `coord/pilot/recette-mvp-resultats.md` | recette complète avec verdicts point par point |
| présent rapport | bilan de mission |

La maquette #25 reste intacte et couverte (T1..T12 : 12/12, artefact non modifié).

## 3. Cycle correctif C1..C6 (verdict complémentaire A_CORRIGER)

Le pilote a rendu un verdict complémentaire `A_CORRIGER` sur `c4c4b4c6`, basé sur
sa revue statique, en demandant la reproduction expérimentale de chaque anomalie.
Protocole appliqué : REPRODUIRE d'abord (chaque anomalie démontrée par un échec
expérimental sur le code non corrigé), CORRIGER ensuite, puis VERROUILLER par un
test d'invariant permanent exécuté sous concurrence :

- **C1** (Obtenir/Sauver séparés, mises à jour perdues) : reproduit (13 appels
  passés / 2 comptabilisés, plafond violé) → `Referentiel.Transaction`, verrou de
  mission détenu du chargement à la sauvegarde incluant budget, transition,
  arrêt et reprise ;
- **C2** (appel d'adaptateur avant validation normative) : reproduit (1 appel +
  jetons consommés pour une transition interdite) → validation sur copie
  indépendante AVANT tout appel ; refus = aucun appel, aucune consommation ;
- **C3** (arrêt humain écrasé par une transition concurrente) : reproduit → arrêt
  contrôlé dans la transaction sérialisée, ordre linéarisable, arrêt durable ;
- **C4** (levée d'arrêt sans reprise normative) : reproduit (200 immédiat) →
  indicateur `RepriseAttendue` : toute transition refusée `REPRISE_ATTENDUE`
  jusqu'à la reprise N-16/N-17, seule voie de déblocage ;
- **C5** (erreurs de journal ignorées) : reproduit (403 malgré journal
  indisponible) → propagation, réponse typée `ERREUR_JOURNAL` (500), état non
  persisté pour une mutation non journalisée ;
- **C6** (collisions de nomDir) : reproduit (« a b » et « a-b » vers un même
  dossier) → codage injectif octet par octet (échappement `~xx`).

Verrous permanents : `coord/go/internal/pilot/concurrence_test.go`
(`TestC1InvariantPlafondSousConcurrence`, `TestC1TransactionSerieArrEtBudget`,
`TestC3InvariantArretDurableSousConcurrence`, `TestC2TransitionInterditeSansAppelNiConsommation`,
`TestC4LeveeExigeRepriseNormative`, `TestC5ErreurJournalExplicite`,
`TestC6IdentifiantsDistinctsDossiersDistincts`), verts sur deux exécutions
consécutives (trace `trace-reproductions.txt`).

## 4. Résultats (résumé ; détail dans `recette-mvp-resultats.md`)

Toutes les exécutions sont REEL, sur le code distribué, toolchain Go 1.27.4
linux/amd64 et Python 3 stdlib :

- `go vet`, `go build`, `go test ./...` : 11 paquets testés OK (re-vérifié après
  remise à zéro du cache de tests en fin de mission) ;
- Python : 78/78 historiques + 10/10 robustesse (= 88/88), v1.5 durcie 47/47,
  vecteurs de conformité 22/22, démo v1.5 exit 0 ;
- maquette T1..T12 : 12/12 ; UI T13..T16 : 4/4 ;
- scénario E2E REEL complet (binaire construit depuis les sources, service
  loopback, HTTP) : **25/25 OK, exit 0** ;
- contrôles négatifs fail-closed effectifs : transitions hors table (R1),
  habilitations malsaines (R2/N-05), dépassement de budget (R3), arrêt pilote
  (R4), divergence avec arbitrage « github fait foi » et reprise conditionnée au
  head (R5/N-16, N-17), verdict périmé (R6/N-13) ; sécurité S1..S5 confirmée.

Empreintes des traces (REEL, révision corrigée) :

- `preuves-mvp/trace-recette-python.txt` sha256 `9849bd09984e4ef778b860708f9620d839995614975cfc60b39fa3f3fc21d682`
- `preuves-mvp/trace-recette-go.txt` sha256 `0368685bde1618267e352b69d20a22bb60d535f8e75f4d3e82797a739a20dd21`
- `preuves-mvp/trace-scenario-e2e.txt` sha256 `d47ef8c8c7f3cdb3e973f354d20120aaade9ab46e80ba3287d38170fa511137c` (BILAN SCENARIO E2E: OK=25 KO=0, byte-identique à la livraison initiale)
- `preuves-mvp/trace-reproductions.txt` sha256 `f41627b48456352dd8e2d670e7f077c82dcd6fb6da46a7a4d57ba23e638f804f` (échecs pré-correction conservés + invariants post-correction)

## 5. Réserves explicites (NON_EXECUTE / SIMULE)

- **Adaptateurs SIMULÉS** : aucun dialogue IA réel n'est revendiqué ; l'interface
  à deux méthodes est le point d'ancrage de la future intégration A2A (module
  séparé, cf. `docs/analyse-a2a.md`).
- **Rendu navigateur complet : NON_EXECUTE** dans le sandbox de la mission
  (aucun navigateur disponible) ; les contrôles dynamiques sont REEL via HTTP,
  le rendu visuel reste à la discrétion du pilote local.
- **Go Windows/macOS : NON_EXECUTE en séance** ; la CI existante les couvre, à
  confirmer sur la branche.
- **Race detector : NON_EXECUTE en local** (compilateur C absent du sandbox) ;
  couvert par le job CI dédié `Race detector` ajouté au workflow de
  qualification (`go test -race -count=2 ./internal/pilot/ ./internal/missions/`).
- **Aucun appel A2A/MCP réel** : l'étude est documentaire, sans dépendance de
  code vers le SDK (module normatif stdlib-only préservé).

## 6. Décisions demandées au pilote

1. Nouvelle réception indépendante de la révision corrigée (avis OpenAI
   sollicité, session pont), conformément au verdict A_CORRIGER.
2. Retour d'expérience v1.5 sur ce cycle : à consigner par le pilote.
3. Fusion de la PR #28 : autorisée par le pilote SI la nouvelle réception est
   favorable ; sans réception favorable, aucune fusion.

**États du code inchangés** : v1.4 = référence opérationnelle ; v1.5 =
expérimentale, non activée. Les clôtures formelles restent réservées au pilote.

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

**Position v1.5 rappelée** : expérimentale, NON activée. Aucune activation générale
n'est effectuée ni revendiquée par ce MVP. Les adaptateurs IA sont SIMULÉS :
aucune interopérabilité réelle n'est revendiquée, aucun réseau sortant, aucune clé,
aucun appel payant.

## 2. Livrables (fichiers nouveaux uniquement)

Contrôle d'intégrité contre l'archive de `main` `ec7dc4f1` : **0 fichier modifié,
0 supprimé, uniquement des fichiers nouveaux** (présent rapport compris).

| Périmètre | Contenu |
| --- | --- |
| `coord/go/cmd/coord-pilot/main.go` | binaire du service : écoute loopback refusée ailleurs (S3), token généré et journalisé en console uniquement |
| `coord/go/internal/adaptateurs/` | interface `Executer`/`Soumettre` (2 méthodes), adaptateur `Simule` fail-closed : jetons `JetonsExecuter=1000`, `JetonsSoumettre=250`, refus des identités réelles (« reel »/« real ») |
| `coord/go/internal/budget/` | plafonds fail-closed : dépassement bloqué AVANT tout appel, motif journalisé (R3) |
| `coord/go/internal/arret/` | états `ARRET_PILOTE`/`LEVEE_ARRET_PILOTE`, refus `REFUS_TRANSITION_ARRET` avec distinction du traitement N-20 (R4) |
| `coord/go/internal/missions/` | référentiel multi-missions, verrou par mission, contrôle d'unicité d'empreinte |
| `coord/go/internal/pilot/` | service HTTP loopback : cycles de mission, rôles résolus côté serveur (S4), sécurité S1..S5, scénario E2E du recette |
| `coord/pilot/ui/` | UI fonctionnelle 8 écrans : sans ressource externe (CSP self), bandeau ADAPTATEURS SIMULÉS permanent, appels typés `/api/*` et `/health`, guide Prise en main |
| `coord/pilot/preuves-mvp/` | scénario REEL exécutable de bout en bout + traces d'exécution horodatées |
| `coord/pilot/docs/analyse-a2a.md` | étude A2A et mapping conceptuel vers l'interface interne |
| `coord/pilot/recette-mvp-resultats.md` | recette complète avec verdicts point par point |
| présent rapport | bilan de mission |

La maquette #25 reste intacte et couverte (T1..T12 : 12/12, artefact non modifié).

## 3. Résultats (résumé ; détail dans `recette-mvp-resultats.md`)

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

Empreintes des traces (REEL) :

- `preuves-mvp/trace-recette-python.txt` sha256 `f64e7f4f0751df4e7724fa9d122ced28269682386d5044de910e588f77e000a4`
- `preuves-mvp/trace-recette-go.txt` sha256 `7a4b902d84c8db9612e0d7d47b441b2ae8d4b72967e3a3da1efd08fb690dd6f9c`
- `preuves-mvp/trace-scenario-e2e.txt` sha256 `d47ef8c8c7f3cdb3e973f354d20120aaade9ab46e80ba3287d38170fa511137c` (BILAN SCENARIO E2E: OK=25 KO=0)

## 4. Réserves explicites (NON_EXECUTE / SIMULE)

- **Adaptateurs SIMULÉS** : aucun dialogue IA réel n'est revendiqué ; l'interface
  à deux méthodes est le point d'ancrage de la future intégration A2A (module
  séparé, cf. `docs/analyse-a2a.md`).
- **Rendu navigateur complet : NON_EXECUTE** dans le sandbox de la mission
  (aucun navigateur disponible) ; les contrôles dynamiques sont REEL via HTTP,
  le rendu visuel reste à la discrétion du pilote local.
- **Go Windows/macOS : NON_EXECUTE en séance** ; la CI existante les couvre, à
  confirmer sur la branche.
- **Aucun appel A2A/MCP réel** : l'étude est documentaire, sans dépendance de
  code vers le SDK (module normatif stdlib-only préservé).

## 5. Décisions demandées au pilote

1. Réception indépendante du livrable (avis OpenAI sollicité, session pont).
2. Retour d'expérience v1.5 sur ce cycle : à consigner par le pilote.
3. Fusion de la PR : réservée au PILOTE, aucune fusion sans décision explicite.

**États du code inchangés** : v1.4 = référence opérationnelle ; v1.5 =
expérimentale, non activée. Les clôtures formelles restent réservées au pilote.

# Rapport post-fusion — mission M-COORD-RELEASE-01 (qualification post-fusion et publication du standard)

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

Date : 24/11/2026 (UTC). Issue : #23 repo public `dbouzon/osnias-clearing`.
Ancrage : `main` `a803b3c533ceba605bbd8795ed061e1c59bc85f9` (commit de workflow
« Rebuild COORD archive »), parent direct vérifié `ba01a8948e8e2a55d40f38d030d054f37c2ab4c8`
(fusion de la PR #22, mission M-COORD-STD-01, réception RECEPTION_CONFORME).

## 1. Inventaire post-fusion

- Les **24 fichiers** de la PR #22 sont présents sur `main`, vérifiés fichier à
  fichier depuis l'arborescence complète téléchargée au SHA d'ancrage (liste en
  annexe A). **Écart de décompte confirmé et corrigé : le manifeste de la
  mission précédente annonce 23 fichiers, la PR en contient 24. Le présent
  rapport retient 24.**
- Empreinte de l'archive ZIP servie par Pages au moment du rapport :
  `451fbe5ca66480c316fe2ed67774e85d6999d7a8e78e072085736059de29c7ab`
  (73 entrées, moteur Go, norme et schémas inclus) — reconstruite
  automatiquement par le workflow à la fusion, encore sous le nom résiduel
  `coord-v1.0-v1.4.zip` (corrigé par la présente mission, cf. § 4).

## 2. Rejeu des batteries sur main fusionné (qualification REEL, code distribué téléchargé depuis le dépôt)

| Test famille | Qualification | Résultat |
| --- | --- | --- |
| Batteries Python v1.0..v1.4 + robustesse | REEL | 88/88 PASS (suite v1.0 : 24 ; suite v1.1 : 13 ; v1.2 : 11 ; v1.3 : 13 ; v1.4 : 17 ; robustesse T3 : 10) |
| Batterie Python v1.5 durcie | REEL | 47/47 PASS |
| 22 vecteurs publiés (runner Python) | REEL | OK=22 KO=0, exit 0, sortie sha256 `a9bcd4ebee5ee5ede0948e30b41dd7ed7c6d25285211f6c7d7de50f11cdb1ebe`, byte-identique à la référence |
| Démonstration Python v1.5 | REEL | exit 0, scénario intégral CONFORME |
| `go vet` / `go build` / `go test ./...` (go1.27.4 linux/amd64 officiel) | REEL | OK (5 paquets testés) |
| 22 vecteurs publiés (runner Go) | REEL | OK=22 KO=0, exit 0, sortie sha256 `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96`, byte-identique à la ligne de base PR #22 |
| Démonstration Go | REEL | CONFORME=19 ECART=0, exit 0, sortie sha256 `e1f1cdbed291de8e03e8f5ebe3ae20a452b600f87275d92eb95269d3d671a6f6`, byte-identique |
| Comparaison croisée Python ⇄ Go | REEL | 22/22 verdicts identiques |
| `go test` Windows/macOS natifs locale | NON_EXECUTE au moment du rapport ; exécuté par CI matrice (cf. § 3) | résultats sur la PR |
| E2E fournisseurs réels (OpenAI/Mistral/GitHub) | NON_EXECUTE | hors périmètre ; aucune assimilation simulée/réel |

Toutes les empreintes d'exécution REEL sont **byte-identiques** aux lignes de
base pré-fusion (rapport de conformité M-COORD-STD-01) : la fusion n'a rien
changé aux comportements.

## 3. Qualification complémentaire (étape 2), sans réécriture du moteur

### 3.1 JCS RFC 8785 : harness croisé Python ⇄ Go

Dix cas couvrant le sous-ensemble COORD (tri des clés, échappement minimal,
caractères de contrôle, entiers, structures imbriquées, conteneurs vides,
tableaux, exemple officiel de tri UTF-16 du RFC 8785) :

- **9/10 empreintes identiques** entre `coord06.canonical_json` (Python) et
  `coordengine/internal/jcs` (Go), y compris l'exemple officiel RFC 8785
  (§ 3.2.3 : `\r` < `1` < `ö` < `€`).
- **1 divergence documentée, hors données des vecteurs publiés** : clés hors
  plan BMP face aux clés U+E000..U+FFFF. Go trie par unités UTF-16 (normatif
  RFC 8785), le tri `sort_keys` de Python suit les points de code ; les deux
  ordres s'inversent pour ces clés. **Aucun vecteur publié ne contient de telles
  clés.** Aucune correction silencieuse : arbitrage laissé au pilote (documenter
  la limite, ou durcir le moteur Python candidat dans une mission dédiée).
- **Nombres non entiers (hors sous-ensemble COORD)** : Go refuse (conforme à la
  spécification 5.2 : refuser plutôt qu'émettre une forme non canonique) ;
  le Python candidat sérialise au lieu de refuser (`[1.5]`, `[1e+30]`).
  Divergence documentée au même titre, hors données publiées.

### 3.2 Schémas JSON 2020-12 : validation par validateur externe

L'installation de paquets est impossible dans l'environnement de qualification
local (pip/npm interdits ; API publiques de validation non exploitables). La
validation REEL par validateur indépendant (`jsonschema`, Draft 2020-12) est
déplacée dans le job `schemas` du workflow `coord-go-ci.yml`, exécuté sur un
runner GitHub, avec instances témoins valides et invalides (identités,
habilitations fail-closed avec joker/doublon/role inconnu, verdicts référencés,
enveloppes avec horodatage UTC-Z, 22 vecteurs sur 4 fichiers). Les résultats
figurent sur la présente PR ; le présent rapport ne prétend pas à une
validation non exécutée.

### 3.3 Accès bibliothèque Go externe : vérifié REEL

Le proxy de modules est accessible depuis l'environnement (`go mod download`
d'un module tiers : OK). Le choix « stdlib uniquement » de `coordengine` reste
donc une décision de conception, pas une contrainte de l'environnement ;
aucune dépendance n'est ajoutée sans arbitrage explicite.

### 3.4 Multi-OS : outillé par CI réelle

Le workflow `coord-go-ci.yml` exécute `go vet`, `go build`, `go test`, les 22
vecteurs et la démo sur une matrice **ubuntu / windows / macos**, plus un job
de rejeu des batteries Python et des vecteurs (runner GitHub). Première passe
sur la PR : **les trois jobs Go (ubuntu, windows, macos) sont passés au
premier tour** (vet/build/test, 22/22 vecteurs, démo 19 CONFORME / 0 ECART).
**Pas d'assimilation : les statuts finaux par OS sont ceux des checks de la
PR après la passe corrective** ; l'activation ne sera pas proposée avant leur
lecture. Le même cycle a détecté l'écart de manifeste (§ 4) : le garde-fou a
fonctionné avant fusion.

## 4. Livrables de publication (étapes 3 et 4)

- **Écart détecté par la CI puis corrigé (preuve que les gardes fonctionnent)**
  : la première passe de la PR modifiait `coord/README.md` et
  `coord/index.html`, deux des 19 fichiers historiques figés octet à octet par
  le manifeste d'intégrité (test M1 de la batterie v1.5, référence main
  323d8b9). Le job CI « Batteries Python » a signalé la divergence avant
  fusion. **Correction : les deux fichiers sont rétablis octet à octet à
  l'état de main.** Conformément à l'ordre de mission (« ne pas modifier les
  fichiers historiques sans arbitrage explicite ; traiter les métadonnées web
  séparément ou proposer une migration contrôlée »), la mise à jour de la page
  FR et du README est **proposée comme migration contrôlée** : le contenu
  cible est préparé et archivé dans le Drive `coord` (02-Missions-et-decisions)
  ; sa mise en œuvre exige l'arbitrage explicite du pilote, le re-calcul du
  manifeste et une nouvelle réception.
- **Pages publiques** : `/en/coord/` mise à jour (section COORD Specification
  1.0, statut CANDIDAT non activé, rejeu v1.5 et 22 vecteurs, téléchargement
  versionné, « tools/ai-coordinator » cadré au rôle documentaire) ; pages
  appel `/coord/implementations/` et `/en/coord/implementations/` créées
  (aucune n'est couverte par le manifeste). En attendant la migration validée
  de la page FR, l'incohérence FR/EN est assumée et documentée ici même.
- **Validation des schémas : sémantique corrigée** : une liste d'adaptateurs
  vide est **valide** au sens du schéma (`habilitations.schema.json`) :
  l'invariant N-01 « fail-closed » (une liste vide ou absente refuse tout au
  moteur) relève du moteur, pas du schéma ; le témoin initial attendait à
  tort un refus et a été corrigé (première passe de CI en échec, corrigée).
  Les `$ref` distants de l'enveloppe résolvent via un registre local, sans
  réseau.
- **Appel aux implémentations** : fichier `coord/CALL-FOR-IMPLEMENTATIONS.md`
  (EN avec résumé FR), entrées sitemap hreflang pour les deux pages.
- **Archive versionnée** : le workflow `coord-zip.yml` produit désormais
  `coord-v1.0-v1.5.zip` et conserve l'ancien lien `coord-v1.0-v1.4.zip`
  comme copie figée de compatibilité (arbitrage Q1 de la critique, validé).
  L'archive est reconstruite automatiquement après fusion ; les contrôles
  post-fusion (HTTP 200, SHA256, rejeu des batteries depuis l'archive)
  sont rejoués à la fusion, pas dans cette PR.
- **Dossier Zenodo** : préparé dans le Drive `coord` (métadonnées, descriptif
  FR/EN, fichier d'archive à déposer, version liée à l'upload existant
  24512445). Le dépôt réel — et l'attribution du DOI — reste réservé au
  pilote : **aucune publication Zenodo n'est prétendue effectuée ici.**

## 5. Réserves

| Réserve | Qualification | Raison, périmètre, condition de levée |
| --- | --- | --- |
| Adaptateurs SIMULÉS (démo Go/Python) | REEL/limité | la démonstration ne prouve rien de la sécurité de l'authentification (norme § 11) ; levée par des adaptateurs réels hors périmètre |
| Validation schémas externe | REEL en CI | exécutée sur runner GitHub au moment de la PR ; consigner les résultats avant fusion |
| `go test` Windows/macOS natif local | REEL en CI | exécuté par la matrice CI ; aucune assimilation avant lecture des résultats |
| JCS : clés astrales et non-entiers (Python) | DIVERGENCE documentée | hors données publiées ; arbitrage pilote requis avant toute évolution normative |
| E2E fournisseurs | NON_EXECUTE | hors périmètre de la mission, aucun compte ni clé requis par le standard |
| Publication Zenodo / DOI | PRÊT, non effectué | dépôt réservé au pilote ; citation du DOI uniquement après attribution |

## 6. Cycle protocole

Plan v1 critiqué par OpenAI (session pont `m-coord-release-01`) : verdict
**A_CORRIGER** (3 points bloquants : décompte, nom d'archive, tests multi-OS ;
2 suggestions : réserves détaillées, page dédiée). Plan v2 corrigé :
**PLAN_ACCEPTE** (second tour, aucune réserve bloquante restante). Réception
indépendante du présent rapport : demander un verdict OpenAI dans l'issue #23.
**Fusion et activation réservées au pilote.**

## 7. Proposition de statut

- v1.4 : **opérationnelle**, reste la référence.
- v1.5 durcie et COORD Specification 1.0 (moteur Go de référence) :
  **candidatures qualifiées, non activées** ; activation sur décision
  explicite du pilote, après lecture des résultats CI multi-OS et de la
  réception indépendante.

---

## Annexe A : les 24 fichiers de la PR #22 vérifiés sur main

1. `coord/docs/coord-guides.md`
2. `coord/docs/guide-utilisateur.md`
3. `coord/docs/rapport-conformite-std01.md`
4. `coord/docs/spec/coord-spec-1.0.md`
5. `coord/go/cmd/coord-demo/main.go`
6. `coord/go/cmd/vector-runner/main.go`
7. `coord/go/go.mod`
8. `coord/go/internal/ctxcontrol/ctxcontrol.go`
9. `coord/go/internal/ctxcontrol/ctxcontrol_test.go`
10. `coord/go/internal/engine/engine.go`
11. `coord/go/internal/engine/engine_test.go`
12. `coord/go/internal/jcs/jcs.go`
13. `coord/go/internal/jcs/jcs_test.go`
14. `coord/go/internal/persist/persist.go`
15. `coord/go/internal/persist/persist_test.go`
16. `coord/go/internal/policy/policy.go`
17. `coord/go/internal/resume/resume.go`
18. `coord/go/internal/resume/resume_test.go`
19. `coord/go/internal/spec/spec.go`
20. `coord/schemas/habilitations.schema.json`
21. `coord/schemas/identity.schema.json`
22. `coord/schemas/message.schema.json`
23. `coord/schemas/vector.schema.json`
24. `coord/schemas/verdict.schema.json`

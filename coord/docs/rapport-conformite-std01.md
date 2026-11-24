# Rapport de conformité — mission M-COORD-STD-01 (COORD Specification 1.0, moteur de référence Go)

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

Date : 24/11/2026 (UTC). Base : `main` `99afbc76a297d14c2ba570dc25eba4e7c119cb01`
(post-fusion PR #21, mission M-COORD-HARDENING-01, réception RECEPTION_CONFORME).

## 1. Livrables

| Livrable | Emplacement |
| --- | --- |
| Norme autonome (anglais, RFC 2119) | `coord/docs/spec/coord-spec-1.0.md` |
| Moteur de référence Go (stdlib uniquement) | `coord/go/` (module `coordengine`) |
| Schémas JSON versionnés (2020-12) | `coord/schemas/` (5 schémas) |
| Vecteurs universels publics | `coord/docs/conformite-v15/` (22 vecteurs, inchangés, réutilisés en place) |
| Guide implémenteur | `coord/docs/coord-guides.md` (anglais) |
| Guide utilisateur | `coord/docs/guide-utilisateur.md` (français) |
| Démonstration autonome | `coord/go/cmd/coord-demo` (agents simulés, sans compte ni clé) |
| Runner de vecteurs Go | `coord/go/cmd/vector-runner` |

## 2. Qualifications exécutées (fusionné une seule nomenclature REEL/SIMULE/NON_EXECUTE)

| Test famille | Qualification | Résultat |
| --- | --- | --- |
| `go vet ./...` | REEL (go1.27.4 linux/amd64 officiel, installé le 24/11/2026) | OK |
| `go build ./...` | REEL | OK |
| `go test ./...` (moteur, JCS, persistance, reprise, contexte) | REEL | OK (tous paquets) |
| 22 vecteurs publiés sur le moteur Go (`vector-runner`) | REEL (adaptateurs SIMULES) | **OK=22 KO=0**, exit 0 |
| 22 vecteurs publiés sur le moteur Python (`runner_conformite.py`) | REEL | **OK=22 KO=0**, exit 0, sortie byte-identique à la référence `a9bcd4eb…` |
| Comparaison croisée Python ⇄ Go (verdicts par vecteur) | REEL | **CONFORME : 22/22 verdicts identiques (OK) sur les deux moteurs** |
| Démonstration autonome Go | REEL (adaptateurs SIMULES) | **CONFORME=19 ECART=0**, exit 0 |
| Batteries Python historiques (non-régression v1.0..v1.5) | REEL | 88/88 + 47/47 + démo Python 18 CONFORME/0 ECART (rejoués sur main 99afbc76, sorties byte-identiques) |
| `go test` Windows/macOS | NON_EXECUTE | pas d'exécution native croisée ; builds documentés, aucune assimilation compilation/exécution |
| E2E fournisseurs (OpenAI/Mistral/GitHub réels) | NON_EXECUTE | hors périmètre de la mission ; aucun compte ni clé requis |

Empreintes (sha256) des sorties d'exécution REEL, pour audit :
- vecteurs Go : `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96`
- démo Go : e1f1cdbed291de8e03e8f5ebe3ae20a452b600f87275d92eb95269d3d671a6f6
- vecteurs Python : `a9bcd4ebee5ee5ede0948e30b41dd7ed7c6d25285211f6c7d7de50f11cdb1ebe`

## 3. Écarts historiques documentés (sans modification des références)

### 3.1 Divergence Go historique (`tools/ai-coordinator`)

`tools/ai-coordinator/` (préexistant, intact) implémente une machine à 7 états
avec sélecteurs de fournisseurs (github/openai/mistral/relay) et un système de
store : c'est de l'infrastructure de coordination, PAS une implémentation de la
norme COORD 1.0. Écarts documentés : pas de table de transitions fermée,
pas d'habilitations fail-closed, pas de verdict référencé obligatoire,
pas de garde par empreinte. La correspondance d'états avec la machine à 8
états de la norme figure en annexe 11 de `docs/proto-coord06-v15.md`.

### 3.2 Vocabulaire d'états Python (v1.4) vs norme

La v1.4 Python utilise des états (`DECISION_API`, `ACK_MISTRAL`, `CLOTUREE`,
`ANNULATION`) qui ne coïncident pas un à un avec les 8 états normatifs ; le
texte de la mission #19 l'avait documenté (constat C8). La norme 1.0 fixe
l'ensemble fermé à 8 états ; la candidature v1.5 durcie l'implémente déjà ; la
v1.4 historique reste LA référence opérationnelle active et n'est pas modifiée.

### 3.3 Enveloppes

Les schémas `message.schema.json` / `identity.schema.json` / `verdict.schema.json`
/ `habilitations.schema.json` formalisent les échanges pour les implémentations
futures ; les vecteurs existants (format `coord-conformite-v1`) restent la
pièce normative d'exécution et ne sont pas modifiés.

## 4. Invariants couverts par les vecteurs

N-01 à N-05 (habilitations, 8 vecteurs), N-06/N-09 à N-13 (transitions,
12 vecteurs), N-16 à N-17 (reprise, 2 vecteurs). Les invariants de persistance
(N-15), de canonicalisation (RFC 8785) et de contrôle du contexte (N-18..N-20)
sont couverts par `go test` et la démonstration ; ils n'exigent pas de
vecteur public supplémentaire pour la présente qualification.

## 5. Réserves

1. `go test` qualifié REEL sur linux/amd64 uniquement ; Windows/macOS
   NON_EXECUTES (dits comme tels).
2. La démonstration n'est pas une preuve de sécurité de l'authentification :
   adaptateurs SIMULES, moteur n'authentifiant jamais.
3. La réception des schémas JSON (validation outillée par un validateur
   2020-12 indépendant) est déclarée NON_EXECUTEE : aucune dépendance externe
   n'est admise dans le périmètre Go stdlib ; les schémas sont publiés pour
   les implémenteurs.
4. MCP/A2A : hors périmètre (validé par le plan critique PLAN_ACCEPTE).
5. La norme est une CANDIDATURE ouverte : aucune prétention à une
   reconnaissance officielle de standard ; « standard ouvert » désigne ici
   une spécification publique, autonome et vérifiable par des tiers.

## 6. Cycle

Plan critiqué A_CORRIGER → corrigé → **PLAN_ACCEPTE** (OpenAI, session
`m-coord-std-01`, premier tour, publié dans l'issue #20). Réception
indépendante : OpenAI sous autorité du pilote. **Fusion et activation
réservées au pilote.** Une seconde implémentation indépendante (le moteur
Python v1.5 durci) a été qualifiée sur les mêmes 22 vecteurs : comparaison
croisée conforme.

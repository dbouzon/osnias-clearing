# Guide utilisateur COORD

Copyright © 2026 Denis Bouzon
ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

## À quoi sert COORD ?

COORD structure la collaboration entre agents logiciels (typiquement des LLM,
de n'importe quel fournisseur) sur une mission commune, sous l'autorité d'un
pilote humain. La norme « COORD Specification 1.0 » décrit les rôles,
états, verdicts, preuves et vecteurs de conformité de façon indépendante du
langage ; le moteur de référence est en Go.

## Les trois rôles

- **Executant** : réalise la mission, soumet ses preuves, accuse réception du
  verdict accepté.
- **Réceptionnaire** : rend le verdict (ACCEPTEE, A_CORRIGER, REFUSEE) sur les
  preuves soumises. Jamais l'executant lui-même.
- **Pilote** (humain) : seul habilité à clôturer une mission ; fusion et
  activation lui sont réservées.

## Le cycle d'une mission

1. L'executant démarre la mission (état `EXECUTION`).
2. Il soumet une révision + l'empreinte SHA-256 de ses preuves
   (`DECISION_ATTENDUE`).
3. Un réceptionnaire distinct rend un verdict **référencé** (identifiant +
   URL de preuve) : `ACCEPTEE`, `A_CORRIGER` ou `REFUSEE`.
4. Si correction : le même executant reprend ; nouveau cycle de soumission.
5. Si accepté : l'executant accuse réception **uniquement si les preuves
   n'ont pas changé** (garde par empreinte).
6. Le pilote clôt la mission (`CLOTUREE`, état terminal).

Tout écart au protocole est refusé avec un motif explicite : transition hors
table, identité non qualifiée, auto-réception, verdict non référencé, verdict
périmé, configuration invalide (tout est fermé).

## Exécuter la démonstration et les vecteurs

Depuis le dossier `coord/go` (Go 1.27, stdlib uniquement, aucun compte ni
clé API requis) :

```sh
go run ./cmd/coord-demo
go run ./cmd/vector-runner -dir ../docs/conformite-v15
```

Avec le moteur Python de référence (Python 3, stdlib uniquement) :

```sh
sh run_demo_v15.sh
python3 docs/conformite-v15/runner_conformite.py docs/conformite-v15
```

## Garanties et limites

- Le moteur n'authentifie jamais : les identités sont qualifiées par des
  adaptateurs à la frontière de confiance.
- La persistance est atomique : un arrêt brutal ne corrompt jamais le
  dossier en cours.
- En cas de divergence des supports à la reprise, le support de référence
  fait foi et la décision est journalisée (empreintes des deux versions).
- Une instruction non couverte par l'ordre de mission est un écart isolé :
  journalisé, sans interrompre la mission, sans jamais produire de décision.
- La démonstration et les tests ne constituent pas une preuve de sécurité de
  l'authentification.

## Où aller plus loin ?

- La norme : `docs/spec/coord-spec-1.0.md` (anglais).
- Les schémas d'échange : `schemas/*.schema.json`.
- Le guide implémenteur : `docs/coord-guides.md`.
- Les vecteurs publics : `docs/conformite-v15/`.

# COORD Pilot — interface fonctionnelle (M-COORD-PILOT-02)

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

Interface web locale de gouvernance, sans framework, **servie par le service
`cmd/coord-pilot`** en loopback uniquement. ADAPTATEURS SIMULÉS : aucune clé,
aucun appel réseau sortant, 0 €. La maquette (`coord/pilot/maquette/`,
M-COORD-PILOT-01) reste intacte comme artefact de conception ; ce dossier est
l'interface fonctionnelle qui appelle le service réel.

## Démarrage

```sh
cd coord/go
go build -o coord-pilot ./cmd/coord-pilot
./coord-pilot -data ./coord-pilot-data -ui ../pilot/ui -addr 127.0.0.1:0
```

La console affiche l'URL locale et le **token de session** (régénéré à chaque
démarrage, jamais journalisé). Le navigateur n'accepte que `http://127.0.0.1:…/`.
Collez le token dans l'écran « Tableau de bord ».

## Écrans

1. **Prise en main** : déroulé d'une mission en 6 étapes, légende des états et rôles.
2. **Tableau de bord** : missions, états machine, arrêts, budgets (rafraîchis via `/api/missions`).
3. **Création de mission** : issue, adaptateurs simulés, exécutant ≠ réceptionnaire (N-05), plafonds.
4. **Suivi du cycle** : transitions normatives avec données (revision, empreintes, référence de verdict).
5. **Budgets** : compteurs par mission/adaptateur ; plafond dépassé = transition bloquée avant tout appel.
6. **Arrêt / levée** : commande humaine journalisée `ARRET_PILOTE`, distincte des `ECART` (N-20).
7. **Arbitrage / reprise** : divergence de copies, `github_fait_foi` (N-16), head vérifié (N-17).
8. **Sécurité** : rappels S1..S5, vérification `/health`.

Chaque refus s'affiche avec **motif explicite + trace**, l'état machine ne bouge
jamais après un refus. Le rôle PILOTE n'est attaché qu'à la session locale
authentifiée ; un champ `role` dans un corps de requête est ignoré (S4).

## Contrôles statiques

`python3 test-ui.py` (Python 3 stdlib) : T13..T16 — aucune ressource externe
(CSP `self`), bandeau simulé permanent, appels fetch typés `/api/*` et
`/health` uniquement, guide Prise en main complet. Rendu visuel complet dans un
navigateur : au pilotage du pilote local (NON_EXECUTE dans le sandbox de la
mission, réserves du rapport).

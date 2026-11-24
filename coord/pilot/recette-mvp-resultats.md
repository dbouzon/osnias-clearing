# Recette MVP — résultats d'exécution (M-COORD-PILOT-02)

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Base** : main `ec7dc4f1` (post-consolidation `8dbcefaf`) + fichiers nouveaux
uniquement de la branche `mission/coord-pilot-02`. **Exécutions REEL** sur code
distribué téléchargé, toolchain Go 1.27.4 linux/amd64 officielle installée en
séance, Python 3 stdlib. **Adaptateurs SIMULÉS** : aucun réseau sortant, aucune
clé, 0 €. Verdicts consignés ci-dessous, extraits complets dans `preuves-mvp/`.

## Moteur, habilitations, budget, arrêt

| Critère | Verdict | Preuve |
| --- | --- | --- |
| R1 transitions hors table fermée refusées avec motif | **OK** | tests `pilot_test.go` + scénario E2E R1.a/R1.b (409, motifs explicites, journalisés) |
| R2 habilitations mal formées → démarrage refusé (N-05) | **OK** | `TestS5EtR2HabilitationMalsaineRefuseDemarrage` |
| R3 dépassement plafond → blocage AVANT tout appel, motif journalisé | **OK** | `budget_test.go`, `TestR3PlafondBloqueAvantAppel`, scénario E2E (409 BUDGET_DEPASSE, état intact, aucun appel) |
| R4 arrêt pilote → transitions refusées, état intact ; levée → reprise | **OK** | `arret_test.go`, `TestR4ArretPiloteEffectif`, scénario E2E R4.1..R4.3 |
| R5 divergence → arbitrage `github_fait_foi` (N-16), reprise conditionnée au head (N-17) | **OK** | `TestR5DivergenceArbitrageReprise`, scénario E2E R5 (refus « sha_head divergent » en cas de mismatch, sha_head attendu obligatoire) |
| R6 verdict sur preuves modifiées → refus (N-13) | **OK** | `TestR6VerdictPerimeRefuse`, scénario E2E (« verdict perime », ACK sur preuves courantes passe) |

## Non-régression (R7)

| Batterie | Verdict |
| --- | --- |
| Python v1.0..v1.4 : 24+13+11+13+17 = 78/78, robustesse 10/10 (= 88/88) | **OK** |
| Python v1.5 durcie : 47/47 | **OK** |
| Vecteurs de conformité Python : 22/22 | **OK** |
| Démo v1.5 : exit 0 | **OK** |
| `go vet ./...`, `go build ./...` | **OK** (aucun diagnostic) |
| `go test ./...` module étendu (11 paquets testés) | **OK** (dont 5 nouveaux : adaptateurs, arret, budget, missions, pilot) |
| Vecteurs Go : 22/22, sortie sha256 `8c097f76…` **byte-identique** à la ligne de base #23 | **OK** |
| Démo Go : 19 CONFORME / 0 ECART, sha256 `e1f1cdbe…` **byte-identique** | **OK** |
| Maquette #25 T1..T12 (artefact intact) | **OK** 12/12 |
| Manifeste d'intégrité des fichiers historiques (vérifié par la batterie v1.5) | **OK**, aucun fichier existant modifié (contrôle sha256 main.zip ⇒ 0 modifié, 0 supprimé, 20 fichiers présents dans l'arbre de travail et absents de main, le présent fichier de recette et le rapport de mission étant les deux derniers ajoutés) |

## Sécurité locale (S1..S5, REEL, prix nul)

| Critère | Verdict |
| --- | --- |
| S1 sans token → 401 journalisé, aucune transition | **OK** |
| S2 token invalide/périmé → 401, aucune transition | **OK** |
| S3 écoute non loopback → démarrage refusé (0.0.0.0, LAN, :8080 testés) ; token vide refusé | **OK** |
| S4 rôle PILOTE non dérivable du corps (champ `role` ignoré, acteur résolu côté serveur, clôture forgée refusée) | **OK** |
| S5 habilitations vides/mal formées → démarrage refusé, motif journalisé | **OK** |

## Interface

| Critère | Verdict |
| --- | --- |
| T1..T12 contrôles statiques maquette | **OK** (artefact #25 intact, 12/12) |
| T13 aucune ressource externe (CSP self) | **OK** |
| T14 bandeau ADAPTATEURS SIMULÉS permanent + `<title>` | **OK** |
| T15 appels fetch typés `/api/*` et `/health` uniquement | **OK** |
| T16 guide Prise en main complet (6 étapes, rôles, N-16/N-17) | **OK** |
| E2E1..E2E6 scénario complet sur le vrai service (binaire + loopback + journal) | **OK** 25/25 |
| Rendu visuel complet dans un navigateur du poste pilote | **NON_EXECUTE** (aucun navigateur dans le sandbox de la mission ; contrôles dynamiques REEL via HTTP, rendu à la discrétion du pilote local) |

## Empreintes des preuves (REEL)

- `preuves-mvp/trace-recette-python.txt` sha256 `f64e7f4f0751df4e7724fa9d122ced28269682386d5044de910e588f77e000a4`
- `preuves-mvp/trace-recette-go.txt` sha256 `7a4b902d84c8db9612e0d7d47b441b2ae8d4b72967e3a3da1fd08fb690dd6f9c`
- `preuves-mvp/trace-scenario-e2e.txt` sha256 `d47ef8c8c7f3cdb3e973f354d20120aaade9ab46e80ba3287d38170fa511137c` (BILAN SCENARIO E2E: OK=25 KO=0)

## NON_EXECUTE / SIMULE (réserves explicites)

- Adaptateurs SIMULÉS : aucune interopérabilité réelle n'est revendiquée.
- Rendu navigateur complet : NON_EXECUTE dans le sandbox (documenté ci-dessus).
- Go Windows/macOS : NON_EXECUTE en séance ; couverts par la CI existante
  (COORD qualification Go multi-OS verte à `2a19993`, non dépendante de cette PR —
  aucun fichier historique n'est modifié) ; à confirmer par la CI sur la branche.
- Aucun appel A2A/MCP : l'analyse `docs/analyse-a2a.md` est documentaire (REEL sur
  sources publiques, sans dépendance de code).

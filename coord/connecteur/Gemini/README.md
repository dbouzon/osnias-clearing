# COORD — Connecteur Gemini (M-COORD-LLM-02)

Connecteur Gemini pour COORD v1.5 (référence opérationnelle) : profil `gemini` au format de requête **compatible OpenAI** (endpoint OpenAI-compatible de Gemini), appelé via le rail A2A direct (`coord/go/internal/a2a`, dépendance [`a2a-go/v2` v2.9.0](https://github.com/a2aproject/a2a-go)).

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX : Apache-2.0.

## Statut

Mission M-COORD-LLM-02 (issue #48 du repo Osnias Clearing). Développement, qualification SIMULÉE (0 €, boucles locales uniquement) et réception indépendante réalisés ; **recette réelle modèle/echo NON EXÉCUTÉE** (autorisation écrite du pilote requise, voir `recette-reelle-gemini.md`).

- Implémentation : `coord/go/internal/connecteurs/gemini.go` (profil, raisonnement borné, finish stop-only) et `coord/go/internal/a2a/` (rail, adaptateur, agents factices, recette E1–E10 + démo E2E).
- Arbitrages : A1 format `openai` + profil `gemini`, finish_reason **stop-only** (tout autre motif refusé et journalisé), API native documentée mais hors tranche ; A2 `raisonnement` borné {none, minimal, low, medium, high}, défaut `low` journalisé, valeur inconnue refusée au chargement ; A3 modèle exact requis + `MODELE_DIVERGENT` inchangé ; Q4 résolution de tâche incertaine **réservée au pilote** (`ResoudrePilote`, SEC-01).
- Rapport : `RAPPORT.md` (FR) / `RAPPORT.en.md` (EN).

## Sécurité

- **Aucune clé API versionnée** : la clé passe par la variable d'environnement `COORD_CLE_GEMINI` (fichiers `.env` ignorés).
- Rail A2A : loopback imposé (`APPEL_REEL_NON_AUTORISE` sinon), jeton `X-Coord-Rail-Jeton` exigé, registre durable 0600 sans secret, artifacts capturés à la délégation, budget 0 € par défaut (voûte monétaire fail closed).

## Fichiers

| Fichier | Rôle |
| --- | --- |
| `exemple-config-gemini.json` | Exemple de configuration SANS SECRET (variable d'environnement uniquement) |
| `recette-reelle-gemini.md` | Procédure de recette réelle modèle/echo, NON EXÉCUTÉE |
| `RAPPORT.md` / `RAPPORT.en.md` | Rapport de mission FR/EN : arbitrages, recette SIMULÉE, mesures, différences API native vs compatible |

Documentation de travail : https://drive.google.com/drive/folders/1vnuFKF5hEqqVbj_l7VUWSBXVrRj75i3t

Présentation : https://www.osnias-clearing.com/coord/

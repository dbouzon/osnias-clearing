# Recette réelle Gemini (modèle/echo) — NON EXÉCUTÉE

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX : Apache-2.0.

Mission M-COORD-LLM-02, réserve unique du plan v2 (PLAN_ACCEPTE par OpenAI) : la recette **réelle** du connecteur Gemini (appel payant vers l'endpoint compatible OpenAI de Gemini) n'a **pas** été exécutée. Mandat à budget 0 € : aucune qualification réelle sans autorisation écrite préalable du pilote (Denis Bouzon).

## Conditions d'exécution (3 exigences de l'arbitrage A3)

1. **Autorisation écrite du pilote** sur l'issue #48 (montant plafonné, endpoint, modèle exact).
2. **Plafond monétaire armé explicitement** dans la mission (voûte fail closed : `APPEL_REEL_REFUSE` sinon).
3. **Jeu de requêtes fixes et empreintées** (sha256), enregistrées avant envoi ; toute divergence de modèle (`MODELE_DIVERGENT`) arrête la recette.

## Procédure

1. Charger la configuration avec `COORD_CLE_GEMINI` définie dans l'environnement (jamais versionnée), `modele` exact (ex. `gemini-2.5-pro`), `profil: gemini`, `raisonnement` dans {none, minimal, low, medium, high}.
2. Requête d'**echo modèle** : une requête minimale dont la réponse attendue est connue ; vérifier a) `finish_reason == "stop"` (tout autre motif : refus `FIN_NON_NOMINAL`) ; b) l'echo `model` renvoyé **identique au modèle demandé** (sinon `MODELE_DIVERGENT`) ; c) le décompte de jetons cohérent ; d) le coût réel ≤ l'estimation conservatrice réservée avant appel.
3. Enregistrer les preuves (requête empreintée, réponse brute, journal) dans le dossier Drive `coord` et clôturer la réserve sur l'issue #48 par le pilote uniquement.

## Justification `modelVersion` (hors tranche)

L'endpoint compatible OpenAI ne retourne pas le champ `modelVersion` de l'API native ; la recette réelle vérifie l'echo `model` (nom du modèle demandé). La vérification de version fine (`modelVersion`, ex. `gemini-2.5-pro-001`) relève de l'API **native**, documentée mais non implémentée dans cette tranche (arbitrage A1).

## Différences API native vs compatible (documentation, hors tranche)

| Sujet | API native Gemini | Endpoint compatible OpenAI (implémenté) |
| --- | --- | --- |
| Comptage jetons | `usageMetadata` catégorisé (prompt, thoughts, réponse, cache) | `usage` non catégorisé (le raisonnement est compté dans `completion_tokens`) |
| Version fine | `modelVersion` renvoyé | echo `model` seul |
| Motifs d'arrêt | natifs (`STOP`, `MAX_TOKENS`…) | `finish_reason` style OpenAI (`stop`, autres motifs refusés : stop-only) |
| Raisonnement | `thinkingLevel`/budget de réflexion natif | `reasoning_effort` (borné {none, minimal, low, medium, high}, défaut `low` journalisé) |
| Facturation réflexion | tokens de réflexion facturés **en sortie** | inclus dans `completion_tokens` → tarifs de sortie |
| Tarifs | USD par million de jetons, dépendants du modèle (ex. 2.5 Pro : entrée 1,25 $ cache-off, sortie 10 $), grille évolutive | **la configuration fait foi** : tarifs en centimes/1M dans la config, pas codés en dur |

La grille tarifaire évoluant, les tarifs vivent dans la configuration (`tarifs`), jamais dans le code.

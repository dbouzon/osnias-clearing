# Rapport M-COORD-RAIL-02 — Sélection d'une bibliothèque commune d'accès aux IA

Ordre v1.0 du 26/11/2026 (étude de mutualisation du rail), récentré par l'ordre v1.1 (sélection d'une bibliothèque), décision du pilote du 26/11/2026 (commentaire 7195050454) : HTTP/JSON transport interne cible. Protocole : COORD v1.5 / Specification 1.0. Exécutant Mistral, directeur/réceptionnaire OpenAI, pilote Denis Bouzon. Base : main 5e1d172eb761b6749737c8067a1b1fb04edb5baf. 0 € dépensé ; aucun appel payant ; aucune clé ; coord/go INCHANGÉ.

## 1. Architecture cible retenue (décision du pilote)

| Composant | Rôle |
|---|---|
| Go + SQLite | moteur COORD (inchangé) |
| HTTP/JSON | transport interne standard (cible) |
| LiteLLM Core (local, sans proxy) | mutualisation des API d'inférence (sous réserve de qualification) |
| SDK/runners officiels | exécution agentique par environnement |
| A2A | reste opérationnel pendant l'étude ; maintien comme interface externe = option, pas une migration autorisée |

## 2. Méthode

Statuts sourcés (NATIF/MUTUALISABLE/SPÉCIFIQUE/INDISPONIBLE/NON VÉRIFIÉ), critères objectifs C1..C6 (licences SPDX relevées dans les fichiers LICENSE des dépôts), distinction stricte inférence/exécution agentique, registre d'exclusions sourcé, statuts de preuve REEL-documentaire / REEL-sandbox / SIMULE / NON_EXÉCUTÉ. Boucle v1.5 complète : plan v1.1 PLAN_ACCEPTE (tour 1), contrôle É3 ACCEPTÉ (tour 1), décision pilote intégrée en É5.

## 3. Résultats

**T1 (v1.0, conservé)** : baseline REEL verte au SHA 5e1d172e (Python 24+10+13+11+13+17+47 PASS, Go 13 paquets + modules verts, vecteurs 22/22 sha 8c097f76…, démo 19 CONFORME/0 ECART sha e1f1cdbe…). Inventaire : mutualisées (rail, orchestrateur, voûte, gabarits, runner), spécifiques (520 l. de profils : anthropic 208, gemini 182, mistral 130, openai 0), redondances documentées.

**Sélection (É2, ACCEPTÉE)** : **LiteLLM SDK via litellm-core v1.111.0** (25/11/2026), 24 dépendances runtime directes (toutes permissives), MIT, provenance PyPI signée ; 7/8 environnements API. Le proxy est ÉCARTÉ (registre d'exclusions : service réseau contraire au fail-closed ; CVE-2026-35029/59823 + bypass OIDC relevés par Check Point/OpenCVE sur le composant proxy, corrigés). Alternatives Go documentées en repli (any-llm-go Apache-2.0, 8 mois sans release ; sashabaranov/go-openai Apache-2.0 mono-format OpenAI). **Agents : aucune bibliothèque mutualisée n'existe** ; briques officielles par environnement : Claude Agent SDK (MIT), cursor-sdk Python first-party + SDK Bridge (MIT, protocole sdk.v1 sur loopback HTTP/1.1), codex exec, runner Vibe COORD, Antigravity (NON VÉRIFIÉ).

**Matrice huit environnements** (publiée #60, commentaire 7194584835) : inférence NATIF/MUTUALISABLE partout sauf Cursor (pas d'API d'inférence autonome documentée, NON VÉRIFIÉ) ; agents fragmentés (Claude Code et Cursor CLI complets et documentés ; Codex exec complet ; Vibe supervisé par runner COORD ; Gemini CLI indisponible aux comptes individuels depuis le 18/06/2026, successeur Antigravity NON VÉRIFIÉ ; DeepSeek/Llama INDISPONIBLES en agents ; xAI NON VÉRIFIÉ).

## 4. Raccordement standard HTTP/JSON (conception, non développée)

Pont local `coord-litellm-bridge` : serveur HTTP/1.1 sur 127.0.0.1 uniquement (boucle locale, refus de toute autre interface), contrat versionné coord.llm.v1 (GET /ping, POST /v1/chat/completions), format de réponse OpenAI-compatible → **aucune modification du moteur Go** (configuration baseURL du HTTPEndPoint existant ; profil stop-only conservé).

Propriétés documentées (exigées par la décision du pilote) :
- **Authentification loopback** : jeton local fichier 0600 exigé des deux côtés, vérifié au chargement (fail-closed), refus sans jeton ; secrets par variables d'environnement au lancement uniquement.
- **Délais** : bornes de requête du côté Go conservées (délais/taille plafonnés existants) ; le pont ne gère AUCUN retry (le SDK sous-jacent non plus : seconde autorité de reprise interdite, conformément au précédent D2 #40).
- **Erreurs et non-masquage** : le pont propage le finish_reason EXACT, le message d'erreur fournisseur, l'identité du modèle réellement servi et l'usage détaillé par catégorie de jetons ; la normalisation du format n'élude AUCUNE information nécessaire aux contrôles COORD (identité authentifiée ≠ modèle déclaré conservé, stop-only vérifié sur le motif exact, refus de troncature).
- **Sessions/reprise** : pont sans état ; relance par le superviseur de sous-processus Go existant (liste blanche env, codes 0-3, SIGTERM→SIGKILL) ; appels en cours traités par l'orchestrateur (REPONSE_TARDIVE, reprise incertaine réservée au pilote).
- **Idempotence** : réservation avant appel et anti-doublon appel_en_cours INCHANGÉS côté Go ; le pont est idempotent par nature (sans état).
- **Usage et budget fail-closed côté Go** : la voûte monétaire reste l'AUTORITÉ budgétaire unique (réservation conservatrice avant appel, surcoûts après) ; les tarifs restent en configuration Go, jamais dans le pont ; litellm-core est un TRANSPORT, pas une autorité.

## 5. Lacunes et limites explicites

1. Compatibilité réelle de litellm-core avec chaque fournisseur : NON_EXÉCUTÉE (documentaire uniquement) ; qualification réelle = recette distincte, triple verrou, autorisation écrite du pilote (précédents #29/#47/#49).
2. Démonstrateur : NON_EXÉCUTÉ dans l'environnement de travail (installation de paquets interdite par l'infrastructure) ; un démonstrateur simulé sans la vraie bibliothèque ne prouverait que le contrat, déjà couvert par le pattern factice des connecteurs. À exécuter sur machine autorisée si le développement est décidé.
3. Antigravity (Gemini agents) : NON VÉRIFIÉ en intégration. xAI agents : NON VÉRIFIÉ. Cursor inférence : NON VÉRIFIÉ.
4. Ressources : runtime Python + pont estimé ~100-200 Mo RSS (ESTIMÉ, NON_EXÉCUTÉ) contre 13,2 MiB pour le rail Go actuel ; compatible VPS 2 Go mais à mesurer réellement avant engagement.
5. L'objectif « un ou deux pivots mutualisés » est atteint côté inférence (LiteLLM Core = 1), PAS côté agents (surfaces officielles par environnement) — limite du marché, documentée.

## 6. Recommandations

1. Conserver le statu quo Go (520 l. de profils) comme référence opérationnelle jusqu'à qualification complète du pont.
2. Si le pilote décide le développement : branche dédiée, paquet isolé coord/llmbridge, litellm-core épinglé par version exacte, tests factices loopback (pattern #49), qualification simulée puis réelle sur triple verrou.
3. Considérer le contrat coord.llm.v1 comme le standard interne HTTP/JSON pour tout futur raccordement (Cursor SDK Bridge = modèle architecturel).
4. A2A : inchangé pendant la qualification ; arbitrage de maintien comme interface externe réservé au pilote après retour d'expérience.

## 7. Traçabilité

Issue #60 : ACK 7193860597 ; T1 7194141872 ; plan v4 PLAN_ACCEPTE 7194149511 ; T2/T3 7194584835 ; T4 CONTINUE 7194603014 ; ordre v1.1 : ACK+plan v1.1 7194787709 (PLAN_ACCEPTE, consigné 7194831290) ; É2 sélection 7194812143, É3 ACCEPTÉ consigné 7194832815 ; É4 7194834314 ; décision pilote 7195050454. Sources documentaires S1..S31 citées dans les commentaires T2/T3 et É2.

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

# Inventaire des connecteurs COORD — présentation au front end

- **Date** : 27 novembre 2026
- **Base** : `main` `2f8e4ea678cde7e7999b55d692b1f9f4d2ce6098` (post-fusion PR #89 M-COORD-FRONT-01)
- **Objet** : recenser tous les connecteurs COORD raccordables ou présentables à un front end pour la connexion des fournisseurs et services, avec leur format, leur clé d'environnement, leur voie de raccordement et leur statut de qualification réelle.
- **Règle transverse** : aucune clé API ne figure jamais dans le dépôt ; chaque connecteur désigne sa clé par `cle_env` (variable d'environnement). Tout appel réel est verrouillé par le triple verrou (mode REEL dans la config + `COORD_MODE_REEL=1` + autorisation écrite du pilote) et par la voûte monétaire (réservation en centimes AVANT l'appel, arrondi haut, fail-closed).

Copyright © 2026 Denis Bouzon — ORCID [0009-0007-8894-8902](https://orcid.org/0009-0007-8894-8902) — SPDX: Apache-2.0

## 1. Connecteurs LLM du moteur Go (`coord/go/internal/connecteurs`)

Ces connecteurs sont les briques d'inférence du moteur COORD v1.5 / COORD Specification 1.0. Chaque fournisseur est déclaré dans la configuration (JSON, sans secret) par : `id`, `endpoint`, `cle_env`, `modele` (identité EXACTE exigée : un alias renvoyé diverge en `MODELE_DIVERGENT`), `format`, `profil` (additif), tarifs (`tarif_entree/sortie_centimes_par_million`, `tarif_requete_centimes`), bornes (`jetons_entree/sortie_max_par_tentative`), `retraits_max`, `timeout_ms`, `taille_max_reponse`.

| # | Fournisseur | Fichier | Format | Profil | Clé (env) | Modèle exact de référence | Qualification réelle |
|---|---|---|---|---|---|---|---|
| 1 | OpenAI | `connecteurs.go` (`HTTPEndPoint`) | `openai` (défaut) | — | `COORD_CLE_OPENAI` | `gpt-4o-mini-2024-07-18` | **QUALIFIÉE RÉELLE** (M-COORD-PILOT-03-REEL2, M-COORD-OPENAI-API-01 #69, M-COORD-OPENAI-VPS-01 #81 : relecteur sur le rail coord.llm.v1, ~1 centime) |
| 2 | Mistral | `mistral.go` | `openai` | `mistral` (stop-only) | `COORD_CLE_MISTRAL` | `mistral-small-latest` | **QUALIFIÉE RÉELLE** (M-COORD-MISTRAL-VPS-01 #78 : exécutant sur le rail coord.llm.v1, 121 jetons) |
| 3 | Anthropic | `anthropic.go` | `anthropic` (Messages API, `x-api-key` + `anthropic-version` 2023-06-01) | — | `COORD_CLE_ANTHROPIC` | à choisir à la qualification | **NON_EXÉCUTÉE** (recette réelle préparée : `coord/pilot/connecteurs/recette-anthropic-reelle.md`, tranche 1 = réceptionnaire/critique) |
| 4 | Google Gemini | `gemini.go` | `openai` (endpoint compatible OpenAI AI Studio) | `gemini` (stop-only strict, `reasoning_effort` borné {none, minimal, low, medium, high}, défaut `low` journalisé) | `COORD_CLE_GEMINI` | identité exacte exigée ; jetons de pensée facturés en sortie | **NON_EXÉCUTÉE** (recette réelle préparée : `coord/connecteur/Gemini/recette-reelle-gemini.md`) |
| 5 | LLMBridge (pivot mutualisé) | `llmbridge_e2e_test.go` + `coord/llmbridge/` | `openai` (baseURL = pont loopback) | — | `COORD_BRIDGE_JETON` (jeton du pont) | préfixe SDK, ex. `mistral/mistral-small-latest`, `openai/gpt-4o-mini` | Pont qualifié (recette 0 € PASS) ; fournisseurs réels qualifiés via le pont : **Mistral** et **OpenAI** (#78, #81) |

Remarques :
- Le format `openai` sans profil garde le comportement historique du moteur (contrôles de base) ; les profils `mistral` et `gemini` ajoutent le strict `finish_reason == stop` (toute autre fin refusée et journalisée).
- Un `format` inconnu est refusé au chargement (fail-closed), de même qu'un `profil` inconnu ou un `raisonnement` hors bornes.
- L'enveloppe budgétaire est réservée AVANT l'appel : `(1+retraits_max) × [coût(borne entrée)+coût(borne sortie)+requête]`, arrondi vers le haut ; le mode REEL derrière un endpoint loopback (le pont) réserve aussi des centimes (PR #80).

## 2. Voies d'exécution du serveur agent A2A (`coord/a2a-agent`, `cmd/coord-agent`)

Le serveur agent mutualisé expose chaque fournisseur comme agent d'inférence (cartes honnêtes, gabarits de verdicts ancrés `points_detail {severite, texte}`). Sélection explicite des 2 intervenants par `roles`/`NouvelleMissionExplicit`.

| Fournisseur (`-fournisseur`) | Inférence (critique de plan / réception) | Exécution outillée |
|---|---|---|
| `anthropic` | OUI (API Messages) | OUI — `VOIE:CODE` : Claude Code supervisé (runner sous-processus, `--permission-mode dontAsk` + deny settings, `--max-budget-usd`, registre de sessions verrouillé, M-COORD-LLM-05 #58) |
| `openai` | OUI (format openai) | NON (`CAPACITE_NON_IMPLEMENTEE`) |
| `gemini` | OUI (profil gemini) | NON (`CAPACITE_NON_IMPLEMENTEE`) |
| `mistral` | OUI (API, profil stop-only) | OUI — `VOIE:VIBE` : runner Vibe CLI supervisé (liste blanche env, export.json fail-closed, M-COORD-LLM-03 #54) |

## 3. Rails et protocoles de transport

| Rail | Localisation | Rôle | Statut |
|---|---|---|---|
| Rail A2A direct | `coord/a2a` (module autonome, a2a-go v2.9.0) | dialogue exécutant ⇄ contrôleur, `X-Coord-Rail-Jeton`, loopback imposé, idempotence, reprise pilote exclusive | Qualifié (RAIL-01 #50, 12 couples interop factices) ; encapsulation A2A-over-MCP = NO-GO court terme (#38) |
| Pont HTTP/JSON `coord.llm.v1` | `coord/llmbridge` (bridge.py loopback, backend LiteLLM Core épinglé) | transport interne cible de l'inférence ; raccordement moteur par simple baseURL | Déployé et qualifié sur le VPS (#78, #81) |
| Serveur MCP | `coord/go/cmd/coord-mcp` + `internal/mcpserver` | exposition LECTURE SEULE des missions : outils `coord_list_missions`, `coord_get_mission` ; jeton de lecture distinct du token console | Qualifié (M-COORD-MCP-01 #35) ; clients MCP réels NON_TESTÉS |
| Adaptateurs simulés | `internal/adaptateurs` | fail-closed, jetons 1000/250, refus des identités réelles ; bandeau « ADAPTATEURS SIMULÉS » dans l'UI Pilot | Permanents (recettes 0 €) |
| Serveur agent A2A | `coord/a2a-agent` | socle neutre + paquet fournisseur mince (voir §2) | Qualifié (LLM-03/04/05) |

## 4. Points de raccordement du front end

| Interface | Localisation | Ce qu'elle offre au front | État |
|---|---|---|---|
| Gateway `coord-front` | `coord/go/cmd/coord-front` | routes `/api/*` (session cookie HMAC HttpOnly SameSite=Strict, token console jamais exposé au navigateur), proxy vers coord-maïeutique, moteur factice de démo, interface `Dossiers` (§5) | Livré (PR #89 fusionnée) ; front statique `coord/front/` (5 écrans, 5 langues) |
| Service maïeutique | `cmd/coord-maieutique` | interface REST du parcours Accueil/Création/Cadrage/Ordre+Devis/Suivi ; route additive `GET /projets/{id}/fille` (lecture seule) | Livré (#42, #84/#89) |
| Service Pilot | `cmd/coord-pilot` | interface web locale de gouvernance (token console, /aic, budgets, arrêt/reprise, `coord-snapshot.json`) ; jeton de lecture distinct pour MCP | Livré (#27) |
| Pont ChatGPT (historique) | hors dépôt (sandbox) | `chatgpt "question"` / `chatgpt -s "session"` : sessions continues OpenAI pour le dialogue binôme | Opérationnel, hors périmètre front |

## 5. Connecteurs de documents / stockage (interface `Dossiers` du gateway)

| Connecteur | Localisation | Rôle | État |
|---|---|---|---|
| Dossiers factices | `coord-front/drive.go` | cadrage client : dossiers Drive du client, dossier d'entrée optionnel (absence journalisée), lecture seule ; refus d'évasion de racine | Livré (PR #89) |
| Google Drive réel via rclone | `rclone.go` (PR #91, branche `mission/coord-drive-01`, NON fusionnée à la date de cet inventaire) | lsf/cat/rcat/mkdir SANS shell, remote validé au démarrage, périmètre préfixé `dossier_drive`, timeout clampé, `POST /api/drive/document` = seul point d'écriture (opt-in), export du dossier de sortie à la transmission (option A) | Réceptionné ACCEPTEE par OpenAI, fusion réservée au pilote |
| Google OAuth (bouton du front) | `coord/front` | affiché en DÉMO | Décision pilote : plugin sur étagère (suite 80), pas d'implémentation maison |

## 6. Gouvernance commune à tous les connecteurs

- Fail-closed généralisé : schéma refusé = refus ; identité authentiquée ≠ modèle déclaré = `MODELE_DIVERGENT` ; usage absent/invalide = `JETONS_INCONNUS` ; secrets expurgés des journaux (`sk-…`, `AIza…`) avec `[EXPURGE]`.
- Voûte monétaire : réservation conservatrice avant chaque appel facturable, plafonds `PlafondCentimes` armés jusqu'au moteur, retraits 429/5xx bornés par `retraits_max`.
- Triple verrou REEL : mode REEL dans la config + `COORD_MODE_REEL=1` + autorisation écrite du pilote (fournisseurs, plafonds, date).
- Idempotence et reprise : clés d'idempotence par appel, `APPEL_EN_COURS_NON_RESOLU`, résolution des appels incertains réservée au pilote (SEC-01).
- Sécurité du mandat : aucune IA ne peut créer, autoriser ni reprendre une mission (audit contradictoire 22 tests, harnais pérennisés).

## 7. À faire côté pilote (au 27/11/2026)

- Fusionner la PR #91 (rclone Drive) pour compléter la ligne §5.
- Qualification réelle Anthropic et Gemini (recettes préparées, triple verrou + autorisation écrite).
- Choix du plugin « étagère » pour Google OAuth / Drive (suite 80) puis intégration derrière l'interface `Dossiers`.
- Révocation des clés dédiées aux essais réels (Mistral, OpenAI) — rappel des issues #78/#81.

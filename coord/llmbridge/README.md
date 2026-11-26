# COORD LLM Bridge — pont local HTTP/JSON (contrat `coord.llm.v1`)

Mission **M-COORD-LLMBRIDGE-01** (issue #62) — COORD v1.5 / Specification 1.0.
Copyright © 2026 Denis Bouzon — ORCID : <https://orcid.org/0009-0007-8894-8902> — SPDX Apache-2.0.

## Objet

Pont local HTTP/JSON qui mutualise l'inférence multi-fournisseurs via
**LiteLLM Core** (SDK Python, MIT, distribution `litellm-core` **sans proxy**)
et se raccorde au moteur Go existant **par simple baseURL**, sans aucune
modification du moteur. Le moteur Go (transitions, voûte monétaire,
anti-doublon, tarifs, budget, annulation, reprise) reste **seul maître** : le
pont est un transport **sans état**, redémarrable sans perte.

A2A reste opérationnel et inchangé : ce pont est une option additive.

## Architecture

```
moteur COORD (Go + SQLite)          pont coord.llm.v1           fournisseurs
┌────────────────────────┐   HTTP/JSON   ┌──────────────┐   SDK litellm-core
│ orchestrateur, voûte,  │ ────────────► │ bridge.py    │ ────────────────►  OpenAI
│ anti-doublon, budget   │  loopback     │ backend.py   │   (réel : API     Mistral
│ (inchangés)            │ ◄──────────── │ config       │    https ; CI :   Anthropic
└────────────────────────┘   OpenAI-compat └──────────────┘    factices loopback) Gemini
```

- `bridge.py` : serveur HTTP/1.1 loopback (stdlib uniquement côté pont),
  jeton Bearer obligatoire (comparaison à temps constant), liste FERMÉE de
  destinations, corps/délais bornés, erreurs structurées JSON, aucun retry,
  aucun fallback, pas de streaming en v1, secrets expurgés des journaux.
- `backend.py` : `FacticeBackend` (qualification locale, 0 €) et
  `LiteLLMBackend` (SDK réel, import différé : absent => `SDK_ABSENT`, jamais
  un faux succès ; `num_retries=0`).
- `contract/coord.llm.v1.schema.json` : contrat versionné (JSON Schema
  2020-12) — `GET /ping`, `GET /v1/models`, `POST /v1/chat/completions`
  (compatible OpenAI, sous-ensemble strict, `usage` et `finish_reason`
  transmis tels quels, jamais inventés).

## Contrat `coord.llm.v1`

| Route | Méthode | Réponse |
|---|---|---|
| `/ping` | GET | `{"statut":"ok","contrat":"coord.llm.v1","backend":...}` |
| `/v1/models` | GET | destinations autorisées (liste fermée) |
| `/v1/chat/completions` | POST | réponse compatible OpenAI (choices, usage, model RÉEL) |

Erreurs : `{"error":{"type":...,"message":...}}` avec types stables
(`JETON_INVALIDE`, `JSON_INVALIDE`, `DESTINATION_REFUSEE`, `CORPS_TROP_GRAND`,
`TYPE_INVALIDE`, `DELAI_DEPASSE`, `REPONSE_BACKEND_INVALIDE`, `SDK_ABSENT`,
`CLE_ABSENTE`, `ERREUR_SDK`, `ERREUR_BACKEND`, `ROUTE_INCONNUE`,
`METHODE_NON_AUTORISEE`, `INTERNE`). Tout modèle non préfixé par une
destination autorisée est refusé (`DESTINATION_REFUSEE`) : aucun routage
arbitraire.

## Installation reproductible (hors sandbox d'exécution)

```sh
python3 -m pip install "litellm-core==1.111.0"   # MIT, épinglé, sans proxy
export COORD_BRIDGE_JETON="$(python3 -c 'import secrets;print(secrets.token_urlsafe(32))')"
export COORD_CLE_OPENAI=...        # chaque clé par variable d'environnement
python3 coord/llmbridge/bridge.py --config coord/llmbridge/config.exemple.json
```

La configuration (`config.exemple.json`) ne contient **aucun secret** : chaque
destination désigne une variable d'environnement (`cle_env`). Un champ `cle`,
`token` ou `api_key` dans le fichier est refusé au chargement (fail closed).

## Raccordement au moteur Go (additif, optionnel)

Aucune modification du moteur : déclarer un fournisseur format `openai` dont
l'endpoint pointe sur le pont (loopback ⇒ aucun triple verrou requis) :

```json
{
  "id": "openai-via-pont",
  "endpoint": "http://127.0.0.1:8788/v1/chat/completions",
  "modele": "openai/gpt-4o-mini",
  "cle_env": "COORD_BRIDGE_JETON",
  "timeout_ms": 60000
}
```

Exemple complet : `coord/connecteur/LLMBridge/exemple-config-llmbridge.json`.
La démonstration E2E réelle (cycle complet critique → exécution → réception →
ACK_RECU, clôture refusée, redémarrage du pont) :
`coord/go/internal/connecteurs/llmbridge_e2e_test.go` (backend factice).
L'anti-doublon et la voûte Go s'appliquent **avant** tout appel réseau : le
pont ne prétend à aucune garantie exactly-once.

## Qualification — statuts tracés (REEL / CI / NON_EXECUTE)

| Couche | Où | Statut |
|---|---|---|
| Contrat coord.llm.v1 (21 tests, backend factice) | `test_llmbridge.py` | REEL, exécutable partout |
| Raccordement Go E2E (cycle complet) | `llmbridge_e2e_test.go` | REEL-sur-factice |
| SDK litellm-core réel vs 4 fournisseurs factices natifs | `test_ci_sdk.py` | CI (installation interdite dans le sandbox d'exécution) |
| DeepSeek / Meta / Grok | — | NON_TESTÉS v1 (couverture litellm documentée, lacune explicite) |
| Fournisseurs réels (clés, réseau) | — | NON_EXÉCUTÉE (qualification DISTINCTE, triple verrou + autorisation écrite du pilote) |
| VPS 2 Go / 72 h | `recette-vps-2go.md` | NON_EXÉCUTÉE sans environnement autorisé |

Mesures sandbox (backend factice) : démarrage ~127 ms, RSS ~22,8 Mo,
latence médiane ~0,8 ms (p95 ~0,9 ms) — à comparer au rail A2A (3,02 ms /
13,2 Mo, M-COORD-RAIL-01) ; mesures SDK (taille d'installation, RSS import)
ajoutées par la CI. **Aucune promesse de tenue sur 2 Go sans mesure.**

## Sécurité

- écoute `127.0.0.1` uniquement ; jeton obligatoire, comparaison à temps
  constant ; secrets uniquement par environnement, expurgés (`sk-…`, `AIza…`)
  des erreurs et journaux ;
- corps borné (2 Mo par défaut), `Content-Type` vérifié, délais bornés
  (lecture, backend) ; réponses backend invalides refusées (`usage` ou
  `finish_reason` absent) ;
- `num_retries=0` : aucun retry ni fallback implicite, la reprise
  appartient au moteur Go ; le pont ne journalise jamais les charges.

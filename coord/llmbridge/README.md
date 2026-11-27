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

### `prefixe_sdk` (destination, facultatif — M-COORD-MISTRAL-VPS-01 D2)

Le SDK litellm exige un préfixe fournisseur (`mistral/…`) alors que le moteur
Go contrôle l'IDENTITÉ EXACTE (`MODELE_DIVERGENT`, égalité stricte). Une
destination peut déclarer `"prefixe_sdk": "mistral/"` : le modèle demandé
(identité exacte, ex. `mistral-small-latest`) est transmis au SDK avec ce
préfixe, et la réponse du fournisseur est transmise **telle quelle**
(l'identité n'est jamais falsifiée). Champ facultatif : absent = comportement
historique inchangé. Exemple : déclarer la destination par l'identité exacte
du modèle, `"mistral-small-latest": {"api_base": "https://api.mistral.ai/v1",
"cle_env": "COORD_CLE_MISTRAL", "prefixe_sdk": "mistral/"}`.

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

## Scénarios factices par marqueurs (M-COORD-DEP-03)

Le `FacticeBackend` produit des scénarios pilotés par des MARQUEURS portés
par la charge (les charges sont composées par le moteur Go ; les marqueurs
n'affectent jamais le routage, l'authentification ou les bornes).

| Rôle | Marqueur dans la charge | Sortie |
|---|---|---|
| `factice/exec/…` | `MANDAT V1` | livrable V1 erroné, contient `SCENARIO:LIVRABLE_A_CORRIGER` |
| `factice/exec/…` | `MANDAT V2` | livrable V2 corrigé (sans marqueur de scénario) |
| `factice/exec/…` | (aucun) | livrable constant historique (figé par test) |
| `factice/recep/…` | charge contenant `SCENARIO:LIVRABLE_A_CORRIGER` | verdict `A_CORRIGER` + 2 points explicites, empreinte ancrée |
| `factice/recep/…` | `CRITIQUE DE PLAN` (sans scénario) | `PLAN_ACCEPTE` (historique) |
| `factice/recep/…` | (aucun) | `ACCEPTEE` + `verdict_id` (historique) |
| les deux | `MANDAT V1` ET `MANDAT V2` simultanés | refus explicite `MARQUEURS_CONTRADICTOIRES` (400) |

Règles : marqueurs exacts et injectifs (aucun n'est sous-chaîne d'un autre) ;
contradiction => refus, jamais un choix arbitraire ; sans marqueur =>
comportement historique inchangé. Toute évolution future : table + tests +
cette section dans la même PR.

### Implications de chaque marqueur

- `MANDAT V1` : l'appelant reçoit un livrable volontairement erroné afin de
  dérouler un cycle complet d'itération (livraison -> A_CORRIGER ->
  correction -> ACCEPTATION) ; aucune autre sortie n'est affectée.
- `MANDAT V2` : l'appelant reçoit le livrable corrigé, sortie sans marqueur
  de scénario ; c'est la phase 2 du même identifiant de mission.
- `SCENARIO:LIVRABLE_A_CORRIGER` (reception) : la réponse du réceptionnaire
  est un verdict A_CORRIGER avec exactement 2 points explicites, ancrés à
  l'empreinte du livrable reçu, référencé par `verdict_id` (le moteur Go
  exige une référence pour toute décision recevable) ; tout autre contenu
  de charge est ignoré.
- `MANDAT V1` + `MANDAT V2` : refus immédiat 400 MARQUEURS_CONTRADICTOIRES,
  aucune inférence produite (défense contre une charge contradictoire).
- Sans marqueur : comportement historique strictement inchangé (figé par
  tests de non-régression).

### Exemples de requête/réponse (protocole coord.llm.v1)

`MANDAT V1` (exécutant) :

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V1 ..."}]}
    -> 200 {"choices": [{"message": {"content":
         "... SCENARIO:LIVRABLE_A_CORRIGER ..."}}]}

`MANDAT V2` (exécutant) :

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V2 ..."}]}
    -> 200 {"choices": [{"message": {"content": "livrable V2 corrigé"}}]}

`SCENARIO:LIVRABLE_A_CORRIGER` (réceptionnaire) :

    POST /v1/chat/completions
    {"model": "factice/recep/openai-dep03", "messages": [
      {"role": "user", "content": "... SCENARIO:LIVRABLE_A_CORRIGER ..."}]}
    -> 200 {"choices": [{"message": {"content":
         "verdict: A_CORRIGER ; points: [2 points explicites] ;
          empreinte: <sha256 du livrable>"}}]}

`MANDAT V1` + `MANDAT V2` simultanés (refus) :

    POST /v1/chat/completions
    {"model": "factice/exec/mistral-dep03", "messages": [
      {"role": "user", "content": "MANDAT V1 ... MANDAT V2 ..."}]}
    -> 400 {"error": {"type": "MARQUEURS_CONTRADICTOIRES",
         "message": "..."}}

## Journal optionnel du pont

Variable d'environnement `COORD_BRIDGE_JOURNAL` = chemin d'un journal JSONL :
une ligne par inférence {horodatage, model, destination, statut,
type_erreur, duree_ms}. JAMAIS de charge, JAMAIS de jeton (modèle expurgé) ;
échec d'écriture signalé sur stderr, sans impact client. Usage : corrélation
des traces moteur ⇄ pont (M-COORD-DEP-03).

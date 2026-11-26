# Rapport M-COORD-LLMBRIDGE-01 — pont HTTP/JSON coord.llm.v1

Issue #62 — ordre v1.0 du 26/11/2026. Protocole COORD v1.5 / Specification 1.0.
Exécutant : Mistral. Directeur/réceptionnaire : OpenAI. Pilote : Denis Bouzon.
Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX Apache-2.0.
La version française fait autorité (traduction : rapport-m-coord-llmbridge-01.en.md).

## T1 — ACK, baseline, plan

- HEAD main relevé `ea37d4ea08a6d3e37115110cf91c9bfb2e061293` (« Rebuild COORD
  archive », enfant du merge #61). Branche `mission/coord-llmbridge-01`
  inexistante. Non-concurrence : PR ouvertes = brouillons AIC #5/#6/#13.
- Baseline REEL (tarball codeload épinglé) : Python 135 PASS (M1 PASS),
  Go moteur 13 paquets + a2a + a2a-agent verts, vecteurs 22/22 sha
  `8c097f7672d5…` byte-identiques #23, démo 19 CONFORME, démos conformes.
- fait déterminant : `HTTPEndPoint` format openai accepte un endpoint
  loopback http (SimuleLocal) → raccordement par simple baseURL, moteur intact.
- litellm-core 1.111.0 vérifié PyPI (MIT, wheel cp310-abi3 signé 41 Mo, 24
  deps directes, sans proxy). **Installation de paquets interdite dans le
  sandbox de l'exécutant** → le SDK est réellement installé et exécuté en CI.
- Plan v1 sha256 `fd71e8d6041c7e09adac28292dba0201b417b2d0153e33617128425f61835d3f`
  → **PLAN_ACCEPTE tour 1** (session pont `m-coord-llmbridge-01`), aucun
  point à corriger. Consignations : commentaires 7195455224 (ACK), 7195550327 (T1).

## T2 — Pont coord/llmbridge (fichiers nouveaux, rien modifié ailleurs)

- `bridge.py` : serveur HTTP/1.1 loopback stdlib, jeton Bearer obligatoire
  (hmac.compare_digest), liste FERMÉE de destinations (préfixe le plus long,
  refus `DESTINATION_REFUSEE`), corps/délais bornés, erreurs JSON structurées
  aux types stables, Content-Type vérifié, sans état, aucun retry/fallback,
  expurgation `sk-`/`AIza`. Fail closed au démarrage (jeton absent = refus).
- `backend.py` : `FacticeBackend` (verdicts ancrés empreinte, reproduction de
  `extraireEmpreinte` Go) et `LiteLLMBackend` (import RÉSULT → `SDK_ABSENT`
  503, `CLE_ABSENTE` 503 avant réseau, `num_retries=0`, `usage`/`finish_reason`
  transmis, jamais inventés).
- `contract/coord.llm.v1.schema.json` : contrat versionné JSON Schema
  2020-12 (`GET /ping`, `GET /v1/models`, `POST /v1/chat/completions`,
  compat OpenAI sous-ensemble strict, `model` réel transmis).
- `config.exemple.json` : destinations fermées, secrets INTERDITS dans le
  fichier (rejet au chargement), `cle_env` uniquement.
- `test_llmbridge.py` : **21 tests REEL-sur-factice** (contrat, auth,
  bornes, routage, erreurs, expurgation, config fail closed, cycle factice).
- `fournisseurs_factices.py` : 4 serveurs factices NATIFS (openai, mistral,
  anthropic /v1/messages, gemini generateContent) pilotables par scénarios
  (401, 429, 500, délai, troncature, modèle divergent, usage absent, JSON
  malforme).
- `test_ci_sdk.py` : 12 tests SDK RÉEL en CI (skip explicite NON_EXECUTE si
  litellm absent, jamais un faux SDK).
- `mesures.py`, `recette-vps-2go.md` (T5), `README.md` (FR fait autorité) /
  `README.en.md`.

## T3 — Raccordement Go additif

- **Aucune modification du moteur** : démontré par test E2E
  `coord/go/internal/connecteurs/llmbridge_e2e_test.go` (fichier NOUVEAU,
  seul ajout sous coord/go) : pont lancé en sous-processus réel, adaptateurs
  `HTTPEndPoint` pointés par baseURL loopback, **cycle complet** :
  critique PLAN_ACCEPTE → exécution → soumission → réception ACCEPTEE
  (verdict_id `factice-verdict`) → ACK_RECU → **CLOTURE_REFUSEE** → puis
  **redémarrage du pont** et nouvelle mission fonctionnelle (sans état).
  Anti-doublon avant réseau : démontré par les tests moteur existants
  (TestCrashPendantAppelAntiDoublons), rejoués en non-régression.
- Exemple de configuration Go : `coord/connecteur/LLMBridge/exemple-config-llmbridge.json`
  (SIMULE, sans secret, conforme aux champs `ConfigFournisseur`).

## T4 — Qualification tracée

- REEL local : 21/21 tests contrat PASS ; E2E Go PASS (0,54 s) ; non-régression
  complète : moteur 13 paquets verts, a2a + a2a-agent verts, Python 47/47 v1.5
  (M1 PASS), vecteurs 22/22 sha `8c097f76…` inchangé, `go vet` linux/windows/darwin OK.
- CI `coord-llmbridge-ci.yml` (matrix 3 OS) : `pip install litellm-core==1.111.0`
  puis test_llmbridge, **test_ci_sdk (SDK réel vs 4 fournisseurs factices
  natifs)**, mesures. Statuts au moment du push : voir le corps de la PR
  (mise à jour après passage CI).
- NON_EXÉCUTÉ : fournisseurs réels (qualification DISTINCTE sous triple
  verrou + autorisation écrite), DeepSeek/Meta/Grok (couverture litellm
  documentée, NON_TESTÉS v1, lacune explicite), VPS 2 Go/72 h.

## T5 — Mesures (sandbox, backend factice, 50 appels)

- démarrage du pont (processus complet → BRIDGE_PRET) : **127 ms**
- RSS pont : **22,8 Mo** (22 843 392 octets)
- latence POST /v1/chat/completions : médiane **0,81 ms**, p95 **0,91 ms**,
  min 0,70 ms, max 14,0 ms
- comparaison à conditions identiques (loopback) : rail A2A = 3,02 ms
  médiane / RSS client 13,2 Mo (M-COORD-RAIL-01) → le pont est plus léger
  en latence, plus lourd en RSS (processus Python dédié). Mesures SDK
  (taille installation, RSS avec import litellm) : CI.
- recette 72 h / 2 Go préparée (critères GO/NO-GO pré-déclarés) :
  NON_EXÉCUTÉE sans environnement autorisé. **Aucune prétention de tenue
  sur 2 Go sans mesure.**

## T6 — Livraison

Branche `mission/coord-llmbridge-01` depuis main `ea37d4ea` ; PR dédiée au
SHA exact ; empreintes sha256 des fichiers dans le corps de la PR ;
réception indépendante OpenAI au SHA ; verdict + ACK consignés dans #62.

## Limites déclarées

- Le pont ne garantit PAS exactly-once (sans état) : l'anti-doublon Go
  s'oppose avant tout appel réseau ; la reprise des appels incertains reste
  un acte du pilote (SEC-01).
- Pas de streaming en v1 (documenté). Délai backend : le fil Python n'est
  pas tuable, le dépassement est RÉPONDU (504) et le processus peut être
  redémarré sans perte (démontré par le test de redémarrage).
- Aucune compatibilité fournisseur réelle revendiquée sans test ; tout
  NON_EXECUTE est tracé. 0 €, aucune clé réelle, aucun appel réseau
  fournisseur hors loopback factice.

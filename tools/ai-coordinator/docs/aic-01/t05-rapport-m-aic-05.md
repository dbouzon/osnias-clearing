# M-AIC-05 — Raccordement sécurisé des API OpenAI ↔ Mistral (rapport)

Issue : #8. Branche : `mission/aic-05-relais-api` (base : `mission/aic-04-openai-api`, PR Draft). Date : 22/11/2026.

## Objectif

Rendre opérationnel un premier échange réel OpenAI API ↔ Mistral API via le coordinateur, sans simulation dans le relais, avec traçabilité SQLite et refus par défaut sans authentification.

## Modifications

- `tools/ai-coordinator/mistral_real.go` (nouveau) : adaptateur `RealMistral` réalisant `MistralAdapter`. `POST {APIURL}/chat/completions` (api.mistral.ai), modèle par défaut `mistral-small-latest`, clé `AIC_MISTRAL_APIKEY` en mémoire uniquement, jamais journalisée. Fail closed sans clé. `Call` pour le dialogue ; `Run` produit un rapport JSON strict pour une exécution de mission (une complétion, aucun accès au système de fichiers). Journalisation `call`/`error`/`response` avec statut, `X-Request-Id`, jetons, extrait borné.
- `tools/ai-coordinator/relay.go` (nouveau) : relais de messagerie authentifié, DISTINCT du webhook GitHub. `POST /aic/message` (JSON : `mission_id`, `message_id`, `from`, `body`) ; cycle : message → agent Mistral → réponse soumise à l'agent OpenAI → verdict structuré. Jeton `AIC_MESSAGE_TOKEN` en mémoire, vérifié en comparaison à temps constante (401 sans jeton, 501 si le relais n'est pas construit) ; `NewRelay` refuse la construction sans jeton ou sans les deux adaptateurs réels (fail closed). Idempotence durable par `message_id` (table `events`, clé `aicmsg:`) : un doublon est rejeté sans aucun nouvel appel payant. Journal SQLite complet (`relay`, `mistral`, `openai`). Aucun secret renvoyé au client.
- `tools/ai-coordinator/config.go` : `AIC_MISTRAL_APIKEY`, `AIC_MISTRAL_APIURL`, `AIC_MISTRAL_MODEL`, `AIC_MISTRAL_MAX_TOKENS`, `AIC_MESSAGE_TOKEN`.
- `tools/ai-coordinator/webhook.go` : champ `Relay` sur le `Server`, route `/aic/message` enregistrée systématiquement mais répondant 501 tant que le relais n'est pas construit ; `NewServer` accepte le relais en paramètre optionnel (compatibilité des tests existants préservée).
- `tools/ai-coordinator/cmd/aic-coordinator/main.go` : `AIC_MISTRAL_MODE=api` construit l'adaptateur réel (le refus T03 est levé conformément à l'ordre M-AIC-05, uniquement avec clé fournie) ; le relais n'est construit que si les deux modes api ET `AIC_MESSAGE_TOKEN` sont fournis ; `AIC_MESSAGE_TOKEN` sans adaptateur complet = arrêt explicite.
- `tools/ai-coordinator/cmd/aic-dialog` (nouveau) : commande de test réel de bout en bout : message → Mistral API → OpenAI API, journalisation SQLite, sortie JSON (mission, réponse Mistral, verdict OpenAI). Fail closed sans les deux clés.
- `tools/ai-coordinator/mistral_real_test.go`, `relay_test.go` (nouveaux) : tests sur serveurs httptest locaux.
- `tools/ai-coordinator/env.example` : documentation des nouvelles variables, aucun secret.

Aucun changement du Core bancaire, des sidrs ou des autres modules. Le mode simulé par défaut est inchangé (stocké en mémoire, aucune régression vérifiée par les 43 tests existants).

## Tests

- `go build ./...` : réussi. `go vet ./...` : réussi.
- `go test -count=1 ./...` : 54/54 réussis (43 base + 11 nouveaux : Mistral fail closed, succès avec `X-Request-Id` journalisé et clé jamais journalisée, statut non-200, `Run` sans rapport structuré refusé, `Run` avec rapport JSON clôturé markdown, relais fail closed sans jeton/adaptateur, cycle complet avec journal SQLite de toutes les étapes, idempotence par `message_id` sans rappel d'API sur doublon, champs requis et émetteur inconnu refusés, HTTP 501 sans relais, HTTP 401 sans jeton, 405 en GET, cycle complet authentifié en HTTP et rejeu idempotent).
- `go test -race` : non exécutable dans l'environnement de développement actuel (cgo indisponible, SQLite sans cgo). Réserve inchangante, à exécuter sous Termux.
- Niveau de preuve : tests internes sur serveurs HTTP locaux simulés (niveau 1). Aucun appel réel vers api.mistral.ai ou api.openai.com n'a encore été effectué : aucune clé de test n'est disponible dans l'environnement de développement. Un HTTP 200 de `/healthz` ne préjuge en rien du dialogue (l'ordre le rappelle explicitement).

## Déploiement sous Termux (contrôles d'accès)

```sh
# 1. Clés et jeton dans l'environnement uniquement (jamais dans le dépôt) :
export AIC_WEBHOOK_SECRET=... AIC_AUTHORIZED_LOGINS=...
export AIC_MISTRAL_MODE=api AIC_MISTRAL_APIKEY=...
export AIC_OPENAI_MODE=api AIC_OPENAI_APIKEY=...
export AIC_MESSAGE_TOKEN=$(openssl rand -hex 32)

# 2. Test réel de bout en bout (prévoir les clés API de test dédiées) :
go run ./cmd/aic-dialog -mission aic-05-dialog -db ai-coordinator.sqlite

# 3. Serveur local avec relais activé (écoute locale par défaut) :
AIC_LISTEN_ADDR=127.0.0.1:8080 go run ./cmd/aic-coordinator

# 4. Échange authentifié :
curl -sS -X POST http://127.0.0.1:8080/aic/message \
  -H "Authorization: Bearer $AIC_MESSAGE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mission_id":"aic-05","message_id":"aic-05-001","from":"openai","body":"..."}'

# 5. Vérification de la trace :
sqlite3 ai-coordinator.sqlite "SELECT agent, phase, message FROM agent_logs WHERE mission_id='aic-05' ORDER BY id;"
```

Contrôles d'accès effectifs : jeton du relais requis et vérifié à temps constant ; sans `AIC_MESSAGE_TOKEN` ou sans les deux adaptateurs réels, la route répond 501 (nulle part un accès non authentifié) ; le webhook GitHub conserve sa signature HMAC séparée ; le relais et le webhook partagent le stockage mais aucun secret commun. Le Quick Tunnel Cloudflare (public et non authentifié) ne doit jamais exposer `/aic/message` sans ce jeton : le jeton constitue le contrôle applicatif exigé par l'issue #8 ; recommandation supplémentaire : ne publier que `/healthz` à travers le tunnel, le relais demeurant sur l'écoute locale.

## Conformité

- Aucun secret dans le dépôt : vérifié (clés factices de test uniquement, aucune valeur réelle).
- Refus des gardes-fous seulement via configuration explicite : les modes api et le relais exigent clé/jeton effectifs, sinon arrêt au démarrage (échec explicite).
- `/healthz` n'est utilisé comme preuve d'aucun dialogue.
- Idempotence et persistance durable avant tout appel payant : un incident n'entraîne ni doublon d'appel, ni perte de message (sémantique identique au webhook).
- OpenAI API et cette conversation ChatGPT restent des instances distinctes : aucun raccordement de la conversation n'est effectué dans cette tranche.

## Git

Publication : branche `mission/aic-05-relais-api` créée depuis `mission/aic-04-openai-api` (PR #7 non fusionnée : la PR M-AIC-05 est empilée, base `mission/aic-04-openai-api`), PR Draft distincte. SHA vérifié indiqué dans la PR. Aucune fusion, aucun déploiement automatique.

## Réserves

1. Le critère de réception de l'issue #8 (preuve d'appel aux deux API réelles, réponses corrélées en SQLite) n'est pas atteint à ce stade : aucune clé de test Mistral ni OpenAI n'est disponible dans l'environnement de développement. La procédure `aic-dialog` est prête et rejouable sous Termux.
2. `go test -race` à exécuter sous Termux (cgo requis).
3. Le raccordement de la conversation ChatGPT (issue #8, limites) nécessite un outil autorisé distinct, hors périmètre.

# M-AIC-04 — Premier message inter-IA par API (rapport)

Branche : `mission/aic-04-openai-api` (base : `main`, PR Draft).

## Objectif

Établir une communication réelle entre Mistral Code et OpenAI via Osnias AI Coordinator, sans simulateur, sans publication de secret et sans contournement des protections T03.

## Modifications

- `tools/ai-coordinator/openai_real.go` (nouveau) : adaptateur `RealOpenAI` réalisant l'interface `Reviewer`. Appel réel `POST {APIURL}/chat/completions`, clé d'API en mémoire uniquement, jamais journalisée. Fail closed sans clé. Réponse exigée en JSON strict `{decision, summary, comments}` ; verdict inconnu ou réponse non structurée = erreur explicite. Clôture markdown tolérée. Journalisation `call`/`error`/`response` avec statut HTTP, identifiant de corrélation `X-Request-Id`, jetons et extrait borné.
- `tools/ai-coordinator/config.go` : variables `AIC_OPENAI_APIKEY` (jamais journalisée), `AIC_OPENAI_APIURL`, `AIC_OPENAI_MODEL` (défaut `gpt-4o-mini`), `AIC_OPENAI_TIMEOUT` (défaut 60 s), `AIC_OPENAI_MAX_TOKENS` (défaut 400).
- `tools/ai-coordinator/cmd/aic-coordinator/main.go` : `AIC_OPENAI_MODE=api` active l'adaptateur réel (raccordement refusé en T03, désormais autorisé par l'ordre M-AIC-04), fail closed sans clé, journalisation SQLite via le Store existant. Mode inconnu = refus explicite.
- `tools/ai-coordinator/cmd/aic-openai-ping` (nouveau) : commande dédiée au premier message test de M-AIC-04. Envoie le message fixé, journalise l'envoi, l'appel, la réponse et l'erreur éventuelle dans SQLite, renvoie en sortie le modèle, la durée, la décision et le résumé.
- `tools/ai-coordinator/openai_real_test.go` (nouveau) : tests sur serveur `httptest` local.
- `tools/ai-coordinator/env.example` : documentation des variables, clé mentionnée uniquement en commentaire.

Aucune modification du Core bancaire ni des autres modules. Appels : `Authorization: Bearer` uniquement en mémoire, `User-Agent: osnias-aic-coordinator`.

## Tests

- `go build ./...` : réussi.
- `go vet ./...` : réussi.
- `go test -count=1 ./...` : réussi, 43 tests au total (36 sur `main` + 7 nouveaux : construction fail closed, succès avec verdict parsé et X-Request-Id journalisé, statut non-200, réponse non structurée, verdict inconnu, clôture markdown, construction du prompt Review).
- `go test -race` : non exécutable dans l'environnement de qualification (cgo indisponible, SQLite modernc sans cgo). Réserves inchangées.

Niveau de preuve : ces tests sont des tests internes sur serveur HTTP local simulé (niveau 1). Aucun appel réel vers `api.openai.com` n'a encore été effectué : aucune clé de test n'est disponible dans l'environnement d'exécution. L'exécution du premier appel réel (niveau 3) requiert une clé de test dédiée fournie par le responsable (variable d'environnement uniquement) ou une exécution sur la machine Termux de référence.

## Procédure du premier appel réel

```sh
export AIC_OPENAI_APIKEY='<clé de test dédiée>'
go run ./cmd/aic-openai-ping -mission aic-04-ping -db ai-coordinator.sqlite
```

Contrôle de la trace SQLite : `SELECT agent, phase, message FROM agent_logs WHERE mission_id='aic-04-ping' ORDER BY id;`.

## Conformité

- Aucun secret dans le dépôt : vérifié (clé uniquement en variable d'environnement, jamais persistée ni journalisée ; valeur factice `sk-test-never-log` dans les tests uniquement).
- Habilitations T03 préservées : mode api refusé sans clé, GitHub et Mistral inchangés.
- Aucun simulateur ne se substitue à l'appel réel : en cas d'échec, `aic-openai-ping` sort en erreur sans fabriquer de réponse.
- Limites Termux/Android : non couvertes dans cette tranche (build Go pur sans cgo compatible Termux ; qualification laissée au responsable).

## Git

Publication : branche `mission/aic-04-openai-api`, PR Draft distincte des PR #5 et #6. SHA vérifié indiqué dans la PR. Aucune fusion sans validation.

## Réserves

1. Le premier appel réel vers l'API OpenAI n'est pas encore exécuté : bloqué sur l'absence de clé de test. Critère de réussite de M-AIC-04 non atteint à ce stade.
2. `go test -race` non exécutable sans cgo.
3. La mitigation requiert `AIC_OPENAI_APIURL` si la clé est rattachée à une URL non standard.

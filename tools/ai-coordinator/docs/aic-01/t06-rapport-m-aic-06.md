# Rapport M-AIC-06 — Dialogue continu Mistral ↔ OpenAI (issue #10)

Date : 22 novembre 2026
Branche : mission/aic-06-dialogue-continu (base : mission/aic-05-relais-api, HEAD 7a04101d)
PR : Draft, jamais fusionnée sans validation du responsable.

## Objectif

Dialogue interactif continu depuis Termux, sans relancer le programme à chaque message (issue #10) : chaque saisie humaine déclenche un tour complet relais (saisie → API Mistral → API OpenAI → verdict affiché), historique conservé dans SQLite, arrêt simple (exit/quit) et arrêt propre Ctrl+C, bornes de consommation API (tours et jetons), vérification AVANT chaque appel.

## Préalable de l'issue (tests 007/008)

- Côté Mistral : cycle réel vérifié. Appels authentifiés vers api.mistral.ai, statut HTTP 200, réponses réelles reçues, trace SQLite complète (relay/receive, mistral/call, mistral/response). Clé de test valide employée uniquement en variable d'environnement locale, jamais journalisée ni publiée.
- Côté OpenAI : blocage de facturation. Statut HTTP 429 « credit_balance_exhausted » sur chaque appel (corrélation req_f9e02260949549fdaf6476549a1fda32). Le compte OpenAI ne dispose d'aucun crédit : action humaine requise, hors du périmètre de la tranche.
- Recommandation de sécurité : les deux clés de test ont été exposées en clair dans la conversation de coordination ; elles doivent être révoquées et remplacées après la phase de test.

## Modifications

- chat.go (nouveau) : ChatSession au-dessus du relais M-AIC-05. Fail closed sans relais ni mission. Bornes MaxTurns (défaut 20) et TokenBudget (défaut 20000) vérifiées AVANT tout appel d'API ; décompte pire cas par tour (MistralMaxTok + OpenAIMaxTok, défaut 1200) ; comptage conservateur (tour décompté dès l'engagement, erreur aval comprise). Identifiants de tours déterministes mission-tNNN (idempotence relais conservée, rejou détecté). Erreurs explicites ErrChatTurnLimit / ErrChatTokenBudget. Aucune boucle autonome : Say est appelée uniquement sur saisie humaine.
- cmd/aic-chat/main.go (nouveau) : boucle interactive terminal. Commandes exit/quit/help, prompt avec compteurs tours/jetons, arrêt propre Ctrl+C (fermeture de la base SQLite puis exit 0), construction identique à aic-dialog (fail closed sans les deux clés, exit 2). Affichage réponse Mistral + verdict OpenAI (décision, résumé, commentaires).
- config.go : champs SessionMaxTurns (AIC_SESSION_MAX_TURNS, défaut 20) et SessionTokenBudget (AIC_SESSION_TOKEN_BUDGET, défaut 20000).
- env.example : bloc M-AIC-06 documenté, sans aucun secret.
- chat_test.go (nouveau) : 6 tests.
- docs/aic-01/t06-rapport-m-aic-06.md : présent rapport.

Aucune modification du Core Osnias ni des composants hors périmètre.

## Tests (exécutés sur la branche mission/aic-06-dialogue-continu)

- gofmt -w . : propre.
- go build ./... : succès.
- go vet ./... : succès.
- go test -count=1 ./... : ok, 60 tests réussis (54 base M-AIC-05 + 6 nouveaux).
- go test -race : non exécutable dans la sandbox (cgo indisponible). Écart conservé ; sous Termux, -race n'est pas supporté sur android/arm64 (constaté au test 007) : écart également documenté.
- Nouveaux tests (niveau 1, serveurs simulés httptest, aucun accès réel) :
  - NewChatSession fail closed (relais nil, mission vide) ;
  - plafond de tours atteint : tour refusé AVANT tout appel, non consommé ;
  - budget de jetons dépassé avant appel ;
  - séquence d'identifiants aic-*-t001/t002 ;
  - cycle complet de 2 tours avec historique SQLite (deux receptions, deux appels Mistral, deux saisies conservées) ;
  - défauts 20 tours / 20000 jetons / 1200 pire cas par tour.

Distinction des niveaux de preuve conforme au §11 : les tests ci-dessus sont des tests internes simulés (niveau 1). Le demi-cycle Mistral réel est démontré (niveau 3 sur ce composant). Le dialogue réel complet Mistral ↔ OpenAI reste bloqué par l'absence de crédits OpenAI (429), il n'est pas annoncé comme réalisé.

## Exemple de session (anonymisé, extrait de l'exécution simulée des tests)

```
Osnias AI Coordinator — dialogue interactif (session aic-06-histo)
Bornes : 20 tours, budget pire cas 20000 jetons. Commandes : exit/quit, help.

[0/20 tours, ~0/20000 j] > Bonjour Mistral, premier tour de test.
[ tour 1 — message_id=aic-06-histo-t001 ]
Mistral : réponse de l'agent Mistral
OpenAI (ACCEPTED) : réception confirmée par OpenAI
[1/20 tours, ~800/20000 j] > Second tour : le dialogue continue sans relance.
[ tour 2 — message_id=aic-06-histo-t002 ]
Mistral : réponse de l'agent Mistral
[2/20 tours, ~1600/20000 j] > exit
session close par commande.
```

## Procédure d'exécution réelle sous Termux

Une fois les crédits OpenAI disponibles :

```
cd tools/ai-coordinator
export AIC_MISTRAL_APIKEY='<clé locale>'
export AIC_OPENAI_APIKEY='<clé locale>'
go run ./cmd/aic-chat -session aic-06-test -db ~/ai-coordinator.sqlite
```

Trace SQLite : aic-journal (cmd existant de la PR #9). Clés : jamais dans le dépôt, jamais dans les journaux, jamais dans la base.

## Réserves

1. OpenAI : aucun crédit sur le compte de test (429 credit_balance_exhausted). Le tour complet réel Mistral → OpenAI ne peut pas être exécuté tant que cette action humaine n'est pas levée ; aucune réception réelle d'OpenAI n'est donc revendiquée.
2. go test -race non exécutable en sandbox (cgo) ni sous Termux android/arm64.
3. Les clés de test Employées dans la conversation de coordination doivent être révoquées après la phase de test.
4. Le dialogue interactif réel n'est pas une conversation continue entre IA autonomes : chaque tour est déclenché par une saisie humaine, conformément au périmètre de l'issue #10.

## Git

- Branche : mission/aic-06-dialogue-continu, créée depuis mission/aic-05-relais-api (7a04101dc8570095e7b10fbab09c7a9932f4f35e).
- Publication : PR Draft (base mission/aic-05-relais-api), empilée sur #9 ; aucune fusion sans ordre.
- SHA du HEAD distant : consigné après publication dans la PR et vérifié par list_commits.

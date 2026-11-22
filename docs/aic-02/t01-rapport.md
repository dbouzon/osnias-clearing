# M-AIC-02 — Connexion Mistral Code au coordinateur Osnias

Tranche T01 de M-AIC-02 (22 novembre 2026). Base : branche `mission/ai-coordinator-v01`, HEAD ca4150f7916ee02cb295b3e5076c216e2aa21375.

## Objectif

Établir la communication opérationnelle entre Mistral Code et Osnias AI Coordinator : recevoir une mission, l'exécuter et retourner un résultat enregistré durablement.

## Mode de connexion retenu : adaptateur CLI

Choix retenu : adaptateur CLI (invocation non interactive du binaire Mistral Code par le coordinateur), plutôt qu'un serveur MCP. Motivation :

- Le modèle d'exécution du coordinateur est une tâche bornée démarrée par une commande humaine (`/aic start`), pas un service persistant interrogé par l'agent. MCP suppose un serveur durable et une inversion du contrôle incompatible avec le cycle mission → livraison Draft → revue.
- L'isolation est structurelle : répertoire de travail dédié par mission (`AIC_WORK_ROOT/<AIC-id>`), processus fils, environnement exempté de toute variable `AIC_`, ressources (deadline via contexte) et bornes existantes (budget d'étapes, plafond d'itérations) inchangées.
- L'interface `MistralAdapter` existante s'applique sans modification du cycle ni des tests : aucun second moteur n'est créé.

Contrat d'invocation : prompt de mission sur l'entrée standard (identifiant + objet, aucun contenu de commentaire exécutable) ; rapport lu sur la sortie standard, JSON structuré optionnel (`report`, `diff`, `steps`, `iters`), sinon sortie brute ; code de sortie non nul = incident (mission BLOCKED) ; sortie vide = incident. Authentification : `AIC_MISTRAL_API_KEY`, en mémoire uniquement, transmise au processus fils par `MISTRAL_API_KEY`, jamais journalisée ; absence de clé = refus au démarrage du démon et refus d'exécution (fail closed). Journalisation : chaque invocation consigne `start`/`done` (durée, étapes, itérations, taille du rapport) ou `error` dans le journal SQLite du Store (agent `mistral-code`) ; la sortie brute est conservée dans le répertoire isolé (`run-output.txt`).

## Modifications

- `tools/ai-coordinator/mistral_cli.go` (nouveau) : adaptateur `CLIMistral` réalisant `MistralAdapter`.
- `tools/ai-coordinator/config.go` : constante `ModeCLI`, champs `MistralBin`, `MistralArgs` (défaut `exec`), `MistralAPIKey` ; lecture `AIC_MISTRAL_BIN`, `AIC_MISTRAL_ARGS`, `AIC_MISTRAL_API_KEY`.
- `tools/ai-coordinator/cmd/aic-coordinator/main.go` : câblage `AIC_MISTRAL_MODE=cli` (mode `api` et modes inconnus refusés, échec explicite sans clé), journalisation branchée sur le Store.
- `tools/ai-coordinator/mistral_cli_test.go` (nouveau) : 6 tests.
- `tools/ai-coordinator/env.example`, `README.md` : documentation du mode CLI.
- Aucun autre module touché (Core bancaire, autres modules Osnias : intacts). Tests existants préservés (36/36 non-régression).

## Tests

Commandes exécutées dans la sandbox depuis la source de la branche distante (Go 1.26.0, tarball portable, sandbox sans cgo) :

- `go build ./...` : OK
- `go vet ./...` : OK
- `gofmt -l .` : aucun écart
- `go test -count=1 ./...` : 42/42 PASS (36 T02/T03 non-régression + 6 M-AIC-02)
- `go test -count=3 ./...` : 42/42 PASS (stabilité, 3 itérations)

Nouveaux tests M-AIC-02 (CLI substituée par un binaire de test local, sans réseau, sans commande arbitraire) :

1. `TestMAIC02RunContract` : prompt sur stdin, isolation, JSON décodé, trace conservée, journal sans exposition de la clé.
2. `TestMAIC02AuthFailClosed` : sans clé, exécution refusée, mission BLOCKED.
3. `TestMAIC02FailureBlocksMission` : code de sortie non nul, mission BLOCKED, journal d'incident.
4. `TestMAIC02FullCycle` : mission exécutée via la CLI, résultat durable (livraison Draft persistée + journal SQLite), mission en AWAITING_APPROVAL.
5. `TestMAIC02ResumeAfterInterruption` : mission RUNNING au verrou périmé (crash simulé), reprise par `Resume`, résultat durable après fermeture/réouverture de la base, sans double livraison.
6. `TestMAIC02EnvConfig` : lecture des variables dédiées, argument non interactif par défaut.

`go test -race` : non exécuté dans la sandbox (« -race requires cgo » ; gcc absent, installation interdite), à exécuter sur la machine Windows comme en T03.

## Opérations réellement exécutées

- Clone de la branche `mission/ai-coordinator-v01` (zip distant, HEAD vérifié ca4150f7 via l'API GitHub) ; développement et tests locaux ; publication sur la branche de mission dédiée avec PR Draft.
- Aucune modification GitHub réelle hors la branche de mission ; aucune fusion ; aucune dépense API (aucune clé réelle utilisée, aucune exécution payante).
- Les tests utilisent une CLI substituable locale : il s'agit d'une qualification contractuelle de l'adaptateur, pas d'une exécution réelle de Mistral Code.

## Réserves et limites

1. Interopérabilité avec la CLI Mistral Code réelle non qualifiée : le contrat (stdin/stdout, JSON optionnel, arguments du mode non interactif, variables d'authentification) devra être validé contre la CLI installée sur la machine cible, comme l'adaptateur GitHub réel en T03 n'a été qualifié que sur simulateur.
2. `go test -race` reste à exécuter sur la machine Windows.
3. Limites Androïd/Termux : l'adaptateur CLI suppose un binaire Mistral Code exécutable sur l'hôte du coordinateur. Sous Termux, l'installation d'une CLI Node/Go est possible mais non qualifiée ; les applications Vibe (Android/iOS) ne sont pas pilotables par API, constat durable de M-AIC-01. Le coordinateur (Go pur, sans cgo) compile théoriquement sous Termux, mais son mode CLI n'y a pas été testé ; la cible opérationnelle demeure le VPS UE prévu en T04.
4. Le mode `api` (APIxab) reste refusé conformément au périmètre.

## Git

- Branche de mission : `mission/aic-02-mistral-cli` (créée depuis `mission/ai-coordinator-v01` @ ca4150f7).
- Livraison : PR Draft vers `mission/ai-coordinator-v01`. Fusion suspendue à validation et autorisation explicite.
- SHA du HEAD publié : à vérifier dans le rapport de réception après publication (section complétée au moment de la publication).

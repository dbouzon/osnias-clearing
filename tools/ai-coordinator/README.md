# Osnias AI Coordinator (M-AIC-01 / T02)

Coordinateur multi-agents Osnias : relie GitHub (Issue = mission, PR Draft = livraison), un agent développeur Mistral (Vibe CLI en mode non interactif) et un agent chef de projet OpenAI. Développé en tranche T02 : prototype local, adaptateurs simulés, aucun appel d'API payant, aucune ressource cloud.

Le module est autonome (module Go distinct), indépendant du Core Osnias, portable Linux (SQLite pur Go via modernc.org/sqlite, sans cgo).

## Composants

| Fichier | Rôle |
| --- | --- |
| config.go | Configuration par variables d'environnement uniquement |
| statemachine.go | Machine d'états : OPEN, RUNNING, REVIEW, CORRECTION, AWAITING_APPROVAL, ACCEPTED, BLOCKED |
| store.go | Persistance SQLite : missions, événements (déduplication), transitions, journal des agents, livraisons |
| webhook.go | Récepteur HTTP des webhooks GitHub : HMAC-SHA256 à temps constant, déduplication de livraison, traitement asynchrone |
| coordinator.go | Orchestration du cycle : exécution Mistral bornée, livraison Draft, revue OpenAI, verdicts, reprise, anti-boucle |
| mistral.go | Adaptateur agent développeur (simulé en T02) ; l'adaptateur API refuse toute exécution tant que T03 n'est pas autorisé |
| openai.go | Adaptateur agent chef de projet (simulé en T02), verdicts structurés ACCEPTED / CORRECTION / BLOCKED |
| github.go | Interface GitHub du coordinateur (simulée en T02) + garde-fou contre les commandes arbitraires |
| cmd/aic-coordinator | Point d'entrée du démon |
| env.example | Configuration d'exemple sans secrets (dont AIC_AUTHORIZED_LOGINS : identités habilitées, fail closed) |

## Cycle d'une mission

1. `/aic start` (commentaire d'une Issue, auteur humain) : mission créée en OPEN, envoyée à l'agent développeur (RUNNING), livraison enregistrée en PR Draft, soumission à la revue (REVIEW).
2. Verdict de l'agent chef de projet : ACCEPTED (→ AWAITING_APPROVAL, validation humaine requise), CORRECTION (demande rattachée à la PR, plafond configurable, défaut 3) ou BLOCKED.
3. `/aic correction` relance une exécution ; `/aic approve` clôture la mission (ACCEPTED) ; `/aic block` la bloque.
4. Les écritures du compte du coordinateur sont ignorées (anti-boucle) ; les événements rejoués sont dédupliqués (X-GitHub-Delivery, persistant) ; le redémarrage ne duplique aucune exécution.

## Construction et tests

```sh
go build ./...
go vet ./...
go test -count=1 ./...
```

## Limites T02

Commandes réservées aux identités habilitées (AIC_AUTHORIZED_LOGINS, politique fail closed : liste vide = aucune commande acceptée). Adaptateurs Mistral, OpenAI et GitHub simulés uniquement ; l'adaptateur d'API refuse explicitement toute exécution (`AIC_MISTRAL_MODE=api` ou `AIC_OPENAI_MODE=api` provoque un arrêt en erreur). Le passage aux API réelles (T03) et au déploiement cloud (T04) requiert une autorisation distincte. Voir docs/deployment-linux.md et docs/security-report.md.

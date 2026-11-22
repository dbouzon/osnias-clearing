# Rapport de réception — M-AIC-01 / T02 : coordinateur multi-agents (prototype local)

Version 1.0 — 22 novembre 2026 · Développeur : Mistral Code · Réception : ChatGPT / agent OpenAI · Responsable : Denis Lamalice

## 1. Objectif

Développer et tester localement un coordinateur minimal en Go transmettant une mission GitHub à un agent développeur Mistral, puis soumettant son résultat à l'agent chef de projet OpenAI pour réception. Le coordinateur est un module autonome `tools/ai-coordinator`, indépendant du Core Osnias, portable Linux, sans appel d'API payant et sans ressource cloud.

## 2. Références Git

- Départ : `main`, commit `ad3a3a3aec4ecf0587e344149cac49d702901e6d` (fusion T01, PR #3).
- Branche de mission : `mission/ai-coordinator-v01`.
- État : intégralité du travail publiée sur la branche distante ; aucun travail non sauvegardé.

## 3. Fichiers livrés

| Fichier | Objet |
| --- | --- |
| config.go | Configuration par variables d'environnement uniquement (AIC_*) |
| statemachine.go | Machine d'états OPEN, RUNNING, REVIEW, CORRECTION, AWAITING_APPROVAL, ACCEPTED, BLOCKED |
| store.go | SQLite (modernc.org/sqlite, pur Go, sans cgo) : missions, événements dédupliqués, transitions, journal des agents, livraisons |
| webhook.go | Récepteur HTTP : HMAC-SHA256 à temps constant, déduplication X-GitHub-Delivery, traitement asynchrone |
| coordinator.go | Orchestration : exécution Mistral bornée, livraison Draft, revue OpenAI, verdicts, reprise, anti-boucle |
| mistral.go | Adaptateur développeur : SimulatedMistral (T02) ; APIMistral refuse toute exécution (T03 non autorisée) |
| openai.go | Adaptateur chef de projet : SimulatedOpenAI (T02) ; verdicts ACCEPTED / CORRECTION / BLOCKED ; APIMistral idem |
| github.go | Interface GitHub simulée (T02) + garde-fou contre les commandes arbitraires (AgentCommandGuard) |
| cmd/aic-coordinator/main.go | Démon : écoute locale, reprise au démarrage |
| coordinator_test.go | Tests couvrant les 10 scénarios T02-E et les garde-fous |
| go.mod / go.sum | Module Go autonome, dépendances épinglées |
| env.example | Configuration d'exemple sans secrets |
| README.md | Documentation du module |
| docs/deployment-linux.md | Guide de déploiement Linux (indicatif, T04 ultérieure) |
| docs/security-report.md | Rapport de sécurité T02-D |

## 4. Conformité à l'ordre de mission

T02-A (socle) : webhooks signés (temps constant), SQLite (missions, événements, états), déduplication, journal horodaté des transitions, reprise après interruption (resume testé), configuration par environnement ; états OPEN → RUNNING → REVIEW → CORRECTION → ACCEPTED avec BLOCKED et AWAITING_APPROVAL.
T02-B (adaptateurs) : runReviewSimulated / APIMistral et APIOpenAI refusent par conception toute exécution payante en T02 ; verdicts structurés ; capture des résultats et statistiques ; isolation du répertoire de travail par mission ; durée, itérations et budget bornés.
T02-C (GitHub) : Issue = mission, identifiant stable (AIC-&lt;numéro&gt;), PR Draft = livraison, demandes de correction rattachées, anti-boucle (écritures du bot ignorées), permissions de fusion hors périmètre.
T02-D (sécurité) : aucun secret versionné, garde-fou contre commandes arbitraires, plafond de corrections (défaut 3, dépassement → BLOCKED), arrêt sur timeout/budget, anti-rejeu persistant, exécutions isolées et journalisées, validation humaine obligatoire (AWAITING_APPROVAL → APPROVED). Détail complet : docs/security-report.md.
T02-E (tests) : les 10 scénarios de la liste sont couverts, y compris la reprise après redémarrage sans double exécution (déduplication persistante testée sur base réouverte).

## 5. Tests

Commandes exécutées dans l'environnement de développement (Go 1.24) :

- `go build ./...` : OK
- `go vet ./...` : OK
- `gofmt -l .` : aucun écart
- `go test -count=1 ./...` : OK, 10/10 PASS

Remarques et limites :

- `go test -race` n'a pas pu être exécuté : le détecteur de courses nécessite cgo et un compilateur C, non installable dans cet environnement ; à exécuter en T03. L'accès concurrentiel au stockage est restreint par conception (une connexion SQLite, traitement asynchrone séquentiel par événement).
- Tous les adaptateurs sont simulés (SimulatedMistral, SimulatedOpenAI, SimulatedGitHub) ; aucun appel réseau, aucune API payante, aucune ressource cloud.

## 6. Limites impératives respectées

Aucune modification du Core EVM ni des composants bancaires, des registres Manager, des connecteurs ERP, du site internet et du projet Eau-Énergie. Aucun serveur ni ressource cloud réservés. Aucune API payante consommée. Aucun secret installé. Aucune fusion dans `main` ni intervention sur le Core.

## 7. Réserves et suite

- `go test -race` à exécuter en T03 sur machine complète.
- Interopérabilité réelle non démontrée en T02 : les adaptateurs Mistral, OpenAI et GitHub sont simulés ; l'ouverture des accès API et la démonstration de bout en bout relèvent de T03, le déploiement de T04.
- Statut : T02 développée, testée et publiée sur `mission/ai-coordinator-v01` ; PR Draft ouverte vers `main` en attente de réception formelle et d'autorisation de fusion.

Décisions attendues du responsable : validation de T02, puis autorisation éventuelle de fusion de la PR Draft.

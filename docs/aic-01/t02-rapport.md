# Rapport de réception — M-AIC-01 / T02 : coordinateur multi-agents (prototype local)

Version 1.2 — 22 novembre 2026 (T02-R1 : levée des réserves de revue ; T02-R2 : correction des deux anomalies de revue) · Développeur : Mistral Code · Réception : ChatGPT / agent OpenAI · Responsable : Denis Lamalice

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
- `go test -count=1 ./...` : OK, 18/18 PASS (10 scénarios initiaux T02-E rejoués + 4 tests T02-R1 + 4 tests T02-R2)
- `go test -count=3 ./...` : OK, 18/18 PASS trois exécutions successives (stabilité)

Tests T02-R1 ajoutés :

- TestT02R1AuthorizationRefusal : un auteur non habilité ne peut démarrer, corriger, valider ni bloquer une mission ; politique fail closed (liste vide = tout refusé) ; approbation non habilitée inefficace.
- TestT02R1DeliveryIDAndStorageAck : identifiant de livraison absent refusé (400) ; échec réel de stockage répondu 503 sans assimilation à un doublon ; rejou de l'événement traité après rétablissement du stockage.
- TestT02R1ConcurrentExecutionExclusive : 8 exécutions simultanées d'une même mission ; exactement 1 passage Mistral, 1 livraison, 7 refus ErrMissionLocked ; verrou relâché en fin d'exécution.
- TestT02R1TransitionAtomicity : déclenchement d'un échec d'insertion au journal ; transaction annulée, état inchangé, aucune écriture partielle ; verrou atomique exclusif et reprise du verrou périmé.

Remarques et limites :

- `go test -race` n'a pas pu être exécuté : cgo et compilateur C indisponibles dans l'environnement T02 (tentative CGO_ENABLED=1 : « gcc not found ») ; à exécuter en T03. Un test de concurrence fonctionnel (8 exécutants simultanés) et un verrou d'exclusivité persistant couvrent la réserve en attendant.
- Tous les adaptateurs sont simulés (SimulatedMistral, SimulatedOpenAI, SimulatedGitHub) ; aucun appel réseau, aucune API payante, aucune ressource cloud.

## 6. Limites impératives respectées

Aucune modification du Core EVM ni des composants bancaires, des registres Manager, des connecteurs ERP, du site internet et du projet Eau-Énergie. Aucun serveur ni ressource cloud réservés. Aucune API payante consommée. Aucun secret installé. Aucune fusion dans `main` ni intervention sur le Core.

## 6 bis. T02-R1 — Levée des réserves de revue

Les quatre réserves de la revue T02-R1 (commentaire PR #4 du 22/11/2026) sont levées :

1. Autorisation GitHub : politique explicite d'identités habilitées (AIC_AUTHORIZED_LOGINS, fail closed), contrôle du sender authentifié GitHub, distinct du HMAC du webhook ; refus des commandes pour tout auteur non listé.
2. Acquittement et persistance : événement enregistré durablement AVANT la réponse 2xx ; X-GitHub-Delivery obligatoire (400 sinon) ; échec réel de stockage → 503 pour rejou GitHub ; aucune erreur SQL n'est assimilée à un doublon.
3. Exclusivité d'exécution : verrou atomique persistant (mission_locks) ; jamais deux passages Mistral ni deux livraisons pour une même mission ; reprise après crash documentée et réservée au démarrage (verrou périmé selon AIC_RUN_STALE_INTERVAL).
4. Atomicité des transitions : état et journal en une même transaction SQL, avec contrôle de concurrence sur l'état observé ; aucun état modifié sans journal.

Correction connexe : les fichiers github.go et mistral.go présentaient dans la version publiée initiale des caractères altérés lors de la publication (motif antérieur à la publication : une séquence d'échappement). Ils sont corrigés et republiés ; l'intégrité de tous les fichiers a été re-vérifiée par construction et tests complets depuis la source distante.

## 6 ter. T02-R2 — Correction des deux anomalies de revue

Les deux anomalies signalées par la revue du 22/11/2026 (T02-R2) sont confirmées et corrigées :

1. Anomalie A (reprise après interruption) : `ReclaimStaleClaim` actualisait `claimed_at` du verrou périmé sans supprimer la ligne ; le `ClaimMission` suivant (INSERT OR IGNORE) échouait donc silencieusement, et toute mission laissée RUNNING par un crash restait bloquée à chaque redémarrage. Correctif : `ReclaimStaleClaim` effectue désormais un DELETE atomique du verrou périmé (âge supérieur à AIC_RUN_STALE_INTERVAL) ; le verrou est ensuite ré-acquis normalement par `ClaimMission`. Les horodatages de verrous sont homogénéisés (RFC3339Nano).
2. Anomalie B (perte possible d'un événement après acquittement) : seul l'identifiant de livraison était persisté ; un arrêt du processus entre la réponse 2xx et le traitement asynchrone perdait définitivement la commande. Correctif : file d'attente durable — `SaveEvent` persiste le payload brut avec l'événement ; les événements non traités (`processed_at` vide) sont retraités au démarrage (`DrainPendingEvents`, appelé avant `Resume`) ; un événement traité ou sans effet (type non commandé, action ≠ created, auteur non habilité, payload illisible) est marqué traité et n'est jamais rejoué ; en cas de verrou actif ou d'erreur, l'événement reste en file pour le redémarrage suivant.

Tests T02-R2 ajoutés : TestT02R2ReclaimThenClaim (verrou périmé supprimé puis re-acquis), TestT02R2ResumeAfterCrash (mission RUNNING + verrou périmé : la reprise au redémarrage débloque réellement la mission), TestT02R2PendingEventQueue (événement acquitté non traité : payload restitué, mission exécutée au dévidage, file vidée), TestT02R2NoLossWhenEventNotCommand (les événements sans effet sont clos, la file ne s'engorge pas).

## 7. Réserves et suite

- `go test -race` à exécuter en T03 sur machine complète.
- Interopérabilité réelle non démontrée en T02 : les adaptateurs Mistral, OpenAI et GitHub sont simulés ; l'ouverture des accès API et la démonstration de bout en bout relèvent de T03, le déploiement de T04.
- Statut : T02, T02-R1 et T02-R2 développées, testées et publiées sur `mission/ai-coordinator-v01` ; PR Draft #4 mise à jour, en attente de la nouvelle revue.

Décisions attendues du responsable : validation de T02, puis autorisation éventuelle de fusion de la PR Draft.

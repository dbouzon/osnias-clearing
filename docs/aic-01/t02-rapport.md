# Rapport de réception — M-AIC-01 / T02 : coordinateur multi-agents (prototype local)

Version 1.5 — 22 novembre 2026 (T02-R1 à T02-R5 ; T02-R5 : clôture de l'idempotence SQLite ; audit documentaire des bibliothèques publié séparément) · Développeur : Mistral Code · Réception : ChatGPT / agent OpenAI · Responsable : Denis Lamalice

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
- `go test -count=1 ./...` : OK, 28/28 PASS (10 scénarios initiaux T02-E rejoués + 4 tests T02-R1 + 4 tests T02-R2 + 4 tests T02-R3 + 2 tests T02-R4 + 4 tests T02-R5)
- `go test -count=3 ./...` : OK, 28/28 PASS trois exécutions successives (stabilité)

Tests T02-R1 ajoutés :

- TestT02R1AuthorizationRefusal : un auteur non habilité ne peut démarrer, corriger, valider ni bloquer une mission ; politique fail closed (liste vide = tout refusé) ; approbation non habilitée inefficace.
- TestT02R1DeliveryIDAndStorageAck : identifiant de livraison absent refusé (400) ; échec réel de stockage répondu 503 sans assimilation à un doublon ; rejou de l'événement traité après rétablissement du stockage.
- TestT02R1ConcurrentExecutionExclusive : 8 exécutions simultanées d'une même mission ; exactement 1 passage Mistral, 1 livraison, 7 refus ErrMissionLocked ; verrou relâché en fin d'exécution.
- TestT02R1TransitionAtomicity : déclenchement d'un échec d'insertion au journal ; transaction annulée, état inchangé, aucune écriture partielle ; verrou atomique exclusif et reprise du verrou périmé.

Remarques et limites :

- `go test -race` n'a pas pu être exécuté en T02, T02-R2 ni T02-R3 : le détecteur exige cgo et un compilateur C (rejet explicite : « -race requires cgo »), indisponible dans la sandbox et non installable (installation de paquets interdite) ; à exécuter en T03 sur machine complète. Un test de concurrence fonctionnel (8 exécutants simultanés) et un verrou d'exclusivité persistant couvrent la réserve en attendant.
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

Tests T02-R2 ajoutés : TestT02R2ReclaimThenClaim (verrou périmé supprimé puis re-acquis), TestT02R2ResumeAfterCrash (mission RUNNING + verrou périmé : la reprise au redémarrage débloque réellement la mission), TestT02R2PendingEventQueue (événement acquitté non traité : payload restitué, mission exécutée au dévidage, file vidée et cloturée), TestT02R2NoLossWhenEventNotCommand (les événements sans effet sont clos, la file ne s'engorge pas).

Tests T02-R3 ajoutés : TestT02R3GitHubOpStates (états pending/sending/done de la table d'idempotence), TestT02R3NoDuplicateComment (commentaire renvoyé après fenêtre de crash : jamais dupliqué), TestT02R3DraftReused (livraison Draft interrompue : même référence, jamais dupliquée, livraison persistée réutilisée), TestT02R3PendingRetryLoop (relance à chaud : un événement bloqué par verrou est retraité en fonctionnement normal après libération du verrou).

Tests T02-R4 ajoutés : TestT02R4DraftRefRecoveredAfterCrash (référence de PR restituée depuis la base après interruption entre création et enregistrement, sans rappel de GitHub, y compris après redémarrage complet avec adaptateur vierge), TestT02R4IdempotenceAfterRestart (déduplication résistante au redémarrage complet : aucun doublon malgré un registre mémoire vierge).

Tests T02-R5 ajoutés : TestT02R5PersistFailureNoEffect (échec injecté d'écriture du registre github_sent : erreur propagée, aucun commentaire produit, mémoire non marquée ; rejeu avec persistance saine : exactement un commentaire), TestT02R5SeenKeyErrorPropagated (échec injecté de lecture du registre : erreur propagée par les trois opérations idempotentes, aucun effet produit), TestT02R5DraftPersistFailureNoCreation (échec de persistance avant création de PR Draft : aucune PR simulée créée ; rejeu après redémarrage complet avec persistance saine : une seule PR, référence restituée), TestT02R5RestartNoDuplicationAnyOp (rejeu après redémarrage complet d'opérations comment et review, dont une interrompue avant le marquage 'done' : aucun doublon, opération cloturée).

## 6 sexies. T02-R5 — Clôture de l'idempotence SQLite

1. Propagation des erreurs : `seenKey` propage désormais les erreurs de lecture du registre durable (on ne conclut jamais à l'absence d'une clé sur une erreur SQLite) ; `markSeenKey` retourne une erreur au lieu d'ignorer l'échec d'écriture ; les trois opérations idempotentes (`PostCommentIdem`, `CreateDraftPRIdem`, `RequestChangesIdem`) propagent ces erreurs à l'appelant (githubApply, donc au coordinateur).
2. Une opération n'est plus jamais considérée comme exécutée si sa persistance échoue : la clé est persistée durablement avant l'effet simulé et avant le marquage mémoire ; en cas d'échec d'écriture, aucun effet n'est produit, la mémoire n'est pas marquée et l'opération sera rejouée. Il ne peut plus exister d'opération simulée exécutée mais non enregistrée (source de duplication après redémarrage).
3. Atomicité locale : mécanisme write-ahead (persistance durable avant cache mémoire) pour le couple cache/registre, équivalent transactionnel pour ce couple ; les écritures multi-instructions du Store (état de mission + journal) restent transactionnelles (déjà en place depuis T02-R1) et les écritures d'idempotence sont mono-instruction donc atomiques au sens SQLite.
4. Tests : 28/28 PASS au total, dont 4 tests d'injection de panne (`failingKeyStore`), de redémarrage complet et de non-duplication (détail en section Tests T02-R5 ajoutés).

## 6 quater. T02-R3 — Traitement des deux risques résiduels

Ordre du responsable du 22/11/2026 : traiter les deux réserves de la revue de réception et exécuter go test -race, en tranche corrective distincte, avant toute intégration aux API réelles (T03).

1. Risque résiduel 1 (relance à chaud) : boucle permanente StartPendingRetryLoop, intervalle AIC_PENDING_RETRY_INTERVAL (défaut 60 s), arrêt propre à l'arrêt du coordinateur ; la file d'événements est retraitée périodiquement en fonctionnement normal, un événement bloqué par verrou ou en erreur n'attend plus le redémarrage ; complément : /aic start accepte de reprendre une mission RUNNING déverrouillée (l'exclusivité repose sur le verrou persistant, non sur l'état), sinon une mission laissée RUNNING par contention resterait en suspens jusqu'au redémarrage.
2. Risque résiduel 2 (duplication des écritures GitHub) : chaque écriture sortante porte une clé d'idempotence déterministe enregistrée en base avant l'envoi (table github_ops, états pending/sending/done) ; une opération 'done' n'est jamais renvoyée ; une opération interrompue entre l'envoi et le marquage est renvoyée sous la même clé et dédupliquée par l'adaptateur (interface IdempotentGitHub, réalisée par SimulatedGitHub en T02) ; une livraison Draft déjà persistée est réutilisée (DraftForMission) sans nouvelle création. Les adaptateurs API réels devront réaliser IdempotentGitHub sur ce modèle en T03.
3. go test -race : non exécutable dans l'environnement de développement — le détecteur exige cgo et un compilateur C (rejet explicite : « -race requires cgo; enable cgo by setting CGO_ENABLED=1 »), aucun compilateur C n'est disponible et l'installation de paquets y est interdite. Tentative CGO_ENABLED=0 refusée. Mesures compensatoires inchangées (verrou persistant, connexion SQLite unique, tests fonctionnels de concurrence) ; exécution requise en T03 sur machine complète.

## 6 quinquies. T02-R4 — Reprise des références de livraison et déduplication durable

Ordre du 22/11/2026 (revue T02-R3, validation partielle) : corriger la restitution de la référence de PR après interruption entre création et enregistrement dans deliveries, et démontrer l'idempotence après un redémarrage complet du processus, sans API réelle.

1. Référence de livraison récupérable : la référence de chaque PR Draft est persistée avec l'opération (colonne detail de github_ops, écrite au moment du marquage 'done') ; une opération 'done' restitue sa référence depuis la base sans rappeler l'adaptateur ; une opération 'pending' ou 'sending' est renvoyée sous la même clé. La fenêtre de crash « création PR effectuée, deliveries non renseigné » ne perd plus la référence.
2. Déduplication durable : l'adaptateur simulé branche sa déduplication sur le Store (interface KeyStore, table github_sent, table persistée) — la déduplication survit à un arrêt complet et reprise du processus ; le démon câble automatiquement ce registre à l'ouverture de la base. Les adaptateurs réels T03 devront réaliser la même vérification effective côté GitHub.
3. Résultat : la garantie « au plus une écriture GitHub par clé » est démontrée par des tests simulant le redémarrage complet (nouveau Store, adaptateur recréé vierge), l'absence de doublon n'étant couverte que par le registre durable.

## 7. Réserves et suite

- `go test -race` à exécuter en T03 sur machine complète (non exécutable dans la sandbox : cgo/compilateur C requis, installation de paquets interdite ; rejet documenté en section 6 quater).
- Interopérabilité réelle non démontrée en T02 : les adaptateurs Mistral, OpenAI et GitHub sont simulés ; l'ouverture des accès API et la démonstration de bout en bout relèvent de T03, le déploiement de T04 ; les adaptateurs API réels devront réaliser IdempotentGitHub (recherche avant création) pour garantir l'absence de doublon côté GitHub.
- Risque résiduel 1 (relance à chaud) : traité en T02-R3 (boucle AIC_PENDING_RETRY_INTERVAL, testée).
- Risque résiduel 2 (duplication GitHub) : traité en T02-R3 (clés d'idempotence persistées + déduplication adaptateur + réutilisation des livraisons Draft, testés), consolidé en T02-R4 (référence de PR restituée depuis la base, déduplication résistante au redémarrage complet, testés), puis clos en T02-R5 (propagation des erreurs SQLite, persistance avant effet, testés) ; la garantie complète repose en T03 sur la réalisation d'IdempotentGitHub par les adaptateurs réels.
- Statut : T02, T02-R1, T02-R2, T02-R3, T02-R4 et T02-R5 développées, testées et publiées sur `mission/ai-coordinator-v01` ; audit documentaire des bibliothèques Go (T02-R5, recommandation scénario B : go-github en T03, conservation de la persistance SQLite) publié dans docs/aic-01/t02-r5-audit-bibliotheques.md et validé par le responsable. PR Draft #4 mise à jour. Fusion suspendue à l'autorisation explicite du responsable.

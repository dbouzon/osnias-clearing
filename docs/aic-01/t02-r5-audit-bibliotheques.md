# Audit des bibliothèques Go — M-AIC-01 / T02-R5

Version 1.0 — 22 novembre 2026 · Branche `mission/ai-coordinator-v01` (base vérifiée : `8b8b224f077aac1c203d7f87d1964f72ca66f54a`) · Audit documentaire : aucun développement, aucune dépendance ajoutée, aucun accès aux API réelles.

## 1. Objet et méthode

Identifier si des bibliothèques Go existantes peuvent remplacer les développements spécifiques de l'AI Coordinator (gestion des événements, tâches persistantes, reprises après interruption, idempotence), et comparer trois scénarios : conserver l'implémentation actuelle, remplacer certains composants, ou adopter une bibliothèque d'orchestration complète.

Méthode : examen du code existant (`tools/ai-coordinator/`, environ 1 560 lignes hors tests, 24 tests) et consultation documentaire des projets cités (dépôts, documentation, registre pkg.go.dev) au 22 novembre 2026, sans installation ni exécution.

## 2. Tableau comparatif des bibliothèques

| Critère | go-github (google) | River (riverqueue) | GoForj queue (goforj) | orc (ella-to) |
|---|---|---|---|---|
| Objet | Client HTTP de l'API GitHub v3 | File de tâches persistante polyglotte | File de tâches multi-pilotes (reprises, délais, unicité, workflows) | Orchestrateur de workflows durable embarqué |
| Licence | MIT (fichier LICENSE du dépôt) | MIT pour l'édition open source (Pro payante séparée) | À confirmer sur le fichier LICENSE au moment de l'adoption | À confirmer sur le fichier LICENSE au moment de l'adoption |
| Maturité | Très élevée : plus de 3 100 commits, version v93.x, maintenance quotidienne (dernier commit 22/11/2026) | Élevée : plus de 1 000 commits, v0.51.2 (09/11/2026), maintenance très active | Faible à moyenne : projet de 2026, environ 280 commits, v0.3.0 (10/11/2026) | Très faible : v0.0.3, 17 commits, dernière activité 08/2026 |
| Support SQLite | Non pertinent (client HTTP) | Oui, mais explicite comme expérimental (cible principale PostgreSQL, pilote riversqlite) | Oui (pilote `driver/sqlitequeue`) | Oui (SQLite natif, module ella.to/sqlite) |
| Transactions atomiques (enqueue + effet) | Non applicable | Oui (argument central du projet, garanties transactionnelles avec la base) | Partielle (dépend du pilote) | Oui (persistance des états de workflow) |
| Reprise après interruption | Non applicable | Oui (états des tâches, retraits automatiques, reprises natives) | Oui (retries configurables) | Oui (workflows durables, GC) |
| Déduplication / unicité | Non applicable | Oui (tâches uniques, périmètre limité sur SQLite) | Oui (unicité déclarée en fonctionnalité) | Non documentée clairement |
| Empreinte de dépendances | Une seule dépendance directe majeure (golang.org/x/oauth2 pour l'auth) | Pilotes River, sérialisation, migrations ; empreinte notable | Pilotes multiples ; pilote SQLite et modules annexes ; empreinte à évaluer | Dépend de ella.to/sqlite et mattn/go-sqlite3 (CGO) — incompatible avec notre contrainte actuelle sans cgo |
| Compatibilité Go | Dernières versions de Go | Dernières versions de Go | Dernières versions de Go | Dernières versions de Go |

Évaluation d'alternatives matures : asynq (hibiken) et gue requièrent respectivement Redis et PostgreSQL ; machinery n'est plus maintenu activement. Aucune n'est retenue : elles contredisent la contrainte légère/SQLite ou la clause de maintenance.

## 3. Correspondance fonctions internes / fonctions réutilisables

| Fonction interne | Lignes (env.) | Couverture bibliothèque | Conclusion |
|---|---|---|---|
| `store.go` : file d'événements persistante (`SaveEvent`, `PendingEvents`, `MarkEventProcessed`) | 60 | River, goforj/queue (files persistantes) | Remplaçable, mais couplée au schéma et aux tests existants |
| `store.go` : registre d'idempotence (`EnqueueGitHubOp`, états pending/sending/done, `github_sent`) | 70 | Unicité de tâches River / goforj | Partiellement remplaçable ; notre registre encode la sémantique métier (restitution de référence) |
| `store.go` : verrous (`ClaimMission`, `ReleaseClaim`, `ReclaimStaleClaim`) | 40 | River (retraits automatiques) | Remplaçable en cas d'adoption complète |
| `store.go` : missions, transitions, journal (transactions) | 120 | orc (workflows) | À conserver : logique métier et machine d'états propres au projet |
| `coordinator.go` : boucle de relance (`StartPendingRetryLoop`), `Resume`, `githubApply` | 90 | River (retries natifs), orc (orchestration) | Remplaçable en grande partie, moyennant réécriture des tests |
| `github.go` : adaptateurs simulés, `IdempotentGitHub`, garde anti-commande | 100 | go-github en T03 (transport uniquement) | go-github remplace la couche HTTP future ; la déduplication et la recherche avant création restent à notre charge |
| `webhook.go` : HMAC, ingestion, rejeu | 196 | Aucune (standard library) | À conserver |
| `statemachine.go`, `config.go`, adaptateurs Mistral/OpenAI | 300 | Aucune | À conserver |

## 4. Limites de l'idempotence des opérations GitHub distantes

L'API GitHub v3 n'offre pas de mécanisme généralisé de clé d'idempotence applicable aux écritures (création de PR, commentaires). En conséquence :

1. Une bibliothèque ne garantit jamais l'unicité côté distant : seul le côté client peut la garantir (recherche avant création, références déterministes, registre local persistant).
2. Notre registre `github_ops`/`github_sent` et la restitution de référence (T02-R4) resteraient nécessaires même en adoptant go-github ou River.
3. Le simulateur n'utilisant pas le réseau, ces limites ne s'appliquent qu'à partir de T03 (adaptateurs réels), conformément au périmètre.

## 5. Évaluation des simplifications possibles

Scénario A — Conserver l'implémentation actuelle (avec la tranche corrective T02-R5 déjà ordonnée : propagation des erreurs SQLite de `markSeenKey`, écriture durable avant effet, tests d'injection de panne).

- Supprimable : rien. Dépendances ajoutées : aucune.
- Avantages : contrôle total, empreinte de dépendances nulle, tests existants intacts, conformité SQLite pur Go sans cgo, alignment avec le statut prototypal du module et l'exigence de dossier minimal.
- Inconvénients : la charge de fiabilisation reste à notre charge (fenêtres de crash, transactions, retraits de verrous).

Scénario B — Remplacement ciblé : adopter go-github en T03, conserver le reste.

- Supprimable : aucune ligne aujourd'hui (le transport HTTP n'existe pas encore) ; évite environ 200 à 300 lignes de client GitHub manuel en T03.
- Dépendances ajoutées : go-github et oauth2 uniquement en T03.
- Avantages : bibliothèque de référence, maintenance très active, auth, pagination, gestion des taux d'appels et des erreurs éprouvées ; l'idempotence client reste implémentée par nous comme prévu.
- Inconvénients : adopte le cycle de versions de go-github (ruptures mineures possibles entre versions majeures).

Scénario C — Adoption d'une bibliothèque d'orchestration (River) pour files, reprises et unicité.

- Supprimable : environ 250 à 300 lignes (file d'événements, registre d'idempotence, verrous, boucle de relance) sur 1 560.
- Dépendances ajoutées : River et son pilote SQLite (plusieurs paquets, migrations propres au projet).
- Inconvénients : support SQLite explicitement expérimental (cible principale PostgreSQL) — risque contre la stabilité recherchée ; réécriture d'environ la moitié des 24 tests ; perte temporaire de garanties en cours de validation. goforj/queue est trop jeune (v0.3.0) et introduit un facteur d'intégration (Temporal) hors du périmètre ; orc est une pré-version (v0.0.3, 17 commits) et repose sur mattn/go-sqlite3 (CGO), incompatible avec l'environnement actuel sans cgo.
- PostgreSQL ne se justifie pas : traitement mono-instance par cycle de mission, contrainte légère/SQLite, absence de dépense.

## 6. Risques transverses

- Licences à reconfirmer sur les fichiers LICENSE au moment de l'adoption (go-github : MIT établi ; River : open source MIT, édition Pro payante à ne pas introduire ; goforj et orc : à vérifier).
- Toute introduction de dépendance devra passer par validation humaine préalable, conformément aux règles du projet.
- Les projets jeunes (goforj, orc) présentent un risque d'abandon supérieur au risque résiduel de notre code, qui est petit, testé et sous contrôle.

## 7. Recommandation

Solution privilégiée : scénario B.

1. Exécuter la tranche corrective T02-R5 ordonnée (clôture de l'idempotence SQLite : propagation des erreurs, écriture durable avant effet, transactions locales atomiques, tests d'injection de panne), sans changement de bibliothèque : le code concerné est court, testé et le remplacement n'apporterait pas de garantie supplémentaire au stade du prototype.
2. En T03, utiliser go-github comme couche de transport des adaptateurs GitHub réels ; conserver notre registre d'idempotence, la recherche avant création et la restitution de référence.
3. Reconsidérer River uniquement si le module évolue vers plusieurs instances de traitement ou une base PostgreSQL, ce qui n'est pas le cas et ne le sera pas sans décision explicite du responsable.
4. Ne pas adopter goforj/queue (maturité insuffisante) ni orc (pré-version, dépendance CGO).

Réserves : audit documentaire effectué le 22/11/2026 sur la base des dépôts et documentations publics ; les versions, licences et statuts de maintenance cités doivent être reconfirmés au moment de toute décision d'adoption. Aucun code modifié, aucune dépendance installée, aucun accès réseau applicatif effectué hors consultation documentaire. La PR #4 demeure en Draft.

Sources principales (consultation du 22/11/2026) :
- https://github.com/google/go-github
- https://github.com/riverqueue/river et https://riverqueue.com/docs
- https://github.com/goforj/queue
- https://github.com/ella-to/orc
- https://pkg.go.dev/github.com/riverqueue/river, https://pkg.go.dev/github.com/goforj/queue/driver/sqlitequeue, https://pkg.go.dev/ella.to/orc

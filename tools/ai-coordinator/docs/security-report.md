# Rapport de sécurité — M-AIC-01 / T02

Établi le 22 novembre 2026. Périmètre : prototype local du coordinateur (tranche T02, adaptateurs simulés).

## Mesures implémentées

1. Vérification HMAC-SHA256 obligatoire de chaque webhook (comparaison à temps constant via hmac.Equal) ; sans secret configuré, tout est refusé ; signature invalide → 401.
2. Déduplication persistante des livraisons GitHub (X-GitHub-Delivery, clé primaire SQLite) : protection contre les rejouements et le double traitement, y compris après redémarrage.
3. Anti-boucle : les commentaires émis par le compte du coordinateur sont ignorés avant tout traitement.
4. Aucune commande arbitraire depuis GitHub : le contenu des Issues et commentaires passe un garde-fou (github.go, AgentCommandGuard) ; le coordinateur n'exécute aucun shell.
5. Garde-boucle de mission : occurrences limitées à AIC_MAX_CORRECTIONS (défaut 3, dépassement → BLOCKED) ; durée bornée (AIC_MISTRAL_TIMEOUT) ; itérations et budget de passages d'outils bornés (dépassement → BLOCKED).
6. Isolation des exécutions : répertoire de travail dédié par mission (AIC_WORK_ROOT), aucun accès à des ressources hors de ce répertoire en T02 ; exécutions et incidents journalisés (table agent_logs).
7. Aucun secret dans le dépôt : configuration exclusivement par variables d'environnement (env.example sans valeur réelle) ; aucun secret journalisé.
8. Droits GitHub minimaux prévus (T03) : lecture Issues et webhooks, écriture commentaires et PR Draft, jamais la permission de fusion ; la fusion reste une opération humaine hors du périmètre du compte bot.
9. Adaptateurs API verrouillés : en T02, AIC_MISTRAL_MODE=api ou AIC_OPENAI_MODE=api provoquent un arrêt en erreur explicite ; aucune consommation d'API payante possible depuis ce code.
10. Identités logiques distinctes : chaque action journalisée porte son émetteur (system, coordinator, mistral, openai, human).


## Mesures complémentaires T02-R1 (réserves de revue)

11. Habilitation explicite des auteurs : seules les identités listées dans AIC_AUTHORIZED_LOGINS peuvent émettre /aic start, /aic correction, /aic approve et /aic block ; politique fail closed (liste vide = refus de toute commande) ; vérification sur le sender authentifié GitHub, distincte du HMAC qui authentifie le webhook ; refus journalisés.
12. Acquittement durable : l'événement (X-GitHub-Delivery, obligatoire, sinon 400) est enregistré dans SQLite avant toute réponse 2xx ; en cas d'échec réel de stockage, réponse 503 afin que GitHub rejoue la livraison ; une erreur SQL n'est jamais assimilée à un doublon (INSERT OR IGNORE + comptage de lignes).
13. Exclusivité d'exécution : verrou atomique persistant (table mission_locks, INSERT OR IGNORE) ; une seule exécution Mistral et une seule livraison par mission à la fois ; les autres appels reçoivent ErrMissionLocked et s'abstiennent ; verrou relâché en fin d'exécution ; après crash, un verrou périmé (âge > AIC_RUN_STALE_INTERVAL) est repris uniquement au redémarrage (Resume, ReclaimStaleClaim).
14. Atomicité des transitions : mise à jour d'état et insertion au journal dans une même transaction SQL, avec relecture conflit (WHERE state = état observé) ; en cas d'échec, retour en arrière complet : aucun état modifié sans journal, aucune écriture partielle (testé par déclencheur d'échec).
15. Reprise réelle des verrous périmés (T02-R2, anomalie A de revue) : `ReclaimStaleClaim` supprime désormais la ligne du verrou périmé (DELETE atomique, âge supérieur à AIC_RUN_STALE_INTERVAL), puis le verrou est ré-acquis par `ClaimMission` ; la version antérieure actualisait `claimed_at` sans supprimer la ligne, ce qui laissait toute mission RUNNING définitivement bloquée après un crash ; horodatages de verrous homogénéisés (RFC3339Nano).
16. File d'attente persistante des événements (T02-R2, anomalie B de revue) : le payload brut de chaque livraison est persisté avec son identifiant au moment de l'acquittement (SaveEvent) ; un événement acquitté mais non traité reste dans la file (processed_at vide) et est retraité au démarrage (DrainPendingEvents, avant Resume) : aucune commande humaine n'est perdue en cas d'arrêt entre le 2xx et le traitement asynchrone ; un événement traité ou sans effet (type non commandé, action ≠ created, auteur non habilité, payload illisible) est marqué traité et jamais rejoué ; en cas de verrou actif ou d'erreur, l'événement reste en file pour le redémarrage suivant. Migration automatique des bases existantes (colonnes payload, processed_at).
17. Relance à chaud de la file d'événements (T02-R3, risque résiduel 1 de la revue) : la file est désormais retraitée périodiquement pendant le fonctionnement normal (boucle StartPendingRetryLoop, intervalle AIC_PENDING_RETRY_INTERVAL, défaut 60 s) ; un événement bloqué par un verrou d'exclusivité ou en erreur n'attend plus le redémarrage ; la commande /aic start accepte de reprendre une mission RUNNING déverrouillée, l'exclusivité restant garantie par le verrou persistant et non par l'état.
18. Idempotence des écritures GitHub (T02-R3, risque résiduel 2 de la revue) : chaque écriture sortante (commentaire, PR Draft, demande de changement) porte une clé d'idempotence déterministe, enregistrée en base (table github_ops, états pending/sending/done) avant l'envoi ; une opération 'done' n'est jamais renvoyée ; une opération interrompue entre l'envoi et le marquage (fenêtre de crash) est renvoyée sous la même clé et l'adaptateur déduplique (interface IdempotentGitHub, réalisée par SimulatedGitHub) ; une livraison Draft déjà persistée est réutilisée (DraftForMission) sans nouvelle création. Les adaptateurs API réels (T03) devront réaliser IdempotentGitHub sur le même modèle (recherche avant création) pour garantir l'absence de doublon côté GitHub.
19. Reprise de la référence de livraison et déduplication durable (T02-R4, réserve de la revue T02-R3) : la référence de chaque PR Draft est persistée avec l'opération (colonne detail de github_ops, marquée 'done' en même temps que l'opération) ; une opération déjà 'done' restitue sa référence depuis la base sans rappeler GitHub : une interruption entre la création de la PR et l'enregistrement dans deliveries ne perd plus la référence (githubApply prend en compte la fenêtre de crash). La déduplication de l'adaptateur simulé est rendue durable (interface KeyStore, table github_sent, adaptateur branché au Store dans le démon) : elle survit à un redémarrage complet du processus, un renvoi sur une opération interrompue ne produit aucun doublon même avec un adaptateur recréé vierge. Les adaptateurs réels T03 conserveront ce principe (idempotence effectivement vérifiée côté GitHub, recherche avant création base sur la clé).
20. Clôture de l'idempotence SQLite (T02-R5) : les erreurs SQLite du registre durable ne sont plus jamais ignorées — une erreur de lecture du registre github_sent est propagée par l'adaptateur (on ne conclut jamais à l'absence d'une clé sur une erreur), et la clé est persistée durablement AVANT tout effet et AVANT le marquage mémoire (mécanisme write-ahead équivalent à une transaction pour le couple cache/registre) : si l'écriture échoue, l'opération n'est pas considérée comme exécutée (aucun effet simulé produit, mémoire non marquée) et l'erreur est renvoyée, l'opération sera rejouée ; il ne peut donc plus exister d'opération simulée exécutée mais non enregistrée, source de duplication après redémarrage. Les écritures multi-instructions du Store restaient par ailleurs déjà transactionnelles (states+journal) ou mono-instruction donc atomiques. Quatre tests dédiés : propagation d'erreur de lecture, absence d'effet sur échec de persistance (commentaire et PR Draft), non-duplication au rejeu et après redémarrage complet pour les trois types d'opérations.

## Limites et réserves T02

- Le garde-fou lexical n'est pas une sandbox : en T03, l'exécution réelle de la Vibe CLI devra se faire en conteneur jetable, utilisateur dédié, sans réseau sortant autre que les API autorisées.
- Le webhook n'accepte en T02 que des connexions locales ; l'exposition publique (TLS, rate limiting du récepteur) relève de T04.
- `go test -race` n'a pas pu être exécuté en T02 ni en T02-R3 : le détecteur exige cgo et un compilateur C (refus explicite : « -race requires cgo »), aucun compilateur C n'est présent et l'installation de paquets est interdite dans l'environnement de développement ; à exécuter en T03 sur machine complète. Les accès concurrents au stockage sont restreints par ailleurs (une connexion SQLite, verrou d'exclusivité persistant, traitement asynchrone séquentiel par événement, tests de concurrence fonctionnels 8 exécutants).
- Risque résiduel 1 (revue T02-R2) : traité en T02-R3 (mesure 17, boucle de relance périodique AIC_PENDING_RETRY_INTERVAL, testée à chaud).
- Risque résiduel 2 (revue T02-R2) : traité en T02-R3 (mesure 18, clés d'idempotence persistées + déduplication côté adaptateur + réutilisation des livraisons Draft) ; réserves complémentaires de la revue T02-R3 (référence vide après interruption entre création et enregistrement, déduplication mémoire non résistante au redémarrage) traitées en T02-R4 (mesure 19) ; erreurs de persistance ignorées dans le chemin de déduplication (revue T02-R5) traitées en T02-R5 (mesure 20, propagation systématique, write-ahead avant effet) ; la garantie exactly-once complète suppose que les adaptateurs API réels réalisent IdempotentGitHub en T03.

## Conformité aux exigences de la direction

| Exigence T02-D | Statut |
| --- | --- |
| Aucun secret dans le dépôt | Conforme (env.example sans valeur réelle) |
| Droits GitHub limités au dépôt Osnias | Prévu par conception, applicable en T03 |
| Identités logiques distinctes | Conforme (journal par émetteur) |
| Aucune commande arbitraire depuis un commentaire | Conforme (garde-fou + aucun shell) |
| Limitation des corrections à trois | Conforme (défaut 3, configurable) |
| Arrêt sur dépassement de durée ou budget | Conforme (timeout, itérations, budget) |
| Protection contre les événements rejoués | Conforme (déduplication persistante) |
| Exécutions isolées et journalisées | Conforme (T02) ; conteneur à ajouter en T03 |
| Validation humaine obligatoire avant fusion | Conforme (AWAITING_APPROVAL, fusion hors périmètre du bot) |

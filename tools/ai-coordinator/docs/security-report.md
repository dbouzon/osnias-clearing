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

## Limites et réserves T02

- Le garde-fou lexical n'est pas une sandbox : en T03, l'exécution réelle de la Vibe CLI devra se faire en conteneur jetable, utilisateur dédié, sans réseau sortant autre que les API autorisées.
- Le webhook n'accepte en T02 que des connexions locales ; l'exposition publique (TLS, rate limiting du récepteur) relève de T04.
- Tests toujours exécutés sans détecteur de courses (`go test -race`) : cgo et un compilateur C demeurent indisponibles dans l'environnement de développement (tentative CGO_ENABLED=1 : « gcc not found ») ; à exécuter en T03. Les accès concurrents au stockage sont restreints par ailleurs (une connexion SQLite, verrou d'exclusivité persistant, traitement asynchrone séquentiel par événement, tests de concurrence fonctionnels 8 exécutants).
- Risque résiduel 1 (revue T02-R2, à traiter en T03) : un événement reste en file d'attente sans mécanisme de relance automatique pendant le fonctionnement normal ; la reprise n'a lieu qu'au démarrage (DrainPendingEvents). Si l'exécution échoue à chaud (par exemple verrou actif ou erreur passagère), l'événement n'est rejoué qu'au prochain redémarrage du coordinateur ; un fonctionnement sans surveillance permanente exige une boucle de relance périodique (à ajouter en T03).
- Risque résiduel 2 (revue T02-R2, à traiter en T03) : les opérations GitHub (création de PR Draft, publication de commentaire) et les transitions SQLite ne constituent pas une transaction unique ; un arrêt du processus entre ces opérations peut provoquer, au rejeu, une écriture GitHub en double (livraison ou commentaire dupliqué). Le prototype T02 ne garantit pas une exécution exactement une fois (exactly-once) : la garantie est au mieux at-least-once avec idempotence des transitions d'état, les écritures GitHub étant non idempotentes. T03 devra mettre en place une table des écritures sortantes (outbox) ou des identifiants d'idempotence côté GitHub.

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

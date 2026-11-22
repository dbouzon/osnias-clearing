# Rapport M-AIC-10 — Canal de discussion bidirectionnel ChatGPT ↔ Mistral Code (issue #12)

Date : 22 novembre 2026. Branche : mission/aic-10-canal-bidirectionnel (base : main, 74c5793e). PR Draft, aucune fusion sans validation.

## Objectif et réalisations

Transformer le module existant en canal bidirectionnel simple, sans refonte, avec routage ciblé, réponses libres, historique et contexte persistés.

**Corrections minimales apportées (trois fichiers, aucun autre modifié) :**
- channel.go (nouveau, ~190 lignes) : Channel au-dessus du relais M-AIC-05 existant. Deliver achemine chaque message vers UN SEUL destinataire (mistral ou chatgpt), sans jamais appeler les deux API. Validation fail closed (émetteur, destinataire, corps, émetteur ≠ destinataire), idempotence durable par message_id AVANT tout appel (même table events que le relais), historique et contexte relus depuis agent_logs (phase « channel », six derniers échanges, ordre chronologique) et inclus dans le prompt du destinataire, réponses libres sans verdict, aucune boucle autonome.
- openai_real.go : factorisation de l'appel HTTP partagé en postChat (bornage, journalisation, contrôles inchangés) ; les trois voies Review (verdict de revue), Call (verdict du relais M-AIC-05, inchangé) et Reply (réponse LIBRE du canal, invitation sans format imposé) réutilisent ce point unique. Aucun comportement existant modifié.
- channel_test.go (nouveau) : cinq tests.

Aucun nouveau serveur, aucune modification du Core bancaire ni des SIDIRS, aucune fusion sur main.

## Tests exécutés (sandbox Linux, Go 1.24/1.26, environnement recréé depuis main)

- gofmt, go build, go vet : propres.
- go test -count=1 ./... : 65/65 réussis (60 base + 5 nouveaux). Séquence de l'issue vérifiée sur serveurs simulés : « Bonjour Mistral. » (ChatGPT → Mistral, 1 appel Mistral, 0 appel OpenAI), « Bonjour ChatGPT. » (Mistral → ChatGPT, 1 appel OpenAI), « Nous pouvons discuter. » : destinataires, réception des messages, ordre chronologique de l'historique SQLite, contexte transmis au troisième tour, réponse libre non JSON restituée telle quelle, idempotence sur rejeu, persistance après réouverture de la base.
- go test -race : non exécutable dans la sandbox (cgo indisponible), écart habituel documenté.

## Démonstration réelle (niveau API, à ne pas confondre avec les sessions interactives)

Exécution réelle des trois messages de l'issue via le canal, API Mistral et API OpenAI réelles (clés en mémoire, jamais journalisées ; jamais dans le dépôt) :
- m001 ChatGPT → Mistral : statut ok, réponse réelle de Mistral.
- m002 Mistral → ChatGPT : statut ok, réponse libre réelle de ChatGPT (« Bonjour Mistral ! Comment ça va ? »).
- m003 ChatGPT → Mistral : statut ok, réponse réelle contenant l'historique complet des trois échanges, preuve du contexte transmis.
- Persistance vérifiée en base : trois lignes « channel » en ordre chronologique (de=chatgpt vers=mistral, de=mistral vers=chatgpt, de=chatgpt vers=mistral).

Ce test démontre la communication réelle entre les DEUX MODÈLES au travers du canal, en sémantique ChatGPT/Mistral déclarée. Conformément à l'issue, il ne prouve PAS la connexion des sessions interactives réelles.

## Le raccordement réel des sessions interactives : obstacles précis, non démontré

- ChatGPT interactif (application ou navigateur) : OpenAI n'expose aucune interface publique permettant à un programme d'injecter un message dans une conversation existante ni d'en lire les réponses ; l'accès programmatique passe uniquement par l'API (clé, facturation séparée des abonnements de conversation). Il n'existe donc pas de pont natif entre une session interactive ChatGPT et le canal. Les solutions observables publiquement (automatisation d'interface, agents pré-configurés importés dans un projet) contournent ou transforment la session ; elles ne la raccordent pas.
- Mistral Code (Vibe CLI) : le mode non interactif (--prompt, déjà intégré au module en M-AIC-02) exécute une tâche isolée ; il ne maintient pas de session de dialogue adressable par un autre programme. La session interactive de terminal n'a pas d'interface d'entrée externe.
- Conséquence : le canal fonctionne au niveau des modèles API, avec les identités chatgpt/mistral comme étiquettes sémantiques du routage, sans vérification possible que ces étiquettes correspondent à des sessions interactives réelles. Le préalable à un vrai raccordement serait une interface d'agent exposée par chaque fournisseur jusqu'ici inexistante ou non publique ; toute autre voie (double transcription humaine, copier-coller) reste manuelle et hors du périmètre du code.

Le présent rapport ne déclare donc la mission fonctionnelle qu'au niveau du canal et des API ; le raccordement réel des deux sessions est explicitement non démontré, obstacles ci-dessus.

## Git

Branche mission/aic-10-canal-bidirectionnel (base main 74c5793e), fichiers publiés : channel.go, channel_test.go, openai_real.go, présent rapport. PR Draft ; SHA distant vérifié après push, consigné dans la PR. Aucune modification de main.

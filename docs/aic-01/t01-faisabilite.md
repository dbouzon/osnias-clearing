# M-AIC-01 / T01 — Rapport de faisabilité Osnias AI Coordinator (OpenAI <-> Mistral)

Dépôt : dbouzon/osnias-clearing (branche main, HEAD e3d20118d65cca0a858eb3b05fda79618a7e2145, vérifié le 22/11/2026)
Date d'établissement : 22 novembre 2026, 14:08 Europe/Paris (UTC+1)
Tranche : M-AIC-01 / T01 — analyse et documentation uniquement. Aucun développement, aucune dépense, aucune réservation engage.

---

## 1. Objectif reconfirmé

Établir la faisabilité technique d'un coordinateur autonome sur serveur cloud européen, permettant à un agent OpenAI (chef de projet : ordres de mission, revue, réception) et à un agent Mistral (développement et tests) de communiquer automatiquement via GitHub, avec validation humaine obligatoire aux points de décision.

## 2. Constats sur les capacités réelles

### 2.1 Mistral — exécution non interactive

Capacités constatées (sources officielles) :

- Les applications Vibe (web, bureau, Android/iOS) sont des produits conversationnels interactifs. L'index complet des documentations officielles Mistral (docs.mistral.ai/llms.txt, consulté le 22/11/2026) ne référence aucune API permettant de créer, nourrir ou déclencher une conversation Vibe Work depuis un programme. En conséquence, une Issue GitHub ne peut pas déclencher Vibe Work Android : l'hypothèse d'automatisation par l'application Android est invalidée. Une telle API pourrait exister un jour ; elle n'existe pas au jour du rapport.
- La Vibe CLI (agent de code en ligne de commande) dispose d'un mode programmatique headless documenté : exécution d'une tâche unique et sortie (vibe --prompt "..." --max-turns N), sortie JSON ou streaming, répertoire d'export (export.json contenant usage, coût, raison d'arrêt), mode de confiance par invocation (--trust), approbations refusées par défaut en mode non interactif, agents dédiés avec permissions d'outils limitées. Ce mode est explicitement conçu pour le scripting et les tâches CI.
- L'API Mistral (api.mistral.ai, SDK officiels Python/TS) offre les complétions de conversation, le function calling et les sorties structurées ; la plateforme Studio offre Agents managés et Workflows avec déclencheurs d'événements et observabilité. Ces voies permettent un agent entièrement programmatique mais exigent de recoder les outils (git, tests) au lieu de tirer parti de la CLI.

Distinction demandée par l'Issue : l'application Android Vibe Work n'est pas pilotable ; un agent API/CLI indépendant (Vibe CLI headless ou SDK) est en revanche pleinement pilotable. Le vecteur recommandé pour l'agent développeur Mistral est donc la Vibe CLI en mode programmatique sur le serveur.

### 2.2 OpenAI — agent chef de projet

Capacités constatées :

- L'Agents API / Agents SDK d'OpenAI fournit l'orchestration d'agents (instructions, outils, MCP, handoffs multi-agents, sessions, compactage automatique du contexte) avec contrôle applicatif du runtime. La Responses API est l'API cœur sous-jacente.
- Limites : fenêtres de contexte bornées par modèle (mitigées par le compactage et par une gestion applicative de l'historique), facturation à l'usage nécessitant plafonds, aucune capacité native d'écoute de GitHub (l'agent est appelé par le coordinateur).

L'agent chef de projet est donc faisable comme appelant : le coordinateur lui soumet les événements et livrables, il produit ordres de mission, revues et réceptions sous forme de commentaires GitHub.

### 2.3 GitHub — mécanismes d'intégration

Constats :

- Webhooks d'événements : issues, issue_comment, pull_request, avec charge utile et signature HMAC ; un récepteur dédié est nécessaire.
- API REST : création et lecture d'Issues, commentaires, branches et Pull Requests, y compris PR Draft (requis par les limites de la mission).
- Permissions minimales : tokens à granularité fine (fine-grained PAT) limités à un dépôt et à des permissions explicites, ou GitHub App d'installation avec permissions minimales et short-lived tokens (préférable à terme).
- Points d'attention : l'émetteur du webhook (actor) doit permettre d'ignorer ses propres écritures, condition du garde-fou anti-boucle.

## 3. Inventaire des composants existants

Examen du dépôt au HEAD main e3d20118 (22/11/2026) : le dépôt contient le site FR/EN/ES, les modules node et end-user (testnet), la documentation business et docs/connectors. Aucun coordinateur, aucun orchestrateur, aucun module d'agent n'existe (« Et si ça existait déjà ? » : non). Réutilisable : les conventions documentaires du dépôt (structure docs/, rapports par mission) ; non directement réutilisable : node/node-admin-api (Node.js, périmètre authentification nœud, sans lien). Aucune double installation à prévoir, aucun conflit avec le Core EVM, les composants sidirs, les registres, connecteurs ERP ou le site (non modifiés).

## 4. Options comparées

| Option | Forces | Faiblesses |
| --- | --- | --- |
| GitHub Actions comme orchestrateur (workflow_dispatch/repository_dispatch appelant la Vibe CLI) | Sans serveur permanent, secrets gérés par Actions | Durée de tâche limitée, passerelles d'approbation humaine peu pratiques |
| Mistral Studio Workflows (déclencheurs d'événements) | Managé, observabilité intégrée | Logique de coordination hors du code du projet, contrôles fins et garde-fous Osnias plus difficiles à exprimer |
| Orchestrateurs tiers (n8n, Temporal, LangGraph) | Riche | Dépendances opérationnelles supplémentaires disproportionnées pour deux agents |
| Démon Go minimal (coordinateur) + Vibe CLI headless + API OpenAI | Simple, maintenable, cohérent avec la stack Go Osnias, contrôle total des garde-fous | Gérer soi-même l'écoute des webhooks et la reprise (faible : un récepteur HTTP + file SQLite) |

Recommandation : démon Go minimal. C'est l'option la plus simple et la plus maintenable ; elle place tous les garde-fous de sécurité exigés par la direction dans un code court, auditable et versionné dans le dépôt de mission.

## 5. Architecture recommandée (T02–T05 indicatif, non engagés)

- Récepteur de webhooks (vérification HMAC, réponses 200 immédiates, persistance des événements dans une file SQLite).
- Machine d'états par mission : une Issue étiquetée « mission » traverse les états ouverte -> assignée -> en cours -> livrée -> révisée -> à valider -> publiée / rejetée. Chaque transition générée par un agent est journalisée (horodatage, émetteur, SHA).
- Agent développeur Mistral : Vibe CLI en mode programmatique dans un clone éphémère, utilisateur UNIX dédié sans autre privilège, agent CLI avec outils limités (interdiction de git push dans la configuration du coordinateur : le CLI écrit, le coordinateur décide).
- Agent chef de projet OpenAI : convoqué par le coordinateur sur les événements de revue et de réception ; ses sorties sont déposées comme commentaires rédigés au nom du coordinateur (bot), jamais exécutées directement.
- Validation humaine obligatoire : aucune fusion, aucun push, aucune action destructrice sans commentaire d'autorisation explicite du responsable ou intervention dans GitHub ; les actions automatisées devront être formellement ordonnées avant tout développement, rien n'est lancé automatiquement par défaut.
- Isolation des exécutions : exécutions en conteneurs jetables (une tranche = un conteneur), timeout et max-turns bornés, suppression contrôlée des branches de mission.
- Contrôle des coûts : quotas par exécution et par jour ; capture de l'usage et du coût de chaque passage CLI (export.json) dans le journal.
- Reprise après incident : file SQLite idempotente (clé d'événement), redémarrage propre du démon sans perte, reprise depuis le dernier état publié.
- Garde-fous anti-boucle : ignorance des écritures du compte du coordinateur lui-même, idempotence par identifiant d'événement, plafond de passages par Issue et par heure, étiquette « run-en-cours » verrouillante.

## 6. Estimation indicative de ressources et coûts (sans engagement)

- Serveur : VPS européen 2 à 4 vCPU / 4 à 8 Go RAM, grille publique des hébergeurs européens, ordre de grandeur 6 à 15 EUR par mois. Aucune réservation effectuée.
- API Mistral : facturation au token (grille docs.mistral.ai/inference/pricing) ; ordre de grandeur par exécution de tranche en Vibe CLI : quelques dizaines de centimes d'euro, à confirmer empiriquement en T02 sur une seule tranche témoin.
- API OpenAI : facturation à l'usage ; ordre de grandeur comparable par revue, selon le modèle et la longueur du contexte, à confirmer en T02.
- Total indicatif en utilisation raisonnable : nettement inférieur à 50 EUR par mois, la quasi-totalité étant le VPS.

## 7. Risques

- Absence d'API pour les applications Vibe : risque principal, neutralisé par le choix du vecteur CLI.
- Gestion des secrets du VPS (clés API et token GitHub) : coffre local chiffré, droits restreints, aucun secret dans le dépôt (conforme aux règles).
- Dérive de boucle entre agents : traitée par les garde-fous du §5, à tester explicitement en T02 sur des cas de rejou et de doublon.
- Fiabilité d'un agent de code non supervisé : tout livrable transite par PR Draft et validation humaine ; aucune écriture directe sur main.
- Évolution rapide des produits cités : les constats sont datés du 22/11/2026 ; toute tranche ultérieure doit re-vérifier les documentations citées.

## 8. Critères d'acceptation proposés pour T02

1. Récepteur de webhooks vérifiant la signature HMAC, refusant les requêtes invalides.
2. Un ordre de mission réel sur une Issue, traité de bout en bout : événement -> passage unique de la Vibe CLI en mode programmatique -> rapport -> PR Draft -> revue par l'agent OpenAI -> arrêt sur demande de validation humaine.
3. Garde-fous démontrés : auto-ignoration des écritures du coordinateur, rejou d'un webhook sans double passage, plafond de passages respecté.
4. Journal complet : empreintes, incidents, usage et coût par passage.
5. Aucune opération sur main, aucun secret dans le dépôt, aucune dépense dépassant l'ordre de grandeur annoncé.

## 9. Sources officielles

- Index complet des documentations Mistral : https://docs.mistral.ai/llms.txt (consulté le 22/11/2026)
- Vibe CLI, mode programmatique : https://docs.mistral.ai/vibe/code/cli/work-with-cli
- Vibe CLI, documentation du mode programmatique : https://github.com/mistralai/mistral-vibe/blob/main/docs/programmatic-mode.md
- Vibe CLI, package et exemples d'agents : https://pypi.org/project/mistral-vibe/
- Comparatif CLI / VS Code / Web / Desktop : https://docs.mistral.ai/vibe/code/choose-cli-vscode-web-sessions
- Mistral, Function calling : https://docs.mistral.ai/inference/conversations/function-calling
- Mistral, tarifs API : https://docs.mistral.ai/inference/pricing
- Mistral Studio (Agents, Workflows, événements) : https://docs.mistral.ai/studio
- OpenAI, guide Agents : https://developers.openai.com/api/docs/guides/agents
- OpenAI, orchestration et handoffs : https://developers.openai.com/api/docs/guides/agents/orchestration
- OpenAI, définition d'agents : https://developers.openai.com/api/docs/guides/agents/define-agents
- GitHub, événements et charges utiles de webhook : https://docs.github.com/fr/webhooks/webhook-events-and-payloads
- GitHub, permissions des tokens à granularité fine : https://docs.github.com/fr/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens
- Dépôt examiné : https://github.com/dbouzon/osnias-clearing (main, e3d20118d65cca0a858eb3b05fda79618a7e2145)

## 10. Séparation constats / hypothèses

Constats (sources ci-dessus) : absence d'API de pilotage des applications Vibe ; existence et périmètre du mode programmatique de la Vibe CLI ; disponibilité des agents et réponses programmatiques OpenAI ; mécanismes GitHub cités. Hypothèses à confirmer en T02 : ordres de grandeur de coût réels ; tenue des garde-fous anti-boucle en conditions réelles ; qualité des livrables de la CLI sur le code Go du projet (aucun test interne n'a été exécuté dans le cadre de cette tranche documentaire ; seule une exécution réelle en T02 constituera une preuve).

# COORD Pilot — Dossier de conception

**Mission** : M-COORD-PILOT-01 (issue #25, dbouzon/osnias-clearing). Plan de conception v2, ACCEPTÉ par la critique OpenAI (session pont `m-coord-pilot-01`, verdict PLAN_ACCEPTE au 2ᵉ tour après correction des 5 points A_CORRIGER).

**Mission** : issue #25 (dbouzon/osnias-clearing), M-COORD-PILOT-01.
**Base de travail** : `main` = `a803b3c533ceba605bbd8795ed061e1c59bc85f9` (post-fusion PR #22). PR #24 vérifiée ouverte (tête `4f709ef172870daac927e5b366eaa145cdfae9f2`) : non touchée ; **aucune dépendance** aux fichiers de #23 ni à PR #24. Le présent plan n'en modifiera aucun.
**Statut v1.5** : candidate NON activée ; v1.4 référence opérationnelle. La mission l'utilise **à titre expérimental** sur son propre cycle, sans revendiquer son activation générale ni de conformité pour des scénarios simulés.
**Périmètre acté** : conception + maquette locale uniquement. Aucun déploiement VPS, aucun achat, aucun appel payant supplémentaire, aucun secret publié. Fusions et activations réservées au pilote.

---

## 1. Inventaire de l'existant (main a803b3c5)

### 1.1 Moteur Go `coordengine` (`coord/go/`, M-COORD-STD-01)

Module Go 1.27, stdlib uniquement, Apache-2.0. Paquets `internal` : `spec` (constantes normatives, 8 états, table fermée de transitions N-10, rôles EXECUTANT/RECEPTIONNAIRE/PILOTE, 5 actions), `policy` (habilitations fail-closed N-01..N-05, adhérence d'identité), `engine` (`Mission.Transition`, contrôle d'obsolescence `Perime` N-13), `jcs` (canonicalisation RFC 8785 + empreinte SHA-256), `persist` (écriture atomique N-15, journal idempotent N-19, simulation de crash), `resume` (détection de divergence, arbitrage `github_fait_foi` N-16, reprise conditionnée au head N-17), `ctxcontrol` (qualification des instructions N-18, écarts journaux, reprise après écart N-20). Binaires : `cmd/coord-demo` (démo autonome 19 contrôles) et `cmd/vector-runner` (22 vecteurs). Tests de paquets présents. Preuves existantes : 22/22 vecteurs et démo byte-identiques Python ⇄ Go (mission #23).

**Propriété structurante** : les paquets sont `internal` → un module Go externe ne peut pas les importer. Deux options : (A) COORD Pilot vit **dans le module coordengine** (nouveaux paquets/cmd, aucun fichier du moteur modifié) ; (B) exposé d'API publique du moteur = modification du moteur, interdit ici. **Le plan retient (A)**, extension par fichiers nouveaux uniquement.

### 1.2 `tools/ai-coordinator` (M-AIC-01, tranche T02)

Démon Go d'orchestration : machine à 7 états **divergente** (OPEN, RUNNING, REVIEW, CORRECTION, AWAITING_APPROVAL, ACCEPTED, BLOCKED), store SQLite (modernc.org/sqlite, sans cgo), récepteur de webhooks GitHub (HMAC-SHA256 à temps constant, déduplication), adaptateurs Mistral/OpenAI/GitHub **simulés** (l'API refuse par défaut, fail closed), anti-boucle, idempotence des écritures GitHub, commandes humaines `/aic ...` réservées aux logins habilités (`AIC_AUTHORIZED_LOGINS`, fail closed). Documenté « historique divergent », conservé intact.

**Réutilisable par inspiration, pas par code** : webhook sécurisé, anti-boucle, idempotence, fail-closed des habilitations. Le code ne peut pas être importé tel quel (états divergents, non conforme à COORD Specification 1.0).

### 1.3 Lignée Python (`coord/coord06*.py`)

Moteurs v1.0→v1.5 et batteries (88 tests jusqu'à v1.4, 47 v1.5, 22 vecteurs). Pour l'expérimentation v1.5 : `coord06_v15.py` + `tests_coord06_v15.py` + `run_v15.sh` sont les outils de référence du protocole v1.5 (critique plan → réalisation → réception), exécutables avec Python 3 local.

### 1.4 Écarts protocole ⇄ orchestration réelle

1. **Aucun orchestrateur conforme n'existe** : `coordengine` est un noyau de décision sans E/S (pas de référentiel multi-missions, pas de service, pas d'adaptateurs réels, pas d'interface) ; `ai-coordinator` orchestre mais sur des états divergents.
2. **L'orchestration réelle actuelle est conversationnelle** : les IA conduisent elles-mêmes le cycle via GitHub + sessions pont ; le pilote humain arbitre dans l'issue. Le « logiciel » est absent : rien n'empêche aujourd'hui une transition non autorisée autre que la discipline des IA.
3. **Budgets/coûts, arrêt humain effectif, permissions web** : absents des deux codebases et de la spec (hors périmètre volontaire du moteur). Ce sont les trous que COORD Pilot doit combler **autour** du moteur, sans le modifier.

---

## 2. Architecture proposée (COORD Pilot, conception locale)

### 2.1 Principe directeur

COORD Pilot = **couche d'orchestration et de pilotage humain** autour du noyau normatif inchangé. Toute décision de transition passe par `coordengine` (aucune réimplémentation), toute écriture passe par `persist.SaveAtomic` (N-15), toute reprise par `resume` (N-16/N-17). Le pilote humain reste la seule autorité de fermeture (N-14) et d'arbitrage.

### 2.2 Couches

```
Navigateur (maquette HTML, données simulées explicites)
        │ HTTP loopback uniquement
Service COORD Pilot (Go stdlib net/http, nouveau paquet dans coordengine)
        │
        ├── packages nouveaux : pilot (API + sessions locales), missions (référentiel
        │   multi-missions sur fichiers JSON atomiques + index), budget, arret,
        │   adaptateurs (simulés par défaut, fail closed)
        │
        └── moteur inchangé : spec / policy / engine / jcs / persist / resume / ctxcontrol
```

- **Service** : serveur HTTP local (127.0.0.1), token d'authentification locale relancé à chaque démarrage, affiché en console uniquement (aucune exposition publique ; le sandbox n'expose rien d'externe).
- **Référentiel missions** : un dossier par mission ; enregistrement via `persist.SaveAtomic`, empreinte des preuves via `jcs.Fingerprint`, journal par mission via `persist.JournalExecute`. Pas de SQLite au stade conception : fichiers atomiques suffisent, une dépendance de moins (arbitrage documenté ; SQLite reste option pour le VPS).
- **Adaptateurs** : interface unique déclarée dans la configuration (liste `adapters`, N-01) ; chaque adaptateur qualifie une identité à sa frontière de confiance (spec §3). Stade conception : uniquement des adaptateurs **simulés** qui ne font aucun appel réseau, n'ont aucune clé, et refusent tout mode « réel » (fail closed, hérité de la leçon T02 d'`ai-coordinator`). Fournisseurs multiples : prévu par design (mistral, openai, …identifiants opaques), sans exclusion ni privilège vendeur.
- **Permissions** : habilitations fail-closed par mission (N-01..N-05) : rôle de chaque identité, actions autorisées, séparation EXECUTANT/RECEPTIONNAIRE (N-05, jamais simultanément). Le pilote humain authentifié localement reçoit le rôle PILOTE ; il est le seul à acker la fermeture (N-14) et à arbitrer une divergence (N-16).
- **Budgets** : paquet `budget` neuf. Compteurs par mission et par adaptateur (appels, jetons déclarés par l'adaptateur simulé, indicateur monétaire optionnel) ; plafonds fixés à la création de mission ; contrôle **avant** toute transition déclenchant un appel d'adaptateur ; dépassement = refus explicite, transition bloquée, aucune action externe. Fail closed : pas de plafond défini = aucun appel autorisé. Mode simulé = coût zéro, aucune dépense.
- **Arrêt humain effectif** : paquet `arret` neuf. Le pilote peut demander l'arrêt d'une mission à tout instant (y compris pendant EXECUTION) : le service refuse toute nouvelle transition (motif explicite « arrêt demandé par le pilote »), journalise la demande, et conserve l'état machine intact. **Clarification normative demandée en correction n°3 (vis-à-vis de N-20)** : N-20 régit les *écarts auto-déclarés de contexte de l'exécutant* : journaux idempotents, sans jamais interrompre la mission. La commande d'arrêt du PILOTE est d'une *nature différente* : une instruction de l'autorité humaine (spec §2 : le pilote est l'autorité ; §12 : seul habilité à fermer), émise volontairement depuis l'interface de pilotage, hors du flux de travail de l'exécutant. Elle n'est donc pas un « écart » au sens de N-20 et ne peut être gérée par lui (N-20 interdit précisément l'interruption qu'un arrêt demande). Trois garanties séparent les deux mécanismes : (a) types de journal distincts — `ARRET_PILOTE` (commande) vs `ECART` (auto-déclaré), jamais confondus ; (b) l'arrêt est intercepté dans le service, le moteur et N-20 restent intacts et conformes (aucune modification du noyau) ; (c) à la levée de l'arrêt, la reprise passe exclusivement par le dispositif normatif existant (N-16 arbitration + N-17 head vérifié), comme après tout arrêt. La distinction est soumise à validation explicite dans cette critique.
- **Reprise** : à la levée de l'arrêt ou au redémarrage du service : `resume.ChampsDivergents` puis `Arbitrer` (`github_fait_foi`) si deux copies divergent, puis `Reprise` conditionnée au head vérifié (N-17) ; verdict obsolète refusé via `engine.Perime` (N-13) si les preuves ont bougé.
- **Prévention des actions externes non autorisées** : liste blanche stricte des adaptateurs, aucun accès shell/réseau en mode simulé, aucun secret dans le repo (variables d'environnement locales uniquement, fichiers ignorés), journal intégral des tentatives (y compris refusées).

### 2.3 Cible VPS Linux (conception seulement, non déployée)

Même code, exécution en service ; ajouts requis alors : TLS (reverse proxy), authentification forte du pilote, secrets en variables d'environnement du service (jamais dans le repo), pare-feu n'exposant que le port public nécessaire. Aucun de ces éléments n'est mis en œuvre dans cette mission ; ils sont documentés pour la mission de déploiement future.

---


### 2.4 Stratégie d'erreurs et de sécurité (service + interface)

**Gestion des erreurs** : chaque refus (moteur, habilitations, budget, arrêt, adaptateur) produit un motif **explicite, typé et journalisé**, jamais masqué — aligné sur la convention du moteur (refus avec motif). Côté service : mappe motif → code HTTP sémantique (400 requête invalide, 403 non habilité, 409 transition refusée/budget dépassé/arrêt actif, 404 mission inconnue, 500 erreur interne journalisée). Aucune erreur interne n'est exposée en clair au client : réponse minimale + identifiant de trace croisé avec le journal local. Côté interface : chaque refus est affiché à l'écran avec son motif et sa trace, l'état machine reste visible et inchangé après refus (idempotence d'affichage), aucune requête ne modifie l'état sans confirmation du journal.

**Tests de sécurité du service local (exécutables dès le MVP, prix nul, sans compte)** :
- S1 : appel sans token d'authentification → rejeté (401), journalisé.
- S2 : token invalide/périmé → rejeté (401), aucune transition effectuée.
- S3 : le service n'écoute que sur 127.0.0.1 ; toute interface externe refusée au démarrage (fail closed).
- S4 : requête forgée avec rôle PILOTE sans authentification → refusée : le rôle PILOTE n'est attaché qu'à la session locale authentifiée, jamais dérivable du corps de requête.
- S5 : habilitations vides ou mal formées → démarrage refusé (N-05), message journalisé.
Ces contrôles sont ajoutés à la recette (mesurables, rejouables en local), exécutés REEL dès qu'un service existe ; à ce stade de conception, ils sont spécifiés et reportés NON_EXECUTE dans le rapport.

### 2.5 Scalabilité du stockage et seuil de migration

Limites assumées et documentées du stockage fichiers JSON atomiques : adapté à l'ordre de grandeur de la conception et du MVP (dizaines de missions, activité séquentielle pilotée par un humain, lectures ponctuelles). Mesures prévues dès le MVP : index en mémoire reconstruit au démarrage (démarrage O(n) mesuré), écriture séquentielle sans concurrence d'écriture sur un même enregistrement (verrou par mission), shard simple = un dossier par mission (pas de fichier global croissant). Seuil de migration documenté : si le référentiel dépasse ~1000 missions ou si des lectures fréquentes multi-connexions apparaissent (cible VPS multi-pilotes), migration vers SQLite (leçon `ai-coordinator`, modernc sans cgo) — périmètre d'une mission ultérieure, arbitrage pilote. Aucun goulot caché : la limite est une donnée explicite de conception, pas un constat a posteriori.

---

## 3. Expérimentation COORD v1.5 sur le cycle de la mission

Le cycle de cette mission (critique du présent plan → corrections éventuelles → réception des livrables) est conduit selon le protocole v1.5 via la session pont `m-coord-pilot-01` :

1. Ce plan envoyé à OpenAI pour critique → verdict `PLAN_ACCEPTE` ou `A_CORRIGER` (boucle de correction jusqu'au verdict, plafond 3 tours puis escalade pilote).
2. Réalisation des livrables, publication dans l'issue #25 des preuves : SHA des états, empreintes, extraits de journaux.
3. Réception indépendante OpenAI des livrables + preuves → verdict publié dans #25.
4. Toute interruption du cycle (session perdue, sandbox réinitialisé) sera documentée, avec reprise et refus de tout verdict obsolète.
5. Distinction explicite permanent : exécutions REEL (batteries Python/Go locales) vs NON_EXECUTE ou SIMULE ; aucune revendication de conformité pour un scénario simulé.

Outils : Python 3 + `coord06_v15.py`/`tests_coord06_v15.py` présents ; toolchain Go officiel à télécharger si exécution Go jugée utile (comme en #23). L'expérimentation documente aussi : transitions, preuves, corrections, reprise après interruption si réalisable, refus de verdict obsolète, séparation des rôles.

---

## 4. Maquette HTML navigable (locale, données simulées explicites)

Maquette statique unique page (ou multi-pages) HTML/CSS/JS pur, **sans framework ni build**, explorable dans un navigateur, bandeau permanent « DONNÉES SIMULÉES ». Écrans :

1. **Tableau de bord** : missions en cours, états machine, budgets consommés.
2. **Création de mission** : référence d'issue, sélection EXECUTANT/RECEPTIONNAIRE (distincts), adaptateurs, habilitations, plafonds de budget.
3. **Suivi du cycle** : timeline critique → correction → réception avec verdicts, preuves (revision, empreinte), motif d'un refus.
4. **Arbitrage humain** : divergence de copies, arbitrage github_fait_foi, reprise conditionnée au head.
5. **Arrêt / reprise** : bouton d'arrêt effectif, journal, reprise post-arbitration.
6. **Coûts** : compteurs simulés par mission/adaptateur, plafond atteint = blocage démontré.

**Documentation utilisateur intégrée (correction n°4)** : la maquette embarque un écran d'accueil « Prise en main » (déroulé d'une mission en 6 étapes, légende des états et verdicts, signification des colonnes du tableau de bord, rôle du bandeau simulé) et une aide contextuelle par écran (bulle d'information sur chaque concept : habilitation, adaptation, empreinte, arbitrage). Un court guide de lecture (fichier README de la maquette) accompagne les livrables. Objectif : un pilote humain doit comprendre l'ergonomie cible sans lire la spec.

Test de la maquette : navigation réelle entre écrans, vérification des liens et des états affichés, fichier de recette listant chaque contrôle avec résultat OK/NON_EXECUTE. La maquette est un artefact de conception : ses transitions ne sont **pas** le moteur ; elle illustre l'ergonomie cible.

## 5. MVP et recette mesurable

MVP proposé (= périmètre de la mission suivante, pas de celle-ci) : service Go local + référentiel missions + adaptateurs simulés + budget + arrêt/reprise, piloté par la maquette enrichie d'appels réels au service ; une mission de bout en bout (création → critique simulée → réception simulée → ack → clôture par le pilote) avec journal complet.

Critères de recette mesurables (extraits, liste complète dans le dossier de conception) :

- R1 : toute transition non listée dans la table fermée est refusée avec motif (test exécutable).
- R2 : une habilitation vide ou mal formée ferme le service (N-05), message journalisé.
- R3 : dépassement de plafond budgétaire bloque la transition avant tout appel, motif journalisé.
- R4 : arrêt demandé → toute transition ultérieure refusée, motif explicite ; reprise → cycle reprend.
- R5 : divergence simulée entre copies → arbitration consignée, reprise conditionnée au head vérifié.
- R6 : verdict sur preuves modifiées → refus (N-13).
- R7 : batteries existantes inchangées rejouées 88/88 + 47/47 + 22/22 + démo, sorties byte-identiques (aucune régression du périmètre publié).
- R8 : la maquette navigue entre les 6 écrans sans erreur de console, avec bandeau simulé visible partout, et l'écran « Prise en main » est accessible depuis le tableau de bord.
- S1 : appel au service sans token → 401 journalisé, aucune transition appliquée.
- S2 : token invalide/périmé → 401, aucune transition.
- S3 : service lié à 127.0.0.1 uniquement ; configuration d'écoute externe refusée au démarrage.
- S4 : rôle PILOTE non dérivable d'un corps de requête non authentifié (tentative forgée → refus).
- S5 : habilitations vides/mal formées → démarrage du service refusé (N-05), motif journalisé.

## 6. Estimation indicative des coûts

- Conception + maquette (cette mission) : **0 €** (exécutions locales uniquement).
- MVP local (mission suivante) : **0 €** (adaptateurs simulés, aucune clé, aucun compte).
- Cible VPS éventuelle (mission future, hors périmètre) : VPS 1 vCPU / 1-2 Go ≈ 5-7 €/mois (indicatif marché) ; API fournisseurs : selon usage réel (non engagé ici).

## 7. Livrables et branche

- Branche `mission/coord-pilot-01` depuis `a803b3c5`, **fichiers nouveaux uniquement** : `coord/pilot/` (dossier de conception + maquette + recette), `coord/go/internal/{missions,budget,arret,adaptateurs}/` ou `coord/go/cmd/coord-pilot/` si le présent plan est accepté. Aucun fichier existant modifié ; PR isolée, fusion réservée au pilote.
- Dossier de conception concis (fr) ; rapport d'expérimentation v1.5 avec preuves et réserves ; plan MVP + recette ; maquette locale navigable.
- En-têtes : `Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902`, SPDX Apache-2.0. Licence du périmètre : Apache-2.0 déjà acté pour COORD (décision pilote 24/11/2026), applicable sans changement.

## 8. Arbitrages demandés au pilote / à la critique

1. Option (A) retenue : COORD Pilot **dans le module coordengine** par fichiers nouveaux (paquets `internal`). Alternative (copie/paquet public) écartée : modification ou duplication du noyau normatif.
2. Stockage fichiers JSON atomiques plutôt que SQLite au stade conception (arbitrage documenté).
3. L'arrêt humain intercepte au service, pas dans le moteur : le moteur reste conforme sans modification.
4. Maquette en français, données simulées uniquement.
5. Arrêt volontaire du pilote traité distinctement de N-20 (écart auto-déclaré) : proposition argumentée ci-dessus, à valider en critique.

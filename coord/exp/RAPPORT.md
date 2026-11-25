# Rapport M-COORD-EXP-01 — Expérimentation A2A-over-MCP

Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

Issue #38 — Ordre du pilote du 25/11/2026. Exécutant : Mistral. Critique du plan et réception indépendante : OpenAI. Décision GO/NO-GO : réservée au pilote.

**Hypothèse testée** : une pile inspirée ISO/OSI COORD (gouvernance) → A2A (sémantique inter-agents) → MCP (enveloppe applicative) → HTTP/STDIO, où « A2A-over-MCP » signifie des messages A2A encapsulés dans des appels/résultats MCP. L'ordre impose de **ne pas présupposer de gain** ; une conclusion négative est recevable.

## 1. Périmètre et garanties

- POC **isolé** : module `coord/exp` (go.mod propre), SDK `a2a-go/v2` v2.9.0 (Apache-2.0) et SDK MCP `go-sdk` v1.8.0. Le moteur `coordengine`, le Pilot et le serveur MCP #37 sont utilisés **inchangés** (binaires reconstruits tels quels).
- Deux agents simulés (AGENT-EXEC, AGENT-CRIT), boucles locales (loopback), hors ligne, **0 €**, aucune clé, aucun appel fournisseur.
- Le **même SDK A2A produit les messages dans les deux modes** ; seule l'enveloppe diffère (loyauté de la comparaison, exigence du plan v2).
- Aucune credential COORD (token console, jeton de lecture) n'est jamais confiée à un agent ; aucun canal de mutation n'est exposé.

## 2. Méthode

Chaque scénario est exécuté **dans les deux modes** avec les mêmes entrées, puis comparé par empreinte sémantique (sha256 de l'état final + artefact) et par mesures (octets, échanges, événements). Le verdict par paire est « conservée » (sha et états identiques), « conservée (service dégradé : …) » ou « PERDUE ». Le scénario S10 mobilise le Pilot réel et le serveur MCP #37 réel avec tests négatifs anti-mandat. Les critères GO/NO-GO pré-déclarés : (a) sémantique conservée sur tous les scénarios ; (b) aucune régression de gouvernance/sécurité ; (c) surcoût mesurable et acceptable ; (d) gain fonctionnel identifié par rapport à l'API directe.

## 3. Résultats (exécutions réelles, boucles locales)

| Scénario | Objet | Octets A | Échanges A | Octets B | Échanges B | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| S1 | découverte AgentCard | 0 | 0 | 840 | 1 | conservée |
| S2 | aller-retour minimal (PING) | 665 | 1 | 1369 | 2 | conservée |
| S3 | tâche à étapes (COMPTER:5) | 680 | 1 | 1399 | 2 | conservée |
| S4 | gros artefact (GROS:1048576) | 1 049 253 | 1 | 2 098 545 | 2 | conservée |
| S6 | erreur EF (ECHEC) | 716 | 1 | 1475 | 2 | conservée |
| S7 | copie verbatim (ECHO) | 804 | 1 | 1647 | 2 | conservée |
| S8 | verdict du critique | 927 | 1 | 1923 | 2 | conservée |
| S3b | streaming (COMPTER:5) | 2832 | 1 | 3966 | 1 | **conservée (service dégradé : non incrémental)** |
| S5 | annulation (PAUSE) | 1575 | 3 | 3412 | 6 | conservée |
| S9 | injection (données opaques) | 1002 | 1 | 2047 | 2 | conservée (échec des deux côtés, comme attendu) |
| S10 | gouvernance COORD | 665 | 1 | 1369 | 2 | conservée (détail en §4) |

Faits saillants :

- **Sémantique intégralement conservée** : aucun PERDUE, les mêmes états finaux et empreintes par paire dans tous les scénarios, y compris erreur, annulation après persistance, gros artefact et injection.
- **Surcoût du mode B ≈ ×2 octets** : l'artefact A2A est transporté deux fois (enveloppe MCP + charge A2A interne de la passerelle). Ex. S4 : 1,05 Mo → 2,10 Mo. Un échange supplémentaire par appel (aller MCP + aller A2A interne).
- **Streaming dégradé en mode B** : même séquence d'événements (6), mais rendue **après** l'appel (non incrémental), MCP ne transportant ici qu'un résultat unique. C'est la seule perte de service constatée, documentée comme telle.
- **Découverte (S1)** sans équivalent : la carte est relayée par l'outil MCP dédié de la passerelle (0 octet en A : résolution directe dans le client).

Traces : `preuves/conduite-s1-s9.md` (sha256 `5cbfb159…`) et `preuves/gouvernance-s10.md` (sha256 `c3e92cd2…`). Les assertions GO/NO-GO sont automatisées (`TestConduiteS1S9`, `TestS10Gouvernance`).

## 4. S10 — Gouvernance COORD (Pilot réel + MCP lecture seule)

Invariants constatés :

- binaires `coord-pilot` et `coord-mcp` construits depuis `coord/go` **inchangé** ;
- Pilot réel démarré sur boucle locale (token console, jamais journalisé) ; mission `M-COORD-EXP-01-S10` **enregistrée via l'API authentifiée** (POST /api/mission, en-tête `X-Coord-Pilot-Token`), état PLANIFIEE ;
- serveur MCP #37 joint en STDIO avec le **jeton de lecture uniquement** : inventaire **exactement 2 outils** (`coord_list_missions`, `coord_get_mission`), aucun outil de mutation ;
- la mission est lisible (`coord_list_missions`, `coord_get_mission` porte le contenu dérivé de COORD : agents A2A expérimentaux, habilitations, plafonds) ;
- **aucune credential COORD** dans les données de mission lues par les agents.

Tests négatifs anti-mandat (tous REFUS CONFORMES) :

- création **sans token** : HTTP 401 ;
- création avec le **jeton de lecture** sur une route de mutation : HTTP 403 `JETON_LECTURE_REFUSE` ;
- auto-mandat tenté (habilitation `cloturer` dérivée au rôle EXECUTANT) : HTTP **409 `CREATION_REFUSEE`** (N-01..N-05, fail closed) ;
- création via le canal MCP (outil inexistant `coord_create_mission`) : REFUSÉE par le serveur lecture seule.

Les agents opèrent dans les deux modes (aller-retour minimal vérifié en A et en B) **sans credential COORD** : la gouvernance reste portée par l'étoile Pilot ; l'enveloppe MCP n'apporte aucun privilège.

## 5. Complexité d'intégration

| Composant | Lignes (Go) |
| --- | --- |
| `agents/` (agents simulés) | 220 |
| `modea/` (pile A2A direct) | 282 |
| `modeb/` (passerelle MCP + même couche A2A) | 276 |
| `conduite/` (harness S1..S9 + S10 gouvernance + tests) | 1023 |
| `cmd/coord-exp/` | 65 |

Faits d'expérience : aucune passerelle A2A⇄MCP existante n'était réutilisable dans nos contraintes (aucune norme) ; la passerelle doit relayer le résultat A2A **brut** (json.Marshal du Task réel), pas un résumé ; un piège du SDK a2a-go v2.9.0 a été isolé (`AgentExecutor.Cancel` doit émettre `TaskStatusUpdateEvent(TaskStateCanceled)`, sinon `tasks/cancel` bloque) ; les états A2A ne correspondent pas un à un aux états COORD, la traduction appartient à l'adaptateur.

## 6. Limites

Agents et passerelle **SIMULÉS** ; boucles locales ; aucun fournisseur réel (qualifications réelles Gemini/Claude : missions distinctes sur autorisation) ; race detector et multi-OS couverts par la CI `coord-exp-ci.yml` (NON exécutés localement dans le bac à sable sans gcc) ; mesures de latence non retenues comme critère (boucles locales non représentatives) ; le streaming en mode B est un choix de l'implémentation de la passerelle (résultat unique), pas une limite du standard MCP, mais il traduit le coût du passage par un résultat unique.

## 7. Conclusion GO/NO-GO (recommandation de l'exécutant)

Au regard des critères pré-déclarés :

- (a) sémantique : **conservée partout** (avec streaming dégradé documenté) ;
- (b) gouvernance/sécurité : **aucune régression** (S10 conforme, 401/403/409 fail closed, aucun privilège via l'enveloppe) ;
- (c) surcoût : réel et mesurable — ≈ **×2 octets** et un échange supplémentaire par appel, plus une passerelle à écrire et maintenir ;
- (d) gain fonctionnel : **aucun identifié** pour l'objectif courant — la qualification d'un fournisseur (Gemini, Claude) passe déjà par l'API directe derrière `adaptateurs.Adaptateur` (chemin qualifié en #29/#30) ; l'encapsulation MCP n'apporte ni fonction ni garantie nouvelles.

**Recommandation : NO-GO pour une qualification réelle A2A-over-MCP avec Gemini à court terme.** Le chemin direct est plus simple, moins coûteux et déjà qualifié ; l'expérimentation établit en revanche que l'encapsulation est **viable sémantiquement** et sûre côté gouvernance : elle reste une option documentée (phase 4 de la proposition Claude v0.1 / analyse #27, façade A2A en module séparé) si un besoin réel émerge — agents tiers, écosystème multi-standards, transport normalisé. Une conclusion négative est recevable conformément à l'ordre. **Décision finale réservée au pilote.**

## 8. Reproducibilité

`go vet ./... && go build ./... && go test ./... -timeout 300s` depuis `coord/exp` (~10 s, 0 €, aucune clé). Sorties : `go run ./cmd/coord-exp` (tableau), `go run ./cmd/coord-exp -s10` (gouvernance), `-json` (qualification). CI : `coord-exp-ci.yml` (ubuntu/windows/macos + race, déclenchée sur `coord/exp/**`).

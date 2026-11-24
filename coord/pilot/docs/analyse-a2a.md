# Analyse A2A — étude du SDK Go et proposition d'intégration (M-COORD-PILOT-02)

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

Mission : M-COORD-PILOT-02 (issue #27). Cette analyse répond à l'étape 1 de la mission
(« Étudier SDK A2A Go (version, licence, maturité, sécurité, compatibilité, exemples) »).
Les faits ci-dessous sont vérifiés REEL depuis l'API GitHub et le `go.mod` publiés
(24/11/2026). Aucune ligne de code du SDK n'est importée dans ce MVP.

## 1. Étude du SDK `github.com/a2aproject/a2a-go` (REEL, 24/11/2026)

| Aspect | Constat vérifié |
| --- | --- |
| Module | `github.com/a2aproject/a2a-go/v2` ; dernier tag **v2.9.0** (v2.5.0→v2.9.0 publiés) |
| Origine | Projet officiel A2A; dépôt créé 05/2025 ; poussées actives (dernier push 24/11/2026) ; ~516 étoiles |
| Licence | **Apache-2.0** (compatible COORD) |
| Maturité | CI « nightly », release-please + CHANGELOG, e2e et integrations présents, documentation pkg.go.dev ; rythme de publication élevé = churn à surveiller |
| Sécurité | `SECURITY.md` : canal privé GitHub Security Advisories ; aucune dépendance problématique identifiée dans `go.mod` (grpc, protobuf, otel, genproto, uuid : licences permissives) |
| Compatibilité | Exige Go ≥ 1.26 (coordengine est en 1.27 : compatible) ; empreinte de dépendances LOURDE (gRPC, protobuf, OpenTelemetry ; transports REST/JSON-RPC/gRPC) |
| Exemples | Dossier `examples/`, SDK client (`a2aclient`) et serveur (`a2asrv`), extensions pluggables (transport, auth, stockage) ; specification A2A **v1.0** (`a2a.Version`) |

Sources : https://github.com/a2aproject/a2a-go , https://a2a-protocol.org .

## 2. Conflit structurel identifié

1. Le module normatif `coordengine` est **stdlib uniquement, zéro dépendance** (décision
   d'architecture M-COORD-STD-01, `go.mod` sans aucune entrée `require`). Importer a2a-go
   dans `coordengine` casserait cette propriété.
2. La mission #27 impose une interface d'adaptateur **« indépendante d'A2A et
   fournisseurs »**, **« sans dépendance normative »**. A2A est un protocole d'agents
   **distants** (réseau) ; le MVP est un service local à adaptateurs **simulés fail-closed
   sans aucun appel réseau**. Une dépendance code au stade MVP n'apporterait rien et
   contournerait la contrainte.
3. Le rythme de publication du SDK (v2.x actif) rend risquée toute précipitation :
   l'interface interne de COORD Pilot doit être stable avant d'y brancher A2A.

## 3. Articulation retenue (validée par critique OpenAI, plan v1 accepté)

- **Le MVP définit son interface d'adaptateur interne** (`internal/adaptateurs`) :
  `ID()`, `Executer(mandat)`, `Soumettre(livrable)`. « Recevoir » est délibérément absent :
  rendre un verdict est une décision enregistrée dans COORD, pas un appel. L'interface ne
  mentionne ni A2A, ni transport, ni fournisseur : identifiants opaques, aucun privilège
  vendeur.
- **Intégration A2A ultérieure en mission dédiée**, dans un **module Go séparé** (par
  ex. `coord/integrations/a2a/`, `go.mod` distinct) : ce module importerait `a2a-go/v2`
  et traduirait les opérations A2A vers l'interface d'adaptateur via un service
  d'adaptation. Le module normatif `coordengine` ne prendrait jamais cette dépendance (le
  seuil de dépendances resterait contenu hors du noyau). Un `go.mod` séparé vérrouille
  techniquement l'isolation.
- **Cette analyse constitue la preuve locale isolée** demandée par la mission : étude
  documentée + mapping conceptuel ci-dessous, sans dépendance de code ni appel réseau.

## 4. Mapping conceptuel opérations A2A ⇄ actions COORD (indicatif, non normatif)

| Concept A2A (specification v1.0) | Équivalent conceptuel COORD | Écart assumé |
| --- | --- | --- |
| AgentCard (découverte d'un agent) | Enregistrement de l'adaptateur dans la liste de configuration (N-01) | COORD qualifie des identités à la frontière de confiance, pas des agents distants |
| `message/send` (client → agent) | Action `executer` (EXECUTANT) ou `soumettre` selon l'étape du cycle | A2A ne connaît pas la table fermée de transitions ; COORD ne connaît pas la négociation de tâches |
| Task / TaskState | `Mission` et ses 8 états normatifs | Les états A2A (submitted, working, …) ne correspondent pas 1-pour-1 aux états COORD ; la traduction relève de l'adaptateur |
| message/streaming, push notifications | Hors périmètre initial | À arbitrer en mission dédiée (chaînabilité des journaux N-19) |
| Auth A2A (schemes multiples) | Qualification d'identité (spec §3) par l'adaptateur | COORD n'authentifie jamais dans le moteur ; l'adaptateur A2A porterait l'authentification à sa frontière |

Conclusion du mapping : la sémantique A2A (tâches asynchrones, streaming, découverte)
dépasse volontairement le cycle de gouvernance COORD ; une intégration A2A doit donc être
une **adaptation** (l'adaptateur traduit), jamais une **dérivation normative** (la table
fermée resterait seule source de vérité des transitions).

## 5. MCP (mention, hors périmètre)

Le SDK MCP Go (`modelcontextprotocol/go-sdk`) répond au même calcul : utile un jour pour
brancher des outils et fournisseurs réels, hors périmètre MVP, même mécanisme d'isolation
(module séparé). Aucune dépendance prise dans cette mission.

## 6. Réserves

- Aucune interopérabilité A2A réelle n'est revendiquée dans ce MVP : l'analyse est
  documentaire, exécutée REEL sur des sources publiques, mais aucun appel A2A n'a eu lieu.
- L'étude reflète l'état du dépôt au 24/11/2026 (tag v2.9.0) ; la mission d'intégration
  future devra re-vérifier version, licence et vulnérabilités à jour (Arbitrage pilote).

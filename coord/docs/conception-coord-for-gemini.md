<!-- Copyright © 2026 Denis Bouzon — ORCID 0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# Conception de COORD for Gemini — Dossier M-COORD-GEMINI-01

- **Version** : 1.0 — 25/11/2026 — Mission [M-COORD-GEMINI-01](https://github.com/dbouzon/osnias-clearing/issues/31) (issue #31)
- **Statut** : conception uniquement. COORD v1.4 = référence opérationnelle ; v1.5 = candidate non activée.
- **Exécutant** : Mistral (Vibe). Critique et réception indépendante : OpenAI. Gouvernance : pilote humain.
- **Base** : main `40c6fd9b7266ac5315fd15f60f75e85b19de11f2` (post-fusion PR #34, harnais M-COORD-SEC-01 pérennisés).

## 1. Objet et périmètre

Concevoir l'extension permettant à Gemini CLI d'utiliser COORD : découverte et consultation des missions, opérations actives uniquement via les contrôles Pilot existants. La direction **Gemini → COORD** (agent consommant COORD) est le périmètre ; la direction **COORD → Gemini** (API fournisseur Google, type connecteur de `coord/go/internal/connecteurs`) est une dépendance ultérieure hors périmètre 01. Aucune modification du moteur, du protocole ou des connecteurs ; aucun appel payant ; aucun secret en repo.

## 2. Fait d'écosystème 2026 : scission Gemini CLI / Antigravity

- Google Developers Blog : depuis le **18/06/2026**, Gemini CLI et Code Assist ne servent plus les requêtes pour AI Pro/Ultra ni l'usage individuel gratuit ; **Antigravity CLI** est le produit de remplacement.
- Antigravity préserve MCP servers, skills et rules ; les extensions Gemini CLI doivent être converties en plugins (`agy plugin import gemini`) ([docs Antigravity](https://antigravity.google), [geminicli.com](https://geminicli.com/docs/extensions/), mise à jour 17/02/2026 ; commit gemini-cli `0c0d88d` du 14/05/2026 pour le format).
- **Conséquence** : l'architecture est **MCP-first portable**. Le cœur réutilisable est le serveur MCP ; l'emballage Gemini CLI est mince et l'emballage Antigravity (plugin) est une conversion documentée. Le serveur MCP **commun** à tous les fournisseurs fait l'objet de la mission séparée [M-COORD-MCP-01](https://github.com/dbouzon/osnias-clearing/issues/35) (issue #35) : la conception présente documente l'emballage Gemini/Antigravity et le contrat d'interface côté client, sans présumer du développement de #35 (qualification réelle requise, non présumée).

## 3. Architecture

```text
Gemini CLI / Antigravity (agent IA)
        │  (emballage : gemini-extension.json, GEMINI.md, commandes /coord-*)
        ▼
Serveur MCP « coord-gemini » (STDIO, loopback, lecture seule par défaut)
        │  HTTP loopback + token console EXISTANT de coord-pilot (jamais de new surface d'autorité)
        ▼
Service COORD Pilot (cmd/coord-pilot, inchangé)
        │
        ▼
Moteur COORD (coord06 v1.4 / module coordengine v1.5, inchangés)
```

- **Serveur MCP** : STDIO, lancé par le client CLI ; s'apparente à un client du service HTTP Pilot. Il s'exécute sur la machine du pilote, en loopback uniquement. Toute opération active (création, transition, soumission, arrêt, reprise) passe par les routes authentifiées existantes de `coord/go/internal/pilot`, avec le **token console existant** : aucune duplication des règles COORD, aucun contournement du pilote. Lecture seule par défaut : le serveur MCP ne détient le token actif que si le pilote le place dans le trousseau local ; sinon, seules les routes publiques de découverte/consultation sont joignables.
- **Secrets** : token via champ `settings[].sensitive` (trousseau système Gemini CLI / Antigravity) ou variable d'environnement locale, jamais dans le repo ni dans les logs. Politique d'assainissement des variables d'environnement du CLI (clarifiée au commit du 14/05/2026) documentée dans le README de l'extension.
- **Isolation** : le serveur MCP n'accède jamais directement au stockage (`missions/*.json`) : tout passe par le service Pilot (règle #35 « ne pas contourner Pilot » reprise ici).

## 4. Manifeste modèle (`gemini-extension.json`)

```json
{
  "name": "coord",
  "version": "0.1.0",
  "description": "Human-governed LLM collaboration via COORD Pilot (read-only by default).",
  "mcpServers": {
    "coord": {
      "command": "node",
      "args": ["${extensionPath}/mcp/coord-gemini.js"],
      "cwd": "${extensionPath}"
    }
  },
  "contextFileName": "GEMINI.md",
  "settings": [
    {
      "name": "coordPilotToken",
      "description": "Token console du service COORD Pilot local (optionnel : lecture seule sans token)",
      "envVar": "COORD_PILOT_TOKEN",
      "sensitive": true
    }
  ],
  "excludeTools": ["run_shell_command"]
}
```

Notes : `name` en minuscules/tirets et égal au nom du répertoire ; manifeste à la **racine** du dépôt ou de l'archive de release ; `trust` non supporté ; `${extensionPath}` officiel. Le champ `copyright`/ORCID figure dans les en-têtes de fichiers sources (SPDX), pas dans le JSON (syntaxe JSON préservée).

## 5. Contrat d'outils MCP (v1 proposée)

| Outil | Mode | Description | Garantie |
|---|---|---|---|
| `coord_list_missions` | lecture seule | Liste mission, verdict, état (JSON pur) | Route publique Pilot |
| `coord_get_mission` | lecture seule | Détail, stratégie, journal d'une mission autorisée | Filtrage par droits Pilot |
| `coord_get_budget` | lecture seule | Jetons/centimes réservés et consommés | Données agrégées, aucun secret |
| `coord_submit_artifact` | actif | Soumission d'un livrable pour réception | Token console requis, idempotence moteur |
| (aucune mutation d'état) | actif-interdit | Toute mutation d'état (création, transition, arrêt, reprise) **interdite** au serveur MCP v1 : le pilote agit via son interface Pilot | Anti-auto-mandatement (cf. §7) |

Règles : schémas JSON d'entrée validés côté serveurs factices avant développement (02) ; toute erreur = refus explicite, jamais d'effet de bord ; négociation/erreurs conforme au standard MCP.

## 6. Matrice exigences → preuves

| Exigence (source mandat) | Preuve en 01 | Vérification en 02/03 |
|---|---|---|
| Installation 3 OS par outils officiels | Matrice §8, commandes vérifiées | CI 3 OS + à chaud par un tiers |
| Lecture par défaut, actions explicites | Contrat §5, routes Pilot existantes | Tests ; refus sans token |
| Aucun contournement du pilote | §3 (service HTTP unique), §7 A-tests | Rejeu harnais SEC-01 PR #34 verts |
| Installation sans clé ≠ exécution Gemini | §8.3 (installation sans authentification Google, exécution = clé du pilote) | Démo locale |
| Galerie : 3 conditions (public + topic + manifeste racine) | §9.2 | Soumission réelle 03 (décision pilote) |
| Indexation ≠ certification | §9.2 mentionné | Sans objet |
| Aucun appel payant en 01 | Démo simulée adaptateurs simulés | Démo locale réexécutée |

## 7. Menaces et garanties

| Menace | Garantie (existant, preuve) | Choix 01 |
|---|---|---|
| Injection via données de mission (identité/model falsifié) | Données opaques sans privilège, aucune route exécute du texte ; résolution d'acteur serveur (S4) ; N-05, fail-closed | Rejeu A1-A15 exigé en 02 (harnais #34) |
| Usurpation de rôle/pilote dans un prompt | Identités déclarées ≠ identités authentifiées ; aucune action sur texte de LLM | `_exclu_` du contrat §5 |
| Détournement de token | `sensitive` → trousseau, pas de log d'auth, refus hors loopback | Documenter + tester en 02 |
| Escalade via payload falsifié | Schéma JSON strict, routes Pilot inchangées, fail-closed transmis | Vecteurs 22/22 à rejouer en 02 |
| Doubles appels/idempotence | Anti-doublon orchestrateur existant (`appel_en_cours.json`) | Non modifié, testé 02 |
| Surcoût/argent | 0 € en 01 (adaptateurs simulés) ; voûte monétaire inchangée | 0 appel réel |

## 8. Installation et exécution

### 8.1 Prérequis

- Windows / Linux / macOS ; Node.js ≥ 20 (serveur MCP) ; COORD Pilot local déjà configuré (`coord/go/cmd/coord-pilot`) ; git pour l'installation CLI.
- Gemini CLI (hérité) : `gemini extensions install <url-du-dépôt> [--consent]` ; Antigravity : conversion plugin documentée : `agy plugin import gemini`, puis configuration du serveur MCP comme tout MCP externe.

### 8.2 Commandes (vérifiées en conception)

```bash
# lecture seule, sans token :
node mcp/coord-gemini.js            # serveur STDIO, attend COORD_PILOT_URL locale
# avec droits actifs : token du pilote dans le trousseau système
```

### 8.3 Installation sans clé ≠ exécution Gemini

L'installation de l'extension ne demande **aucune** clé ni authentification (dépôt public + manifeste). L'exécution effective de Gemini (requêtes IA) requiert l'authentification du compte du pilote côté CLI ; COORD n'en dépend pas pour sa part lecture seule (routes publiques loopback). En 02/03, la qualification réelle reste à la charge du pilote (0 € en 01).

## 9. Hébergement, release, diffusion

### 9.1 Recommandation (décision finale réservée au pilote)

Dépôt public **dédié** à l'extension (ex. `dbouzon/coord-gemini`) : satisfait les trois conditions d'indexation sans placer de manifeste à la racine du monorepo `osnias-clearing` (site multi-produits). Alternative documentée : archive de release auto-contenue dans le monorepo (manifeste à la racine de l'archive), avec le risque que la validation galerie ne la reconnaisse pas (à confirmer empiriquement en 03).

### 9.2 Diffusion

Topic `gemini-cli-extension`, licence Apache-2.0 (cohérente avec COORD, décision pilote), nom court minuscules/tirets (recommandation : `coord-gemini` ou `coord` en dépôt dédié), release GitHub avec manifeste à la racine de l'archive, branche par défaut stable. **Indexation ≠ certification Google** rappelé dans le README.

## 10. Licences (tableau vérifié en 01, re-vérification tierce en 02)

| Composant | Licence | Compatibilité Apache-2.0 (COORD) |
|---|---|---|
| `@modelcontextprotocol/sdk` | MIT | Oui (MIT compatible) |
| `zod` (validation entrée) | MIT | Oui |
| Node.js stdlib | MIT | Oui |
| Gemini CLI (client, non modifié) | Apache-2.0 | Oui |
| COORD (Apache-2.0, inchangé) | Apache-2.0 | — |

Fichiers propres : `Copyright © 2026 Denis Bouzon — ORCID 0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0` en en-tête. Aucun changement de licence du repo sans décision du pilote.

## 11. Démo simulée (01, sans clé ni compte Osnias)

Scénario reproductible en 02 : (S1) service Pilot local lancé avec missions factices ; (S2) serveur MCP démarré en STDIO ; (S3) négociation MCP standard + liste missions + détail ; (S4) soumission sans token activé → refus explicite sans effet de bord (S5) données hostiles (identité falsifiée, JSON profond) → données pures refusées ; trace sha256 publiée. Critères : refus, jamais d'effet de bord ; PROFIL(res) trace reproductible.

## 12. Plan des sous-missions 02 (développement) et 03 (publication)

- **02 — développement** (à publier par le pilote après ∃ serveur MCP commun #35) : serveur MCP réel conformément au contrat §5, adaptateurs simulés, tests factices loopback, CI 3 OS, réception indépendante au SHA, journalisation détaillée des verdicts avec sévérité (exigence de #31 initiale reportée de #29).
- **03 — publication/indexation** : dépôt dédié si décision pilote, topic, release, contrôles galerie, contrôles HTTP 200, licence affichée.

## 13. Critères de réussite vérifiables 02/03 (extrait)

1. CI verte 3 OS sur la PR 02 (Go batteries Python inchangées 88/88).
2. Harnais SEC-01 (#34) rejoués verts ; refus sans effet de bord démontré.
3. Démo locale (S1..S5) exit 0, trace sha256.
4. 03 : manifeste racine + topic vérifiés ; /coord/ et release HTTP 200 ; README sans secret.

---

**Réserves** : validation galerie de l'archive monorepo NON_EXÉCUTÉE (à confirmer en 03) ; exécution réelle Gemini NON_TESTÉE en 01 (compte pilote) ; départ des tests sur serveurs factices uniquement.

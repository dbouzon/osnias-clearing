<!-- Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# COORD MCP — serveur MCP commun (M-COORD-MCP-01, issue #35)

Serveur MCP (Model Context Protocol) **commun et portable** de COORD : une interface
de **lecture seule stricte** au-dessus du service COORD Pilot local, réutilisable
par tout client MCP indépendant (Gemini CLI, Antigravity, Claude, OpenAI, Mistral… ;
qualification réelle exigée pour chaque client, non présumée).

- **Aucune mutation exposée en v1** : deux outils, `coord_list_missions` et
  `coord_get_mission`. Toute mutation reste du ressort exclusif du pilote via son
  interface Pilot.
- **Aucun token console transmis aux clients IA** : le serveur ne connaît que le
  **jeton de lecture** distinct, lu dans un fichier local `0600` détenu par le pilote.
- **0 €, aucune clé API, aucun compte** pour la démonstration locale (adaptateurs SIMULÉS).
- SDK : `github.com/modelcontextprotocol/go-sdk` v1.8.0 (licence MIT/Apache-2.0 en
  transition, compatible COORD Apache-2.0), épinglé dans `coord/go/go.mod`.

## Architecture

```text
Client IA (Gemini CLI / Antigravity / Claude / …, client MCP quelconque)
        │  MCP over STDIO (transport local choisi ici par cmd/coord-mcp)
        ▼
cmd/coord-mcp → internal/mcpserver (cœur lecture seule, SANS transport)
        │  HTTP loopback uniquement, header X-Coord-Pilot-Token = jeton de lecture
        ▼
Service COORD Pilot (coord/go/cmd/coord-pilot, inchangé :
  GET /api/missions et GET /api/mission/{issue} acceptent le jeton de lecture ;
  TOUTE route de mutation le refuse : 403 JETON_LECTURE_REFUSE)
        ▼
Moteur COORD coordengine (inchangé)
```

Le paquet `internal/mcpserver` ne référence aucun transport (garde-fou testé) :
une future architecture client-serveur distante (transport HTTP diffus) pourra
réutiliser le cœur et le contrat d'outils sans changement. Ce développement n'est
PAS anticipé ici.

## Cycle de vie du jeton de lecture

```text
démarrage de coord-pilot
        │  (rotation IMPLICITE : tout jeton précédent devient invalide)
        ├─ token console      ── affiché console, jamais journalisé, pilote seul
        └─ jeton de lecture   ── écrit local 0600 (jeton-lecture), affiché nom du fichier
                │
                ▼
coord-mcp -pilot http://127.0.0.1:PORT -jeton /chemin/du/jeton-lecture
        │  contrôle AVANT démarrage : fichier régulier, 0600, propriétaire = utilisateur
        │  courant (POSIX) — sinon REFUS (exit 2), aucune valeur affichée
        ▼
sessions de lecture uniquement (missions : liste, détail)
```

- **Rotation** : à chaque (re)démarrage de `coord-pilot`, le jeton de lecture est
  régénéré ; les anciens jetons sont invalidés immédiatement (aucune persistance).
- **Compromission suspectée** : supprimer le fichier de jeton puis redémarrer
  `coord-pilot` (rotation immédiate) ; aucune donnée sensible ne transite par le jeton.
- **Vérification des permissions** :

```bash
ls -l /chemin/du/jeton-lecture        # attendu : -rw------- (0600)
chmod 0600 /chemin/du/jeton-lecture   # correction si nécessaire
```

## Utilisation locale

```bash
cd coord/go
go build -o coord-pilot ./cmd/coord-pilot
go build -o coord-mcp   ./cmd/coord-mcp
./coord-pilot -data ./coord-pilot-data -ui ../pilot/ui   # affiche l'URL, le token
                                                          # console et le fichier du
                                                          # jeton de lecture (0600)
./coord-mcp -pilot http://127.0.0.1:PORT -jeton /chemin/du/jeton-lecture
```

Le serveur écoute sur STDIO et annonce ses deux outils ; tout client MCP standard
peut s'y connecter. Refus explicites (sans secret, sans effet de bord) :
`JETON_REFUSE`, `MISSION_INCONNUE`, `PILOT_INJOIGNABLE`, `REQUETE_INVALIDE`,
`ACCES_REFUSE`, `SERVICE_INDISPONIBLE`, `REPONSE_ILLISIBLE`.

### Démonstration locale (sans clé, sans compte, 0 €)

```bash
go run ./cmd/coord-mcp -demo-local
```

Enchaîne service Pilot factice (adaptateurs SIMULES, loopback) + fichier jeton
0600 + serveur MCP + client MCP factice, vérifie la négociation, les 2 outils,
la liste, le détail, le refus explicite d'une mission inconnue sans fuite de
secret, et l'absence de tout outil de mutation. Trace de référence :
`preuves/demo-mcp-locale-trace.txt` (exit 0 attendu).

## Sécurité (dérivé de M-COORD-SEC-01, issues #33/PR #34)

| Menace | Garantie |
| --- | --- |
| Injection via données de mission | Données pures renvoyées au client, aucune route n'exécute du texte, identités NE JAMAIS dérivées d'un prompt |
| Usurpation du pilote | Aucun outil actif en v1 ; mutations impossibles par construction ; rôle résolu côté service (S4) |
| Transfert du token console au client IA | Le serveur ne connaît que le jeton de lecture ; jeton jamais dans l'environnement du client, les `settings`, les logs ni les réponses |
| Jeton de lecture compromis | Fichier 0600 contrôlé au démarrage (fail-closed), rotation par redémarrage, valeur jamais affichée ni journalisée |
| Fuite dans les erreurs | Table erreur→code stable, messages sans jeton/URL/chemin/corps brut ; tests permanents de non-divulgation (`internal/mcpserver/mcpserver_test.go` + `-race` en CI) |
| Accès distant | URL du service restreinte à loopback (refus au démarrage), pas de port exposé par coord-mcp |

## FAQ

- **Puis-je mettre à jour le SDK Go MCP ?** Oui, par une PR dédiée : re-vérifier la
  licence, `go vet/build/test` complets, CI multi-OS verte, réception indépendante.
  La version est épinglée dans `go.mod`.
- **Et si le SDK posait problème ?** Repli documenté : un cœur JSON-RPC minimal en
  std-lib Go peut remplacer le SDK sans changer le contrat d'outils (le paquet
  `mcpserver` est majoritairement indépendant du SDK pour la logique de lecture).
- **Où sont les tests ?** `coord/go/internal/pilot/jeton_lecture_test.go` (jeton :
  GET accepté, mutations 403, sans option = non-régression, journal sans secret) et
  `coord/go/internal/mcpserver/mcpserver_test.go` (lecture, refus, non-divulgation,
  permissions, protocole MCP complet, boundary sans transport).
- **Extensions fournisseurs ?** Hors périmètre de cette mission : de futures
  missions qualifieront réellement chaque client (emballages légers).

## English summary

COORD MCP is the common, vendor-neutral Model Context Protocol server of COORD:
a strictly read-only facade above the local COORD Pilot service (tools:
`coord_list_missions`, `coord_get_mission`). No mutation is exposed; the pilot
console token is never known to the program — only a separate read-only token,
read from a local `0600` file owned by the pilot and rotated at every service
restart. Any standard MCP client may connect over STDIO on the local machine;
a future remote client-server architecture can reuse the same transport-free
core (guarded by tests) without changing the tool contract. Local demo:
`go run ./cmd/coord-mcp -demo-local` (no key, no account, simulated adapters, €0).
License: Apache-2.0, © 2026 Denis Bouzon (ORCID https://orcid.org/0009-0007-8894-8902).

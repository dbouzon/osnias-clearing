<!-- Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# Rapport — M-COORD-MCP-01 — Serveur MCP commun et portable

- **Date** : 25/11/2026 — **Issue** : #35 — **Protocole** : COORD v1.4 (référence opérationnelle ; v1.5 candidate non activée).
- **Exécutant** : Mistral (Vibe). **Critique et réception** : OpenAI. **Gouvernance** : pilote humain.
- **Base de développement** : main `10defea61f61e8198e710933c766a865995a5d3d` (post-fusion PR #36, dossier Gemini v1.1 livré).
- **Plan** : v2 **PLAN_ACCEPTE** (tour 2, session pont `m-coord-mcp-01-plan`, consigné dans #35).

## Mandat

Développer le serveur MCP commun de COORD, indépendant des fournisseurs, **lecture
seule stricte**, en réutilisant le service Pilot et le moteur existants (aucune
duplication), sans token console transmis aux clients IA, sans mutation exposée,
0 €, sans activation v1.5, sans extension fournisseur. Une future architecture
client-serveur distante reste possible sans développement anticipé.

## Réalisations

### 1. Extension additive minimale du service Pilot (jeton de lecture)

- `pilot.AvecJetonLecture(jeton)` : option variadique additive de `NouveauService` ;
  aucun appelant existant modifié, comportement par défaut inchangé (testé).
- `GET /api/missions` et `GET /api/mission/{issue}` acceptent le jeton de lecture
  OU le token console (`authLecture`) ; toute route de mutation le refuse :
  403 `JETON_LECTURE_REFUSE` + journal `REFUS_LECTURE_MUTATION` (fail-closed).
- `cmd/coord-pilot` génère et écrit le jeton de lecture via `EcrireJeton`
  (SEC-MCP-02 : création EXCLUSIVE d'un temporaire dans le même répertoire —
  aucune collision, aucun suivi de lien symbolique —, protection effective
  appliquée AVANT le renommage atomique, remplacement de la cible = rotation
  implicite à chaque démarrage), affiche le nom du fichier, jamais la valeur ;
  le token console reste inchangé dans son rôle.

### 2. Serveur MCP commun (paquet `coord/go/internal/mcpserver` + `cmd/coord-mcp`)

- Cœur lecture seule `NouveauServeur(baseURL, jeton)` : URL **loopback uniquement**
  (refus fail-closed), appel HTTP des routes GET de lecture avec le **jeton de
  lecture uniquement**, mapping erreurs→codes stables (`JETON_REFUSE`,
  `MISSION_INCONNUE`, `PILOT_INJOIGNABLE`, `REQUETE_INVALIDE`, `ACCES_REFUSE`,
  `SERVICE_INDISPONIBLE`, `REPONSE_ILLISIBLE`), aucun secret dans les messages.
- Deux outils MCP, aucun autre : `coord_list_missions`, `coord_get_mission`
  (schéma d'entrée strict). **Aucune mutation exposée, par construction** — testé
  au niveau protocole (l'appel d'un outil de mutation inexistant est refusé).
- `ChargerJeton` : fichier régulier (Lstat, les liens symboliques sont
  REFUSÉS, jamais suivis), protection EFFECTIVE exigée en fail-closed :
  POSIX — permissions strictement `0600` et propriétaire = utilisateur
  courant ; Windows — DACL natif de `golang.org/x/sys/windows` restreint au
  seul utilisateur courant et PROTÉGÉ contre l'héritage du répertoire parent
  (SEC-MCP-01). Valeur jamais affichée ni journalisée ; toute écriture passe
  par `EcrireJeton` qui applique la même protection avant rotation.
- Transport STDIO choisi **par la commande uniquement** : le paquet ne référence
  aucun transport (garde-fou testé `TestBoundary_AucunTransport`) → une future
  architecture distante (HTTP diffus) réutilise cœur et contrat inchangés.
- SDK : `github.com/modelcontextprotocol/go-sdk` v1.8.0 épinglé (MIT/Apache-2.0,
  compatible) ; alternative std-lib documentée dans le README.

### 3. Tests et documentation

- `internal/pilot/jeton_lecture_test.go` : GET acceptés (jeton lecture ou token
  console), toutes mutations 403 typées, sans option = non-régression, journal
  `REFUS_LECTURE_MUTATION` présent et SANS valeur de jeton.
- `internal/mcpserver/mcpserver_test.go` : lecture/détail réels via service Pilot
  factice loopback, refus et **non-divulgation** (jeton/URL/chemins absents de
  tous les messages d'erreur), URL non-loopback refusée, service injoignable sans
  détail réseau, identifiants hostiles rejetés, permissions du fichier jeton
  (0600 ok, 0644 refus, vide/absent/dossier refusés), protocole MCP complet sur
  transports en mémoire (négociation, 2 outils exactement, refus IsError),
  boundary sans transport.
- `coord/mcp/README.md` (FR + résumé EN) : architecture, cycle de vie du jeton
  (rotation, compromission, chmod), utilisation locale, matrice menaces/garanties,
  FAQ ; `coord/mcp/preuves/demo-mcp-locale-trace.txt`.
- CI : `coord-go-ci.yml` — la démonstration MCP locale rejoint la qualification
  multi-OS (step dédié `go run ./cmd/coord-mcp -demo-local`) et le race detector
  s'étend à `./internal/mcpserver/`. Le step `go test` publie désormais les noms
  de tests en échec en annotations CI (diagnostic multi-OS sans accès admin aux logs).
- Qualification multi-OS CI **REELLE et VERTE** au commit final
  `fb780bc68fa74c4c67f686869d3c220933c98dd8` : les 6 jobs passent (Go
  ubuntu/windows/macos, race detector étendu, schemas JSON, batteries Python).
  Correction apportée après premier échec du job Windows : le test de permissions
  POSIX du jeton exigeait un refus en 0644 non applicable sous Windows (bits
  POSIX sans objet) ; le test est devenu OS-aware et **affirme** désormais le
  comportement documenté sous Windows (chargement OK, contrôle sans objet), le
  refus 0600 restant exigé sur POSIX.

## Preuves REEL (sandbox, go1.27.4 linux/amd64, toolchain officielle)

- `go vet ./...` OK ; `go build ./...` OK ; **`go test -count=1 ./...` : 12 paquets ok**
  (dont `internal/pilot` avec harnais SEC-01 `sec01_pilot_test.go` 7 tests verts et
  `internal/connecteurs` avec `sec01_attaque_test.go` 15 tests verts — rejoués).
- `go run ./cmd/coord-demo` OK (19 CONFORME / 0 ECART, sortie standard) ;
  `go run ./cmd/vector-runner` OK (22/22).
- **Démonstration MCP locale : `go run ./cmd/coord-mcp -demo-local` → exit 0** ;
  trace `preuves/demo-mcp-locale-trace.txt`, sha256
  `43b8a07d9109a46f9235c61f81177670a7cff375b05aeefd6181433dcf94e208`
  (outils = exactement 2 en lecture ; refus mission inconnue sans fuite).
- Batteries Python sur le code livré : 24+13+11+13+10+17 (88+10) PASS, 0 FAIL ;
  v1.5 47/47 PASS, **manifeste d'intégrité des fichiers historiques vérifié**
  (coord/README.md et coord/index.html intacts, non modifiés par la mission).
- `-race` NON_EXÉCUTÉ localement (pas de gcc dans le sandbox) : couvert par le job
  CI **Race detector** étendu à mcpserver (garde de la boucle corrective précédente).

## Empreinte des modifications (diff contre main 10defea6)

Modifiés : `coord/go/internal/pilot/pilot.go`, `coord/go/cmd/coord-pilot/main.go`,
`coord/go/go.mod`, `.github/workflows/coord-go-ci.yml`. Nouveaux :
`coord/go/internal/mcpserver/{mcpserver,jeton,jeton_uid_posix,jeton_uid_windows}.go`,
`coord/go/internal/mcpserver/mcpserver_test.go`,
`coord/go/internal/pilot/jeton_lecture_test.go`, `coord/go/cmd/coord-mcp/main.go`,
`coord/go/go.sum`, `coord/mcp/README.md`, `coord/mcp/rapport-m-coord-mcp-01.md`,
`coord/mcp/preuves/demo-mcp-locale-trace.txt`. **Aucun** fichier des tables
normatives, du moteur, des connecteurs, du référentiel missions ni des documents
figés (manifeste M1) n'est modifié.

## Limites respectées

Lecture seule stricte ; aucun token console aux clients IA ; aucune duplication du
moteur ni des règles ; aucun appel payant (0 €, adaptateurs SIMULÉS) ; v1.5 non
activée ; aucune extension fournisseur développée ; future architecture distante
documentée, non développée. v1.4 reste la référence opérationnelle.

## Réserves

- `-race` couvert par CI uniquement (pas de gcc local).
- Protection du fichier de jeton : exigée sur TOUS les systèmes — POSIX
  0600 + propriétaire (refus 0644), Windows DACL natif restreint à
  l'utilisateur courant sans héritage (SEC-MCP-01) ; les liens symboliques
  sont refusés au chargement ET à l'écriture (SEC-MCP-02). Exécution Windows
  RÉELLE assurée par la CI multi-OS (le sandbox de développement est Linux).
- Compatibilité réelle des clients (Gemini CLI, Antigravity, Claude, OpenAI,
  Mistral) : NON_TESTÉE, hors périmètre ; qualifications futures distinctes.

## Corrections SEC-MCP-01/02 (ordre du pilote, 25/11/2026)

À la suite de la contre-vérification du pilote au SHA exact (A_CORRIGER), deux
corrections ciblées ont été apportées sur la même branche, sans nouvelle
mission ni modification d'architecture :

- **SEC-MCP-01 — protection effective sous Windows** : les bits POSIX étant
  sans objet, la garantie équivalente est désormais un **DACL natif**
  (`golang.org/x/sys/windows`, déjà requis) appliqué à l'écriture (ACL
  restreint au seul utilisateur courant, DACL **protégé** = aucun héritage du
  répertoire parent) et **vérifié en fail-closed au chargement** : DACL
  présent, protégé, sans ACE hérité, aucun titulaire autre que l'utilisateur
  courant — sinon REFUS. Un fichier créé sans cette protection (droits
  hérités) est refusé ; le test l'affirme en CI Windows.
- **SEC-MCP-02 — sécurisation de la création** : `EcrireJeton` remplace
  l'écriture directe (répertoire temporaire, nom fixe, `O_TRUNC`) : cible
  non régulière (lien symbolique/répertoire) refusée avant toute écriture,
  création EXCLUSIVE (`O_EXCL`) d'un fichier temporaire dans le même répertoire
  (aucune collision, aucune écrasement accidentel), fsync, protection
  effective AVANT le renommage atomique (rotation, les liens ne sont jamais
  suivis), nettoyage du temporaire en cas d'échec. `ChargerJeton` passe de
  `Stat` à `Lstat` : un lien symbolique vers un fichier de jeton valide est
  refusé.

Acquis conservés : jeton de lecture distinct additif, lecture seule stricte,
2 outils only, noyau sans transport, non-divulgation testée, 0 €, v1.5 non
activée, aucune extension fournisseur.

## Restent réservés au pilote

Fusion de la PR, clôture de #35, décisions d'activation, emballages fournisseurs
(missions ultérieures).

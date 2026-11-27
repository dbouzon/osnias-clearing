# COORD — Parcours client (front + gateway)

Mission M-COORD-FRONT-01 (issue #84). Interface web client rapide, sobre
et responsive, raccordée au backend Go maïeutique. Ce document décrit
l'architecture, l'installation de recette, la procédure complète et le
chemin de repli démo→réel.

## 1. Architecture

```
navigateur ── same origin ── coord-front (gateway, 127.0.0.1:8080)
                                ├── statiques : coord/front/
                                ├── /api/connexion, /api/session (cookie signé)
                                └── /api/projets* ──┬─ mode DÉMO  : moteur factice embarqué
                                                    └─ mode RÉEL : proxy → instance du compte
                                                                   (token pilote injecté côté serveur)
```

- **Front** : `coord/front/` — HTML/CSS/JS vanilla, aucun framework,
  aucun outil de construction. Un module par écran (`js/ecrans/*.js`,
  espace `Coord.ecrans`), i18n central (`Coord.t`), 5 langues (fr, en,
  de, es, zh-Hans), priorité : choix mémorisé (localStorage) > langue du
  navigateur > anglais. Responsive mobile-first (< 720 px : cartes).
- **Gateway** : `coord/go/cmd/coord-front` — sert le front ET l'API de
  la même origine (zéro CORS). Loopback uniquement (fail closed au
  démarrage). Stdlib Go uniquement, 0 €, aucune dépendance.
- **Isolation multi-compte** : un compte = une instance
  coord-maieutique (data dir propre) derrière le gateway ; le moteur
  n'est JAMAIS modifié pour le multi-compte.
- **Sécurité** : le token pilote d'une instance n'existe QUE dans le
  fichier de config du gateway (chmod 600, hors dépôt) ; il est
  injecté à chaque requête sortante, jamais envoyé au navigateur,
  jamais journalisé. La session navigateur est un cookie HttpOnly
  SameSite=Strict signé HMAC-SHA256. Un token compromis se révoque en
  redémarrant l'instance (le token change à chaque démarrage).
- **D1 respecté** : le paquet `internal/maieutique` est INTACT. La
  seule adaptation serveur (route additive lecture seule
  `GET /projets/{id}/fille`) est isolée dans un commit dédié, soumis à
  critique préalable.

### Seuils et conventions (plan v2 §5.4)

- Un module JS par écran, i18n obligatoire (`Coord.t`), aucun style en
  ligne, aucun build step.
- Point de revisite : si le front dépasse ~10 écrans ou exige du
  routage complexe, une micro-mission réévaluera l'opportunité d'un
  framework.

## 2. Fichier de configuration du gateway

Exemple complet à VALEURS FACTICES (`gateway.json`, chmod 600, jamais
commité) :

```json
{
  "secret_session": "changement-obligatoire-32-caracteres-min",
  "comptes": [
    {
      "compte": "alice",
      "url": "http://127.0.0.1:8091",
      "token": "jeton-affiche-par-linstance-alice",
      "demo": false,
      "dossier_drive": "/srv/coord/alice/drive",
      "remote_rclone": "",
      "timeout_rclone": 30,
      "export_sortie": false,
      "autoriser_ecriture_drive": false
    },
    {
      "compte": "bob",
      "url": "",
      "token": "",
      "demo": true,
      "dossier_drive": "/srv/coord/bob/drive"
    }
  ]
}
```

- `demo: true` : le compte est servi par le moteur factice embarqué
  (parcours complet déterministe, bannière « Données de démonstration »
  dans le front, aucune instance ni fournisseur requis).
- `demo: false` : chaque requête `/api/projets*` est proxifiée vers
  `url` avec l'en-tête `X-Coord-Pilot-Token: token`.
- Champs Drive (mission M-COORD-DRIVE-01) : `remote_rclone` non vide
  active l'adaptateur rclone (`dossier_drive` devient alors le chemin
  racine DANS le remote), `timeout_rclone` (s, défaut 30, clampé
  [5,120]), `export_sortie` (export du dossier de sortie à la
  transmission, non bloquant), `autoriser_ecriture_drive` (opt-in
  d'écriture). Détails et recette : §6.
- `secret_session` : 16 caractères minimum (refus au démarrage sinon).

## 3. Procédure de recette (T4)

```sh
# 0) Construire (module coord/go)
cd coord/go && go build -o /tmp/coord-front ./cmd/coord-front
go build -o /tmp/coord-maieutique ./cmd/coord-maieutique

# 1) Créer deux comptes factices
cd ../front && sh outils/creer-compte.sh alice /srv/coord
sh outils/creer-compte.sh bob /srv/coord

# 2) Lancer les ponts factices llm (endpoints loopback, 0 €),
#    puis une instance par compte réel (alice), bob reste en démo :
#    suivre la sortie de creer-compte.sh (data dir, port, token).

# 3) Rédiger gateway.json (ci-dessus, valeurs réelles locales),
#    chmod 600, puis :
/tmp/coord-front -config /srv/coord/gateway.json \
  -front /home/.../coord/front -addr 127.0.0.1:8080

# 4) Recette dans le navigateur : http://127.0.0.1:8080
```

Scénario de recette complet (mobile ET desktop, dans les 5 langues) :
création → dialogue (3 échanges : question, clarification, proposition
d'ordre en démo) → qualification (version v1 figée + empreinte) → devis
dérivé → REFUS (la mission reste « Définie · non lancée ») → ACCORD →
validation (jeton) → publication → transmission → mission fille
visible dans le suivi + journal complet.

Vérifications obligatoires :
- `ss -ltn` : le gateway et les instances n'écoutent QUE sur
  127.0.0.1 (aucun écouteur public, port 8080 non ouvert dans ufw) ;
- isolation : la session bob ne voit aucun projet d'alice (404) ;
- la page du compte en démo affiche la bannière DÉMO en permanence ;
- `go test ./cmd/coord-front/` et `python3 front/test-ui.py` verts.

## 4. Tests

- `go test ./cmd/coord-front/` : session/authentification, scénario
  démo complet, isolation inter-comptes, proxy réel (injection du
  token, jamais renvoyé au navigateur), adaptateur Drive factice
  (évasion hors racine refusée), parité i18n des 5 langues ; mission
  M-COORD-DRIVE-01 : adaptateur rclone (fail-closed, timeout, mapping
  d'erreurs couverts par un FAUX BINAIRE partout, y compris CI sans
  rclone ; intégration sur remote LOCAL sautée explicitement si le
  binaire est absent), routes d'écriture et d'export, option A de bout
  en bout (fichiers exportés présents ; export défaillant →
  transmission 200 + échec signalé + relance).
- `python3 front/test-ui.py` : parité des clés i18n, clés utilisées
  présentes dans les 5 langues, libellés de tous les états de la
  machine, scripts référencés par index.html, câblage de l'export du
  dossier de sortie (GET + relance POST).

## 5. Bascule démo → réel

1. Lancer l'instance coord-maieutique du compte (creer-compte.sh).
2. Dans `gateway.json`, passer le compte de `"demo": true` à
   `"demo": false` avec `url` et `token` réels.
3. Redémarrer le gateway (les sessions restent valides si
   `secret_session` est inchangé). Le front détecte la bascule via
   `GET /api/session` : la bannière DÉMO disparaît.

## 6. Drive réel via rclone (mission M-COORD-DRIVE-01)

Décision du pilote (suite 80 du protocole) : plugins sur étagère, pas de
développement OAuth/Drive ad hoc. Le raccordement Drive passe par
**rclone** (binaire unique, sans démon, OAuth Google intégré) ; le MCP
Drive officiel Google reste la cible de migration future. Toute
l'adaptation vit dans `cmd/coord-front` (rclone.go) : le front ne
connaît que l'interface `Dossiers`, le moteur n'est jamais touché (D1).

### 6.1 Installation de rclone

```sh
# Télécharger le binaire statique (adaptez la version/architecture) :
curl -LO https://downloads.rclone.org/rclone-current-linux-amd64.zip
unzip rclone-current-linux-amd64.zip && sudo cp rclone-*/rclone /usr/local/bin/
rclone version  # doit afficher la version attendue
```

Le binaire et son `rclone.conf` restent HORS du dépôt ; `rclone.conf`
en `chmod 600`.

### 6.2 Configuration du remote Google Drive (headless)

Sur un VPS sans navigateur, `rclone authorize` s'exécute en deux temps :

```sh
# 1) Créer le remote (répondre "n" quand rclone propose l'auto-config
#    locale, puis coller le résultat de l'étape 2) :
rclone config
#   n) new remote → nom : alice
#   type : drive
#   client_id / client_secret : ceux de VOTRE projet Google Cloud
#   (factices ici) ; scope : drive ; root_folder_id : laisser vide.

# 2) Sur une machine AVEC navigateur (même version de rclone) :
rclone authorize "drive" "id-factice.apps.googleusercontent.com" "secret-factice"
#   → se connecter au compte Google, autoriser, copier le token affiché.

# 3) Coller ce token dans la question de l'étape 1, puis vérifier :
rclone lsf alice:            # liste la racine du Drive du compte
```

Exemple de `~/.config/rclone/rclone.conf` (VALEURS FACTICES) :

```ini
[alice]
type = drive
client_id = id-factice.apps.googleusercontent.com
client_secret = secret-factice
token = {"access_token":"jeton-factice","refresh_token":"rafraichissement-factice"}
```

### 6.3 Configuration du gateway

```json
{
  "compte": "alice",
  "url": "http://127.0.0.1:8091",
  "token": "jeton-affiche-par-linstance-alice",
  "demo": false,
  "remote_rclone": "alice",
  "dossier_drive": "coord",
  "timeout_rclone": 30,
  "export_sortie": true,
  "autoriser_ecriture_drive": false
}
```

- `remote_rclone` : nom de la section rclone.conf (validé au démarrage :
  `[A-Za-z0-9_-]+`, un nom invalide refuse de lancer le gateway) ;
  non vide → adaptateur rclone, VIDE → adaptateur factice local
  (compatibilité zéro rupture).
- `dossier_drive` : avec rclone, chemin racine DANS le remote
  (périmètre du compte ; toute opération est préfixée, impossible
  d'en sortir).
- `timeout_rclone` : secondes par appel rclone, défaut 30, clampé
  [5, 120]. Un timeout donne `DRIVE_INDISPONIBLE` (503), jamais un
  appel pendu.
- `export_sortie` : option A — après une transmission réussie, le
  gateway copie `ordre.json`, `devis.json` et `journal.ndjson` dans le
  dossier de sortie DÉCLARÉ DANS LE CADRAGE (`dossier_sortie`).
  NON BLOQUANT : un échec d'export n'annule jamais la transmission ;
  l'état est visible (champ `export_sortie` de la réponse, bandeau
  dans l'écran Suivi avec relance manuelle `POST /api/drive/export/{id}`,
  motif journalisé côté gateway).
- `autoriser_ecriture_drive` : opt-in pour `POST /api/drive/document`
  (seul point d'écriture du parcours : session obligatoire, fail-closed
  strict, 1 Mo maximum, écriture limitée sous le dossier du compte).
  `false` par défaut.

### 6.4 Recette Drive sans clé Google (remote LOCAL)

Mêmes chemins de code qu'avec Google Drive, zéro clé : un remote
rclone de backend `alias` pointé sur un dossier local (c'est exactement
ce que font les tests d'intégration Go) :

```sh
export RCLONE_CONFIG_RECETTE_TYPE=alias
export RCLONE_CONFIG_RECETTE_REMOTE=/srv/coord/recette-drive
/tmp/coord-front -config /srv/coord/gateway.json -front .../front -addr 127.0.0.1:8080
```

Scénario de recette Drive (complète le scénario §3) :

1. compte alice (`remote_rclone` + `export_sortie`), compte bob
   (factice) : vérifier l'isolation des racines ;
2. parcours complet ; à la transmission : les trois fichiers
   apparaissent dans le dossier de sortie, l'écran Suivi affiche
   l'export réussi ;
3. casser le remote (supprimer les variables RCLONE_CONFIG_RECETTE_*
   et redémarrer : remote inconnu → DRIVE_ERREUR) : la transmission
   réussit QUAND MÊME (vérifié en recette), bandeau d'échec dans le
   suivi avec motif, relance manuelle réparatrice ;
4. retirer rclone du PATH : `DRIVE_INDISPONIBLE` (503) propre, le
   parcours sans Drive reste fonctionnel ;
5. `POST /api/drive/document` : 401 sans session, 403 si
   `autoriser_ecriture_drive` absent, 400 sur entrée interdite
   (`..`, `:`, > 1 Mo).

### 6.5 Diagnostic

| Symptôme | Cause probable |
| --- | --- |
| 503 `DRIVE_INDISPONIBLE` | rclone absent du PATH, ou timeout par appel dépassé |
| 502 `DRIVE_ERREUR` | rclone en échec : remote inconnu, Drive inaccessible, quota |
| 404 `DRIVE_ABSENT` | document demandé introuvable, ou aucun Drive pour le compte |
| 400 `DRIVE_REFUS` | entrée interdite (fail-closed) : dossier/nom invalide |
| export `echec` « aucun dossier de sortie » | cadrage sans `dossier_sortie` |

Aucune clé, aucun jeton, aucun `rclone.conf` dans le dépôt ; le token
OAuth vit uniquement dans `rclone.conf` (chmod 600) sur le VPS.


## 7. nginx (optionnel, TLS terminé en local)

En recette le gateway sert déjà tout en même origine. Pour un service
permanent local derrière nginx :

```nginx
server {
  listen 127.0.0.1:8443 ssl;
  server_name coord.local;
  ssl_certificate     /etc/ssl/coord.local.crt;
  ssl_certificate_key /etc/ssl/coord.local.key;
  location /api/ { proxy_pass http://127.0.0.1:8080; }
  location /     { proxy_pass http://127.0.0.1:8080; }
}
```

Aucun port n'est ouvert vers l'extérieur : la règle ufw reste fermée.

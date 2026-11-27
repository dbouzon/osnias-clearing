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
      "dossier_drive": "/srv/coord/alice/drive"
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
  (évasion hors racine refusée), parité i18n des 5 langues.
- `python3 front/test-ui.py` : parité des clés i18n, clés utilisées
  présentes dans les 5 langues, libellés de tous les états de la
  machine, scripts référencés par index.html.

## 5. Bascule démo → réel

1. Lancer l'instance coord-maieutique du compte (creer-compte.sh).
2. Dans `gateway.json`, passer le compte de `"demo": true` à
   `"demo": false` avec `url` et `token` réels.
3. Redémarrer le gateway (les sessions restent valides si
   `secret_session` est inchangé). Le front détecte la bascule via
   `GET /api/session` : la bannière DÉMO disparaît.

## 6. Feuille de route Google / Drive (accès non fournis)

- **Bouton « Se connecter avec Google »** : aujourd'hui en mode DÉMO
  (aucun credential OAuth dans le périmètre). Quand le pilote fournira
  les accès : échange d'authorization code géré par le gateway
  (redirect loopback, secret côté serveur uniquement).
- **Connecteur Drive** : le front ne connaît que l'interface
  `Dossiers` (lister/lire/écrire, `drive.go`). L'implémentation de
  recette est un dossier local factice par compte. Une seconde
  implémentation (API REST Google Drive) suffira le jour venu, SANS
  toucher au front ni au moteur. Ces étapes seront des micro-missions
  séparées avec critique préalable.

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

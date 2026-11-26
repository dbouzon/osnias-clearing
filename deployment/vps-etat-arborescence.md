# État du VPS et arborescence de déploiement — rapport du 26/11/2026

> Rapport d'état du serveur d'hébergement COORD/Osnias Clearing.
> Conformément aux règles de sécurité du dépôt public, les identifiants réseau
> (adresses IP, noms d'hôte) et les secrets ne figurent pas dans ce document ;
> ils sont consignés dans le Drive privé `coord/vps-nom.odt`.
>
> Missions liées : M-COORD-VPS-01 (issue #64, recette conforme), M-COORD-SSH-01
> (issue #66, en cours).

## 1. Plateforme

- VPS Lite Infomaniak, 1 vCPU, 2 Go RAM, 20 Go disque, Ubuntu 24.04 LTS.
- État au 26/11/2026 soir : disque 3,0/19 Go utilisés (17 %), mémoire 373 Mo
  utilisés / 1 594 Mo disponibles, uptime 2 h, noyau 6.8.0-106-generic.

## 2. Sécurité et services

| Élément | État |
| --- | --- |
| ufw | actif ; entrées autorisées : 22/tcp (ssh), 80/tcp (http), 443/tcp (https, IPv4+IPv6) ; défaut : deny incoming |
| fail2ban | actif, jail sshd ; 0 échec, 0 banni au rapport |
| unattended-upgrades | actif (mises à jour automatiques de sécurité) |
| sshd | actif ; authentification par clé uniquement ; clé hôte épinglée côté clients de confiance |
| nginx | actif ; sites : default (port 80) et osnias-clearing (port 8080, local uniquement, non ouvert dans ufw) |

## 3. Comptes de service

| Compte | uid | Shell | Home | Rôle |
| --- | --- | --- | --- | --- |
| coord | 999 | /usr/sbin/nologin | /srv/coord/private | service COORD, sans sudo |
| osnias | 997 | /usr/sbin/nologin | /srv/osnias-clearing/private | service Osnias Clearing, sans sudo |

Les comptes `coord` et `osnias` ne sont pas sudoers. L'installation des
releases est réservée au compte d'administration (`ubuntu`) via sudo.

## 4. Arborescence actuelle et plan

Deux racines, même motif (back end privé / front end public) :

```
/srv
├── coord/                        root:root 0755
│   ├── public/                   root:root 0755   (front end COORD, vide, nginx en lecture seule à terme)
│   └── private/                  root:coord 0750  (back end COORD)
│       ├── releases/             root:coord 0750  (livraisons installées par l'administrateur ; coord lecture seule)
│       ├── state/                coord:coord 0750 (état du service COORD)
│       └── recette/              coord:coord 0750 (traces et attestations de recette)
└── osnias-clearing/              root:root 0755
    ├── public/                   root:root 0755   (front end Osnias Clearing, servi par nginx sur le port 8080, local uniquement)
    └── private/                  root:osnias 0750 (back end Osnias Clearing)
        ├── releases/             root:osnias 0750
        ├── state/                osnias:osnias 0750
        └── recette/              osnias:osnias 0750
```

Règles effectives (vérifiées par 15 contrôles de preuve, issue #64) :

- Les comptes de service écrivent dans `state/` et `recette/` mais jamais dans
  `releases/` ni `public/`.
- Le serveur web (`www-data`) ne peut pas traverser `private/` ; il ne lit
  que `public/`.
- `public/` n'est jamais inscriptible par les comptes de service.

## 5. Plan de déploiement (cycle d'une release)

1. L'administrateur installe la livraison dans `private/releases/<sha>/`
   (jamais dans `public/` ni `state/`).
2. Le service concerné exécute depuis la release, en lecture seule ;
   il écrit son état dans `state/` et ses traces dans `recette/`.
3. Le front end est servi par nginx depuis `public/`, en lecture seule ;
   contenu installé par l'administrateur.
4. Retour arrière : version précédente dans `releases/` + retour de l'état ;
   chaque racine (`coord`, `osnias-clearing`) est indépendante
   (retours arrière séparés documentés dans l'issue #64).

## 6. Outils et environnement

- Go 1.27.4 en espace utilisateur (`~/.go-toolchain/go1.27.4`, tarball
  officiel, empreinte vérifiée) ; aucune installation Go système
  (`/usr/local/go` retiré) ; variables de travail :
  `GOTOOLCHAIN=local GOFLAGS=-mod=mod`.
- git 2.43.0 disponible pour les checkouts de recette isolés.

## 7. Recette de référence (M-COORD-VPS-01, résumé)

Au SHA `fc1bac7` : batteries Python v1.4 (PASS=17 FAIL=0), robustesse
(PASS=10 FAIL=0), démo v1.5 (RC=0) ; Go build/test : 14 packages
`coordengine` + `tools/ai-coordinator` verts ; mission simulée avec
interruptions SIGKILL réelles : persistance atomique attestée (empreinte
sha256 du journal enregistrée dans le snapshot = empreinte recalculée du
fichier), reprise post-crash saine. Rapport complet : Drive `coord/03-Recettes`
(recette-M-COORD-VPS-01-2026-11-26.md).

## 8. En cours et à venir

- M-COORD-SSH-01 (#66) : compte `coord-review` en lecture seule pour la
  session Codex/OpenAI du pilote (validation depuis le PC du pilote) ; à
  configurer après réception de la clé publique dédiée.
- Mise en production future : domaine + certificat TLS, exposition des
  fronts (80/443), ouverture du port 8080 à décider. Décisions réservées
  au pilote.

## 9. Confidentialité

Ce rapport ne contient aucune adresse IP, aucun secret, aucune clé, aucun
journal nominatif. Les preuves détaillées (commandes, sorties expurgées)
sont publiées dans les issues #64 et #66 ; les informations sensibles
restent dans le Drive privé.

#!/bin/sh
# creer-compte.sh — créer un compte client COORD (mission M-COORD-FRONT-01).
# Un compte = un data dir + une instance coord-maieutique loopback avec
# son propre token pilote + une entrée dans la config du gateway.
#
# Usage : ./outils/creer-compte.sh <nom> <repertoire-racine>
# Exemple : ./outils/creer-compte.sh alice /srv/coord
#
# Aucune valeur secrète n'est commitée : le fichier de config généré
# (chmod 600) reste sur la machine. Les fournisseurs ci-dessous sont
# FACTICES (endpoints loopback, 0 €) : en recette, lancez d'abord les
# ponts factices (voir docs/front.md §3).
set -eu

NOM="${1:-}"
RACINE="${2:-}"

if [ -z "$NOM" ] || [ -z "$RACINE" ]; then
  echo "Usage : $0 <nom> <repertoire-racine>" >&2
  exit 1
fi

DATA="$RACINE/$NOM/state"
DRIVE="$RACINE/$NOM/drive"
mkdir -p "$DATA" "$DRIVE"

TOKEN=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
PORT_MOTOR=$(awk -v min=8091 -v max=9090 'BEGIN{srand();print int(min+rand()*(max-min+1))}')
PORT_GATEWAY=8080

cat <<EOF

Compte COORD « $NOM » créé :
  data dir    : $DATA
  dossier Drive factice : $DRIVE

1) Lancer l'instance maïeutique (boucle séparée, token en variable) :

     COORD_TOKEN="$TOKEN" \\
     go run ./cmd/coord-maieutique \\
       -data "$DATA" \\
       -addr "127.0.0.1:$PORT_MOTOR" \\
       -fournisseur openai-dir=http://127.0.0.1:8787/llm \\
       -fournisseur openai-exec=http://127.0.0.1:8788/llm

   Le service affiche son token à chaque démarrage : reprenez CETTE
   valeur dans la config du gateway si vous relancez l'instance.

2) Ajouter l'entrée suivante au fichier de config du gateway
   (ex. $RACINE/gateway.json, chmod 600, JAMAIS commité) :

     {
       "compte": "$NOM",
       "url": "http://127.0.0.1:$PORT_MOTOR",
       "token": "$TOKEN",
       "demo": false,
       "dossier_drive": "$DRIVE"
     }

3) Lancer le gateway :

     go run ./cmd/coord-front -config $RACINE/gateway.json \\
       -front ../front -addr 127.0.0.1:$PORT_GATEWAY

4) Ouvrir http://127.0.0.1:$PORT_GATEWAY (loopback uniquement).

Mode DÉMO (sans backend) : remplacez "url" et "token" par la seule
clé "demo": true — le moteur factice embarqué répond au parcours
complet sans instance ni fournisseur.

EOF

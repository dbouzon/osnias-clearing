# Recette de qualification VPS Linux 2 Go (72 h) — pont coord.llm.v1

Mission M-COORD-LLMBRIDGE-01 (issue #62), T5. **NON_EXÉCUTÉE** : aucun achat
ni déploiement VPS n'est autorisé dans cette mission ; ce document prépare la
recette reproductible pour exécution ultérieure sur un environnement autorisé.
La capacité sur 2 Go devra être **démontrée par mesure**, jamais présumée.

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX Apache-2.0.

## Prérequis

- VPS Linux (Debian/Ubuntu) 2 Go RAM, 1 vCPU, disque ≥ 10 Go, swap 2 Go.
- OS de base mesuré AU REPOS avant tout déploiement (`free -m`, `vmstat 1 10`).
- Go 1.27.x, Python 3.10+, SQLite.
- litellm-core==1.111.0 installé (mesurer l'empreinte disque réelle).

## Déploiement sous test

1. Moteur COORD (binaires `coord-pilot` ou `coord-mission`), données SQLite.
2. Pont : `python3 coord/llmbridge/bridge.py --config <config>` (jeton par
   variable d'environnement, fichier 0600 jamais dans le dépôt).
3. Concurrence bornée : 1 pont, 1 moteur, au plus 2 appels inférence
   simultanés (bornes du contrat : corps 2 Mo, délai backend 60 s).

## Mesures à relever (journal horodaté)

| Mesure | Outil | Fréquence |
|---|---|---|
| RAM totale/used/available, swap used | `free -m` | toutes les 60 s |
| RSS pont, RSS moteur | `/proc/<pid>/status` (VmRSS) | toutes les 60 s |
| CPU user/system/idle/iowait | `vmstat 1` | continu |
| Latence médiane/p95 POST /v1/chat/completions | `mesures.py` adapté | début, 24 h, 48 h, 71 h |
| Démarrage pont (BRIDGE_PRET) | `mesures.py` | chaque redémarrage |
| Redémarrages du pont | journal | à chaque événement |
| OOM killer / dmesg anomalies | `dmesg -T` | toutes les heures |

## Scénario de stabilité 72 h

- S1 charge de fond : une mission factice complète (critique → exécution →
  réception → ACK) toutes les 30 s via `llmbridge_e2e`-like, 72 h.
- S2 pic borné : 10 missions en 5 minutes, deux fois par jour.
- S3 redémarrage du pont toutes les 12 h (sans état : aucune donnée perdue,
  l'état vit dans le moteur Go).
- S4 arrêt brutal (kill -9) du pont puis reprise : la mission en cours doit
  retrouver un appel incertain refusé au doublon côté Go (reprise = pilote).

## Critères GO/NO-GO (pré-déclarés)

- GO si : available ≥ 256 Mo en permanence ; swap used < 100 Mo hors S4 ;
  aucun OOM ; latence p95 < 50 ms en charge de fond ; aucun crash moteur ;
  toutes les missions de S1 abouties ou refusées proprement (fail closed).
- NO-GO sinon, avec mesures à l'appui. Aucune extrapolation : uniquement les
  valeurs relevées.

# M-AIC-02 T02 — Rebasing, compatibilité CLI et préparation de la recette réelle

Tranche T02 (22 novembre 2026). Base : PR #5 (branche mission/aic-02-mistral-cli), initiallement fondée sur mission/ai-coordinator-v01.

## 1. Rebasing et base de la PR #5

PR #4 a été fusionnée dans main le 22/11/2026 (merge 15295e90) : le coordinateur (T02+T03 de M-AIC-01) figure désormais sur main. Actions :

- Base de la PR #5 corrigée : mission/ai-coordinator-v01 → main ; diff vérifié via l'API : exactement les 8 fichiers du périmètre T01, état « clean », aucune autre divergence de contenu.
- Branche synchronisée avec main (merge de mise à jour GitHub, HEAD 7042554d) : main est désormais ancêtre de la branche.
- Réserve de méthode : la réécriture d'historique stricte (rebase au sens Git) n'est pas disponible via le connecteur GitHub (aucun push forcé possible) ; le résultat est équivalent au niveau du contenu et de la fusion future, documenté ici sans ambiguïté.

## 2. Compatibilité avec la CLI Mistral Code réelle

Vérifiée contrée la référence officielle (docs.mistral.ai/vibe/code/cli) : le mode programmatique est `vibe --prompt` (stdin accepté quand aucun texte n'accompagne -p), `--trust` requis en programmatique, `--max-turns` recommandé, `MISTRAL_API_KEY` par l'environnement, codes de sortie 0-3 (3 = arrêt avant achèvement). Corrigés dans l'adaptateur :

- défauts bin/args : `vibe --prompt --trust` (adaptateur et configuration) ;
- `--max-turns` injecté depuis `AIC_MISTRAL_MAX_ITER` (sans doublon si déjà fourni) ;
- code de sortie restitué dans le message d'incident et le journal ;
- `env.example` et commentaires mis à jour.

Tests : 49/49 PASS (42 non-régression + 2 compat/exit-code + 1 journal + 4 outil de recette dont un bout-en-bout simulé), go vet/gofmt clean, count=3 OK. go test -race toujours impossible dans la sandbox (cgo/gcc absents) : à exécuter sur la machine cible (procédure, étape 0).

Note : la CLI `vibe` elle-même ne peut pas être installée dans la sandbox Vibe (installation de paquets interdite par l'environnement, tentée une fois puis abandonnée conformément au message système). La vérification de compatibilité est donc documentaire (référence officielle) + contractuelle (shim), pas une exécution réelle ; cette distinction est conservée jusqu'à la recette sur machine.

## 3. Outils de recette ajoutés (lecture seule / boucle locale)

- `cmd/aic-journal` : preuve de recette en lecture seule (missions, états, livraisons, journal des agents) ; `Store.Journal` ajouté au store.
- `cmd/aic-recette-inject` : livre un webhook issue_comment signé HMAC au récepteur, refus structurel hors boucle locale, forcing natif (aucune écriture GitHub réelle, AIC_GITHUB_MODE=off requis dans la procédure).

## 4. Exécution réelle : décision du responsable (22/11/2026)

La mission réelle bornée sera exécutée sur sa machine ; la clé API sera fournie par variable d'environnement sur la machine cible. La procédure complète est publiée : `docs/aic-02/t02-procedure-recette.md` (prérequis, contrat direct, coordinateur de bout en bout, reprise après interruption, preuves à collecter). Aucune manipulation Android/Termux demandée, aucune exposition réseau : tout reste en boucle locale (AIC_LISTEN_ADDR=127.0.0.1, injection locale uniquement, GitHub simulé).

## 5. État à la clôture de cette série de travaux

- Exécuté et vérifié : points 1 et 2 de l'ordre, préparation des points 3 à 5.
- En attente : exécution par le responsable de la procédure de recette sur sa machine, retour des preuves (sorties, aic-journal, run-output.txt), puis rédaction du rapport de recette final distinguant explicitement tests simulés et exécution réelle.
- Git : branche mission/aic-02-mistral-cli, commits publiés et vérifiés au fil de la tranche (dernier SHA : section Git du rapport de réception). PR #5 en Draft, base main, toute fusion suspendue à validation explicite.

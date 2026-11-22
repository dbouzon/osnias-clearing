# M-AIC-02 T02 — Procédure de recette sur machine locale (CLI Mistral Code réelle)

Cette procédure exécute une mission réelle bornée sur votre machine, sans exposition réseau (tout en boucle locale : `AIC_LISTEN_ADDR=127.0.0.1`), sans écriture GitHub réelle (`AIC_GITHUB_MODE=off` : simulateur), sans fusion, sans secret dans le dépôt. La clé API est fournie uniquement par variable d'environnement sur la machine cible.

## Prérequis

1. Go 1.26 ou ultérieur (`go version`).
2. La CLI Mistral Code installée : `curl -LsSf https://mistral.ai/vibe/install.sh | bash`, puis `vibe --version` doit répondre.
3. La clé API dans l'environnement : `MISTRAL_API_KEY` (ou fournie à Vibe par son keyring). Ne jamais la versionner.
4. Le dépôt sur la branche de mission : `git fetch && git checkout mission/aic-02-mistral-cli`, répertoire `tools/ai-coordinator`.

## Étape 0 — Non-régression locale

```
go build ./...
go vet ./...
gofmt -l .
go test -count=1 ./...
go test -race ./...   compilateur C requis ; à exécuter ici (impossible dans la sandbox Vibe)
```

Attendu : 49/49 tests PASS, aucune ligne de gofmt, -race sans incident.

## Étape 1 — Contrat direct avec la CLI réelle (référence contractuelle)

Répertoire isolé, mission bornée à 5 tours, approbations refusées par défaut (mode programmatique) :

```
mkdir -p recette-direct && cd recette-direct
echo "Mission AIC-DIRECT : rédiger un fichier notes.md de trois lignes décrivant ce répertoire. Ne jamais effectuer de push." | vibe --prompt --trust --max-turns 5 ; echo "exit=$?"
ls ; cat notes.md 2>/dev/null
```

Attendus : `exit=0` ; le fichier produit ; aucun prompt d'approbation (mode programmatique). Un `exit=3` signale une limite atteinte (à documenter), `exit=1/2` un incident de démarrage ou d'API.

## Étape 2 — Coordinateur de bout en bout, mission réelle

Dans un terminal, depuis `tools/ai-coordinator` :

```
go build -o aic-coordinator ./cmd/aic-coordinator
go build -o aic-recette-inject ./cmd/aic-recette-inject
go build -o aic-journal ./cmd/aic-journal

SecretHMAC=$(openssl rand -hex 32)   (Windows PowerShell : -join ((1..32)|%{'{0:x2}' -f (Get-Random -Max 256)}))
AIC_WEBHOOK_SECRET=$SecretHMAC
AIC_AUTHORIZED_LOGINS=dbouzon
AIC_GITHUB_MODE=off
AIC_MISTRAL_MODE=cli
AIC_MISTRAL_BIN=vibe
AIC_MISTRAL_MAX_ITER=5
AIC_MISTRAL_TIMEOUT=300
AIC_LISTEN_ADDR=127.0.0.1:8080
AIC_DB_PATH=recette.sqlite
AIC_WORK_ROOT=recette-work
./aic-coordinator
```

Dans un second terminal (mêmes valeurs de `AIC_WEBHOOK_SECRET`) :

```
./aic-recette-inject -addr 127.0.0.1:8080 -secret "$AIC_WEBHOOK_SECRET" -issue 800 -body "/aic start" -delivery recette-r1
./aic-journal -db recette.sqlite -mission AIC-800
```

Attendus dans la sortie de `aic-journal` :

- mission AIC-800 état `AWAITING_APPROVAL` (revue simulée favorable ; la validation humaine `/aic approve` reste hors recette) ;
- journal : entrées `mistral-code/start`, `mistral-code/done`, jamais la clé API ;
- `livraisons=1` et une référence de livraison non vide (le rapport de la CLI est enregistré durablement) ;
- dans `recette-work/AIC-800/` : `run-output.txt` contenant le rapport réel de la CLI.

## Étape 3 — Reprise après interruption

Répéter l'étape 2 avec une mission plus longue (par exemple « rédiger dix fichiers et un rapport »), puis interrompre : arrêter le coordinateur (Ctrl+C) pendant l'exécution RUNNING. Redémarrer ensuite avec :

```
AIC_RUN_STALE_INTERVAL=2 ./aic-coordinator
```

Attendus dans `aic-journal` : ` coordinator/resume : verrou périmé repris après crash` ; la mission progresse jusqu'à `AWAITING_APPROVAL` ; `livraisons=1` (aucune double livraison ; la reprise réutilise la clé d'idempotence). Répéter éventuellement plusieurs fois : aucune duplication.

## Étape 4 — Preuves à collecter pour le rapport de recette

1. Sorties complètes des étapes 0 à 3 (commandes et résultats).
2. Le contenu `aic-journal` final de chaque mission.
3. Le fichier `recette-work/AIC-800/run-output.txt`.
4. La version exacte de la CLI (`vibe --version`) et le système d'exploitation.

## Limites connues

- `AIC_GITHUB_MODE=off` : les livraisons Draft et commentaires restent simulés ; la couche GitHub réelle (qualifiée en T03 sur simulateur) n'est pas re-recettée ici.
- L'écho des commandes est en syntaxe bash ; sous PowerShell, exporter les variables par `$env:AIC_...="..."`.
- Termux/Android : hors périmètre de cette tranche (aucune manipulation demandée, aucune exposition).

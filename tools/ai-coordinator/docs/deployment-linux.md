# Guide de déploiement Linux — Osnias AI Coordinator (T02)

Document indicatif établi en T02. Le déploiement réel fait l'objet de la tranche T04 sous autorisation distincte. Aucune ressource cloud n'a été engagée à ce stade.

## Prérequis

- Linux x86_64 ou arm64, Go 1.24 ou supérieur.
- Aucune dépendance cgo : SQLite est assuré par modernc.org/sqlite (pur Go). Aucun compilateur C n'est requis.
- Prévoir plus tard : un VPS européen, TLS (reverse proxy), un compte GitHub dédié au coordinateur.

## Construction

```sh
cd tools/ai-coordinator
CGO_ENABLED=0 go build -o /usr/local/bin/aic-coordinator ./cmd/aic-coordinator
go vet ./...
go test -count=1 ./...
```

## Configuration

Copier `env.example` vers l'environnement du service (jamais dans le dépôt). Variables principales :

- `AIC_WEBHOOK_SECRET` (obligatoire) : secret HMAC des webhooks GitHub.
- `AIC_LISTEN_ADDR` : écoute locale uniquement en T02 (127.0.0.1:8080).
- `AIC_COORDINATOR_LOGIN` : compte bot, ses écritures sont ignorées (anti-boucle).
- `AIC_AUTHORIZED_LOGINS` : identités habilitées à émettre les commandes /aic (séparées par des virgules ; liste vide = refus de toute commande).
- `AIC_MISTRAL_MODE=simulated`, `AIC_OPENAI_MODE=simulated` : seuls modes autorisés en T02.
- `AIC_MAX_CORRECTIONS` (défaut 3), `AIC_MISTRAL_TIMEOUT` (défaut 600 s), `AIC_MISTRAL_MAX_ITER` (défaut 40), `AIC_MISTRAL_BUDGET_STEPS` (défaut 50).
- `AIC_DB_PATH`, `AIC_WORK_ROOT` : SQLite et répertoires de travail isolés.

## Exécution en service (systemd, modèle)

```ini
[Unit]
Description=Osnias AI Coordinator
After=network.target

[Service]
User=aic
Group=aic
EnvironmentFile=/etc/aic/aic.env
ExecStart=/usr/local/bin/aic-coordinator
Restart=on-failure
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=/var/lib/aic

[Install]
WantedBy=multi-user.target
```

## Points de sécurité au déploiement

- Utilisateur UNIX dédié sans privilège ; le coordinateur n'exécute aucune commande arbitraire issue de GitHub (garde-fou dans github.go).
- TLS à terminer sur un reverse proxy ; webhook non exposé directement en T02 (tests locaux).
- Secret HMAC et futures clés d'API : uniquement dans l'environnement du service, jamais versionnés, jamais journalisés.
- Reprise : au démarrage, le démon relit les missions non terminées ; la déduplication persistante interdit toute double exécution.
- Fusion : hors du périmètre du coordinateur par conception ; voir le rapport de sécurité.

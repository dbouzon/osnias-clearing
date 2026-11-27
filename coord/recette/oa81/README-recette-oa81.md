# Recette M-COORD-OPENAI-VPS-01 — guide reproductible (FR)

Mission #81 (ordre v1.0). Raccordement de l'API OpenAI comme relecteur
COORD v1.5 sur le VPS via le rail privé `coord.llm.v1`, avec réutilisation
intégrale de l'assemblage Go/pont de #78 : **zéro modification moteur/pont**.
Exécutant : Mistral (SIMULÉ, factice). Relecteur : OpenAI
(`gpt-4o-mini-2024-07-18`, REEL). Pilote : Denis Bouzon.

## Composants (ce dossier)

| Fichier | Rôle |
| --- | --- |
| `recette-oa81.sh` | driver recette SIMULÉE (blocs A/B/D/E, rejets pont, plafond jetons), adapté du kit mvps01 de #78, 0 € |
| `recette-vault-oa81.sh` | driver recette MONÉTAIRE REEL 0 € derrière pont loopback (voûte : plafond avant envoi, réservation, épuisement, cycle REEL complet via témoin) |
| `temoin-oa81.py` | témoin factice OpenAI loopback 8791 : réponse verdict conforme `coord.llm.v1`, journalisation par requête (model, destination, jamais la charge) |
| `config-mauvais.json` | destinations fermées du pont vers `../mvps01/mauvais.py` (rejets fournisseur) |
| `config-oa81-vault.json` | destinations du pont vers le témoin pour la voûte monétaire |
| `config-reel-oa81.json` | config REEL fournisseur `pont-recep` pour la voûte (tarifs 15/60 c/M + 1 c/requête, bornes 1000/800, retraits 0) |
| `config-reel-essai-oa81.json` | config REEL de l'essai (plafond pilote 5 centimes, retraits 0) |
| `config-pont-essai-reel.json` | destinations réelles de l'essai : `api.openai.com/v1`, clé par `cle_env OPENAI_API_KEY` |
| `config-pont-essai-rep.json` | destinations de répétition 0 € : vers le témoin 8791 en lieu et place d'OpenAI |
| `livrable-defaut.txt` | livrable synthétique à défauts connus (« 200 centimes = 20 euros », « 0,006 centime ») |
| `essai-reel-oa81.sh` | driver de l'essai réel : modes `rep` (répétition 0 €) et `reel` (UN SEUL appel OpenAI) |
| `../../go/cmd/harnais-oa81-reel/main.go` | harnais REEL de #78 : flags `-config-reel` (à CHAQUE invocation, sinon retombée SIMULÉ silencieuse) et `-plafond-centimes` |

## Topologie (tout en 127.0.0.1 sauf l'appel unique)

```
recette simulée    : moteur ──> pont 8790 --factice           [cycle nominal]
rejets fournisseur : moteur ──> pont 8795 --config mauvais ──> mauvais.py 8792
voûte monétaire    : moteur ──> pont 8790 ──> témoin 8791     [REEL derrière loopback, 0 €]
essai répétition   : moteur ──> pont 8790 ──> témoin 8791     [0 €]
essai réel         : moteur ──> pont 8790 ──> https://api.openai.com/v1  [1 appel, plafond 5 c]
```

## Flux de travail de l'essai (réserve doc 2 du plan v1)

```
creer REEL (plafond 5 c, 0 appel)
  └─ livrable synthétique posé (défauts volontaires)
       └─ executer SIMULÉ (pont factice 8793) : PLANIFIEE → EXECUTION, 0 €
            └─ soumettre SIMULÉ : EXECUTION → DECISION_ATTENDUE, 0 €
                 └─ recevoir REEL : L'APPEL UNIQUE via coord.llm.v1
                      ├─ APPEL_RESERVE journalisé AVANT l'appel (enveloppe 3 c)
                      ├─ réponse OpenAI → verdict structuré fail-closed
                      └─ DECISION_ATTENDUE → EN_CORRECTION (verdict A_CORRIGER attendu)
```

Ordre imposé par la table fermée N-10 : `executer` est requis avant
`soumettre` (PLANIFIEE → EXECUTION → DECISION_ATTENDUE). Toute séquence
incomplète est refusée avant appel, sans consommation.

## Gestion et sécurisation des secrets (réserve doc 1 du plan v1)

- Aucune clé n'est JAMAIS écrite dans ce dépôt, sur le VPS (fichier), ni
  dans les journaux. La clé OpenAI passe exclusivement par variable
  d'environnement : `cle_env: OPENAI_API_KEY` dans les configs, injectée
  par `env` au lancement : `sudo -u coord env OPENAI_API_KEY=… bash essai-reel-oa81.sh reel`.
- Le script refuse de s'exécuter en mode `reel` sans la clé en
  environnement (exit 3, aucun appel).
- Contrôles automatiques en fin de run : grep du secret dans
  run/state/workspace (FAIL si trouvé), grep de traces `sk-`, et grep
  du secret dans les sorties du pré-contrôle.
- Les journaux du pont et du témoin n'enregistrent jamais la charge ni
  la clé (chemin, model, statut, durée).
- Recommandation pilote : révoquer/régénérer la clé après usage réel.

## Procédures d'erreurs inattendues (réserve doc 3 du plan v1)

- **Suspension sans renvoi** : tout code retour non attendu de la
  réception (≠ 0 et ≠ 2) marque `SUSPENDRE, aucun renvoi automatique`.
  La décision (renvoi, rejeu, abandon) appartient au pilote ; le moteur
  refuse toute reprise automatique (`APPEL_EN_COURS_NON_RESOLU`) tant que
  l'appel incertain n'est pas résolu par `resoudre`.
- **Pré-contrôle 0 €** : GET `/v1/models` avant tout appel ; code ≠ 200
  ou modèle absent du catalogue → suspension, aucun appel.
- **Écouteurs résiduels** : pré-contrôle en début de script (refus si
  un port 879x est occupé) et contrôle en fin (FAIL si résiduel). Tuer
  par PID vérifié via `/proc/<pid>/cmdline`, jamais par groupe.
- **Budget** : refus `BUDGET_DEPASSE` AVANT tout envoi réseau ;
  réservation conservatrice journalisée (`APPEL_RESERVE`), écart réel
  journalisé (`APPEL_SURPLUS`) sans blocage.
- **Idempotence** : chaque run d'essai purge l'état de sa mission en
  début de script ; les runs sont horodatés et isolés dans
  `recette/oa81/essai-<mode>-<instant>/`.

## Exécution

```sh
# 1. recette SIMULÉE (0 €)               → attendu PASS=71 FAIL=0
sudo -u coord bash /srv/coord/private/workspace/oa81/recette-oa81.sh
# 2. voûte monétaire REEL derrière témoin (0 €)  → attendu PASS=31 FAIL=0
sudo -u coord bash /srv/coord/private/workspace/oa81/recette-vault-oa81.sh
# 3. répétition de l'essai (0 €)          → attendu PASS=21 FAIL=0
sudo -u coord bash /srv/coord/private/workspace/oa81/essai-reel-oa81.sh rep
# 4. essai réel (UN SEUL appel, plafond 5 c)      → attendu PASS=21 FAIL=0, verdict A_CORRIGER
sudo -u coord env OPENAI_API_KEY=… bash /srv/coord/private/workspace/oa81/essai-reel-oa81.sh reel
```

## Attendus clés (preuves exécutées, 27/11/2026)

- Recette SIMULÉE `run-20261127T102533Z` : **PASS=71 FAIL=0**.
- Voûte monétaire `run-vault-20261127T102758Z` : **PASS=31 FAIL=0**.
- Répétition 0 € `essai-rep-20261127T103812Z` : **PASS=21 FAIL=0**.
- Essai réel `essai-reel-20261127T104800Z` : **PASS=21 FAIL=0**,
  verdict OpenAI **A_CORRIGER** (rc=2), 2/2 défauts semés détectés,
  exactement 1 inférence vers `api.openai.com`, réservation 3 c,
  consommation 1 appel / 558 jetons (~1 centime réel).

## Preuves

Traces dans `/srv/coord/private/recette/oa81/` (runs + `empreintes.txt`) ;
état persisté dans `/srv/coord/private/state/oa81/`. Empreintes sha256
consignées dans l'issue #81. Retour arrière : archive du workspace puis
purge, conservations de `state/` et `recette/`.

# Recette M-COORD-MISTRAL-VPS-01 — guide reproductible (FR)

Mission #78 (ordre v1.0). Recette simulée du moteur COORD v1.5 (main
`0a869f4e`) + rail privé `coord.llm.v1` sur VPS, 100 % loopback, 0 €,
aucune clé vraie. Exécutant : Mistral. Relecteur : OpenAI. Pilote : Denis Bouzon.

## Composants (ce dossier)

| Fichier | Rôle |
| --- | --- |
| `mauvais.py` | fournisseur défaillant loopback 8792 : scénarios par modèle (divergent, finish-length, usage-absent, 429, 500, lent), journal JSONL par requête (chemin, model, authorization booléen — jamais la charge) |
| `config-mauvais.json` | destinations fermées du pont vers `mauvais.py` (préfixe_sdk `openai/`, clé par variable d'environnement uniquement) |
| `recette-mvps01.sh` | driver S1..S6 : rejets pont, cycle nominal + rejeu D5, arrêt réel + issue incertaine, rejets moteur, plafond avant envoi, contrôles finaux |
| `../go/cmd/harnais-mvps01/main.go` | harnais de recette : une transition par processus, modes creer/critiquer/executer/soumettre/recevoir/acker/cloturer/resoudre/rejeu, flags `-pont`, `-modele-exec/-modele-recep`, `-profil-exec`, `-retraits`, `-rejeu-cible`, plafonds |

## Topologie (tout en 127.0.0.1)

```
moteur (harnais-mvps01) ──HTTP──> pont 8790 (--factice)            [cycle nominal]
moteur (harnais-mvps01) ──HTTP──> pont 8795 (--config mauvais) ──> mauvais.py 8792  [rejets]
```

Le pont factice embarque les scénarios nominaux (MANDAT V1 → livrable
erroné `SCENARIO:LIVRABLE_A_CORRIGER` → A_CORRIGER 2 points ; MANDAT V2 →
corrigé → ACCEPTEE ; CRITIQUE DE PLAN → PLAN_ACCEPTE). Jeton par
`COORD_BRIDGE_JETON`, journal par `COORD_BRIDGE_JOURNAL` (une ligne JSONL
par inférence, sans charge ni jeton).

## Exécution

```sh
sudo -u coord bash /srv/coord/private/mvps01/recette-mvps01.sh
```

Attendu : `PASS=70 FAIL=0`, sortie 0, aucune trace de secret, aucun
écouteur 879x résiduel, nginx et la page témoin intacts.

## Attendus clés (contrat D5, #76)

- Rejeu explicite d'une opération TERMINÉE : refus `OPERATION_DEJA_TERMINEE`
  (zéro appel, zéro budget), `OPERATION_REJETEE` au registre sous le MÊME
  `id_operation`, aucune nouvelle OUVERTE, journal du pont inchangé.
- Rejeu d'une opération ÉCHOUÉE (429) : relance avec le MÊME
  `id_operation`, clé d'idempotence neuve, nouvelle requête fournisseur.
- Arrêt réel (SIGKILL du harnais par PID vérifié, jamais par groupe)
  pendant un appel : `appel_en_cours` persisté, reprise automatique
  refusée (`APPEL_EN_COURS_NON_RESOLU`), résolution pilote `PERDU`,
  cycle mené à ACK_RECU.
- Plafond de jetons : refus `BUDGET_DEPASSE` AVANT tout envoi, journal
  du pont inchangé.

## Preuves

Traces dans `/srv/coord/private/recette/mvps01/run-<instant>/`
(recette.log, empreintes.txt, sorties par étape) ; état persisté dans
`/srv/coord/private/state/mvps01/` ; journal de bilan
`dernier-bilan.txt`. Empreintes sha256 consignées dans l'issue #78.

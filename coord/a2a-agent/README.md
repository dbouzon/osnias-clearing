# coord/a2a-agent — socle serveur agent A2A et connecteur Mistral (M-COORD-LLM-03)

Module Go autonome (mission M-COORD-LLM-03, COORD v1.5). Il fournit :

- un **socle serveur A2A mutualisable** (`socle/`), réutilisable pour d'autres
  fournisseurs (OpenAI, Anthropic, etc.) sans duplication du rail ;
- un **connecteur Mistral à deux voies** (`mistral/` + `runner/`) :
  - voie **API** : appels directs à l'API Mistral (emprunts du rail existant,
    verdicts ancrés, mode factice pour la qualification simulée) ;
  - voie **Vibe** : exécution du binaire Vibe CLI en sous-processus supervisé
    (liste blanche d'environnement, expurgation des secrets, codes de sortie
    0/1/2/3, récolte `export.json` en échec fermé) ;
- une **CLI simulée** `vibe-factice` (cmd/vibe-factice) qui rejoue tous les
  scénarios de recette S11..S20 sans clé, sans réseau, à budget nul ;
- une **CLI de démonstration** `coord-agent -demo-local` (cmd/coord-agent) qui
  exécute un cycle complet (critique de plan → exécution → correction) en
  mode factice.

Le module ne dépend pas du moteur `coord/go` en test : la recette est
autonome. Les chemins `coord/a2a` (rail) et `coord/go` (moteur) restent
inchangés.

## Écart documenté par rapport au plan v2

Le plan v2 décrivait « réponse immédiate + suivi asynchrone ». L'exécution
réelle du travail se fait **dans l'appel `message/send`** : l'adaptateur
capture l'artefact à l'envoi et refuse tout achèvement asynchrone
(verdict `RESULTAT_NON_TRANSMIS`). La borne de 20 s est une limite de suivi
post-envoi, pas un délai d'exécution. Le client reste donc bloqué pendant
l'exécution ; aucun changement de contrat n'est introduit.

## Composants

| Répertoire | Rôle |
| --- | --- |
| `socle/` | Enveloppe du rail : jeton obligatoire, carte, écoute loopback |
| `runner/` | Superviseur de sous-processus : env en liste blanche, groupe de processus, codes 0/1/2/3, `export.json` fail-closed |
| `mistral/` | Agent A2A Mistral : routage des deux voies, verdicts ancrés, registre de sessions |
| `cmd/vibe-factice/` | CLI simulée pour la qualification simulée |
| `cmd/coord-agent/` | CLI serveur + démonstration locale |

## Usage

```sh
cd coord/a2a-agent
go vet ./...
go build ./...
go test ./...            # recette simulée complète, sans clé, sans réseau
go run ./cmd/coord-agent -demo-local
```

Aucune clé API ne figure dans ce module : les configurations réelles vivent
dans des fichiers locaux hors repo. Les tests et la démonstration tournent en mode
factice (budget 0 €, qualification NON_EXÉCUTÉE sur infrastructure réelle).

## Frontières de sécurité

- Liste blanche d'environnement stricte pour le sous-processus Vibe
  (le `PATH` seul est transmis ; l'héritage complet est refusé).
- Expurgation des motifs de secrets connus (`sk-`, `AIza`, `gh?_`, `xoxb-`)
  avant toute écriture durable ; présence d'un secret dans `export.json`
  ⇒ verdict en échec fermé.
- Socket loopback uniquement, jeton obligatoire (`ErrJetonVide`).
- Registre de sessions atomique, permissions 0600.

## Qualification (B1..B5) : NON_EXÉCUTÉE

Conformément à COORD v1.5, la qualification sur infrastructure réelle (VPS,
binaires réels, réseau fournisseur) n'est **pas exécutée** dans cette
mission. Elle nécessitera une autorisation distincte du pilote.

# Rapport de mission — M-COORD-LLM-03 : serveur agent A2A et connecteur Mistral API/Vibe

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Mission : issue [#54](https://github.com/dbouzon/osnias-clearing/issues/54) (COORD v1.5)

## 1. Résumé

Socle **serveur agent A2A mutualisable** (réutilisable pour d'autres
fournisseurs sans duplication du rail) et **connecteur Mistral à deux
voies** : VOIE API (profil `mistral` stop-only, strictement additif au
modèle gemini de l'LLM-02, même boucle commune `baseHTTP.appel`) et
VOIE VIBE (exécution supervisée du binaire Vibe CLI en sous-processus :
liste blanche d'environnement, expurgation des secrets, codes 0/1/2/3,
`export.json` en échec fermé). Qualification **SIMULÉE, 0 €** : la
qualification réelle (VPS, binaires réels, réseau fournisseur, B1..B5)
demeure **NON_EXÉCUTÉE**, sur autorisation distincte du pilote.

## 2. Livrables (branche `mission/coord-llm-03`, tête `60baf922` + rapport)

Module `coord/a2a-agent` (Go 1.27, autonome, ne dépend pas du moteur en test) :

| Fichier | Nature | Contenu |
| --- | --- | --- |
| `socle/socle.go` | NOUVEAU | Enveloppe du rail `coorda2a` (loopback + jeton + carte RÉUTILISÉS, zéro duplication), `ErrJetonVide` fail-closed |
| `runner/runner.go` | NOUVEAU | Superviseur sous-processus : liste blanche env STRICTE (seul `PATH` transmis ; piège débusqué : `append(nil…)` hérite TOUT l'env), expurgation `sk-`/`AIza`/`gh?_`/`xoxb-`, codes 0/1/2/3, SIGTERM→SIGKILL du groupe, `export.json` fail-closed (`EXPORT_MANQUANT`/`CORROMPU`/`SECRET_DANS_EXPORT`) |
| `runner/group_posix.go`, `runner/group_windows.go` | NOUVEAU | Groupe de processus (`Setpgid` vs `taskkill /T /F`, limite documentée) |
| `runner/runner_test.go` | NOUVEAU | S11..S15, S19, S19bis, T-ENV, options fail-closed, annulation ctx |
| `mistral/mistral.go` | NOUVEAU | Agent A2A Mistral : routage `VOIE:API`/`VOIE:VIBE`, verdicts ancrés, registre de sessions atomique 0600, validation `BUDGET_DEPASSE`/`SESSION_LOGGING_DESACTIVE`/`DEPOT_AVEC_REMOTE`/`ENVELOPPE`, reprise `--resume` même session (S16) |
| `mistral/mistral_test.go` | NOUVEAU | S1, S2, S9, E9-jeton, S16, T-GIT, S17, S18, S12-via-A2A |
| `cmd/vibe-factice/main.go` | NOUVEAU | CLI simulée (codes, fuites, `-factice-ignore-sigterm` via `signal.Ignore` SANS `stop()`, `-factice-env-dump`) |
| `cmd/coord-agent/main.go` | NOUVEAU | `-demo-local` : cycle complet critique plan → exécution → correction, `DEMO_OK` |
| `README.md` | NOUVEAU | Frontières, usage, écart vs plan v2, B1..B5 NON_EXÉCUTÉS |

Côté moteur (`coord/go`, liste fermée respectée) :

| Fichier | Nature | Contenu |
| --- | --- | --- |
| `internal/connecteurs/mistral.go` | NOUVEAU | Profil applicatif `mistral` STOP-ONLY : seul `finish_reason:"stop"` accepté ; troncature (`length`/`model_length`), `tool_calls`, absent/inconnu = REFUS avec motif exact, jamais réparé |
| `internal/connecteurs/mistral_test.go` | NOUVEAU | C1..C9 sur serveurs factices loopback |
| `internal/connecteurs/connecteurs.go` | MODIFIÉ (additif) | Validation du profil `mistral` au chargement + dispatch `executerContexteMistral` |

Documentation/outillage : `coord/connecteur/Mistral/exemple-config-mistral.json`
(exemple SANS secret, champs plats du moteur), section Mistral dans
`coord/pilot/connecteurs/README.md`, CI `.github/workflows/coord-agent-ci.yml`
(matrix ubuntu/windows/macos + race, chemins `coord/a2a-agent/**`).

## 3. Écart documenté vs plan v2 (fait de code déterminant)

L'adaptateur `coord/a2a` capture l'artefact **à l'envoi** (`message/send`)
et **refuse tout achèvement asynchrone** (verdict `RESULTAT_NON_TRANSMIS`) ;
la borne 20 s est une limite de suivi post-envoi. Le serveur exécute donc le
travail **dans l'appel `message/send`** : le client reste bloqué pendant
l'exécution. Le plan v2 prévoyait « réponse immédiate + suivi asynchrone » ;
l'asynchrone aurait exigé une **extension de contrat A2A** à faire valider
par les deux IA. Implémenté en synchrone-dans-send, sans extension de
contrat. **Contrôle intermédiaire T3 OpenAI : écart ACCEPTABLE** (fidélité
au contrat `a2a-go` existant, déterminisme, traçabilité).

## 4. Sécurité

1. Aucun secret dans le repo : clés par variables d'environnement
   (`COORD_CLE_MISTRAL`), exemples sans secret.
2. Socket loopback uniquement, jeton obligatoire (`ErrJetonVide`).
3. Environnement du sous-processus en liste blanche STRICTE : seules les
   entrées explicites sont transmises ; l'héritage complet est impossible.
4. Expurgation des motifs de secrets avant toute écriture durable ;
   secret dans `export.json` ⇒ verdict d'échec fermé.
5. Codes de sortie opposables 0/1/2/3 en toutes circonstances
   (y compris `signal.Ignore` : le `stop()` du contexte n'est jamais requis).
6. Registre de sessions atomique (écriture+renommage), permissions 0600.

## 5. Recette (SIMULÉE, 0 €, aucune clé, aucun réseau fournisseur)

- `coord/a2a-agent` : `go vet ./...` propre, `go test ./...` **VERT** au
  complet (runner, mistral, connecteurs) ; `go build` OK.
- `coord/go` : `go vet` + tests complets OK (S7 aucun secret, connecteurs).
- `coord/a2a` : inchangé, OK.
- Vecteurs de conformité v1.5 : **22/22** (sha `8c097f76…156` inchangé).
- Batterie Python v1.5 durcie : **47/47 PASS** (manifeste v1.0..v1.4 vérifié).
- Démo `-demo-local` : **DEMO_OK** (voie inférence + voie Vibe).

## 6. Mesures (sandbox Linux Go 1.27.4, SIMULÉ)

| Mesure | Valeur |
| --- | --- |
| Binaire `coord-agent` | 12 370 335 octets (~11,8 Mo) |
| Binaire `vibe-factice` | 4 413 994 octets (~4,2 Mo) |
| Suite de tests du module (`go test ./...`, build factice inclus) | ~3,8 s |
| Démo `-demo-local` (cycle complet) | ~0,53 s |

Suivi des performances de l'approche synchrone (latence/charge serveur)
consigné comme point de vigilance T3 OpenAI ; pertinence opérationnelle à
réévaluer lors de la qualification réelle.

## 7. Qualification réelle : NON_EXÉCUTÉE

Conformément à COORD v1.5 et à l'ordre de mission #54, aucune qualification
sur infrastructure réelle (VPS, binaires réels network, B1..B5) n'est
exécutée. Elle fera l'objet d'une autorisation distincte du pilote.
Fusion et clôture réservées au pilote.

## 8. Réceptions

- Critique préalable OpenAI du plan : v1 → `A_CORRIGER` (3 bloquants) ;
  v2 → **PLAN_ACCEPTE** (consigné #54).
- Réception intermédiaire T3 (session pont `m-coord-llm-03`) : écart
  **acceptable**, socle **conforme**, aucun `A_CORRIGER` (consignée :
  commentaire FROM:MISTRAL 7177918226 dans #54).
- Réception indépendante finale OpenAI : ancrée au SHA de tête, consignée
  dans la PR et #54 (voir historique de réception ci-dessous).

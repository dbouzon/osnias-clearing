# COORD rail A2A — module autonome (M-COORD-RAIL-01)

Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0

## Objet

Module Go **autonome** (`go.mod` propre) contenant le rail A2A DIRECT de
COORD : délégation, suivi, annulation, reprise et idempotence vers des agents
A2A v1.0. Issu de la mission M-COORD-LLM-02 (#48), consolidé en
infrastructure commune par M-COORD-RAIL-01 (#50, PLAN_ACCEPTE). Il ne dépend
de RIEN de `coordengine` et n'expose AUCUN type du SDK `a2a-go` : la
dépendance SDK (`github.com/a2aproject/a2a-go/v2` v2.9.0) est un détail
d'implémentation confiné à ce module.

## Frontière publique

- `NouveauRail(ctx, baseURL, jeton, cheminRegistre) (*Rail, error)` — agent
  A2A loopback UNIQUEMENT dans cette tranche (fail closed), jeton local
  DISTINCT du token console pilote, registre durable atomique 0600 requis.
- `(*Rail) Deleguer(ctx, action, charge, cle)` — UNE délégation par clé
  d'idempotence (`ErrDoublon` au second appel, SANS réseau).
- `(*Rail) Suivre`, `Annuler`, `ResultatDurable`, `ResultatParCle`,
  `NonResolues` — suivi borné ; les tâches incertaines restent à la
  résolution du PILOTE exclusivement (`ResoudrePilote`, SEC-01) : aucun
  retry automatique, jamais de second débit.
- États fermés : `EtatSoumise/EnCours/SaisieAttendue/Achevee/Annulee/Echouee`
  (`EtatDepuisA2A` : tout état inconnu est refusé).
- Agents factices de qualification : `DemarrerAgentFactice`,
  `ExecEchoFactice` (neutre), `ExecGeminiFactice`/`PartageGemini` (Gemini
  simulé, traduction A2A → « API fournisseur »), `CompteurEnvois`
  (observatoire d'envois), alias `Executable` (contrat d'exécuteur
  ré-exporté : les consommateurs n'importent pas le SDK).

## Invariants

1. Aucun second orchestrateur : le rail ne crée AUCUNE mission, ne délivre
   AUCUN verdict, ne modifie AUCUN état mission. COORD reste l'autorité
   (adaptateurs.Adaptateur côté moteur, voir `coordengine/internal/a2a`,
   pont mince).
2. Identifiant de tâche A2A DURABLE exigé : un message/send sans tâche
   suivie est REFUSÉ (`TACHE_NON_DURABLE`).
3. Résultat capturé À L'ENVOI (tasks/get ne garantit pas les artefacts),
   persisté avant retour.
4. Contrat d'artefact strict `{jetons, texte}` : jetons non déclarés ou
   réponse vide = REFUS (`JETONS_INCONNUS`, `REPONSE_VIDE`).
5. Registre durable atomique (tmp + fsync + rename, 0600) : reprise après
   arrêt réel du processus ; le jeton n'y figure JAMAIS.
6. Boucle locale uniquement : `APPEL_REEL_NON_AUTORISE` pour tout agent non
   loopback (banc d'essai factice, 0 € ; HTTPS distant documenté, non
   activé). Aucune qualification réelle de fournisseur n'est présupposée.

## Isolation : ce que ce module garantit (et ne garantit pas)

Le `go.mod` du moteur (`coord/go`) ne porte plus `a2a-go` qu'en dépendance
INDIRECTE (via `replace github.com/dbouzon/osnias-clearing/coord/a2a =>
../a2a`). L'isolation est de PÉRIMÈTRE source/CI (churn du SDK, frontière
`internal`, CI et go.sum propres, réutilisabilité externe), PAS du binaire :
un binaire qui importe ce module lie toujours ses dépendances transitives
(option C « service séparé » écartée : cible VPS 2 Go, un seul processus).

## Qualification

- Recette du module SANS moteur : `rail_test.go` (M1..M6 : délégation,
  annulation, doublon sans réseau, mapping fermé, sécurité jeton/loopback
  0600, reprise + résolution pilote). 0 €, aucune clé, agents loopback.
- Recette d'intégration complète (E1..E10 + démo E2E cycle COORD) :
  `coordengine/internal/a2a` (pont). Aucune réécriture des connecteurs.
- CI : `.github/workflows/coord-a2a-ci.yml` (Go 1.27, ubuntu/windows/macos,
  race `-race -count=2`).

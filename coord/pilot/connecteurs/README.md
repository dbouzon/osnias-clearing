# Connecteurs IA réels — COORD Pilot (M-COORD-PILOT-03)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Mission [#29](https://github.com/dbouzon/osnias-clearing/issues/29) : connecteurs
réels Mistral/OpenAI et première mission inter-IA, **séparés du moteur COORD**
(`coordengine` inchangé), avec orchestration minime
plan/critique → exécution → soumission → réception → correction → ACK.
Les adaptateurs simulés du MVP (#27) sont **préservés** (démo gratuite et tests
de conformité).

## Architecture

```
coord/go/internal/connecteurs/          (NOUVEAU, #29)
  connecteurs.go    Config locale, HTTPEndPoint (adaptateurs.Adaptateur),
                    triple verrou d'appel réel, retry 429/5xx, limites,
                    expurgation des secrets, MODELE_DIVERGENT
  orchestre.go      Orchestrateur : RÉSERVER (TX1) -> APPELER (hors
                    verrou) -> COMMITTER (TX2) ; anti-doublons persistant
                    (appel_en_cours.json) ; verdicts structurés ancrés ;
                    REPONSE_TARDIVE ; aucune clôture automatique
coord/go/cmd/coord-mission/            conducteur (démo locale 0 €; réel
                                        verrouillé, sinon NON_EXECUTE)
```

- **Aucun privilège fournisseur** : tout endpoint déclaré compatible
  `{model, messages}` (Mistral `/v1/chat/completions`, OpenAI
  `/v1/chat/completions`) se comporte à l'identique. `mistral` et `openai`
  ne sont que des identifiants opaques.
- **API directes retenues** pour la v1 ; A2A écarté (analyse #27) mais un
  module futur peut implémenter `adaptateurs.Adaptateur` sans dépendance
  normative ; MCP hors périmètre.
- Le moteur normatif (`coordengine` : spec/engine/policy/missions) est
  **inchangé** ; la seule exception documentée est `internal/budget/budget.go`
  (méthodes additives : `AjusterJetons` à l'étape 4, puis
  `ReserverCentimes`/`AjusterCentimes`/`CentimesReservesTotal` pour la voûte
  monétaire, réserve 7154948522 — aucune sémantique existente modifiée).
  Tous les changements d'état passent par la machine à états fermée
  (N-10..N-17).

## Sécurité (mission #29, étape 4)

1. **Secrets** : uniquement des variables d'environnement (noms déclarés par
   `cle_env` dans la configuration locale). Jamais dans un dépôt, un journal,
   l'UI ou une erreur ; toute sortie contenant la clé est rejetée
   (`SORTIE_COMPROMISE`) ou expurgée (`[EXPURGE]`).
2. **Triple verrou d'appel réel** (tout endpoint non-loopback) :
   mode `REEL` dans la configuration **ET** `COORD_MODE_REEL=1` **ET** clé
   présente **ET** autorisation pilote explicite (fournisseurs, plafonds
   monétaire/d'appels/de jetons, date). Aucune porte ne sevre seule.
   Les endpoints loopback servent UNIQUEMENT aux serveurs factices des
   tests (hors réseau, 0 €).
3. **Identité ≠ déclaration** : le modèle déclaré est vérifié a posteriori
   contre le champ `model` de la réponse (`MODELE_DIVERGENT` sinon).
4. **Budget R3 avant appel** : `VerifierAvantAppel` puis `Consommer`
   DANS la transaction TX1 (réservation durable, fail-closed : une tentative
   interrompue reste payée, l'incertitude n'est jamais reimboursée) ; le
   surplus réel est ajouté au commit (TX2, journal `APPEL_SURPLUS`).
   **Et voûte monétaire** : voir section suivante.
5. **Appel hors verrou** : jamais d'appel réseau sous le verrou de mission ;
   l'arrêt du pilote reste instantané. Une réponse arrivée après un arrêt
   est journalisée `REPONSE_TARDIVE` et **aucune transition** n'est appliquée.
6. **Anti-doublons** : chaque aller-retour inscrit `appel_en_cours.json`
   (TX1) et ne le clôt qu'au commit. Un crash laisse un appel
   `EN_COURS` (ou `ORPHELIN`) : toute reprise est refusée
   (`APPEL_EN_COURS_NON_RESOLU`) jusqu'à la décision EXPLICITE du pilote
   (`ResoudreAppelEnCours`). Chaque `Idempotency-Key` (sha256) est envoyé
   au fournisseur.
7. **Verdicts** : uniquement structurés (JSON strict), validés contre
   l'énumération `ACCEPTEE|A_CORRIGER|REFUSEE` (réception) ou
   `PLAN_ACCEPTE|A_CORRIGER|REFUSEE` (plan), **ancrés** à l'empreinte
   sha256 de la soumission/du plan (`empreinte_attendue`). Un verdict non
   conforme est rejeté, jamais interprété. Aucun texte de modèle ne vaut
   autorisation ; la clôture n'existe pas dans l'orchestrateur
   (`CLOTURE_REFUSEE` par construction).
8. **Habilitations** : rôles résolus côté orchestrateur depuis les
   identités qualifiées de la mission ; validation normative sur COPIE
   avant tout appel (C2 conservé) ; budget refusé avant l'appel si plafond
   atteint (aucun adaptateur sans plafond ne peut jamais être appelé).

## Voûte monétaire (correction de la réserve 7154948522 : plafond rendu opposable)

Contre-réception du 25/11/2026 : `plafond_centimes` était validé dans la
configuration mais jamais transmis au moteur budgétaire, et le surplus était
comptabilisé après l'appel, la dépense étant déjà engagée. Correction :

1. **Plafond transmis et armé** : `Autorisation.PlafondCentimes` est passé au
   budget de la mission (`budget.Plafonds.Centimes`, centimes d'euro ENTIERS,
   plafond GLOBAL de la mission).
2. **Réservation durable AVANT chaque tentative facturable** (TX1) :
   enveloppe conservatrice = `(1 + retraits_max) × [coût(borne entrée) +
   coût(borne sortie) + prix par requête]`, tarifs et bornes DÉCLARÉS par
   fournisseur (`tarif_entree_centimes_par_million`,
   `tarif_sortie_centimes_par_million`, `tarif_requete_centimes`,
   `jetons_entree_max_par_tentative`, `jetons_sortie_max_par_tentative`),
   arrondis TOUJOURS vers le haut en arithmétique entière. Chaque retrait
   429/5xx pouvant être intégralement facturé, son coût potentiel est
   réservé dès la première tentative.
3. **Refus avant réseau** : plafond dépassé, plafond non armé (<= 0) ou
   tarif/borne inconnu (<= 0, `TARIF_INCONNU`) => refus AVANT tout octet
   émis. La requête elle-même est refusée (`CHARGE_HORS_BORNE`) si sa charge
   dépasse la borne d'entrée d'après l'heuristique conservatrice 4 octets
   UTF-8 par jeton.
4. **Conservateur par construction** : la réservation n'est JAMAIS libérée
   (échec, réponse tardive, verdict non conforme, crash : l'incertitude est
   payée) ; le coût réel constaté ne peut qu'AJOUTER un éventuel surcoût.
5. **Plafond local estimé ≠ garantie fournisseur** : l'enveloppe est une
   estimation LOCALE fondée sur les tarifs et bornes déclarés par le pilote.
   Le logiciel garantit seulement qu'il refuse d'émettre au-delà ; il ne
   garantit pas ce que le fournisseur facture. Les tarifs de l'exemple sont
   indicatifs : le pilote les fixe selon la grille de son compte.

## Guide local (SANS AUCUN SECRET DANS UN FICHIER)

```bash
cd coord/go

# 1. Démo locale autonome (serveurs factices loopback, aucune clé, 0 €) :
go run ./cmd/coord-mission -demo-local -data /tmp/coord-demo

# 2. Tests hors réseau (serveurs factices, race detector via CI) :
go test ./internal/connecteurs/

# 3. Mission réelle (uniquement après autorisation du pilote) :
export COORD_CLE_MISTRAL="…votre clé Mistral…"
export COORD_CLE_OPENAI="…votre clé OpenAI…"
export COORD_MODE_REEL=1
go run ./cmd/coord-mission \
  -config ./../pilot/connecteurs/ma-config-reelle.json \
  -data /tmp/coord-mission \
  -issue M-COORD-XX-01 \
  -plan plan.md -mandat mandat.md -livrable livrable.txt
```

- La configuration `ma-config-reelle.json` suit le modèle
  [`exemple-config-reel.json`](exemple-config-reel.json) (structure, aucuns
  secrets : seulement les NOMS des variables d'environnement).
- Sans autorisation complète (fournisseurs, plafonds, date, clés), le
  conducteur répond **NON_EXECUTE** + procédure (exit 3), il ne simule
  JAMAIS un succès réel. Une configuration `SIMULE` ne peut pas conduire une
  mission présentée comme réelle.
- La mission réelle bénigne de qualification (petit livrable textuel,
  critères objectifs) se prépare avec `plan.md`, `mandat.md`,
  `livrable.txt` ; son exécution exige l'autorisation budgétaire écrite du
  pilote.

## Ce qui reste réservé au pilote

- Fusion de la PR, clôture de mission (`ACK_RECU` → `CLOTUREE`),
- premiere mission réelle facturée (fournisseurs, données, plafond monétaire),
- activation éventuelle de v1.5.

## Tableau des capacités (extrait ; le rapport final fait foi)

| Capacité | Statut |
| --- | --- |
| Connecteurs réels Mistral/OpenAI (API directes) | REEL (testés via serveurs factices ; appel facturé NON_EXECUTE) |
| Boucle plan/critique/réception structurée ancrée | REEL (factices) |
| Triple verrou + autorisation pilote | REEL |
| Voûte monétaire : plafond en centimes opposé AVANT appel, tarifs/bornes déclarés, réservation jamais libérée | REEL (serveurs factices) |
| Anti-doublons crash/interruption | REEL |
| Race detector (connecteurs) | CI multi-OS (`go test -race`) |
| Appels facturés réels | NON_EXECUTE (autorisation pilote requise) |

## Connecteur Anthropic Claude (M-COORD-LLM-01, issue #40)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Troisième fournisseur LLM réel, pour les rôles **critique de plan** et
**réceptionnaire** (tranche 1 ; Claude exécutant = mission ultérieure).
Plan v2 PLAN_ACCEPTE ([commentaire 7169582206](https://github.com/dbouzon/osnias-clearing/issues/40#issuecomment-7169582206)),
ordre de reprise V1.1 sous COORD v1.5
([commentaire 7173095544](https://github.com/dbouzon/osnias-clearing/issues/40#issuecomment-7173095544)).

### Architecture (décisions D1..D4 du plan v2)

```
coord/go/internal/connecteurs/
  commun.go         NOUVEAU (#40) : baseHTTP, briques de sécurité COMMUNES
                    (boucle d'appel contrôlée, bornage à l'émission, retraits
                    0..5 bornés, plafond de réponse, expurgation, voûte
                    monétaire) + interface ConnecteurReel (orchestrateur)
  anthropic.go      NOUVEAU (#40) : AnthropicPoint, format Messages API
  connecteurs.go    HTTPEndPoint inchangé de comportement ; hérite de
                    baseHTTP ; champ additif "format" (D4)
  orchestre.go      assertions *HTTPEndPoint -> interface ConnecteurReel
                    (aucune sémantique modifiée : voûte, TX1/TX2, verdicts)
coord/go/cmd/coord-mission/  dispatch par "format" (branchement unique)
```

- **D1** : aucune duplication de la boucle de sécurité ; extraction
  mécanique prouvée par le rejeu intégral (63 tests connecteurs verts,
  batteries Python 135/135, vecteurs 22/22 byte-identiques `8c097f76…`).
- **D2** : client HTTP stdlib, SDK `anthropic-sdk-go` ÉCARTÉ (ses retries
  seraient une seconde autorité de reprise contournant la voûte monétaire ;
  périmètre minimal pour POST /v1/messages ; `go.mod` INCHANGÉ, zéro
  dépendance ajoutée). `anthropic-version` ÉPINGLÉ (`2023-06-01`) :
  toute montée de version est un commit explicite du pilote.
- **D3** (fail closed) : en-têtes `x-api-key` + `anthropic-version`
  (jamais `Bearer`) ; gabarit COORD en `system` top-level (anti-injection
  identique mot pour mot) ; `max_tokens` obligatoire = borne
  `jetons_sortie_max_par_tentative` ; sortie = SEULS blocs `content[]` de
  type `text` (tout autre bloc = refus intégral) ; usage =
  `input_tokens + output_tokens` (absent/nul = `JETONS_INCONNUS`) ;
  `stop_reason=max_tokens` = troncature refusée intégralement ; stop_reason
  inconnu = refus ; champs de prompt caching non nuls = `JETONS_INCONNUS`
  (COORD n'émet jamais `cache_control`) ; `MODELE_DIVERGENT` inchangé ;
  anti-doublon côté orchestrateur (aucun en-tête d'idempotence supposé
  côté Anthropic, la clé SHA-256 reste calculée et journalisée).
- **D4** : `"format": "openai"|"anthropic"` champ additif et
  rétrocompatible dans la déclaration du fournisseur (absent = `openai`,
  comportement Mistral/OpenAI inchangé ; valeur inconnue = refus).

### Configuration

Voir `exemple-anthropic.json` (SANS secret : la clé passe par
`COORD_CLE_ANTHROPIC`). Mêmes tarifs/bornes/plafonds déclarés que les
autres fournisseurs (voûte monétaire opposable avant chaque appel).
REEL exige le triple verrou habituel + le bloc `autorisation_pilote`
(qui EST l'autorisation budgétaire écrite).

### Qualification

Recette C1..C15 sur serveurs factices loopback UNIQUEMENT (SIMULÉ, 0 €) :
`anthropic_test.go`. Statut : qualification logicielle et simulée RÉELLE
(exécutée) ; **qualification réelle auprès de l'API Anthropic :
NON_EXÉCUTÉE** (aucun appel, aucune clé réelle) — procédure préparée dans
`recette-anthropic-reelle.md`, exécution sur autorisation écrite du pilote
uniquement.

## Mistral, profil stop-only (M-COORD-LLM-03, issue #54)

Champ additif `"profil": "mistral"` : profil applicatif de validation
renforcée STRICTEMENT ADDITIF au modèle gemini (même champ, même analyse
stop-only). Mistral est déclaré au FORMAT `openai` (endpoint natif
`api.mistral.ai/v1/chat/completions`, boucle commune partagée via
`baseHTTP.appel`, zéro règle de sécurité dupliquée).

- **Stop-only** : seul `finish_reason: "stop"` est accepté ;
  `length`/`model_length` (troncature), `tool_calls`, absent ou inconnu =
  réponse REFUSÉE avec motif exact journalisé, jamais réparée.
- **Identité exacte** : champ `model` contrôlé par la boucle commune
  (`MODELE_DIVERGENT` si divergence avec le modèle déclaré) ; usage
  `prompt_tokens + completion_tokens` exigé (`JETONS_INCONNUS` sinon).
- Aucun tool/function calling dans cette tranche ; tarifs GRILLE ÉVOLUTIVE
  en configuration, jamais codés en dur.

Configuration : voir `coord/connecteur/Mistral/exemple-config-mistral.json`
(SANS secret : la clé passe par `COORD_CLE_MISTRAL`). Recette C1..C9 sur
serveurs factices loopback
UNIQUEMENT (SIMULÉ, 0 €) : `mistral_test.go`. Statut : **qualification
réelle auprès de l'API Mistral : NON_EXÉCUTÉE** (aucun appel, aucune clé
réelle), sur autorisation écrite du pilote uniquement.

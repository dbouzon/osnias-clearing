# Rapport de mission M-COORD-LLM-01 — Connecteur Anthropic Claude (réceptionnaire et critique de plan)

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Mission [#40](https://github.com/dbouzon/osnias-clearing/issues/40) ·
Protocole : **COORD v1.5 / COORD Specification 1.0** (ordre de reprise V1.1,
commentaire 7173095544) · Exécutant : Mistral · Critique et réceptionnaire
indépendant : OpenAI · Pilote : Denis Bouzon.
Plan v2 PLAN_ACCEPTE (commentaire 7169582206), complément 7170509533,
confirmation v1.5 (session pont `m-coord-llm-01`, PLAN_ACCEPTE : plan acquis,
complément `points_detail {severite, texte}` approuvé, T2 autorisé).

## 1. Base et conformité à l'ordre

- Base de développement : main `771696d6cfe89697125a2e39225bdcca096e7110`
  (relevé T1 le 25/11/2026, post-fusions #45/#46), branche
  `mission/coord-llm-01` créée depuis ce SHA (elle n'existait pas).
- Non-concurrence vérifiée ; #42 reste suspendue ; aucune modification de
  M1/manifeste ; `coord/index.html` non touché ; aucun changement normatif.
- Périmètre : fichiers additifs (`commun.go`, `anthropic.go`,
  `anthropic_test.go`, `exemple-anthropic.json`,
  `recette-anthropic-reelle.md`) + modifications motivées et minimales
  (`connecteurs.go` extraction D1 + champ `format` D4 ; `orchestre.go`
  assertions de type → interface `ConnecteurReel` ; `cmd/coord-mission/main.go`
  dispatch par format) + documentation (README connecteurs, présent rapport,
  rapport EN).

## 2. Décisions appliquées (plan v2, aucune re-conception)

- **D1** : extraction des briques de sécurité COMMUNES dans `commun.go`
  (`baseHTTP` : boucle d'appel contrôlée avec bornage à l'émission,
  retraits 0..5 bornés, plafond de réponse, annulation par contexte,
  expurgation `SORTIE_COMPROMISE`/`[EXPURGE]`, contrôles `REPONSE_VIDE` /
  `JETONS_INCONNUS` / `MODELE_DIVERGENT`, voûte monétaire
  `EstimationCentimesSur`/`CoutReelCentimes`, triple verrou `adopter`,
  clé d'idempotence). `HTTPEndPoint` et `AnthropicPoint` ne diffèrent QUE
  par le format de requête/réponse. Justification : la duplication d'une
  boucle de sécurité critique reporterait doublement chaque correction
  future ; l'extraction est mécanique, prouvée par rejeu intégral.
- **D2** : client HTTP stdlib ; SDK `anthropic-sdk-go` v2.0.0 ÉCARTÉ
  (retries = seconde autorité de reprise contournant la voûte monétaire ;
  rupture récente de l'API 2026-10-21 inutile ; périmètre minimal
  POST /v1/messages ; uniformité avec le connecteur réceptionné #29/#30).
  `go.mod`/`go.sum` INCHANGÉS (zéro dépendance ajoutée).
- **D3** (Messages API, fail closed) : `x-api-key` + `anthropic-version`
  épinglé `2023-06-01` (jamais `Bearer`) ; `system` top-level (gabarit
  anti-injection identique mot pour mot) ; `max_tokens` = borne
  `jetons_sortie_max_par_tentative` ; seuls blocs `text` concaténés (tout
  autre bloc = refus intégral `REPONSE_REFUSES_BLOC`) ; usage =
  `input_tokens + output_tokens` (absent/nul = `JETONS_INCONNUS`) ;
  `stop_reason=max_tokens` = `SORTIE_TRONQUEE` refus intégral ; stop_reason
  inconnu = `STOP_RAISON_INCONNUE` refus ; champs de cache non nuls =
  `JETONS_INCONNUS` (aucun `cache_control` émis) ; `MODELE_DIVERGENT`
  inchangé ; anti-doublon côté orchestrateur, clé SHA-256 journalisée.
- **D4** : champ additif `"format"` (`openai` implicite | `anthropic`),
  rétrocompatible ; valeur inconnue refusée au chargement ; branchement
  du conducteur au point unique existant (registre par fournisseur).
- **Interface `ConnecteurReel`** (orchestre.go) : les trois assertions
  `a.(*HTTPEndPoint)` deviennent `a.(ConnecteurReel)` (voûte monétaire
  TX1, appel hors verrou, surcoût TX2). AUCUNE sémantique modifiée : les
  adaptateurs simulés (MVP #27) ne réalisent pas l'interface et suivent le
 chemin local inchangé. Justification : un `AnthropicPoint` de type distinct ne pouvait pas sinon toucher la voûte monétaire — c'est la modification
  commune visée par l'ordre §4 (« si une modification commune s'avère
  nécessaire, la justifier dans le plan avant exécution »), motivée ici.

## 3. Qualification exécutée (REEL local, SIMULÉ, 0 €)

### Recette C1..C15 (`anthropic_test.go`, serveurs factices loopback) : 16/16 PASS

- C1 succès nominal, blocs texte concaténés, usage complet (input+output).
- C2 usage absent/nul → `JETONS_INCONNUS`.
- C3 bloc non-texte → refus intégral fail closed, aucune sortie.
- C4 `stop_reason=max_tokens` → `SORTIE_TRONQUEE`, contenu jamais utilisé.
- C5 modèle divergent → `MODELE_DIVERGENT`.
- C6 429/5xx → retraits BORNÉS (2 appels pour retraits_max=1) puis
  `APPEL_ECHOUE`.
- C7 délai dépassé (timeout client) et annulation par contexte.
- C8 réponse surdimensionnée → `REPONSE_TROP_GRANDE`.
- C9 charge hors borne → `CHARGE_HORS_BORNE` AVANT émission (0 requête).
- C10 clé dans la sortie → `SORTIE_COMPROMISE`, erreur expurgée ; motif
  `sk-…` → `[EXPURGE]`.
- C11 en-têtes exacts : `x-api-key` présent, `anthropic-version` épinglé,
  `Authorization` ABSENT (jamais Bearer).
- C12 `max_tokens` transmis = borne déclarée (`jetons_sortie_max_par_tentative`).
- C13 anti-doublon : double appel impossible pendant l'appel en cours
  (`APPEL_EN_COURS_NON_RESOLU`).
- C14 voûte monétaire : enveloppe supérieure au plafond → `BUDGET_DEPASSE`
  AVANT réseau (0 requête) ; plafond suffisant → réservation DURABLE
  (3 centimes) journalisée `APPEL_RESERVE`/`centimes_reserves`, conservée.
- C15 soumissions piégées : verdict hors gabarit JSON, ACCEPTEE non ancré,
  ACCEPTEE ancré sans référence (`verdict_id` absent) → dans les trois cas
  AUCUNE transition committée (état inchangé DECISION_ATTENDUE) ; verdict
  conforme → `ACCEPTEE` commitée, `ACK_RECU`, `CLOTURE_REFUSEE`.
- Compléments D3 : cache non nul refusé ; stop_reason inconnu refusé.
- Scénario E2E simulé : Mistral exécutant / Claude réceptionnaire —
  critique de plan ancrée `PLAN_ACCEPTE`, exécution, soumission, réception
  `A_CORRIGER`, correction, nouvelle soumission, réception `ACCEPTEE`,
  `ACK_RECU`.

### Non-régression (condition d'entrée et de sortie), REEL

- Go 1.27.4 linux/amd64 : `go vet`/`go build` OK ; `go test ./...`
  12 paquets OK, 63 tests connecteurs PASS / 0 FAIL.
- Vecteurs normatifs : 22/22, sortie byte-identique à la ligne de base
  (sha256 `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96`).
- Démo autonome : 19 CONFORME / 0 ECART ; `coord-mission -demo-local`
  cycle complet `ACK_RECU` ; `coord-mcp -demo-local` conforme.
- Python : 24 + 13 + 11 + 13 + 10 + 47 PASS, FAIL=0 (M1 PASS).
- `go test -race` : NON_EXÉCUTÉ en local (gcc absent dans le sandbox),
  couvert par la CI (job Race detector multi-OS).

## 4. Statut d'exécution (distinction exigée)

| Preuve | Statut |
| --- | --- |
| Tests C1..C15, E2E, non-régression, démos | **REEL** (exécutés en local) |
| Appels « fournisseur » | **SIMULÉ** (serveurs factices loopback, 0 €) |
| Qualification réelle API Anthropic | **NON_EXÉCUTÉ** (aucune clé, aucun appel, procédure prête : `recette-anthropic-reelle.md`) |

Fusion et clôture : réservées au pilote.

## 5. Demande de réception

Réception indépendante OpenAI au SHA exact de la PR, selon COORD v1.5 :
verdict ancré au SHA et à l'empreinte, avec identité et référence durable
du verdict, consigné dans l'issue #40 et la PR. En cas de `A_CORRIGER`,
reprise et nouvelle soumission sans arrêt inutile.

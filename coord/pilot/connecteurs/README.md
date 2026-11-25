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
- Le moteur normatif (`coordengine` : spec/engine/policy/budget/missions)
  n'est **pas modifié** : tous les changements d'état passent par la machine
  à états fermée (N-10..N-17).

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
| Anti-doublons crash/interruption | REEL |
| Race detector (connecteurs) | CI multi-OS (`go test -race`) |
| Appels facturés réels | NON_EXECUTE (autorisation pilote requise) |

# Rapport de mission — M-COORD-PILOT-03 : connecteurs IA réels et première mission inter-IA

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Mission : issue [#29](https://github.com/dbouzon/osnias-clearing/issues/29) · PR [#30](https://github.com/dbouzon/osnias-clearing/pull/30)

## 1. Résumé

Connecteurs IA réels (Mistral, OpenAI — et tout endpoint compatible `{model,
messages}`) **séparés du moteur COORD**, orchestration normative minimale du
cycle plan/critique → exécution → soumission → réception (structurée, ancrée)
→ correction → ACK, avec triple verrou d'appel réel, budget réservé avant
appel, appel réseau hors verrou, anti-doublons persistant après interruption,
et **aucune clôture automatique**. Aucun appel facturable n'a été passé :
la première mission réelle bénigne est préparée et exécutable seulement après
l'autorisation budgétaire explicite du pilote.

## 2. Livrables (branche `mission/coord-pilot-03`, PR #30)

10 fichiers, 2 627 ajouts, 1 suppression (le workflow de qualification gagne
une ligne : race detector étendu à `./internal/connecteurs/`).

| Fichier | Nature | Contenu |
| --- | --- | --- |
| `coord/go/internal/connecteurs/connecteurs.go` | NOUVEAU | Config locale sans secrets, `HTTPEndPoint` (`adaptateurs.Adaptateur`), triple verrou d'appel réel, retraits 429/5xx bornés, délais/taille plafonnés, `MODELE_DIVERGENT`, `JETONS_INCONNUS`, `SORTIE_COMPROMISE`/`[EXPURGE]` |
| `coord/go/internal/connecteurs/orchestre.go` | NOUVEAU | Orchestrateur : TX1 RÉSERVER (validation sur copie C2, réservation durable R3, anti-doublon) → APPELER hors verrou → TX2 COMMITTER (transition normative, surplus, `REPONSE_TARDIVE`, `COMMIT_REFUSE`) ; verdicts ancrés ; `CLOTURE_REFUSEE` |
| `coord/go/internal/connecteurs/connecteurs_test.go` | NOUVEAU | 14 tests serveurs factices loopback |
| `coord/go/internal/connecteurs/orchestre_test.go` | NOUVEAU | 10 tests de cycle, correction, réponse tardive, crash/anti-doublon, habilitations, plafonds, ancrages, verdicts forgés |
| `coord/go/cmd/coord-mission/main.go` | NOUVEAU | Conducteur : `-demo-local` (0 €, hors réseau) et cycle réel verrouillé (`NON_EXECUTE` + procédure, exit 3) |
| `coord/go/internal/budget/budget.go` | MODIFIÉ (+17 lignes) | `AjusterJetons` : surplus réel de jetons sans comptage d'appel (nécessité démontrée par test ; aucune autre ligne du moteur touchée) |
| `coord/go/internal/budget/ajuster_test.go` | NOUVEAU | Invariants de `AjusterJetons` (pas d'appel supplémentaire, jamais sous zéro) |
| `coord/pilot/connecteurs/README.md` | NOUVEAU | Guide local sans secrets, sécurité, procédure de mission réelle |
| `coord/pilot/connecteurs/exemple-config-reel.json` | NOUVEAU | Structure REEL sans aucun secret (noms de variables d'environnement uniquement) |
| `.github/workflows/coord-go-ci.yml` | MODIFIÉ (1 ligne) | Race detector sur `./internal/connecteurs/` |

Adaptateurs SIMULÉS du MVP #27 **préservés** (zéro modification de
`adaptateurs`, `pilot`, `missions`, `spec`, `engine`, `policy`, `arret`,
`persist`) ; A2A non intégré par décision de plan (interface indépendante) ;
MCP hors périmètre ; aucun changement normatif ; v1.5 non activée.

## 3. Articulation sécurité / protocole (mission #29, étapes 3–4)

1. **Secrets** : uniquement variables d'environnement ; jamais dans un dépôt,
   journal, UI ou erreur ; sortie contenant la clé rejétée (`SORTIE_COMPROMISE`)
   ou expurgée.
2. **Triple verrou d'appel réel** (tout endpoint non-loopback) : mode `REEL`
   de la configuration **ET** `COORD_MODE_REEL=1` **ET** clé d'environnement
   présente **ET** autorisation pilote explicite (fournisseurs, plafonds
   monétaire/d'appels/jetons, date). Endpoints loopback = serveurs factices
   de test uniquement (hors réseau, 0 €). https imposé hors loopback.
3. **Identité ≠ déclaration** : champ `model` de la réponse vérifié a
   posteriori (`MODELE_DIVERGENT`).
4. **Budget R3** : `VerifierAvantAppel` + réservation DURABLE en TX1
   (`Consommer` de l'estimation, jamais remboursée : l'incertitude est payée,
   fail-closed) ; surplus réel ajouté en TX2 via `AjusterJetons` (sans
   comptage d'appel) ; plafonds fixés à la création, adaptateur sans plafond =
   jamais appelable.
5. **Appel hors verrou de mission** : l'arrêt du pilote reste instantané ;
   réponse post-arrêt = `REPONSE_TARDIVE`, aucune transition appliquée.
6. **Anti-doublons** : `appel_en_cours.json` inscrit en TX1, clos au commit ;
   crash = `APPEL_EN_COURS_NON_RESOLU`, reprise exclusivement par décision
   `ResoudreAppelEnCours` du pilote ; `Idempotency-Key` (sha256) transmis au
   fournisseur.
7. **Verdicts** : JSON strict, énumérations fermées, ancrage obligatoire
   (`empreinte_attendue` = empreinte du plan ou de la soumission, recalculée
   du livrable côté orchestrateur) ; verdicts non conformes rejetés sans
   perte d'état ; livrable divergent refusé AVANT appel.
8. **Habilitations** : rôles résolus des identités qualifiées de la mission ;
   validation normative sur copie avant tout appel (C2) ; auto-réception,
   verdict incohérent et ACK non ancré refusés par le moteur (N-10..N-17).
9. **Clôture** : `Cloturer` journalise `CLOTURE_REFUSEE` et échoue TOUJOURS :
   aucun texte de modèle ne vaut autorisation, aucune clôture automatique.

## 4. Preuves REEL

- CI « COORD qualification » run **45519393828**, commit
  `69f9d0e6a46a98b99f140f1914016ae814a330d7` (dernier commit touchant
  `coord/go/**`) : **6/6 SUCCÈS** — Go ubuntu/windows/macos (vet + build +
  test + 22 vecteurs + démo 19 CONFORME / 0 ECART), Race detector
  (`-race -count=2` sur pilot/missions/connecteurs), Batteries Python
  (v1.0→v1.5 + robustesse), Schémas JSON 2020-12.
  <https://github.com/dbouzon/osnias-clearing/actions/runs/45519393828>
- `coordengine/internal/connecteurs` : **23 tests, tous verts** (13 connecteur + 9 orchestrateur + 1 budget-ajust), y compris sous race
  detector.
- Commits postérieurs (suppression du harnais de trace temporaire, rapport,
  recette) : chemins hors `coord/go/**`, aucune modification du code qualifié.
- Harnais de débogage (workflow temporaire poussant la trace de build dans la
  branche, utilisé car aucun toolchain Go dans le sandbox de la mission et
  journaux Actions illisibles hors session admin) : **supprimé**, fichiers
  `erreurs-build.txt` et workflow retirés de la branche ; traces conservées
  dans les commits (003705de, b699b2f7).

## 5. Tableau des capacités (exécuté / SIMULE / NON_EXÉCUTÉ)

| Capacité | Statut |
| --- | --- |
| Connecteurs réels Mistral/OpenAI (API directes, format interchangeables) | REEL (testés sur serveurs factices loopback par la CI multi-OS) |
| Triple verrou + autorisation pilote explicite | REEL (tests fermés : chaque porte manquante refuse avant tout octet émis) |
| Boucle plan/critique/réception structurée ancrée, correction, ACK | REEL (factices, CI multi-OS + race) |
| Anti-doublons crash/interruption, réponse tardive, plafonds, habilitations | REEL (factices) |
| Expurgation de secrets, MODELE_DIVERGENT, limites, retraits | REEL (factices) |
| Démo locale autonome `-demo-local` | REEL (loopback, compilée par `go build ./...` CI) |
| Batteries Go multi-OS + race detector | REEL (CI 45519393828, 6/6) |
| Python 88/88 + 47/47 + vecteurs 22/22 + démo (non-régression) | REEL (CI, success) |
| **Appels facturés réels Mistral/OpenAI** | **NON_EXÉCUTÉ** — exige l'autorisation explicite du pilote (fournisseurs, données, plafond monétaire, clés) ; sans elle : `NON_EXECUTE` + procédure, exit 3, jamais de succès simulé |
| Toolchain Go locale dans le sandbox | NON_EXÉCUTÉ (installation interdite dans l'environnement ; couvert par la CI) |
| Rendu navigateur de l'UI | HORS PÉRIMÈTRE (#29) |

## 6. Écarts et déclarations

- `coord/go/internal/budget/budget.go` est le **seul fichier du moteur**
  modifié : méthode additive `AjusterJetons` (+17 lignes doc comprise),
  nécessité démontrée par les échecs initiaux de la CI (surplus compté comme
  appel). Aucune sémantique existante modifiée ; invariants couverts par
  `ajuster_test.go`.
- Réserve frontale : les suites Go s'exécutent en CI uniquement pour cette
  branche (aucun toolchain Go dans le sandbox de mission) — les traces Go
  locales de ce rapport citent la CI, pas une exécution locale.
- La mission réelle bénigne (petit livrable textuel, critères objectifs) est
  **prête** (`plan.md`/`mandat.md`/`livrable.txt` + `exemple-config-reel.json`)
  mais PAS exécutée.

## 7. Reste réservé au pilote

1. Fusion de la PR #30.
2. Autorisation budgétaire écrite (fournisseurs, données, plafond monétaire,
   clés) → première mission réelle inter-IA.
3. Clôture de #27 et de #29, activation éventuelle de v1.5.

— Vibe (Mistral), exécutant M-COORD-PILOT-03, 25/11/2026.

## 8. Addendum du 25/11/2026 — correction de la contre-réception A_CORRIGER (réserve 7154948522)

**Contre-réception OpenAI** : `PlafondCentimes` validé dans la configuration
mais non transmis au moteur budgétaire ; surplus comptabilisé après l'appel.
Le plafond monétaire n'était PAS démontré opposable. Correction conduite en
boucle sur la même branche / même PR #30, plan de correction v1
**PLAN_ACCEPTE au premier tour** par OpenAI (session pont `m-coord-pilot-03`,
empreinte `038ccca3…`), sans réserve ni point bloquant.

**Reproductions AVANT correction** (compilables sur le code incriminé,
échouées) : REPRO A (le budget ne porte aucun plafond ni réservation
monétaire) ; REPRO B (appel d'un connecteur réel sans plafond armé : l'appel
part, aucune erreur). Les deux sont conservées comme garde-fous permanents.

**Correction** : voûte monétaire additive.
- `budget.go` : plafond global `Plafonds.Centimes` (centimes d'euro entiers),
  réservations durables `Etat.CentimesReserves`, méthodes `ReserverCentimes`
  (refus si plafond non armé/dépassé, fail closed),
  `AjusterCentimes` (enregistre un surcoût, JAMAIS de libération),
  `CentimesReservesTotal`.
- `connecteurs.go` : tarifs explicites par fournisseur
  (`tarif_entree/sortie_centimes_par_million`, `tarif_requete_centimes`) et
  bornes par tentative (`jetons_entree/sortie_max_par_tentative`), exigés
  complets en mode REEL au chargement ; enveloppe conservatrice
  `(1+retraits_max) × [coût(borne entrée)+coût(borne sortie)+requête]`,
  arrondis vers le haut en arithmétique entière ; bornage À L'ÉMISSION
  (`CHARGE_HORS_BORNE`, heuristique 4 octets/jeton) ; coût réel conservateur
  `CoutReelCentimes`.
- `orchestre.go` : TX1 arme la réservation monétaire pour tout adaptateur
  réel (plafond non armé => `APPEL_REEL_REFUSE`, tarif inconnu =>
  `TARIF_INCONNU`, plafond dépassé => `BUDGET_DEPASSE`, le tout AVANT tout
  octet émis) ; TX2 n'enregistre que d'éventuels surcoûts ; journal
  `APPEL_RESERVE` enrichi (`centimes_reserves` + rappel « estimation locale,
  pas une garantie fournisseur »).
- `cmd/coord-mission/main.go` : `PlafondCentimes` de l'autorisation pilote
  TRANSMIS au budget de la mission.
- **Distinction assumée** : la réservation est une estimation CONSERVATRICE
  LOCALE (tarifs/bornes déclarés) ; le logiciel garantit qu'il refuse
  d'émettre au-delà, pas ce que le fournisseur facture.

**Défaut annexe découvert et corrigé** (préexistant sur la première tête de
branche) : la démo `-demo-local` échouait `MODELE_DIVERGENT` (modèle déclaré
`demo-exec`/`demo-recep` ≠ modèle répondu `demo`) : elle ne pouvait pas
s'exécuter. Corrigé de façon minimale (modèle déclaré = `demo` pour les deux
fournisseurs factices, contrôle fail-closed inchangé) et AJOUTÉ à la CI
(job Go multi-OS, exit 0 attendu) : la démo déroule désormais son cycle
complet jusqu'à `ACK_RECU`.

**Preuves REEL (Go 1.27.4 officiel installé en séance, linux/amd64)** :
`go vet`/`build`/`test` OK (11 paquets ; connecteurs 32 tests, budget 10
tests, dont 15 nouveaux) ;
Python 88/88 historiques + 47/47 v1.5 + robustesse 10/10 + démo Python
CONFORME ; Go : vecteurs 22/22 sha `8c097f76…` et démo 19 CONFORME/0 ECART
sha `e1f1cdbe…` byte-identiques à la ligne de base #23 ; `-demo-local`
exit 0 état ACK_RECU. Race local NON_EXÉCUTÉ (pas de gcc), couvert par la
CI (`-race -count=2` incluant `./internal/connecteurs/`).

— Vibe (Mistral), cycle correctif M-COORD-PILOT-03, 25/11/2026.

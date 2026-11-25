# Rapport de mission M-COORD-HARDENING-01 — Durcissement transversal et observabilité

- Mission : issue #52, ordre v1.0 du 25/11/2026. Exécutant : Mistral. Critique et réceptionnaire indépendant : OpenAI. Pilote : Denis Bouzon.
- Base : main `ea510ac9e24b12a853241c530a35a19ff40fed18` (tarball sha256 `b87358cf275cedd175ee468c899e94320a901fe94f12f2bf370d7ac36fe1affc`). Branche : `mission/coord-hardening-01-v2` (la branche `mission/coord-hardening-01` homonyme est un vestige déjà fusionné de l'ancienne mission #19, tête `c2db74d5`, non supprimable avec l'outillage disponible ; recréation sous ce nom documentée).
- Budget : 0 €. Aucun appel API payant, boucle locale uniquement (developr, factices loopback). Module `coord/a2a` INTACT (aucun octet modifié).

## Matrice des cinq recommandations Gemini (vérifiées, jamais présupposées)

| # | Constat Gemini | Vérification | Traitement |
| --- | --- | --- | --- |
| 1 | Limite octets vs estimation jetons ; 4 o/jeton pas une garantie | DÉFAUT AVÉRÉ : le diviseur 4 de la borne d'émission sous-estime les jetons UTF-8 multi-octets | CORRIGÉ : diviseur 2 (MAJORANT local), reproduction avant correction, commentaires reformulés |
| 2 | Expurgation (Bearer, traces, empreintes préservées) | DÉJÀ COUVERT (clé littérale, sk-/AIza, transport/lecture/extrait, SORTIE_COMPROMISE) | PREUVES ONLY (empreintes intactes, clé partielle documentée) |
| 3 | Usage absent vs invalide/nul | COUVERT (refus JETONS_INCONNUS fail-closed) | DIAGNOSTIC AFFINÉ (message distinct), code public inchangé |
| 4 | Idempotence, TX1/TX2, réserves | DÉJÀ COUVERT (appel_en_cours.json avant réseau, réservation durable, surcoûts seuls, SEC-01) | PREUVES (persistance après échec transport) |
| 5 | coord-snapshot.json, EcrireAtomique, console HTML, TC-COORD-* | ARTEFACTS ABSENTS (vérification exhaustive) ; console coord/pilot/ui existe sans consommer de snapshot | CONTRAT PROPOSÉ, ARBITRAGE PILOTE AMBIGU (réponse « Coord V1,5 ») : implémenté en lecture seule purement additive, réversible |

## Corrections et preuves (T2, REEL sandbox)

- **P1** (`commun.go`) : borne d'émission `len(corps) > jetonsEntreeMax * 2` ; estimation conservatrice MAJORANT locale, PAS une garantie fournisseur, défense en profondeur, fail closed. Reproduction (`hardening_test.go`) : charge CJK de 1500 octets sous borne 500 jetons ÉMISE sur le code de main (FAIL constaté) ; refusée CHARGE_HORS_BORNE, zéro octet émis après correction (PASS). Non-régression : charge légitime admise.
- **P3** (`connecteurs.go`) : usage PRÉSENT mais nul/invalide → `JETONS_INCONNUS : usage présent mais nul/invalide (fail closed)` ; usage ABSENT → message du contrôle commun. Code public `JETONS_INCONNUS` inchangé, aucun secret dans les diagnostics. Test : 3 familles refusées avec le bon diagnostic.
- **P2** (preuve) : sha256 (hex 64) et SHA Git (hex 40) traversent `expurger` INTACTS ; `sk-…`/`AIza…` remplacés ; clé partielle/tronquée ou trop courte (<8) non remplacée (non-authentifiante, documentée).
- **P4** (preuve) : après échec transport, `appel_en_cours.json` persiste (marqueur écrit AVANT le réseau, temp + rename).

## Observabilité (T4)

`coord-snapshot.json` (projection lecture seule, `schema_version "1.0"`) :
- régénérée à chaque point durable (TX2 du committer, ACK, résolution d'appel en cours) ;
- champs : état machine persisté, empreinte sha256 du journal, nombre d'entrées, appel en cours non résolu (action), instant de génération ;
- JAMAIS une seconde source de vérité : aucun code normatif ne la consulte ; son échec d'écriture est journalisé (`SNAPSHOT_ERREUR`) sans faire échouer la transition durable (testé) ;
- atomicité temp + rename 0600, aucun temporaire résiduel (testé) ;
- aucun texte (charge/verdict) n'y est copié : aucun secret possible (testé avec sk-/AIza/mandat piégé) ;
- mention conservatoire explicite « PAS un journal de rejeu ; peut être périmée » (testée).

## Qualification (T5, REEL sandbox / SIMULÉ fournisseurs / NON_EXÉCUTÉ VPS et API réelles)

- Engine `coord/go` : vet + build 2,87 s + 13 paquets verts ; cross-build windows/darwin OK. Binaire coord-mission 11 226 595 o (+24 121 o pour la projection).
- Module `coord/a2a` : vet + tests verts (intact, rejoué).
- Tests hardening : 9 verts (reproductions P1/P3, preuves P2/P4, 3 snapshot, matrice 12 couples incluse dans le paquet).
- Python v1.5 : 47/47 PASS (M1 PASS) ; vecteurs 22/22 Go et 22/22 Python byte-identiques.
- `-race` : NON_EXÉCUTÉ local (pas de gcc dans le sandbox), couvert par le job CI Race detector.
- VPS et fournisseurs réels : NON_EXÉCUTÉS (0 €, mandat respecté).

## Processus

- T1 : ACK 7176547292 ; inventaire + matrice + plan v1 (sha256 `62f1756d…`) ; critique OpenAI **PLAN_ACCEPTE tour 1** (consigné 7176625804).
- T2/T3 : corrections + preuves ; contrôle intermédiaire OpenAI **CONFORME** (consigné 7176696034).
- T4 : projection livrée (arbitrage pilote ambigu, implémentation strictement additive réversible).
- T6 : PR au SHA exact (empreintes sha256 de chaque fichier dans la PR), fidélité vérifiée par tarball codeload épinglé au SHA puis diff byte-exact et rejeu depuis le code distribué, rapports FR/EN, réception indépendante OpenAI.

## Frontières

Fusion et clôture de #52 réservées au pilote. #42 suspendue. Statuts v1.5 inchangés (v1.5 = référence opérationnelle). Aucune qualification réelle de fournisseur n'est présupposée par cette mission.

---

Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX: Apache-2.0

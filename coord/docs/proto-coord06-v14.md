<!-- Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 -->

# Protocole M-COORD-06 v1.4 (candidate, NON ACTIVEE) — spécification

Issue de référence : #157 (mission M-COORD-09). Version en vigueur : v1.3 (#150, activée par le pilote, merge 19701e6c).
Statut : candidate. Aucune activation sans décision expresse du pilote. Aucune fusion automatique.

## 1. Objet et diagnostic T0

La v1.4 prolonge la v1.3 sans la modifier. Diagnostic T0 (inventaire M-COORD-09, 24 novembre 2026, sources : #150, #132, #155 et commentaires, #106, Knowledge chatgpt-pont) :

- E1 : après un verdict A_CORRIGER, la pratique rend la main au pilote entre les rounds ; la règle C2 impose la même issue et la même PR mais pas l'enchaînement immédiat. Latence et échanges redondants constatés.
- E2 : les réinitialisations de sandbox sont fréquentes et la reconstruction est coûteuse (M-COORD-13 : re-exécution des 71 tests ; M-ERP-CONFORM-01 : 97 empreintes re-vérifiées).
- E3 : des erreurs de transcription manuelle d'empreintes ont provoqué un incident de divergence de 20 octets (#155, règle durable).
- E4 : la faisabilité de certains tests est découverte tardivement (`go test -race` cgo absent découvert en T6 ; test d'intégration réel exécuté puis non conservé).
- E5 : le budget d'appels B5=3 a été dépassé en pratique (M-COORD-13) ; multiplicité des sollicitations intermédiaires.
- E6 : l'état de reprise persisté ne comporte ni tranche, ni round, ni prochaine action, ni curseur relais : la reprise exige une reconstruction du fil de conversation.
- E7 : la réception n'est pas différentielle : C4 ancre le verdict au SHA mais ne définit pas le réexamen des critères validés lors de la réception précédente.

Baseline T1 (exécutée, reconstruction vérifiée 19/19 empreintes de blobs depuis main 19701e6c) : run.sh 24+13 PASS, run_v12.sh 11, run_v13.sh 13, run_robustness.sh 10 ; 71/71 PASS, exit 0.

## 2. Amendement D1..D7

### D1 — Continuité autonome impérative (étend C2)

Énoncé : tout verdict A_CORRIGER entraîne un retour immédiat à l'étape concernée, dans la même session : A_CORRIGER lors de la critique préalable renvoie à la conception ; A_CORRIGER lors de la réception renvoie à l'exécution. Mistral analyse, corrige, teste et soumet à nouveau immédiatement, sans attendre le pilote, sans clôturer ni rendre la main volontairement. ACCEPTEE et ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES entraînent la tranche suivante ou la préparation de clôture ; les réserves explicitement non bloquantes n'interrompent pas le travail. Une sollicitation du pilote n'est justifiée que dans deux cas bornés : blocage objectif hors autonomie, documenté après recherche d'alternatives autorisées, ou décision réservée au pilote. Détection des boucles sans progrès par compteur absolu de rounds consécutifs dans une même phase : critique ciblée demandée au pont au second round A_CORRIGER consécutif ; à partir du troisième round sans convergence, escalade bornee C2 (motif deux_cycles_sans_convergence) ; l'escalade ne suspend jamais le travail déjà publié.

Budget d'appels par phase (remplace le budget global B5=3, dont l'esprit est conservé) : critique préalable 2 appels ; réception 2 appels par round plus un appel par round de correction supplémentaire ; un appel de vérification du pont est admis après une réinitialisation de sandbox. Tout appel est comptabilisé dans D7 ; aucun appel déjà joué n'est jamais rejoué (B4) ; tout dépassement est une réserve justifiée, jamais une interruption.

Règles : continuity_step(verdict, phase, round_count) ; pilot_solicitation(blockage, alternatives, decision_pilote) ; phase_api_budget(phase, rounds).

### D2 — État de reprise étendu, support double (étend A3/B4)

Énoncé : chaque mission publie un enregistrement de reprise normalisé : mission, tranche, round, issue, branche, SHA de base, SHA de tête, curseur relais (#106), réserves ouvertes (registre C2), prochaine action, horodatage UTC. Support double : (a) fichier d'état durable dans la Knowledge OSNIAS, persistant entre sessions et synchronisé vers GitHub dès qu'une publication est autorisée ; (b) commentaire d'issue à chaque fin de round. GitHub demeure la référence publique faisant foi. La reprise est automatique dès qu'une session est disponible : lecture de l'état durable, vérification d'identité du dépôt et du HEAD, exécution de la prochaine action. Il est interdit de prétendre qu'une sandbox arrêtée continue de fonctionner. En cas d'indisponibilité GitHub, l'état est conservé et republié à la connexion suivante (aucun poll).

Règles : resume_record(...) ; validate_resume_record(...) ; resume_marker(...) ; persist_resume(record, sinks) ; resume_next_action(record).

### D3 — Empreintes automatisées (étend C3)

Énoncé : aucune transcription manuelle d'empreinte dans les preuves. Les empreintes (SHA-1 de blob Git, SHA-256 des sorties) sont calculées par le module ; la vérification de parité local/distant est consignée comme enregistrement normalisé C3 (étiquette REEL si et seulement si parité OK). La parité vérifiée fait partie des preuves de preflight quand git est BLOQUE (plan alternatif C1). L'incident de divergence de 20 octets devient structurellement impossible dans les preuves.

Règles : git_blob_sha1(data) ; verify_parity(expected_manifest, compute_sha) ; parity_proof(...).

### D4 — Faisabilité des tests qualifiée à la conception (étend C1/B3)

Énoncé : la matrice de preflight est complétée, dès le plan, par une qualification de chaque test prévu : FEASIBLE_REEL, FEASIBLE_MOCK ou INFEASIBLE_ALTERNATIVE (avec plan obligatoire). FEASIBLE_REEL exige un statut de preflight EXECUTABLE. Un test non exécutable n'est jamais déclaré PASS (B3) ; son étiquette figure dans le dossier de réception. Les tests de non-régression utiles sont conservés dans le périmètre autorisé.

Règles : qualify_test(...) ; validate_feasibility(qualifications).

### D5 — Dossier de réception rationalisé (réutilise §13 + C3)

Énoncé : le dossier de réception comporte exactement six sections normées : mandat, environnement, manifeste, tests et preuves, réserves, verdict. Les formats existants suffisent : la section tests et preuves contient les enregistrements normalisés C3 (étiquettes REEL/SIMULE/NON_EXECUTE), la section réserves le registre C2, la section verdict une ancre C4/D6. Aucun nouveau format n'est créé.

Règles : validate_dossier(dossier) ; DOSSIER_SECTIONS.

### D6 — Réception différentielle liée au SHA précédent (étend C4)

Énoncé : la réception est rendue sur une ancre différentielle complète : SHA précédent, SHA courant, empreinte des preuves, liste des critères validés lors de la réception précédente avec les fichiers auxquels chacun s'applique. Toute réception après correction comporte un réexamen explicite : corrections demandées appliquées, impacts collateraux du diff, critères validés conservés seulement si leurs fichiers sont inchangés ; tout critère absent de la liste ou dont un fichier a changé est réévalué. La réévaluation proportionnée C4 (restreinte/complète) est conservée.

Règles : differential_anchor(sha_prev, sha_cur, proofs, validated_criteria) ; is_stale_differential(...) ; conserve_criteria(anchor, changed_files) ; reexamen_differential(anchor, corrections, changed_files).

### D7 — Mesures de coordination (étend B5)

Énoncé : chaque tranche enregistre des mesures normalisées nommées avec leur unité : durée des sessions actives (s), rounds (par phase), réinitialisations, reconstructions (fichiers re-vérifiés), appels API (par type), messages publiés (par marqueur et canal), sollicitations du pilote, interventions humaines (nombre et nature). Jamais de métrique inventée ni de simulation présentée comme mesure réelle. Les mesures alimentent la comparaison avant/après et sont conservées dans le dossier de réception.

Règles : coordination_metrics(...) ; validate_metrics(metrics) ; METRIC_UNITS.

### Non-redondance et couverture des risques

Chaîne sans recouvrement : D3 généralise C3 (empreintes) ; D2 généralise A3/B4 (durabilité) ; D7 étend B5 (comptage par phase) ; D1 étend C2 (enchaînement immédiat) ; D4 étend C1/B3 (faisabilité au plan) ; D5 réutilise les formats existants ; D6 étend l'ancre C4. Gestion des erreurs et interruptions : indisponibilité GitHub ou du pont → état conservé et reprise (B4, aucun verdict ni ACK fictif) ; interruption de session → enregistrement de reprise D2 ; test impossible → étiquette D4, jamais PASS (B3) ; reconstruction → parité automatisée D3 et re-exécution obligatoire des tests concernés avant toute affirmation.

## 3. Matrice v1.3 vers v1.4

| Aspect | v1.3 (#150) | v1.4 candidate (#157) |
| --- | --- | --- |
| A_CORRIGER | même issue, même PR, escalade à deux cycles | D1 continuité impérative : retour immédiat à l'étape (conception/exécution), compteur absolu de rounds, critique ciblée au 2e, escalade C2 au 3e, sollicitations bornées |
| Reprise | RECEPTION_DEMANDEE persisté | D2 enregistrement de reprise complet (tranche, round, curseur, réserves, prochaine action), support double Knowledge + GitHub, reprise automatique |
| Empreintes | empreintes stockées | D3 calculées et vérifiées par parité automatisée, preuve C3 |
| Faisabilité | preflight à T0 | D4 qualification de chaque test dès la conception, liée à la preflight |
| Réception | ancre couple (SHA, empreinte) | D6 ancre différentielle complète avec critères validés listés, réexamen explicite |
| Dossier | rapport §13 + preuves C3 | D5 six sections normées, formats existants réutilisés |
| Budget d'appels | B5 = 3 global | D1 budget par phase, tout appel compté dans D7, aucun rejeu (B4) |
| Mesures | appels consignés | D7 huit mesures nommées avec unités |
| Réception indépendante | B1/C4 conservés | inchangés et renforcés (D6) |
| Clôture, fusion, activation | pilote | inchangés |

## 4. Migration et rétrocompatibilité

- coord06.py (v1.0/v1.1), coord06_v12.py et coord06_v13.py ne sont jamais modifiés ; coord06_v14.py importe les trois sans les altérer.
- Les batteries et lanceurs v1.0..v1.3 restent inchangés et doivent rester verts ; la batterie v1.4 est distincte (tests_coord06_v14.py, lanceur run_v14.sh).
- L'enregistrement v1.4 étend l'enregistrement v1.3 par délégation stricte (validation v1.3 appliquée sur copie portant la version v1.3, la version v1.4 étant contrôlée séparément).
- La v1.3 reste en vigueur jusqu'à validation expresse de la v1.4 par le pilote. En cas de non-validation, la branche de candidature est abandonnée sans effet sur la version en vigueur.

## 5. Références de test

- tests/coord06/coord06_v14.py : module candidat (import coord06, coord06_v12, coord06_v13).
- tests/coord06/tests_coord06_v14.py : batterie D1..D7 (17 tests, UNITAIRES/MOCKS, aucun E2E réel).
- tests/coord06/run_v14.sh : lanceur séparé ; run.sh, run_v12.sh, run_v13.sh, run_robustness.sh inchangés.
- E2E réels de la mission : la mission M-COORD-09 elle-même (critique préalable réelle A_CORRIGER puis PLAN_ACCEPTE en deux rounds sans intervention humaine ; réception indépendante réelle sur le SHA exact ; blocage réel documenté : git et apt interdits dans la sandbox, plan alternatif appliqué et prouvé).

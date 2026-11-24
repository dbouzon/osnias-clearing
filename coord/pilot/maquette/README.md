# Maquette COORD Pilot — guide de lecture

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

## Ouvrir la maquette

Ouvrir `index.html` dans un navigateur, directement depuis le système de fichiers : aucun
serveur, aucune dépendance, aucune connexion requise. Le bandeau **DONNÉES SIMULÉES** est
permanent sur tous les écrans : tout ce qui s'affiche est codé en dur ou calculé en mémoire
par le JavaScript de la page ; rien n'est persisté, aucun appel réseau n'est émis.

## Parcours suggéré (10 minutes)

1. **Prise en main** : déroulé d'une mission en 6 étapes, légende des 8 états, concepts
   (adaptateur, habilitation, preuves/empreinte, arbitrage, budget).
2. **Missions** : référentiel multi-missions simulé, compteurs de budget, statut d'arrêt.
3. **Créer une mission** : tester les refus fail-closed — exécutant = réceptionnaire
   (bouton « Vérifier le refus »), liste d'adaptateurs vide ; puis une création valide.
4. **Suivi du cycle** : chronologie d'une mission, soumission avec révision SHA-1 et
   empreinte SHA-256 ; émettre un verdict ; cocher la case N-13 pour voir le refus d'un
   verdict obsolète (preuves modifiées).
5. **Arbitrage humain** : champs divergents entre copies, arbitrage « GitHub fait foi »
   (N-16) ; reprise refusée si le head déclaré ≠ head attendu (N-17), exécutée sinon.
6. **Arrêt / reprise** : demander l'arrêt, tenter une transition (refus motivé), lever
   l'arrêt ; tableau comparatif `ARRET_PILOTE` vs `ECART` (N-20).
7. **Coûts** : compteurs par mission/adaptateur ; bouton « Tenter une transition sans
   budget restant » pour voir le blocage avant tout appel.
8. **Journal** : entrées de tous types (TRANSITION, REFUS, VERDICT, ARRET_PILOTE,
   ARBITRAGE, ECART, BUDGET), avec motif — les refus sont journalisés autant que les
   réussites.

## Ce que la maquette n'est PAS

Ce n'est pas le moteur et ce n'est pas une preuve de conformité : les transitions
affichées sont simulées pour valider l'ergonomie cible. La vérité normative réside dans
`coord/go` (COORD Specification 1.0, invariants N-01..N-20).

## Contrôles exécutables

`python3 test-maquette.py` rejoue 12 contrôles statiques (bandeau, 8 écrans, cohérence de
la navigation, aucun appel externe, identifiants DOM, états/invariants/rôles normatifs,
mention ARRET_PILOTE/N-20, copyright, équilibre des sections, parse HTML) et écrit
`recette-maquette.resultat.txt`. Résultat au 24/11/2026 : **PASS=12 FAIL=0 (REEL)** ; le
rendu navigateur reste à la charge du pilote (NON_EXÉCUTÉ ici, aucun navigateur embarqué).

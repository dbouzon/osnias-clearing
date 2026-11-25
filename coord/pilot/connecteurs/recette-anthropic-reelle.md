# Procédure de recette RÉELLE du connecteur Anthropic — M-COORD-LLM-01

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

**Statut : NON_EXÉCUTÉ.** Préparée (tranche 1 : Claude critique de plan et
réceptionnaire), à exécuter UNIQUEMENT sur autorisation écrite du pilote
(bloc `autorisation_pilote` du fichier de configuration réel, conformément
au cycle établi en #29). Aucune qualification réelle n'est acquise tant
que cette recette n'est pas exécutée et consignée.

## 1. Pré-requis

- Compte Anthropic avec clé API (variable d'environnement
  `COORD_CLE_ANTHROPIC`, jamais dans un fichier commité).
- Tarifs/bornes relevés sur la grille réelle du compte et saisis dans la
  configuration (voir `exemple-anthropic.json`) ; plafonds soumis à
  approbation du pilote.
- Mode `REEL` + `COORD_MODE_REEL=1` + clé présente + bloc
  `autorisation_pilote` complet (fournisseurs, plafond monétaire,
  plafonds d'appels et de jetons, date). Le bloc EST l'autorisation.

## 2. Vérifications préalables gratuites

- `GET https://api.anthropic.com/v1/models` avec `x-api-key` (hors COORD,
  coûte 0 €) : valider la clé et l'identité EXACTE du modèle à déclarer
  (leçon #29 : l'alias peut diverger de l'identité répondante ;
  `MODELE_DIVERGENT` est opposé fail-closed).

## 3. Déroulé (mission bénigne Mistral exécutant / Claude réceptionnaire)

1. **Critique de plan réelle** : plan court ancré, Claude doit répondre en
   JSON strict `{verdict, points, points_detail[{severite, texte}],
   empreinte_attendue}` (sévérité obligatoire, enseignement v1.5/#29).
   Attendre `PLAN_ACCEPTE` ou boucler `A_CORRIGER` avec points
   journalisés (détail + sévérité).
2. **Exécution Mistral** (connecteur existant) puis **soumission**.
3. **Réception réelle par Claude** : verdict JSON strict ancré à
   l'empreinte (`empreinte_attendue`) + `verdict_id` + `verdict_url`
   (référence durable du verdict, exigence v1.5 de réception).
   Contrôler : gabarit respecté sinon RECette en échec (fail closed,
   aucun commit à tort — prouvé en C15 simulée).
4. **ACK_RECU** (0 €), `CLOTURE_REFUSEE` attendu (clôture réservée
   au pilote).
5. **Contrôles de traçabilité** : aucun secret dans les journaux
   (grep de la clé : 0 occurrence), réservation monétaire avant chaque
   appel (`APPEL_RESERVE` + `centimes_reserves` dans le journal),
   plafonds et usage réels cohérents avec la facturation Anthropic.

## 4. Critères de succès

- Cycle complet jusqu'à `ACK_RECU` sans violation fail-closed.
- Budget : total facturé ≤ total réservé (l'estimation locale est
  conservatrice) ; dépassement plafond = `BUDGET_DEPASSE` opposé.
- Traçabilité : verdict ancré, points avec sévérité, référence durable,
  aucun secret publié.

## 5. Compte rendu

Consigner : SHA de la version, commandes, journaux expurgés, coûts
réels constatés (centimes), verdict de qualification réelle
(`QUALIFICATION_REELLE_ACQUISE` uniquement après exécution), dans
l'issue #40 et le dossier Drive `coord/02-Missions-et-decisions`.

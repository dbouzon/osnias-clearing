# COORD Pilot — proposition de MVP et recette mesurable

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Statut** : proposition issue de M-COORD-PILOT-01 (issue #25). Toute mise en œuvre est une
mission distincte, sur autorisation du pilote. v1.5 candidate non activée ; v1.4 référence.

## 1. Proposition de MVP (périmètre de la mission suivante)

Un service Go local (`cmd/coord-pilot`, paquets `internal/{pilot,missions,budget,arret,adaptateurs}`)
dans le module `coordengine`, fichiers nouveaux uniquement, piloté par une interface web
locale (extension de la maquette) :

- serveur HTTP lié à `127.0.0.1`, token d'authentification locale régénéré à chaque
  démarrage, affiché en console uniquement ;
- référentiel multi-missions : un dossier par mission, enregistrement via
  `persist.SaveAtomic` (N-15), journal via `persist.JournalExecute` (N-19) ;
- adaptateurs **simulés uniquement** (aucune clé, aucun réseau, refus fail-closed de tout
  mode « réel », hérité de la leçon T02 d'`ai-coordinator`) ;
- habilitations fail-closed par mission (N-01..N-05), rôles cinq actions ;
- budgets : compteurs par mission/adaptateur, plafonds fixés à la création, contrôle
  AVANT toute transition déclenchant un appel, refus motivé au-delà ;
- arrêt humain effectif : interception au service, journal `ARRET_PILOTE`, reprise
  exclusive via N-16/N-17 ;
- une mission complète de bout en bout : création → exécution → soumission → verdict →
  ACK → clôture par le pilote, journal intégral, 0 € (aucune API appelée).

## 2. Critères de recette mesurables

Chaque critère est rejouable en local par un test automatisé ou une manipulation listée ;
le résultat est consigné OK / NON_EXECUTE avec preuve (extrait de journal, SHA).

**Moteur et habilitations**
- R1 : toute transition hors table fermée est refusée avec motif (test exécutable, fixtures).
- R2 : une configuration d'habilitations vide ou mal formée ferme le service au démarrage
  (N-05), message journalisé.
- R5 : divergence simulée entre deux copies → champs divergents listés, arbitrage
  `github_fait_foi` consigné, reprise exécutée si et seulement si le head vérifié
  correspond (N-16/N-17).
- R6 : verdict rendu sur des preuves modifiées → refus (N-13), motif journalisé.

**Budget et arrêt**
- R3 : dépassement de plafond → transition bloquée avant tout appel d'adaptateur, motif
  journalisé, aucune action externe.
- R4 : arrêt demandé → toute transition refusée (motif « arrêt demandé par le pilote »),
  état machine intact ; à la levée, le cycle reprend via N-16/N-17 uniquement.

**Non-régression**
- R7 : batteries existantes rejouées à l'identique : 88/88 (v1.0..v1.4), 47/47 (v1.5),
  22/22 vecteurs et démo, sorties byte-identiques aux lignes de base publiées
  (vecteurs `8c097f76…`, démo `e1f1cdbe…`).

**Sécurité locale (S1..S5, exécutables REEL dès le MVP, prix nul)**
- S1 : appel sans token → 401 journalisé, aucune transition.
- S2 : token invalide/périmé → 401, aucune transition.
- S3 : le service refuse de démarrer si l'écoute n'est pas loopback (fail-closed).
- S4 : rôle PILOTE non dérivable d'un corps de requête non authentifié (forgé → refus).
- S5 : habilitations invalides → démarrage refusé (N-05), motif journalisé.

**Interface**
- R8 : la maquette navigue entre ses 8 écrans sans erreur, bandeau simulé visible
  partout, écran « Prise en main » accessible depuis le tableau de bord.
- T1..T12 : contrôles statiques de la maquette (`test-maquette.py`) au vert.

## 3. Estimation indicative des coûts

- Conception + maquette (cette mission) : **0 €**.
- MVP local : **0 €** (adaptateurs simulés, aucune clé, aucun compte).
- Cible VPS éventuelle (mission future, hors périmètre actuel) : VPS 1 vCPU / 1-2 Go
  ≈ 5-7 €/mois (indicatif marché) ; API fournisseurs selon usage réel, non engagé ici.

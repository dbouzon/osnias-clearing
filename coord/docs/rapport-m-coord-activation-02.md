<!-- Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902 — SPDX-License-Identifier: Apache-2.0 -->

# Rapport — M-COORD-ACTIVATION-02 — cohérence documentaire post-activation

Mission : issue #86 (mandat v2.0 du 27/11/2026, recadré après erratum). Pilote : Denis Bouzon. Exécutant : Mistral (Vibe). Critique et réception indépendante : OpenAI. Contexte : l'activation officielle de COORD v1.5 et COORD Specification 1.0 a été accomplie le 25/11/2026 19:15 UTC par la mission M-COORD-ACTIVATION-01 (issue #44) ; la présente mission corrige les résidus documentaires relevés à l'inventaire du 27/11/2026.

## Correctifs (4 constats, 3 fichiers documentaires)

1. `en/coord/index.html` section 07 : contradiction interne supprimée (« v1.4 operational (reference). Non-activated candidates… » vs hero activé) ; réécrite en miroir de la section 06 FR : v1.5 opérationnelle activée le 25/11/2026 (mission #44), v1.4 et antérieures historiques, COORD Specification 1.0 norme découplée dont v1.5 est l'implémentation conforme de référence, lien vers le dossier de qualification, mention que les résultats simulés ne qualifient pas les fournisseurs.
2. Mentions « (candidate) » obsolètes retirées : sections Architecture des pages FR et EN (« COORD Specification 1.0 » sans qualificatif), section 04 EN (« hardened v1.5 battery », « v1.5: 47/47 PASS »).
3. `coord/index.html` section 04 : bloc de commandes complété de `run_v15.sh` (47/47 PASS), total étendu (« 88/88 PASS, exit 0 ; v1.5 opérationnelle : 47/47 PASS et 22/22 vecteurs de conformité »), ressource « COORD Specification 1.0 » ajoutée (lien `coord-spec-1.0.md`).
4. `coord/README.md` : puce Batteries complétée de `tests_coord06_v15.py` (47) ; bloc de commandes complété de `run_v15.sh` et du runner de vecteurs ; section « Implémenter le protocole » : pointeurs vers `proto-coord06-v15.md` (activée), `coord-spec-1.0.md` et `coord/go` (moteur de référence), `proto-coord06-v14.md` et `tools/ai-coordinator` requalifiés historiques.

## Paires FR/EN vérifiées

| # | Élément | FR | EN |
|---|---|---|---|
| P1 | Statut hero | « v1.5 opérationnelle (activée le 25 novembre 2026) » | « v1.5 operational (activated on 25 November 2026) » |
| P2 | Section état | Section 06 : v1.5 opérationnelle, #44, dossier de qualification | Section 07 réécrite : miroir (issue #44, dossier de qualification) |
| P3 | Libellé batterie v1.5 | « batterie v1.5 (opérationnelle) : 47/47 PASS » | « hardened v1.5 battery: 47/47 PASS » |
| P4 | Specification 1.0 sans « (candidate) » | Section Architecture | Section Architecture |
| P5 | Totaux batteries | 88/88 + 47/47 + 22/22 vecteurs (section 04) | 88/88 + 47/47 + 22/22 vecteurs (section 04) |
| P6 | Ressource Specification 1.0 | Ajoutée section 04 | Déjà présente section 04 |

## Recensement « candidate » (avant → après)

Avant : 8 occurrences dans les 3 fichiers, dont 4 obsolètes (v1.5/Specification 1.0) et 4 légitimes (batteries historiques v1.2/v1.4 des blocs de commandes). Après : 4 occurrences, toutes légitimes et inchangées (« batterie v1.2 candidate », « batterie v1.4 candidate »). Le libellé du lanceur `run_v15.sh` (« v1.5 CANDIDATURE non activee » dans sa sortie) relève du fichier figé par le manifeste et n'est pas modifié (D1).

## Contrôles REEL (27/11/2026, arbre main 733b717e + patch)

- Batteries Python : v1.0+v1.1 OK, v1.2 OK, v1.3 OK, robustesse T3 OK, v1.4 OK, v1.5 **PASS=47 FAIL=0 (M1 PASS : index.html et README.md exemptés « pages publiques évolutives », présence obligatoire, manifeste 323d8b9 intact)** ; vecteurs OK=22 KO=0.
- Cibles de liens présentes dans le dépôt : `coord-spec-1.0.md`, `proto-coord06-v15.md`, `qualification-m-coord-activation-01.md`, `runner_conformite.py`, `coord/go/`, `tools/ai-coordinator/` ; issue #44 (clôturée completed).
- Scan de secrets dans les 3 fichiers : 0 occurrence.
- Aucun fichier moteur, batterie, schéma ou vecteur modifié (D1) ; `tests_coord06_v15.py` inchangé ; manifeste `manifeste-historique-main.json` inchangé.

## Empreintes sha256 des fichiers livrés

- `coord/index.html` : `9c62c49be925738ccaf890c9568c87e9af970df0c2b804e8963f47186f9ec479`
- `en/coord/index.html` : `f59a828becf3d276f6940653fc5e9e5ce2a38ba3a07b35ff25eaaa4628e28c17`
- `coord/README.md` : `6a1dab9d1a9d300f88b948b332a6af653c27b4c19be98548da4be1f9bc66215b`

## NON_EXECUTE et réserves

Rendu navigateur complet : contrôles HTTP/contenu post-fusion uniquement. Go : non re-executé dans ce rapport (sans objet : aucun fichier Go modifié ; CI multi-OS inchangée). Post-fusion (décision du pilote) : contrôles Pages `/coord/`, `/en/coord/` (HTTP 200 + nouveaux contenus), archive zip reconstruite contenant les fichiers actualisés. 0 €, aucune clé, aucun appel facturable.

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — Apache-2.0.

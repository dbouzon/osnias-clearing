# Archives COORD

Répertoire d'archivage des issues et travaux accomplis ou rendus obsolètes.
Chaque relevé d'archivage est daté et conserve l'historique : numéros, objet, motif de clôture, référence vers le livrable ou la trace réelle.

## Relevé 2026-11-27 — Première vague d'archivage (pilote : Denis Bouzon)

Décision du pilote : « créer un répertoire archives et on met dedans tel que » — archiver les items ni mergés ni en cours, accomplis ou obsolètes.

| Item | Objet | Motif de clôture | Trace / livrable |
|---|---|---|---|
| #2 (M-AIC-01, T01) | Faisabilité Osnias AI Coordinator (OpenAI ↔ Mistral) | Faisabilité tranchée : le pont ChatGPT ⇄ Mistral est opérationnel depuis le 22/11 | Knowledge chatgpt-pont, sessions de travail binôme |
| #8 (M-AIC-05) | Raccordement sécurisé des API OpenAI ↔ Mistral (Termux) | Accompli par le pont ChatGPT (clé locale, appels directs depuis le sandbox) | /home/user/knowledge/chatgpt-pont |
| #12 (M-AIC-10) | Canal de discussion bidirectionnel ChatGPT ↔ Mistral Code | Doublon fonctionnel du pont ; la PR #13 (mission/aic-10) reste ouverte à l'initiative du pilote | PR #13 conservée ouverte |
| #14 (M-COORD-DOC-01) | Organiser la documentation sur Drive | Accompli : Drive coord structuré (01-Specifications, 02-Missions-et-decisions, 03-Recettes) | Drive dossier coord |
| #15 (M-COORD-WEB-01) | Présenter COORD et guider son implémentation | Accompli : site coord.anhydra.fr en production, design v2 fusionné (PR #103) | https://coord.anhydra.fr |
| #64 (M-COORD-VPS-01) | Première recette autonome de COORD sur le VPS | Accompli : COORD servi en production (release 20850ee6, front-v3-3), /api/sante ok, recettes consignées | release-actuelle.txt, blocs FRONT-V3-x |

Items explicitement conservés ouverts à cette date :
- PR AIC #5, #6, #13 (patrimoine des premières missions, statut laissé à l'arbitrage du pilote) ;
- issues mission actives : #19, #20, #23, #25, #27, #65, #66.

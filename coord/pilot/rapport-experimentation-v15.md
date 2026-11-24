# Rapport d'expérimentation COORD v1.5 — M-COORD-PILOT-01

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0

**Mission** : M-COORD-PILOT-01, issue #25 (dbouzon/osnias-clearing). Rapport du 24/11/2026.
**Objet** : documenter l'expérimentation du protocole v1.5 sur le cycle de cette mission même,
comme le demande l'issue : transitions, preuves, SHA, corrections, reprise après
interruption, refus d'un verdict obsolète, séparation des rôles.

**Statut v1.5 rappelé** : candidate NON activée ; v1.4 référence opérationnelle. Cette
expérimentation n'active rien et ne revendique aucune conformité pour un scénario simulé :
le moteur (Python `coord06_v15.py` et Go `coordengine`) est exécuté REEL ; les identités IA
sont celles, réelles, de la mission, mais les adaptateurs qui les qualifient sont déclarés
en configuration locale (SIMULÉS au sens de la spec §11 : canal non vérifié par le moteur).

## 1. Le cycle de mission enregistré dans le moteur v1.5 (REEL)

Le pilote `coordengine/experimentation-v15.py` (livré dans `preuves/`) a rejoué les
événements réels du cycle de plan de cette mission dans la machine d'états durcie v1.5 :

| Transition | Décision / preuve | Référence réelle |
| --- | --- | --- |
| PLANIFIEE → EXECUTION | exécutant `mistral-vibe` (EXECUTANT, adaptateur `mistral-sandbox`) | travail engagé après ACK (commentaire 7146730093) |
| EXECUTION → DECISION_ATTENDUE (plan v1) | revision `4383302ec91d41a015bc573c65664bc4f9b81017`, empreinte `d4fa45f2bd9d057870521341dee0a6a57fa8901e6873004a7355cb020e10eaa1` | plan v1 soumis à la critique (SHA-1/SHA-256 du fichier) |
| DECISION_ATTENDUE → EN_CORRECTION | verdict `A_CORRIGER`, 5 points | session pont `m-coord-pilot-01`, critique REEL d'OpenAI |
| EN_CORRECTION → EXECUTION | même exécutant (stabilité vérifiée) | corrections intégrées |
| EXECUTION → DECISION_ATTENDUE (plan v2) | revision `446dcc813675645c0296c97c8bea6a6ef587645a`, empreinte `f252754d2595861928ce881624d8fb90b385f9a501fca470345b815fd3f110e5` | plan v2 (SHA-1/SHA-256 du fichier) |
| DECISION_ATTENDUE → ACCEPTEE | verdict `ACCEPTEE` (label pont « PLAN_ACCEPTE »), verdict_id `issuecomment-7146765454` | publication REEL du verdict dans #25 (N-12 : référence obligatoire) |
| ACCEPTEE → ACK_RECU | exécutant, preuves v2 confirmées | publication de l'étape dans #25 |

**État atteint : ACK_RECU. La clôture (ACK_RECU → CLOTUREE) n'est PAS effectuée** : elle est
réservée au PILOTE (N-14) et suivra la réception des livrables et la décision du pilote.
Aucun état au-delà du réel n'a été simulé. Bilan d'exécution : **26 CONFORME / 0 ECART**,
exit 0 (adaptateurs côté IA simulés, moteur réel), trace complète dans
`preuves/experimentation-trace.txt` (sha256
`030c8d9eae34e98ff1ee10e38b24b146913f49d7108a2c5af20d200aa5df09fb`),
enregistrement de mission dans `preuves/record-m-coord-pilot-01.json` (sha256
`400302a61141cacd6f3178d9d13afc792bc3e0f3b4ba9cf84079d149d3c41520`).

## 2. Contrôles négatifs exécutés REEL (fail-closed vérifiés sur cette mission)

- configuration avec role inconnu, joker, adaptateurs vides : refusée ;
- identité non qualifiée par un adaptateur déclaré : `executer` refusé ;
- auto-réception (l'exécutant rend un verdict sur sa propre soumission) : refusée ;
- ACK avec preuves divergentes de celles du verdict : « verdict perime » refusé (H3/N-13) ;
- clôture tentée par l'exécutant : refusée ("attendu: PILOTE") ;
- ressoumission : la décision A_CORRIGER rendue sur le plan v1 est aujourd'hui périmée
  (empreinte divergente) — conforme H3 : la garde est par empreinte, jamais par compteur ;
- la décision ACCEPTEE sur le plan v2 n'est pas périmée (empreinte identique) ;
- crash injecté « avant_remplacement » : fichier original INTACT, temporaire nettoyé (H4/N-15) ;
- reprise après arbitrage avec head divergent : refusée dans les deux variantes testées
  (head de l'essai, sha_head de l'enregistrement arbitré) : « reprise impossible sans
  re-verification du monde » (H5/N-17) ;
- journal de conflits : rejoué à l'identique, 0 ligne dupliquée (idempotence).

## 3. Articulation, divergence et reprise (H5 : N-16/N-17)

Divergence simulée entre copie locale et copie de référence d'un enregistrement de reprise
(champ `round`) : détectée (`champs_divergents`), arbitrée par « github fait foi » (N-16),
consignée au journal (`journal_conflits`), puis reprise exécutée uniquement avec le head
vérifié ; refusée avec head divergent (N-17). L'enregistrement de reprise complet de la
mission (branche `mission/coord-pilot-01`, base `a803b3c5…`, prochaine action
« réception des livrables par OpenAI ») est consigné dans la trace.

## 4. Reprise après interruption (REEL, survenue en séance)

Au démarrage de la séance, le sandbox avait été réinitialisé : la commande `chatgpt` était
absente. Le pont s'est auto-réparé (`setup.sh`, dossier de connaissances persistant), la
session `m-coord-pilot-01` a repris sans perte de contexte, et le cycle de plan (v1
critiqué → v2 accepté) s'est poursuivi sans reprise de zéro. C'est une démonstration réelle
de reprise après interruption hors du moteur (infraçstructure), distincte de la reprise
in-engine (§3) : les deux niveaux ont fonctionné.

## 5. Séparation des rôles (REEL, appliquée au cycle)

- EXECUTANT : Mistral (Vibe), identité `mistral-vibe`, adaptateur `mistral-sandbox` ;
  a produit le plan, l'a corrigé, a publié l'ACK.
- RECEPTIONNAIRE : OpenAI (ChatGPT), identité `chatgpt-pont`, adaptateur `openai-pont` ;
  a critiqué (A_CORRIGER, 5 points) puis accepté (PLAN_ACCEPTE) le plan via la session
  pont. Jamais exécutant et réceptionnaire simultanément sur la même mission (N-05·6.3).
- PILOTE : Denis (humain), s'est prononcé par l'issue #25 (ordre de mission, périmètre,
  interdits) ; la clôture lui reste réservée. L'humain arbitre.

## 6. Preuves d'exécution des batteries (REEL, sans régression)

Exécutées depuis la copie de travail de `main` (`a803b3c5…`), périmètre non modifié :

- Python : batterie v1.5 durcie — **47/47 PASS** (exit 0, `tests_coord06_v15.py`) ;
- Go 1.27.4 linux/amd64 (toolchain officiel téléchargé en séance) : `go vet ./...` OK,
  `go build ./...` OK, `go test ./...` OK (6 paquets testés) ;
- vecteurs de conformité : **22/22 OK**, sortie byte-identique à la ligne de base de la
  mission #23 — sha256 `8c097f7672d5a031956b9f02d5162c04de411d41985bf62e233c108e53388f96` ;
- démonstration autonome : **19 CONFORME / 0 ECART**, exit 0, byte-identique — sha256
  `e1f1cdbed291de8e03e8f5ebe3ae20a452b600f87275d92eb95269d3d671a6f6` ;
- maquette : contrôles statiques **12/12 PASS**
  (`preuves/recette-maquette.resultat.txt` reproduit par `maquette/test-maquette.py`).

## 7. Réserves et distinctions REEL / SIMULÉ / NON_EXÉCUTÉ

- **REEL** : moteur Python et Go, batteries, vecteurs, démo, pilote d'expérimentation,
  auto-réparation du pont, critiques et verdicts de plan (session pont), commentaires
  publiés dans #25.
- **SIMULÉ** : adaptateurs qui qualifient les identités côté moteur (le moteur
  n'authentifie pas, spec §3/§11) ; aucune interface COORD Pilot logicielle n'existe
  encore : la maquette HTML seule est livrée, ses données sont simulées et explicites.
- **NON_EXÉCUTÉ** : rendu navigateur de la maquette (pas de navigateur dans le sandbox ;
  à la charge du pilote en local) ; tests multi-OS du Go (linux/amd64 seul) ; tout
  déploiement VPS, achat, appel payant. PR #24 volontairement non touchée.
- Résidu : le toolchain Go téléchargé (70 Mo) est un outil de séance, il n'est ni livré
  ni requis pour rejouer le pilote d'expérimentation (Python seul suffit).

## 8. Conclusion

Le cycle de cette mission s'est déroulé sous le protocole v1.5 de bout en bout : soumission
avec preuves, critique motivée, corrections, acceptation avec référence publiée, ACK,
enregistrement d'état à ACK_RECU, sans clôture (réservée au pilote). Les invariants
sollicités par l'issue (N-05, N-09, N-10, N-12, N-13, N-14, N-15, N-16, N-17, N-19, N-20
via les contrôles de la batterie et du pilote) ont été vérifiés REEL sur le moteur,
sans modification d'aucun fichier du périmètre publié. v1.5 reste candidate non activée.

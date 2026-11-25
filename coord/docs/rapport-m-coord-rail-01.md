# Rapport de mission M-COORD-RAIL-01 — Rail commun API/A2A multi-LLM

- Mission : M-COORD-RAIL-01 (issue #50, ordre de mission commentaire 7175583376)
- Exécutant : Mistral (Vibe). Critique et réceptionnaire indépendant : OpenAI (ChatGPT).
- Pilote : Denis Bouzon. Fusion et clôture réservées au pilote.
- Base : `main` f51bc8a5… Branche : `mission/coord-rail-01`.
- Budget : 0 €. Aucun appel API payant, aucune qualification réelle des quatre fournisseurs présupposée.

## 1. Inventaire et décision d'architecture (T1)

Inventaire du rail A2A issu de la mission #48 : le rail vivait dans `coord/go/internal/a2a` (rail.go, factories.go, adaptateur.go) avec le SDK externe a2a-go v2.9.0 comme dépendance directe de l'orchestrateur.

Décision du pilote (consignée, commentaire 7175791250 d'arbitrage) : **Option B, module autonome**. Le rail est extrait dans un module Go séparé et autonome `coord/a2a` (go.mod `github.com/dbouzon/osnias-clearing/coord/a2a`), dont l'unique dépendance directe est a2a-go v2.9.0. L'orchestrateur `coord/go` ne dépend du SDK qu'indirectement, via un `replace` local vers le module. Le pont `coord/go/internal/a2a` est réduit à un adaptateur plus sa batterie de tests, sans aucun import du SDK hors de cet adaptateur.

Limite déclarée : l'isolation obtenue est une isolation de périmètre source et CI (frontière de module, go.mod/go.sum propres, workflow CI dédié), **pas** une isolation du binaire : les binaires de l'orchestrateur embarquent toujours le même code rail par liaison statique usuelle en Go.

## 2. Corrections et évolutions (T2)

- Migration byte-exact de rail.go et des fabriques dans `coord/a2a/rail.go` et `factices.go`, plus accès publics purement additifs : `Chemin()`, `EtatDepuisA2A()`, `JetonsFactices()`, alias `Executable = a2asrv.AgentExecutor`, décorateur `CompteurEnvois`.
- `secretRE` de vérification étendu : `(sk-[A-Za-z0-9_\-]{8,}|AIza[0-9A-Za-z_\-]{35})`. Reproduction du FAIL observé avant correction, PASS ensuite (jamère de reproduction respectée).
- NouvelleMissionExplicit rendu additif et fail-closed : adaptateur inconnu refusé avec `ADAPTATEUR_INCONNU` ; comportement historique inchangé sinon.
- Rôles explicites : tests `roles_test.go` (R1..R3). R3 fige le comportement historique : avec trois adaptateurs ou plus, le récepteur était qualifié par le second adaptateur trié (défaut `ids[:2]`), et R3 verrouille ce comportement documenté.
- CI dédiée `.github/workflows/coord-a2a-ci.yml` : Linux/macOS/Windows, avec `-race`.

## 3. Matrice d'interopérabilité (T4) — SIMULÉ

`coord/go/internal/connecteurs/interop_test.go`, `TestMatriceInteropDouzeCouples`. Quatre fournisseurs factices en boucle locale : banc-anthropic (Messages API, end_turn), banc-gemini (profil gemini, stop-only), banc-mistral, banc-openai (format openai, finish_reason "stop"). Les 12 couples ordonnés passent un cycle complet : NouvelleMissionExplicit → Executer → Soumettre → Recevoir → qualification ACCEPTEE ancrée → Acker → ACK_RECU, clôture refusée après ack. PASS (0,41 s).

Note de qualification : le profil gemini stop-only exige finish_reason "stop" dans les factices au format openai, faute de quoi l'échange simulé est refusé. Piège rencontré et corrigé pendant T4.

## 4. Résilience (T4) — REEL (sandbox)

- TI-06b (`coord/a2a/ti06b_test.go`) : interruption réelle du processus. Le binaire de test enfant se ré-exécute (COORD_A2A_PROBE=1), délègue, marque son stdout "PROBE_DELEGE", puis reçoit SIGKILL. À la réouverture du registre : tâche incertaine, doublon refusé par `ErrDoublon` sans aucun réseau, n==1 envoi, `ResoudrePilote` aboutit. PASS.
- TI-07 : déjà couvert côté engine par `TestReponseTardiveArretPendantAppel` (orchestre_test.go:314) — réponse produite après arrêt du contexte pendant l'appel. Cité, non dupliqué.

## 5. Preuves de rejeu — REEL (sandbox)

- Module `coord/a2a` : 8 tests ok (dont M1..M6 et TI-06b).
- Engine `coord/go` : 13 paquets ok, build `./...` 2,66 s.
- Batteries Python v1.5 : 47/47 (M1 PASS).
- Matrice interop 12 couples : PASS.
- Fidélité : chaque état poussé est vérifié par tarball codeload épinglé au SHA (empreinte sha256) puis rejeu des batteries depuis le code distribué.

## 6. Mesures (T4) — REEL (sandbox), NON_EXÉCUTÉ sur VPS

Harnais jetable (module de mesure hors livrable, loopback, 0 €), 50 délégations-exchanges complètes contre un agent factice :

| Mesure | Valeur |
| --- | --- |
| Latence médiane | 3,02 ms |
| Latence p95 | 3,82 ms |
| Latence min / max | 2,30 ms / 5,03 ms |
| RSS du client | ~13,2 MiB |
| Taille du registre après 50 échanges | 10 771 octets (sha256 e75c2d8f…) |

Tailles des binaires : coord-demo 4 832 097 o ; coord-mission 11 202 474 o ; coord-pilot 12 867 974 o ; coord-mcp 14 155 352 o.

Statut : mesures REELLES mais obtenues dans le sandbox ; toute mesure sur VPS de production est NON_EXÉCUTÉE et à qualifier avant exploitation.

## 7. Statuts et frontières

- REEL : rejeus des batteries Go et Python, TI-06b (SIGKILL réel), mesures sandbox, empreintes tarballs.
- SIMULÉ : matrice 12 couples (fournisseurs factices, boucle locale), démonstrations E2E.
- NON_EXÉCUTÉ : qualification réelle des quatre fournisseurs, mesures VPS. Aucun appel payant.

## 8. Livraison

Branche `mission/coord-rail-01`, PR vers `main` avec empreintes SHA et historique complet (T1 ACK, baseline, PLAN_ACCEPTE, arbitrages, T3 CONTINUE, T4/T5). La fusion, la clôture de #50 et tout arbitrage restent réservés au pilote Denis Bouzon.

---

Copyright © 2026 Denis Bouzon — ORCID https://orcid.org/0009-0007-8894-8902 — SPDX: Apache-2.0

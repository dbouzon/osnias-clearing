# RAPPORT — M-COORD-LLM-02 : connecteur Gemini + rail A2A direct

Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX : Apache-2.0.

Mission : issue #48 (repo Osnias Clearing). Base : `main` @ `161cb304` (tarball codeload épinglé, sha256 `afaca4dd…`). Référence opérationnelle : COORD v1.5. Réception : OpenAI (ChatGPT), réceptionnaire indépendant.

## 1. Livrable

- **Profil `gemini`** (`coord/go/internal/connecteurs/gemini.go`) : format de requête compatible OpenAI, `raisonnement` borné {none, minimal, low, medium, high} (défaut `low` journalisé, inconnu refusé au chargement), **finish stop-only** (tout autre motif : refus `FIN_NON_NOMINAL`/`GEMINI_SANS_CANDIDAT`), modèle exact requis (`MODELE_DIVERGENT` inchangé), voûte monétaire partagée (`EnveloppeCentimes`/`CoutConservateurJetons`).
- **Rail A2A direct** (`coord/go/internal/a2a/`) : `rail.go` (délégation avec capture du résultat à l'envoi, registre JSON durable atomique 0600, jeton `X-Coord-Rail-Jeton`, loopback imposé, `ResoudrePilote` pour résolution de tâche incertaine réservée au pilote), `adaptateur.go` (Adaptateur + ConnecteurReel, boucle d'attente bornée 20 s, erreurs `RESULTAT_NON_TRANSMIS`/`TACHE_TERMINALE_NEGATIVE`/`SAISIE_ATTENDUE`/`BORNE_ATTEINTE`), `factices.go` (agents A2A factices loopback, carte A2A standard, jeton 401, contrat d'artefact `{"jetons":N,"texte":"…"}`).
- **Recette** : 11 tests connecteur Gemini (C1–C9, C14 plafond avant réseau, C15 verdicts piégés jamais committés) + 12 tests rail/démo (E1–E10 + cycle complet COORD réel : critique de plan via A2A → exécution → A_CORRIGER → correction → ACCEPTEE → ACK → clôture pilote).
- **CI** : job race étendu à `./internal/a2a/` ; `github.com/a2aproject/a2a-go/v2 v2.9.0` en dépendance directe.
- **Docs** : `connecteur/Gemini/` (README, exemple de config SANS SECRET, procédure de recette réelle NON EXÉCUTÉE, présent rapport FR/EN).

## 2. Arbitrages (plan v2, PLAN_ACCEPTE)

A1 : format `openai` + profil `gemini`, stop-only ; API native documentée NON EXÉCUTÉE. A2 : raisonnement borné. A3 : modèle exact + `MODELE_DIVERGENT`. A4 : CLI/MCP/frontend hors tranche. Q3 : rail dans `coord/go/internal/a2a` (a2a-go v2.9.0). Q4 : résolution de tâche incertaine = pilote exclusif (SEC-01), aucune automatisation.

## 3. Qualification SIMULÉE (0 €)

- Go : `go vet ./...` propre ; `go test ./... -count=1` **tous packages ok** (63 tests connecteurs anciens + 11 Gemini + 12 a2a).
- Non-régression : vecteurs `conformite-v15` **22/22**, sha de bilan `8c097f76…` ; démo `coord-demo` **19 CONFORME / 0 ECART**, sha `e1f1cdbe…` (identiques à la ligne de base) ; batteries Python **135 PASS** (24+10+13+11+13+17+47) ; `coord/exp` tests + conduite **PASS**.
- Mesures (sandbox, Go 1.27.4) : build complet ~2,8 s ; binaire démo ~4,8 Mo ; RSS max démo ~10,6 Mo ; suite a2a ~1,6 s. Compatible cible VPS 2 Go.

## 4. Réserve

**Recette réelle modèle/echo NON EXÉCUTÉE** (budget 0 €) : procédure et conditions sur `recette-reelle-gemini.md` (autorisation écrite pilote, plafond armé, requêtes empreintées). Rappel : la vérification fine de version (`modelVersion`) relève de l'API native, hors tranche.

## 5. Décision

Fusion de la branche `mission/coord-llm-02` et clôture de l'issue #48 : **réservées au pilote**.

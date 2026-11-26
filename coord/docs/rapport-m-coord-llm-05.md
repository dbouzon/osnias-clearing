# Rapport de mission — M-COORD-LLM-05 : intégration de Claude Code au serveur agent A2A commun

Copyright © 2026 Denis Bouzon · ORCID <https://orcid.org/0009-0007-8894-8902> · Licence Apache-2.0

Mission : issue [#58](https://github.com/dbouzon/osnias-clearing/issues/58) (COORD v1.5)
Exécutant : Mistral · Direction technique, critique et réception : OpenAI · Pilote : Denis Bouzon
Branche : `mission/coord-llm-05` · Départ : main `34baff2c` (enfant direct du merge PR #57, tarball épinglé `c07af05a…`)

## 1. Résumé

Le connecteur Anthropic est passé **deux voies** : la voie API d'inférence
EXISTANTE est INCHANGÉE octet pour octet (régression interdite par
l'ordre) et une nouvelle voie **Claude Code supervisée** est raccordée
au même serveur agent A2A, pilotée par le marqueur de charge
`VOIE:CODE`. Le superviseur **réutilise TEL QUEL** le runner générique
de M-COORD-LLM-03 (liste blanche d'environnement, expurgation des
secrets, SIGTERM coopératif puis SIGKILL du groupe, codes 0/1/2/3) :
aucune duplication. La qualification est SIMULÉE de bout en bout via la
CLI factice `claude-factice` (contrat `claude -p --output-format
json`) : **0 EUR, aucune clé, aucun réseau fournisseur, aucune écriture
GitHub**. Les trois points bloquants de la critique OpenAI du plan
(verrouillage des sessions, erreurs de validation explicites, contrôles
de permissions) sont traités nominalement et testés. La qualification
RÉELLE demeure NON_EXÉCUTÉE (recette distincte, triple verrou).

## 2. Décisions et contrôles

- **Plan** : tour 1 A_CORRIGER (3 bloquants), plan v2 consolidé,
  **tour 2 PLAN_ACCEPTÉ** (session pont `m-coord-llm-05`, plan figé
  sha256 `0682005d…`). T3 contrôle intermédiaire : **CONFORME**.
- **Honnêteté des cartes** : l'agent anthropic déclare `coord-inference`
  ET `coord-claude-code` ; les autres fournisseurs déclarent toujours
  l'inférence seule. `OptionsServeur.Skills` est ADDITIF (vide = défaut
  rétrocompatible).
- **Parité avec Mistral Vibe sans alignement artificiel** : même
  squelette d'agent deux voies et même runner, mais les contrats propres
  de Claude Code sont respectés (sessions = `--resume <session_id>` du
  CLI, permissions = `dontAsk` + `--allowedTools` + deny `--settings`,
  coût = `total_cost_usd` estimé côté client).

## 3. Livrables (branche `mission/coord-llm-05`)

| Fichier | Nature | Contenu |
| --- | --- | --- |
| `coord/a2a-agent/claudecode/claudecode.go` | NOUVEAU | `ExecClaudeCode` (voie VOIE:CODE ; MISSION/REVISION/CORRECTION/TACHE), `Agent` deux voies (routage de DONNÉES), validation stricte de la sortie CLI, écriture + validation du settings.json, HOME réécrit, budget local `--max-budget-usd` |
| `claudecode/registre.go` | NOUVEAU | Registre de sessions VERROUILLÉ : verrou fichier exclusif NON BLOQUANT (reprises bornées 200×5 ms), écriture atomique (temp+rename) STRICTEMENT post-validation, fail closed `REGISTRE_VERROU_INDISPONIBLE` / `SESSION_INCOHERENTE` / `REGISTRE_CORROMPU` |
| `claudecode/verrou_posix.go` / `verrou_windows.go` | NOUVEAU | flock POSIX / LockFileEx Windows (builder tags, y compris go.sum complété) |
| `claudecode/claudecode_test.go` | NOUVEAU | Recette CC1..CC21 + CC-CONCUR1/2 + CC-REINTEG (détail §5) |
| `cmd/claude-factice/main.go` (+ `signaux_posix.go`/`signaux_windows.go`) | NOUVEAU | CLI factice imitant `claude -p --output-format json` : champs result/session_id/is_error/total_cost_usd/num_turns/environment ; marqueurs SCENARIO: dans le prompt (BASE, ERR_IS_ERROR, JSON_INVALIDE, VIDE, SESSION_ABS, PERM_REFUS, BUDGET, FUIE, HANGUE, LENTE, HOSTILE, QUITTE1/2/3, OUTIL:, LIVRABLE_A_CORRIGER) ; état de session et compteur d'appels dans le HOME réécrit ; contrôles embarqués (dontAsk exigé, deny requis, tool hors liste refusé) |
| `cmd/coord-agent/claudecode_agent.go` | NOUVEAU | `optionsClaudeCode` (build de la CLI factice, workspace dédié, registre), skills honnêtes, démo `-demo-claudecode` A2A reproductible |
| `cmd/coord-agent/agents_llm04.go` | MODIFIÉ (additif) | `-fournisseur anthropic` passe DEUX VOIES (inférence inchangée + VOIE:CODE) ; les autres fournisseurs inchangés |
| `cmd/coord-agent/main.go` | MODIFIÉ (additif) | Drapeau `-demo-claudecode` ; démos existantes inchangées |
| `agents/serveurs.go` | MODIFIÉ (additif) | `OptionsServeur.Skills` optionnel (carte honnête paramétrable, rétrocompatible) |
| `coord/go/internal/a2a/claudecode_llm05_test.go` | NOUVEAU (moteur) | CC15 : mission simulée Claude Code exécutant / OpenAI réceptionnaire, boucle A_CORRIGER complète, sur les VRAIS serveurs en sous-processus |
| `coord/go/internal/a2a/agents_llm04_test.go` | MODIFIÉ | Lanceur : répertoire de travail = module agent (l'agent anthropic construit sa CLI au démarrage) |
| `coord/connecteur/Claude/exemple-config-claudecode.json` | NOUVEAU | Configuration d'exemple SANS secret, mode SIMULÉ, triple verrou documenté |

## 4. Sécurité (fail closed, motifs nominaux)

1. **Permissions, triple contrôle** : superviseur AVANT lancement (seul
   `dontAsk` admis, `MODE_PERMISSION_REFUSE`, bypassPermissions interdit
   par l'ordre) ; settings.json ÉCRIT puis VALIDÉ par relecture (deny
   réseau obligatoires `WebFetch`/`WebSearch`/`WebRequest`, allow ⊆
   liste blanche sinon `REGLE_PERMISSION_INCONNUE`) ; contrôle embarqué
   du factice (refuse tout autre mode, refuse un tool hors
   `--allowedTools`, refuse un settings sans deny).
2. **Validation de sortie, motifs constants par cas** :
   `RESULTAT_NON_JSON`, `RESULTAT_ECHEC:<code>`, `IS_ERROR_VRAI:<motif>`,
   `SESSION_ID_ABSENT`, `RESULT_VIDE`, `COUT_INCONNU` (jamais
   inconnu=gratuit), `RESULTAT_COUT_HORS_BORNE`, `RESULTAT_TROP_GRAND`,
   `ENV_HORS_LISTE_BLANCHE`. Tout échec de validation n'ouvre AUCUNE
   session (un run raté ne peut jamais être repris).
3. **Isolation** : un espace de travail dédié par mission, HOME réécrit
   (aucune lecture du `~/.claude` réel, aucune credential), liste
   blanche d'environnement EXACTE vérifiée sur la
   déclaration d'environnement renvoyée par la CLI ([HOME, PATH] ; sous
   Windows le runtime Go ajoute SYSTEMROOT, attendu nommément ; toute
   divergence
   est refusée, motif `ENV_HORS_LISTE_BLANCHE`).
4. **Secrets** : expurgation runner conservée (sk-, xoxb-, etc.) ;
   aucun secret dans livrables, registre, settings ou configurations
   d'exemple (vérifié par la recette).
5. **Sessions** : verrou exclusif non bloquant à reprises bornées ;
   écriture atomique post-validation ; défauts nominaux
   (`REGISTRE_VERROU_INDISPONIBLE`, `SESSION_INCOHERENTE`,
   `REGISTRE_CORROMPU`).
6. **GitHub** : aucune écriture possible (aucun remote, aucun argument
   réseau, deny réseau) ; auto-réception interdite par la machine
   normative (inchangée), vérifiée par CC15 (le réceptionnaire est un
   agent DISTINCT et le verdict est ancré).

## 5. Recette exécutée (sandbox, Go 1.27.4, Linux)

Paquet `claudecode` (5/5 paquets module ok) : CC1 nominal (session
enregistrée, compteur 1) ; CC2 correction même session (`--resume`,
compteur 2) + reprise sans session REFUSÉE ; CC3 `IS_ERROR_VRAI` sans
session ouverte ; CC4 `RESULTAT_NON_JSON` ; CC5 `RESULT_VIDE` ; CC6
`SESSION_ID_ABSENT` ; CC7 refus de permission (`PERM_REFUS` et
`OUTIL:WebFetch`, code 2) ; CC8 budget (code 3) + `RESULTAT_COUT_HORS_BORNE`
côté options ; CC9 annulation `ANNULE` (aucune session) + arrêt forcé du
groupe sur `HANGUE` (`ARRET_FORCE` borné, POSIX) ; CC10 hostile endigué
(données, jamais exécutées) ; CC11/CC12 secrets `sk-`/`xoxb-` expurgés
(livrable, registre et settings nets) ; CC13 deny réseau présents ;
CC14 liste blanche ENV exacte, fuite `GH_TOKEN` REFUSÉE ; CC19 le
factice refuse `bypassPermissions` ; CC20 allow hors blanche REFUSÉ ;
CC-CONCUR1 (2 goroutines × 20 écritures : 40/40, zéro perte) ;
CC-CONCUR2 (verrou tenu : REFUS borné, jamais bloqué) ; CC-REINTEG
(registre corrompu : REFUS explicite).
CC15 (moteur, E2E) : mission simulée **Claude Code exécutant / OpenAI
réceptionnaire** sur les vrais serveurs en sous-processus — exécution
VOIE:CODE (run 1, livrable portant le marqueur A_CORRIGER) → réception
A_CORRIGER → EN_CORRECTION → correction sur la MÊME session (run 2,
marqueur retiré) → ACCEPTEE (verdict ancré, verdict_id/verdict_url) →
ACK_RECU ; clôture `CLOTURE_REFUSEE` hors pilote. PASS.
Non-régressions : matrice 12 couples LLM-04 12/12 PASS (lanceur
corrigé), moteur 13/13 paquets, module agent 5/5, coord/a2a vert,
vecteurs de conformité 22/22, démos `-demo-local`, `-demo-llm04` et
`-demo-claudecode` identiques et REJOUÉES (2×).
Multi-OS : `GOOS=windows go vet` + build OK (go.sum complété pour
golang.org/x/sys/windows).

## 6. Tableau comparatif Mistral / Anthropic (voies CLI supervisées)

| Aspect | Mistral — voie Vibe CLI | Anthropic — voie Claude Code |
| --- | --- | --- |
| Charge | `VOIE:VIBE MISSION= REVISION= CORRECTION= TACHE=` | `VOIE:CODE MISSION= REVISION= CORRECTION= TACHE=` (même gabarit) |
| Exemple d'exécution | worktree déterministe, `--prompt-file`, `--max-tokens` bornant l'enveloppe AVANT lancement | workspace dédié + HOME réécrit, `-p`, `--max-budget-usd` borne locale |
| Sessions | session par révision, `--resume` de la session Vibe | même clé (mission, révision), `--resume <session_id>` CLI, registre VERROUILLÉ |
| Exemple d'erreur | `EXPORT_MANQUANT` → enveloppe TOTALE (jamais inconnu=gratuit) | `COUT_INCONNU` → REFUS, aucune session ouverte (même principe, contrats distincts) |
| Permissions | liste blanche env + codes 0/1/2/3 + sans remote | idem + triple contrôle dontAsk (mode, settings, embarqué) |
| Annulation | SIGTERM → SIGKILL du groupe (runner commun) | idem, runner commun réutilisé tel quel |
| Comptabilité | jetons relevés dans export.json | jetons déclarés (800/run simulé) + `total_cost_usd` estimé CLI recoupé par `--max-budget-usd` ; la voûte COORD (TX1/TX2) reste l'AUTORITÉ |

Le README du paquet et le présent rapport documentent chaque motif
d'erreur (cause, remède) et chaque marqueur SCENARIO:.

## 7. Mesures

E2E CC15 : 1,9-2,2 s (build binaire + 2 agents + boucle complète).
Binaire `coord-agent` 12,98 Mo ; `claude-factice` 3,98 Mo ; agent deux
voies en écoute ≈ 8,1 Mo RSS. Synchrone inchangé (exécution dans
`message/send`) : l'asynchronisme A2A reste hors périmètre (extension de
contrat à valider), la reprise d'une tâche incertaine reste au PILOTE.

## 8. Limites documentées

1. **Contrat factice volontairement minimal** : la CLI factice imite le
   contrat documenté de Claude Code (`-p`, JSON, sessions, permissions,
   budget) mais n'est PAS Claude Code ; la qualification réelle suivra
   la recette DISTINCTE : triple verrou (autorisation écrite du
   pilote, clé `ANTHROPIC_API_KEY` du trousseau processus jamais
   versionnée, compte dédié), plafonds bas, B1..B5 comme pour Mistral.
   NON_EXÉCUTÉE.
2. **Si une limite budgétaire ou une isolation suffisante ne pouvait
   être garantie en réel, la voie réelle resterait désactivée** (ordre
   §4).
3. **Windows/HANGUE** : le test de l'arrêt forcé par signaux est écarté sous Windows (talons de compilation) ; Windows reste
   couvert par `taskkill /T` (S15, runner commun).
4. `stream-json` non requis en synchrone ; le coût CLI est une
   estimation client, recoupée par la voûte COORD (autorité).

## 9. État final et suite

CC1..CC21 + concurrence + E2E : TOUT PASS en local (Linux) ; CI 3 OS +
race confirmée sur la PR (T5). Une seule PR vers `main`, rapports FR/EN,
réception indépendante OpenAI au SHA exact consignée dans l'issue #58.
Fusion et clôture réservées au pilote (Denis Bouzon). La qualification
réelle demeure une recette DISTINCTE sur autorisation écrite du
pilote.

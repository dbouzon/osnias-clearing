# claudecode — voie Claude Code supervisée (M-COORD-LLM-05)

Agent A2A Anthropic **deux voies** : inférence (inchangée, M-COORD-LLM-04)
+ exécution outillée **Claude Code supervisée**, routée par le marqueur de
charge `VOIE:CODE MISSION=<m> REVISION=<r> CORRECTION=<0|1> TACHE=<texte>`.
Qualification SIMULÉE via `cmd/claude-factice` : 0 €, aucune clé, aucun
réseau fournisseur, aucune écriture GitHub. Qualification réelle = recette
DISTINCTE (triple verrou, autorisation écrite du pilote), NON_EXÉCUTÉE.

## Frontières

- runner générique réutilisé tel quel (env liste blanche, expurgation,
  SIGTERM→SIGKILL du groupe, codes 0/1/2/3) ;
- workspace dédié par mission, HOME réécrit, registre de sessions
  VERROU par fichier exclusif (écriture atomique post-validation) ;
- seules permissions `dontAsk` + `--allowedTools` ⊆ liste blanche +
  deny réseau obligatoires ; `bypassPermissions` interdit ;
- la voûte monétaire vit côté orchestrateur COORD : `--max-budget-usd`
  n'est qu'une borne locale du run.

## Motifs d'erreur (cause → remède)

| Motif | Cause | Remède |
| --- | --- | --- |
| `OPTIONS_INCOMPLETES` | binaire, racine, registre, durée, budget, outils ou taille max absents | déclarer toutes les options bornées |
| `MODE_PERMISSION_REFUSE` | mode autre que `dontAsk` demandé | verrouiller `dontAsk` (bypassPermissions interdit) |
| `REGLE_PERMISSION_INCONNUE` | allow hors liste blanche ou deny manquant dans settings | réduire les allow à la liste blanche, garder les deny réseau |
| `RESULTAT_NON_JSON` | sortie CLI non JSON | livrable invalide, rejouer le run |
| `RESULTAT_ECHEC:<code>` | code de sortie CLI 1/2/3 | voir le stderr expurgé du run |
| `IS_ERROR_VRAI:<motif>` | la CLI signale `is_error` | corriger la tâche, relancer |
| `SESSION_ID_ABSENT` | `session_id` manquant | sortie non conforme, rejouer |
| `RESULT_VIDE` / `RESULTAT_TROP_GRAND` | result vide ou hors enveloppe | borne `ResultatMax` |
| `COUT_INCONNU` | `total_cost_usd` absent/négatif | jamais inconnu=gratuit : refus |
| `RESULTAT_COUT_HORS_BORNE` | coût > budget local | augmenter `BudgetUSD` (voûte COORD = autorité) |
| `ENV_HORS_LISTE_BLANCHE` | environnement reçu ≠ [HOME, PATH] | isolation rompue : fuite à corriger |
| `SESSION_INCOHERENTE` | reprise demandée sans session valide | session absente ou divergente |
| `REGISTRE_VERROU_INDISPONIBLE` / `REGISTRE_CORROMPU` | verrou tenu / fichier corrompu | contention bornée ; fichier à réparer |
| `ANNULE` / `ARRET_FORCE` | annulation / limite de durée | reprise = décision PILOTE |

## Marqueurs SCENARIO: du factice (dans la TACHE)

`BASE`, `ERR_IS_ERROR`, `JSON_INVALIDE`, `VIDE`, `SESSION_ABS`,
`PERM_REFUS`, `BUDGET`, `FUIE`, `HANGUE` (POSIX), `LENTE`, `HOSTILE`,
`QUITTE1/2/3`, `OUTIL:<nom hors liste>`, `LIVRABLE_A_CORRIGER`
(livrable portant le marqueur `SCENARIO:A_CORRIGER`, pour la boucle de
réception CC15).

## Recette

`go test ./claudecode/` : CC1..CC21 + CC-CONCUR1/2 + CC-REINTEG.
E2E moteur : `coord/go/internal/a2a/claudecode_llm05_test.go` (CC15).
Démo : `coord-agent -demo-claudecode` (rejouable) ; `-demo-local` et
`-demo-llm04` inchangées.

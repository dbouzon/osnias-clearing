# Recette finale M-AIC-06 — Dialogue interactif continu (issue #10)

Date : 22 novembre 2026
Branche : mission/aic-06-dialogue-continu (base au démarrage de la recette : ec4faf9e99184923419cc2bea6405ddbbcbadf2e)
Conclusion : **PASS**

## Objectif

Valider une conversation interactive entre l'API Mistral et l'API OpenAI via Osnias AI Coordinator : trois tours successifs dans une seule session `aic-chat`, sans relancer le programme.

## Exécution

Commande : `aic-chat -session aic-06-recette -db recette.sqlite`, environnements AIC_MISTRAL_APIKEY et AIC_OPENAI_APIKEY fournis uniquement par variables d'environnement.

Trois tours exécutés dans la même session, un seul lancement du programme :

- Tour 1 (`aic-06-recette-t001`) : question de débat sur la compensation multilatérale ; réponse réelle de Mistral (analyse structurée, environ 600 mots) ; verdict OpenAI réel : CORRECTION, résumé « Ajouter une conclusion succincte », un commentaire.
- Tour 2 (`aic-06-recette-t002`) : demande d'un avantage concret ; réponse réelle de Mistral en une phrase ; verdict OpenAI réel : ACCEPTED.
- Tour 3 (`aic-06-recette-t003`) : demande d'une conclusion finale ; réponse réelle de Mistral ; verdict OpenAI réel : ACCEPTED.

Continuité vérifiée : les trois tours partagent la session, les compteurs progressent (1/20, 2/20, 3/20 tours ; 1200, 2400, 3600 jetons pire cas sur 20000), arrêt par commande `exit`, code de sortie 0, message « session close par commande. ». Les verdicts des deux niveaux (CORRECTION puis ACCEPTED) démontrent que chaque tour traverse bien les deux agents réels et que le verdict n'est pas un texte fixe.

## Vérifications

- Journalisation SQLite (base recette.sqlite) : 21 entrées sous la mission, soit exactement trois cycles complets de sept phases chacun (relay/receive, mistral/call, mistral/response, relay/mistral, openai/call, openai/response, relay/openai). Chaque tour possède sa ligne relay/receive avec son message_id (t001, t002, t003).
- Idempotence : table events, trois entrées `aicmsg:aic-06-recette-t00X`, toutes marquées traitées.
- Absence de divulgation des clés : requêtes de la base sur les motifs `mstrl_` et `sk-proj-` : zéro occurrence ; la sortie console ne contient aucune clé. Les clés sont passées par environnement de processus uniquement, jamais écrites sur disque.
- Arrêt propre : fin par commande `exit` (exit 0). Le chemin Ctrl+C (fermeture de la base puis exit 0) est couvert par les tests internes ; il peut être confirmé sous Termux à la discrétion du responsable.

## Limites et niveau de preuve

- Aucune modification du Banking Core ni des SIDIRS ; code exclusivement réutilisé depuis la branche M-AIC-06 existante ; PR #11 conservée en Draft, aucune fusion.
- Les réponses des modèles sont des réponses de modèles, non des faits techniques ; la recette valide le fonctionnement du mécanisme de dialogue, pas le contenu des réponses.
- Interopérabilité réelle Mistral et OpenAI démontrée en environnement de test (niveau 3 du §11) ; aucune homologation bancaire ni qualification n'est revendiquée.
- `go test -race` reste non exécutable dans la sandbox (cgo) : la suite de tests de la branche (60 tests simulés) reste la preuve interne de niveau 1, déjà publiée.

## Git

Publication du présent compte rendu sur la branche mission/aic-06-dialogue-continu uniquement (PR Draft #11 inchangée). SHA vérifié après push, consigné dans le PR.

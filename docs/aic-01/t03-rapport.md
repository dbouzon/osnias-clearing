# Rapport de réception — M-AIC-01 / T03 : intégration GitHub réelle

Version 1.0 — 22 novembre 2026 · Développeur : Mistral Work (Vibe) · Réception : ChatGPT / agent OpenAI · Responsable : Denis Lamalice

## 1. Objectif

Remplacer l'adaptateur GitHub simulé par une intégration réelle, sécurisée et testable, sans modification de l'architecture du coordinateur validée en T02, en réutilisant la bibliothèque google/go-github (recommandation du scénario B de l'audit T02-R5). Trois sous-objectifs : T03-A validation de concurrence ; T03-B adaptateur GitHub réel désactivé par défaut ; T03-C maîtrise des doublons d'opérations distantes.

## 2. Références Git

- Départ : branche `mission/ai-coordinator-v01`, commit de référence `40513121ab461a218627e30502e42b05f9c5eff3` (clôture T02-R5).
- Publication : commits `aic(t03): ...` sur la même branche ; SHA distants vérifiés après push (section 7).
- PR Draft #4 maintenue en Draft, aucune fusion.

## 3. Fichiers livrés

| Fichier | Objet |
| --- | --- |
| tools/ai-coordinator/github_real.go | Adaptateur RealGitHub (go-github v93) : GitHubClient + IdempotentGitHub, write-ahead, recherche avant création, branche déterministe |
| tools/ai-coordinator/github_real_test.go | 8 tests contractuels sur serveur HTTP simulé (pannes, ambiguïtés, délais, redémarrages, 422) |
| tools/ai-coordinator/config.go | Champs AIC_GITHUB_MODE (défaut off), AIC_GITHUB_TOKEN, AIC_GITHUB_APIURL, AIC_GITHUB_REPO, AIC_GITHUB_TIMEOUT |
| tools/ai-coordinator/cmd/aic-coordinator/main.go | Câblage : défaut simulateur ; AIC_GITHUB_MODE=real active l'adaptateur réel ; mode inconnu → arrêt en erreur |
| tools/ai-coordinator/go.mod / go.sum | Ajout go-github v93.0.0 ; directive go portée à 1.26.0 (exigée par v93) ; correction d'empreinte strutil (section 6) |
| tools/ai-coordinator/docs/security-report.md | Mesures 21 à 24 et limites T03 |
| docs/aic-01/t03-rapport.md | Présent rapport |

## 4. Conformité à l'ordre de mission

| Exigence | Statut |
| --- | --- |
| T03-A : go test -race | Non exécutable dans l'environnement de développement (détail section 6) ; suite stable sur 3 passes |
| T03-B : go-github, pas de client HTTP spécifique | Conforme : google/go-github v93.0.0, seule dépendance ajoutée |
| T03-B : interfaces GitHubClient et IdempotentGitHub réutilisées | Conforme : RealGitHub réalise les deux interfaces sans modification du coordinateur, du Core, du Manager ou du site |
| T03-B : lecture Issues/commentaires | Conforme : Issues.ListComments, PullRequests.List, PullRequests.ListReviews |
| T03-B : publication commentaires, PR Draft, REQUEST_CHANGES | Conforme : Issues.CreateComment, PullRequests.Create (Draft), PullRequests.CreateReview |
| T03-B : permissions minimales | Conforme par conception : jeton via AIC_GITHUB_TOKEN uniquement, jamais journalisé ni persisté ; lecture/écriture Issues et PR uniquement ; à vérifier sur serveur réel |
| T03-B : aucune fusion automatique | Conforme structurellement : l'adaptateur n'expose ni n'utilise aucune opération de fusion |
| T03-B : désactivé par défaut | Conforme : AIC_GITHUB_MODE=off par défaut ; seul real construit l'adaptateur ; mode inconnu → exit 2 |
| T03-C : clés déterministes, recherche avant création | Conforme : marqueur HTML invisible par clé sur commentaires et revues ; branche de tête déterministe par mission |
| T03-C : reprise après interruption, réponses ambiguës | Conforme : rejeu sans duplication testé (502 après traitement, 500, délais, redémarrage complet, perte du registre local/422) |
| T03-C : exactement-once non revendiqué absolu | Documenté explicitement (security-report mesure 23 et section 6) |
| Aucune API réelle, aucun cloud, aucune dépense | Conforme : essais sur serveur HTTP simulé (httptest) uniquement ; aucun jeton réel ; aucune API payante |

## 5. Mécanisme d'idempotence distante (T03-C)

Chaque écriture (commentaire, revue REQUEST_CHANGES, PR Draft) porte une clé d'idempotence déterministe :

1. Write-ahead (T02-R5) : la clé est persistée dans le registre durable (github_sent) AVANT tout effet ; un échec de persistance interdit l'écriture distante.
2. Recherche avant création : le marqueur HTML invisible `<!-- osnias-aic-idem: CLÉ -->` est recherché dans les commentaires et revues existants ; une PR Draft existante est retrouvée par sa branche de tête déterministe `aic/aic-<mission>/draft`.
3. Effet : une opération déjà réalisée est restituée sans nouvelle écriture ; une erreur 500, un dépassement de délai ou une réponse ambiguë (création traitée puis 502) laissent l'opération rejouable ; le rejeu reconnaît l'effet déjà produit, jamais de doublon.
4. Refus serveur 422 (branche identique) : récupéré par recherche et restitution de la livraison existante, même avec un registre local perdu.

Garantie obtenue : déduplication observée côté GitHub sur l'ensemble des scénarios testés. Limite : cette combinaison n'est PAS une transaction atomique avec l'API distante. En cas d'ambiguïté non résolue (ni marqueur trouvable, ni création confirmée), l'opération reste en erreur et sera rejouée. Aucun exactly-once absolu n'est revendiqué.

## 6. Limites impératives et anomalies

- go test -race : non exécutable dans l'environnement de développement — le détecteur exige cgo et un compilateur C (rejet explicite « -race requires cgo »), aucun compilateur C n'est disponible et l'installation de paquets y est interdite. À exécuter sur machine complète. Mesures compensatoires inchangées : connexion SQLite unique, verrou d'exclusivité persistant, traitement séquentiel par événement.
- Anomalie de dépendance : l'empreinte go.mod du module modernc.org/strutil publiée sur la branche portait un caractère altéré (terminaison « TosXO= » au lieu de « TosX0= »). La valeur conforme au registre de référence sum.golang.org (vérifiée par lookup) a été rétablie dans go.sum. Aucun autre changement de dépendance.
- go.mod : directive portée à go 1.26.0, exigée par go-github v93 ; la chaîne d'outils de l'environnement de développement est 1.26.0.
- Les essais ont été menés sur un serveur HTTP simulé : le contrat go-github et la maîtrise des doublons sont validés sur ce scénario, mais l'interopérabilité complète avec l'API GitHub réelle (pagination effective, limites de débit, verrouillage de revues) n'est pas qualifiée ; environnement GitHub isolé explicitement autorisé requis avant tout usage opérationnel.
- La création d'une PR Draft suppose que la branche de tête déterministe existe côté distant : sa préparation relève du processus de publication autorisé, hors adaptateur ; à défaut, le serveur refuse et l'opération reste en erreur rejouable.
- Permissions du jeton à vérifier sur serveur réel : lecture Issues/PR et écriture Issues/PR uniquement ; pas de permission de fusion.

## 7. Tests

- Commandes : go build ./... ; go vet ./... ; gofmt -l . (vide) ; go test -count=1 ./... ; go test -count=3 ./... (stabilité). Go 1.26.0. go test -race non exécutable (section 6).
- Résultat complet : 36/36 PASS — 28 tests T02 (non-régression parfaite : aucune modification du coordinateur, du store ou des tests T02) et 8 tests T03 :
  - TestT03RealDisabledByDefault : mode off par défaut, dépôt invalide refusé.
  - TestT03CommentIdempotentAndAuth : création unique, rejeu, redémarrage complet (même base, adaptateur recréé) : un seul commentaire, une seule écriture, autorisation Bearer présente sur toutes les requêtes.
  - TestT03CommentServerErrorThenRetry : 500 propagé, rejeu réussi, aucun doublon.
  - TestT03CommentAmbiguousResponse : 502 après traitement serveur, rejeu par reconnaissance du marqueur, une seule écriture au total.
  - TestT03DraftPRIdempotent : branche déterministe, PR en Draft, rejeu avec même référence ; réponse ambiguë puis rejeu sans duplication.
  - TestT03RequestChangesIdempotent : une seule revue REQUEST_CHANGES.
  - TestT03TimeoutErrors : délai propagé ; le rejeu neutralise la fenêtre ambiguë (le serveur peut avoir traité la requête interrompue) : un seul commentaire.
  - TestT03DraftPR422Recovery : PR préexistante côté GitHub, registre local vierge : restitution sans écriture distante.
- Stable sur trois passes consécutives (-count=3).

## 8. État Git et sauvegarde

Publication sur `mission/ai-coordinator-v01` par commits `aic(t03): ...` ; SHA distants vérifiés après publication par lecture du dépôt distant. La liste exacte des SHA figure dans le commentaire de réception publié sur la PR #4.

## 9. Réserves et suite

- go test -race à exécuter sur machine complète (T03-A résiduel).
- Qualification de l'adaptateur sur un environnement GitHub réel isolé et explicitement autorisé (permissions du jeton, pagination, limites de débit) : à planifier avec le responsable.
- Préparation de la branche de tête déterministe lors des publications : à intégrer au processus de publication autorisé.
- Raccordements Mistral et OpenAI : hors périmètre T03, non engagés.

Décision attendue : réception de T03 ou ordre correctif.	Aucune intervention supplémentaire engagée après publication.

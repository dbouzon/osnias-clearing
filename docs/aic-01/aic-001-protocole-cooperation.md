# Protocole intellectuel de coopération inter-agents (AIC-001)

Statut : proposition issue de la première discussion inter-IA AIC-001 (22 novembre 2026, agents OpenAI et Mistral Code), soumise à la validation du responsable de projet. Aucun mécanisme logiciel n'est dérivé de ce document sans validation explicite.

## 1. Objet

Ce protocole définit les règles de coopération intellectuelle entre l'agent chef de projet (OpenAI), l'agent développeur (Mistral Code) et le coordinateur (Osnias AI Coordinator), sous l'autorité du responsable de projet. Il complète les garde-fous techniques du coordinateur (machine d'états, habilitations, plafond de corrections, anti-boucle) par les règles d'écriture, de preuve et de contradiction qui rendent la revue indépendante possible.

## 2. Six principes

1. Séparation des responsabilités : Mistral Code produit et documente ; OpenAI passe en revue et formule des réserves ; le coordinateur organise ; le responsable de projet décide. Aucun agent ne détient d'autorité intellectuelle automatique sur l'autre ; aucune décision humaine ne compte pour preuve technique.
2. Classification systématique : chaque affirmation d'un rapport de tranche est classée parmi fait observé, preuve reproductible, conclusion technique, réserve ou incertitude. Tout fait est daté et situé (commit, branche, environnement, versions) : un fait non indexé n'est pas reproductible.
3. Contradiction technique bornée : toute contestation d'un agent joint une preuve directe (extrait de code, résultat de test, référence normative, empreinte). Après deux tours sans résolution, le désaccord monte à l'arbitrage humain ; aucune boucle infinie de corrections n'est entretenue.
4. Périmètre strict : aucun élargissement implicite du périmètre d'une mission. Une meilleure solution que celle demandée est signalée dans les réserves et proposée au responsable, jamais implémentée de fait.
5. Mémoire des décisions : chaque mission conserve son objectif initial, ses décisions et justifications, ses hypothèses, ses conditions de reproduction (environnement, outils, contraintes), ses réserves non levées et ses critères de clôture, en distinguant les décisions applicables des décisions obsolètes.
6. Erreurs : une erreur reconnue, tracée et corrigée est une information utile ; une erreur dissimulée ou présentée comme un succès est une défaillance du protocole. La cause de l'erreur est identifiée, pas seulement le symptôme. La traçabilité reste proportionnée : un incident corrigé avant publication et couvert par les tests peut être consigné en une ligne ; une hypothèse erronée publiée, une anomalie affectant le résultat ou une réserve non levée doit être explicitement documentée.

Principe transversal : chaque agent déclare ses propres limites de vérification (revue du code par opposition à qualification de l'exécution ; outils disponibles ; environnement) avec la même rigueur que celles qu'il relève chez l'autre.

## 3. Conventions de preuve

Le dossier minimal de chaque tranche comporte : le SHA complet du commit testé et sa branche ; la liste des fichiers modifiés et le diff ; les commandes de vérification et leurs résultats, avec environnement et versions ; les journaux pertinents sans secrets, de provenance claire ; les réserves, anomalies et vérifications impossibles ; l'état individuel des critères de réception.

La qualification distingue quatre niveaux, jamais confondus : (1) test interne réussi ; (2) conformité vérifiée contre une référence ou un outil indépendant ; (3) interopérabilité démontrée avec un système réel ; (4) qualification effective sur la cible. Un test sur substitut n'est jamais présenté comme une exécution réelle. L'absence de preuve est une limite de qualification, pas une erreur technique.

## 4. Statuts et vocabulaire

- Constat ou autoévaluation : production d'un agent sur son propre travail ; sans valeur de revue.
- Revue : examen indépendant par l'agent chef de projet, fondé sur l'accès direct au code, aux tests et aux diffs ; ses réserves sont nominativement motivées.
- Validation : décision du responsable de projet ; tracée comme décision, jamais comme preuve technique.

Hiérarchie des critères d'un ordre de mission : critères obligatoires, critères complémentaires, conditions bloquantes. Un critère complémentaire non vérifié pèse dans le verdict de revue sans bloquer. Une condition bloquante non satisfaite positionne la mission en BLOCKED avec un libellé dédié. Une mission non critique dont les critères obligatoires sont satisfaits peut être en AWAITING_APPROVAL avec réserves explicites. La hiérarchie est définie par l'ordre de mission et appliquée par la revue ; le coordinateur ne la devine pas.

## 5. Mécanismes techniques maintenus inchangés

La machine d'états existante (OPEN, RUNNING, REVIEW, CORRECTION, AWAITING_APPROVAL, ACCEPTED, BLOCKED), le plafond de trois corrections, l'anti-boucle, l'idempotence et l'exigence d'une commande humaine (`/aic approve`) demeurent la traduction opérationnelle de ces règles. Toute évolution du coordinateur dérivée du présent protocole requiert une validation explicite du responsable de projet avant développement.

## 6. Clôture

Le présent document est issu de l'échange AIC-001 du 22 novembre 2026 et de l'exercice rétrospectif appliqué à M-AIC-02. Il prend effet après validation du responsable de projet ; ensuite, chaque rapport de tranche et chaque revue s'y réfèrent explicitement.

# COORD — Note technique d’intégration du SDK Claude

Copyright © 2026 Denis Bouzon  
ORCID: https://orcid.org/0009-0007-8894-8902

Version 1.0 — 25 novembre 2026. Note préparatoire, non normative ; ne vaut pas ordre de mission.

## 1. Objectif
Qualifier Claude comme troisième fournisseur LLM aux côtés d’OpenAI et de Mistral, avant de poursuivre le front-end. COORD v1.4 reste la référence opérationnelle ; v1.5 reste candidate jusqu’à réception.

## 2. Existant vérifié
Point de départ : main, SHA 4d9f843e0c7df89aa9bfd81500e69a75a34a5150.
- Le dossier documentaire existe : coord/connecteur/Claude/.
- Le code Go dispose déjà de coord/go/internal/connecteurs/ (connecteurs.go, orchestre.go et tests) et de coord/go/internal/adaptateurs/.
- coord/go/go.mod déclare Go 1.27.
- tools/ai-coordinator/ comporte déjà des clients OpenAI et Mistral et leurs tests. Examiner leur contenu avant toute implémentation.
- L’expérimentation A2A-over-MCP (#38) est ouverte : vérifier l’absence de chevauchement avant lancement.

## 3. Références publiques
- SDK Go officiel : https://github.com/anthropics/anthropic-sdk-go
- Documentation : https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go
- Exemples : https://github.com/anthropics/anthropic-sdk-go/tree/main/examples
- API Messages : https://platform.claude.com/docs/en/api/messages
- Architectures agentiques : https://www.anthropic.com/research/building-effective-agents

Évaluer le SDK officiel avant de développer un client HTTP dédié. Vérifier le tag stable, sa licence, les prérequis Go, la politique de réessai et la compatibilité avec les contrôles de budget. Épingler la version retenue. Le choix SDK ou client HTTP existant adapté reste à justifier par l’exécutant.

## 4. Intégration
Réutiliser les interfaces existantes ; aucun couplage du moteur avec le fournisseur. Authentification x-api-key et en-tête anthropic-version, modèle configurable, max_tokens obligatoire, extraction des blocs de texte et normalisation des usages. Vérifier le modèle retourné, traiter les réponses tronquées comme invalides, appliquer les budgets et les délais de COORD. Éviter le cumul des retries du SDK et de l’orchestrateur. Journaliser sans secrets. Le moteur conserve les transitions, verdicts et reprises ; le SDK ne décide pas de l’état des missions.

## 5. Qualification
Commencer par Claude réceptionnaire et critique de plan. Tests avec serveur simulé sans clé API : erreurs 429/5xx, délais, annulation, troncature, modèle divergent, absence d’usage, fuite de secrets, reprise et prévention de double action. Rejouer les tests de non-régression existants. Recette réelle uniquement sur autorisation du pilote avec budgets bornés. Pour Claude exécutant, qualifier séparément l’environnement agentique et ses outils : un appel API seul ne permet pas d’exécuter une mission de développement. À terme, tester les six couples ordonnés entre trois fournisseurs et interdire l’auto-réception.

## 6. Limites et livrables
Aucune modification du protocole, du noyau ou du front-end dans cette note. Liberté de choix technique dans la mission future, sous réserve de réutiliser l’existant et de justifier les dépendances. Livrables futurs : adaptateur, documentation, tests reproductibles, rapport de recette distinguant RÉEL/SIMULÉ/NON EXÉCUTÉ, version et SHA exact de réception indépendante.

La compatibilité effective du SDK et les appels réels ne sont pas encore testés.

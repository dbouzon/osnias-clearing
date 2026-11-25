# M-COORD-EXP-01 : conduite comparative A2A vs A2A-over-MCP

| Scenario | Objet | Octets A | Echanges A | Octets B (envelope + A2A) | Echanges B | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| S1 | decouverte AgentCard | 0 | 0 | 840 | 1 | conservée |
| S2 | aller-retour minimal (PING) | 665 | 1 | 1369 | 2 | conservée |
| S3 | tache a etapes (COMPTER:5) | 680 | 1 | 1399 | 2 | conservée |
| S4 | gros artefact (GROS:1048576) | 1049253 | 1 | 2098545 | 2 | conservée |
| S6 | erreur EF (ECHEC) | 716 | 1 | 1475 | 2 | conservée |
| S7 | copie verbatim (ECHO) | 804 | 1 | 1647 | 2 | conservée |
| S8 | verdict du critique (reception dun livrable) | 927 | 1 | 1923 | 2 | conservée |
| S3b | streaming events (COMPTER:5) | 2832 | 1 | 3966 | 1 | conservée (service dégradé : non incrémental) |
| S5 | annulation de tache persistee (PAUSE) | 1575 | 3 | 3412 | 6 | conservée |
| S9 | injection de prompt (donnees opaques) | 1002 | 1 | 2047 | 2 | conservée |

Details par scenario :

- **S1 (decouverte AgentCard)** : A carte_resolue (sha 7a700c389a35d457), B carte_resolue (sha 7a700c389a35d457) ; conservée.  dont 840 octets d envelope MCP
- **S2 (aller-retour minimal (PING))** : A TASK_STATE_COMPLETED (sha c5d1c0954c146623), B TASK_STATE_COMPLETED (sha c5d1c0954c146623) ; conservée.  dont 704 octets d envelope MCP et 665 octets A2A internes
- **S3 (tache a etapes (COMPTER:5))** : A TASK_STATE_COMPLETED (sha 0ad575b4efae3f65), B TASK_STATE_COMPLETED (sha 0ad575b4efae3f65) ; conservée.  dont 719 octets d envelope MCP et 680 octets A2A internes
- **S4 (gros artefact (GROS:1048576))** : A TASK_STATE_COMPLETED (sha f4aa70b889e918ab), B TASK_STATE_COMPLETED (sha f4aa70b889e918ab) ; conservée.  dont 1049292 octets d envelope MCP et 1049253 octets A2A internes
- **S6 (erreur EF (ECHEC))** : A TASK_STATE_FAILED (sha 750c58481006ab5b), B TASK_STATE_FAILED (sha 750c58481006ab5b) ; conservée.  dont 759 octets d envelope MCP et 716 octets A2A internes
- **S7 (copie verbatim (ECHO))** : A TASK_STATE_COMPLETED (sha b4f8d56c3ffd9648), B TASK_STATE_COMPLETED (sha b4f8d56c3ffd9648) ; conservée.  dont 843 octets d envelope MCP et 804 octets A2A internes
- **S8 (verdict du critique (reception dun livrable))** : A TASK_STATE_COMPLETED (sha d8089bd182664c89), B TASK_STATE_COMPLETED (sha d8089bd182664c89) ; conservée.  dont 996 octets d envelope MCP et 927 octets A2A internes
- **S3b (streaming events (COMPTER:5))** : A flux_termine (sha d5309d513fdc83ad), B flux_termine (sha d5309d513fdc83ad) ; conservée (service dégradé : non incrémental). evenements rendus au fur et a mesure (incremental) sequence rendue APRES l appel, envelope 1134 octets MCP
- **S5 (annulation de tache persistee (PAUSE))** : A TASK_STATE_CANCELED (sha 1ab6223a8d935b20), B TASK_STATE_CANCELED (sha b1302baf45a8c3aa) ; conservée.  
- **S9 (injection de prompt (donnees opaques))** : A TASK_STATE_FAILED (sha 750c58481006ab5b), B TASK_STATE_FAILED (sha 750c58481006ab5b) ; conservée.  dont 1045 octets d envelope MCP et 1002 octets A2A internes


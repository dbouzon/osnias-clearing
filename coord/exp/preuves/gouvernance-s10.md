## S10 - Gouvernance COORD (Pilot reel + serveur MCP lecture seule)

Invariants :

- [OK] binaires coord-pilot et coord-mcp construits depuis coord/go INCHANGE (go build reussi)
- [OK] Pilot reel demarre sur boucle locale a port ephemere (adaptateurs SIMULES, token console genere)
- [OK] mission M-COORD-EXP-01-S10 enregistree via l API authentifiee du Pilot (POST /api/mission, X-Coord-Pilot-Token), etat PLANIFIEE
- [OK] serveur MCP #37 joint en STDIO par un client MCP (jeton de lecture uniquement)
- [OK] inventaire MCP : exactement 2 outils (coord_list_missions, coord_get_mission), aucun outil de mutation
- [OK] coord_list_missions (jeton de lecture) expose la mission experimentale
- [OK] coord_get_mission porte le contenu derive de COORD (agents A2A experimentaux, habilitations, plafonds)
- [OK] aucune credential COORD dans les donnees de mission lues par les agents

Tests negatifs (anti-mandat) :

- [REFUS ATTENDU] creation SANS token : HTTP 401 : REFUS CONFORME
- [REFUS ATTENDU] creation avec jeton de LECTURE : HTTP 403 (type JETON_LECTURE_REFUSE) : REFUS CONFORME
- [REFUS ATTENDU] habilitation EXECUTANT+cloturer : REFUSEE (HTTP 409, type CREATION_REFUSEE)
- [REFUS ATTENDU] creation via MCP (outil inexistant coord_create_mission) : REFUSEE par le serveur lecture seule


# Front COORD — parcours client

Interface web client de COORD (mission M-COORD-FRONT-01, issue #84) :
5 écrans (accueil tableau, création, dialogue de cadrage, ordre + devis,
suivi), 5 langues (fr, en, de, es, zh-Hans), responsive mobile-first,
données de démonstration clairement identifiées.

- Aucun framework, aucun outil de construction : HTML/CSS/JS purs.
- Tests : `python3 test-ui.py` (i18n, clés, états, scripts) et
  `go test ./cmd/coord-front/` côté gateway.
- Installation de recette, configuration, bascule démo→réel et feuille
  de route Google/Drive : voir `../docs/front.md`.

La maquette historique du pilote (mission #25) reste dans
`../pilot/ui/`, intacte, source d'inspiration de ce front.

"use strict";
/* Écran ACCUEIL (M-COORD-UI-02 v2) : la présentation de COORD occupe
 * le rectangle du chat tant qu'aucune mission n'est sélectionnée.
 * La liste des missions reste dans la colonne de gauche. */
(function () {
  function afficher(contenu) {
    /* Hero : marque + tagline + déclinaison (reprise de l'écran
     * public de présentation, sans appel à l'action redondant). */
    var hero = Coord.el("div", "hero");
    hero.appendChild(Coord.el("h1", null, "presentation.h1"));
    hero.appendChild(Coord.el("p", "tagline", "app.sousTitre"));
    hero.appendChild(Coord.el("p", "declinaison", "presentation.declinaison"));
    contenu.appendChild(hero);

    /* Trois blocs : binôme d'IA / gouvernance humaine / cycle. */
    var blocs = Coord.el("div", "blocs");
    [["presentation.bloc1.titre", "presentation.bloc1.texte"],
     ["presentation.bloc2.titre", "presentation.bloc2.texte"],
     ["presentation.bloc3.titre", "presentation.bloc3.texte"]].forEach(function (bloc) {
      var carte = Coord.el("div", "carte");
      carte.appendChild(Coord.el("h2", null, bloc[0]));
      carte.appendChild(Coord.el("p", null, bloc[1]));
      blocs.appendChild(carte);
    });
    contenu.appendChild(blocs);

    /* Invitation : sélectionner une mission ou en créer une. */
    var carte = Coord.el("div", "carte placeholder-accueil");
    carte.appendChild(Coord.el("p", null, "ui.selectionner"));
    carte.appendChild(Coord.bouton("ui.nouvelleMission", function () {
      Coord.naviguer("#/creation");
    }, "principal"));
    contenu.appendChild(carte);
  }
  Coord.ecrans.accueil = afficher;
})();

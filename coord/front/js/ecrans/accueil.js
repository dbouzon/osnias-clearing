"use strict";
/* Écran ACCUEIL (M-COORD-UI-01, S1) : avec la colonne des missions à
 * gauche, le centre ne porte plus le tableau des missions : il
 * affiche le placeholder d'invitation à sélectionner une mission (ou
 * à en créer une). La liste, le rafraîchissement et les erreurs de
 * chargement vivent dans la colonne (js/colonne-missions.js). */
(function () {
  function afficher(contenu) {
    contenu.appendChild(Coord.el("h1", null, "accueil.titre"));
    const carte = Coord.el("div", "carte placeholder-accueil");
    carte.appendChild(Coord.el("p", null, "ui.selectionner"));
    carte.appendChild(Coord.bouton("ui.nouvelleMission", function () {
      Coord.naviguer("#/creation");
    }, "principal"));
    contenu.appendChild(carte);
  }
  Coord.ecrans.accueil = afficher;
})();

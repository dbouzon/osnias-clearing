"use strict";
/* Écran PRÉSENTATION (public, avant connexion) — mission M-COORD-BYCOORD-01.
 * Nom commercial, tagline, trois blocs (binôme d'IA / gouvernance humaine /
 * cycle vérifiable), « Comment ça marche » (7 états du parcours + cycle de
 * vérification plan → critique → réception → ACK), appel à l'action vers
 * le parcours démo. Aucune requête API : contenu statique i18n. */
(function () {
  var ETATS_PARCOURS = ["INTENTION", "DIALOGUE", "ORDRE_PROPOSE", "ORDRE_QUALIFIE", "VALIDE", "PUBLIE", "TRANSMIS"];

  async function afficher(contenu) {
    /* Hero : marque + tagline + déclinaison. */
    var hero = Coord.el("div", "hero");
    hero.appendChild(Coord.el("h1", null, "presentation.h1"));
    hero.appendChild(Coord.el("p", "tagline", "app.sousTitre"));
    hero.appendChild(Coord.el("p", "declinaison", "presentation.declinaison"));
    contenu.appendChild(hero);

    /* Trois blocs. */
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

    /* Comment ça marche : parcours public en 7 états. */
    contenu.appendChild(Coord.el("h2", null, "presentation.commentTitre"));
    contenu.appendChild(Coord.el("p", "aide", "presentation.parcoursIntro"));
    var etapes = document.createElement("ol");
    etapes.className = "etapes";
    ETATS_PARCOURS.forEach(function (etat) {
      var li = document.createElement("li");
      li.textContent = Coord.libelleEtat(etat);
      etapes.appendChild(li);
    });
    contenu.appendChild(etapes);

    /* Cycle de vérification de chaque livrable. */
    contenu.appendChild(Coord.el("p", "aide", "presentation.cycleIntro"));
    var cycle = document.createElement("ol");
    cycle.className = "etapes";
    cycle.appendChild(Coord.el("li", null, "presentation.cycle1"));
    cycle.appendChild(Coord.el("li", null, "presentation.cycle2"));
    cycle.appendChild(Coord.el("li", null, "presentation.cycle3"));
    cycle.appendChild(Coord.el("li", null, "presentation.cycle4"));
    contenu.appendChild(cycle);

    /* Appel à l'action vers le parcours démo. */
    var cta = Coord.el("div", "cta-zone");
    cta.appendChild(Coord.bouton("presentation.cta", function () {
      Coord.naviguer("#/accueil");
    }, "principal"));
    cta.appendChild(Coord.el("p", "aide", "presentation.ctaAide"));
    contenu.appendChild(cta);
  }
  Coord.ecrans.presentation = afficher;
})();

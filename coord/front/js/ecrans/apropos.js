"use strict";
/* Écran À PROPOS (public) — mission M-COORD-BYCOORD-01.
 * Histoire du projet, crédits, traçabilité du cycle. Contenu statique
 * i18n, aucune requête API. */
(function () {
  async function afficher(contenu) {
    contenu.appendChild(Coord.el("h1", null, "apropos.titre"));

    contenu.appendChild(Coord.el("h2", null, "apropos.histoireTitre"));
    contenu.appendChild(Coord.el("p", null, "apropos.histoireTexte"));

    contenu.appendChild(Coord.el("h2", null, "apropos.creditsTitre"));
    var credits = document.createElement("ul");
    credits.className = "etapes";
    credits.appendChild(Coord.el("li", null, "apropos.creditPilote"));
    credits.appendChild(Coord.el("li", null, "apropos.creditBinome"));
    credits.appendChild(Coord.el("li", null, "apropos.creditLicence"));
    contenu.appendChild(credits);

    contenu.appendChild(Coord.el("h2", null, "apropos.tracabiliteTitre"));
    contenu.appendChild(Coord.el("p", null, "apropos.tracabiliteTexte"));

    contenu.appendChild(Coord.bouton("apropos.retour", function () {
      Coord.naviguer("#/presentation");
    }));
  }
  Coord.ecrans.apropos = afficher;
})();

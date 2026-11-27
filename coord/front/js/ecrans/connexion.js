"use strict";
/* Écran CONNEXION : choix du compte client. Les noms publics viennent
 * de GET /api/comptes ; aucun token ne transite jamais. */
(function () {
  async function afficher(contenu) {
    const titre = Coord.el("h1", null, "app.connexionTitre");
    contenu.appendChild(titre);
    contenu.appendChild(Coord.el("p", "aide", "app.connexionAide"));
    const rep = await Coord.requete("GET", "/api/comptes");
    if (rep.statut !== 200 || !rep.donnees || !rep.donnees.comptes) {
      Coord.messageErreur(contenu, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
      contenu.appendChild(Coord.bouton("ui.reessayer", function () {
        Coord.naviguer("#/accueil");
      }, "principal"));
      return;
    }
    const liste = Coord.el("div", "carte");
    rep.donnees.comptes.forEach(function (nom) {
      const rangée = Coord.el("div", "rangée");
      const b = Coord.bouton("app.entrer", async function () {
        const r = await Coord.requete("POST", "/api/connexion", { compte: nom });
        if (r.statut === 200 && r.donnees && r.donnees.compte) {
          Coord.setSession(r.donnees);
          Coord.router();
        } else {
          Coord.messageErreur(contenu, r.donnees && r.donnees.motif ? r.donnees.motif : "app.erreurReseau");
        }
      }, "principal");
      rangée.appendChild(b);
      const label = document.createElement("span");
      label.textContent = nom;
      rangée.appendChild(label);
      liste.appendChild(rangée);
    });
    contenu.appendChild(liste);
  }
  Coord.ecrans.connexion = afficher;
})();

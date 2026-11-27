"use strict";
/* Écran CONNEXION (M-COORD-AUTH-01) : bouton « Se connecter avec
 * Google » quand la section google est active (OIDC serveur, aucun
 * secret ici). Repli fail-closed : si la connexion Google n'est pas
 * configurée côté serveur, l'écran historique de choix de compte
 * reste disponible (maintenance). */
(function () {
  async function afficher(contenu) {
    const etat = await Coord.requete("GET", "/api/auth/google/etat");
    const googleActif = etat.statut === 200 && etat.donnees && etat.donnees.activer === true;

    if (googleActif) {
      const carte = Coord.el("div", "carte");
      const b = document.createElement("a");
      b.className = "bouton principal";
      b.href = "/api/auth/google/login";
      b.textContent = Coord.t("app.googleConnexion");
      carte.appendChild(b);
      carte.appendChild(Coord.el("div", "aide", "app.googleAide"));
      contenu.appendChild(carte);
      return;
    }

    /* Repli : liste des comptes (mode maintenance, sans Google). */
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

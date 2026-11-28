"use strict";
/* Écran CONNEXION (M-COORD-SRC-01, règle du pilote) : le moyen de
 * connexion désigne la source de travail —
 *   - « Se connecter avec Google » : compte Gmail, missions dans le
 *     Google Drive de la personne (analyse documentaire) ;
 *   - « Se connecter avec GitHub » : compte GitHub, missions dans un
 *     repo GitHub.
 * Chaque bouton n'apparaît que si sa source est configurée côté
 * serveur (fail-closed). Repli : liste des comptes (maintenance). */
(function () {
  function boutonSource(cle, href, aide) {
    const b = document.createElement("a");
    b.className = "bouton principal";
    b.href = href;
    b.textContent = Coord.t(cle);
    const bloc = Coord.el("div");
    bloc.appendChild(b);
    if (aide) { bloc.appendChild(Coord.el("div", "aide", aide)); }
    return bloc;
  }

  async function afficher(contenu) {
    var googleActif = false;
    var githubActif = false;
    try {
      const e1 = await Coord.requete("GET", "/api/auth/google/etat");
      googleActif = e1.statut === 200 && e1.donnees && e1.donnees.activer === true;
    } catch (e) { /* fail-closed */ }
    try {
      const e2 = await Coord.requete("GET", "/api/auth/github/etat");
      githubActif = e2.statut === 200 && e2.donnees && e2.donnees.activer === true;
    } catch (e) { /* fail-closed */ }

    if (googleActif || githubActif) {
      const carte = Coord.el("div", "carte");
      if (googleActif) {
        carte.appendChild(boutonSource("app.googleConnexion", "/api/auth/google/login", "app.googleAide"));
      }
      if (githubActif) {
        carte.appendChild(boutonSource("app.githubConnexion", "/api/auth/github/login", "app.githubAide"));
      }
      contenu.appendChild(carte);
      return;
    }

    /* Aucune source configurée : plus de repli de comptes démo
     * (ordre du pilote, M-COORD-ENGINE-03). Message sobre uniquement ;
     * l'API /api/comptes et /api/connexion restent inchangées côté
     * serveur (recettes curl). */
    const carte = Coord.el("div", "carte");
    const msg = document.createElement("p");
    msg.className = "aide";
    msg.textContent = Coord.t("app.connexionIndisponible");
    carte.appendChild(msg);
    contenu.appendChild(carte);
  }
  Coord.ecrans.connexion = afficher;
})();

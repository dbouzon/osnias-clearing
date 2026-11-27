"use strict";
/* Colonne MISSIONS (mission M-COORD-UI-01, S1) : volet gauche de
 * l'espace connecté. Bouton « + Nouvelle mission », liste des
 * missions (pastille client, nom, libellé du statut client), missions
 * clôturées visibles, rafraîchie à chaque navigation. Sur mobile
 * (< 720 px), la colonne est repliée derrière le bouton « Missions »
 * de l'en-tête. Données : GET /api/projets puis GET /api/projets/{id}
 * séquentiels (aucune nouvelle route côté serveur). */
(function () {
  const EVENEMENTS_BINOME = ["PROPOSITION_ORDRE", "QUALIFICATION", "VALIDATION", "PUBLICATION", "TRANSMISSION"];

  function panneau() { return document.getElementById("col-missions"); }

  async function chargerMissions() {
    /* Retourne {missions: [...]} ou {erreur: motif}. */
    const rep = await Coord.requete("GET", "/api/projets");
    if (rep.statut !== 200 || !rep.donnees) {
      return { erreur: (rep.donnees && rep.donnees.motif) || Coord.t("app.erreurReseau") };
    }
    const missions = [];
    const ids = rep.donnees.projets || [];
    for (let i = 0; i < ids.length; i++) {
      const r = await Coord.requete("GET", "/api/projets/" + encodeURIComponent(ids[i]));
      if (r.statut === 200 && r.donnees) { missions.push(r.donnees); }
    }
    return { missions: missions };
  }

  async function rendre(idCourant) {
    const aside = panneau();
    aside.textContent = "";
    aside.appendChild(Coord.bouton("ui.nouvelleMission", function () {
      fermer(); /* mobile : replier avant de naviguer */
      Coord.naviguer("#/creation");
    }, "principal bloc"));

    const resultat = await chargerMissions();
    if (resultat.erreur) {
      const blocErreur = Coord.el("div", "erreur");
      blocErreur.appendChild(document.createTextNode(
        Coord.t("ui.erreurChargement", { motif: resultat.erreur })));
      aside.appendChild(blocErreur);
      aside.appendChild(Coord.bouton("ui.reessayer", function () { rendre(idCourant); }));
      return;
    }
    const missions = resultat.missions;
    if (missions.length === 0) {
      aside.appendChild(Coord.el("p", "aide", "accueil.vide"));
      return;
    }
    const liste = document.createElement("ul");
    liste.className = "liste-missions";
    missions.forEach(function (p) {
      const li = document.createElement("li");
      const boutonMission = document.createElement("button");
      boutonMission.type = "button";
      boutonMission.className = "ligne-mission" + (p.id === idCourant ? " actuelle" : "");
      if (p.id === idCourant) { boutonMission.setAttribute("aria-current", "page"); }
      boutonMission.appendChild(document.createTextNode(Coord.pastilleClient(p.etat) + " "));
      const nom = Coord.el("span", "mission-nom mono");
      nom.textContent = p.id;
      boutonMission.appendChild(nom);
      const libelle = Coord.el("span", "mission-statut");
      libelle.textContent = Coord.libelleStatutClient(p.etat);
      boutonMission.appendChild(libelle);
      boutonMission.addEventListener("click", function () {
        fermer();
        Coord.naviguer("#/mission/" + encodeURIComponent(p.id));
      });
      li.appendChild(boutonMission);
      liste.appendChild(li);
    });
    aside.appendChild(liste);
  }

  /* ---- visibilité ---- */
  let idCourant = null;

  function maj(idMission) {
    idCourant = idMission || null;
    const aside = panneau();
    aside.classList.remove("cache");
    aside.classList.remove("repliee");
    rendre(idCourant);
  }

  function masquer() {
    panneau().classList.add("cache");
    fermer();
  }

  /* ---- repli mobile ---- */
  function ouvert() { return panneau().classList.contains("ouvert"); }

  function fermer() { panneau().classList.remove("ouvert"); }

  function basculer() {
    /* Desktop (M-COORD-UI-02 v2) : le bouton « Missions » replie ou
     * déplie la colonne (classe .repliee). Mobile : tiroir existant. */
    if (window.innerWidth >= 720) {
      const aside = panneau();
      if (aside.classList.contains("cache")) { maj(idCourant); return; }
      aside.classList.toggle("repliee");
      return;
    }
    panneau().classList.remove("cache");
    panneau().classList.toggle("ouvert");
    if (ouvert()) { rendre(idCourant); }
  }

  /* Initialisation des interactions : appelé par Coord.demarrer. */
  function init() {
    document.getElementById("btn-missions").addEventListener("click", basculer);
    /* Fermeture au clic en dehors du panneau (mobile). */
    document.addEventListener("click", function (e) {
      const aside = panneau();
      if (!ouvert()) { return; }
      if (aside.contains(e.target) || document.getElementById("btn-missions").contains(e.target)) { return; }
      fermer();
    });
  }

  Coord.colonneMissions = {
    maj: maj, masquer: masquer, init: init,
    evenementsBinome: EVENEMENTS_BINOME
  };
})();

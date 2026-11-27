"use strict";
/* Écran ACCUEIL : tableau central des missions (desktop) et cartes
 * empilées (mobile), « Nouvelle mission » en haut, filtre par état. */
(function () {
  let filtreMemorise = "";

  async function afficher(contenu) {
    filtreMemorise = (document.getElementById("filtre-etat") || { value: filtreMemorise }).value || filtreMemorise;
    contenu.appendChild(Coord.el("h1", null, "accueil.titre"));
    const outils = Coord.el("div", "outils");
    outils.appendChild(Coord.bouton("accueil.nouvelle", function () {
      Coord.naviguer("#/creation");
    }, "principal"));
    const filtre = Coord.el("div", "filtre");
    filtre.appendChild(Coord.el("span", "aide", "accueil.filtre"));
    const select = document.createElement("select");
    select.id = "filtre-etat";
    const optionTous = document.createElement("option");
    optionTous.value = "";
    optionTous.textContent = Coord.t("accueil.filtreTous");
    select.appendChild(optionTous);
    ["INTENTION", "DIALOGUE", "ORDRE_PROPOSE", "ORDRE_QUALIFIE", "VALIDE", "PUBLIE", "TRANSMIS"].forEach(function (e) {
      const o = document.createElement("option");
      o.value = e;
      o.textContent = Coord.libelleEtat(e);
      select.appendChild(o);
    });
    select.value = filtreMemorise;
    select.addEventListener("change", function () {
      filtreMemorise = select.value;
      afficher(contenu);
    });
    filtre.appendChild(select);
    outils.appendChild(filtre);
    contenu.appendChild(outils);

    const rep = await Coord.requete("GET", "/api/projets");
    if (rep.statut !== 200 || !rep.donnees) {
      Coord.messageErreur(contenu, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
      return;
    }
    const ids = rep.donnees.projets || [];
    const projets = [];
    for (let i = 0; i < ids.length; i++) {
      const r = await Coord.requete("GET", "/api/projets/" + encodeURIComponent(ids[i]));
      if (r.statut === 200 && r.donnees) { projets.push(r.donnees); }
    }
    const filtreEtat = filtreMemorise;
    const visibles = filtreEtat ? projets.filter(function (p) { return p.etat === filtreEtat; }) : projets;
    if (visibles.length === 0) {
      contenu.appendChild(Coord.el("p", "aide", "accueil.vide"));
      return;
    }

    /* Desktop : tableau */
    const table = document.createElement("table");
    table.className = "tableau et-desktop";
    const entete = document.createElement("tr");
    ["acc.col.mission", "acc.col.etat", "acc.col.version", "acc.col.empreinte", "acc.col.dernier"].forEach(function (cle) {
      const th = document.createElement("th");
      th.textContent = Coord.t(cle);
      entete.appendChild(th);
    });
    const thead = document.createElement("thead");
    thead.appendChild(entete);
    table.appendChild(thead);
    const tbody = document.createElement("tbody");
    visibles.forEach(function (p) {
      const tr = document.createElement("tr");
      const tdNom = document.createElement("td");
      tdNom.className = "mono";
      tdNom.textContent = p.id;
      tr.appendChild(tdNom);
      const tdEtat = document.createElement("td");
      tdEtat.innerHTML = "";
      tdEtat.appendChild(document.createTextNode(Coord.pastilleEtat(p.etat) + " "));
      const lib = document.createElement("span");
      lib.className = "etat-libelle";
      lib.textContent = Coord.libelleEtat(p.etat);
      tdEtat.appendChild(lib);
      tr.appendChild(tdEtat);
      const tdVersion = document.createElement("td");
      tdVersion.textContent = p.version ? ("v" + p.version) : "—";
      tr.appendChild(tdVersion);
      const tdEmp = document.createElement("td");
      tdEmp.className = "empreinte";
      tdEmp.textContent = p.empreinte_ordre ? p.empreinte_ordre.slice(0, 12) + "…" : "—";
      tr.appendChild(tdEmp);
      const tdDernier = document.createElement("td");
      const ouvrir = Coord.bouton("app.ouvrir", function () {
        Coord.naviguer("#/mission/" + encodeURIComponent(p.id));
      });
      tdDernier.appendChild(ouvrir);
      tr.appendChild(tdDernier);
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);
    contenu.appendChild(table);

    /* Mobile : cartes */
    const cartes = Coord.el("div", "cartes-mobile");
    visibles.forEach(function (p) {
      const carte = Coord.el("div", "carte");
      const titre = Coord.el("div", "mission-titre mono");
      titre.textContent = p.id;
      carte.appendChild(titre);
      const etat = Coord.el("div", null);
      etat.appendChild(document.createTextNode(Coord.pastilleEtat(p.etat) + " "));
      const lib = document.createElement("span");
      lib.className = "etat-libelle";
      lib.textContent = Coord.libelleEtat(p.etat);
      etat.appendChild(lib);
      carte.appendChild(etat);
      const meta = Coord.el("div", "aide");
      meta.textContent = (p.version ? "v" + p.version : "—") + (p.empreinte_ordre ? " · " + p.empreinte_ordre.slice(0, 12) + "…" : "");
      carte.appendChild(meta);
      carte.appendChild(Coord.bouton("app.ouvrir", function () {
        Coord.naviguer("#/mission/" + encodeURIComponent(p.id));
      }));
      cartes.appendChild(carte);
    });
    contenu.appendChild(cartes);
  }
  Coord.ecrans.accueil = afficher;
})();

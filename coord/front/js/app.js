"use strict";
/* COORD parcours client — noyau applicatif (mission M-COORD-FRONT-01).
 * Conventions (plan v2 §5.4) : un module par écran dans l'espace
 * Coord.ecrans, i18n central via Coord.t(cle), aucun style en ligne,
 * aucun outil de construction. Priorité de langue : choix mémorisé
 * (localStorage) > langue du navigateur > anglais. */
window.Coord = (function () {
  const LANGUES = ["fr", "en", "de", "es", "zh"];
  let langue = "en";
  let session = null; /* {compte, demo, dossier_drive} */

  /* ---- i18n ---- */
  function t(cle, vars) {
    const table = window.CoordLangues && (window.CoordLangues[langue] || window.CoordLangues.en) || {};
    let texte = table[cle] !== undefined ? table[cle] : (window.CoordLangues.en[cle] !== undefined ? window.CoordLangues.en[cle] : cle);
    if (vars) {
      Object.keys(vars).forEach(function (k) {
        texte = texte.split("{" + k + "}").join(String(vars[k]));
      });
    }
    return texte;
  }

  function initLangue() {
    let choix = null;
    try { choix = window.localStorage.getItem("coord-langue"); } catch (e) { /* stockage indisponible */ }
    if (choix && LANGUES.indexOf(choix) !== -1) { langue = choix; return; }
    const nav = (window.navigator && (window.navigator.languages || [window.navigator.language])) || [];
    for (let i = 0; i < nav.length; i++) {
      const code = String(nav[i] || "").toLowerCase().slice(0, 2);
      if (LANGUES.indexOf(code) !== -1) { langue = code; return; }
    }
    langue = "en";
  }

  function changerLangue(l) {
    if (LANGUES.indexOf(l) === -1) { return; }
    langue = l;
    try { window.localStorage.setItem("coord-langue", l); } catch (e) { /* stockage indisponible */ }
    document.documentElement.lang = l === "zh" ? "zh-Hans" : l;
    router();
  }

  /* ---- API ---- */
  async function requete(methode, chemin, corps) {
    const opts = { method: methode, headers: { "Content-Type": "application/json" } };
    if (corps !== undefined) { opts.body = JSON.stringify(corps); }
    let rep;
    try {
      rep = await window.fetch(chemin, opts);
    } catch (e) {
      return { statut: 0, donnees: null, texte: "", erreur: t("app.erreurReseau") };
    }
    const texte = await rep.text();
    let donnees = null;
    try { donnees = JSON.parse(texte); } catch (e) { donnees = null; }
    return { statut: rep.status, donnees: donnees, texte: texte };
  }

  /* ---- affichage ---- */
  function contenu() { return document.getElementById("contenu"); }

  function effacer() { contenu().textContent = ""; return contenu(); }

  function texte(cle) { return document.createTextNode(t(cle)); }

  function el(tag, classe, cle) {
    const n = document.createElement(tag);
    if (classe) { n.className = classe; }
    if (cle) { n.setAttribute("data-i18n", cle); n.textContent = t(cle); }
    return n;
  }

  function bouton(cle, onClick, classe) {
    const b = el("button", "bouton" + (classe ? " " + classe : ""), cle);
    b.type = "button";
    b.addEventListener("click", onClick);
    return b;
  }

  function messageErreur(cible, cleOuTexte, vars) {
    const d = el("div", "erreur");
    if (cleOuTexte && cleOrTexteEstCle(cleOrTexte)) { d.textContent = t(cleOrTexte, vars); }
    else { d.textContent = cleOuTexte || ""; }
    cible.appendChild(d);
  }

  function cleOrTexteEstCle(s) { return s.indexOf(" ") === -1 && s.indexOf(".") !== -1; }

  function libelleEtat(etat) {
    /* Chaque état de la machine porte son libellé dans la table i18n ;
     * le libellé est TOUJOURS affiché à côté de la pastille. */
    return t("etat." + etat) !== "etat." + etat ? t("etat." + etat) : etat;
  }

  function pastilleEtat(etat) {
    /* 🟠 définie · non lancée (ordre qualifié, devis en attente de
     * décision) ; ⚪ autres états actifs ; 🟢 livrable réceptionné. */
    if (etat === "ORDRE_QUALIFIE") { return t("etat.pastille.DEFINIE"); }
    if (etat === "TRANSMIS") { return t("etat.pastille.AUTRE"); }
    return t("etat.pastille.AUTRE");
  }

  function rafraichirEntete() {
    const drive = document.getElementById("etat-drive");
    if (session) {
      drive.textContent = t("app.dossierDrive", { nom: session.dossier_drive || t("app.aucun") });
    } else {
      drive.textContent = "";
    }
    const bandeau = document.getElementById("bandeau-demo");
    if (session && session.demo) { bandeau.classList.remove("cache"); }
    else { bandeau.classList.add("cache"); }
    const select = document.getElementById("select-langue");
    select.value = langue;
  }

  /* ---- routeur ---- */
  const ecrans = {}; /* rempli par les modules d'écran */

  async function router() {
    rafraichirEntete();
    const h = window.location.hash || "#/accueil";
    if (!session) {
      if (h.indexOf("#/mission/") === 0 || h === "#/creation") { /* conserve le hash */ }
      await ecrans.connexion(effacer());
      return;
    }
    if (h === "#/creation") { await ecrans.creation(effacer()); return; }
    const m = h.match(/^#\/mission\/([^/]+)$/);
    if (m) { await ecrans.mission(effacer(), decodeURIComponent(m[1])); return; }
    await ecrans.accueil(effacer());
  }

  function naviguer(hash) {
    window.location.hash = hash;
    router();
  }

  async function demarrer() {
    initLangue();
    document.documentElement.lang = langue === "zh" ? "zh-Hans" : langue;
    document.getElementById("select-langue").addEventListener("change", function (e) {
      changerLangue(e.target.value);
    });
    document.getElementById("btn-compte").addEventListener("click", async function () {
      await requete("POST", "/api/deconnexion");
      session = null;
      naviguer("#/accueil");
    });
    window.addEventListener("hashchange", router);
    const rep = await requete("GET", "/api/session");
    if (rep.statut === 200 && rep.donnees && rep.donnees.compte) {
      session = rep.donnees;
    }
    router();
  }

  return {
    t: t, requete: requete, el: el, bouton: bouton, texte: texte,
    messageErreur: messageErreur, libelleEtat: libelleEtat, pastilleEtat: pastilleEtat,
    naviguer: naviguer, router: router, demarrer: demarrer,
    ecrans: ecrans,
    getSession: function () { return session; },
    setSession: function (s) { session = s; }
  };
})();

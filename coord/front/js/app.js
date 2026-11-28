successfully downloaded text file (SHA: 22f85b603639f7daa057d188e3bf312cceb4ae05)"use strict";
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
    const secours = (window.CoordLangues && window.CoordLangues.en) || {};
    let texte = table[cle] !== undefined ? table[cle] :
      (secours[cle] !== undefined ? secours[cle] : cle);
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
    appliquerMetadonnees();
    appliquerTextesStatiques();
    router();
  }

  function appliquerMetadonnees() {
    /* Nom commercial et description par langue (M-COORD-BYCOORD-01). */
    document.title = t("app.title");
    const meta = document.querySelector('meta[name="description"]');
    if (meta) { meta.setAttribute("content", t("app.metaDescription")); }
  }

  function appliquerTextesStatiques() {
    /* Chrome statique (en-tête, bandeau, pied) : le HTML de référence est
     * en français ; chaque clé data-i18n est appliquée au chargement et à
     * chaque changement de langue (M-COORD-UI-01, S6). */
    const noeuds = document.querySelectorAll("[data-i18n]");
    for (let i = 0; i < noeuds.length; i++) {
      /* Le bouton compte porte une clé dynamique (Se connecter /
       * Se déconnecter) : rafraichirEntete s'en occupe. */
      if (noeuds[i].id === "btn-compte") { continue; }
      noeuds[i].textContent = t(noeuds[i].getAttribute("data-i18n"));
    }
    appliquerTheme();
    /* Badge « COORD by COORD » en haut à gauche (frame fixe : l'en-tête est
     * sticky) : le PNG est transporté en data-URI (assets/coord-by-coord.js),
     * le binaire ne traversant pas le canal de push GitHub. */
    const logo = document.querySelector(".logo-img");
    if (logo && window.CoordLogoBadge) { logo.src = window.CoordLogoBadge; }
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
    if (cleOuTexte && cleOrTexteEstCle(cleOuTexte)) { d.textContent = t(cleOuTexte, vars); }
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

  /* ---- statuts client (M-COORD-UI-01, S3) ---- */
  /* Vue SIMPLIFIÉE demandée par le pilote : 3 statuts lisibles au lieu
   * des 11 états machine, qui restent affichés en détail dans
   * l'écran mission (libellés etat.* inchangés). */
  function statutClient(etat) {
    if (etat === "INTENTION" || etat === "DIALOGUE" ||
        etat === "ORDRE_PROPOSE" || etat === "ORDRE_QUALIFIE") { return "initiee"; }
    if (etat === "VALIDE" || etat === "PUBLIE" || etat === "TRANSMIS" ||
        etat === "ATTENTE_PILOTE") { return "encours"; }
    /* REALISEE, ABANDONNE, INVALIDE + tout état inconnu fermé. */
    return "cloturee";
  }

  function pastilleClient(etat) {
    return t("statut.pastille." + statutClient(etat));
  }

  function libelleStatutClient(etat) {
    return t("statut." + statutClient(etat));
  }

  function rafraichirEntete() {
    /* En-tête compact (M-COORD-UI-01, S5) : badge de compte à la
     * place de l'indicateur « Dossier Drive » ; le sélecteur de
     * langue et le changement de compte restent sur la même ligne. */
    const badge = document.getElementById("badge-compte");
    if (session) {
      badge.textContent = session.compte || "";
      badge.setAttribute("title", t("ui.compte"));
    } else {
      badge.textContent = "";
      badge.removeAttribute("title");
    }
    const bandeau = document.getElementById("bandeau-demo");
    if (session && session.demo) { bandeau.classList.remove("cache"); }
    else { bandeau.classList.add("cache"); }
    const select = document.getElementById("select-langue");
    select.value = langue;
    /* Bouton compte (M-COORD-UI-02 v2) : « Se connecter » sans session,
     * « Se déconnecter » avec session ; l'écran « Choisir un compte »
     * est supprimé, la connexion directe se fait depuis ce bouton. */
    const btnCompte = document.getElementById("btn-compte");
    const cleCompte = session ? "app.seDeconnecter" : "app.seConnecter";
    btnCompte.setAttribute("data-i18n", cleCompte);
    btnCompte.textContent = t(cleCompte);
  }

  /* ---- thème clair / sombre (M-COORD-UI-02 v2) ----
   * Le bouton montre l'ACTION : 🌙 pour passer en sombre, ☀️ pour
   * revenir en clair. Choix mémorisé (repli silencieux si stockage
   * indisponible). */
  function appliquerTheme() {
    const sombre = document.body.classList.contains("sombre");
    const btn = document.getElementById("btn-theme");
    btn.textContent = sombre ? "☀️" : "🌙";
    const libelle = t("ui.theme");
    btn.setAttribute("aria-label", libelle);
    btn.setAttribute("title", libelle);
  }

  function basculerTheme() {
    document.body.classList.toggle("sombre");
    try {
      window.localStorage.setItem("coord-theme",
        document.body.classList.contains("sombre") ? "sombre" : "clair");
    } catch (e) { /* stockage indisponible */ }
    appliquerTheme();
  }

  function initTheme() {
    let choix = null;
    try { choix = window.localStorage.getItem("coord-theme"); } catch (e) { /* stockage indisponible */ }
    if (choix === "sombre") { document.body.classList.add("sombre"); }
    appliquerTheme();
    document.getElementById("btn-theme").addEventListener("click", basculerTheme);
  }

  /* ---- routeur ---- */
  const ecrans = {}; /* rempli par les modules d'écran */

  async function router() {
    rafraichirEntete();
    const h = window.location.hash || "#/presentation";
    /* Écrans plein cadre : la colonne des missions est masquée. */
    if (h === "#/presentation") { Coord.colonneMissions.masquer(); await ecrans.presentation(effacer()); return; }
    if (h === "#/apropos") { Coord.colonneMissions.masquer(); await ecrans.apropos(effacer()); return; }
    if (!session) {
      if (h.indexOf("#/mission/") === 0 || h === "#/creation") { /* conserve le hash */ }
      Coord.colonneMissions.masquer();
      await ecrans.connexion(effacer());
      return;
    }
    if (h === "#/creation") { Coord.colonneMissions.masquer(); await ecrans.creation(effacer()); return; }
    /* M-COORD-API-STATUS-01 : console de supervision (admin serveur). */
    if (h === "#/console") { Coord.colonneMissions.masquer(); await ecrans.console(effacer()); return; }
    /* M-COORD-ENGINE-01 : #/engine = alias public du bouton Engine ; la
     * console reste contrôlée côté serveur (403 si non admin). */
    if (h === "#/engine") { Coord.colonneMissions.masquer(); await ecrans.console(effacer()); return; }
    /* Écrans deux volets : colonne des missions visible. */
    const m = h.match(/^#\/mission\/([^/]+)$/);
    if (m) {
      const id = decodeURIComponent(m[1]);
      await ecrans.mission(effacer(), id);
      Coord.colonneMissions.maj(id);
      return;
    }
    await ecrans.accueil(effacer());
    Coord.colonneMissions.maj(null);
  }

  function naviguer(hash) {
    window.location.hash = hash;
    routerProtege();
  }

  /* routerProtege : toute erreur d'écran est AFFICHÉE (cause +
   * Réessayer) au lieu de devenir une promesse rejetée silencieuse
   * (incident M-COORD-UI-01 : un écran vide sans explication). */
  function afficherErreurFatale(e) {
    const c = contenu();
    c.textContent = "";
    messageErreur(c, "ui.erreurDemarrage",
      { motif: (e && e.message) ? e.message : String(e) });
    c.appendChild(bouton("ui.reessayer", function () { window.location.reload(); }, "principal"));
  }

  async function routerProtege() {
    try {
      await router();
    } catch (e) {
      afficherErreurFatale(e);
    }
  }

  async function demarrerInterne() {
    initLangue();
    document.documentElement.lang = langue === "zh" ? "zh-Hans" : langue;
    appliquerMetadonnees();
    appliquerTextesStatiques();
    Coord.colonneMissions.init();
    document.getElementById("select-langue").addEventListener("change", function (e) {
      changerLangue(e.target.value);
    });
    document.getElementById("btn-engine").addEventListener("click", function () {
      naviguer("#/engine");
    });
    document.getElementById("btn-compte").addEventListener("click", async function () {
      if (session) {
        await requete("POST", "/api/deconnexion");
        session = null;
      }
      /* Sans session, le routeur affiche l'écran de connexion. */
      naviguer("#/accueil");
    });
    initTheme();
    window.addEventListener("hashchange", function () { routerProtege(); });
    const rep = await requete("GET", "/api/session");
    if (rep.statut === 200 && rep.donnees && rep.donnees.compte) {
      session = rep.donnees;
    }
    await routerProtege();
  }

  async function demarrer() {
    /* Dernier filet (M-COORD-UI-01, S5) : JAMAIS un écran figé sans
     * explication. Toute erreur au démarrage affiche la cause et un
     * bouton « Réessayer » au lieu du « Chargement… » muet. */
    try {
      await demarrerInterne();
    } catch (e) {
      afficherErreurFatale(e);
    }
  }

  return {
    t: t, requete: requete, el: el, bouton: bouton, texte: texte,
    messageErreur: messageErreur, libelleEtat: libelleEtat, pastilleEtat: pastilleEtat,
    statutClient: statutClient, pastilleClient: pastilleClient, libelleStatutClient: libelleStatutClient,
    naviguer: naviguer, router: router, demarrer: demarrer,
    ecrans: ecrans,
    getSession: function () { return session; },
    setSession: function (s) { session = s; }
  };
})();

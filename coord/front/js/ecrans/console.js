"use strict";
/* Écran CONSOLE (M-COORD-API-STATUS-01) : supervision des cinq moteurs
 * IA. Depuis M-COORD-ENGINE-02 la LECTURE est publique (décision du
 * pilote, 28/11/2026) ; les boutons de TEST (dépense ou appel réseau)
 * ne sont affichés qu'à l'admin (drapeau "admin" de /api/console/etat,
 * contrôle serveur inchangé sur POST).
 * Aucun appel réseau fournisseur depuis le navigateur : tout passe par
 * /api/console/* (côté serveur uniquement). Les logos proviennent de
 * @lobehub/icons-static-svg 1.106.0 (MIT) — attribution conservée dans
 * assets/ et le pied de page de la console. */
(function () {
  var FOURNISSEURS = [
    ["mistral", "Mistral AI", "mistral.ai"],
    ["anthropic", "Anthropic", "PBC · anthropic.com"],
    ["openai", "OpenAI", "openai.com"],
    ["gemini", "Google Gemini", "AI Studio · google.com"],
    ["deepseek", "DeepSeek", "deepseek.com"]
  ];

  function badgeEtat(etat) {
    var classe = { OPERATIONNELLE: "ok", ERREUR: "bad" }[etat] || "neutre";
    var b = Coord.el("span", "badge-console " + classe);
    b.textContent = Coord.t("console.etat." + etat);
    return b;
  }

  function ligne(label, valeur) {
    var p = document.createElement("p");
    p.className = "console-paire";
    var s = document.createElement("span");
    s.textContent = label;
    var v = document.createElement("span");
    v.textContent = valeur;
    p.appendChild(s); p.appendChild(v);
    return p;
  }

  function blocContrat(carte) {
    var c = carte.contrat;
    var div = document.createElement("div");
    div.className = "console-contrat";
    if (!c) {
      div.textContent = Coord.t("console.contratNonRenseigne");
      return div;
    }
    var texte = Coord.t("console.contratQuota") + " : " +
      (c.quota === null ? Coord.t("console.contratNonRenseigne") : c.quota) +
      (c.type === "credit" ? " " + (c.devise || "") : "");
    if (c.periode) { texte += " / " + c.periode; }
    if (c.consomme !== null && c.consomme !== undefined) {
      texte += " · " + Coord.t("console.contratConsomme") + " : " + c.consomme;
    }
    texte += " (" + c.source + ", " + c.date_source + ")";
    div.textContent = texte;
    return div;
  }

  /* État de charge du serveur LLM du fournisseur : LATENCE MOYENNE
   * réelle de la période (aucune métrique serveur n'est exposée par
   * les API). Décision du pilote : < 1 s vert (rapide), 1-2 s orange
   * (moyen), > 2 s rouge (congestionné). Les erreurs ne colorent pas
   * le badge : elles restent affichées sur la ligne des appels. */
  function latenceMoyenne(charge) {
    return charge && charge.duree_totale_ms && charge.appels ? charge.duree_totale_ms / charge.appels : null;
  }

  function badgeCharge(charge) {
    var classe = "neutre", cle = "console.chargeInactif";
    var lat = latenceMoyenne(charge);
    if (lat !== null) {
      if (lat < 1000) { classe = "ok"; cle = "console.rapide"; }
      else if (lat <= 2000) { classe = "moyen"; cle = "console.moyen"; }
      else { classe = "bad"; cle = "console.congestionne"; }
    }
    var b = document.createElement("span");
    b.className = "badge-console " + classe;
    b.textContent = Coord.t(cle);
    return b;
  }

  /* Total commandé / restant : depuis le contrat déclaratif
   * (console.contrats de gateway.json) et la consommation réelle
   * de la période (journal). Non renseigné tant que le pilote
   * n'a pas fixé les quotas. */
  function ligneTotalRestant(carte, charge) {
    var div = document.createElement("div");
    var consomme = charge ? (charge.jetons_entree + charge.jetons_sortie) : 0;
    var quota = carte.contrat && carte.contrat.quota;
    var pT = document.createElement("p");
    pT.className = "console-paire";
    var sT = document.createElement("span");
    sT.textContent = Coord.t("console.totalCommande");
    var vT = document.createElement("span");
    vT.textContent = quota ? String(quota) : Coord.t("console.contratNonRenseigne");
    pT.appendChild(sT); pT.appendChild(vT);
    var pR = document.createElement("p");
    pR.className = "console-paire";
    var sR = document.createElement("span");
    sR.textContent = Coord.t("console.restant");
    var vR = document.createElement("span");
    vR.textContent = quota ? String(Math.max(0, quota - consomme)) : Coord.t("console.contratNonRenseigne");
    pR.appendChild(sR); pR.appendChild(vR);
    div.appendChild(pT); div.appendChild(pR);
    return div;
  }

  function blocCharge(carte, charge) {
    var div = document.createElement("div");
    div.className = "console-charge";
    var t = document.createElement("p");
    t.className = "console-charge-titre";
    t.textContent = Coord.t("console.charge");
    div.appendChild(t);
    var ligneCharge = document.createElement("p");
    ligneCharge.className = "console-paire";
    var sC = document.createElement("span");
    sC.textContent = Coord.t("console.etatCharge");
    ligneCharge.appendChild(sC);
    ligneCharge.appendChild(badgeCharge(charge));
    div.appendChild(ligneCharge);
    if (!charge) {
      var vide = document.createElement("p");
      vide.className = "console-paire";
      vide.textContent = Coord.t("console.aucunAppel");
      div.appendChild(vide);
      div.appendChild(ligneTotalRestant(carte, charge));
      return div;
    }
    div.appendChild(ligne(Coord.t("console.appelsReels"),
      charge.appels + (charge.erreurs ? " (" + charge.erreurs + " " + Coord.t("console.erreurs") + ")" : "")));
    div.appendChild(ligne(Coord.t("console.jetonsConsommes"),
      (charge.jetons_entree + charge.jetons_sortie) + " (" + charge.jetons_entree + " / " + charge.jetons_sortie + ")"));
    div.appendChild(ligne(Coord.t("console.latenceMoyenne"),
      (charge.duree_totale_ms && charge.appels ? Math.round(charge.duree_totale_ms / charge.appels) : 0) + " ms"));
    div.appendChild(ligne(Coord.t("console.dernierAppel"),
      charge.dernier_appel_utc || Coord.t("console.jamaisTeste")));
    div.appendChild(ligneTotalRestant(carte, charge));
    return div;
  }

  function carteFournisseur(etat, carte, tester, charge) {
    var article = document.createElement("article");
    article.className = "console-carte";
    var haut = document.createElement("div");
    haut.className = "console-carte-haut";
    var identite = document.createElement("div");
    identite.className = "console-identite";
    var img = document.createElement("img");
    img.src = "assets/logo-" + carte.fournisseur + ".svg";
    img.alt = Coord.t("console.logo") + " " + carte.nom;
    img.className = "console-logo";
    var titre = document.createElement("div");
    var nom = document.createElement("strong");
    nom.textContent = carte.nom;
    var prop = document.createElement("small");
    prop.textContent = carte.proprietaire;
    titre.appendChild(nom); titre.appendChild(prop);
    identite.appendChild(img); identite.appendChild(titre);
    haut.appendChild(identite);
    haut.appendChild(badgeEtat(carte.etat));
    article.appendChild(haut);

    var corps = document.createElement("div");
    corps.className = "console-carte-corps";
    corps.appendChild(ligne(Coord.t("console.modeleTeste"), carte.modele_teste || "—"));
    corps.appendChild(ligne(Coord.t("console.active"),
      carte.active ? Coord.t("console.oui") : Coord.t("console.non") +
      (carte.cle_presente ? "" : " (" + Coord.t("console.cleAbsente") + ")")));
    var dernier = carte.dernier_test;
    corps.appendChild(ligne(Coord.t("console.dernierTest"),
      dernier ? (dernier.instant_utc + (dernier.categorie ? " · " + dernier.categorie : "")) :
        Coord.t("console.jamaisTeste")));
    corps.appendChild(blocCharge(carte, charge));
    corps.appendChild(blocContrat(carte));
    article.appendChild(corps);

    var pied = document.createElement("div");
    pied.className = "console-carte-pied";
    var bDispo = Coord.bouton("console.testDispo", function () {
      tester(carte.fournisseur, "dispo", bDispo, bReel);
    });
    var bReel = Coord.bouton("console.testReel", function () {
      var message = Coord.t("console.confirmationTexte", { plafond: etat.plafond_jetons || 20 });
      if (window.confirm(message)) {
        tester(carte.fournisseur, "reel", bDispo, bReel);
      }
    });
    if (etat && etat.admin) { pied.appendChild(bDispo); pied.appendChild(bReel); }
    article.appendChild(pied);
    return article;
  }

  async function afficher(contenu) {
    var titre = Coord.el("h1", null, "console.titre");
    contenu.appendChild(titre);
    var etat = null;
    try {
      var rep = await Coord.requete("GET", "/api/console/etat?periode=historique");
      if (rep.statut === 403 || rep.statut === 401) {
        var refuse = Coord.el("p", "erreur", "console.refusee");
        contenu.appendChild(refuse);
        return;
      }
      if (rep.statut !== 200) {
        Coord.messageErreur(contenu, "console.erreur");
        return;
      }
      etat = rep.donnees;
    } catch (e) {
      Coord.messageErreur(contenu, "console.erreur");
      return;
    }

    /* Sélecteur de période (mois en cours / historique disponible). */
    var barre = document.createElement("div");
    barre.className = "console-barre";
    var sel = document.createElement("select");
    sel.setAttribute("aria-label", Coord.t("console.periode"));
    [["historique", "console.periodeHistorique"], ["mois", "console.periodeMois"]].forEach(function (o) {
      var opt = document.createElement("option");
      opt.value = o[0];
      opt.textContent = Coord.t(o[1]);
      sel.appendChild(opt);
    });
    sel.addEventListener("change", function () { rafraichir(sel.value); });
    var bTous = null;
    if (etat && etat.admin) {
      bTous = Coord.bouton("console.testTous", function () { testerTous(bTous); }, "principal");
    }
    barre.appendChild(sel);
    if (bTous) { barre.appendChild(bTous); }
    contenu.appendChild(barre);

    var notice = Coord.el("p", "console-notice", "console.origine");
    contenu.appendChild(notice);

    var grille = document.createElement("div");
    grille.className = "console-grille";
    contenu.appendChild(grille);
    var retour = document.createElement("p");
    retour.className = "console-retour";
    retour.setAttribute("aria-live", "polite");
    contenu.appendChild(retour);

    /* Historique des tests. */
    var panneau = document.createElement("section");
    panneau.className = "console-panneau";
    panneau.appendChild(Coord.el("h2", null, "console.historique"));
    var table = document.createElement("table");
    table.className = "console-table";
    var thead = document.createElement("thead");
    var trh = document.createElement("tr");
    ["console.colonneInstant", "console.colonneFournisseur", "console.colonneType",
     "console.colonneResultat", "console.colonneDuree", "console.colonneCategorie"].forEach(function (c) {
      var th = document.createElement("th");
      th.textContent = Coord.t(c);
      trh.appendChild(th);
    });
    thead.appendChild(trh);
    table.appendChild(thead);
    var corpsTable = document.createElement("tbody");
    table.appendChild(corpsTable);
    panneau.appendChild(table);
    contenu.appendChild(panneau);

    var enCours = false;

    async function rafraichir(periode) {
      var rep = await Coord.requete("GET", "/api/console/etat?periode=" + encodeURIComponent(periode));
      if (rep.statut !== 200) { return; }
      etat = rep.donnees;
      grille.textContent = "";
      var noms = {};
      etat.fournisseurs.forEach(function (c) { noms[c.fournisseur] = c; });
      var charges = {};
      (etat.jetons_missions || []).forEach(function (a) { charges[a.moteur] = a; });
      /* Classement du plus rapide au plus lent (décision du pilote) ;
       * moteurs sans appel réel de la période en fin de liste, tri
       * alphabétique stable. */
      var tries = FOURNISSEURS.slice().sort(function (a, b) {
        var la = latenceMoyenne(charges[a[0]]);
        var lb = latenceMoyenne(charges[b[0]]);
        if (la === null && lb === null) { return a[0] < b[0] ? -1 : 1; }
        if (la === null) { return 1; }
        if (lb === null) { return -1; }
        return la - lb;
      });
      tries.forEach(function (f) {
        var c = noms[f[0]] || { fournisseur: f[0], etat: "NON_CONFIGUREE" };
        c.nom = f[1]; c.proprietaire = f[2];
        grille.appendChild(carteFournisseur(etat, c, tester, charges[f[0]] || null));
      });
      notice.textContent = Coord.t("console.origine") + " · " + Coord.t("console.actualisation") +
        " : " + etat.actualisation_utc;
      await rafraichirHistorique();
    }

    async function rafraichirHistorique() {
      corpsTable.textContent = "";
      var rep = await Coord.requete("GET", "/api/console/historique-tests");
      if (rep.statut !== 200) { return; }
      var entrees = (rep.donnees && rep.donnees.entrees) || [];
      if (!entrees.length) {
        var tr = document.createElement("tr");
        var td = document.createElement("td");
        td.colSpan = 6;
        td.textContent = Coord.t("console.vide");
        tr.appendChild(td);
        corpsTable.appendChild(tr);
        return;
      }
      entrees.slice().reverse().forEach(function (e) {
        var tr = document.createElement("tr");
        [e.instant_utc, e.fournisseur, e.type,
         e.resultat === "ok" ? "✓" : "✗", (e.duree_ms || 0) + " ms", e.categorie || ""].forEach(function (v) {
          var td = document.createElement("td");
          td.textContent = v;
          tr.appendChild(td);
        });
        corpsTable.appendChild(tr);
      });
    }

    async function tester(fournisseur, type, b1, b2) {
      if (enCours) { return; }
      enCours = true;
      if (b1) { b1.disabled = true; }
      if (b2) { b2.disabled = true; }
      retour.textContent = Coord.t("console.traitement");
      try {
        var rep = await Coord.requete("POST", "/api/console/test",
          { fournisseur: fournisseur, type: type });
        if (rep.statut === 200) {
          retour.textContent = fournisseur + " : " + Coord.t("console.etat." + rep.donnees.etat);
        } else {
          retour.textContent = fournisseur + " : " +
            ((rep.donnees && rep.donnees.code) ? rep.donnees.code : "erreur " + rep.statut);
        }
      } catch (e) {
        retour.textContent = "erreur réseau";
      }
      enCours = false;
      if (b1) { b1.disabled = false; }
      if (b2) { b2.disabled = false; }
      await rafraichir(sel.value);
    }

    async function testerTous(bouton) {
      if (enCours) { return; }
      enCours = true;
      bouton.disabled = true;
      retour.textContent = Coord.t("console.traitement");
      for (var i = 0; i < FOURNISSEURS.length; i++) {
        var f = FOURNISSEURS[i][0];
        try {
          var rep = await Coord.requete("POST", "/api/console/test",
            { fournisseur: f, type: "dispo" });
          retour.textContent = f + " : " + ((rep.donnees && rep.donnees.etat) ? rep.donnees.etat : rep.statut);
        } catch (e) { /* on continue la file */ }
      }
      enCours = false;
      bouton.disabled = false;
      await rafraichir(sel.value);
    }

    await rafraichir("historique");
  }

  Coord.ecrans.console = afficher;
})();

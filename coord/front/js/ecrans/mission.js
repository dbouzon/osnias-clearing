"use strict";
/* Écran MISSION : sections pilotées par l'état de la machine maïeutique.
 * DIALOGUE → chat de cadrage ; ORDRE_PROPOSE → ordre à qualifier ;
 * ORDRE_QUALIFIE → devis dérivé + décision ACCORD/REFUS explicite puis
 * actions pilote ; TRANSMIS → suivi avec mission fille ; journal
 * toujours visible en bas (écran SUIVI). */
(function () {
  let projet = null;
  let journalBrut = [];

  async function charger(id) {
    const rep = await Coord.requete("GET", "/api/projets/" + encodeURIComponent(id));
    if (rep.statut !== 200) { return null; }
    projet = rep.donnees;
    const j = await Coord.requete("GET", "/api/projets/" + encodeURIComponent(id) + "/journal");
    journalBrut = [];
    if (j.statut === 200) {
      j.texte.split("\n").forEach(function (ligne) {
        if (ligne.trim()) {
          try { journalBrut.push(JSON.parse(ligne)); } catch (e) { /* ligne hors JSON ignorée */ }
        }
      });
    }
    return projet;
  }

  function entete(contenu) {
    const carte = Coord.el("div", "carte");
    const rangée = Coord.el("div", "rangée");
    const titre = Coord.el("span", "mission-titre mono");
    titre.textContent = projet.id;
    rangée.appendChild(titre);
    rangée.appendChild(document.createTextNode(Coord.pastilleEtat(projet.etat) + " "));
    const lib = Coord.el("span", "etat-libelle");
    lib.textContent = Coord.libelleEtat(projet.etat);
    rangée.appendChild(lib);
    if (projet.version) {
      const v = Coord.el("span", "badge");
      v.textContent = Coord.t("projet.version", { version: "v" + projet.version });
      rangée.appendChild(v);
    }
    carte.appendChild(rangée);
    if (projet.empreinte_ordre) {
      const emp = Coord.el("div", "rangée");
      const etiquette = Coord.el("span", "aide");
      etiquette.textContent = Coord.t("projet.empreinte") + " : ";
      emp.appendChild(etiquette);
      const valeur = Coord.el("span", "empreinte");
      valeur.textContent = projet.empreinte_ordre;
      emp.appendChild(valeur);
      const copier = Coord.bouton("app.copier", function () {
        try {
          navigator.clipboard.writeText(projet.empreinte_ordre);
          valeur.textContent = projet.empreinte_ordre + "  ✓";
        } catch (e) { /* presse-papiers indisponible */ }
      }, "discret");
      emp.appendChild(copier);
      carte.appendChild(emp);
    }
    const dossiers = Coord.el("div", "aide");
    let texteDossiers = "";
    if (projet.cadrage) {
      const entree = projet.cadrage.dossier_entree;
      const sortie = projet.cadrage.dossier_sortie;
      texteDossiers = Coord.t("creation.dossierEntree") + " : " +
        (entree ? entree : Coord.t("projet.aucunDossierEntree")) + " · " +
        Coord.t("projet.dossierSortie") + " : " + (sortie || Coord.t("app.aucun"));
    }
    dossiers.textContent = texteDossiers;
    carte.appendChild(dossiers);
    contenu.appendChild(carte);
  }

  /* ---- section dialogue (chat) ---- */
  function sectionDialogue(contenu, id) {
    const section = Coord.el("div", null);
    section.appendChild(Coord.el("h2", null, "dlg.titre"));
    const chat = Coord.el("div", "chat");
    section.appendChild(chat);

    function messageDirecteur(auteur, texte) {
      const m = Coord.el("div", "message directeur");
      const a = Coord.el("span", "auteur");
      a.textContent = auteur;
      m.appendChild(a);
      m.appendChild(document.createTextNode(texte));
      chat.appendChild(m);
    }
    function messageClient(texte) {
      const m = Coord.el("div", "message client");
      const a = Coord.el("span", "auteur");
      a.textContent = Coord.t("dlg.vous");
      m.appendChild(a);
      m.appendChild(document.createTextNode(texte));
      chat.appendChild(m);
    }

    /* Rejoue le journal connu dans le chat. */
    journalBrut.forEach(function (e) {
      if (e.type === "TOUR_CLIENT" && e.entree !== undefined) { messageClient(e.entree); }
      else if (e.type === "TOUR_DIRECTEUR" && e.contenu) {
        messageDirecteur(Coord.t("dlg.directeur") +
          (e.reponse ? " (" + Coord.t(e.reponse === "CLARIFICATION" ? "dlg.clarification" : "dlg.question") + ")" : ""),
          e.contenu);
      }
    });
    if (chat.childNodes.length === 0) {
      messageDirecteur(Coord.t("dlg.directeur"), Coord.t("dlg.ouvrirAide"));
    }

    const saisie = Coord.el("div", "saisie");
    const champ = document.createElement("input");
    champ.type = "text";
    champ.placeholder = Coord.t("dlg.placeholder");
    saisie.appendChild(champ);
    saisie.appendChild(Coord.bouton("dlg.envoyer", async function () {
      const texteSaisi = champ.value.trim();
      if (!texteSaisi) { return; }
      champ.value = "";
      const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/tour", { entree: texteSaisi });
      if (rep.statut !== 200) {
        messageDirecteur(Coord.t("dlg.directeur"),
          rep.donnees && rep.donnees.motif ? rep.donnees.motif : Coord.t("app.erreurReseau"));
        return;
      }
      messageClient(texteSaisi);
      const r = rep.donnees.reponse || {};
      if (r.action === "QUESTION" || r.action === "CLARIFICATION") {
        messageDirecteur(Coord.t("dlg.directeur") + " (" + Coord.t(r.action === "QUESTION" ? "dlg.question" : "dlg.clarification") + ")", r.contenu || "");
      } else if (r.action === "PROPOSITION_ORDRE") {
        Coord.naviguer("#/mission/" + encodeURIComponent(id));
      } else if (r.action === "VERDICT") {
        messageDirecteur(Coord.t("dlg.directeur"), r.verdict || "");
      }
    }, "principal"));
    section.appendChild(saisie);

    /* Dépôt de document (corpus, D4). */
    section.appendChild(Coord.el("h2", null, "dlg.documents"));
    const docs = Coord.el("div", "carte");
    const zoneDepot = Coord.el("div", "rangée");
    const nomDoc = document.createElement("input");
    nomDoc.type = "text";
    nomDoc.placeholder = Coord.t("dlg.nomDoc");
    const contenuDoc = document.createElement("input");
    contenuDoc.type = "text";
    contenuDoc.placeholder = Coord.t("dlg.contenuDoc");
    zoneDepot.appendChild(nomDoc);
    zoneDepot.appendChild(contenuDoc);
    zoneDepot.appendChild(Coord.bouton("dlg.envoyerDoc", async function () {
      if (!nomDoc.value.trim() || !contenuDoc.value) { return; }
      const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/documents", {
        nom: nomDoc.value.trim(), contenu: contenuDoc.value
      });
      if (rep.statut === 200) {
        nomDoc.value = ""; contenuDoc.value = "";
        await afficher(contenu, id, false);
      } else {
        Coord.messageErreur(docs, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
      }
    }));
    docs.appendChild(zoneDepot);
    section.appendChild(docs);
    contenu.appendChild(section);
  }

  /* ---- section ordre + devis + décision + actions pilote ---- */
  function sectionOrdre(contenu, id) {
    const o = projet.ordre;
    const propose = projet.etat === "ORDRE_PROPOSE";
    const section = Coord.el("div", null);
    section.appendChild(Coord.el("h2", null, propose ? "ord.titre" : "ord.qualifie"));

    const carte = Coord.el("div", "carte");
    if (o) {
      const idn = o.identification || {};
      if (idn.version) {
        const v = Coord.el("div", "badge");
        v.textContent = Coord.t("projet.version", { version: idn.version });
        carte.appendChild(v);
      }
      ajouterChamp(carte, "ord.objectif", o.objectif);
      ajouterChamp(carte, "ord.livrable", o.livrable);
      const etapes = (o.etapes || []).map(function (e) {
        return e.nom + (e.controle_intermediaire ? " (" + Coord.t("ord.etapeControle", { controle: e.controle_intermediaire }) + ")" : "");
      }).join(" → ");
      ajouterChamp(carte, "ord.etapes", etapes);
      ajouterChamp(carte, "ord.limites", (o.limites || []).join(" ; "));
    }
    section.appendChild(carte);

    /* ORDRE_PROPOSE : qualifier ou revenir au dialogue. */
    if (propose) {
      const actions = Coord.el("div", "actions");
      actions.appendChild(Coord.bouton("ord.qualifier", async function () {
        const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/qualifier");
        if (rep.statut !== 200) {
          Coord.messageErreur(section, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
          return;
        }
        await afficher(contenu, id, false);
      }, "principal"));
      actions.appendChild(Coord.bouton("dlg.retour", async function () {
        const motif = window.prompt(Coord.t("dlg.motifRetour"));
        if (!motif || !motif.trim()) {
          Coord.messageErreur(section, "dlg.errMotif");
          return;
        }
        const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/retour-dialogue", { motif: motif.trim() });
        if (rep.statut !== 200) {
          Coord.messageErreur(section, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
          return;
        }
        await afficher(contenu, id, false);
      }));
      section.appendChild(actions);
    }
    contenu.appendChild(section);
  }

  function ajouterChamp(carte, cle, valeur) {
    if (valeur === undefined || valeur === null || valeur === "") { return; }
    const etiquette = Coord.el("div", "aide");
    etiquette.textContent = Coord.t(cle) + " : ";
    const corps = Coord.el("div");
    corps.textContent = valeur;
    carte.appendChild(etiquette);
    carte.appendChild(corps);
  }

  function sectionDevis(contenu, id) {
    if (projet.etat !== "ORDRE_QUALIFIE" && projet.etat !== "VALIDE" &&
        projet.etat !== "PUBLIE" && projet.etat !== "TRANSMIS") { return; }
    const section = Coord.el("div", null);
    section.appendChild(Coord.el("h2", null, "ord.devis"));
    const carte = Coord.el("div", "carte");
    if (!projet.devis) {
      const fixer = Coord.bouton("ord.devis", async function () {
        await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/devis");
        await afficher(contenu, id, false);
      });
      carte.appendChild(fixer);
    } else {
      ajouterChamp(carte, "ord.etapes", (projet.devis.etapes || []).join(" → "));
      ajouterChamp(carte, "ord.livrable", projet.devis.livrable);
      const paiement = Coord.el("div", "aide");
      paiement.textContent = Coord.t("ord.paiement");
      carte.appendChild(paiement);
      const accord = Coord.el("span", "badge " + (projet.accord_devis ? "accord" : "refus"));
      accord.textContent = projet.accord_devis ? Coord.t("ord.accord") : Coord.t("ord.refus");
      carte.appendChild(accord);
    }
    section.appendChild(carte);

    /* Décision explicite sur le devis : uniquement en ORDRE_QUALIFIE. */
    if (projet.etat === "ORDRE_QUALIFIE" && projet.devis && !projet.accord_devis) {
      const avert = Coord.el("div", "erreur");
      avert.textContent = Coord.t("ord.avertissement");
      section.appendChild(avert);
      const decision = Coord.el("div", "rangée");
      decision.appendChild(Coord.el("span", "aide", "ord.decision"));
      decision.appendChild(Coord.bouton("ord.accord", async function () {
        const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/accord-devis", { decision: "ACCORD" });
        if (rep.statut === 200) {
          const info = Coord.el("div", "info");
          info.textContent = Coord.t("ord.accordOk");
          section.appendChild(info);
          await afficher(contenu, id, false);
        }
      }, "principal"));
      decision.appendChild(Coord.bouton("ord.refus", async function () {
        const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/accord-devis", { decision: "REFUS" });
        if (rep.statut === 200) {
          const info = Coord.el("div", "info");
          info.textContent = Coord.t("ord.refusOk");
          section.appendChild(info);
        }
      }, "danger"));
      section.appendChild(decision);
    }
    contenu.appendChild(section);
  }

  function sectionPilote(contenu, id) {
    /* Actions pilote : l'exécution n'est proposée qu'après ACCORD
     * (règle d'UX ; le jeton d'action reste un acte explicite). */
    if (projet.etat === "ORDRE_QUALIFIE" && projet.accord_devis) {
      const section = Coord.el("div", null);
      section.appendChild(Coord.el("h2", null, "ord.actionsPilote"));
      const carte = Coord.el("div", "carte");
      carte.appendChild(Coord.el("label", null, "ord.jeton"));
      const jeton = document.createElement("input");
      jeton.type = "password";
      jeton.id = "jeton-action";
      carte.appendChild(jeton);
      carte.appendChild(Coord.el("div", "aide", "ord.jetonAide"));
      carte.appendChild(Coord.bouton("ord.valider", async function () {
        if (!window.confirm(Coord.t("ord.confirmValider"))) { return; }
        const rep = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/valider", {
          jeton_action: jeton.value
        });
        if (rep.statut !== 200) {
          Coord.messageErreur(carte, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
          return;
        }
        await afficher(contenu, id, false);
      }, "principal"));
      section.appendChild(carte);
      contenu.appendChild(section);
    }
    if (projet.etat === "VALIDE") {
      const actions = Coord.el("div", "actions");
      actions.appendChild(Coord.bouton("ord.publier", async function () {
        if (!window.confirm(Coord.t("ord.confirmPublier"))) { return; }
        await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/publier");
        await afficher(contenu, id, false);
      }, "principal"));
      contenu.appendChild(actions);
    }
    if (projet.etat === "PUBLIE") {
      const actions = Coord.el("div", "actions");
      actions.appendChild(Coord.bouton("ord.transmettre", async function () {
        if (!window.confirm(Coord.t("ord.confirmTransmettre"))) { return; }
        await Coord.requete("POST", "/api/projets/" + encodeURIComponent(id) + "/transmettre");
        await afficher(contenu, id, false);
      }, "principal"));
      contenu.appendChild(actions);
    }
  }

  /* ---- section suivi : mission fille + journal ---- */
  async function sectionSuivi(contenu, id) {
    const section = Coord.el("div", null);
    section.appendChild(Coord.el("h2", null, "sui.titre"));
    if (projet.etat === "TRANSMIS") {
      const fille = await Coord.requete("GET", "/api/projets/" + encodeURIComponent(id) + "/fille");
      if (fille.statut === 200 && fille.donnees) {
        const carte = Coord.el("div", "carte");
        ajouterChamp(carte, "sui.filleIssue", fille.donnees.issue);
        ajouterChamp(carte, "sui.filleExecutant", fille.donnees.executant);
        if (projet.devis) { ajouterChamp(carte, "sui.livrable", projet.devis.livrable); }
        section.appendChild(carte);
      }
    } else {
      const absence = Coord.el("div", "aide");
      absence.textContent = Coord.t("sui.filleAbsente");
      section.appendChild(absence);
    }
    /* Journal : dernière entrée en haut. */
    section.appendChild(Coord.el("h2", null, "sui.journal"));
    const journal = Coord.el("div", "journal");
    journalBrut.slice().reverse().forEach(function (e) {
      const ligne = Coord.el("div", "entree");
      const type = Coord.el("span", "type");
      type.textContent = e.type || "?";
      ligne.appendChild(type);
      const reste = JSON.stringify(Object.assign({}, e, { type: undefined, instant_utc: undefined }));
      ligne.appendChild(document.createTextNode(" " + (reste === "{}" ? "" : reste) + " " + (e.instant_utc || "")));
      journal.appendChild(ligne);
    });
    section.appendChild(journal);
    contenu.appendChild(section);
  }

  async function afficher(contenu, id, recharger) {
    if (recharger !== false) {
      const p = await charger(id);
      if (!p) {
        contenu.appendChild(Coord.el("h1", null, "sui.titre"));
        Coord.messageErreur(contenu, "erreur projet inconnu : " + id);
        contenu.appendChild(Coord.bouton("app.retour", function () { Coord.naviguer("#/accueil"); }));
        return;
      }
    }
    entete(contenu);
    if (projet.etat === "DIALOGUE") { sectionDialogue(contenu, id); }
    if (projet.etat === "ORDRE_PROPOSE" || projet.etat === "ORDRE_QUALIFIE" ||
        projet.etat === "VALIDE" || projet.etat === "PUBLIE" || projet.etat === "TRANSMIS") {
      sectionOrdre(contenu, id);
    }
    sectionDevis(contenu, id);
    sectionPilote(contenu, id);
    await sectionSuivi(contenu, id);
  }
  Coord.ecrans.mission = afficher;
})();

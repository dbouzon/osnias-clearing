"use strict";
/* Écran CRÉATION (M-COORD-AUTH-01, spec v0.5 §3.5) : Nom → Intention
 * → RÉPERTOIRE DE LA MISSION (choisir OU créer un dossier dédié sur le
 * Drive de la personne) → « Commencer le cadrage gratuit ».
 *
 * Le répertoire dédié est unique pour toute la mission : COORD y crée
 * les sous-dossiers « entrée » (documents sources) et « sortie »
 * (livrables), puis associe le répertoire à la mission via
 * POST /api/missions/repertoire (garde-fou serveur : un répertoire =
 * une seule mission). Le scope Drive est demandé au PREMIER usage :
 * si le Drive n'est pas encore connecté, l'écran affiche « Connecter
 * mon Drive » et le parcours reprend sans perte après consentement.
 *
 * Repli fail-closed : sans Google actif, l'écran historique (chemins
 * libres entree/sortie) reste utilisable (mode maintenance). */
(function () {
  var dossierChoisi = null; /* {id, nom} répertoire dédié de la mission */

  function champTexte(cible, cleLabel, id, aide) {
    cible.appendChild(Coord.el("label", null, cleLabel));
    const input = document.createElement("input");
    input.type = "text";
    input.id = id;
    cible.appendChild(input);
    if (aide) { cible.appendChild(Coord.el("div", "aide", aide)); }
    return input;
  }

  /* Bloc « Répertoire de la mission » : connecter OU choisir OU créer. */
  function blocRepertoire(contenu, carte, erreur) {
    const bloc = Coord.el("div");
    carte.appendChild(bloc);

    function afficherConnexionDrive() {
      bloc.textContent = "";
      const b = document.createElement("a");
      b.className = "bouton principal";
      b.href = "/api/auth/google/login?drive=1";
      b.textContent = Coord.t("creation.connecterDrive");
      bloc.appendChild(b);
      bloc.appendChild(Coord.el("div", "aide", "creation.connecterDriveAide"));
    }

    function afficherChoix(dossiers) {
      bloc.textContent = "";
      bloc.appendChild(Coord.el("label", null, "creation.repertoireMission"));
      const select = document.createElement("select");
      select.id = "creer-repertoire";
      const vide = document.createElement("option");
      vide.value = "";
      vide.textContent = Coord.t("creation.choisirDossier");
      select.appendChild(vide);
      dossiers.forEach(function (d) {
        const o = document.createElement("option");
        o.value = d.id;
        o.textContent = d.nom;
        select.appendChild(o);
      });
      bloc.appendChild(select);
      const aide = Coord.el("div", "aide", "creation.repertoireAide");
      bloc.appendChild(aide);

      /* Créer un dossier dédié sans quitter l'écran (spec §3.5). */
      bloc.appendChild(Coord.el("label", null, "creation.creerDossier"));
      const input = document.createElement("input");
      input.type = "text";
      input.id = "creer-repertoire-nouveau";
      input.placeholder = Coord.t("creation.creerDossierPlaceholder");
      bloc.appendChild(input);
      const bCreer = Coord.bouton("creation.creerDossierBouton", async function () {
        const nom = input.value.trim();
        if (!nom) { Coord.messageErreur(erreur, "creation.errNomDossier"); return; }
        bCreer.disabled = true;
        const rep = await Coord.requete("POST", "/api/drive/dossier", { nom: nom });
        bCreer.disabled = false;
        if (rep.statut !== 200 || !rep.donnees || !rep.donnees.id) {
          Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
          return;
        }
        dossierChoisi = { id: rep.donnees.id, nom: rep.donnees.nom || nom };
        bloc.textContent = "";
        bloc.appendChild(Coord.el("label", null, "creation.repertoireMission"));
        const ok = document.createElement("div");
        ok.className = "aide";
        ok.textContent = "📁 " + dossierChoisi.nom;
        bloc.appendChild(ok);
        bloc.appendChild(Coord.el("div", "aide", "creation.repertoireConfirme"));
      });
      bloc.appendChild(bCreer);

      select.addEventListener("change", function () {
        if (select.value) {
          const o = select.selectedOptions[0];
          dossierChoisi = { id: select.value, nom: o ? o.textContent : select.value };
        } else {
          dossierChoisi = null;
        }
      });
    }

    return async function charger() {
      const etat = await Coord.requete("GET", "/api/auth/google/etat");
      if (!(etat.statut === 200 && etat.donnees && etat.donnees.activer)) {
        bloc.textContent = "";
        bloc.appendChild(Coord.el("div", "aide", "creation.repertoireIndisponible"));
        return false; /* mode maintenance : écran historique */
      }
      if (!etat.donnees.drive_connecte) {
        afficherConnexionDrive();
        return false;
      }
      const rep = await Coord.requete("GET", "/api/drive/dossiers");
      if (rep.statut !== 200 || !rep.donnees || !rep.donnees.dossiers) {
        Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
        afficherConnexionDrive();
        return false;
      }
      afficherChoix(rep.donnees.dossiers);
      return true;
    };
  }

  async function afficher(contenu) {
    contenu.appendChild(Coord.el("h1", null, "creation.titre"));
    const carte = Coord.el("div", "carte");
    const erreur = Coord.el("div");

    champTexte(carte, "creation.nom", "creer-nom", "creation.nomAide");
    const intention = Coord.el("label", null, "creation.intention");
    carte.appendChild(intention);
    const zone = document.createElement("textarea");
    zone.id = "creer-intention";
    zone.placeholder = Coord.t("creation.intentionPlaceholder");
    carte.appendChild(zone);

    const chargerRepertoire = blocRepertoire(contenu, carte, erreur);
    const modeGoogle = await chargerRepertoire();

    /* Mode maintenance (sans Google) : champs historiques libres. */
    var champEntree = null;
    var champSortie = null;
    if (!modeGoogle) {
      champEntree = champTexte(carte, "creation.dossierEntree", "creer-entree", "creation.dossierEntreeAide");
      champSortie = champTexte(carte, "creation.dossierSortie", "creer-sortie", "creation.dossierSortieAide");
    }
    const directeur = champTexte(carte, "creation.directeur", "creer-directeur");
    directeur.value = "openai-dir";
    const executant = champTexte(carte, "creation.executant", "creer-executant");
    executant.value = "openai-exec";

    carte.appendChild(Coord.el("div", "aide", "creation.cadrageGratuit"));
    contenu.appendChild(carte);
    contenu.appendChild(erreur);

    contenu.appendChild(Coord.bouton("creation.commencer", async function () {
      erreur.textContent = "";
      const nom = document.getElementById("creer-nom").value.trim();
      const textIntention = document.getElementById("creer-intention").value.trim();
      if (!nom) { Coord.messageErreur(erreur, "creation.errNom"); return; }
      if (!textIntention) { Coord.messageErreur(erreur, "creation.errIntention"); return; }

      var entree;
      var sortie;
      if (modeGoogle) {
        /* Repertoire dédié OBLIGATOIRE (fail-closed spec §3.5) :
         * choisir OU créer, puis créer les sous-dossiers entrée et
         * sortie DANS le répertoire dédié. */
        if (!dossierChoisi) { Coord.messageErreur(erreur, "creation.errRepertoire"); return; }
        Coord.messageErreur(erreur, "creation.preparationRepertoire");
        const sousEntree = await Coord.requete("POST", "/api/drive/dossier",
          { nom: "entrée", parent: dossierChoisi.id });
        if (sousEntree.statut !== 200) {
          erreur.textContent = "";
          Coord.messageErreur(erreur, sousEntree.donnees && sousEntree.donnees.motif ?
            sousEntree.donnees.motif : "app.erreurReseau");
          return;
        }
        const sousSortie = await Coord.requete("POST", "/api/drive/dossier",
          { nom: "sortie", parent: dossierChoisi.id });
        if (sousSortie.statut !== 200) {
          erreur.textContent = "";
          Coord.messageErreur(erreur, sousSortie.donnees && sousSortie.donnees.motif ?
            sousSortie.donnees.motif : "app.erreurReseau");
          return;
        }
        erreur.textContent = "";
        /* Association mission → répertoire AVANT création (garde-fou
         * serveur : un répertoire = une seule mission). */
        const assoc = await Coord.requete("POST", "/api/missions/repertoire", {
          mission: nom,
          dossier_id: dossierChoisi.id,
          dossier_nom: dossierChoisi.nom
        });
        if (assoc.statut !== 200) {
          Coord.messageErreur(erreur, assoc.donnees && assoc.donnees.motif ?
            assoc.donnees.motif : "app.erreurReseau");
          return;
        }
        entree = dossierChoisi.nom + "/entrée";
        sortie = dossierChoisi.nom + "/sortie";
      } else {
        entree = document.getElementById("creer-entree").value.trim();
        sortie = document.getElementById("creer-sortie").value.trim();
        if (!entree) { Coord.messageErreur(erreur, "creation.errDossierEntree"); return; }
        if (!sortie) { Coord.messageErreur(erreur, "creation.errDossierSortie"); return; }
      }

      /* 1. Vérification serveur des deux dossiers AVANT toute
       * création : un dossier inaccessible bloque le parcours avec
       * un message compréhensible, rien n'est créé. */
      Coord.messageErreur(erreur, "creation.verifEnCours");
      const verif = await Coord.requete("POST", "/api/drive/verifier", {
        dossier_entree: entree,
        dossier_sortie: sortie
      });
      erreur.textContent = "";
      if (verif.statut === 403 && verif.donnees && verif.donnees.motif) {
        Coord.messageErreur(erreur, "verif.ecritureDesactivee", { motif: verif.donnees.motif });
        return;
      }
      if (verif.statut !== 200 || !verif.donnees) {
        Coord.messageErreur(erreur, verif.donnees && verif.donnees.motif ?
          "verif.impossible" : "app.erreurReseau",
        verif.donnees && verif.donnees.motif ? { motif: verif.donnees.motif } : undefined);
        return;
      }
      const accesEntree = verif.donnees.entree || {};
      const accesSortie = verif.donnees.sortie || {};
      if (accesEntree.ok !== true) {
        Coord.messageErreur(erreur, "verif.entreeKo", { motif: accesEntree.motif || "?" });
        return;
      }
      if (accesSortie.ok !== true) {
        Coord.messageErreur(erreur, "verif.sortieKo", { motif: accesSortie.motif || "?" });
        return;
      }

      /* 2. Vérification réussie : création, cadrage (dossiers Drive
       * reliés dès la création), ouverture du dialogue. */
      const rep = await Coord.requete("POST", "/api/projets", {
        id: nom,
        intention: textIntention
      });
      if (rep.statut !== 201) {
        Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
        return;
      }
      const rCadrage = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(nom) + "/cadrage", {
        dossier_entree: entree,
        dossier_sortie: sortie,
        dossier_entree_absent: false
      });
      if (rCadrage.statut !== 200) {
        Coord.messageErreur(erreur, rCadrage.donnees && rCadrage.donnees.motif ? rCadrage.donnees.motif : "app.erreurReseau");
        return;
      }
      const rDialogue = await Coord.requete("POST", "/api/projets/" + encodeURIComponent(nom) + "/dialogue", {
        directeur: document.getElementById("creer-directeur").value.trim(),
        executant: document.getElementById("creer-executant").value.trim()
      });
      if (rDialogue.statut !== 200) {
        Coord.messageErreur(erreur, rDialogue.donnees && rDialogue.donnees.motif ? rDialogue.donnees.motif : "app.erreurReseau");
        return;
      }
      Coord.naviguer("#/mission/" + encodeURIComponent(nom));
    }, "principal"));
  }
  Coord.ecrans.creation = afficher;
})();

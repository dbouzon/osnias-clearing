"use strict";
/* Écran CRÉATION (M-COORD-SRC-01, spec v0.3) : Nom → Intention →
 * SOURCE du répertoire de mission (Google Drive OU GitHub) →
 * « Commencer le cadrage gratuit ».
 *
 * RÈGLE D'HOMOGÉNÉITÉ (pilote) : l'entrée et la sortie vivent dans la
 * MÊME source — documents (PDF/DOCX) → Drive ; repo GitHub → GitHub.
 * Vérifiée aussi côté serveur (409 SOURCE_HETEROGENE).
 *
 *   - Source Drive   : choisir OU créer un dossier dédié, sous-dossiers
 *     entrée/sortie créés par COORD (AUTH-01 §3.5) ;
 *   - Source GitHub  : se connecter avec SON compte GitHub, choisir un
 *     repo public, COORD crée missions/<nom>/entrée et missions/<nom>/
 *     sortie dans le repo (contenu versionné dès la création).
 *
 * Repli fail-closed : chaque source ne s'affiche que si son état
 * serveur est actif ; sans aucune, l'écran historique (chemins libres)
 * reste disponible (mode maintenance). */
(function () {
  var dossierChoisi = null;   /* {id, nom} répertoire dédié Drive */
  var repoChoisi = null;      /* "proprio/repo" source GitHub */
  var sourceActive = "drive"; /* "drive" | "github" */

  function champTexte(cible, cleLabel, id, aide) {
    cible.appendChild(Coord.el("label", null, cleLabel));
    const input = document.createElement("input");
    input.type = "text";
    input.id = id;
    cible.appendChild(input);
    if (aide) { cible.appendChild(Coord.el("div", "aide", aide)); }
    return input;
  }

  /* --- Sélecteur de source (Drive / GitHub) ------------------------- */

  function blocSource(carte, etats, erreur) {
    var sources = [];
    if (etats.drive) { sources.push("drive"); }
    if (etats.github) { sources.push("github"); }
    if (sources.length === 0) { return null; }
    if (sources.length === 1) { sourceActive = sources[0]; return carte; }

    const bloc = Coord.el("div");
    carte.appendChild(bloc);
    bloc.appendChild(Coord.el("label", null, "creation.source"));
    const select = document.createElement("select");
    select.id = "creer-source";
    sources.forEach(function (s) {
      const o = document.createElement("option");
      o.value = s;
      o.textContent = Coord.t(s === "drive" ? "creation.sourceDrive" : "creation.sourceGithub");
      if (s === sourceActive) { o.selected = true; }
      select.appendChild(o);
    });
    bloc.appendChild(select);
    bloc.appendChild(Coord.el("div", "aide", "creation.sourceAide"));
    select.addEventListener("change", function () {
      sourceActive = select.value;
      blocDrive.textContent = "";
      blocGithub.textContent = "";
      if (sourceActive === "drive") { chargerDrive(); } else { chargerGithub(); }
    });
    return bloc;
  }

  /* --- Source Drive : répertoire dédié (AUTH-01) ------------------- */

  var blocDrive = null;
  var blocGithub = null;
  var chargerDrive = function () {};

  function preparerBlocDrive(carte, erreur, driveConnecte) {
    blocDrive = Coord.el("div");
    carte.appendChild(blocDrive);
    chargerDrive = function () {
      blocDrive.textContent = "";
      if (!driveConnecte) {
        const b = document.createElement("a");
        b.className = "bouton principal";
        b.href = "/api/auth/google/login?drive=1";
        b.textContent = Coord.t("creation.connecterDrive");
        blocDrive.appendChild(b);
        blocDrive.appendChild(Coord.el("div", "aide", "creation.connecterDriveAide"));
        return;
      }
      (async function () {
        const rep = await Coord.requete("GET", "/api/drive/dossiers");
        if (rep.statut !== 200 || !rep.donnees || !rep.donnees.dossiers) {
          Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
          return;
        }
        blocDrive.appendChild(Coord.el("label", null, "creation.repertoireMission"));
        const select = document.createElement("select");
        select.id = "creer-repertoire";
        const vide = document.createElement("option");
        vide.value = "";
        vide.textContent = Coord.t("creation.choisirDossier");
        select.appendChild(vide);
        rep.donnees.dossiers.forEach(function (d) {
          const o = document.createElement("option");
          o.value = d.id;
          o.textContent = d.nom;
          select.appendChild(o);
        });
        blocDrive.appendChild(select);
        blocDrive.appendChild(Coord.el("div", "aide", "creation.repertoireAide"));
        blocDrive.appendChild(Coord.el("label", null, "creation.creerDossier"));
        const input = document.createElement("input");
        input.type = "text";
        input.id = "creer-repertoire-nouveau";
        input.placeholder = Coord.t("creation.creerDossierPlaceholder");
        blocDrive.appendChild(input);
        const bCreer = Coord.bouton("creation.creerDossierBouton", async function () {
          const nom = input.value.trim();
          if (!nom) { Coord.messageErreur(erreur, "creation.errNomDossier"); return; }
          bCreer.disabled = true;
          const r = await Coord.requete("POST", "/api/drive/dossier", { nom: nom });
          bCreer.disabled = false;
          if (r.statut !== 200 || !r.donnees || !r.donnees.id) {
            Coord.messageErreur(erreur, r.donnees && r.donnees.motif ? r.donnees.motif : "app.erreurReseau");
            return;
          }
          dossierChoisi = { id: r.donnees.id, nom: r.donnees.nom || nom };
          blocDrive.appendChild(Coord.el("div", "aide", "creation.repertoireConfirme"));
        });
        blocDrive.appendChild(bCreer);
        select.addEventListener("change", function () {
          if (select.value) {
            const o = select.selectedOptions[0];
            dossierChoisi = { id: select.value, nom: o ? o.textContent : select.value };
          } else {
            dossierChoisi = null;
          }
        });
      })();
    };
  }

  /* --- Source GitHub : repo public (SRC-01) ------------------------ */

  var chargerGithub = function () {};

  function preparerBlocGithub(carte, erreur, githubConnecte) {
    blocGithub = Coord.el("div");
    carte.appendChild(blocGithub);
    chargerGithub = function () {
      blocGithub.textContent = "";
      if (!githubConnecte) {
        const b = document.createElement("a");
        b.className = "bouton principal";
        b.href = "/api/auth/github/login";
        b.textContent = Coord.t("creation.connecterGithub");
        blocGithub.appendChild(b);
        blocGithub.appendChild(Coord.el("div", "aide", "creation.connecterGithubAide"));
        return;
      }
      (async function () {
        const rep = await Coord.requete("GET", "/api/github/repos");
        if (rep.statut !== 200 || !rep.donnees || !rep.donnees.repos) {
          Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "creation.reposErreur");
          return;
        }
        blocGithub.appendChild(Coord.el("label", null, "creation.repoMission"));
        const select = document.createElement("select");
        select.id = "creer-repo";
        const vide = document.createElement("option");
        vide.value = "";
        vide.textContent = Coord.t("creation.choisirRepo");
        select.appendChild(vide);
        rep.donnees.repos.forEach(function (r) {
          const o = document.createElement("option");
          o.value = r.id;
          o.textContent = r.nom;
          select.appendChild(o);
        });
        blocGithub.appendChild(select);
        blocGithub.appendChild(Coord.el("div", "aide", "creation.repoAide"));
        select.addEventListener("change", function () {
          repoChoisi = select.value || null;
        });
      })();
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

    /* États serveur des deux sources (fail-closed : absentes = off). */
    var etats = { drive: false, github: false, driveConnecte: false, githubConnecte: false };
    try {
      const e1 = await Coord.requete("GET", "/api/auth/google/etat");
      if (e1.statut === 200 && e1.donnees && e1.donnees.activer) {
        etats.drive = true;
        etats.driveConnecte = e1.donnees.drive_connecte === true;
      }
    } catch (e) { /* silencieux : repli historique */ }
    try {
      const e2 = await Coord.requete("GET", "/api/auth/github/etat");
      if (e2.statut === 200 && e2.donnees && e2.donnees.activer) {
        etats.github = true;
        etats.githubConnecte = e2.donnees.github_connecte === true;
      }
    } catch (e) { /* silencieux */ }

    /* Règle du pilote : le moyen de connexion désigne la source.
     * Compte Gmail -> Drive ; compte GitHub -> repo. La source par
     * défaut suit l'origine, l'autre reste sélectionnable. */
    try {
      const moi = await Coord.requete("GET", "/api/moi");
      if (moi.statut === 200 && moi.donnees && moi.donnees.origine === "github") {
        if (etats.github) { sourceActive = "github"; }
      } else if (etats.drive) {
        sourceActive = "drive";
      }
    } catch (e) { /* repli : drive par défaut */ }
    const modeSources = blocSource(carte, etats, erreur) !== null;
    var modeGoogle = false;
    if (modeSources) {
      preparerBlocDrive(carte, erreur, etats.driveConnecte);
      preparerBlocGithub(carte, erreur, etats.githubConnecte);
      if (sourceActive === "drive") { chargerDrive(); } else { chargerGithub(); }
      modeGoogle = true;
    }

    /* Mode maintenance (sans aucune source) : champs historiques libres. */
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
      var source = "drive";
      if (modeGoogle) {
        if (sourceActive === "github") {
          /* Source GitHub : structure missions/<nom>/entrée|sortie créée
           * DANS le repo, puis association avec source "github"
           * (homogénéité : entrée et sortie dans le repo). */
          if (!repoChoisi) { Coord.messageErreur(erreur, "creation.errRepo"); return; }
          source = "github";
          Coord.messageErreur(erreur, "creation.preparationRepertoire");
          const st = await Coord.requete("POST", "/api/github/structure", {
            mission: nom, repo: repoChoisi
          });
          erreur.textContent = "";
          if (st.statut !== 200 || !st.donnees || !st.donnees.id) {
            Coord.messageErreur(erreur, st.donnees && st.donnees.motif ? st.donnees.motif : "app.erreurReseau");
            return;
          }
          const assoc = await Coord.requete("POST", "/api/missions/repertoire", {
            mission: nom, source: "github",
            dossier_id: st.donnees.id, dossier_nom: st.donnees.nom
          });
          if (assoc.statut !== 200) {
            Coord.messageErreur(erreur, assoc.donnees && assoc.donnees.motif ? assoc.donnees.motif : "app.erreurReseau");
            return;
          }
          entree = "missions/" + nom + "/entrée";
          sortie = "missions/" + nom + "/sortie";
        } else {
          /* Source Drive (AUTH-01) : répertoire dédié obligatoire. */
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
          const assoc = await Coord.requete("POST", "/api/missions/repertoire", {
            mission: nom, source: "drive",
            dossier_id: dossierChoisi.id, dossier_nom: dossierChoisi.nom
          });
          if (assoc.statut !== 200) {
            Coord.messageErreur(erreur, assoc.donnees && assoc.donnees.motif ?
              assoc.donnees.motif : "app.erreurReseau");
            return;
          }
          entree = dossierChoisi.nom + "/entrée";
          sortie = dossierChoisi.nom + "/sortie";
        }
      } else {
        entree = document.getElementById("creer-entree").value.trim();
        sortie = document.getElementById("creer-sortie").value.trim();
        if (!entree) { Coord.messageErreur(erreur, "creation.errDossierEntree"); return; }
        if (!sortie) { Coord.messageErreur(erreur, "creation.errDossierSortie"); return; }
      }

      /* 1. Vérification serveur des deux dossiers AVANT toute
       * création (fail-closed, inchangé depuis M-COORD-UI-01). */
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

      /* 2. Vérification réussie : création, cadrage, dialogue. */
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

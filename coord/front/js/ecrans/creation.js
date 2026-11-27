"use strict";
/* Écran CRÉATION (mission M-COORD-UI-01, S4) : Nom → Intention →
 * Dossier d'entrée OBLIGATOIRE → Dossier de rendu OBLIGATOIRE →
 * « Commencer le cadrage gratuit ».
 *
 * La vérification d'accès est SERVEUR (POST /api/drive/verifier) :
 * lecture du dossier d'entrée (Lister), sonde d'écriture dans le
 * dossier de rendu (COORD-test-acces.txt écrit puis supprimé). Un
 * échec de vérification affiche un message compréhensible PAR
 * DOSSIER et RIEN n'est créé. L'intention reste requise par l'API
 * (une mission sans intention n'a pas de sens) ; l'écart avec la
 * lettre du pilote est signalé et justifié. */
(function () {
  function champTexte(cible, cleLabel, id, aide) {
    cible.appendChild(Coord.el("label", null, cleLabel));
    const input = document.createElement("input");
    input.type = "text";
    input.id = id;
    cible.appendChild(input);
    if (aide) { cible.appendChild(Coord.el("div", "aide", aide)); }
    return input;
  }

  async function afficher(contenu) {
    contenu.appendChild(Coord.el("h1", null, "creation.titre"));
    const carte = Coord.el("div", "carte");

    champTexte(carte, "creation.nom", "creer-nom", "creation.nomAide");
    const intention = Coord.el("label", null, "creation.intention");
    carte.appendChild(intention);
    const zone = document.createElement("textarea");
    zone.id = "creer-intention";
    zone.placeholder = Coord.t("creation.intentionPlaceholder");
    carte.appendChild(zone);

    champTexte(carte, "creation.dossierEntree", "creer-entree", "creation.dossierEntreeAide");
    champTexte(carte, "creation.dossierSortie", "creer-sortie", "creation.dossierSortieAide");
    const directeur = champTexte(carte, "creation.directeur", "creer-directeur");
    directeur.value = "openai-dir";
    const executant = champTexte(carte, "creation.executant", "creer-executant");
    executant.value = "openai-exec";

    carte.appendChild(Coord.el("div", "aide", "creation.cadrageGratuit"));
    contenu.appendChild(carte);

    const erreur = Coord.el("div");
    contenu.appendChild(erreur);
    contenu.appendChild(Coord.bouton("creation.commencer", async function () {
      erreur.textContent = "";
      const nom = document.getElementById("creer-nom").value.trim();
      const textIntention = document.getElementById("creer-intention").value.trim();
      const entree = document.getElementById("creer-entree").value.trim();
      const sortie = document.getElementById("creer-sortie").value.trim();
      if (!nom) { Coord.messageErreur(erreur, "creation.errNom"); return; }
      if (!textIntention) { Coord.messageErreur(erreur, "creation.errIntention"); return; }
      if (!entree) { Coord.messageErreur(erreur, "creation.errDossierEntree"); return; }
      if (!sortie) { Coord.messageErreur(erreur, "creation.errDossierSortie"); return; }

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

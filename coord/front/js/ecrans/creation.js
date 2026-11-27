"use strict";
/* Écran CRÉATION : nom + intention + dossier d'entrée Drive (optionnel,
 * l'absence est journalisée) + dossier de sortie + binôme
 * directeur/exécutant. Cadrage gratuit affiché comme tel (D8). */
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

    champTexte(carte, "creation.dossierEntree", "creer-entree");
    champTexte(carte, "creation.dossierSortie", "creer-sortie");
    const directeur = champTexte(carte, "creation.directeur", "creer-directeur");
    directeur.value = "openai-dir";
    const executant = champTexte(carte, "creation.executant", "creer-executant");
    executant.value = "openai-exec";

    carte.appendChild(Coord.el("div", "aide", "creation.cadrageGratuit"));
    contenu.appendChild(carte);

    const erreur = Coord.el("div");
    contenu.appendChild(erreur);
    contenu.appendChild(Coord.bouton("creation.creer", async function () {
      erreur.textContent = "";
      const nom = document.getElementById("creer-nom").value.trim();
      const textIntention = document.getElementById("creer-intention").value.trim();
      if (!nom) { Coord.messageErreur(erreur, "creation.errNom"); return; }
      if (!textIntention) { Coord.messageErreur(erreur, "creation.errIntention"); return; }
      const rep = await Coord.requete("POST", "/api/projets", {
        id: nom,
        intention: textIntention
      });
      if (rep.statut !== 201) {
        Coord.messageErreur(erreur, rep.donnees && rep.donnees.motif ? rep.donnees.motif : "app.erreurReseau");
        return;
      }
      /* Cadrage gratuit : dossiers Drive du client + mention explicite
       * de l'absence éventuelle du dossier d'entrée (plan v2 §5.3). */
      const entree = document.getElementById("creer-entree").value.trim();
      const sortie = document.getElementById("creer-sortie").value.trim();
      await Coord.requete("POST", "/api/projets/" + encodeURIComponent(nom) + "/cadrage", {
        dossier_entree: entree,
        dossier_sortie: sortie,
        dossier_entree_absent: entree === ""
      });
      /* Ouverture du dialogue avec le binôme saisi. */
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

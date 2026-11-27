"use strict";
/* COORD front — exécution de contrôle (appelé par test-ui.py quand
 * node est disponible) : charge les tables de langue, le noyau et les
 * écrans DANS L'ORDRE DE index.html, puis démarre l'application.
 * Réussit uniquement si : aucun script ne lève d'exception, les 5
 * tables de langue sont remplies, demarrer() termine, et #contenu
 * est remplacé (l'application ne reste pas sur « Chargement… »).
 * Usage : node test-ui-exec.js [dossier-du-front] */

const fs = require("fs");
const path = require("path");

const racine = path.resolve(process.argv[2] || ".");
require(path.join(racine, "test-ui-stub.js"));

const scripts = [
  "js/langues/fr.js", "js/langues/en.js", "js/langues/de.js",
  "js/langues/es.js", "js/langues/zh.js",
  "js/app.js", "js/colonne-missions.js",
  "js/ecrans/connexion.js", "js/ecrans/accueil.js", "js/ecrans/creation.js",
  "js/ecrans/mission.js", "js/ecrans/presentation.js", "js/ecrans/apropos.js"
];

function echec(message) {
  console.error("ECHEC EXECUTION JS : " + message);
  process.exit(1);
}

/* Fidèle au navigateur : on exécute les scripts de index.html DANS
 * L'ORDRE (inline compris — c'est là que vit « var CoordLangues »).
 * Le bootstrap DOMContentLoaded est remplacé par l'appel direct ci-dessous. */
const html = fs.readFileSync(path.join(racine, "index.html"), "utf8");
const motifScript = /<script(?:\s+src="([^"]+)")?[^>]*>([\s\S]*?)<\/script>/g;
let m;
while ((m = motifScript.exec(html)) !== null) {
  if (m[1]) {
    /* global eval : les modules du front s'installent sur window */
    const fichier = m[1];
    if (!fs.existsSync(path.join(racine, fichier))) {
      echec("index.html référence un script absent : " + fichier);
    }
    try {
      (0, eval)(fs.readFileSync(path.join(racine, fichier), "utf8"));
    } catch (e) {
      echec(fichier + " lève " + e.constructor.name + " : " + e.message);
    }
  } else if (m[2] && m[2].indexOf("Coord.demarrer") === -1 && m[2].trim() !== "") {
    try {
      (0, eval)(m[2]);
    } catch (e) {
      echec("script inline de index.html lève " + e.constructor.name + " : " + e.message);
    }
  }
}

(async function () {
  if (typeof global.Coord === "undefined") { echec("window.Coord absent après chargement"); }
  const langues = global.window.CoordLangues;
  ["fr", "en", "de", "es", "zh"].forEach(function (l) {
    if (!langues[l] || Object.keys(langues[l]).length === 0) {
      echec("table de langue vide ou absente : " + l);
    }
  });
  try {
    await global.Coord.demarrer();
  } catch (e) {
    echec("Coord.demarrer() lève " + e.constructor.name + " : " + e.message);
  }
  const contenu = global.document.getElementById("contenu");
  if (!contenu || contenu.children.length === 0) {
    echec("#contenu vide : l'application est figée sur « Chargement… »");
  }
  if (global.Coord.t("app.title") !== "COORD by COORD") {
    echec("i18n inopérant : t('app.title') = " + JSON.stringify(global.Coord.t("app.title")));
  }
  console.log("EXECUTION_JS_OK");
})().catch(function (e) { echec(e && e.stack ? e.stack : String(e)); });

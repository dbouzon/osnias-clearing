"use strict";
/* COORD front — mini-DOM d'exécution (mission M-COORD-UI-01, incident
 * recette mobile). Le harnais test-ui.py vérifiait la syntaxe et la
 * parité des tables mais n'EXÉCUTAIT jamais le JavaScript : le défaut
 * « ReferenceError: CoordLangues is not defined » (aucun fichier ne
 * déclarait la variable avant les js/langues/*.js) a tenu de la v1 à
 * la v3 sans être vu, figeant le navigateur sur « Chargement… ».
 * Ce module fournit juste assez de DOM pour exécuter app.js et les
 * écrans hors navigateur ; il ne remplace pas la recette réelle. */

function noeud(tag) {
  return {
    tagName: tag, children: [], className: "", textContent: "", value: "",
    attrs: {}, listeners: {},
    get childNodes() { return this.children; },
    appendChild: function (c) { this.children.push(c); return c; },
    setAttribute: function (k, v) { this.attrs[k] = v; },
    getAttribute: function (k) { return this.attrs[k]; },
    addEventListener: function (t, f) {
      (this.listeners[t] = this.listeners[t] || []).push(f);
    },
    classList: {
      _s: [],
      add: function (c) { if (this._s.indexOf(c) === -1) { this._s.push(c); } },
      remove: function (c) { this._s = this._s.filter(function (x) { return x !== c; }); },
      toggle: function (c) { this.contains(c) ? this.remove(c) : this.add(c); },
      contains: function (c) { return this._s.indexOf(c) !== -1; }
    },
    insertBefore: function (n) { this.children.unshift(n); },
    querySelector: function () { return null; },
    querySelectorAll: function () { return []; },
    contains: function () { return false; },
    removeAttribute: function () {}
  };
}

const parId = {};

global.document = {
  title: "",
  documentElement: noeud("html"),
  getElementById: function (id) {
    if (!parId[id]) { parId[id] = noeud("div"); }
    return parId[id];
  },
  createElement: function (tag) { return noeud(tag); },
  createTextNode: function (t) { return { text: t }; },
  querySelector: function () { return null; },
  querySelectorAll: function () { return []; },
  addEventListener: function () {},
  cookie: ""
};

global.window = global;
global.window.location = { hash: process.env.COORD_HASH || "#/accueil", reload: function () {} };
/* node ≥ 21 expose navigator en propriété sans setter : passer par
 * defineProperty pour le stub. */
Object.defineProperty(global, "navigator", {
  value: { languages: ["fr"] }, configurable: true, writable: true
});
global.window.navigator = global.navigator;
global.window.localStorage = {
  _m: {},
  getItem: function (k) { return global.window.localStorage._m[k] || null; },
  setItem: function (k, v) { global.window.localStorage._m[k] = String(v); }
};
global.window.addEventListener = function () {};
global.window.confirm = function () { return true; };
global.window.prompt = function () { return "motif de test"; };
global.navigator = global.window.navigator;

/* Moteur démo simulé : réponses minimales du gateway pour un parcours
 * connecté (session demo, une mission DIALOGUE). */
global.window.fetch = async function (chemin) {
  let objet = null;
  let texte = "";
  const repondre = function (o) { objet = o; texte = JSON.stringify(o); };
  if (chemin === "/api/session") { repondre({ compte: "demo", demo: true, dossier_drive: "" }); }
  else if (chemin === "/api/comptes") { repondre({ comptes: ["demo"] }); }
  else if (chemin === "/api/projets") { repondre({ projets: ["mission-essai"] }); }
  else if (chemin === "/api/projets/mission-essai") {
    repondre({
      id: "mission-essai", etat: "DIALOGUE", version: 1,
      cadrage: { dossier_entree: "entree", dossier_sortie: "rendu" }
    });
  } else if (chemin.indexOf("/journal") !== -1) {
    texte = JSON.stringify({ type: "TOUR_CLIENT", entree: "essai" }) + "\n";
  } else if (chemin === "/api/drive/verifier") {
    repondre({ entree: { ok: true }, sortie: { ok: true } });
  } else { repondre({}); }
  return {
    status: 200,
    ok: true,
    text: async function () { return texte; },
    json: async function () { return objet; }
  };
};

module.exports = {
  contenu: function () { return global.document.getElementById("contenu"); },
  langues: function () { return global.window.CoordLangues || {}; }
};

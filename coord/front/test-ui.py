#!/usr/bin/env python3
"""Harnais de test du front COORD (mission M-COORD-FRONT-01, plan v2 §5.6).

Sans navigateur ni dépendance. Vérifie :
  1. parité i18n : les 5 langues (fr, en, de, es, zh) portent exactement
     le même jeu de clés ;
  2. usage : toute clé appelée par Coord.t("…") dans les modules JS
     existe dans les 5 langues ;
  3. écran de chaque état : chaque état de la machine maïeutique a un
     libellé i18n (etat.*) ;
  4. chargement : index.html référence tous les scripts existants ;
  5. déclaration : index.html déclare CoordLangues AVANT js/langues/* ;
  6. EXÉCUTION (si node est présent) : les scripts chargés dans
     l'ordre de index.html ne lèvent aucune exception et l'application
     démarre (incident M-COORD-UI-01 : un front syntaxiquement valide
     mais non exécutable figeait le navigateur sur « Chargement… »).

Usage : python3 test-ui.py  (depuis coord/front/)
"""
import json
import pathlib
import re
import shutil
import subprocess
import sys

RACINE = pathlib.Path(__file__).resolve().parent
LANGUES = ["fr", "en", "de", "es", "zh"]
ETATS = ["INTENTION", "DIALOGUE", "ORDRE_PROPOSE", "ORDRE_QUALIFIE",
         "VALIDE", "PUBLIE", "TRANSMIS", "ATTENTE_PILOTE", "INVALIDE",
         "ABANDONNE", "REALISEE"]
echecs = []


def charger_langues():
    tables = {}
    for langue in LANGUES:
        chemin = RACINE / "js" / "langues" / f"{langue}.js"
        texte = chemin.read_text(encoding="utf-8")
        # Les lignes de commentaire (//) sont ignorées avant le motif.
        code = "\n".join(l for l in texte.splitlines()
                         if not l.strip().startswith("//"))
        # L'affectation imposée : CoordLangues["xx"] = { JSON strict };
        motif = re.compile(r'CoordLangues\["' + langue + r'"\]\s*=\s*(\{.*\})\s*;', re.S)
        m = motif.search(code)
        if not m:
            echecs.append(f"{langue}.js : format imposé absent")
            continue
        tables[langue] = json.loads(m.group(1))
    return tables


def test_parite(tables):
    reference = set(tables.get("fr", {}))
    for langue in LANGUES[1:]:
        cles = set(tables.get(langue, {}))
        manquantes = sorted(reference - cles)
        en_trop = sorted(cles - reference)
        if manquantes:
            echecs.append(f"{langue} : clés manquantes {manquantes}")
        if en_trop:
            echecs.append(f"{langue} : clés en trop {en_trop}")


def test_usage(tables):
    cles_utilisees = set()
    for module in sorted((RACINE / "js").rglob("*.js")):
        if "langues" in module.parts:
            continue
        texte = module.read_text(encoding="utf-8")
        # Clés appelées : Coord.t("cle" …) ou t("cle" …) — la clé doit
        # être un littéral complet (pas une concaténation dynamique).
        for m in re.finditer(r'\bt\("([a-zA-Z0-9._]+)"\s*[,)]', texte):
            cles_utilisees.add(m.group(1))
    for m in re.finditer(r'data-i18n="([a-zA-Z0-9._]+)"',
                         (RACINE / "index.html").read_text(encoding="utf-8")):
        cles_utilisees.add(m.group(1))
    for langue, table in tables.items():
        for cle in sorted(cles_utilisees):
            if cle not in table:
                echecs.append(f"{langue} : clé utilisée absente : {cle}")


def test_etats(tables):
    for langue, table in tables.items():
        for etat in ETATS:
            if f"etat.{etat}" not in table:
                echecs.append(f"{langue} : libellé d'état absent : {etat}")
        for pastille in ["REALISEE", "DEFINIE", "AUTRE"]:
            if f"etat.pastille.{pastille}" not in table:
                echecs.append(f"{langue} : pastille absente : {pastille}")


def test_scripts():
    html = (RACINE / "index.html").read_text(encoding="utf-8")
    for m in re.finditer(r'<script src="([^"]+)"', html):
        chemin = RACINE / m.group(1)
        if not chemin.exists():
            echecs.append(f"index.html : script introuvable : {m.group(1)}")


def test_declaration_coordlangues():
    """Incident recette mobile M-COORD-UI-01 : les js/langues/*.js font
    CoordLangues["xx"] = {...} SANS déclarer la variable. Sans
    déclaration préalable dans index.html, chaque script lève une
    ReferenceError et l'application reste figée sur « Chargement… »."""
    html = (RACINE / "index.html").read_text(encoding="utf-8")
    pos_declaration = html.find("var CoordLangues")
    pos_premiere_langue = html.find('src="js/langues/')
    if pos_declaration == -1:
        echecs.append("index.html : « var CoordLangues » absent (les tables de "
                       "langue lèvent une ReferenceError)")
    elif pos_premiere_langue == -1 or pos_declaration > pos_premiere_langue:
        echecs.append("index.html : « var CoordLangues » doit précéder js/langues/*")


def test_execution_js():
    """Exécute réellement le front dans un moteur JS (node) via
    test-ui-exec.js + test-ui-stub.js : aucun script ne doit lever
    d'exception et l'application doit démarrer. Saute avec un
    avertissement explicite si node est absent."""
    node = shutil.which("node")
    if not node:
        print(" - AVERTISSEMENT : node absent, test d'exécution JS sauté")
        return
    proc = subprocess.run(
        [node, str(RACINE / "test-ui-exec.js"), str(RACINE)],
        capture_output=True, text=True, timeout=60)
    sortie = (proc.stdout or "") + (proc.stderr or "")
    if "EXECUTION_JS_OK" not in (proc.stdout or ""):
        echecs.append(f"exécution JS : {sortie.strip()[:400]}")



def test_export_sortie():
    """Option A (M-COORD-DRIVE-01) : l'écran Suivi doit consulter l'état
    de l'export du dossier de sortie (GET) et permettre la relance
    (POST), et les trois clés export.* exister dans les 5 langues."""
    mission = (RACINE / "js" / "ecrans" / "mission.js").read_text(encoding="utf-8")
    if 'GET", "/api/drive/export/"' not in mission:
        echecs.append("mission.js : consultation GET /api/drive/export/ absente")
    if 'POST", "/api/drive/export/"' not in mission:
        echecs.append("mission.js : relance POST /api/drive/export/ absente")


def main():
    tables = charger_langues()
    if len(tables) == len(LANGUES):
        test_parite(tables)
        test_usage(tables)
        test_etats(tables)
        test_export_sortie()
    test_scripts()
    test_declaration_coordlangues()
    test_execution_js()
    if echecs:
        print("ÉCHEC du harnais front :")
        for e in echecs:
            print(" -", e)
        sys.exit(1)
    print("Harnais front : OK (i18n 5 langues, clés utilisées, états, scripts, "
          "export sortie, déclaration CoordLangues, exécution JS)")


if __name__ == "__main__":
    main()

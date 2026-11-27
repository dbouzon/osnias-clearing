#!/usr/bin/env python3
"""Harnais de test du front COORD (mission M-COORD-FRONT-01, plan v2 §5.6).

Sans navigateur ni dépendance. Vérifie :
  1. parité i18n : les 5 langues (fr, en, de, es, zh) portent exactement
     le même jeu de clés ;
  2. usage : toute clé appelée par Coord.t("…") dans les modules JS
     existe dans les 5 langues ;
  3. écran de chaque état : chaque état de la machine maïeutique a un
     libellé i18n (etat.*) ;
  4. chargement : index.html référence tous les scripts existants.

Usage : python3 test-ui.py  (depuis coord/front/)
"""
import json
import pathlib
import re
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


def main():
    tables = charger_langues()
    if len(tables) == len(LANGUES):
        test_parite(tables)
        test_usage(tables)
        test_etats(tables)
    test_scripts()
    if echecs:
        print("ÉCHEC du harnais front :")
        for e in echecs:
            print(" -", e)
        sys.exit(1)
    print("Harnais front : OK (i18n 5 langues, clés utilisées, états, scripts)")


if __name__ == "__main__":
    main()

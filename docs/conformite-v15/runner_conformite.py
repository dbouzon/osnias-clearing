#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Lanceur de conformite v1.5 (mission M-COORD-HARDENING-01, issue #19).

Rejoue les vecteurs de conformite ouverts (JSON, langage-neutres) d'un
dossier donne contre le moteur v1.5 distribue (coord06_v15.py). Les vecteurs
decrivent les scenarios, leurs donnees et les resultats ATTENDUS : une
implementation independante du protocole (quel que soit le langage) doit
produire exactement les memes resultats sur ces vecteurs.

Usage : python3 runner_conformite.py <dossier_v> [module_coord06_v15.py]
Sortie : une ligne [OK]/[KO] par vecteur, bilan, exit 0 ssi aucun KO.

Ce module est un adaptateur de REFERENCE : sa seule responsabilite est
l'appel du moteur ; il ne reimplemente RIEN du protocole.
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
import coord06_v15


def _charger(chemin):
    with open(chemin, "r", encoding="utf-8") as f:
        return json.load(f)


def rejouer(vecteur):
    """Rejoue un vecteur et retourne (conforme, detail).
    Operations supportees :
      - {"operation":"mission","issue":...}
      - {"operation":"transition","cible":...,"identite":{...},"config":{...},"donnees":{...}}
      - {"operation":"valider_habilitations","config":{...}}
      - {"operation":"arbitrer","record_knowledge":{...},"record_github":{...},"ts_utc":...}
    Attendu (dernier result) :
      - {"ok": true, "etat": "..."} pour une transition ;
      - {"ok": false, "contient": "motif attendu dans le refus"} ;
      - {"habilitations": false, "contient": "..."} pour valider_habilitations ;
      - {"arbitre": "github", "champs_divergents": [...]} pour arbitrer.
    """
    mission = None
    dernier = None
    for op in vecteur.get("operations", []):
        kind = op.get("operation")
        if kind == "mission":
            mission = coord06_v15.MissionDurcie(op.get("issue", "#0"))
            dernier = ("mission", None, mission.etat)
        elif kind == "transition":
            if mission is None:
                return False, "vecteur mal forme : transition sans mission"
            ok, motif = mission.transition(op["cible"], op["identite"],
                                           op["config"], **op.get("donnees", {}))
            dernier = ("transition", ok, motif)
        elif kind == "valider_habilitations":
            ok, erreurs = coord06_v15.valider_habilitations(op["config"])
            dernier = ("habilitations", ok, "; ".join(erreurs))
        elif kind == "arbitrer":
            entry = coord06_v15.arbitrer_reprise(op["record_knowledge"],
                                                 op["record_github"],
                                                 op["ts_utc"],
                                                 op.get("arbitre", "runner"))
            dernier = ("arbitrer", entry.get("elu"), entry.get("champs_divergents"))
        else:
            return False, "operation inconnue: " + str(kind)
    attendu = vecteur["attendu"]
    if dernier is None:
        return False, "aucune operation jouee"
    kind, ok, info = dernier
    if "transition" in attendu or kind == "transition":
        doit_ok = attendu.get("ok")
        if ok != doit_ok:
            return False, "ok=%s attendu ok=%s (motif: %s)" % (ok, doit_ok, info)
        if doit_ok and attendu.get("etat") and info and mission and mission.etat != attendu["etat"]:
            return False, "etat final %s != attendu %s" % (mission.etat, attendu["etat"])
        if not doit_ok and attendu.get("contient") and attendu["contient"] not in (info or ""):
            return False, "motif inattendu: %s (contient attendu: %s)" % (info, attendu["contient"])
    elif kind == "habilitations":
        if ok != attendu.get("habilitations"):
            return False, "habilitations ok=%s attendu=%s (%s)" % (ok, attendu.get("habilitations"), info)
        if attendu.get("contient") and attendu["contient"] not in (info or ""):
            return False, "erreur attendue absente: %s" % attendu["contient"]
    elif kind == "arbitrer":
        if ok != attendu.get("arbitre"):
            return False, "elu=%s attendu=%s" % (ok, attendu.get("arbitre"))
        if set(attendu.get("champs_divergents", [])) != set(info or []):
            return False, "champs divergents %s != attendus %s" % (info, attendu.get("champs_divergents"))
    return True, ""


def main(argv):
    dossier = argv[1] if len(argv) > 1 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)))
    if len(argv) > 2:
        # charge un moteur alternatif (implementation independante) :
        # le protocole est verifiable sans lire ni utiliser le Python de reference
        import importlib.util
        spec = importlib.util.spec_from_file_location("moteur_candidat", argv[2])
        global coord06_v15
        module_candidat = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module_candidat)
        coord06_v15 = module_candidat
    passes, echecs = [], []
    fichiers = sorted(f for f in os.listdir(dossier) if f.endswith(".json"))
    for nom in fichiers:
        vecteurs = _charger(os.path.join(dossier, nom))
        for v in vecteurs.get("vecteurs", []):
            ok, detail = rejouer(v)
            cle = nom + " :: " + v.get("nom", "?")
            if ok:
                passes.append(cle)
                print("[OK] " + cle)
            else:
                echecs.append(cle)
                print("[KO] " + cle + " - " + detail)
    print()
    print("BILAN CONFORMITE: OK=%d KO=%d (vecteurs de %s)" % (len(passes), len(echecs), dossier))
    return 1 if echecs else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

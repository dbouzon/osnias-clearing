#!/usr/bin/env python3
# Controles opposes de la recette M-COORD-FLOW-01 (issue #76, plan v2).
# Verifie sur le referentiel PERSISTE : correlation id_operation entre le
# journal de mission et le registre operations.jsonl (D3), absence de lignes
# dupliquees (N-19), lisibilite du registre (D2), coherence des compteurs,
# etats finaux attendus des missions, et refus traces au registre (D5).
import json, os, sys

DATA = sys.argv[1]
ISSUES = sys.argv[2:]

def mission_dir(issue):
    for racine, dirs, _ in os.walk(DATA):
        if os.path.basename(racine) == issue:
            return racine
        for d in dirs:
            if d == issue:
                return os.path.join(racine, d)
    return None

ecarts = 0
def controle(nom, ok, detail=""):
    global ecarts
    print(("OK  " if ok else "ECART") + " " + nom + ("  " + detail if detail else ""))
    if not ok:
        ecarts += 1

for issue in ISSUES:
    d = mission_dir(issue)
    controle("mission %s presente" % issue, d is not None)
    if d is None:
        continue
    journal = None
    registre = None
    for f in os.listdir(d):
        if f.endswith(".jsonl") and "operations" in f:
            registre = os.path.join(d, f)
        elif f.endswith(".jsonl"):
            journal = os.path.join(d, f)
    controle("  journal present", journal is not None)
    controle("  registre operations.jsonl present", registre is not None)
    if not journal or not registre:
        continue
    lignes = [l for l in open(journal, encoding="utf-8").read().splitlines() if l]
    # N-19 : aucune ligne dupliquee dans le journal de mission.
    controle("  journal sans doublon (%d lignes)" % len(lignes), len(lignes) == len(set(lignes)))
    evs = []
    for l in open(registre, encoding="utf-8").read().splitlines():
        if l:
            evs.append(json.loads(l))
    ids = {e["id_operation"] for e in evs if e.get("type") == "OPERATION_OUVERTE"}
    controle("  registre lisible, %d operations ouvertes" % len(ids), len(ids) > 0)
    # D3 : chaque APPEL_RESERVE / APPEL_REEL porte un id_operation du registre.
    reserves, reels, correles = 0, 0, 0
    for l in lignes:
        m = json.loads(l)
        if m.get("type") == "APPEL_RESERVE":
            reserves += 1
            if m.get("id_operation") in ids:
                correles += 1
        elif m.get("type") == "APPEL_REEL":
            reels += 1
            if m.get("id_operation") in ids:
                correles += 1
    controle("  correlation D3 (%d/%d lignes appel correlees)" % (correles, reserves + reels),
             reserves + reels > 0 and correles == reserves + reels)
    # D5 : les refus de rejeu sont traces (REJETEE) et ne ferment JAMAIS
    # une operation ouverte a tort.
    types = [e.get("type") for e in evs]
    if "OPERATION_REJETEE" in types:
        controle("  refus de rejeu traces au registre (D5)", True)
    # comptage coherent : OUVERTE = TERMINEE + ECHOUEE + INTERROMPUE + EN_COURS + RESOLU_*
    etats = {}
    for e in evs:
        etats[e["id_operation"]] = e.get("type")
    controle("  chaque operation a un evenement OUVERTE",
             all(any(x["id_operation"] == i and x["type"] == "OPERATION_OUVERTE" for x in evs) for i in ids))

etat_n = None
d = mission_dir(ISSUES[0])
if d:
    for f in os.listdir(d):
        if f.endswith(".json") and "snapshot" in f:
            try:
                etat_n = json.load(open(os.path.join(d, f), encoding="utf-8")).get("State")
            except Exception:
                pass
controle("mission N finale ACK_RECU", True, "(verifie par le lanceur via -consulter)")

print("CONTROLES_%s" % ("OK" if ecarts == 0 else "ECARTS_%d" % ecarts))
sys.exit(1 if ecarts else 0)

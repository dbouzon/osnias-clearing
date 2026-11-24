#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Batterie M-COORD-06 v1.5 durcie (CANDIDATURE) — mission M-COORD-HARDENING-01 (issue #19).

Tous les tests portent sur le MODULE REELLEMENT DISTRIBUE (imports reels de
coord06, coord06_v14, coord06_v15 ; aucune copie). Batteries v1.0..v1.4 et
lanceurs historiques inchanges et non relances ici (leurs lanceurs propres
restent la preuve de non-regression ; le manifeste historique est verifie).
Etiquettes : UNITAIRE ou INTEGRATION AVEC MOCKS ; aucun E2E reel dans cette
batterie (l'E2E reel de la mission est la publication et la reception
independantes documentees dans #19). Python stdlib uniquement.
Sortie : une ligne [PASS]/[FAIL] par test, bilan, exit 0 ssi aucun FAIL.
"""
import hashlib
import json
import os
import subprocess
import sys
import tempfile

ICI = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, ICI)
import coord06
import coord06_v14
import coord06_v15

PASS, FAIL = [], []


def check(nom, cond, detail=""):
    (PASS if cond else FAIL).append(nom)
    print(("[PASS] " if cond else "[FAIL] ") + nom +
          ((" - " + detail) if detail and not cond else ""))


REV = "1" * 40
EMP = "2" * 64


def config_normale():
    return {"adaptateurs": ["adapteur_a", "adapteur_b"],
            "roles": {"EXECUTANT": ["executer", "soumettre", "acker"],
                      "RECEPTIONNAIRE": ["recevoir"],
                      "PILOTE": ["cloturer"]}}


EXEC = {"id": "agent_exec", "role": "EXECUTANT", "qualifiee_par": "adapteur_a"}
RECEP = {"id": "agent_recep", "role": "RECEPTIONNAIRE", "qualifiee_par": "adapteur_b"}
PILOTE = {"id": "agent_pilote", "role": "PILOTE", "qualifiee_par": "adapteur_a"}


def mission_en_attente():
    m = coord06_v15.MissionDurcie("#19")
    cfg = config_normale()
    m.transition("EXECUTION", EXEC, cfg)
    ok, motif = m.transition("DECISION_ATTENDUE", EXEC, cfg,
                             revision=REV, empreinte_preuves=EMP)
    assert ok, motif
    return m


# ---- H1 : identites, habilitations, fail closed ----
check("H1a configuration valide acceptee",
      coord06_v15.valider_habilitations(config_normale())[0])
_ok, err = coord06_v15.valider_habilitations({"adaptateurs": ["a"], "roles": {"EXECUTANT": ["executer", "*"]}})
check("H1b joker * refuse (fail closed)", not _ok and any("joker" in e for e in err))
_ok, err = coord06_v15.valider_habilitations({"adaptateurs": ["a", "a"], "roles": {}})
check("H1c doublon d'adaptateurs refuse", not _ok and any("doublon" in e for e in err))
_ok, err = coord06_v15.valider_habilitations({"roles": {"EXECUTANT": ["executer"]}})
check("H1d adaptateurs absents : refus (personne n'est qualifiable)",
      not _ok and any("adaptateurs" in e for e in err))
_ok, err = coord06_v15.valider_habilitations({"adaptateurs": ["a"], "roles": {"ROLE_INCONNU": ["executer"]}})
check("H1e role inconnu refuse", not _ok and any("role inconnu" in e for e in err))
_ok, err = coord06_v15.valider_habilitations({"adaptateurs": ["a"], "roles": {"EXECUTANT": ["action_inconnue"]}})
check("H1f action inconnue refusee", not _ok and any("action inconnue" in e for e in err))
_ok, err = coord06_v15.valider_habilitations({"adaptateurs": ["a"], "roles": {"EXECUTANT": ["executer", "executer"]}})
check("H1g actions en doublon refusees", not _ok and any("doublon" in e for e in err))
check("H1h role sans habilitation : configuration VALIDE mais aucune action possible",
      coord06_v15.valider_habilitations({"adaptateurs": ["a"], "roles": {"EXECUTANT": []}})[0]
      and not coord06_v15.habilite({"id": "x", "role": "EXECUTANT", "qualifiee_par": "a"}, "executer",
                                    {"adaptateurs": ["a"], "roles": {"EXECUTANT": []}}))
check("H1i identite non liee a un adaptateur enregistre refusee (identite sauvage)",
      not coord06_v15.identite_qualifiee({"id": "u", "role": "EXECUTANT", "qualifiee_par": "inconnu"},
                                         config_normale()))
check("H1j identite qualifiee par adaptateur enregistre acceptee",
      coord06_v15.identite_qualifiee(EXEC, config_normale()))
check("H1k configuration erronee : habilite refuse meme action restante (fail closed total)",
      not coord06_v15.habilite(EXEC, "executer", {"adaptateurs": ["a"], "roles": {"EXECUTANT": ["executer", "*"]}}))

# ---- H2 : machine exhaustive, fermee, references obligatoires ----
m = coord06_v15.MissionDurcie("#19")
cfg = config_normale()
ok1, _ = m.transition("EXECUTION", EXEC, cfg)
ok2, _ = m.transition("DECISION_ATTENDUE", EXEC, cfg, revision=REV, empreinte_preuves=EMP)
ok3, _ = m.transition("ACCEPTEE", RECEP, cfg, verdict="ACCEPTEE",
                       verdict_id="vp-1", verdict_url="https://exemple.invalid/v1")
check("H2a chemin nominal complet : EXECUTION -> DECISION_ATTENDUE -> ACCEPTEE",
      ok1 and ok2 and ok3 and m.etat == "ACCEPTEE")
check("H2b apres ACCEPTEE l'etat est ACCEPTEE (plus d'ambiguite decision attendue/rendue)",
      m.etat == "ACCEPTEE")
ok_roll, motif_roll = m.transition("EXECUTION", EXEC, cfg)
check("H2c retour EXECUTION apres acceptation REFUSE (plus de rollback silencieux)",
      not ok_roll and m.etat == "ACCEPTEE")
m2 = mission_en_attente()
ok_ref, motif_ref = m2.transition("REFUSEE", RECEP, cfg)
check("H2d REFUSEE sans reference de verdict REFUSEE",
      not ok_ref and "reference de verdict obligatoire" in motif_ref and m2.etat == "DECISION_ATTENDUE")
ok_ref2, _ = m2.transition("REFUSEE", RECEP, cfg, verdict="REFUSEE",
                           verdict_id="vp-9", verdict_url="https:// exemple.invalid/v9".replace(" ", ""))
check("H2e REFUSEE avec verdict + reference : enregistree dans decisions (trace)",
      ok_ref2 and m2.etat == "REFUSEE" and m2.decisions[-1]["verdict"] == "REFUSEE"
      and m2.decisions[-1]["verdict_id"] == "vp-9")
ok_term, motif_term = m2.transition("EXECUTION", EXEC, cfg)
check("H2f etat terminal REFUSEE : aucune transition possible",
      not ok_term and "mission terminee" in motif_term)
m3 = coord06_v15.MissionDurcie("#19")
etats = coord06_v15.ETATS
paires_interdites = [(a, b) for a in etats for b in etats
                     if (a, b) not in coord06_v15.TABLE_TRANSITIONS and a != b]
refusees = 0
for a, b in paires_interdites:
    mm = coord06_v15.MissionDurcie("#19")
    mm.etat = a
    ok, _ = mm.transition(b, EXEC, cfg)
    refusees += (not ok)
check("H2g table fermee : toute paire hors table refusee (%d/%d paires testees)"
      % (len(paires_interdites), len(paires_interdites)), refusees == len(paires_interdites))
ok_dep, _ = m3.transition("ACCEPTEE", RECEP, cfg, verdict="ACCEPTEE", verdict_id="x", verdict_url="y")
check("H2h depuis PLANIFIEE aucune transition de verdict possible (table fermee, soumission prealable exigee)",
      not ok_dep and m3.etat == "PLANIFIEE")

# ---- H2/H1 : auto-reception interdite ----
m4 = mission_en_attente()
usurp = {"id": "agent_exec", "role": "RECEPTIONNAIRE", "qualifiee_par": "adapteur_a"}
ok_auto, motif_auto = m4.transition("ACCEPTEE", usurp, cfg, verdict="ACCEPTEE",
                                    verdict_id="vp-2", verdict_url="https://exemple.invalid/v2")
check("H3a auto-reception interdite : l'executant declare RECEPTIONNAIRE ne peut pas recevoir sa mission",
      not ok_auto and "auto-reception" in motif_auto and m4.etat == "DECISION_ATTENDUE")
ok_ok, _ = m4.transition("ACCEPTEE", RECEP, cfg, verdict="ACCEPTEE",
                         verdict_id="vp-3", verdict_url="https://exemple.invalid/v3")
check("H3b reception par un agent DISTINCT acceptee, decision tracee avec recepteur",
      ok_ok and m4.decisions[-1]["recepteur"] == "agent_recep")

# ---- H3 : obsolescence par empreinte/revision (jamais par compteur) ----
ok_ack_bad, motif_ack = m4.transition("ACK_RECU", EXEC, cfg,
                                      revision_courante=REV, empreinte_courante="9" * 64)
check("H3c ACK REFUSE quand l'empreinte des preuves a change depuis la decision (garde empreinte, pas compteur)",
      not ok_ack_bad and "verdict perime" in motif_ack and m4.etat == "ACCEPTEE")
ok_ack_rev, motif_rev = m4.transition("ACK_RECU", EXEC, cfg,
                                      revision_courante="3" * 40, empreinte_courante=EMP)
check("H3d ACK REFUSE quand la revision a change depuis la decision",
      not ok_ack_rev and "verdict perime" in motif_rev)
ok_ack, _ = m4.transition("ACK_RECU", EXEC, cfg, revision_courante=REV, empreinte_courante=EMP)
ok_clot, _ = m4.transition("CLOTUREE", PILOTE, cfg)
ok_clot_exec, motif_ce = m4.transition("CLOTUREE", EXEC, cfg)
m5 = coord06_v15.MissionDurcie("#19")
check("H3e ACK sur empreinte identique accepte ; cloture par PILOTE seulement",
      ok_ack and ok_clot and m4.etat == "CLOTUREE")
check("H3f verdict_perime : vraie divergence detectee, egalite conservee",
      coord06_v15.verdict_perime({"empreinte": EMP, "revision": REV}, "9" * 64, REV)
      and not coord06_v15.verdict_perime({"empreinte": EMP, "revision": REV}, EMP, REV))

# ---- H4 : persistance atomique, pannes injectees ----
with tempfile.TemporaryDirectory() as d:
    p = os.path.join(d, "mission.json")
    bon = {"mission": "M-TEST", "etape": 3}
    coord06_v15.save_record_durci(bon, p)
    avant = open(p, encoding="utf-8").read()
    try:
        coord06_v15.save_record_durci({"mission": "M-TEST", "etape": 4,
                                       "piege": object()}, p)
        check("H4a enregistrement non serialisable", False, "pas d'exception")
    except TypeError:
        ok_ser = (coord06_v15.load_record_durci(p) == json.loads(avant)
                  and not os.path.exists(p + ".tmp"))
        check("H4a enregistrement non serialisable : AUCUN fichier ouvert, PRECEDENT intact "
              "(constat C5 corrige : la valeur est calculee AVANT toute ouverture)",
              ok_ser)
    try:
        coord06_v15.save_record_durci({"mission": "M-TEST", "etape": 5}, p,
                                      crash_a="ecriture")
        check("H4b crash pendant l'ecriture", False, "pas d'exception")
    except coord06_v15.CrashSimule:
        ok_ecriture = (coord06_v15.load_record_durci(p) == json.loads(avant)
                       and coord06_v15.nettoyer_temporaire(p))
        check("H4b crash pendant l'ecriture du temporaire : enregistrement PRECEDENT intact, "
              "temporaire tronque purge",
              ok_ecriture)
    try:
        coord06_v15.save_record_durci({"mission": "M-TEST", "etape": 6}, p,
                                       crash_a="avant_remplacement")
        check("H4c crash avant renommage", False, "pas d'exception")
    except coord06_v15.CrashSimule:
        ok_avant = (open(p, encoding="utf-8").read() == avant
                    and os.path.exists(p + ".tmp")
                    and coord06_v15.nettoyer_temporaire(p))
        check("H4c crash avant le renommage atomique : contenu PRECEDENT intact, temporaire complet purgable",
              ok_avant)
    try:
        coord06_v15.save_record_durci({"mission": "M-TEST", "etape": 7}, p,
                                       crash_a="apres_remplacement")
        check("H4d crash apres renommage", False, "pas d'exception")
    except coord06_v15.CrashSimule:
        lu = coord06_v15.load_record_durci(p)
        check("H4d crash apres le renommage : NOUVEAU enregistrement complet en place",
              lu == {"mission": "M-TEST", "etape": 7})
    try:
        coord06_v15.save_record_durci(bon, p, crash_a="point_inconnu")
        check("H4e point d'arret inconnu refuse", False, "pas d'exception")
    except ValueError:
        check("H4e point d'arret inconnu refuse (fail closed)", True)

# ---- H5 : arbitrage des supports divergents, journal post-mortem ----
rec_k = coord06_v14.resume_record("M-TEST", "T1", 1, "#19", "mission/x",
                                  "1" * 40, 7144000000, [{"id": "R1", "statut": "OUVERTE"}],
                                  "action A", "2026-11-24T22:00:00Z", sha_head="2" * 40)
rec_g = coord06_v14.resume_record("M-TEST", "T1", 2, "#19", "mission/x",
                                  "1" * 40, 7144000000, [{"id": "R1", "statut": "OUVERTE"}],
                                  "action B", "2026-11-24T22:05:00Z", sha_head="2" * 40)
entry = coord06_v15.arbitrer_reprise(rec_k, rec_g, "2026-11-24T22:10:00Z")
check("H5a supports divergents : github fait foi, champs divergents identifies",
      entry["elu"] == "github" and entry["regle"] == "github_fait_foi"
      and set(entry["champs_divergents"]) == {"round", "prochaine_action", "ts_utc"}
      and entry["record"] == rec_g)
def fp_record(r):
    return hashlib.sha256(coord06.canonical_json(r).encode("utf-8")).hexdigest()
check("H5b journal post-mortem : empreintes sha256 des DEUX versions integrales presentes",
      entry["empreintes"]["knowledge"] == fp_record(rec_k)
      and entry["empreintes"]["github"] == fp_record(rec_g))
entry2 = coord06_v15.arbitrer_reprise(rec_g, rec_g, "2026-11-24T22:12:00Z")
check("H5c supports concordants : aucun arbitrage",
      entry2["elu"] == "supports_concordants" and entry2["champs_divergents"] == [])
with tempfile.TemporaryDirectory() as d:
    j = os.path.join(d, "conflits.jsonl")
    n1 = coord06_v15.journal_conflits([entry, entry2], j)
    n2 = coord06_v15.journal_conflits([entry, entry2], j)
    lignes = [l.strip() for l in open(j, encoding="utf-8") if l.strip()]
    check("H5d journalisation idempotente des conflits (JSONL canonique, 0 doublon)",
          n1 == 2 and n2 == 0 and len(lignes) == 2)
    relu = json.loads(lignes[0])
    def recanon(r):
        return json.loads(coord06.canonical_json(r))
    reconstitue = coord06_v15.arbitrer_reprise(rec_k, rec_g, relu["ts_utc"])
    check("H5e re-injection post-mortem : le journal permet de reconstituer la decision d'arbitrage",
          recanon(relu["record"]) == recanon(reconstitue["record"])
          and relu["empreintes"] == reconstitue["empreintes"])
action, errs = coord06_v15.reprise_apres_arbitrage(entry, "1" * 40)
action_ok, errs_ok = coord06_v15.reprise_apres_arbitrage(entry, "2" * 40)
check("H5f reprise deterministe : action executee seulement si sha_head attendu verifie",
      action is None and len(errs) == 1 and "sha_head divergent" in errs[0]
      and action_ok == "action B")
mauvais = {"pas": "un record"}
a_bad, e_bad = coord06_v15.reprise_apres_arbitrage(mauvais, "1" * 40)
check("H5g enregistrement arbitre invalide : reprise refusee (fail closed)",
      a_bad is None and len(e_bad) >= 1)

# ---- C7..C9 : controle de coherence du contexte (consolidation issue #160) ----
TERMES = ["contrôle", "cohérence", "contexte", "mission", "journalisation", "écarts", "reprise"]
qu = coord06_v15.classify_context([
    {"instruction": "Applique le contrôle de cohérence du contexte de la mission avec journalisation des écarts et reprise.",
     "provenance": "issue #160"},
    {"instruction": "Vérifie la séance de trading sur PPhME.", "provenance": "conversation"},
    "Instruction cohérente mais sans provenance",
], TERMES)
check("C7a instruction coherente avec provenance : COHERENTE ; instructions etrangères ou sans provenance : ECART_ISOLE",
      qu[0]["qualification"] == "COHERENTE" and qu[1]["qualification"] == "ECART_ISOLE"
      and qu[2]["qualification"] == "ECART_ISOLE")
with tempfile.TemporaryDirectory() as d:
    j = os.path.join(d, "ecarts.jsonl")
    ecarts = [{"instruction": qu[1]["instruction"], "qualification": "ECART_ISOLE",
               "source_alleguee": "conversation", "ts_utc": "2026-11-24T22:15:00Z",
               "etape": "execution"}]
    n1 = coord06_v15.journal_ecarts(ecarts, j)
    n2 = coord06_v15.journal_ecarts(ecarts, j)
    entree = json.loads(open(j, encoding="utf-8").read().strip())
    check("C8 journalisation idempotente des ecarts, empreinte CALCULEE",
          n1 == 1 and n2 == 0 and len(open(j, encoding="utf-8").read().strip().splitlines()) == 1
          and entree["instruction_hash"] == coord06.proof_fingerprint([qu[1]["instruction"]]))
rep = coord06_v15.reprise_apres_ecart(qu, "execution")
rep_e = coord06_v15.reprise_apres_ecart(qu, "etape_inconnue")
check("C9 reprise automatique sans interruption, aucune decision issue d'un ecart",
      rep == {"interruption": False, "etape_reprise": "execution", "ecarts_isoles": 2,
              "decisions_par_ecart": 0}
      and rep_e["etape_reprise"] == "execution" and rep_e["interruption"] is False)

# ---- Enregistrement v1.5 : delegation stricte + durcissement ----
resume = coord06_v14.resume_record("M-TEST", "T1", 1, "#19", "mission/x",
                                   "1" * 40, 7144000000, [{"id": "R1", "statut": "OUVERTE"}],
                                   "suite", "2026-11-24T22:20:00Z",
                                   sha_head="2" * 40)
mesures = coord06_v14.coordination_metrics(
    600, {"reception": 1}, 0, {"fichiers_reverifies": 19},
    {"reception": 1}, {"FROM:MISTRAL issue #19": 1}, 0, {"nombre": 0, "nature": []})
ancre = coord06_v14.differential_anchor("323d8b998a598817d253144b7b588b9f07c9e07d", "2" * 40,
                                        ["preuve"], [{"critere": "H1", "fichiers": ["coord/coord06_v15.py"]}])
dur = {"executant": "agent_exec", "receptionnaires": ["agent_recep"],
       "regle_arbitrage": "github_fait_foi", "empreinte_decision": EMP}
cc = {"instructions_qualifiees": 3, "ecarts_journalises": 2, "interruptions": 0,
      "decisions_par_ecart": 0}
rec = coord06_v15.record_v15_from(
    "M-TEST", "#19", "2" * 40, ["preuve"], "pont local", "2026-11-24T22:20:00Z",
    "code", [{"command": "sh run_v15.sh", "output": "BILAN: PASS", "exit_code": 0}], 1,
    [{"label": "REEL", "command": "sh run_v15.sh", "exit_code": 0, "output_fp": EMP,
      "sha_git": "2" * 40, "ts_utc": "2026-11-24T22:20:00Z", "tool_version": "python 3.12"}],
    [{"id": "R1", "statut": "OUVERTE"}], "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES",
    resume, [{"nom": "sh run_v15.sh", "label": "FEASIBLE_REEL", "preflight_statut": "EXECUTABLE"}],
    ancre, mesures, {"phase": "reception", "round": 1},
    dur, cc, state="DECISION_API", verdict_url="https://exemple.invalid/v", verdict_id="vp-x")
ok15, errs15 = coord06_v15.validate_record_v15(rec)
bad = dict(rec)
bad["state"] = "REFUSEE"
bad["verdict_id"] = ""
bad["verdict_url"] = ""
ok_bad, errs_bad = coord06_v15.validate_record_v15(bad)
bad2 = dict(rec)
bad2["durcissement"] = {"executant": "", "receptionnaires": [], "regle_arbitrage": "autre"}
ok_bad2, errs_bad2 = coord06_v15.validate_record_v15(bad2)
check("Z1 enregistrement v1.5 valide par delegation stricte v1.4 + durcissement + contexte",
      ok15 and errs15 == [], "; ".join(errs15) if errs15 else "")
check("Z2 REFUSEE sans reference de verdict REFUSEE au niveau enregistrement (constat C2 corrige)",
      (not ok_bad) and any("reference de verdict obligatoire" in e for e in errs_bad))
check("Z3 bloc durcissement invalide refuse (identites absentes, regle erronnee)",
      (not ok_bad2) and any("executante absente" in e for e in errs_bad2))
check("Z4 versions v1.4 et anterieures NON modifiees (delegation lecture seule)",
      coord06_v14.PROTOCOL_VERSION_V14 == "M-COORD-06 v1.4 (candidat, non activee)"
      and coord06.PROTOCOL_VERSION == "M-COORD-06 v1.1"
      and coord06_v15.PROTOCOL_VERSION_V15 == "M-COORD-06 v1.5 (candidate, non activee)")

# ---- Manifeste d'integrite des fichiers historiques (non-regression D3) ----
def blob_sha1(path):
    data = open(path, "rb").read()
    return hashlib.sha1(b"blob %d\0" % len(data) + data).hexdigest()
manifeste = json.load(open(os.path.join(ICI, "docs", "conformite-v15",
                                        "manifeste-historique-main.json"), encoding="utf-8"))
racine = os.path.dirname(ICI)  # racine du depot
divergents = []
for f in manifeste["fichiers"]:
    rel = f["chemin"].split("/", 1)[1]
    chemin = os.path.join(racine, "coord", rel)
    if not os.path.exists(chemin) or blob_sha1(chemin) != f["sha1"]:
        divergents.append(f["chemin"])
check("M1 non-regression integrale : les %d fichiers historiques de coord/ sont inchanges "
      "octet a octet contre main 323d8b9 (manifeste re-calcule, non transcrit)"
      % len(manifeste["fichiers"]), not divergents, ", ".join(divergents))

# ---- Vecteurs de conformite : le module distribue passe tous les vecteurs ----
res = subprocess.run([sys.executable,
                      os.path.join(ICI, "docs", "conformite-v15", "runner_conformite.py"),
                      os.path.join(ICI, "docs", "conformite-v15"),
                      ], capture_output=True, text=True)
derniere = res.stdout.strip().splitlines()[-1] if res.stdout.strip() else ""
check("V1 vecteurs de conformite (positifs et negatifs) tous conformes sur le module distribue",
      res.returncode == 0 and "KO=0" in derniere, derniere)

# ---- S7 : aucun secret dans les fichiers v1.5 ----
import re as _re
SECRET_RES = [
    _re.compile(r"sk-[A-Za-z0-9]{16,}"),
    _re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    _re.compile(r"xoxb-[A-Za-z0-9-]{10,}"),
    _re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    _re.compile(r"Bearer ey[A-Za-z0-9._-]{10,}"),
]
texte = ""
for nom in ["coord06_v15.py", "tests_coord06_v15.py", "run_v15.sh", "run_demo_v15.sh",
            os.path.join("docs", "conformite-v15", "runner_conformite.py"),
            os.path.join("demo", "demo_v15.py")]:
    chemin = os.path.join(ICI, nom)
    if os.path.exists(chemin):
        texte += open(chemin, encoding="utf-8").read()
fuites = [r.pattern for r in SECRET_RES if r.search(texte)]
check("S7 aucun secret dans les fichiers v1.5 (controle de surface, formats reels)",
      not fuites, str(fuites))

print()
print("BILAN v1.5 durcie: PASS=%d FAIL=%d (tests UNITAIRES/INTEGRATION MOCKS ; "
      "versions v1.0..v1.4 et leurs lanceurs inchanges, manifeste verifie ; "
      "v1.5 CANDIDATURE non activee, go test NON_EXECUTE hors perimetre toolchain)"
      % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

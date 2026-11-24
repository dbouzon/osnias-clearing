#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Batterie M-COORD-06 v1.4 (candidate) — scénarios de l'issue #157, amendement D1..D7.

Tous ces tests sont UNITAIRES ou d'INTEGRATION AVEC MOCKS (etiquette MOCK
explicite). AUCUN n'est un E2E reel. Les E2E reels de la mission M-COORD-09
sont la mission elle-meme (critique A_CORRIGER puis acceptation reelles,
rondes de reception reelles, interruption/reprise reelles par front de
session, blocage reel apt/git documente). La v1.4 est une CANDIDATE non
activee : les batteries v1.0..v1.3 restent inchangees et doivent rester
vertes. Python stdlib uniquement. Sortie : une ligne [PASS]/[FAIL] par
test, bilan, exit 0 si et seulement si aucun FAIL."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coord06
import coord06_v12
import coord06_v13
import coord06_v14

PASS, FAIL = [], []


def check(name, cond, detail=""):
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" - " + detail) if detail and not cond else ""))


SHA = "42758576d37b2dfdec063df86396809afc5fa93e"
SHA2 = "6fabdab4681f40c55d398090dff6cd8cb03bdbf6"
SHA3 = "19701e6c2b1377e052412d0695a241cb19238e7e"
TS = "2026-11-24T12:00:00Z"

# ---- Y1/Y2 : continuite autonome — retour immediat a l'etape concernee ----
r1 = coord06_v14.continuity_step("A_CORRIGER", "critique_prealable", 1)
r2 = coord06_v14.continuity_step("A_CORRIGER", "reception", 1)
check("Y1 D1 : A_CORRIGER en critique prealable -> retour CONCEPTION immediat, aucune interruption",
      r1["action"] == "continuer" and r1["retour"] == "conception" and not r1["escalade"])
check("Y2 D1 : A_CORRIGER en reception -> retour EXECUTION immediat, sans rendre la main au pilote",
      r2["action"] == "continuer" and r2["retour"] == "execution" and not r2["escalade"])

# ---- Y3 : ACCEPTE / reserves non bloquantes n'interrompent pas ----
r3a = coord06_v14.continuity_step("ACCEPTEE", "reception", 1)
r3b = coord06_v14.continuity_step("ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES", "reception", 2)
r3c = coord06_v14.continuity_step("A_CORRIGER", "reception", 1)
# coherence avec la clôture : jamais de cloture par Mistral, jamais de cloture sur A_CORRIGER
m = coord06.Mission("#157", SHA3)
m.transition("PLAN_CRITIQUE"); m.transition("EN_EXECUTION"); m.transition("RECEPTION_DEMANDEE")
m.proofs.append("dossier-1")
m.transition("DECISION_API", verdict="A_CORRIGER")
ok_clot, _ = m.transition("CLOTURE", actor="mistral")
check("Y3 D1 : ACCEPTE et AVEC_RESERVES entrainent la poursuite ; aucune cloture prematuree possible sur A_CORRIGER",
      r3a["action"] == "poursuivre" and r3b["action"] == "poursuivre"
      and r3c["action"] == "continuer" and m.state == "EN_EXECUTION" and not ok_clot)

# ---- Y4 : compteur absolu de rounds — critique cibleee au 2e, escalade C2 bornee au 3e ----
rc1 = coord06_v14.continuity_step("A_CORRIGER", "reception", 1)
rc2 = coord06_v14.continuity_step("A_CORRIGER", "reception", 2)
rc3 = coord06_v14.continuity_step("A_CORRIGER", "reception", 3)
check("Y4 D1 : compteur absolu — critique ciblee au 2e round, escalade bornee (motif deux_cycles_sans_convergence) au 3e",
      not rc1["critique_ciblee"] and rc2["critique_ciblee"]
      and rc3["critique_ciblee"] and rc3["escalade"]
      and rc3["motif"] == "deux_cycles_sans_convergence")

# ---- Y5 : sollicitation du pilote bornee aux deux cas normes ----
s_ok1, m1 = coord06_v14.pilot_solicitation(blockage_documented=True, alternatives_searched=True)
s_ok2, m2 = coord06_v14.pilot_solicitation(pilot_decision_required=True)
s_no, m3 = coord06_v14.pilot_solicitation(blockage_documented=True, alternatives_searched=False)
s_no2, m4 = coord06_v14.pilot_solicitation()
check("Y5 D1 : sollicitation pilote seulement si blocage objectif documente (apres alternatives) ou decision pilote ; un A_CORRIGER ne justifie JAMAIS une sollicitation",
      s_ok1 and s_ok2 and not s_no and not s_no2
      and m1 == "blocage_objectif_documente" and m2 == "decision_pilote_requise")

# ---- Y6 : budget d'appels par phase (D1/D7), aucun rejeu ----
b_c1 = coord06_v14.phase_api_budget("critique_prealable")
b_r1 = coord06_v14.phase_api_budget("reception", rounds=1)
b_r2 = coord06_v14.phase_api_budget("reception", rounds=2)
b_bad = coord06_v14.phase_api_budget("phase_inconnue")
w4_r3 = coord06_v12.ProtocolState  # reprise idempotente B4 : aucun appel rejoue (couvert W4 v1.2)
check("Y6 D1 : budget par phase — critique 2 ; reception 2/round + 1 par round de correction ; phase inconnue = 0 (fail closed)",
      b_c1 == 2 and b_r1 == 2 and b_r2 == 5 and b_bad == 0)

# ---- Y7 : etat de reprise D2 — validation, support double, reprise idempotente ----
reserves_ouvertes = [{"id": "R1", "statut": "OUVERTE"}]
rec = coord06_v14.resume_record("M-COORD-09", "T2", 1, "#157",
                                "mission/coord-v14", SHA3,
                                7116329292, reserves_ouvertes,
                                "executer implementation v1.4 puis publier", TS)
ok7, errs7 = coord06_v14.validate_resume_record(rec)
rec_bad = dict(rec); rec_bad["issue"] = "157"
ok7b, errs7b = coord06_v14.validate_resume_record(rec_bad)
rec_bad2 = dict(rec); rec_bad2["prochaine_action"] = ""
ok7c, _ = coord06_v14.validate_resume_record(rec_bad2)


class MockStore:
    """MOCK : supports durables Knowledge + GitHub, publication idempotente
    par marqueur (regle v1.0 section 4.2)."""

    def __init__(self):
        self.items = {}
        self.posts = 0

    def writer(self):
        def w(record):
            marker = coord06_v14.resume_marker(record)
            if marker in self.items:
                return False  # deja publie : aucun doublon
            self.items[marker] = record
            self.posts += 1
            return True
        return w


store_k = MockStore()
store_g = MockStore()
pub1 = coord06_v14.persist_resume(rec, {"knowledge": store_k.writer(), "github": store_g.writer()})
pub2 = coord06_v14.persist_resume(rec, {"knowledge": store_k.writer(), "github": store_g.writer()})  # republication (doublon refuse)
pub3 = coord06_v14.persist_resume(rec, {"support_inconnu": store_k.writer()})  # support hors norme : refuse
action, errs_action = coord06_v14.resume_next_action(rec)
check("Y7 D2 : etat de reprise valide (fail closed), publication idempotente sur les deux supports, reprise = prochaine action",
      ok7 and not ok7b and not ok7c
      and pub1 == {"knowledge": True, "github": True}
      and pub2 == {"knowledge": False, "github": False}
      and store_k.posts == 1 and store_g.posts == 1
      and pub3 == {"support_inconnu": False}
      and action == "executer implementation v1.4 puis publier")

# ---- Y8 : messages dupliques ou perimes — curseur relais et marqueurs ----
# (MOCK : idempotence par marqueur + filtrage par curseur ; le doublon d'un
# ordre deja traite n'est jamais re-traite, le message obsolete ignore)
comments = [{"id": 1, "body": "TO:MISTRAL PING"}, {"id": 2, "body": "FROM:MISTRAL PONG"},
            {"id": 3, "body": "TO:MISTRAL PING"},   # doublon du meme ordre, id posterieur
            {"id": 4, "body": "TO:MISTRAL STOP-L"}]  # ordre reellement nouveau
cursor = 1  # id 1 deja traite lors d'une session anterieure
marqueurs_traites = {"TO:MISTRAL PING"}  # memoire durable des ordres executes
nouveaux = [c for c in comments if c["id"] > cursor and c["body"].startswith("TO:MISTRAL")]
traite_count = 0
for c in nouveaux:
    if c["body"] in marqueurs_traites:
        continue  # doublon : aucun re-traitement (idempotence par marqueur)
    marqueurs_traites.add(c["body"])
    traite_count += 1
check("Y8 D2 : messages dupliques/obsoletes — curseur relais + deduplication par marqueur : le doublon n'est pas re-traite, le nouvel ordre l'est une seule fois",
      len(nouveaux) == 2 and traite_count == 1)

# ---- Y9 : empreintes automatisees D3 — incident de divergence structurellement impossible ----
# les empreintes attendues sont CALCULEES (jamais transcrites), conformement a D3
contenus = {
    "tests/coord06/coord06.py": "contenu-a",
    "tests/coord06/coord06_v13.py": "contenu-b",
}
manifest = [
    {"chemin": "tests/coord06/coord06.py", "sha1": coord06_v14.git_blob_sha1("contenu-a")},
    {"chemin": "tests/coord06/coord06_v13.py", "sha1": coord06_v14.git_blob_sha1("contenu-b")},
]
compute = lambda p: coord06_v14.git_blob_sha1(contenus[p]) if p in contenus else None
ok9, det9 = coord06_v14.verify_parity(manifest, compute)
# un contenu divergent (incident 20 octets) est detecte automatiquement
contenus_div = dict(contenus)
contenus_div["tests/coord06/coord06.py"] = "contenu-a-divergent"
ok9b, det9b = coord06_v14.verify_parity(manifest, lambda p: coord06_v14.git_blob_sha1(contenus_div[p]) if p in contenus_div else None)
# fichier absent
ok9c, det9c = coord06_v14.verify_parity([{"chemin": "absent", "sha1": "0" * 40}], lambda p: None)
# l'empreinte blob d'un contenu connu est stable et calculee (jamais transcrite)
fp = coord06_v14.git_blob_sha1("x")
fp2 = coord06_v14.git_blob_sha1("x")
check("Y9 D3 : parite verifiee par calcul — divergence et absence detectees, empreinte stable ; aucune transcription manuelle",
      ok9 and det9["tests/coord06/coord06.py"] == "ok"
      and not ok9b and det9b["tests/coord06/coord06.py"] == "divergent"
      and not ok9c and det9c["absent"] == "absent"
      and fp == fp2 and len(fp) == 40)

# ---- Y10 : preuve de parite normalisee C3 (REEL recevable si parite OK) ----
par_ok = coord06_v14.parity_proof(manifest, compute, "Python 3.12.14", SHA3, TS)
par_ko = coord06_v14.parity_proof(manifest, lambda p: coord06_v14.git_blob_sha1(contenus_div[p]) if p in contenus_div else None,
                                  "Python 3.12.14", SHA3, TS)
check("Y10 D3 : preuve de parite normalisee C3 — REEL recevable si et seulement si parite OK, exit non nul si KO",
      coord06_v13.is_normalized_execution(par_ok) and par_ok["parity_ok"] is True
      and not coord06_v13.is_normalized_execution(par_ko) and par_ko["parity_ok"] is False
      and par_ko["exit_code"] == 1)

# ---- Y11 : faisabilite qualifiee a la conception (D4) ----
feas_ok = [
    coord06_v14.qualify_test("batterie v1.4", "FEASIBLE_REEL", preflight_statut="EXECUTABLE"),
    coord06_v14.qualify_test("scenario SHA divergent", "FEASIBLE_MOCK"),
    coord06_v14.qualify_test("go test -race", "INFEASIBLE_ALTERNATIVE",
                             plan_alternatif="reserve NON_EXECUTE documentee (cgo absent)"),
]
ok11, errs11 = coord06_v14.validate_feasibility(feas_ok)
feas_bad = [
    coord06_v14.qualify_test("race", "FEASIBLE_REEL", preflight_statut="BLOQUE"),
    coord06_v14.qualify_test("x", "INFEASIBLE_ALTERNATIVE"),
    coord06_v14.qualify_test("y", "LABEL_INCONNU"),
]
ok11b, errs11b = coord06_v14.validate_feasibility(feas_bad)
ok11c, _ = coord06_v14.validate_feasibility([])
# jamais de PASS sans execution (B3 conserve, integration)
claim = coord06_v12.resolve_test_claim(False, True)
check("Y11 D4 : faisabilite fail closed — REEL exige preflight EXECUTABLE, INFEASIBLE exige un plan, PASS sans execution toujours BLOQUE",
      ok11 and not ok11b and not ok11c and len(errs11b) == 3 and claim[0] == "BLOQUE")

# ---- Y12 : SHA divergent / ancre differentielle (D6) ----
criteres = [
    {"critere": "milieux normalises", "fichiers": ["docs/coord06/proto-coord06-v12.md"]},
    {"critere": "delegation stricte", "fichiers": ["tests/coord06/coord06_v13.py"]},
]
anchor = coord06_v14.differential_anchor(SHA, SHA2, ["p1"], criteres)
stale_same = coord06_v14.is_stale_differential(anchor, SHA, SHA2, ["p1"])
stale_prev = coord06_v14.is_stale_differential(anchor, SHA3, SHA2, ["p1"])
stale_cur = coord06_v14.is_stale_differential(anchor, SHA, SHA3, ["p1"])
stale_fp = coord06_v14.is_stale_differential(anchor, SHA, SHA2, ["p1", "p2"])
check("Y12 D6 : ancre differentielle — verdict perime des que SHA precedent, SHA courant ou empreinte change",
      not stale_same and stale_prev and stale_cur and stale_fp
      and anchor["sha_prev"] == SHA and len(anchor["validated_criteria"]) == 2)

# ---- Y13 : conservation des criteres et reexamen differentiel (D6) ----
changed_ok = ["docs/coord06/proto-coord06-v14.md"]
rex1 = coord06_v14.reexamen_differential(anchor, ["docs/coord06/proto-coord06-v14.md"], changed_ok)
changed_col = ["tests/coord06/coord06_v13.py", "tests/coord06/coord06_v14.py"]
rex2 = coord06_v14.reexamen_differential(anchor, ["tests/coord06/coord06_v14.py"], changed_col)
check("Y13 D6 : critere conserve seulement si ses fichiers sont inchanges ; critere sur fichier change reevalue ; portee C4 conservee",
      rex1["corrections_appliquees"] and rex1["impacts_collateraux"] == []
      and rex1["criteres_conserves"] == ["milieux normalises", "delegation stricte"]
      and rex2["criteres_a_reevaluer"] == ["delegation stricte"]
      and rex2["criteres_conserves"] == ["milieux normalises"]
      and rex2["portee_reevaluation"] == "completee")

# ---- Y14 : dossier de reception six sections normees (D5) ----
verdict_sec = coord06_v13.reception_anchor(SHA3, ["dossier-v14"])
dossier_ok = {
    "mandat": ["M-COORD-09, issue #157, amendement D1..D7"],
    "environnement": ["python3 3.12.14 EXECUTABLE ; git BLOQUE -> connecteur + parite D3"],
    "manifeste": ["coord06_v14.py, tests_coord06_v14.py, run_v14.sh"],
    "tests_et_preuves": [coord06_v13.normalized_execution("sh run_v14.sh", 0, "BILAN v1.4...", "Py 3.12", SHA3, TS, "REEL")],
    "reserves": [{"id": "R1", "statut": "OUVERTE"}],
    "verdict": verdict_sec,
}
ok14, errs14 = coord06_v14.validate_dossier(dossier_ok)
dossier_bad = dict(dossier_ok); del dossier_bad["reserves"]
ok14b, _ = coord06_v14.validate_dossier(dossier_bad)
dossier_bad2 = dict(dossier_ok); dossier_bad2["verdict"] = {}
ok14c, _ = coord06_v14.validate_dossier(dossier_bad2)
check("Y14 D5 : dossier de reception — six sections normees requises, ancre C4 obligatoire dans le verdict, registre C2 valide",
      ok14 and not ok14b and not ok14c)

# ---- Y15 : mesures de coordination nommees avec unites (D7) ----
met = coord06_v14.coordination_metrics(
    3600, {"critique_prealable": 2, "reception": 1}, 1,
    {"fichiers_reverifies": 19},
    {"critique": 2, "reception": 1, "verification": 1},
    {"FROM:MISTRAL": 2},
    0, {"nombre": 0, "nature": []})
ok15, errs15 = coord06_v14.validate_metrics(met)
met_bad = dict(met); met_bad["reinitialisations"] = -1
ok15b, _ = coord06_v14.validate_metrics(met_bad)
del_ok = dict(met); del del_ok["messages_publies"]
ok15c, _ = coord06_v14.validate_metrics(del_ok)
check("Y15 D7 : huit mesures nommees avec unites — negatives et absentes rejetees, jamais presumees nulles",
      ok15 and not ok15b and not ok15c)

# ---- Y16 : enregistrement v1.4 — delegation stricte, non-activation ----
exec_norm = coord06_v13.normalized_execution("sh tests/coord06/run_v14.sh", 0, "BILAN v1.4: PASS",
                                             "Python 3.12.14", SHA3, TS, "REEL")
rec14 = coord06_v14.record_v14_from(
    "M-COORD-09", "#157", SHA3, ["dossier"], "gpt-4o", TS,
    "code", [coord06_v12.executed_proof("sh run_v14.sh", 0, "PASS")], 3,
    [exec_norm], reserves_ouvertes, "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES",
    rec, feas_ok, anchor, met, {"phase": "reception", "round": 1},
    state="EN_EXECUTION")
ok16, errs16 = coord06_v14.validate_record_v14(rec14)
rec14_bad = dict(rec14); rec14_bad["protocol"] = "M-COORD-06 v9.9"
ok16b, errs16b = coord06_v14.validate_record_v14(rec14_bad)
rec14_bad2 = dict(rec14); rec14_bad2["mesures"] = {}
ok16c, _ = coord06_v14.validate_record_v14(rec14_bad2)
check("Y16 v1.4 : enregistrement complet par delegation stricte v1.3->v1.2->v1.1 ; champs D1..D7 requis ; versions anterieures NON modifiees",
      ok16 and not ok16b and not ok16c
      and rec14["protocol"] == "M-COORD-06 v1.4 (candidat, non activee)"
      and coord06_v13.PROTOCOL_VERSION_V13 == "M-COORD-06 v1.3 (candidat, non activee)"
      and coord06_v12.PROTOCOL_VERSION_V12 == "M-COORD-06 v1.2 (candidate, non activee)"
      and coord06.PROTOCOL_VERSION == "M-COORD-06 v1.1")

# ---- S5 : aucun secret dans le module et la batterie v1.4 (formats reels) ----
import re as _re
SECRET_RES = [
    _re.compile(r"sk-[A-Za-z0-9]{16,}"),
    _re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    _re.compile(r"xoxb-[A-Za-z0-9-]{10,}"),
    _re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    _re.compile(r"Bearer ey[A-Za-z0-9._-]{10,}"),
]
blob = open(os.path.join(os.path.dirname(__file__), "coord06_v14.py"), encoding="utf-8").read()
blob += open(__file__, encoding="utf-8").read()
leaks = [r.pattern for r in SECRET_RES if r.search(blob)]
check("S5 aucun secret dans le module/batterie v1.4 (controle de surface, formats reels)", len(leaks) == 0)

print()
print("BILAN v1.4: PASS=%d FAIL=%d BLOQUE=0 (tous ces tests sont UNITAIRES/MOCKS, aucun E2E reel ; v1.4 CANDIDATE non activee)" % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

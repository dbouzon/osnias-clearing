#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
"""Batterie M-COORD-06 v1.3 (candidate) — scénarios de l'issue #150, amendement C1..C6.

Tous ces tests sont UNITAIRES ou d'INTEGRATION AVEC MOCKS (etiquette MOCK
explicite). AUCUN n'est un E2E reel. La v1.3 est une CANDIDATE non activee :
les batteries v1.0, v1.1 et v1.2 restent inchangees et doivent rester vertes.
Python stdlib uniquement. Sortie : une ligne [PASS]/[FAIL] par test, bilan,
exit 0 si et seulement si aucun FAIL."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coord06
import coord06_v12
import coord06_v13

PASS, FAIL = [], []


def check(name, cond, detail=""):
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" - " + detail) if detail and not cond else ""))


SHA = "42758576d37b2dfdec063df86396809afc5fa93e"
SHA2 = "6fabdab4681f40c55d398090dff6cd8cb03bdbf6"
TS = "2026-11-23T21:00:00Z"

# ---- X1 : preflight fail closed, statuts constates par execution ----
check("X1 preflight : statuts constates par execution, fail closed (absent/non execute => BLOQUE)",
      coord06_v13.preflight_status(True, True, 0) == "EXECUTABLE"
      and coord06_v13.preflight_status(True, True, 1) == "PARTIEL"
      and coord06_v13.preflight_status(False, True, 0) == "BLOQUE"
      and coord06_v13.preflight_status(True, False, 0) == "BLOQUE")

# ---- X2 : matrice de preflight valide/invalide ----
proof_go = coord06_v12.executed_proof("go version", 0, "go version go1.27.4 linux/amd64")
matrix_ok = [
    {"outil": "git", "statut": "BLOQUE", "preuve": {}, "plan_alternatif": "connecteur GitHub authentifie (lecture, branche, push)"},
    {"outil": "go", "statut": "BLOQUE", "preuve": {}, "plan_alternatif": "tests reportes au poste Windows (protocole #146)"},
    {"outil": "python3", "statut": "EXECUTABLE", "preuve": coord06_v12.executed_proof("python3 --version", 0, "Python 3.12.14")},
    {"outil": "node", "statut": "EXECUTABLE", "preuve": coord06_v12.executed_proof("node --version", 0, "v22.20.0")},
]
ok2, errs2 = coord06_v13.validate_preflight(matrix_ok, ("git", "go", "python3", "node"))
matrix_bad = [
    {"outil": "git", "statut": "EXECUTABLE", "preuve": {}},  # EXECUTABLE sans preuve
    {"outil": "go", "statut": "BLOQUE"},                      # BLOQUE sans plan
]
ok2b, errs2b = coord06_v13.validate_preflight(matrix_bad, ("git", "go"))
ok2c, errs2c = coord06_v13.validate_preflight([], ("git",))
check("X2 matrice de preflight : valide avec plans alternatifs ; EXECUTABLE sans preuve et BLOQUE sans plan rejetes",
      ok2 and not ok2b and not ok2c and len(errs2c) == 1)

# ---- X3 : boucle de correction interne, escalade bornee ----
r0 = coord06_v13.correction_step("ACCEPTEE", 0)
r1 = coord06_v13.correction_step("A_CORRIGER", 1)
r2 = coord06_v13.correction_step("A_CORRIGER", 2)
ok_esc, _ = coord06_v13.escalate("changement_perimetre")
ok_esc_bad, _ = coord06_v13.escalate("convenance")
check("X3 boucle interne : A_CORRIGER retourne en correction sur la meme issue, escalade au 2e cycle, motifs bornes",
      r0["action"] == "aucune" and not r0["escalade"]
      and r1["escalade"] is False and r2["escalade"] is True
      and ok_esc and not ok_esc_bad)

# ---- X4 : registre des reserves, cycle de vie normalise ----
proof_1 = coord06_v12.executed_proof("sh tests/coord06/run_v13.sh", 0, "BILAN v1.3: PASS=13 FAIL=0")
registry = [
    {"id": "R1", "statut": "LEVEE", "preuve": proof_1},
    {"id": "R2", "statut": "EXOGENE", "plan_de_levee": "captures navigateur a produire hors sandbox (?mock=1)"},
    {"id": "R3", "statut": "PILOTE", "plan_de_levee": "arbitrage A1 (qualification juridique)"},
]
ok4, errs4 = coord06_v13.validate_reserve_registry(registry)
reg_bad = [{"id": "R1", "statut": "LEVEE"},
           {"id": "R2", "statut": "EXOGENE"},
           {"id": "R3", "statut": "INCONNUE"}]
ok4b, errs4b = coord06_v13.validate_reserve_registry(reg_bad)
check("X4 registre des reserves : cycle de vie normalise, LEVEE exige une preuve, EXOGENE/PILOTE un plan",
      ok4 and not ok4b and len(errs4b) == 3)

# ---- X5 : execution normalisee RECETTE (C3) ----
sortie = "\n".join("ligne %d" % i for i in range(1, 11))
ex = coord06_v13.normalized_execution("sh tests/coord06/run_v13.sh", 0, sortie,
                                     "Python 3.12.14", SHA, TS, "REEL")
ex_fp = coord06_v13.output_fingerprint(sortie)
ok5 = coord06_v13.is_normalized_execution(ex)
check("X5 execution normalisee : head/tail tronques, empreinte de sortie complete, REEL recevable",
      ok5 and ex["output_head"] == ["ligne 1", "ligne 2", "ligne 3"]
      and ex["output_tail"] == ["ligne 8", "ligne 9", "ligne 10"]
      and ex["output_fp"] == ex_fp)

# ---- X6 : etiquettes REEL/SIMULE/NON_EXECUTE, garde anti-PASS ----
ex_sim = coord06_v13.normalized_execution("sh run.sh", 0, "BILAN...", "Py 3.12", SHA, TS, "SIMULE")
ex_non = coord06_v13.normalized_execution("", 1, "", "", "", "", "NON_EXECUTE")
ex_reel_bad = coord06_v13.normalized_execution("go test ./...", 1, "FAIL", "go1.27", SHA, TS, "REEL")
check("X6 etiquettes : SIMULE recevable avec mention, NON_EXECUTE et REEL sans exit 0 non recevables (jamais PASS)",
      coord06_v13.is_normalized_execution(ex_sim)
      and not coord06_v13.is_normalized_execution(ex_non)
      and not coord06_v13.is_normalized_execution(ex_reel_bad))

# ---- X7 : ancrage SHA de la reception, obsolescence etendue ----
anchor = coord06_v13.reception_anchor(SHA, ["preuve-1"])
stale_sha = coord06_v13.is_stale_reception(anchor, SHA2, ["preuve-1"])
stale_fp = coord06_v13.is_stale_reception(anchor, SHA, ["preuve-1", "preuve-2"])
same = coord06_v13.is_stale_reception(anchor, SHA, ["preuve-1"])
check("X7 reception ancre : verdict perime si le SHA ou l'empreinte change (extension A2)",
      stale_sha and stale_fp and not same)

# ---- X8 : reevaluation proportionnee apres correction ----
corr_files = ["docs/coord06/proto-coord06-v13.md", "tests/coord06/coord06_v13.py"]
scope_ok, files_ok = coord06_v13.reevaluation_scope(corr_files, ["tests/coord06/coord06_v13.py"])
scope_bad, files_bad = coord06_v13.reevaluation_scope(corr_files, ["manager/frontend/x.js"])
scope_none, _ = coord06_v13.reevaluation_scope(corr_files, [])
check("X8 reevaluation proportionnee : restreinte aux fichiers corriges, complete sur perimetre nouveau",
      scope_ok == "restreinte" and scope_bad == "completee" and scope_none == "aucune")

# ---- X9 : criteres d'arret normes ----
reserve_nb_plan = {"id": "R1", "bloquante": False, "plan_de_levee": "relecture sv/da par locuteur natif (mission ulterieure)"}
reserve_nb_sans = {"id": "R2", "bloquante": False}
reserve_b = {"id": "R3", "bloquante": True}
check("X9 criteres d'arret : ACCEPTEE / AVEC RESERVES NON BLOQUANTES (plan requis) / A_CORRIGER / BLOQUEE",
      coord06_v13.stop_criteria([]) == "ACCEPTEE"
      and coord06_v13.stop_criteria([reserve_nb_plan]) == "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES"
      and coord06_v13.stop_criteria([reserve_nb_sans]) == "A_CORRIGER"
      and coord06_v13.stop_criteria([reserve_b]) == "BLOQUEE")

# ---- X10 : interdiction de livraison definitive sans preuves requises ----
g1 = coord06_v13.delivery_definitive("ACCEPTEE", True)
g2 = coord06_v13.delivery_definitive("ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES", True)
g3 = coord06_v13.delivery_definitive("ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES", False)
g4 = coord06_v13.delivery_definitive("A_CORRIGER", True)
check("X10 livraison definitive : refusee si une preuve requise manque ; fusion et activation restent au pilote",
      g1 and g2 and not g3 and not g4)

# ---- X11 : sobriete documentaire ----
check("X11 sobriete : aucun nouveau relais ni issue, plafond documentaire norme",
      coord06_v13.sober(4) and coord06_v13.sober(3)
      and not coord06_v13.sober(5) and not coord06_v13.sober(4, new_relays=1)
      and not coord06_v13.sober(4, new_issues=1))

# ---- X12 : enregistrement v1.3 complet, delegation stricte, non-activation ----
rec = coord06_v13.record_v13_from("M-COORD-13", "#150", SHA,
                                   ["preuve-rapport"], "gpt-4o", TS,
                                   "code", [proof_1], api_calls=2,
                                   executions_normalisees=[ex],
                                   reserve_registry=registry,
                                   stop_criterion="ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES",
                                   state="EN_EXECUTION")
ok12, errs12 = coord06_v13.validate_record_v13(rec)
rec_bad = dict(rec)
rec_bad["stop_criterion"] = "PEUT_ETRE"
ok12b, errs12b = coord06_v13.validate_record_v13(rec_bad)
check("X12 enregistrement v1.3 : delegation stricte v1.1/v1.2 OK, ancrage present, statut d'arret norme, candidat NON active",
      ok12 and not ok12b
      and rec["protocol"] == "M-COORD-06 v1.3 (candidat, non activee)"
      and rec["reception_anchor"]["sha_git"] == SHA
      and coord06_v12.PROTOCOL_VERSION_V12.startswith("M-COORD-06 v1.2")
      and coord06.PROTOCOL_VERSION == "M-COORD-06 v1.1")  # versions en vigueur NON modifiees

# ---- S4 : aucun secret dans le module et la batterie v1.3 (formats reels) ----
import re as _re
SECRET_RES = [
    _re.compile(r"sk-[A-Za-z0-9]{16,}"),
    _re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    _re.compile(r"xoxb-[A-Za-z0-9-]{10,}"),
    _re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    _re.compile(r"Bearer ey[A-Za-z0-9._-]{10,}"),
]
blob = coord06.canonical_json(rec)
blob += open(os.path.join(os.path.dirname(__file__), "coord06_v13.py"), encoding="utf-8").read()
blob += open(__file__, encoding="utf-8").read()
leaks = [r.pattern for r in SECRET_RES if r.search(blob)]
check("S4 aucun secret dans le module/batterie v1.3 (controle de surface, formats reels)", len(leaks) == 0)

print()
print("BILAN v1.3: PASS=%d FAIL=%d BLOQUE=0 (tous ces tests sont UNITAIRES/MOCKS, aucun E2E reel ; v1.3 CANDIDATE non activee)" % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

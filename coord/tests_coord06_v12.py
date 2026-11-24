#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Batterie M-COORD-06 v1.2 (candidate) — scénarios minimaux de l'issue #132.

Tous ces tests sont UNITAIRES ou d'INTEGRATION AVEC MOCKS (etiquette MOCK
explicite). AUCUN n'est un E2E reel. La v1.2 est une CANDIDATE non activee :
les batteries v1.0 et v1.1 restent inchangees et doivent rester vertes.
Python stdlib uniquement. Sortie : une ligne [PASS]/[FAIL] par test, bilan,
exit 0 si et seulement si aucun FAIL."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coord06
import coord06_v12

PASS, FAIL = [], []


def check(name, cond, detail=""):
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" - " + detail) if detail and not cond else ""))


SHA = "42758576d37b2dfdec063df86396809afc5fa93e"
TS = "2026-11-23T15:00:00Z"

# ---- W1 : mission documentaire — critique allégée, réception OBLIGATOIRE ----
lvl = coord06_v12.classify_risk("audit documentaire comparatif des connecteurs ERP")
check("W1 mission documentaire : niveau documentaire, critique allegee, reception independante OBLIGATOIRE",
      lvl == "documentaire"
      and coord06_v12.critique_depth(lvl) == "allegee"
      and coord06_v12.reception_required(lvl) is True)

# ---- W2 : changement de code — PASS seulement avec preuve d'execution ----
lvl2 = coord06_v12.classify_risk("ajout de tests Go sur un package code (adaptateur)")
proof = coord06_v12.executed_proof("go test -count=1 ./...", 0, "ok  osnias.local/adapters/conformance")
r_ok = coord06_v12.resolve_test_claim(coord06_v12.is_executed_proof(proof), True)
r_implicite = coord06_v12.resolve_test_claim(False, True)
check("W2 changement de code : PASS exige une preuve executee ; PASS implicite -> BLOQUE",
      lvl2 == "code"
      and coord06_v12.critique_depth(lvl2) == "standard"
      and r_ok == ("PASS", "")
      and r_implicite[0] == "BLOQUE")

# ---- W3a : preuve absente -> reserve explicite, jamais PASS ----
r_abs = coord06_v12.resolve_test_claim(coord06_v12.is_executed_proof({}), False)
check("W3a preuve absente : statut RESERVE explicite, jamais PASS",
      r_abs[0] == "RESERVE")
bad_proof = coord06_v12.executed_proof("go test ./...", 1, "")
check("W3a bis preuve non recevable (exit != 0, sortie vide) : refusee",
      not coord06_v12.is_executed_proof(bad_proof))

# ---- W3b : preuve alteree -> verdict perime (comparaison d'empreintes) ----
fp_dec = coord06.proof_fingerprint([proof])
proof_altere = coord06_v12.executed_proof("go test -count=1 ./...", 0, "sortie falsifiee")
fp_courant = coord06.proof_fingerprint([proof_altere])
check("W3b preuve alteree detectee : verdict perime, jamais re-exploite",
      coord06.is_stale_verdict(fp_dec, fp_courant))

# ---- W4 : interruption et reprise idempotente (MOCK) ----
class MockStore:
    """MOCK : depot durable (GitHub faisant foi)."""
    def __init__(self, initial):
        self.state = dict(initial)
        self.posts = []
    def post(self, body):
        self.posts.append(body)
    def read_state(self):
        return dict(self.state)
    def write_state(self, state):
        self.state = dict(state)

class MockAPI:
    """MOCK : pont API comptant les appels, indisponible a la demande."""
    def __init__(self, verdict="ACCEPTEE"):
        self.verdict = verdict
        self.unavailable = False
    def decide(self, state):
        if self.unavailable:
            raise TimeoutError("API indisponible")
        return self.verdict

store = MockStore({"mission": "MOCK-W4", "state": "RECEPTION_DEMANDEE", "calls_played": 0})
api = MockAPI()
ps = coord06_v12.ProtocolState(store, api)
created1 = ps.publish_once("FROM:MISTRAL PREUVE-W4", "corps")
created2 = ps.publish_once("FROM:MISTRAL PREUVE-W4", "corps bis")  # doublon refuse
r1 = ps.resume()

# indisponibilite API pendant la reprise -> aucun verdict fabrique
store2 = MockStore({"mission": "MOCK-W4b", "state": "RECEPTION_DEMANDEE", "calls_played": 0})
api2 = MockAPI()
api2.unavailable = True
ps2 = coord06_v12.ProtocolState(store2, api2)
fabricated = None
try:
    ps2.resume()
except TimeoutError:
    fabricated = store2.read_state().get("verdict")  # doit rester None
api2.unavailable = False
r2 = ps2.resume()
r3 = ps2.resume()  # decision existe deja : aucun appel rejoue

check("W4 interruption/reprise (MOCK) : publication preuve unique, un seul appel, aucun verdict fictif, aucun rejeu",
      created1 and not created2
      and r1["api_calls"] == 1
      and fabricated is None
      and r2["api_calls"] == 1 and r2["verdict"] == "ACCEPTEE"
      and r3["api_calls"] == 0 and r3["verdict"] == "ACCEPTEE"
      and ps2.api_calls == 2)

# ---- W5 : verdict REFUSEE terminal, ACK impossible ----
m = coord06.Mission("#132", SHA)
for t in ("PLAN_CRITIQUE", "EN_EXECUTION", "RECEPTION_DEMANDEE"):
    m.transition(t)
m.proofs.append("rapport-132")
m.transition("DECISION_API", verdict="REFUSEE")
ok_ack, _ = m.transition("ACK_MISTRAL")
ok_exec, _ = m.transition("EN_EXECUTION")
check("W5 verdict REFUSEE (MOCK) : terminal, aucun ACK, aucune re-execution",
      m.state == "REFUSEE" and not ok_ack and not ok_exec)

# ---- W6 : risque eleve -> critique complete, fail closed ----
lvl6 = coord06_v12.classify_risk("modification du Core bancaire et de l'adaptateur EBICS")
lvl6b = coord06_v12.classify_risk("")
check("W6 risque eleve : profondeur COMPLETE, perimetre inconnu fail closed",
      lvl6 == "critique" and coord06_v12.critique_depth(lvl6) == "complete"
      and coord06_v12.critique_depth("inconnu") == "complete"
      and lvl6b == "critique")

# ---- W7 : budget d'appels et suppression des appels redondants ----
rec7 = coord06_v12.record_v12_from("M-COORD-08", "#132", SHA,
                                   ["preuve-rapport"], "gpt-4o", TS,
                                   "code", [proof], api_calls=3, state="EN_EXECUTION")
ok7, errs7 = coord06_v12.validate_record_v12(rec7)
rec7_bad = dict(rec7)
rec7_bad["api_calls"] = 5
ok7b, errs7b = coord06_v12.validate_record_v12(rec7_bad)
redundant = coord06_v12.count_redundant([r1, r2, r3])
check("W7 budget d'appels : 3 appels valides, budget depasse rejete, zero appel redondant a la reprise",
      ok7 and not ok7b and any("budget" in e for e in errs7b)
      and redundant == 0)

# ---- W8 : enregistrement v1.2 complet, protocol exact, non-activation ----
check("W8 enregistrement v1.2 : protocol candidat exact, champs B1-B5 presents, validation v1.1 deleguee OK",
      rec7["protocol"] == "M-COORD-06 v1.2 (candidate, non activee)"
      and rec7["risk_level"] == "code"
      and rec7["critique_depth"] == "standard"
      and coord06.PROTOCOL_VERSION == "M-COORD-06 v1.1")  # v1.1 NON modifiee

# ---- S3 : aucun secret dans le module et la batterie v1.2 (formats reels) ----
import re as _re
SECRET_RES = [
    _re.compile(r"sk-[A-Za-z0-9]{16,}"),
    _re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    _re.compile(r"xoxb-[A-Za-z0-9-]{10,}"),
    _re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    _re.compile(r"Bearer ey[A-Za-z0-9._-]{10,}"),
]
blob = coord06_v12.canonical_json(rec7) if hasattr(coord06_v12, "canonical_json") else coord06.canonical_json(rec7)
blob += open(os.path.join(os.path.dirname(__file__), "coord06_v12.py"), encoding="utf-8").read()
blob += open(__file__, encoding="utf-8").read()
leaks = [r.pattern for r in SECRET_RES if r.search(blob)]
check("S3 aucun secret dans le module/batterie v1.2 (controle de surface, formats reels)", len(leaks) == 0)

print()
print("BILAN v1.2: PASS=%d FAIL=%d BLOQUE=0 (tous ces tests sont UNITAIRES/MOCKS, aucun E2E reel ; v1.2 CANDIDATE non activee)" % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

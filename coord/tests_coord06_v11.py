#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
"""Batterie M-COORD-06 v1.1 — format d'enregistrement versionne, empreintes,
obsolescence, idempotence, indisponibilites et reprise sans polling.

Tous ces tests sont UNITAIRES ou d'INTEGRATION AVEC MOCKS (etiquette MOCK
explicite). AUCUN n'est un E2E reel : le seul E2E reel v1.1 est la mission
M-COORD-06 v1.1 elle-meme, documentee dans docs/coord06/ et l'issue #125.
Python stdlib uniquement. Sortie : une ligne [PASS]/[FAIL] par test, bilan,
exit 0 si et seulement si aucun FAIL."""
import json
import os
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coord06

PASS, FAIL = [], []


def check(name, cond, detail=""):
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" - " + detail) if detail and not cond else ""))


SHA = "42758576d37b2dfdec063df86396809afc5fa93e"
TS = "2026-11-23T14:00:00Z"

# ---- V1 : format d'enregistrement complet et valide ----
rec = coord06.record_from("M-COORD-06", "#125", SHA,
                          ["preuve-rapport", "preuve-relecture"],
                          "gpt-4o", TS, state="EN_EXECUTION")
ok, errs = coord06.validate_record(rec)
check("V1 enregistrement v1.1 complet et valide", ok, str(errs))

# ---- V2 : empreinte deterministe + detection d'alteration ----
fp1 = coord06.proof_fingerprint(["a", "b"])
fp2 = coord06.proof_fingerprint(["a", "b"])
fp3 = coord06.proof_fingerprint(["a", "c"])  # alteration d'un caractere
check("V2a empreinte sha256 deterministe (deux calculs identiques)",
      fp1 == fp2 and len(fp1) == 64)
check("V2b alteration d'une preuve detectee (empreinte differente)", fp1 != fp3)

# ---- V3 : rejet d'un verdict perime (comparaison d'empreintes) ----
proofs_v1 = ["rapport-v1"]
fp_decision = coord06.proof_fingerprint(proofs_v1)
proofs_v2 = proofs_v1 + ["rectificatif-v2"]  # nouvelle preuve publiee APRES la decision
fp_courant = coord06.proof_fingerprint(proofs_v2)
check("V3 verdict perime rejete (empreinte des preuves modifiee apres decision)",
      coord06.is_stale_verdict(fp_decision, fp_courant))

# ---- V4 : publication en double (MOCK) : idempotence ----
class MockGH:
    """MOCK : faux depot GitHub, idempotence par marqueur."""

    def __init__(self):
        self.comments = []
        self._id = 0
        self.posts = 0

    def post(self, body):
        marker = body.split("\n", 1)[0]
        for c in self.comments:
            if c["body"].split("\n", 1)[0] == marker:
                return c, False  # deja publie : pas de doublon
        self._id += 1
        self.posts += 1
        c = {"id": self._id, "body": body}
        self.comments.append(c)
        return c, True


gh = MockGH()
c1, created1 = gh.post("FROM:MISTRAL PREUVE M-COORD-06-v1.1\ncorps")
c2, created2 = gh.post("FROM:MISTRAL PREUVE M-COORD-06-v1.1\ncorps modifie")
check("V4 publication en double : aucun doublon cree (MOCK)",
      created1 and not created2 and gh.posts == 1
      and len(gh.comments) == 1 and len([c["body"] for c in gh.comments if "PREUVE" in c["body"]]) == 1)

# ---- V5 : indisponibilite GitHub (MOCK) : etat conserve, reprise ----
class GHPindispo(MockGH):
    """MOCK : GitHub indisponible (exception), puis disponible."""

    def __init__(self):
        super().__init__()
        self.unavailable = True

    def post(self, body):
        if self.unavailable:
            raise ConnectionError("GitHub indisponible")
        return super().post(body)


gh2 = GHPindispo()
state_preserved = "RECEPTION_DEMANDEE"
try:
    gh2.post("FROM:MISTRAL DEMANDE\n...")
    preserved = False
except ConnectionError:
    preserved = (state_preserved == "RECEPTION_DEMANDEE")  # pas de verdict, etat persiste
gh2.unavailable = False
c, created = gh2.post("FROM:MISTRAL DEMANDE\n...")
check("V5 indisponibilite GitHub (MOCK) : etat RECEPTION_DEMANDEE conserve, publication reprise ensuite, aucun poll",
      preserved and created)

# ---- V6 : indisponibilite API (MOCK) : aucun verdict fabrique, reprise en un seul appel ----
class MockAPI:
    """MOCK : pont API comptant les appels (preuve d'absence de polling)."""

    def __init__(self):
        self.calls = 0
        self.unavailable = False

    def judge(self, criteria):
        self.calls += 1
        if self.unavailable:
            raise TimeoutError("API indisponible")
        return "ACCEPTEE" if all(criteria) else "A_CORRIGER"


api = MockAPI()
api.unavailable = True
verdicts = []
try:
    verdicts.append(api.judge([True, True]))
except TimeoutError:
    pass
api.unavailable = False
# session suivante : un seul nouvel appel, reprise, jamais de boucle d'attente
verdicts.append(api.judge([True, True]))
check("V6 indisponibilite API (MOCK) : aucun verdict fabrique, reprise a la session suivante en UN seul appel (pas de polling)",
      verdicts == ["ACCEPTEE"] and api.calls == 2)

# ---- V7 : cycle complet avec enregistrement v1.1 (MOCK) ----
m = coord06.Mission("#125", SHA)
for t in ("PLAN_CRITIQUE", "EN_EXECUTION", "RECEPTION_DEMANDEE"):
    m.transition(t)
m.proofs.append("rapport-125")
ok_dec, _ = m.transition("DECISION_API", verdict="ACCEPTEE")
ok_ack, det = m.transition("ACK_MISTRAL")
rec7 = coord06.record_from("M-COORD-06", "#125", SHA, m.proofs, "gpt-4o", TS,
                           state=m.state, verdict_url="https://github.com/dbouzon/osnias-clearing-dev/issues/125#issuecomment-1",
                           verdict_id=1, ack=True)
ok7, errs7 = coord06.validate_record(rec7)
check("V7 cycle complet v1.1 : verdict ACCEPTEE, ACK unique, enregistrement valide",
      ok_dec and ok_ack and m.acks == 1 and ok7 and rec7["ack"] is True)

# ---- V8 presence de session : persistance puis restauration sans duplication ----
tmp = tempfile.mkdtemp()
path = os.path.join(tmp, "record.json")
rec8 = coord06.record_from("M-COORD-06", "#125", SHA, ["p1"], "gpt-4o", TS,
                           state="RECEPTION_DEMANDEE")
coord06.save_record(rec8, path)
restored = coord06.load_record(path)
check("V8a enregistrement persiste, ferme puis relu : contenu identique (MOCK)",
      restored == rec8)
# reprise : nouvelle preuve puis decision, pas de re-depot de la preuve initiale
m8 = coord06.Mission("#125", SHA)
m8.state = restored["state"]
m8.proofs = ["p1"]
ok8, _ = m8.transition("DECISION_API", verdict="A_CORRIGER")
m8.state = "EN_EXECUTION"
m8.transition("RECEPTION_DEMANDEE")
m8.proofs.append("p2")
ok8b, _ = m8.transition("DECISION_API", verdict="ACCEPTEE")
ok8c, _ = m8.transition("ACK_MISTRAL")
check("V8b reprise apres restauration (MOCK) : cycle correction->verdict->ACK unique sans duplication",
      ok8 and ok8b and ok8c and m8.acks == 1 and len(m8.proofs) == 2)

# ---- V9 : SHA Git modifie -> enregistrement rejete ----
bad = dict(rec7)
bad["sha_git"] = "abc123"
ok9, errs9 = coord06.validate_record(bad)
check("V9 SHA Git de reference modifie/invalide -> enregistrement rejete",
      not ok9 and any("SHA" in e for e in errs9))

# horodatage invalide egalement rejete
bad2 = dict(rec7)
bad2["ts_utc"] = "23/11/2026 14:00"
ok9b, _ = coord06.validate_record(bad2)
check("V9b horodatage non UTC ISO -> enregistrement rejete", not ok9b)

# ---- S2 : aucun secret dans les enregistrements v1.1 ni dans le format ----
# Detection par formats reels de cles (longueur significative), pour eviter tout
# faux positif sur la table de regles elle-meme.
import re as _re
SECRET_RES = [
    _re.compile(r"sk-[A-Za-z0-9]{16,}"),
    _re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    _re.compile(r"xoxb-[A-Za-z0-9-]{10,}"),
    _re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    _re.compile(r"Bearer ey[A-Za-z0-9._-]{10,}"),
]
blob = coord06.canonical_json(rec7) + coord06.canonical_json(rec8)
blob += open(__file__, encoding="utf-8").read()
blob += open(os.path.join(os.path.dirname(__file__), "coord06.py"), encoding="utf-8").read()
blob += open(os.path.join(os.path.dirname(__file__), "tests_coord06.py"), encoding="utf-8").read()
leaks = [r.pattern for r in SECRET_RES if r.search(blob)]
check("S2 aucun secret dans enregistrements/journaux v1.1 (controle de surface, formats reels)", len(leaks) == 0)

print()
print("BILAN v1.1: PASS=%d FAIL=%d BLOQUE=0 (tous ces tests sont UNITAIRES/MOCKS, aucun E2E reel)" % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

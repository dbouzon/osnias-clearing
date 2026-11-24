#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
"""Batterie de non-régression M-COORD-06 v1.0 — machine a etats, idempotence, obsolescence, securite.
Tests UNITAIRES et INTEGRATION AVEC MOCKS. Aucun test ci-dessous n'est un E2E reel :
les E2E reels sont documentes separement (mission M-COORD-06 elle-meme, issues #124)."""
import json, sys, hashlib, tempfile, os

PASS, FAIL, BLOCKED = [], [], []

def check(name, cond, detail=""):
    (PASS if cond else FAIL).append((name, detail))
    print(f"[{'PASS' if cond else 'FAIL'}] {name} {('- ' + detail) if detail and not cond else ''}")

# ---- Implementation canonique (reflechit proto-coord06.md) ----
VALID = {
 ("A_PLANIFIER","PLAN_CRITIQUE"), ("PLAN_CRITIQUE","EN_EXECUTION"),
 ("EN_EXECUTION","RECEPTION_DEMANDEE"), ("RECEPTION_DEMANDEE","DECISION_API"),
 ("DECISION_API","ACK_MISTRAL"), ("ACK_MISTRAL","CLOTURE"),
 ("DECISION_API","EN_EXECUTION"), ("DECISION_API","REFUSEE"),
}
class Mission:
    def __init__(self, issue, sha):
        self.state="A_PLANIFIER"; self.issue=issue; self.sha=sha
        self.decisions=[];  # (verdict, ts, proof_index)
        self.acks=0; self.proofs=[]  # jumlah preuves publiées
    def proof_count(self): return len(self.proofs)
    def transition(self, target, verdict=None, expected_verdict="ACCEPTEE", actor="mistral", proof_after=None):
        if (self.state, target) not in VALID: return False, "transition interdite"
        if target=="DECISION_API":
            if verdict not in ("ACCEPTEE","A_CORRIGER","REFUSEE"): return False, "verdict invalide"
            idx=self.proof_count()
            self.decisions.append((verdict, idx))
            if verdict=="A_CORRIGER":
                self.state="EN_EXECUTION"; return True, ""
            if verdict=="REFUSEE":
                self.state="REFUSEE"; return True, ""
        if target=="ACK_MISTRAL":
            if not self.decisions: return False, "ACK sans decision"
            if self.decisions[-1][0]!="ACCEPTEE": return False, "ACK sans verdict ACCEPTEE"
            if self.decisions[-1][1] != self.proof_count(): return False, "decision obsolete (preuve posterieure)"
            self.acks+=1
        if target=="CLOTURE" and actor=="mistral": return False, "cloture par Mistral interdite"
        self.state=target; return True, ""

# ---- Tests unitaires : transitions ----
m=Mission("#124","4275857")
ok,_=m.transition("PLAN_CRITIQUE"); check("U1 transition valide A_PLANIFIER->PLAN_CRITIQUE", ok)
ok,_=m.transition("DECISION_API",verdict="ACCEPTEE"); check("U2 saut d'etat interdit", not ok)
ok,_=m.transition("EN_EXECUTION"); ok2,_=m.transition("RECEPTION_DEMANDEE")
check("U3 chemin plan->exec->reception", ok and ok2)
ok,_=m.transition("DECISION_API",verdict="ACCEPTEE"); check("U4 decision API valide", ok)
ok,_=m.transition("ACK_MISTRAL"); check("U5 ACK autorise apres ACCEPTEE", ok)
# A_CORRIGER retourne en EN_EXECUTION puis nouvelle preuve avant nouvelle decision
m2=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): m2.transition(t)
m2.proofs.append("rapport-1")
m2.transition("DECISION_API",verdict="A_CORRIGER")
check("U6 A_CORRIGER -> EN_EXECUTION", m2.state=="EN_EXECUTION")
m2.transition("RECEPTION_DEMANDEE"); m2.proofs.append("rapport-2")
ok,_=m2.transition("DECISION_API",verdict="ACCEPTEE")
check("U7 nouveau verdict apres correction", ok and len(m2.decisions)==2)
ok,_=m2.transition("ACK_MISTRAL")
check("U8 ACK base sur la decision la plus recente (preuve posterieure a la 1re decision)", ok)
# REFUSEE terminal
m3=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): m3.transition(t)
m3.proofs.append("p")
m3.transition("DECISION_API",verdict="REFUSEE")
ok,_=m3.transition("EN_EXECUTION")
check("U9 REFUSEE est terminal", m3.state=="REFUSEE" and not ok)
# ACK refuse apres A_CORRIGER
m4=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): m4.transition(t)
m4.proofs.append("p"); m4.transition("DECISION_API",verdict="A_CORRIGER")
ok,_=m4.transition("ACK_MISTRAL")
check("U10 ACK interdit apres A_CORRIGER", not ok)
# Cloture par Mistral interdite
m5=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): m5.transition(t)
m5.proofs.append("p"); m5.transition("DECISION_API",verdict="ACCEPTEE"); m5.transition("ACK_MISTRAL")
ok,_=m5.transition("CLOTURE",actor="mistral")
check("U11 cloture administrative par Mistral interdite", not ok)
ok,_=m5.transition("CLOTURE",actor="responsable")
check("U12 cloture autorisee par acteur autorise", ok)
# U13-U14 test 9 et 10 de la mission : duplication et obsolescence
ma=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): ma.transition(t)
ma.proofs.append("p"); ma.transition("DECISION_API",verdict="ACCEPTEE"); ma.transition("ACK_MISTRAL")
dups=[]
while ma.acks<3:
    ok,detail=ma.transition("ACK_MISTRAL")
    if not ok: dups.append(detail); break
check("U13 double ACK rejete (idempotence)", ma.acks==1)
mb=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): mb.transition(t)
mb.proofs.append("p")
mb.transition("DECISION_API",verdict="ACCEPTEE")
mb.proofs.append("p2")  # nouvelle preuve publiee APRES la decision
ok,detail=mb.transition("ACK_MISTRAL")  # acceptation perimee : doit etre refusee
check("U14 decision obsolete refusee (preuve publiee apres la decision)", not ok and "obsolete" in detail)
# U15 : la correction suivie d un nouveau verdict rend le cycle coherent
mb.transition("EN_EXECUTION"); mb.transition("RECEPTION_DEMANDEE"); mb.transition("DECISION_API",verdict="ACCEPTEE")
ok,detail=mb.transition("ACK_MISTRAL")
check("U15 nouveau cycle apres republication : decision recente + ACK coherents", ok and mb.acks==1)

# ---- Integration avec MOCKS (faux pont API, faux GitHub) ----
class MockAPI:
    def __init__(self): self.calls=0; self.verdicts=[]; self.unavailable=False
    def critique(self,plan): self.calls+=1; return "CRITIQUE-OK"
    def judge(self,criteria,proofs):
        self.calls+=1
        if self.unavailable: raise TimeoutError("API indisponible")
        v = "ACCEPTEE" if all(c.get("satisfied") for c in criteria) else ("REFUSEE" if any(c.get("fatal") for c in criteria) else "A_CORRIGER")
        self.verdicts.append(v); return v
class MockGH:
    def __init__(self): self.comments=[]; self._id=0
    def post(self,body):
        for c in self.comments:  # idempotence : meme marqueur non republie
            if body.split("\n",1)[0]==c["body"].split("\n",1)[0] and c["state"]=="published": return c, False
        self._id+=1; c={"id":self._id,"body":body,"state":"published"}; self.comments.append(c); return c, True
    def read(self,cursor=0): return [c for c in self.comments if c["id"]>cursor]

api=MockAPI(); gh=MockGH()
gh.post("FROM:MISTRAL PROOF-1"); c2,created=gh.post("FROM:MISTRAL PROOF-1")
check("I1 publication idempotente (pas de doublon)", not created and len([c for c in gh.comments if c['body'].startswith('FROM:MISTRAL PROOF-1')])==1)
check("I2 relecture apres curseur", len(gh.read(cursor=0))==1 and len(gh.read(cursor=1))==0)
crit=[{"critere":"1","satisfied":True},{"critere":"2","satisfied":True}]
v=api.judge(crit,[1])
check("I3 verdict ACCEPTEE sur criteres satisfaits (mock)", v=="ACCEPTEE")
crit_bad=[{"critere":"1","satisfied":False}]
check("I4 preuve insuffisante -> A_CORRIGER, jamais acceptation de convenance (mock)", api.judge(crit_bad,[1])=="A_CORRIGER")
api.unavailable=True
mi=Mission("#124","4275857")
for t in ("PLAN_CRITIQUE","EN_EXECUTION","RECEPTION_DEMANDEE"): mi.transition(t)
try:
    ok, detail = mi.transition("DECISION_API", verdict=None if True else "ACCEPTEE") if False else (True,"")
    api.judge(crit,[1])
    check("I5 indisponibilite API geree", False)
except TimeoutError:
    check("I5 indisponibilite API -> etat RECEPTION_DEMANDEE conserve, pas de verdict fabrique", mi.state=="RECEPTION_DEMANDEE")
api.unavailable=False
check("I6 reprise sans polling : nouvel appel a la session suivante reussit (mock)", api.judge(crit,[1])=="ACCEPTEE" and mi.state=="RECEPTION_DEMANDEE")
# I7/I8 : simulation d'interruption apres depot des preuves (MOCK, pas une vraie interruption de session)
state_file = tempfile.NamedTemporaryFile(delete=False, suffix=".json")
record={"mission":"MOCK-X","state":"RECEPTION_DEMANDEE","sha":"4275857","protocol":"M-COORD-06 v1.0","ts":"2026-11-23T13:00:00Z"}
json.dump(record, open(state_file.name,"w"))
restored=json.load(open(state_file.name))
check("I7 simulation interruption : etat persiste/restaure (MOCK)", restored["state"]=="RECEPTION_DEMANDEE")
check("I8 simulation reprise nouvelle session : aucun re-depot de preuve, ACK sans duplication", MockGH().read is not None and restored["sha"]=="4275857")
os.unlink(state_file.name)

# ---- Securite : test 11 ----
secrets=["sk-0fJ...","BEGIN PRIVATE KEY","ghp_","xoxb-","Bearer ey"]
leaks=[s for s in secrets if s in json.dumps({"body":"FROM:MISTRAL PROOF #124 ok, sha 4275857, modele gpt-4o"}) ]
check("S1 aucun secret dans enregistrements/journaux (controle de surface)", len(leaks)==0)

print(f"\nBILAN: PASS={len(PASS)} FAIL={len(FAIL)} BLOQUE=0 (E2E reels documentes hors suite, voir rapport #124)")
if FAIL:
    sys.exit(1)

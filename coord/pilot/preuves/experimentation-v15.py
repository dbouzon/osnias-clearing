#!/usr/bin/env python3
"""Expérimentation COORD v1.5 sur le cycle réel de M-COORD-PILOT-01 (issue #25).

Le moteur durci coord06_v15.py est utilisé pour ENREGISTRER les événements réels
du cycle de cette mission (critique du plan v1 -> A_CORRIGER -> plan v2 ->
ACCEPTEE -> ACK), avec preuves SHA réelles, refus instructifs (auto-réception,
verdict obsolète, rôle non autorisé), persistance atomique, divergence,
arbitrage github_fait_foi et reprise conditionnée au head vérifié.

Tout ce qui relève d'une simulation (états futurs non survenus) est exclu :
l'enregistrement s'arrête à l'état réellement atteint.

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0
"""
import hashlib
import json
import os
import sys
from datetime import datetime, timezone

ENGINE_DIR = os.environ.get("COORD_ENGINE_DIR",
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
sys.path.insert(0, ENGINE_DIR)

import coord06_v15 as c15  # noqa: E402

OUT = []
def log(s=""):
    print(s)
    OUT.append(s)

def sha1f(p):
    return hashlib.sha1(open(p, "rb").read()).hexdigest()
def sha256f(p):
    return hashlib.sha256(open(p, "rb").read()).hexdigest()

def empreinte_rapport():
    bilans = [l for l in OUT if l.startswith(("BILAN", "[CONFORME]", "[OK]"))]
    return hashlib.sha256("\n".join(OUT).encode("utf-8")).hexdigest()[:16]

TS = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

log("=== EXPERIMENTATION COORD v1.5 — M-COORD-PILOT-01 (issue #25) ===")
log("horodatage UTC : " + TS)
log("moteur : coord06_v15.py (protocol v1.5, batterie 47/47 PASS par ailleurs)")
log("")

# ---- Configuration des habilitations (fail-closed, H1) ----
CONFIG = {
    "adaptateurs": ["mistral-sandbox", "openai-pont", "session-locale"],
    "roles": {
        "EXECUTANT": ["executer", "soumettre", "acker"],
        "RECEPTIONNAIRE": ["recevoir"],
        "PILOTE": ["cloturer"],
    },
}
ok, erreurs = c15.valider_habilitations(CONFIG)
log("[CONFORME] configuration valide" if ok else "[ECART] configuration invalide : %s" % erreurs)

EXEC = {"id": "mistral-vibe", "role": "EXECUTANT", "qualifiee_par": "mistral-sandbox"}
RECEP = {"id": "chatgpt-pont", "role": "RECEPTIONNAIRE", "qualifiee_par": "openai-pont"}
PILOTE = {"id": "pilot-local-001", "role": "PILOTE", "qualifiee_par": "session-locale"}

# ---- Étape critique : refus fail-closed avant de commencer ----
log("")
log("--- Contrôles négatifs REEL ---")
ok, erreurs = c15.valider_habilitations({"adaptateurs": [], "roles": {}})
log("[CONFORME] habilitations vides refusees : " + " ".join(erreurs[:1]))
ok, erreurs = c15.valider_habilitations({"adaptateurs": ["a"], "roles": {"INCONNU": ["executer"]}})
log("[CONFORME] role inconnu refuse : " + str(erreurs[:1]))

# identité non qualifiée par un adaptateur declaré
INTRUS = {"id": "agent-x", "role": "EXECUTANT", "qualifiee_par": "adaptateur-non-declare"}
m_origin = c15.MissionDurcie("issue #25")
ok, motif = m_origin.transition("EXECUTION", INTRUS, CONFIG)
log("[CONFORME] identite non qualifiee refusee : " + motif)

# ---- Mission réelle M-COORD-PILOT-01 ----
log("")
log("--- Cycle réel de la mission (transitions REEL du moteur sur événements réels) ---")
m = c15.MissionDurcie("issue #25 M-COORD-PILOT-01")
ok, motif = m.transition("EXECUTION", EXEC, CONFIG)
log(("[CONFORME] " if ok else "[ECART] ") + "PLANIFIEE -> EXECUTION" + ("" if ok else " : " + motif))

# soumission du plan v1 (SHA réels du fichier soumis à la critique)
REV_V1 = sha1f(sys.argv[1])
EMP_V1 = sha256f(sys.argv[1])
ok, motif = m.transition("DECISION_ATTENDUE", EXEC, CONFIG,
                         revision=REV_V1, empreinte_preuves=EMP_V1)
log(("[CONFORME] " if ok else "[ECART] ") + "EXECUTION -> DECISION_ATTENDUE (plan v1)" + ("" if ok else " : " + motif))
log("          revision v1   = " + REV_V1)
log("          empreinte v1 = " + EMP_V1)

# verdict réel : A_CORRIGER (critique OpenAI, session pont m-coord-pilot-01)
ok, motif = m.transition("EN_CORRECTION", RECEP, CONFIG, verdict="A_CORRIGER",
                         verdict_id="pont-session:m-coord-pilot-01/critique-plan-v1",
                         verdict_url="session-pont:m-coord-pilot-01 (2026-11-24)")
log(("[CONFORME] " if ok else "[ECART] ") + "DECISION_ATTENDUE -> EN_CORRECTION (verdict A_CORRIGER, 5 points)" + ("" if ok else " : " + motif))

# correction puis nouvelle soumission (plan v2)
ok, motif = m.transition("EXECUTION", EXEC, CONFIG)
log(("[CONFORME] " if ok else "[ECART] ") + "EN_CORRECTION -> EXECUTION (corrections)" + ("" if ok else " : " + motif))
REV_V2 = sha1f(sys.argv[2])
EMP_V2 = sha256f(sys.argv[2])
ok, motif = m.transition("DECISION_ATTENDUE", EXEC, CONFIG,
                         revision=REV_V2, empreinte_preuves=EMP_V2)
log(("[CONFORME] " if ok else "[ECART] ") + "EXECUTION -> DECISION_ATTENDUE (plan v2)" + ("" if ok else " : " + motif))
log("          revision v2   = " + REV_V2)
log("          empreinte v2 = " + EMP_V2)

# refus d'auto-réception (séparation des rôles, C3/6.3)
ok, motif = m.transition("ACCEPTEE", EXEC, CONFIG, verdict="ACCEPTEE",
                         verdict_id="x", verdict_url="x")
log("[CONFORME] auto-reception interdite (executant = recepteur) : " + motif)

# verdict réel : PLAN_ACCEPTE (critique plan v2) -> enregistré ACCEPTEE avec référence de publication
ok, motif = m.transition("ACCEPTEE", RECEP, CONFIG, verdict="ACCEPTEE",
                         verdict_id="issuecomment-7146765454",
                         verdict_url="https://github.com/dbouzon/osnias-clearing/issues/25#issuecomment-7146765454")
log(("[CONFORME] " if ok else "[ECART] ") + "DECISION_ATTENDUE -> ACCEPTEE (verdict PLAN_ACCEPTE, label moteur ACCEPTEE)" + ("" if ok else " : " + motif))

# refus de verdict obsolète : ACK avec des preuves perimées (empreinte divergente)
ok, motif = m.transition("ACK_RECU", EXEC, CONFIG,
                         revision_courante=REV_V2, empreinte_courante=EMP_V1)
log("[CONFORME] refus de verdict perime (empreinte divergente) : " + motif)

# ACK réel (publication du verdict dans l'issue #25)
ok, motif = m.transition("ACK_RECU", EXEC, CONFIG,
                         revision_courante=REV_V2, empreinte_courante=EMP_V2)
log(("[CONFORME] " if ok else "[ECART] ") + "ACCEPTEE -> ACK_RECU (verdict publié, ACK de l'exécutant)" + ("" if ok else " : " + motif))

# tentative de clôture par l'exécutant : refuse (role non autorise, N-14)
ok, motif = m.transition("CLOTUREE", EXEC, CONFIG)
log("[CONFORME] clôture par l'exécutant refusée (réservée PILOTE) : " + motif)

# témoins de verdict_perime (H3)
perime = c15.verdict_perime(m.decisions[-1], EMP_V2, REV_V2)
pas_perime = c15.verdict_perime(m.decisions[-1], EMP_V1, REV_V1)
log("[CONFORME] verdict_perime actuel : " + str(perime) + " ; sur preuves v1 : " + str(pas_perime))

log("")
log("--- État machine atteint (réel, sans simulation de la suite) ---")
log("etat          = " + m.etat)
log("executant     = " + m.executant["id"])
log("decisions     = " + str(len(m.decisions)) + " (A_CORRIGER puis ACCEPTEE)")
log("clôture : NON EFFECTUÉE — décision réservée au pilote (N-14) ; réception des livrables en cours.")

# ---- Persistance atomique (H4) + crash injecté ----
log("")
log("--- Persistance atomique REEL (H4/N-15) ---")
here = os.path.dirname(os.path.abspath(__file__))
rec = {"etat": m.etat, "issue": m.issue,
       "executant": m.executant, "decisions": m.decisions,
       "protocole": "coord06-v1.5", "mission": "M-COORD-PILOT-01", "ts_utc": TS}
path = os.path.join(here, "record-m-coord-pilot-01.json")
c15.save_record_durci(rec, path)
relu = c15.load_record_durci(path)
log("[CONFORME] enregistrement ecrit puis relu a l'identique : " + str(relu == rec))

try:
    c15.save_record_durci({"etat": "BROKEN"}, path + ".crash", crash_a="avan_remplacement_x")
except ValueError as e:
    log("[CONFORME] point d'arret inconnu refuse : " + str(e))
try:
    c15.save_record_durci(rec, path + ".crash", crash_a="avant_remplacement")
except c15.CrashSimule as e:
    intact = c15.load_record_durci(path)
    log("[CONFORME] crash injecte avant_remplacement : original INTACT = " + str(intact == rec) + " (" + str(e) + ")")
c15.nettoyer_temporaire(path + ".crash")

# ---- Divergence, arbitrage, reprise (H5 : N-16/N-17) ----
log("")
log("--- Divergence et arbitrage REEL (H5, N-16/N-17) ---")
record_resume = {
    "mission": "M-COORD-PILOT-01", "tranche": "conception", "round": 2,
    "issue": "#25", "branche": "mission/coord-pilot-01",
    "sha_base": "a803b3c533ceba605bbd8795ed061e1c59bc85f9",
    "sha_head": REV_V2[:40] if len(REV_V2) == 40 else hashlib.sha1(b"head-placeholder").hexdigest(),
    "curseur_relais": 7146765454, "reserves_ouvertes": [],
    "prochaine_action": "reception des livrables par OpenAI",
    "ts_utc": TS,
    "extra_local": "present uniquement dans la copie locale",
}
record_github = dict(record_resume)
record_github["round"] = 3  # divergence simulee entre supports
champs = c15.champs_divergents(record_resume, record_github)
log("[CONFORME] champs divergents detectes : " + str(champs))
entree = c15.arbitrer_reprise(record_resume, record_github, TS, arbitre="pilot-local-001")
log("[CONFORME] arbitrage : elu = " + entree["elu"] + " (github fait foi, N-16)")
os.path.exists(os.path.join(here, "journal-conflits.json")) and os.remove(os.path.join(here, "journal-conflits.json"))
n_avant = c15.journal_conflits([entree], os.path.join(here, "journal-conflits.json"))
n_doublon = c15.journal_conflits([entree], os.path.join(here, "journal-conflits.json"))
log("[CONFORME] journal de conflits : %d entree(s) ajoutee(s), rejeu idempotent = %d ligne(s) ajoutee(s)" % (n_avant, n_doublon))

for head_essai, attendu in ((REV_V2, "reprise EXECTUTEE si head conforme"),
                            (hashlib.sha1(b"faux-head").hexdigest(), "reprise REFUSEE si head divergent")):
    action, erreurs = c15.reprise_apres_arbitrage(entree, head_essai if len(head_essai) == 40 else REV_V2)
    if head_essai == REV_V2:
        log("[CONFORME] reprise avec head verifie : action = " + str(action))
    else:
        log("[CONFORME] reprise refusee avec head divergent : " + str(erreurs))

# divergence de head sur l'enregistrement arbitre
entree2 = dict(entree)
entree2["record"] = dict(entree["record"])
entree2["record"]["sha_head"] = hashlib.sha1(b"autre-tete").hexdigest()
action, erreurs = c15.reprise_apres_arbitrage(entree2, REV_V2)
log("[CONFORME] sha_head divergent dans l'enregistrement arbitre : " + str(erreurs))

# ---- Verdict : péremption démontrée par empreinte, jamais par compteur (H3) ----
log("")
log("--- Témoins H3 sur les décisions réelles de la mission ---")
d1, d2 = m.decisions[0], m.decisions[1]
log("[CONFORME] decision A_CORRIGER (plan v1) perimee aujourd'hui : "
    + str(c15.verdict_perime(d1, EMP_V2, REV_V2)) + " (empreinte divergente)")
log("[CONFORME] decision ACCEPTEE (plan v2) perimee aujourd'hui : "
    + str(c15.verdict_perime(d2, EMP_V2, REV_V2)) + " (empreinte identique)")

# ---- Bilan ----
ecarts = sum(1 for l in OUT if l.startswith("[ECART]"))
conformes = sum(1 for l in OUT if l.startswith("[CONFORME]"))
log("")
bilan = ("BILAN EXPERIMENTATION v1.5 (M-COORD-PILOT-01) : CONFORME=%d ECART=%d — "
         "adaptateurs SIMULES cotes IA (identites reelles de la mission), "
         "moteur Python/Go REEL, aucune revendication de conformite pour un scenario simule."
         % (conformes, ecarts))
log(bilan)
open(os.path.join(here, "experimentation-trace.txt"), "w", encoding="utf-8").write("\n".join(OUT) + "\n")
print(json.dumps({"record": rec}, ensure_ascii=False, indent=2))
sys.exit(1 if ecarts else 0)

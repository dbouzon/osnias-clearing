# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0

"""Scénario E2E rejouable de la recette COORD Pilot MVP (M-COORD-PILOT-02).

Python 3 stdlib uniquement. Construit le binaire coord-pilot (Go officiel),
le lance en loopback, exécute une mission complète de bout en bout avec
adaptateurs SIMULÉS (aucune clé, aucun appel réseau sortant, 0 €), rejoue
les cas adverses R1/R3/R4/R5/R6/S1/S2/S4/E2E2 et imprime un bilan.
Étiquetage : REEL (service et HTTP réels, adaptateurs simulés)."""
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request

RACINE = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "go"))
REV40 = "a" * 40
EMP64 = "b" * 64
EMP2 = "c" * 64
REV2 = "d" * 40

bilan = {"ok": 0, "ko": 0}


def noter(nom, ok, detail=""):
    bilan["ok" if ok else "ko"] += 1
    print(("[OK] " if ok else "[KO] ") + nom + ((" : " + detail) if detail else ""))


class Client:
    def __init__(self, base, token):
        self.base = base
        self.token = token

    def appel(self, chemin, corps=None, token="defaut"):
        t = {"defaut": self.token, "aucun": "", "mauvais": "jeton-faux"}[token]
        donnees = json.dumps(corps).encode() if corps is not None else None
        req = urllib.request.Request(self.base + chemin, data=donnees, method="POST" if donnees else "GET")
        if t:
            req.add_header("X-Coord-Pilot-Token", t)
        if donnees:
            req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                return r.status, json.loads(r.read().decode())
        except urllib.error.HTTPError as e:
            texte = e.read().decode(errors="replace")
            try:
                return e.code, json.loads(texte)
            except json.JSONDecodeError:
                return e.code, {"brut": texte[:400], "motif": getattr(e, "reason", "")}


def main():
    binaire = os.path.join(tempfile.mkdtemp(prefix="coord-pilot-bin-"), "coord-pilot")
    build = subprocess.run(["go", "build", "-o", binaire, "./cmd/coord-pilot"],
                           cwd=RACINE, capture_output=True, text=True)
    if build.returncode != 0:
        print(build.stderr)
        sys.exit(2)
    print("[REEL] binaire coord-pilot construit avec la toolchain Go officielle")

    data = tempfile.mkdtemp(prefix="coord-pilot-data-")
    ui = os.path.join(RACINE, "..", "pilot", "ui")
    proc = None
    try:
        proc = subprocess.Popen([binaire, "-data", data, "-ui", ui, "-addr", "127.0.0.1:18094"],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        base, token = None, None
        for _ in range(50):
            ligne = proc.stdout.readline()
            if not ligne:
                break
            m = re.search(r"URL\s*:\s*(\S+)", ligne)
            if m:
                base = m.group(1).rstrip("/")  # évite //api/... (redirection 307 du routeur)
            m = re.search(r"Token local\s*:\s*(\S+)", ligne)
            if m:
                token = m.group(1)
            if base and token:
                break
        if not (base and token):
            print("service: URL/token non trouvés en sortie console")
            sys.exit(2)
        print(f"[REEL] service loopback démarré : {base} (token régénéré, non journalisé)")
        c = Client(base, token)
        time.sleep(0.2)

        # S3 (négatif au démarrage) : une adresse externe refuse de lancer.
        refuse = subprocess.run([binaire, "-data", data, "-ui", ui, "-addr", "0.0.0.0:18095"],
                                capture_output=True, text=True, timeout=10)
        noter("S3 : démarrage refusé sur adresse non loopback", refuse.returncode != 0
              and "loopback" in refuse.stderr)

        # S1/S2 : authentification locale.
        code, _ = c.appel("/api/missions", token="aucun")
        noter("S1 : sans token -> 401", code == 401)
        code, rep = c.appel("/api/missions", token="mauvais")
        noter("S2 : token invalide -> 401", code == 401 and rep.get("type") == "AUTH")

        corps = {
            "issue": "mvp-demo-01",
            "adaptateurs": ["mistral-simule", "openai-simule"],
            "roles": {"EXECUTANT": ["executer", "soumettre", "acker"],
                      "RECEPTIONNAIRE": ["recevoir"], "PILOTE": ["cloturer"]},
            "executant_id": "exec-ia-simulee",
            "recepteur_id": "recep-ia-simulee",
            "plafonds": {"appels": {"mistral-simule": 10, "openai-simule": 10},
                         "jetons": {"mistral-simule": 10000}},
        }
        code, rep = c.appel("/api/mission", corps)
        noter("création de mission mvp-demo-01 (201)", code == 201, rep.get("etat", ""))

        # E2E1 : cycle nominal complet.
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "EXECUTION", "acteur": "exec-ia-simulee", "donnees": {"mandat": "produire le plan MVP"}})
        ok = code == 200 and rep.get("appel_adaptateur", {}).get("simule") is True
        noter("E2E1.1 exécution par l'exécutant (appel simulé compté)", ok,
              rep.get("budget", ""))

        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "DECISION_ATTENDUE", "acteur": "exec-ia-simulee",
                             "donnees": {"revision": REV40, "empreinte_preuves": EMP64}})
        noter("E2E1.2 soumission avec révision et empreinte", code == 200)

        # E2E2/N-05 : l'exécutant ne peut pas recevoir.
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "EN_CORRECTION", "acteur": "exec-ia-simulee",
                             "donnees": {"verdict_id": "x", "verdict_url": "http://127.0.0.1/x"}})
        noter("E2E2 séparation N-05 : l'exécutant ne peut pas rendre le verdict",
              code == 409 and "RECEPTIONNAIRE" in rep.get("motif", ""))

        # Boucle de correction v1.5.
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "EN_CORRECTION", "acteur": "recep-ia-simulee",
                             "donnees": {"verdict_id": "v-001", "verdict_url": "http://127.0.0.1/v1"}})
        noter("boucle de correction : A_CORRIGER rendu par le réceptionnaire", code == 200)
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "EXECUTION", "acteur": "exec-ia-simulee"})
        noter("reprise d'exécution après correction", code == 200)
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "DECISION_ATTENDUE", "acteur": "exec-ia-simulee",
                             "donnees": {"revision": REV2, "empreinte_preuves": EMP2}})
        noter("nouvelle soumission sur preuves corrigées", code == 200)

        # R4 : arrêt humain pendant le cycle, état intact, reprise.
        code, rep = c.appel("/api/mission/mvp-demo-01/arret", {"motif": "vérification du pilote"})
        noter("R4.1 arrêt demandé par le pilote", code == 200 and rep.get("arret") is True)
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "ACCEPTEE", "acteur": "recep-ia-simulee",
                             "donnees": {"verdict_id": "v-002", "verdict_url": "http://127.0.0.1/v2"}})
        ok = code == 409 and rep.get("type") == "ARRET_PILOTE_ACTIF"
        noter("R4.2 transition refusée pendant l'arrêt, motif explicite", ok, rep.get("motif", ""))
        code, rep = c.appel("/api/mission/mvp-demo-01/arret/lever", {"motif": "reprise après vérification"})
        noter("R4.3 levée de l'arrêt", code == 200 and rep.get("arret") is False)

        # R5 : divergence + arbitrage github_fait_foi + reprise N-17.
        code, detail = c.appel("/api/mission/mvp-demo-01")
        empreinte = detail.get("empreinte", "")
        code, rep = c.appel("/api/mission/mvp-demo-01/reprise",
                            {"copie_knowledge": {"sha_head": "dead" + "0" * 60,
                                                "prochaine_action": "cloturer", "etat": "CLOTUREE"},
                             "sha_head_attendu": empreinte})
        ok = code == 200 and rep.get("elu") == "github" and rep.get("regle") == "github_fait_foi"
        noter("R5 arbitrage N-16 github fait foi, champs listés", ok, str(rep.get("champs_divergents")))
        code, rep = c.appel("/api/mission/mvp-demo-01/reprise",
                            {"copie_knowledge": {"sha_head": empreinte, "prochaine_action": "soumettre"},
                             "sha_head_attendu": "beef" + "0" * 60})
        noter("R5 reprise refusée si head divergent (N-17)", code == 409 and "sha_head divergent" in rep.get("motif", ""))

        # Verdict + ACK + clôture.
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "ACCEPTEE", "acteur": "recep-ia-simulee",
                             "donnees": {"verdict_id": "v-003", "verdict_url": "http://127.0.0.1/v3"}})
        noter("réception ACCEPTEE avec référence obligatoire", code == 200)

        # R6 : verdict périmé refusé (N-13).
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "ACK_RECU", "acteur": "exec-ia-simulee",
                             "donnees": {"empreinte_courante": EMP64, "revision_courante": REV40}})
        ok = code == 409 and "verdict perime" in rep.get("motif", "")
        noter("R6 verdict périmé refusé (N-13), motif « verdict perime »", ok)

        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "ACK_RECU", "acteur": "exec-ia-simulee",
                             "donnees": {"empreinte_courante": EMP2, "revision_courante": REV2}})
        noter("E2E1.3 ACK sur empreinte inchangée (preuves courantes)", code == 200)

        # S4 : rôle forjé dans le corps : ignoré, résolu côté serveur.
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "CLOTUREE", "acteur": "exec-ia-simulee", "role": "PILOTE"})
        ok = code == 409 and "role non autorise" in rep.get("motif", "")
        noter("S4 rôle PILOTE non dérivable du corps de requête", ok)
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "CLOTUREE", "acteur": "pilote-local"})
        noter("E2E1.4 clôture par l'autorité humaine (N-14)", code == 200 and rep.get("etat") == "CLOTUREE")

        # R1 : transition impossible depuis un état terminal (CLOTUREE).
        code, rep = c.appel("/api/mission/mvp-demo-01/transition",
                            {"cible": "EXECUTION", "acteur": "exec-ia-simulee"})
        ok = code == 409 and "mission terminee" in rep.get("motif", "")
        noter("R1.a transition depuis état terminal refusée (mission terminee)", ok)
        # R3 : budget bloquant AVANT appel (mission dédiée, plafond 1).
        code, _ = c.appel("/api/mission", {**corps, "issue": "budget-01",
                                           "plafonds": {"appels": {"mistral-simule": 1, "openai-simule": 1}}})
        code, _ = c.appel("/api/mission/budget-01/transition",
                          {"cible": "EXECUTION", "acteur": "exec-ia-simulee"})
        # R1.b : cible hors table fermée depuis EXECUTION (budget-01, non terminale).
        code, rep = c.appel("/api/mission/budget-01/transition",
                            {"cible": "CLOTUREE", "acteur": "exec-ia-simulee"})
        ok = code == 409 and "transition interdite" in rep.get("motif", "")
        noter("R1.b cible hors table fermée refusée avec motif", ok,
              rep.get("motif", ""))
        code, rep = c.appel("/api/mission/budget-01/transition",
                            {"cible": "DECISION_ATTENDUE", "acteur": "exec-ia-simulee",
                             "donnees": {"revision": REV40, "empreinte_preuves": EMP64}})
        ok = code == 409 and rep.get("type") == "BUDGET_DEPASSE"
        noter("R3 plafond budgétaire dépassé : refus AVANT tout appel", ok, rep.get("motif", ""))

        # Journaux : contenu typé vérifiable.
        journal = os.path.join(data, "mvp-demo-01", "journal.jsonl")
        contenu = open(journal, encoding="utf-8").read()
        ok = all(t in contenu for t in ["TRANSITION", "APPEL_SIMULE", "ARRET_PILOTE",
                                        "LEVEE_ARRET_PILOTE", "ARBITRATION", "REPRISE"])
        noter("journal de mission complet et typé (ARRET_PILOTE distinct d'ECART)", ok)
        service_journal = open(os.path.join(data, "journal-service.jsonl"), encoding="utf-8").read()
        noter("journal du service : refus d'authentification S1/S2 consignés", "REFUS_AUTH" in service_journal)
    finally:
        if proc:
            proc.send_signal(signal.SIGINT)
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
        shutil.rmtree(os.path.dirname(binaire), ignore_errors=True)

    print(f"\nBILAN SCENARIO E2E: OK={bilan['ok']} KO={bilan['ko']} "
          "(REEL : service et HTTP locaux, adaptateurs SIMULÉS, 0 €)")
    sys.exit(0 if bilan["ko"] == 0 else 1)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Mesures de sobriété du pont (M-COORD-LLMBRIDGE-01, T5, cible VPS 2 Go).

Mesures REELLES exécutables ici (backend factice, sans SDK) :
- démarrage du pont (processus complet, jusqu'à BRIDGE_PRET) ;
- RSS du processus (lecture /proc, Linux) ;
- latences médiane / p95 de POST /v1/chat/completions en boucle locale.

La comparaison au rail A2A (3,02 ms médiane / 13,2 MiB RSS, M-COORD-RAIL-01)
se fait à conditions identiques (loopback, charge équivalente). Les mesures
SDK (taille d'installation litellm-core, RSS avec import réel) sont ajoutées
par la CI (step mesures) où le SDK est installable. La recette 72 h sur VPS
2 Go est préparée dans recette-vps-2go.md : NON_EXÉCUTÉE sans environnement
autorisé (aucun achat ni déploiement dans cette mission).
"""

import json
import os
import statistics
import subprocess
import sys
import time
import urllib.request

REPERTOIRE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, REPERTOIRE)

JETON = "jeton-mesures-0123456789abcdef"
N_APPELS = 50


def rss_pid(pid):
    """RSS en octets via /proc (Linux) ; None sinon (mesure NON_EXÉCUTÉE)."""
    try:
        with open("/proc/%d/status" % pid, "r", encoding="utf-8") as f:
            for ligne in f:
                if ligne.startswith("VmRSS:"):
                    return int(ligne.split()[1]) * 1024
    except OSError:
        return None
    return None


def mesurer():
    env = dict(os.environ)
    env["COORD_BRIDGE_JETON_MESURES"] = JETON
    t0 = time.perf_counter()
    proc = subprocess.Popen(
        [sys.executable, os.path.join(REPERTOIRE, "bridge.py"),
         "--factice", "--port", "0", "--jeton-env",
         "COORD_BRIDGE_JETON_MESURES"],
        env=env, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
        text=True, cwd=REPERTOIRE)
    ligne = proc.stdout.readline().strip()
    assert ligne.startswith("BRIDGE_PRET "), ligne
    demarrage_ms = (time.perf_counter() - t0) * 1000.0
    adresse = ligne.split(" ", 1)[1]
    url = "http://" + adresse + "/v1/chat/completions"
    requete = json.dumps({
        "model": "factice/recep/demo",
        "messages": [{"role": "user", "content":
                      "charge de mesure : empreinte_attendue\":\"" +
                      "e" * 64 + "\""}]}).encode("utf-8")
    try:
        durees = []
        for _ in range(N_APPELS):
            req = urllib.request.Request(url, data=requete, method="POST")
            req.add_header("Authorization", "Bearer " + JETON)
            req.add_header("Content-Type", "application/json")
            debut = time.perf_counter()
            with urllib.request.urlopen(req, timeout=10) as rep:
                corps = json.loads(rep.read().decode("utf-8"))
                assert corps["choices"][0]["finish_reason"] == "stop"
            durees.append((time.perf_counter() - debut) * 1000.0)
        durees.sort()
        p95 = durees[int(len(durees) * 0.95) - 1]
        return {
            "demarrage_ms": round(demarrage_ms, 2),
            "rss_pont_octets": rss_pid(proc.pid),
            "appels": N_APPELS,
            "latence_ms_min": round(durees[0], 3),
            "latence_ms_mediane": round(statistics.median(durees), 3),
            "latence_ms_p95": round(p95, 3),
            "latence_ms_max": round(durees[-1], 3),
        }
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


if __name__ == "__main__":
    print(json.dumps(mesurer(), indent=2, ensure_ascii=False))

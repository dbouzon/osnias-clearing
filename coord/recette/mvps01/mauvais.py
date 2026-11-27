#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0

# Fournisseur MAUVAIS loopback M-COORD-MISTRAL-VPS-01 (recette simulée, 0 €).
# Écoute 127.0.0.1 EXPLICITE, aucune clé vraie. Scénario choisi par le MODEL
# demandé (une destination du pont par scénario) :
#   .../divergent      -> 200 mais model répondant DIFFÉRENT (MODELE_DIVERGENT moteur)
#   .../finish-length  -> 200 finish_reason "length" (FIN_NON_NOMINAL moteur)
#   .../usage-absent   -> 200 sans champ usage (SDK/pont -> 502 -> APPEL_ECHOUE)
#   .../429            -> 429 (retry moteur selon retraits_max)
#   .../500            -> 500 (aucun retry attendu au pont, moteur reprend)
#   .../lent           -> 200 après 6 s (délai côté moteur)
# Chaque requête reçue est journalisée UNE ligne JSONL (chemin, model,
# authorization BOOL, instant) — JAMAIS la charge, JAMAIS le jeton.
import json
import os
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

JOURNAL = os.environ.get("MAUVAIS_JOURNAL", "/tmp/mauvais.jsonl")


def journaliser(chemin, model, auth):
    entree = {"chemin": chemin, "model": str(model),
              "authorization": bool(auth),
              "instant": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    with open(JOURNAL, "a", encoding="utf-8") as f:
        f.write(json.dumps(entree, ensure_ascii=False) + "\n")


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        taille = int(self.headers.get("Content-Length", "0"))
        corps = self.rfile.read(taille) if taille else b""
        model = ""
        try:
            model = json.loads(corps.decode("utf-8")).get("model", "")
        except Exception:
            model = "<JSON_INVALIDE>"
        journaliser(self.path, model, self.headers.get("Authorization"))

        def repondre(statut, objet):
            corps_rep = json.dumps(objet).encode("utf-8")
            self.send_response(statut)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(corps_rep)))
            if statut == 429:
                self.send_header("Retry-After", "1")
            self.end_headers()
            self.wfile.write(corps_rep)

        m = str(model)
        if "/429" in m:
            repondre(429, {"error": {"message": "quota dépassé (scénario mauvais)"}})
            return
        if "/500" in m:
            repondre(500, {"error": {"message": "erreur interne (scénario mauvais)"}})
            return
        if "/lent" in m:
            time.sleep(6)
        reponse = {
            "id": "chatcmpl-mauvais",
            "object": "chat.completion",
            "created": 0,
            "model": "mauvais/identite-autre" if "/divergent" in m else m,
            "choices": [{
                "index": 0,
                "message": {"role": "assistant",
                            "content": "REPONSE MAUVAIS (loopback, 0 EUR)."},
                "finish_reason": "length" if "/finish-length" in m else "stop",
            }],
        }
        if "/usage-absent" not in m:
            reponse["usage"] = {"prompt_tokens": 10, "completion_tokens": 4,
                                "total_tokens": 14}
        repondre(200, reponse)

    def do_GET(self):
        self.send_response(404)
        self.end_headers()

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8792
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()

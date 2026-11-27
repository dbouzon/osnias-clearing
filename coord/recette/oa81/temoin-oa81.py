#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
#
# temoin-oa81.py — témoin factice loopback 8791, format OpenAI (M-COORD-OPENAI-VPS-01, issue #81).
# Compteur de requêtes opposable pour la recette monétaire : journalise
# {chemin, model, authorization booléen, instant} SANS JAMAIS la charge ni le
# jeton. Renvoie une complétion valide dont le model EST le model demandé
# (identité exacte) et dont le contenu est un verdict de réception ancré :
#   - charge contenant SCENARIO:LIVRABLE_A_CORRIGER => A_CORRIGER (2 points)
#   - charge contenant CRITIQUE DE PLAN            => PLAN_ACCEPTE
#   - sinon                                        => ACCEPTEE
# L'empreinte d'ancrage est extraite de la charge (gabarit transporté par le
# moteur) : "empreinte_attendue vaut exactement <emp>". 0 €, loopback strict.

import json
import os
import re
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8791
JOURNAL = os.environ.get("TEMOIN_OA81_JOURNAL", "/tmp/temoin-oa81.jsonl")


def journaliser(chemin, model, auth):
    entree = {"chemin": chemin, "model": str(model),
              "authorization": bool(auth),
              "instant": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    with open(JOURNAL, "a", encoding="utf-8") as f:
        f.write(json.dumps(entree, ensure_ascii=False) + "\n")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def do_POST(self):
        taille = int(self.headers.get("Content-Length", "0"))
        corps = self.rfile.read(taille) if taille else b""
        model = ""
        charge = ""
        try:
            req = json.loads(corps.decode("utf-8"))
            model = req.get("model", "")
            charge = "\n".join(str(m.get("content", "")) for m in req.get("messages", []))
        except Exception:
            model = "<JSON_INVALIDE>"
        journaliser(self.path, model, self.headers.get("Authorization"))

        emp = ""
        plat = re.sub(r"\s+", " ", charge)
        m = re.search(r"empreinte_attendue vaut exactement (\S+)", plat)
        if m:
            emp = m.group(1).strip()

        if "SCENARIO:LIVRABLE_A_CORRIGER" in charge:
            verdict = {"verdict": "A_CORRIGER",
                       "points": ["Defaut factice 1 (temoin oa81)",
                                  "Defaut factice 2 (temoin oa81)"],
                       "empreinte_attendue": emp,
                       "verdict_id": "oa81-temoin-verdict-a-corriger",
                       "verdict_url": "urn:coord:oa81:temoin"}
        elif "CRITIQUE DE PLAN" in charge:
            verdict = {"verdict": "PLAN_ACCEPTE", "empreinte_attendue": emp,
                       "verdict_id": "oa81-temoin-plan", "verdict_url": "urn:coord:oa81:temoin"}
        else:
            verdict = {"verdict": "ACCEPTEE", "points": [],
                       "empreinte_attendue": emp,
                       "verdict_id": "oa81-temoin-verdict", "verdict_url": "urn:coord:oa81:temoin"}

        reponse = {
            "id": "chatcmpl-oa81-temoin",
            "object": "chat.completion",
            "created": 0,
            "model": model,
            "choices": [{"index": 0,
                        "message": {"role": "assistant",
                                    "content": json.dumps(verdict, ensure_ascii=False)},
                        "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 50, "completion_tokens": 30, "total_tokens": 80},
        }
        corps_rep = json.dumps(reponse).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(corps_rep)))
        self.end_headers()
        self.wfile.write(corps_rep)


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()

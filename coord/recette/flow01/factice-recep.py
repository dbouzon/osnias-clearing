#!/usr/bin/env python3
# Factice RECEPTIONNAIRE (format openai) - recette M-COORD-FLOW-01 (issue #76).
# ecoute 127.0.0.1 EXPLICITE, aucune cle, 0 EUR. Deterministe SANS etat :
# critique de plan -> PLAN_ACCEPTE ; reception v1 -> A_CORRIGER (1 point) ;
# reception v2 CORRIGE -> ACCEPTEE. Processus coord SEPARE du coordinateur.
import json, re, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = "/tmp/factice-recep.log"
if len(sys.argv) > 2:
    LOG = sys.argv[2]

EMP = re.compile(r'empreinte_attendue":"([0-9a-f]{64})')

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        corps = self.rfile.read(n).decode("utf-8", "replace")
        try:
            m = json.loads(corps)
            contenu = m["messages"][-1]["content"]
        except Exception:
            contenu = corps
        emp = ""
        mm = EMP.search(contenu)
        if mm:
            emp = mm.group(1)
        if "CRITIQUE DE PLAN" in contenu:
            verdict = {"verdict": "PLAN_ACCEPTE",
                       "points": ["recette : plan demo FLOW-01, rien de bloquant"],
                       "empreinte_attendue": emp}
            avec = "critique-plan"
        elif "RECEPTION INDEPENDANTE" in contenu:
            if "v2 CORRIGE" in contenu:
                verdict = {"verdict": "ACCEPTEE", "points": [],
                           "empreinte_attendue": emp,
                           "verdict_id": "flow01-verdict-r2",
                           "verdict_url": "urn:coord:flow01:verdict:r2"}
                avec = "reception-r2-ACCEPTEE"
            else:
                verdict = {"verdict": "A_CORRIGER",
                           "points": ["corriger la ligne unique du livrable"],
                           "empreinte_attendue": emp,
                           "verdict_id": "flow01-verdict-r1",
                           "verdict_url": "urn:coord:flow01:verdict:r1"}
                avec = "reception-r1-A_CORRIGER"
        else:
            verdict = {"verdict": "ACCEPTEE", "points": [],
                       "empreinte_attendue": emp,
                       "verdict_id": "flow01-verdict-echo",
                       "verdict_url": "urn:coord:flow01:verdict:echo"}
            avec = "echo"
        with open(LOG, "a") as f:
            f.write("POST %s idem=%s\n" % (avec, self.headers.get("Idempotency-Key", "")[:12]))
        reponse = json.dumps({"model": "flow01-demo",
                              "choices": [{"message": {"content": json.dumps(verdict)}}],
                              "usage": {"total_tokens": 421}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(reponse)))
        self.end_headers()
        self.wfile.write(reponse)
    def log_message(self, fmt, *args):
        pass

if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8094
    HTTPServer(("127.0.0.1", port), H).serve_forever()

#!/usr/bin/env python3
# Factice EXECUTANT (format openai) - recette M-COORD-FLOW-01 (issue #76).
# ecoute 127.0.0.1 EXPLICITE, aucune cle, 0 EUR. Reponses deterministes :
# MANDAT LENT FLOW01 -> reponse LENTE (4 s, cible du SIGKILL pendant l'appel,
# scenario I) ; MANDAT V2 -> livrable r2 corrige ; sinon livrable r1.
# Processus coord SEPARE du coordinateur (jamais tue avec lui, scenario P).
import json, sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = "/tmp/factice-exec.log"
if len(sys.argv) > 2:
    LOG = sys.argv[2]

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        corps = self.rfile.read(n).decode("utf-8", "replace")
        try:
            m = json.loads(corps)
            contenu = m["messages"][-1]["content"]
        except Exception:
            contenu = corps
        lent = "MANDAT LENT FLOW01" in contenu
        if lent:
            time.sleep(4)  # fenetre d'appel reseau : SIGKILL du coordinateur
        if "MANDAT V2" in contenu:
            out = ("LIVRABLE FLOW-01 v2 CORRIGE\nligne unique corrigee selon le point 1")
        elif "MANDAT" in contenu:
            out = ("LIVRABLE FLOW-01 v1\nligne unique erronee")
        else:
            out = contenu
        with open(LOG, "a") as f:
            f.write("POST lent=%s -> %s\n" % (lent, "v2" if "MANDAT V2" in contenu else ("v1" if "MANDAT" in contenu else "echo")))
        reponse = json.dumps({"model": "flow01-demo",
                              "choices": [{"message": {"content": out}}],
                              "usage": {"total_tokens": 421}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(reponse)))
        self.end_headers()
        self.wfile.write(reponse)
    def log_message(self, fmt, *args):
        pass

if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8093
    HTTPServer(("127.0.0.1", port), H).serve_forever()

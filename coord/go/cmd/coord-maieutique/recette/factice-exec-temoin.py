#!/usr/bin/env python3
# Factice EXECUTANT de la mission fille MAIEUTIQUE-TEMOIN-01 (format openai)
# - recette M-COORD-MAIEUTIQUE-01 (issue #42, plan v2 §5).
# Ecoute 127.0.0.1 EXPLICITE, aucune cle, 0 EUR, processus coord SEPARE.
# Deterministe : la charge Executer (mandat = ordre fige) reçoit un livrable
# FIXE témoin ; toute autre charge est repondue en écho (jamais utilisée
# pour une décision). Repercute m["model"] (identité répondante déclarée).
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8293
LOG = sys.argv[2] if len(sys.argv) > 2 else "/tmp/factice-exec-temoin.log"

LIVRABLE = ("LIVRABLE TEMOIN MAIEUTIQUE-01\n"
            "Ordre de mission formulé par le dialogue maïeutique de COORD,\n"
            "qualifié, validé par le pilote, publié puis transmis : le cycle\n"
            "complet est démontré sans aucun paiement réel (paiement_reel:false).")

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        corps = self.rfile.read(n).decode("utf-8", "replace")
        try:
            m = json.loads(corps)
            contenu = m["messages"][-1]["content"]
        except Exception:
            m, contenu = {}, corps
        ordre = '"objectif"' in contenu
        out = LIVRABLE if ordre else ("ECHO " + contenu[:80])
        with open(LOG, "a") as f:
            f.write("POST ordre=%s idem=%s\n"
                    % (ordre, self.headers.get("Idempotency-Key", "")[:16]))
        reponse = json.dumps({"model": m.get("model", "modele-openai-exec"),
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
    HTTPServer(("127.0.0.1", PORT), H).serve_forever()

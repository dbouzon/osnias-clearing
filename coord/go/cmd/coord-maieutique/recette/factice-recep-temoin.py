#!/usr/bin/env python3
# Factice RECEPTIONNAIRE de la mission fille MAIEUTIQUE-TEMOIN-01 (format
# openai) - recette M-COORD-MAIEUTIQUE-01 (issue #42, plan v2 §5).
# Ecoute 127.0.0.1 EXPLICITE, aucune cle, 0 EUR, processus coord SEPARE.
# Deterministe SANS etat : charge RECEPTION INDEPENDANTE -> verdict ACCEPTEE
# ancre a l'empreinte extraite de la charge (fail closed côté moteur sinon).
# Repercute m["model"].
import json, re, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8294
LOG = sys.argv[2] if len(sys.argv) > 2 else "/tmp/factice-recep-temoin.log"
VERDICT_ID = sys.argv[3] if len(sys.argv) > 3 else "maieutique-temoin-verdict-1"

EMP = re.compile(r'empreinte_attendue[^0-9a-f]*([0-9a-f]{64})')

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        corps = self.rfile.read(n).decode("utf-8", "replace")
        try:
            m = json.loads(corps)
            contenu = m["messages"][-1]["content"]
        except Exception:
            m, contenu = {}, corps
        emp = ""
        mm = EMP.search(contenu)
        if mm:
            emp = mm.group(1)
        if "RECEPTION INDEPENDANTE" in contenu:
            verdict = {"verdict": "ACCEPTEE",
                       "points": ["recette : livrable témoin conforme aux critères annoncés"],
                       "empreinte_attendue": emp,
                       "verdict_id": VERDICT_ID,
                       "verdict_url": "urn:coord:maieutique:verdict:" + VERDICT_ID}
            avec = "reception-ACCEPTEE"
        else:
            verdict = {"verdict": "ACCEPTEE", "points": [],
                       "empreinte_attendue": emp,
                       "verdict_id": VERDICT_ID + "-echo",
                       "verdict_url": "urn:coord:maieutique:verdict:" + VERDICT_ID + "-echo"}
            avec = "echo"
        with open(LOG, "a") as f:
            f.write("POST %s idem=%s\n" % (avec, self.headers.get("Idempotency-Key", "")[:16]))
        reponse = json.dumps({"model": m.get("model", "modele-openai-recep"),
                              "choices": [{"message": {"content": json.dumps(verdict, ensure_ascii=False)}}],
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

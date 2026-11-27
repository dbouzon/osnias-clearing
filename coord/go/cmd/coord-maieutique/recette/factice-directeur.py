#!/usr/bin/env python3
# Factice DIRECTEUR maieutique - recette M-COORD-MAIEUTIQUE-01 (issue #42).
# Ecoute 127.0.0.1 EXPLICITE, aucune cle, 0 EUR, processus coord SEPARE.
# Sert une FILE de reponses JSON (programme de recette) depuis un fichier :
#   chaque element = la reponse EXACTE du directeur (JSON strict, D4) :
#   {"action":"QUESTION","contenu":"..."}
#   {"action":"CLARIFICATION","contenu":"..."}
#   {"action":"PROPOSITION_ORDRE","ordre":{...dix champs...}}
#   {"action":"VERDICT","verdict":"...","points_detail":[{"severite":"...","texte":"..."}]}
# Repercute m["model"] (identite déclarée = identité répondante, sinon
# MODELE_DIVERGENT côté moteur, fail closed).
# Fenetre SIGKILL : si la charge du tour contient "SIGKILL_FENETRE", le
# serveur DORT 4 s AVANT de consommer la file : un coordinateur tué pendant
# ce sommeil n'a consommé AUCUNE réponse (preuve de reprise propre).
# File epuisee -> HTTP 500 (compté INDISPONIBILITE_DIRECTEUR côté moteur).
import json, sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8292
FICHIER = sys.argv[2] if len(sys.argv) > 2 else "reponses-directeur.json"
LOG = sys.argv[3] if len(sys.argv) > 3 else "/tmp/factice-directeur.log"

with open(FICHIER, encoding="utf-8") as f:
    FILE = json.load(f)
CUR = 0

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        global CUR
        n = int(self.headers.get("Content-Length", 0))
        corps = self.rfile.read(n).decode("utf-8", "replace")
        try:
            m = json.loads(corps)
            contenu = m["messages"][-1]["content"]
        except Exception:
            m, contenu = {}, corps
        fenetre = "SIGKILL_FENETRE" in contenu
        if fenetre:
            # Fenêtre SIGKILL : réponse DÉDIÉE, la FILE n'est PAS consommée
            # (le coordinateur tué pendant ce sommeil ne consomme rien ;
            # la reprise doit retrouver le programme de recette intact).
            time.sleep(4)
            with open(LOG, "a") as f:
                f.write("POST fenetre_sigkill idem=%s (file non consommée)\n"
                        % self.headers.get("Idempotency-Key", "")[:16])
            rep = {"action": "CLARIFICATION", "contenu": "fenêtre SIGKILL (réponse jamais consommée)"}
        elif CUR >= len(FILE):
            with open(LOG, "a") as f:
                f.write("POST file_epuisee idem=%s\n"
                        % self.headers.get("Idempotency-Key", "")[:16])
            reponse = json.dumps({"error": {"message": "FILE_EPUISEE"}}).encode()
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(reponse)))
            self.end_headers()
            self.wfile.write(reponse)
            return
        else:
            rep = FILE[CUR]
            CUR += 1
            action = rep.get("action", "?") if isinstance(rep, dict) else "?"
            with open(LOG, "a") as f:
                f.write("POST appel=%d action=%s idem=%s lent=%s\n"
                        % (CUR, action, self.headers.get("Idempotency-Key", "")[:16], fenetre))
        reponse = json.dumps({"model": m.get("model", "modele-directeur"),
                              "choices": [{"message": {"content": json.dumps(rep, ensure_ascii=False)}}],
                              "usage": {"total_tokens": 500}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(reponse)))
        self.end_headers()
        self.wfile.write(reponse)

    def log_message(self, fmt, *args):
        pass

if __name__ == "__main__":
    HTTPServer(("127.0.0.1", PORT), H).serve_forever()

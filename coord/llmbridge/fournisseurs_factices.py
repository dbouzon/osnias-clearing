#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Fournisseurs HTTP FACTICES loopback pour la qualification CI du pont.

Quatre formats NATIFS, pour exercer le routage réel de litellm-core :

- openai    : POST {base}/chat/completions (format OpenAI)
- mistral   : POST {base}/chat/completions (format Mistral, compatible OpenAI)
- anthropic : POST {base}/v1/messages (Messages API, x-api-key)
- gemini    : POST {base}/models/{model}:generateContent (format Google)

Scénarios pilotables par serveur (nominal, auth401, err429, err500,
delai, troncature, modele_divergent, usage_absent, json_malforme) : la
réponse imite le comportement du fournisseur, le pont et le SDK réagissent
réellement. 0 €, loopback uniquement, aucune clé vraie.
"""

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCENARIOS = ("nominal", "auth401", "err429", "err500", "delai",
             "troncature", "modele_divergent", "usage_absent", "json_malforme")


class ServeurFactice:
    """Un serveur factice par fournisseur, sur port éphémère loopback."""

    def __init__(self, format_four, modele="factice-modele", delai_scenario_s=5.0):
        if format_four not in ("openai", "mistral", "anthropic", "gemini"):
            raise ValueError("format inconnu : " + format_four)
        self.format = format_four
        self.modele = modele
        self.delai_scenario_s = delai_scenario_s
        self.scenario = "nominal"
        self.requetes = []          # journal {chemin, autorisation, corps}
        self._serveur = None
        self._fil = None
        self.lock = threading.Lock()

    def demarrer(self):
        serveur = self
        modele = self.modele

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, fmt, *args):
                pass  # jamais de charge dans les journaux

            def _json(self, statut, objet):
                corps = json.dumps(objet).encode("utf-8")
                self.send_response(statut)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(corps)))
                self.end_headers()
                self.wfile.write(corps)

            def do_POST(self):
                try:
                    longueur = int(self.headers.get("Content-Length", 0))
                    corps = self.rfile.read(longueur) if longueur else b""
                    with serveur.lock:
                        serveur.requetes.append({
                            "chemin": self.path,
                            "autorisation": self.headers.get("Authorization", ""),
                            "cle_api": self.headers.get("x-api-key", ""),
                        })
                        scenario = serveur.scenario
                    if scenario == "json_malforme":
                        corps_rep = b"{pas du json"
                        self.send_response(200)
                        self.send_header("Content-Type", "application/json")
                        self.send_header("Content-Length", str(len(corps_rep)))
                        self.end_headers()
                        self.wfile.write(corps_rep)
                        return
                    if scenario == "auth401":
                        self._json(401, {"error": {"message": "clé invalide"}})
                        return
                    if scenario == "err429":
                        self._json(429, {"error": {"message": "quota dépassé"}})
                        return
                    if scenario == "err500":
                        self._json(500, {"error": {"message": "erreur interne"}})
                        return
                    if scenario == "delai":
                        time.sleep(serveur.delai_scenario_s)
                    try:
                        req = json.loads(corps.decode("utf-8"))
                    except Exception:
                        req = {}
                    if serveur.format in ("openai", "mistral"):
                        self._repondre_openai(req, scenario, serveur.modele)
                    elif serveur.format == "anthropic":
                        self._repondre_anthropic(req, scenario, serveur.modele)
                    else:
                        self._repondre_gemini(req, scenario, serveur.modele)
                except (BrokenPipeError, ConnectionResetError):
                    pass  # client parti (délai dépassé) : rien à émettre

            def _repondre_openai(self, req, scenario, modele_defaut):
                # Le fournisseur réel ÉCHO le modèle demandé (litellm retire
                # le préfixe fournisseur avant l'appel).
                modele = req.get("model") or modele_defaut
                contenu = "RÉPONSE FACTICE du fournisseur (loopback, 0 EUR)."
                fin = "stop"
                if scenario == "troncature":
                    fin = "length"
                modele_repondu = modele
                if scenario == "modele_divergent":
                    modele_repondu = modele + "-divergent"
                rep = {
                    "id": "chatcmpl-factice",
                    "object": "chat.completion",
                    "created": 0,
                    "model": modele_repondu,
                    "choices": [{
                        "index": 0,
                        "message": {"role": "assistant", "content": contenu},
                        "finish_reason": fin,
                    }],
                    "usage": {"prompt_tokens": 120, "completion_tokens": 30,
                              "total_tokens": 150},
                }
                if scenario == "usage_absent":
                    del rep["usage"]
                self._json(200, rep)

            def _repondre_anthropic(self, req, scenario, modele_defaut):
                modele = req.get("model") or modele_defaut
                modele_repondu = modele
                motif = "end_turn"
                if scenario == "modele_divergent":
                    modele_repondu = modele + "-divergent"
                if scenario == "troncature":
                    motif = "max_tokens"
                rep = {
                    "id": "msg_factice",
                    "type": "message",
                    "role": "assistant",
                    "model": modele_repondu,
                    "content": [{"type": "text",
                                 "text": "RÉPONSE FACTICE anthropic (loopback)."}],
                    "stop_reason": motif,
                    "usage": {"input_tokens": 120, "output_tokens": 30},
                }
                if scenario == "usage_absent":
                    del rep["usage"]
                self._json(200, rep)

            def _repondre_gemini(self, req, scenario, modele_defaut):
                # Le modèle demandé est dans le chemin : models/{model}:generateContent
                modele = modele_defaut
                if "/models/" in self.path:
                    morceau = self.path.split("/models/", 1)[1]
                    modele = morceau.split(":", 1)[0] or modele_defaut
                modele_repondu = modele
                motif = "STOP"
                if scenario == "modele_divergent":
                    modele_repondu = modele + "-divergent"
                if scenario == "troncature":
                    motif = "MAX_TOKENS"
                rep = {
                    "candidates": [{
                        "content": {"parts": [{"text": "RÉPONSE FACTICE gemini "
                                                    "(loopback)."}],
                                    "role": "model"},
                        "finishReason": motif,
                        "index": 0,
                    }],
                    "usageMetadata": {"promptTokenCount": 120,
                                      "candidatesTokenCount": 30,
                                      "totalTokenCount": 150},
                    "modelVersion": modele_repondu,
                }
                if scenario == "usage_absent":
                    del rep["usageMetadata"]
                self._json(200, rep)

        self._serveur = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self._serveur.daemon_threads = True
        self._fil = threading.Thread(target=self._serveur.serve_forever,
                                     daemon=True)
        self._fil.start()
        hote, port = self._serveur.server_address[:2]
        self.adresse = "http://127.0.0.1:" + str(port)
        return self.adresse

    @property
    def api_base(self):
        """api_base à déclarer au pont (sans suffixe de version)."""
        if self.format in ("openai", "mistral"):
            return self.adresse + "/v1"
        return self.adresse

    def arreter(self):
        if self._serveur:
            self._serveur.shutdown()
            self._serveur.server_close()
            self._fil.join(timeout=5)

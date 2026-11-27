#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Pont COORD HTTP/JSON — contrat coord.llm.v1 (M-COORD-LLMBRIDGE-01, #62).

Serveur HTTP/1.1 loopback (stdlib UNIQUEMENT côté pont), JSON seul :

    GET  /ping                 -> {"statut":"ok","contrat":"coord.llm.v1",...}
    GET  /v1/models            -> destinations autorisées
    POST /v1/chat/completions  -> inférence (compatible OpenAI)

Sécurité (fail closed) :
- écoute 127.0.0.1 UNIQUEMENT ; jeton Bearer obligatoire (comparaison à temps
  constant) ; secrets par variables d'environnement, JAMAIS journalisés ;
- liste FERMÉE de destinations autorisées (préfixe de modèle -> api_base) :
  tout modèle hors liste est refusé (DESTINATION_REFUSEE), aucun routage
  arbitraire ;
- corps borné, type de contenu vérifié, délais bornés (lecture, backend) ;
- AUCUN retry, AUCUN fallback, AUCUN streaming : le pont est un transport
  SANS ÉTAT ; la voûte monétaire, l'anti-doublon, l'annulation et la reprise
  appartiennent au moteur Go.

Le pont est redémarrable sans perte : aucune donnée persistée ici.
"""

import argparse
import hmac
import json
import os
import queue
import socket
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from backend import (ErreurBackend, FacticeBackend, LiteLLMBackend,  # noqa: E402
                     expurger)

CONTRAT = "coord.llm.v1"
CORPS_MAX_DEFAUT = 2 * 1024 * 1024   # 2 Mo
DELAI_LECTURE_S = 30
DELAI_BACKEND_DEFAUT = 60

_TYPES_ERREUR = {
    "JSON_INVALIDE": 400, "REQUETE_INVALIDE": 400, "DESTINATION_REFUSEE": 400,
    "MARQUEURS_CONTRADICTOIRES": 400,
    "JETON_INVALIDE": 401, "ROUTE_INCONNUE": 404,
    "METHODE_NON_AUTORISEE": 405, "CORPS_TROP_GRAND": 413,
    "TYPE_INVALIDE": 415, "SDK_ABSENT": 503, "CLE_ABSENTE": 503,
    "REPONSE_BACKEND_INVALIDE": 502, "ERREUR_SDK": 502, "ERREUR_BACKEND": 502,
    "DELAi_DEPASSE": 504, "DELAI_DEPASSE": 504, "INTERNE": 500,
}


def statut_depuis_type(type_err, statut_defaut=400):
    return _TYPES_ERREUR.get(type_err, statut_defaut)


def charger_config(chemin):
    """Charge et valide la configuration (fail closed, aucun secret dedans).

    Rejet explicite : version inconnue, destinations vides, entrée sans
    api_base/cle_env, champ de type « cle »/« token » (un secret ne vit QUE
    dans l'environnement).
    """
    with open(chemin, "r", encoding="utf-8") as f:
        cfg = json.load(f)
    if not isinstance(cfg, dict):
        raise ValueError("config : objet JSON attendu")
    if cfg.get("version") != CONTRAT:
        raise ValueError("config : version attendue " + CONTRAT)
    dests = cfg.get("destinations")
    if not isinstance(dests, dict) or not dests:
        raise ValueError("config : destinations (liste fermée) requises")
    for prefixe, d in dests.items():
        if not isinstance(d, dict):
            raise ValueError("config : destination " + str(prefixe) + " invalide")
        base = d.get("api_base", "")
        if not (str(base).startswith("http://127.0.0.1") or
                str(base).startswith("http://localhost") or
                str(base).startswith("https://")):
            raise ValueError("config : api_base de " + str(prefixe) + " invalide")
        if not d.get("cle_env"):
            raise ValueError("config : cle_env requise pour " + str(prefixe))
        for interdit in ("cle", "api_key", "token", "jeton"):
            if interdit in d:
                raise ValueError("config : secret interdit dans le fichier ("
                                 + interdit + " pour " + str(prefixe) + ")")
    bornes = cfg.get("bornes", {})
    return {
        "version": CONTRAT,
        "destinations": dests,
        "bornes": {
            "corps_max_octets": int(bornes.get("corps_max_octets",
                                               CORPS_MAX_DEFAUT)),
            "delai_backend_s": float(bornes.get("delai_backend_s",
                                                DELAI_BACKEND_DEFAUT)),
        },
    }


def resoudre_destination(model, destinations):
    """Préfixe le plus long matché ; modèle hors liste => None (refus)."""
    meilleur = None
    for prefixe in destinations:
        if model.startswith(prefixe) and (meilleur is None or
                                          len(prefixe) > len(meilleur)):
            meilleur = prefixe
    return meilleur


def valider_requete(corps):
    """Validation stricte du contrat coord.llm.v1 (jamais de devinette)."""
    try:
        req = json.loads(corps.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        raise ErreurBackend("JSON_INVALIDE", "corps JSON invalide", 400)
    if not isinstance(req, dict):
        raise ErreurBackend("REQUETE_INVALIDE", "objet JSON attendu", 400)
    model = req.get("model")
    if not isinstance(model, str) or not model:
        raise ErreurBackend("REQUETE_INVALIDE", "model requis (chaîne)", 400)
    messages = req.get("messages")
    if not isinstance(messages, list) or not messages:
        raise ErreurBackend("REQUETE_INVALIDE", "messages requis (liste)", 400)
    for m in messages:
        if (not isinstance(m, dict) or not isinstance(m.get("role"), str)
                or not isinstance(m.get("content"), str)):
            raise ErreurBackend("REQUETE_INVALIDE",
                                "message {role, content} requis", 400)
    max_tokens = req.get("max_tokens")
    if max_tokens is not None and (not isinstance(max_tokens, int)
                                    or max_tokens < 1):
        raise ErreurBackend("REQUETE_INVALIDE", "max_tokens entier >= 1", 400)
    return model, messages, max_tokens


def valider_reponse_backend(rep, model_demande):
    """Contrôle de surface de la réponse du backend (fond = côté Go)."""
    if not isinstance(rep, dict):
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "réponse non objet", 502)
    for champ in ("model", "choices", "usage"):
        if champ not in rep:
            raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                                "champ " + champ + " absent", 502)
    if not rep["model"] or not isinstance(rep["model"], str):
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "model absent", 502)
    choix = rep["choices"]
    if not isinstance(choix, list) or not choix:
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "choices vide", 502)
    premier = choix[0]
    if not isinstance(premier, dict) or not isinstance(premier.get("message"), dict):
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "message absent", 502)
    if not premier["message"].get("content") and \
            premier["message"].get("content") != "":
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "content absent", 502)
    if not premier.get("finish_reason"):
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                            "finish_reason absent", 502)
    usage = rep["usage"]
    if not isinstance(usage, dict) or not isinstance(usage.get("total_tokens"), int):
        raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                            "usage.total_tokens absent", 502)
    return rep


class Pont:
    """Cœur du pont, séparable du transport HTTP (testable directement)."""

    def __init__(self, backend, destinations, bornes=None, jeton="",
                 journal=""):
        self.backend = backend
        self.destinations = destinations or {}
        b = bornes or {}
        self.corps_max = int(b.get("corps_max_octets", CORPS_MAX_DEFAUT))
        self.delai_backend = float(b.get("delai_backend_s", DELAI_BACKEND_DEFAUT))
        self.jeton = jeton
        # Journal additif optionnel (M-COORD-DEP-03) : une ligne JSONL par
        # requête d'inférence — JAMAIS de charge, JAMAIS de jeton (corrélation
        # des traces moteur ⇄ pont). Échec d'écriture : stderr, jamais un
        # faux silence, jamais une erreur client.
        self.journal = journal or ""
        self._verrou_journal = threading.Lock()

    def _ecrire_journal(self, entree):
        if not self.journal:
            return
        entree = dict(entree)
        entree["model"] = expurger(str(entree.get("model", "")))
        ligne = json.dumps(entree, ensure_ascii=False)
        try:
            with self._verrou_journal:
                with open(self.journal, "a", encoding="utf-8") as f:
                    f.write(ligne + "\n")
        except Exception as e:
            sys.stderr.write("[llmbridge] journal indisponible : " +
                              expurger(str(e)) + "\n")

    def authentifier(self, entete):
        if not self.jeton:
            return False  # jeton non configuré = tout refusé (fail closed)
        if not entete or not entete.startswith("Bearer "):
            return False
        return hmac.compare_digest(entete[len("Bearer "):], self.jeton)

    def resoudre(self, model):
        prefixe = resoudre_destination(model, self.destinations)
        if prefixe is None:
            raise ErreurBackend(
                "DESTINATION_REFUSEE",
                "model hors liste fermée des destinations : " + model, 400)
        return self.destinations[prefixe]

    def inferer(self, model, messages, max_tokens):
        destination = self.resoudre(model)
        prefixe = resoudre_destination(model, self.destinations) or ""
        debut = time.time()
        # Délai backend : exécution bornée dans un fil dédié (un fil Python
        # n'est pas tuable ; le dépassement est RÉPONDU, le processus peut
        # être redémarré sans perte : le pont est sans état).
        resultat = queue.Queue(maxsize=1)

        def _appel():
            try:
                resultat.put(("ok", self.backend.completion(
                    model, messages, max_tokens,
                    contexte={"destination": destination})))
            except ErreurBackend as e:
                resultat.put(("err", e))
            except Exception as e:  # erreur interne : jamais un faux succès
                resultat.put(("err", ErreurBackend("ERREUR_BACKEND",
                                                   expurger(str(e)), 502)))
        fil = threading.Thread(target=_appel, daemon=True)
        fil.start()
        fil.join(self.delai_backend)
        if fil.is_alive():
            raise ErreurBackend("DELAI_DEPASSE",
                               "backend au-delà du délai " +
                               str(self.delai_backend) + " s", 504)
        _, valeur = resultat.get_nowait()
        if isinstance(valeur, ErreurBackend):
            self._ecrire_journal({
                "horodatage": time.strftime(
                    "%Y-%m-%dT%H:%M:%S", time.gmtime(debut)) + "Z",
                "model": model,
                "destination": prefixe,
                "statut": "erreur",
                "type_erreur": valeur.type,
                "duree_ms": int((time.time() - debut) * 1000),
            })
            raise valeur
        reponse = valider_reponse_backend(valeur, model)
        self._ecrire_journal({
            "horodatage": time.strftime(
                "%Y-%m-%dT%H:%M:%S", time.gmtime(debut)) + "Z",
            "model": model,
            "destination": prefixe,
            "statut": "ok",
            "duree_ms": int((time.time() - debut) * 1000),
        })
        return reponse


def fabrique_handler(pont, mode_backend):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, fmt, *args):  # journal minimal, sans charge
            sys.stderr.write("[llmbridge] " + (fmt % args) + "\n")

        def _repondre_json(self, statut, objet):
            corps = json.dumps(objet, ensure_ascii=False).encode("utf-8")
            self.send_response(statut)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(corps)))
            self.end_headers()
            self.wfile.write(corps)

        def _erreur(self, type_err, message):
            statut = statut_depuis_type(type_err)
            self._repondre_json(statut, {"error": {"type": type_err,
                                                   "message": expurger(message)}})

        def _lire_corps(self):
            longueur = self.headers.get("Content-Length")
            try:
                n = int(longueur) if longueur else 0
            except ValueError:
                raise ErreurBackend("REQUETE_INVALIDE", "Content-Length invalide", 400)
            if n > pont.corps_max:
                raise ErreurBackend("CORPS_TROP_GRAND",
                                   "corps > " + str(pont.corps_max) + " octets", 413)
            if n == 0:
                raise ErreurBackend("JSON_INVALIDE", "corps vide", 400)
            return self.rfile.read(n)

        def do_GET(self):
            if not pont.authentifier(self.headers.get("Authorization")):
                self._erreur("JETON_INVALIDE", "jeton Bearer requis (fail closed)")
                return
            if self.path == "/ping":
                self._repondre_json(200, {
                    "statut": "ok", "contrat": CONTRAT,
                    "backend": mode_backend})
                return
            if self.path == "/v1/models":
                self._repondre_json(200, {
                    "object": "list",
                    "data": [{"id": p, "object": "model",
                              "owned_by": "coord-llmbridge"} for p in
                             sorted(pont.destinations)]})
                return
            self._erreur("ROUTE_INCONNUE", "route inconnue : " + self.path)

        def do_POST(self):
            if not pont.authentifier(self.headers.get("Authorization")):
                self._erreur("JETON_INVALIDE", "jeton Bearer requis (fail closed)")
                return
            if self.path != "/v1/chat/completions":
                self._erreur("ROUTE_INCONNUE", "route inconnue : " + self.path)
                return
            ctype = self.headers.get("Content-Type", "")
            if ctype and "application/json" not in ctype:
                self._erreur("TYPE_INVALIDE", "Content-Type application/json requis")
                return
            try:
                corps = self._lire_corps()
                model, messages, max_tokens = valider_requete(corps)
                reponse = pont.inferer(model, messages, max_tokens)
            except ErreurBackend as e:
                self._erreur(e.type, e.message)
                return
            except Exception as e:
                self._erreur("INTERNE", str(e))
                return
            self._repondre_json(200, reponse)

        def do_DELETE(self):
            self._erreur("METHODE_NON_AUTORISEE", "méthode refusée")

    return Handler


def main():
    parseur = argparse.ArgumentParser(description="Pont COORD coord.llm.v1")
    parseur.add_argument("--config", default="", help="fichier de configuration JSON")
    parseur.add_argument("--factice", action="store_true",
                         help="backend factice déterministe (qualification locale, 0 €)")
    parseur.add_argument("--adresse", default="127.0.0.1")
    parseur.add_argument("--port", type=int, default=8788,
                         help="port d'écoute (0 = éphémère)")
    parseur.add_argument("--jeton-env", default="COORD_BRIDGE_JETON",
                         help="variable d'environnement contenant le jeton (OBLIGATOIRE)")
    parseur.add_argument("--pret", action="store_true", default=True,
                         help="imprime BRIDGE_PRET <adresse:port> à l'écoute")
    args = parseur.parse_args()

    destinations = {}
    bornes = {}
    if args.config:
        cfg = charger_config(args.config)
        destinations = cfg["destinations"]
        bornes = cfg["bornes"]

    if args.factice:
        destinations = {"factice/exec/": {"api_base": "factice://local",
                                          "cle_env": "COORD_BRIDGE_CLE_FACTICE"},
                        "factice/recep/": {"api_base": "factice://local",
                                           "cle_env": "COORD_BRIDGE_CLE_FACTICE"}}
        backend = FacticeBackend()
        mode_backend = "factice"
    else:
        if not destinations:
            print("NON_EXECUTE : --config requis (destinations fermées) hors "
                  "mode factice", file=sys.stderr)
            return 3
        backend = LiteLLMBackend(delai_s=bornes.get("delai_backend_s",
                                                    DELAI_BACKEND_DEFAUT))
        mode_backend = "litellm-core"

    jeton = os.environ.get(args.jeton_env, "").strip()
    if not jeton or len(jeton) < 8:
        print("NON_EXECUTE : jeton absent ou trop court (variable " +
              args.jeton_env + ", fail closed)", file=sys.stderr)
        return 3

    journal = os.environ.get("COORD_BRIDGE_JOURNAL", "").strip()
    pont = Pont(backend, destinations, bornes, jeton=jeton, journal=journal)
    serveur = ThreadingHTTPServer((args.adresse, args.port),
                                  fabrique_handler(pont, mode_backend))
    serveur.socket.settimeout(None)
    serveur.daemon_threads = True
    if args.pret:
        hote, port = serveur.server_address[:2]
        print("BRIDGE_PRET " + str(hote) + ":" + str(port), flush=True)
    try:
        serveur.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        serveur.server_close()
    return 0


if __name__ == "__main__":
    sys.exit(main())

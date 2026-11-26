#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Qualification LOCALE du contrat coord.llm.v1 (sans SDK, backend factice).

Exécutables partout (stdlib uniquement). La couche litellm-core réelle est
qualifiée par test_ci_sdk.py dans la CI (installation du SDK permise là-bas).
Ici : REEL-sur-factice — le serveur HTTP, l'authentification, les bornes, le
routage fermé et les erreurs structurées sont réellement exercés.
"""

import json
import os
import sys
import threading
import time
import unittest
import urllib.request
from http.server import ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bridge  # noqa: E402
from backend import ErreurBackend, FacticeBackend, expurger  # noqa: E402

JETON = "jeton-test-0123456789abcdef"


class BackendScenario:
    """Backend pilotable : nom(inal) / lent / erreur / usage_absent."""

    def __init__(self, mode="nominal", delai=1.0):
        self.mode = mode
        self.delai = delai

    def completion(self, model, messages, max_tokens=None, contexte=None):
        if self.mode == "lent":
            time.sleep(self.delai)
            return FacticeBackend().completion(model, messages)
        if self.mode == "erreur":
            raise ErreurBackend(
                "ERREUR_BACKEND",
                "panneau backend : clé sk-ABCDEF0123456789 vue", 502)
        rep = FacticeBackend().completion(model, messages)
        if self.mode == "usage_absent":
            rep["usage"] = {}
        return rep


class Harnais:
    """Serveur HTTP réel du pont, port éphémère, fil dédié."""

    def __init__(self, backend, destinations=None, bornes=None):
        destinations = destinations or {
            "factice/exec/": {"api_base": "factice://local", "cle_env": "X"},
            "factice/recep/": {"api_base": "factice://local", "cle_env": "X"},
        }
        self.pont = bridge.Pont(backend, destinations, bornes, jeton=JETON)
        handler = bridge.fabrique_handler(self.pont, "factice")
        self.serveur = ThreadingHTTPServer(("127.0.0.1", 0), handler)
        self.serveur.daemon_threads = True
        self.fil = threading.Thread(target=self.serveur.serve_forever,
                                    daemon=True)
        self.fil.start()
        self.base = "http://127.0.0.1:" + str(self.serveur.server_address[1])

    def arreter(self):
        self.serveur.shutdown()
        self.serveur.server_close()
        self.fil.join(timeout=5)

    def appel(self, chemin, methode="GET", corps=None, jeton=JETON,
              ctype="application/json"):
        if corps is not None and methode == "GET":
            methode = "POST"
        req = urllib.request.Request(self.base + chemin, method=methode)
        if jeton is not None:
            req.add_header("Authorization", "Bearer " + jeton)
        if corps is not None:
            req.add_header("Content-Type", ctype)
            donnees = corps if isinstance(corps, bytes) else json.dumps(
                corps).encode("utf-8")
        else:
            donnees = None
        try:
            with urllib.request.urlopen(req, donnees, timeout=10) as rep:
                return rep.status, json.loads(rep.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            return e.code, json.loads(e.read().decode("utf-8"))


class TestContrat(unittest.TestCase):

    def setUp(self):
        self.h = Harnais(FacticeBackend())
        self.addCleanup(self.h.arreter)

    def test_ping(self):
        statut, corps = self.h.appel("/ping")
        self.assertEqual(statut, 200)
        self.assertEqual(corps["statut"], "ok")
        self.assertEqual(corps["contrat"], "coord.llm.v1")

    def test_auth_refusée(self):
        for jeton in (None, "mauvais-jeton-000000"):
            statut, corps = self.h.appel("/ping", jeton=jeton)
            self.assertEqual(statut, 401)
            self.assertEqual(corps["error"]["type"], "JETON_INVALIDE")

    def test_modeles(self):
        statut, corps = self.h.appel("/v1/models")
        self.assertEqual(statut, 200)
        ids = [m["id"] for m in corps["data"]]
        self.assertIn("factice/exec/", ids)

    def test_completion_reception(self):
        charge = ("verdict au gabarit {\"verdict\":\"...\","
                  "\"empreinte_attendue\":\"" + "a" * 64 + "\"} livrable reçu")
        statut, corps = self.h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": charge}]})
        self.assertEqual(statut, 200)
        self.assertEqual(corps["object"], "chat.completion")
        self.assertEqual(corps["model"], "factice/recep/demo")
        self.assertEqual(corps["choices"][0]["finish_reason"], "stop")
        self.assertEqual(corps["usage"]["total_tokens"], 150)
        verdict = json.loads(corps["choices"][0]["message"]["content"])
        self.assertEqual(verdict["verdict"], "ACCEPTEE")
        self.assertEqual(verdict["empreinte_attendue"], "a" * 64)
        self.assertEqual(verdict["verdict_id"], "factice-verdict")

    def test_completion_critique_plan(self):
        charge = ('CRITIQUE DE PLAN : plan trivial. Répondre au gabarit avec '
                  '"empreinte_attendue":"' + "b" * 64 + '"')
        statut, corps = self.h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": charge}]})
        self.assertEqual(statut, 200)
        verdict = json.loads(corps["choices"][0]["message"]["content"])
        self.assertEqual(verdict["verdict"], "PLAN_ACCEPTE")
        self.assertEqual(verdict["empreinte_attendue"], "b" * 64)

    def test_execution(self):
        statut, corps = self.h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/exec/demo",
                   "messages": [{"role": "user", "content": "exécute le mandat"}],
                   "max_tokens": 500})
        self.assertEqual(statut, 200)
        self.assertTrue(corps["choices"][0]["message"]["content"]
                        .startswith("LIVRABLE 1"))

    def test_json_malforme(self):
        statut, corps = self.h.appel("/v1/chat/completions", corps=b"{pas json")
        self.assertEqual(statut, 400)
        self.assertEqual(corps["error"]["type"], "JSON_INVALIDE")

    def test_destination_refusée(self):
        statut, corps = self.h.appel(
            "/v1/chat/completions",
            corps={"model": "openai/gpt-4o-mini",
                   "messages": [{"role": "user", "content": "x"}]})
        self.assertEqual(statut, 400)
        self.assertEqual(corps["error"]["type"], "DESTINATION_REFUSEE")

    def test_requêtes_invalides(self):
        requetes = (
            {"messages": [{"role": "user", "content": "x"}]},          # sans model
            {"model": "", "messages": [{"role": "user", "content": "x"}]},
            {"model": "factice/recep/demo", "messages": []},           # vide
            {"model": "factice/recep/demo", "messages": "texte"},      # pas liste
            {"model": "factice/recep/demo", "messages": [{"role": "u"}]},  # sans content
            {"model": "factice/recep/demo", "max_tokens": 0,
             "messages": [{"role": "user", "content": "x"}]},
        )
        for req in requetes:
            statut, corps = self.h.appel("/v1/chat/completions", corps=req)
            self.assertEqual(statut, 400, req)
            self.assertEqual(corps["error"]["type"], "REQUETE_INVALIDE", req)

    def test_corps_trop_grand(self):
        gros = json.dumps({
            "model": "factice/recep/demo",
            "messages": [{"role": "user", "content": "x" * 5000}]}).encode()
        h = Harnais(FacticeBackend(), bornes={"corps_max_octets": 1024})
        self.addCleanup(h.arreter)
        statut, corps = h.appel("/v1/chat/completions", corps=gros)
        self.assertEqual(statut, 413)
        self.assertEqual(corps["error"]["type"], "CORPS_TROP_GRAND")

    def test_type_invalide(self):
        statut, corps = self.h.appel(
            "/v1/chat/completions", ctype="text/plain",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": "x"}]})
        self.assertEqual(statut, 415)
        self.assertEqual(corps["error"]["type"], "TYPE_INVALIDE")

    def test_routes_et_methodes(self):
        self.assertEqual(self.h.appel("/inconnu")[1]["error"]["type"],
                         "ROUTE_INCONNUE")
        self.assertEqual(
            self.h.appel("/v1/chat/completions", methode="DELETE")[1]
            ["error"]["type"], "METHODE_NON_AUTORISEE")


class TestBornesEtErreursBackend(unittest.TestCase):

    def test_delai_depasse(self):
        h = Harnais(BackendScenario("lent", delai=1.5),
                    bornes={"delai_backend_s": 0.2})
        self.addCleanup(h.arreter)
        statut, corps = h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": "x"}]})
        self.assertEqual(statut, 504)
        self.assertEqual(corps["error"]["type"], "DELAI_DEPASSE")

    def test_erreur_backend_expurgée(self):
        h = Harnais(BackendScenario("erreur"))
        self.addCleanup(h.arreter)
        statut, corps = h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": "x"}]})
        self.assertEqual(statut, 502)
        self.assertEqual(corps["error"]["type"], "ERREUR_BACKEND")
        self.assertIn("[EXPURGE]", corps["error"]["message"])
        self.assertNotIn("sk-ABCDEF", corps["error"]["message"])

    def test_usage_absent_refusé(self):
        h = Harnais(BackendScenario("usage_absent"))
        self.addCleanup(h.arreter)
        statut, corps = h.appel(
            "/v1/chat/completions",
            corps={"model": "factice/recep/demo",
                   "messages": [{"role": "user", "content": "x"}]})
        self.assertEqual(statut, 502)
        self.assertEqual(corps["error"]["type"], "REPONSE_BACKEND_INVALIDE")


class TestConfiguration(unittest.TestCase):

    def _ecrire(self, cfg):
        chemin = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                              "_tmp_config_test.json")
        with open(chemin, "w", encoding="utf-8") as f:
            json.dump(cfg, f)
        self.addCleanup(os.remove, chemin)
        return chemin

    def test_version_inconnue_refusée(self):
        chemin = self._ecrire({"version": "coord.llm.v0",
                              "destinations": {"openai/": {
                                  "api_base": "https://x", "cle_env": "K"}}})
        with self.assertRaises(ValueError):
            bridge.charger_config(chemin)

    def test_secret_dans_config_refusé(self):
        chemin = self._ecrire({
            "version": "coord.llm.v1",
            "destinations": {"openai/": {"api_base": "https://x",
                                         "cle_env": "K", "cle": "sk-vrai"}}})
        with self.assertRaises(ValueError):
            bridge.charger_config(chemin)

    def test_destination_incomplète_refusée(self):
        chemin = self._ecrire({
            "version": "coord.llm.v1",
            "destinations": {"openai/": {"api_base": "https://x"}}})
        with self.assertRaises(ValueError):
            bridge.charger_config(chemin)

    def test_config_exemple_valide(self):
        chemin = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                              "config.exemple.json")
        cfg = bridge.charger_config(chemin)
        self.assertIn("openai/", cfg["destinations"])


class TestExpurgation(unittest.TestCase):

    def test_expurger(self):
        texte = "clé sk-ABCDEF0123456789 et AIza" + "A1" * 17 + "A" + " vues"
        resultat = expurger(texte)
        self.assertNotIn("sk-ABCDEF", resultat)
        self.assertNotIn("AIza", resultat)
        self.assertEqual(resultat.count("[EXPURGE]"), 2)


class TestCycleMissionFactice(unittest.TestCase):
    """Cycle complet simulé au niveau du pont (le cycle GO réel est qualifié
    par llmbridge_e2e_test.go côté moteur) : critique, exécution, réception."""

    def test_cycle(self):
        backend = FacticeBackend()
        modele_emp = "c" * 64
        critique = backend.completion(
            "factice/recep/demo",
            [{"role": "user",
              "content": "CRITIQUE DE PLAN ... empreinte_attendue\\\":\\\"" +
              modele_emp + "\\\""}])["choices"][0]["message"]["content"]
        self.assertEqual(json.loads(critique)["verdict"], "PLAN_ACCEPTE")
        execution = backend.completion(
            "factice/exec/demo",
            [{"role": "user", "content": "exécute"}])["choices"][0]["message"]["content"]
        reception = backend.completion(
            "factice/recep/demo",
            [{"role": "user", "content": execution + " empreinte_attendue\\\":\\\"" +
              "d" * 64 + "\\\""}])["choices"][0]["message"]["content"]
        self.assertEqual(json.loads(reception)["verdict"], "ACCEPTEE")


if __name__ == "__main__":
    unittest.main(verbosity=2)

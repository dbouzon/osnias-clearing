#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Qualification CI du pont avec litellm-core RÉEL (M-COORD-LLMBRIDGE-01, T4).

Prérequis : `pip install litellm-core==1.111.0` (interdit dans le sandbox de
l'exécutant, autorisé dans la CI GitHub Actions). Le SDK est RÉELLEMENT
importé et exécuté contre des fournisseurs FACTICES loopback natifs
(openai, mistral, anthropic, gemini) ; le pont route réellement.

Si litellm-core est absent, chaque test est marqué NON_EXECUTE (skip
explicite) : JAMAIS un faux SDK, jamais un succès simulé.
"""

import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

try:
    import litellm  # noqa: F401
    SDK_PRESENT = True
except Exception:
    SDK_PRESENT = False

RAISON_ABSENT = ("NON_EXECUTE : litellm-core absent (installation interdite "
                 "dans le sandbox de l'exécutant ; installer litellm-core=="
                 "1.111.0, exécution réelle assurée par la CI)")

if SDK_PRESENT:  # stabilization de litellm avant tout test
    try:
        litellm.telemetry = False
    except Exception:
        pass

from backend import ErreurBackend, LiteLLMBackend  # noqa: E402
from fournisseurs_factices import ServeurFactice  # noqa: E402

CLE_TEST = "cle-factice-ci-0123456789"

MODELES = {
    "openai": "openai/factice-openai",
    "mistral": "mistral/factice-mistral",
    "anthropic": "anthropic/factice-anthropic",
    "gemini": "gemini/factice-gemini",
}


@unittest.skipUnless(SDK_PRESENT, RAISON_ABSENT)
class TestSDKReel(unittest.TestCase):
    """LiteLLMBackend réel → pont → fournisseurs factices natifs loopback."""

    def setUp(self):
        os.environ["COORD_CLE_CI"] = CLE_TEST
        self.serveurs = {}
        for fmt in MODELES:
            s = ServeurFactice(fmt)
            s.demarrer()
            self.serveurs[fmt] = s
            self.addCleanup(s.arreter)
        destinations = {
            fmt + "/": {"api_base": s.api_base, "cle_env": "COORD_CLE_CI"}
            for fmt, s in self.serveurs.items()
        }
        self.backend = LiteLLMBackend(delai_s=15.0)
        self.contexte_dests = destinations

    def _completion(self, model, messages=None):
        # Même résolution que le pont (bridge.Pont.resoudre) : préfixe le plus
        # long, la destination unique est passée au backend.
        prefixe = None
        for p in self.contexte_dests:
            if model.startswith(p) and (prefixe is None or len(p) > len(prefixe)):
                prefixe = p
        if prefixe is None:
            raise ValueError("destination inconnue : " + model)
        return self.backend.completion(
            model, messages or [{"role": "user", "content": "ping factice"}],
            contexte={"destination": self.contexte_dests[prefixe]})

    def test_nominal_quatre_fournisseurs(self):
        for fmt, model in MODELES.items():
            with self.subTest(fournisseur=fmt):
                rep = self._completion(model)
                self.assertTrue(
                    rep["model"].endswith(model.split("/", 1)[1]),
                    "identité du modèle transmise telle quelle (contrôle "
                    "MODELE_DIVERGENT côté moteur Go) : " + rep["model"])
                self.assertEqual(rep["choices"][0]["finish_reason"], "stop")
                self.assertGreaterEqual(rep["usage"]["total_tokens"], 1)
                self.assertIn("FACTICE", rep["choices"][0]["message"]["content"])

    def test_4_fournisseurs_tous_appeles(self):
        for fmt in MODELES:
            self._completion(MODELES[fmt])
        for fmt, s in self.serveurs.items():
            self.assertGreaterEqual(len(s.requetes), 1,
                                    "le fournisseur factice " + fmt +
                                    " doit avoir été réellement appelé")

    def test_auth_401(self):
        for fmt in ("openai", "mistral", "anthropic", "gemini"):
            with self.subTest(fournisseur=fmt):
                self.serveurs[fmt].scenario = "auth401"
                with self.assertRaises(Exception):
                    self._completion(MODELES[fmt])
                self.serveurs[fmt].scenario = "nominal"

    def test_429_puis_500(self):
        for fmt, scenario in (("openai", "err429"), ("mistral", "err500")):
            self.serveurs[fmt].scenario = scenario
            with self.assertRaises(Exception):
                self._completion(MODELES[fmt])
            self.serveurs[fmt].scenario = "nominal"

    def test_json_malforme(self):
        self.serveurs["openai"].scenario = "json_malforme"
        with self.assertRaises(Exception):
            self._completion(MODELES["openai"])
        self.serveurs["openai"].scenario = "nominal"

    def test_troncature_transmise(self):
        """finish_reason non-stop TRANSMIS (le refus stop-only appartient aux
        profils du moteur Go, pas au pont)."""
        self.serveurs["mistral"].scenario = "troncature"
        rep = self._completion(MODELES["mistral"])
        self.assertEqual(rep["choices"][0]["finish_reason"], "length")
        self.serveurs["mistral"].scenario = "nominal"

    def test_modele_divergent_transmis(self):
        for fmt in ("openai", "anthropic", "gemini"):
            self.serveurs[fmt].scenario = "modele_divergent"
            rep = self._completion(MODELES[fmt])
            self.assertTrue(rep["model"].endswith("-divergent"),
                            "divergence transmise telle quelle : " + rep["model"])
            self.serveurs[fmt].scenario = "nominal"

    def test_usage_absent_refusé(self):
        self.serveurs["gemini"].scenario = "usage_absent"
        with self.assertRaises(Exception) as ctx:
            self._completion(MODELES["gemini"])
        self.assertIn("usage", str(ctx.exception))
        self.serveurs["gemini"].scenario = "nominal"

    def test_cle_absente_fail_closed(self):
        del os.environ["COORD_CLE_CI"]
        try:
            with self.assertRaises(Exception) as ctx:
                self._completion(MODELES["openai"])
            # L'erreur structurée porte son type dans l'attribut (le message
            # seul n'est pas un contrat) :
            self.assertEqual(getattr(ctx.exception, "type", ""),
                             "CLE_ABSENTE")
            self.assertIn("non renseignée", str(ctx.exception))
        finally:
            os.environ["COORD_CLE_CI"] = CLE_TEST

    def test_delai_depasse(self):
        self.serveurs["openai"].scenario = "delai"
        backend = LiteLLMBackend(delai_s=1.0)
        with self.assertRaises(Exception):
            backend.completion(
                MODELES["openai"],
                [{"role": "user", "content": "x"}],
                contexte={"destination": self.contexte_dests["openai/"]})
        self.serveurs["openai"].scenario = "nominal"

    def test_aucun_retry_implicite(self):
        """num_retries=0 : un seul appel réel du fournisseur malgré l'échec."""
        self.serveurs["mistral"].scenario = "err500"
        s = self.serveurs["mistral"]
        with self.assertRaises(Exception):
            self._completion(MODELES["mistral"])
        self.assertEqual(len(s.requetes), 1,
                         "aucun retry implicite : la reprise est côté Go")
        self.serveurs["mistral"].scenario = "nominal"

    def test_secret_absent_des_requetes(self):
        """La clé est portée par l'en-tête Authorization (mécanisme NORMAL du
        fournisseur) mais ne fuit JAMAIS dans la charge ni dans le chemin."""
        self._completion(MODELES["openai"])
        for s in self.serveurs.values():
            for r in s.requetes:
                self.assertNotIn(CLE_TEST, r["corps"])
                self.assertNotIn(CLE_TEST, r["chemin"])


@unittest.skipUnless(SDK_PRESENT, RAISON_ABSENT)
class TestPrefixeSdk(unittest.TestCase):
    """M-COORD-MISTRAL-VPS-01 (D2) : destination avec prefixe_sdk.

    Défaut reproduit (NO-GO-D1) : sans préfixe fournisseur, litellm refuse
    ("LLM Provider NOT provided") et AUCUNE requête n'atteint le fournisseur.
    Avec prefixe_sdk declare : le SDK recoit "<prefixe_sdk><modele>", le
    fournisseur recoit le modele SANS préfixe (identité exacte côté moteur
    Go) et la réponse model est transmise telle quelle (passthrough).
    """

    def test_modele_sans_prefixe_refuse_par_le_sdk(self):
        """Reproduction NO-GO-D1 : modèle sans préfixe => ERREUR_SDK."""
        os.environ["COORD_CLE_CI"] = CLE_TEST
        s = ServeurFactice("mistral")
        s.demarrer()
        self.addCleanup(s.arreter)
        backend = LiteLLMBackend(delai_s=15.0)
        dest = {"api_base": s.api_base, "cle_env": "COORD_CLE_CI"}
        with self.assertRaises(ErreurBackend) as ctx:
            backend.completion(
                "factice-mistral-sans-prefixe",
                [{"role": "user", "content": "repro"}],
                contexte={"destination": dest})
        self.assertEqual(ctx.exception.type, "ERREUR_SDK")

    def test_prefixe_sdk_route_et_preserve_identite(self):
        """D2 : prefixe_sdk ajoute le préfixe SDK, identité passthrough."""
        os.environ["COORD_CLE_CI"] = CLE_TEST
        s = ServeurFactice("mistral")
        s.demarrer()
        self.addCleanup(s.arreter)
        backend = LiteLLMBackend(delai_s=15.0)
        dest = {"api_base": s.api_base, "cle_env": "COORD_CLE_CI",
                "prefixe_sdk": "mistral/"}
        rep = backend.completion(
            "factice-mistral-identite",
            [{"role": "user", "content": "MISTRAL-VPS-01 D2"}],
            contexte={"destination": dest})
        # le factice echo le model recu : le SDK a transmis le modele SANS
        # préfixe (identité exacte) et la réponse est passée telle quelle.
        self.assertEqual(rep["model"], "factice-mistral-identite")
        self.assertTrue(s.requetes, "au moins une requête atteinte")
        corps = json.loads(s.requetes[-1]["corps"])
        self.assertEqual(corps.get("model"), "factice-mistral-identite")
        self.assertNotIn(CLE_TEST, str(rep))


if __name__ == "__main__":
    unittest.main(verbosity=2)

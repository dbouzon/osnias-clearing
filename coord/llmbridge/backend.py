# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Backends d'inférence du pont COORD (contrat coord.llm.v1).

Deux implémentations, même frontière :

- FacticeBackend  : déterministe, hors réseau, pour la qualification locale
                    du contrat et la recette E2E avec le moteur Go (0 €).
- LiteLLMBackend  : mutualisation réelle via litellm-core (import RÉSULT au
                    premier appel : jamais de faux succès si le SDK est
                    absent, erreur structurée SDK_ABSENT).

Le pont est un TRANSPORT sans état : la voûte monétaire, l'anti-doublon,
les tarifs, l'annulation et la reprise restent au moteur Go.
"""

import json
import os
import re

# Expurgation partagée avec le moteur Go (sk-…, clés AIza…).
_MOTIF_SECRET = re.compile(r"(sk-[0-9A-Za-z_\-]{8,}|AIza[0-9A-Za-z_\-]{35})")


def expurger(texte):
    """Remplace tout secret potentiel par [EXPURGE] (jamais journalisé)."""
    return _MOTIF_SECRET.sub("[EXPURGE]", texte or "")


def extraire_empreinte(contenu):
    """Reproduit extraireEmpreinte du moteur Go (gabarit échappé, repli brut).

    Le gabarit de verdict est transporté ÉCHAPPÉ dans la charge (leçon
    M-COORD-PILOT-03) : le marqueur canonique est `empreinte_attendue\":\"`
    avec un repli sur la forme non échappée.
    """
    for marqueur in ('empreinte_attendue\\":\\"', 'empreinte_attendue":"'):
        idx = contenu.find(marqueur)
        if idx < 0:
            continue
        reste = contenu[idx + len(marqueur):]
        fin = reste.find('"')
        if fin == 64 and len(reste) >= 64:
            return reste[:64]
    return ""


class ErreurBackend(Exception):
    """Erreur structurée du backend : type stable, message expurgé, statut HTTP."""

    def __init__(self, type_err, message, statut=502):
        super().__init__(message)
        self.type = type_err
        self.message = expurger(message)
        self.statut = statut


class FacticeBackend:
    """Inférence factice déterministe pour la qualification locale du contrat.

    Routage par préfixe de modèle :
    - ``factice/exec/…``  : renvoie un livrable texte (exécutant) ;
      « MANDAT V1 » => livrable v1 erroné (marque SCENARIO:LIVRABLE_A_CORRIGER),
      « MANDAT V2 » => livrable v2 corrigé, sans marqueur => livrable constant
      historique.
    - ``factice/recep/…`` : renvoie un verdict JSON ancré (réceptionnaire) ;
      charge contenant SCENARIO:LIVRABLE_A_CORRIGER => A_CORRIGER (2 points),
      « CRITIQUE DE PLAN » => PLAN_ACCEPTE, sinon ACCEPTEE,
      empreinte extraite de la charge (ancrage vérifié par le moteur Go).
      V1+V2 simultanés => refus explicite MARQUEURS_CONTRADICTOIRES.
    Le modèle demandé est ÉCHANGÉ tel quel (aucun MODELE_DIVERGENT ici).
    """

    LIVRABLE = "LIVRABLE 1\nLIVRABLE 2\nLIVRABLE 3 (backend factice du pont, 0 EUR)"

    # Scénarios par MARQUEURS portés par la charge (M-COORD-DEP-03, #72).
    # Règles : marqueurs exacts et injectifs ; contradiction => refus
    # explicite (jamais un choix arbitraire) ; SANS marqueur => comportement
    # historique inchangé (figé par tests). Les marqueurs ne proviennent que
    # de charges composées par le moteur (boucle simulée 0 EUR) : aucun
    # effet sur le routage, les destinations, l'authentification ou les
    # bornes.
    MARQUEUR_V1 = "MANDAT V1"
    MARQUEUR_V2 = "MANDAT V2"
    MARQUEUR_LIVRABLE_A_CORRIGER = "SCENARIO:LIVRABLE_A_CORRIGER"

    LIVRABLE_V1 = ("LIVRABLE V1\n" + MARQUEUR_LIVRABLE_A_CORRIGER +
                   "\nBUDGET : 200 centimes = 20 euros"
                   "\nCOUT : 0,006 centime par appel, soit 0,00006 EUR")
    LIVRABLE_V2 = ("LIVRABLE V2 (corrige)\nBUDGET : 200 centimes = 2 euros"
                   "\nCOUT : 0,006 centime par appel, soit 0,006 c")

    POINTS_A_CORRIGER = [
        "correction 1 : ecrire « 200 centimes = 2 euros » et non 20 euros",
        "correction 2 : ecrire le cout reel « 0,006 centime par appel » "
        "sans division erronee",
    ]

    def _livrable(self, charge):
        v1 = self.MARQUEUR_V1 in charge
        v2 = self.MARQUEUR_V2 in charge
        if v1 and v2:
            raise ErreurBackend(
                "MARQUEURS_CONTRADICTOIRES",
                "mandat V1 et V2 dans la meme charge (refus explicite)", 400)
        if v1:
            return self.LIVRABLE_V1
        if v2:
            return self.LIVRABLE_V2
        return self.LIVRABLE

    def _verdict(self, charge):
        empreinte = extraire_empreinte(charge)
        if self.MARQUEUR_LIVRABLE_A_CORRIGER in charge:
            # Recette E3 DEP-03 : le moteur exige verdict_id pour toute
            # decision recevable (verdict sans reference sinon). Ajout
            # minimal, marqueurs et points INCHANGES.
            return json.dumps({
                "verdict": "A_CORRIGER",
                "points": list(self.POINTS_A_CORRIGER),
                "empreinte_attendue": empreinte,
                "verdict_id": "factice-verdict-a-corriger",
                "verdict_url": "urn:coord:factice",
            }, ensure_ascii=False)
        if "CRITIQUE DE PLAN" in charge:
            return json.dumps({
                "verdict": "PLAN_ACCEPTE",
                "points": ["factice : plan simple, rien de bloquant"],
                "empreinte_attendue": empreinte,
            }, ensure_ascii=False)
        return json.dumps({
            "verdict": "ACCEPTEE",
            "points": ["factice : 3 lignes présentes"],
            "empreinte_attendue": empreinte,
            "verdict_id": "factice-verdict",
            "verdict_url": "urn:coord:factice",
        }, ensure_ascii=False)

    def completion(self, model, messages, max_tokens=None, contexte=None):
        dernier = messages[-1]["content"] if messages else ""
        if model.startswith("factice/exec"):
            contenu = self._livrable(dernier)
        else:
            contenu = self._verdict(dernier)
        return {
            "id": "chatcmpl-factice",
            "object": "chat.completion",
            "created": 0,
            "model": model,
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": contenu},
                "finish_reason": "stop",
            }],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50,
                      "total_tokens": 150},
        }


class LiteLLMBackend:
    """Inférence mutualisée via litellm-core (SDK RÉEL, sans proxy).

    Garde-fous :
    - import RÉSULT : SDK absent => ErreurBackend SDK_ABSENT (503), JAMAIS
      un succès simulé ;
    - clé exclusivement par variable d'environnement (cle de la destination),
      absente => CLE_ABSENTE (503) AVANT tout appel réseau ;
    - num_retries=0 : AUCUN retry implicite (l'anti-doublon et la reprise
      appartiennent au moteur Go) ;
    - délai transmis au SDK ; usage et finish_reason TRANSMIS tels quels,
      jamais inventés : absent => REPONSE_BACKEND_INVALIDE.
    """

    def __init__(self, delai_s=60.0):
        self.delai_s = delai_s

    def completion(self, model, messages, max_tokens=None, contexte=None):
        try:
            import litellm  # import différé, réel, jamais contourné
        except Exception as e:  # pragma: no cover - dépend de l'environnement
            raise ErreurBackend(
                "SDK_ABSENT",
                "litellm-core non installé : installation interdite dans ce "
                "environnement (NON_EXECUTE local, couvert par la CI) : " + str(e),
                503)
        try:
            litellm.telemetry = False
        except Exception:
            pass
        dest = contexte["destination"]
        cle = os.environ.get(dest["cle_env"], "").strip()
        if not cle:
            raise ErreurBackend(
                "CLE_ABSENTE",
                "variable " + dest["cle_env"] + " non renseignée (fail closed, "
                "aucun appel réseau sans clé)", 503)
        kwargs = {
            "model": model,
            "messages": messages,
            "api_base": dest["api_base"],
            "api_key": cle,
            "num_retries": 0,       # aucun retry : la reprise est côté Go
            "timeout": self.delai_s,
        }
        if max_tokens:
            kwargs["max_tokens"] = max_tokens
        try:
            reponse = litellm.completion(**kwargs)
        except ErreurBackend:
            raise
        except Exception as e:
            raise ErreurBackend("ERREUR_SDK", str(e), 502)

        choix = getattr(reponse, "choices", None)
        if not choix:
            raise ErreurBackend("REPONSE_BACKEND_INVALIDE", "aucun choix", 502)
        premier = choix[0]
        message = getattr(premier, "message", None)
        if message is None or getattr(message, "content", None) is None:
            raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                                "message ou contenu absent", 502)
        fin = getattr(premier, "finish_reason", None)
        if not fin:
            raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                                "finish_reason absent (refus, jamais inventé)", 502)
        usage = getattr(reponse, "usage", None)
        total = getattr(usage, "total_tokens", None) if usage else None
        if total is None:
            raise ErreurBackend("REPONSE_BACKEND_INVALIDE",
                                "usage absent (refus, jamais inventé)", 502)
        return {
            "id": getattr(reponse, "id", "") or "chatcmpl-llmbridge",
            "object": "chat.completion",
            "created": getattr(reponse, "created", 0) or 0,
            "model": getattr(reponse, "model", "") or model,
            "choices": [{
                "index": 0,
                "message": {"role": "assistant",
                            "content": message.content or ""},
                "finish_reason": fin,
            }],
            "usage": {
                "prompt_tokens": getattr(usage, "prompt_tokens", 0) or 0,
                "completion_tokens": getattr(usage, "completion_tokens", 0) or 0,
                "total_tokens": total,
            },
        }

#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
"""Coord06 v1.2 (candidate, NON ACTIVEE) — extensions du protocole M-COORD-06.

Module d'extension : importe coord06 (v1.1) SANS le modifier et ajoute les
règles candidates v1.2 (issue #132, amendement B1..B5) :
- B1 profondeur de critique proportionnee au risque, reception independante
  obligatoire a tous les niveaux ;
- B2 integrite des preuves d'execution (commande, exit, sortie) ;
- B3 interdiction du PASS implicite sans test reellement execute ;
- B4 reprise idempotente apres interruption depuis l'etat durable ;
- B5 budget d'appels API et suppression des appels redondants.

La v1.1 reste la version en vigueur : PROTOCOL_VERSION de coord06 n'est pas
modifie. Python stdlib uniquement.
"""
import coord06

PROTOCOL_VERSION_V12 = "M-COORD-06 v1.2 (candidate, non activee)"

# ---- B1 : classification du risque et profondeur de critique ----
RISK_LEVELS = {
    # niveau -> (profondeur de critique, reception independante obligatoire)
    "documentaire": ("allegee", True),
    "code": ("standard", True),
    "critique": ("complete", True),  # Core bancaire, crypto, EBICS, sequestre
}

CRITICAL_SCOPES = ("core", "crypto", "ebics", "sequestre", "bancaire", " solidity")


def classify_risk(scope_description):
    """Classe une mission par niveau de risque a partir de son perimetre.

    Fail closed : tout perimetre inconnu ou critique explicite est classe
    'critique' (profondeur complete), jamais allege.
    """
    s = (scope_description or "").lower()
    if any(k in s for k in CRITICAL_SCOPES) or not s.strip():
        return "critique"
    if "code" in s or "go " in s or "test" in s or "implementation" in s:
        return "code"
    return "documentaire"


def critique_depth(level):
    if level not in RISK_LEVELS:
        return "complete"  # fail closed
    return RISK_LEVELS[level][0]


def reception_required(level):
    # La reception independante est OBLIGATOIRE a tous les niveaux (B1).
    return True


# ---- B2 : integrite des preuves d'execution ----
def executed_proof(command, exit_code, output):
    """Construit une preuve d'execution (deterministe, reproductible)."""
    return {"command": command, "exit_code": exit_code, "output": output}


def is_executed_proof(proof):
    """Une preuve d'execution est recevable ssi commande NON vide + exit connu +
    sortie non vide. Toute absence rend la preuve non recevable."""
    return (
        isinstance(proof, dict)
        and proof.get("command")
        and proof.get("output")
        and proof.get("exit_code") in (0, "0")
    )


# ---- B3 : garde anti PASS implicite ----
def resolve_test_claim(has_executed_proof, declared_pass):
    """Un PASS declare sans preuve d'executee n'est jamais accepte.
    Retourne ('PASS', '') ou ('RESERVE', motif) ou ('BLOQUE', motif).
    """
    if declared_pass and not has_executed_proof:
        return "BLOQUE", "PASS declare sans test reellement execute"
    if declared_pass and has_executed_proof:
        return "PASS", ""
    return "RESERVE", "absence d'execution documentee"


# ---- B4 : reprise idempotente apres interruption ----
class ProtocolState:
    """Etat durable de mission (GitHub faisant foi), reprise idempotente.

    - resume() : exactement UNE relecture de l'etat durable et AU PLUS un
      nouvel appel API ; aucun re-depot de preuve, aucun ACK ni verdict fictif ;
    - le compteur api_calls prouve l'absence d'appels redondants (B5).
    """

    def __init__(self, store, api):
        self.store = store          # depot durable (issue GitHub relle ou mock)
        self.api = api              # pont API (reel ou mock)
        self.api_calls = 0
        self.published_markers = set()

    def publish_once(self, marker, body):
        """Publication idempotente par marqueur (regle v1.0 §4.2)."""
        if marker in self.published_markers:
            return False  # deja publie : aucun doublon
        self.store.post(marker + "\n" + body)
        self.published_markers.add(marker)
        return True

    def resume(self):
        """Reprise apres interruption : 1 relecture + au plus 1 appel API."""
        state = self.store.read_state()      # exactement une relecture
        if state.get("state") == "DECISION_API":
            # Une decision existe deja : elle est re-exploitee, JAMAIS
            # rejouee ni fabriquee (aucun nouvel appel).
            return {"resumed": True, "api_calls": 0, "verdict": state.get("verdict")}
        if state.get("calls_played", 0) > 0:
            # Un appel a deja ete joue pour cette etape : pas de rejeu.
            return {"resumed": True, "api_calls": 0, "verdict": None,
                    "motif": "appel deja joue pour cette etape"}
        self.api_calls += 1
        verdict = self.api.decide(state)
        self.store.write_state(dict(state, state="DECISION_API", verdict=verdict,
                                    calls_played=1))
        return {"resumed": True, "api_calls": 1, "verdict": verdict}


# ---- B5 : budget d'appels API ----
DEFAULT_API_BUDGET = 3


def within_budget(calls, budget=DEFAULT_API_BUDGET):
    return calls <= budget


def count_redundant(resume_results):
    """Compte les appels joues lors de reprises : tout appel rejoue alors que
    la decision existe deja est redondant (B5, garde anti-rejeu)."""
    return sum(1 for r in resume_results if r.get("redundant"))


# ---- Enregistrement v1.2 (extension du format v1.1, non active) ----
def validate_record_v12(record):
    """Etend validate_record v1.1 : l'enregistrement v1.2 porte en plus
    risk_level, critique_depth, executed_proofs et api_calls.
    Ne modifie PAS la validation v1.1 (delegation stricte)."""
    ok, errors = coord06.validate_record(record)
    if not ok:
        return ok, errors
    if record.get("protocol") != PROTOCOL_VERSION_V12:
        errors.append("version v1.2 attendue")
    if record.get("risk_level") not in RISK_LEVELS:
        errors.append("niveau de risque inconnu")
    if record.get("critique_depth") not in ("allegee", "standard", "complete"):
        errors.append("profondeur de critique inconnue")
    for p in record.get("executed_proofs", []):
        if not is_executed_proof(p):
            errors.append("preuve d'execution non recevable")
    if not within_budget(record.get("api_calls", 0)):
        errors.append("budget d'appels API depasse")
    return len(errors) == 0, errors


def record_v12_from(mission, issue, sha_git, proofs, model_api, ts_utc,
                    risk_level, executed_proofs, api_calls,
                    state=None, verdict_url="", verdict_id="", ack=False):
    rec = coord06.record_from(mission, issue, sha_git, proofs, model_api,
                              ts_utc, state=state, verdict_url=verdict_url,
                              verdict_id=verdict_id, ack=ack)
    rec["protocol"] = PROTOCOL_VERSION_V12
    rec["risk_level"] = risk_level
    rec["critique_depth"] = critique_depth(risk_level)
    rec["executed_proofs"] = executed_proofs
    rec["api_calls"] = api_calls
    return rec

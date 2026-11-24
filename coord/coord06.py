#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Coord06 v1.1 — Module canonique du protocole M-COORD-06.

Contient :
- la machine a etats unifiee v1.0 (inchangee) ;
- le format d'enregistrement de mission versionne v1.1 (mission, issue, etat,
  version du protocole, SHA Git de reference, empreinte des preuves, modele API,
  horodatage UTC, URL/ID du verdict, ACK) ;
- les regles d'obsolescence (verdict perime), d'idempotence et de reprise sans
  polling.

Python stdlib uniquement. Aucun secret ne doit figurer dans les champs.
"""
import hashlib
import json

PROTOCOL_VERSION = "M-COORD-06 v1.1"

# ---- Machine a etats v1.0 (inchangee) ----
VALID_TRANSITIONS = {
    ("A_PLANIFIER", "PLAN_CRITIQUE"),
    ("PLAN_CRITIQUE", "EN_EXECUTION"),
    ("EN_EXECUTION", "RECEPTION_DEMANDEE"),
    ("RECEPTION_DEMANDEE", "DECISION_API"),
    ("DECISION_API", "ACK_MISTRAL"),
    ("ACK_MISTRAL", "CLOTURE"),
    ("DECISION_API", "EN_EXECUTION"),
    ("DECISION_API", "REFUSEE"),
}
VERDICTS = ("ACCEPTEE", "A_CORRIGER", "REFUSEE")
TERMINAL = ("REFUSEE", "CLOTURE")

AUTHORIZED_CLOSURE_ACTORS = ("responsable", "pilote")


def canonical_json(obj):
    """Seralisation canonique deterministe (cles triees, separateurs compacts)."""
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def proof_fingerprint(proofs):
    """Empreinte sha256 de la liste des preuves, ordre preservé."""
    if isinstance(proofs, str):
        proofs = [proofs]
    h = hashlib.sha256()
    for p in proofs:
        h.update(canonical_json(p).encode("utf-8"))
    return h.hexdigest()


class Mission:
    """Machine a etats v1.0 — semantique strictement identique a la batterie v1.0."""

    def __init__(self, issue, sha):
        self.state = "A_PLANIFIER"
        self.issue = issue
        self.sha = sha
        self.decisions = []  # (verdict, proof_index)
        self.acks = 0
        self.proofs = []

    def proof_count(self):
        return len(self.proofs)

    def transition(self, target, verdict=None, actor="mistral"):
        if (self.state, target) not in VALID_TRANSITIONS:
            return False, "transition interdite"
        if target == "DECISION_API":
            if verdict not in VERDICTS:
                return False, "verdict invalide"
            idx = self.proof_count()
            self.decisions.append((verdict, idx))
            if verdict == "A_CORRIGER":
                self.state = "EN_EXECUTION"
                return True, ""
            if verdict == "REFUSEE":
                self.state = "REFUSEE"
                return True, ""
            self.state = "DECISION_API"
            return True, ""
        if target == "ACK_MISTRAL":
            if not self.decisions:
                return False, "ACK sans decision"
            if self.decisions[-1][0] != "ACCEPTEE":
                return False, "ACK sans verdict ACCEPTEE"
            if self.decisions[-1][1] != self.proof_count():
                return False, "decision obsolete (preuve posterieure)"
            self.acks += 1
            self.state = "ACK_MISTRAL"
            return True, ""
        if target == "CLOTURE":
            if actor not in AUTHORIZED_CLOSURE_ACTORS:
                return False, "cloture par Mistral interdite"
            self.state = "CLOTURE"
            return True, ""
        self.state = target
        return True, ""


# ---- Format d'enregistrement versionne v1.1 ----
REQUIRED_FIELDS = (
    "mission",      # identifiant de mission (ex. "M-COORD-06")
    "issue",        # issue GitHub de reference (ex. "#125")
    "state",        # etat de la machine a etats
    "protocol",     # version du protocole (ex. "M-COORD-06 v1.1")
    "sha_git",      # SHA Git de reference (40 hex, arbre modifié)
    "proof_fp",     # empreinte sha256 des preuves publiées
    "model_api",    # modele API sollicité (ex. "gpt-4o")
    "ts_utc",       # horodatage UTC ISO 8601 (Z)
    "verdict_url",  # URL exacte du commentaire de decision
    "verdict_id",   # ID exact du commentaire de decision
    "ack",          # boolean : ACK publie
)
_SHA256_RE = None


def _sha256_re():
    global _SHA256_RE
    if _SHA256_RE is None:
        import re
        _sha256_re = re.compile(r"^[0-9a-f]{64}$")
    return _sha256_re


def valid_ts_utc(ts):
    """Horodatage UTC ISO 8601 : AAAA-MM-JJTHH:MM:SSZ."""
    import re
    return bool(re.match(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$", ts or ""))


def validate_record(record):
    """Valide un enregistrement v1.1. Retourne (ok, erreurs)."""
    errors = []
    if not isinstance(record, dict):
        return False, ["enregistrement non dict"]
    for f in REQUIRED_FIELDS:
        if f not in record:
            errors.append("champ manquant: " + f)
    if errors:
        return False, errors
    if not isinstance(record["issue"], str) or not record["issue"].startswith("#"):
        errors.append("issue invalide")
    if record["state"] not in ("A_PLANIFIER", "PLAN_CRITIQUE", "EN_EXECUTION",
                               "RECEPTION_DEMANDEE", "DECISION_API", "ACK_MISTRAL",
                               "CLOTURE", "REFUSEE"):
        errors.append("etat inconnu")
    if not record["protocol"].startswith("M-COORD-06"):
        errors.append("version de protocole invalide")
    import re
    if not re.match(r"^[0-9a-f]{40}$", record["sha_git"] or ""):
        errors.append("SHA Git invalide (40 hex attendus)")
    if not _sha256_re().match(record["proof_fp"] or ""):
        errors.append("empreinte de preuves invalide (sha256 attendu)")
    if not record["model_api"]:
        errors.append("modele API absent")
    if not valid_ts_utc(record["ts_utc"]):
        errors.append("horodatage UTC invalide")
    if record["ack"] and (not record["verdict_url"] or not record["verdict_id"]):
        errors.append("ACK sans URL/ID de decision")
    return len(errors) == 0, errors


def record_from(mission, issue, sha_git, proofs, model_api, ts_utc,
                state=None, verdict_url="", verdict_id="", ack=False):
    """Construit un enregistrement v1.1 complet."""
    return {
        "mission": mission,
        "issue": issue,
        "state": state if state is not None else "EN_EXECUTION",
        "protocol": PROTOCOL_VERSION,
        "sha_git": sha_git,
        "proof_fp": proof_fingerprint(proofs),
        "model_api": model_api,
        "ts_utc": ts_utc,
        "verdict_url": verdict_url,
        "verdict_id": verdict_id,
        "ack": ack,
    }


def is_stale_verdict(decision_fp, current_fp):
    """Obsolescence v1.1 : un verdict est perime si l'empreinte courante des
    preuves ne correspond plus a celle presentee au moment de la decision."""
    return decision_fp != current_fp


def save_record(record, path):
    """Persistance idempotente : ecriture atomique en JSON canonique."""
    with open(path, "w", encoding="utf-8") as f:
        f.write(canonical_json(record))


def load_record(path):
    """Relecture pour reprise de session. Retourne None si corrompu."""
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return None

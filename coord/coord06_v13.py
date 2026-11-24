#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Coord06 v1.3 (candidate, NON ACTIVEE) — extensions du protocole M-COORD-06.

Module d'extension : importe coord06 (v1.1) et coord06_v12 (v1.2 candidate)
SANS les modifier et ajoute les regles candidates v1.3 (issue #150,
amendement C1..C6) :
- C1 matrice de preflight des outils et acces, constatee par execution ;
- C2 boucle de correction interne sur la meme issue / meme PR Draft ;
- C3 format norme d'enregistrement d'execution (recette reproductible) ;
- C4 reception ancre au SHA Git exact, reevaluation proportionnee ;
- C5 criteres d'arret normes et reserves bloquantes / non bloquantes ;
- C6 sobriete documentaire et applicative.

La v1.2 reste la version en vigueur. Python stdlib uniquement.
"""
import hashlib

import coord06
import coord06_v12

PROTOCOL_VERSION_V13 = "M-COORD-06 v1.3 (candidat, non activee)"


# ---- C1 : matrice de preflight ----
PREFLIGHT_STATUSES = ("EXECUTABLE", "PARTIEL", "BLOQUE")


def preflight_status(present, executed, exit_code):
    """Statut de preflight constate PAR EXECUTION, jamais declare.

    Fail closed : outil absent ou non execute => BLOQUE ; execute avec exit
    non nul => PARTIEL (plan de preuve alternatif requis) ; execute avec
    exit 0 => EXECUTABLE.
    """
    if not present or not executed:
        return "BLOQUE"
    if exit_code in (0, "0"):
        return "EXECUTABLE"
    return "PARTIEL"


def validate_preflight(matrix, required_tools):
    """Valide une matrice de preflight. Chaque outil requis doit apparaitre
    avec un statut constate, une preuve d'execution recevable et un plan de
    preuve alternatif si le statut n'est pas EXECUTABLE. Fail closed."""
    errors = []
    if not isinstance(matrix, list) or not matrix:
        return False, ["matrice de preflight absente"]
    by_tool = {e.get("outil"): e for e in matrix if isinstance(e, dict)}
    for tool in required_tools:
        entry = by_tool.get(tool)
        if entry is None:
            errors.append("outil requis absent de la matrice: " + str(tool))
            continue
        if entry.get("statut") not in PREFLIGHT_STATUSES:
            errors.append("statut non normalise: " + str(tool))
        if entry.get("statut") == "EXECUTABLE" and not coord06_v12.is_executed_proof(entry.get("preuve", {})):
            errors.append("statut EXECUTABLE sans preuve executee: " + str(tool))
        if entry.get("statut") in ("PARTIEL", "BLOQUE") and not entry.get("plan_alternatif"):
            errors.append("statut non EXECUTABLE sans plan de preuve alternatif: " + str(tool))
    return len(errors) == 0, errors


# ---- C2 : boucle de correction interne et registre des reserves ----
RESERVE_LIFECYCLE = ("OUVERTE", "LEVEE", "EXOGENE", "PILOTE", "CLOTUREE")
ESCALATION_REASONS = ("changement_perimetre", "outil_indisponible",
                      "deux_cycles_sans_convergence", "decision_pilote_requise")


def correction_step(verdict, cycle_count):
    """Apres A_CORRIGER : retour en T2/T3 sur la meme issue et la meme PR
    Draft. Escalade au pilote au bout de deux cycles sans convergence."""
    if verdict != "A_CORRIGER":
        return {"action": "aucune", "escalade": False}
    if cycle_count >= 2:
        return {"action": "correction", "escalade": True,
                "motif": ESCALATION_REASONS[2]}
    return {"action": "correction", "escalade": False}


def escalate(reason):
    """Escalade au pilote uniquement pour les motifs bornes de l'amendement."""
    if reason not in ESCALATION_REASONS:
        return False, "motif d'escalade non normalise"
    return True, ""


def validate_reserve_registry(registry):
    """Chaque reserve du registre porte un cycle de vie normalise et, si elle
    est LEVEE, une preuve recevable ; si EXOGENE/PILOTE, un plan de levee."""
    errors = []
    if not isinstance(registry, list):
        return False, ["registre non list"]
    for r in registry:
        if not isinstance(r, dict) or r.get("statut") not in RESERVE_LIFECYCLE:
            errors.append("reserve sans cycle de vie normalise")
            continue
        if r["statut"] == "LEVEE" and not coord06_v12.is_executed_proof(r.get("preuve", {})):
            errors.append("reserve LEVEE sans preuve executee: " + str(r.get("id")))
        if r["statut"] in ("EXOGENE", "PILOTE") and not r.get("plan_de_levee"):
            errors.append("reserve sans plan de levee: " + str(r.get("id")))
    return len(errors) == 0, errors


# ---- C3 : format norme d'enregistrement d'execution ----
EXEC_LABELS = ("REEL", "SIMULE", "NON_EXECUTE")
_HEAD_LINES = 3
_TAIL_LINES = 3


def output_fingerprint(output):
    return hashlib.sha256((output or "").encode("utf-8")).hexdigest()


def normalized_execution(command, exit_code, output, tool_version, sha_git, ts_utc, label):
    """Construit un enregistrement d'execution normalise (C3).

    La sortie complete n'est pas stockee : seules les premieres/dernieres
    lignes sont conservees, l'empreinte sha256 de la sortie complete permet
    la comparaison deterministe cote recepteur.
    """
    lines = (output or "").splitlines()
    head = lines[:_HEAD_LINES]
    tail = lines[-_TAIL_LINES:] if len(lines) > _HEAD_LINES else []
    return {
        "command": command,
        "exit_code": exit_code,
        "output_head": head,
        "output_tail": tail,
        "output_fp": output_fingerprint(output or ""),
        "tool_version": tool_version,
        "sha_git": sha_git,
        "ts_utc": ts_utc,
        "label": label,
    }


def is_normalized_execution(entry):
    """Recevabilite d'un enregistrement normalise (etiquette obligatoire,
    REEL exige exit 0 + sortie + empreinte + SHA + UTC ; NON_EXECUTE n'est
    jamais recevable comme preuve de PASS (B3 conserve))."""
    if not isinstance(entry, dict):
        return False
    if entry.get("label") not in EXEC_LABELS:
        return False
    if entry.get("label") == "NON_EXECUTE":
        return False
    if entry.get("label") == "REEL":
        return (bool(entry.get("command")) and entry.get("exit_code") in (0, "0")
                and bool(entry.get("output_fp"))
                and len(entry.get("sha_git", "")) == 40
                and coord06.valid_ts_utc(entry.get("ts_utc"))
                and bool(entry.get("tool_version")))
    return bool(entry.get("command")) and bool(entry.get("output_fp"))


# ---- C4 : reception ancre au SHA, reevaluation proportionnee ----
def reception_anchor(sha_git, proofs):
    return {"sha_git": sha_git, "proof_fp": coord06.proof_fingerprint(proofs)}


def is_stale_reception(anchor, sha_git, proofs):
    """Un verdict rendu sur un couple (SHA, empreinte) est perime pour tout
    autre couple (extension de A2 au SHA Git)."""
    cur = reception_anchor(sha_git, proofs)
    return anchor.get("sha_git") != cur["sha_git"] or anchor.get("proof_fp") != cur["proof_fp"]


def reevaluation_scope(corrected_files, changed_files):
    """Reevaluation proportionnee : complete si le diff apporte un perimetre
    nouveau (fichier hors corrections demandees), restreinte sinon."""
    unexpected = [f for f in changed_files if f not in corrected_files]
    if unexpected:
        return "completee", unexpected
    changed_corrected = [f for f in changed_files if f in corrected_files]
    if changed_corrected:
        return "restreinte", changed_corrected
    return "aucune", []


# ---- C5 : criteres d'arret et reserves bloquantes ----
STOP_CRITERIA = ("ACCEPTEE", "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES",
                 "A_CORRIGER", "BLOQUEE")


def blocking_reserve(reserve):
    """Une reserve est BLOQUANTE si elle empeche l'usage contractuel du
    livrable ou contredit une limite de la mission."""
    return bool(reserve and isinstance(reserve, dict) and reserve.get("bloquante") is True)


def stop_criteria(reserves):
    """Statut d'arret norme a partir du registre des reserves.

    - reserve bloquante presente => BLOQUEE ;
    - reserve non bloquante sans plan de levee => A_CORRIGER (le plan est
      exigible avant tout arret) ;
    - reserve non bloquante avec plan => ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES ;
    - aucun registre => ACCEPTEE.
    La fusion et l'activation restent reservees au pilote, hors de ce module.
    """
    if not reserves:
        return "ACCEPTEE"
    if any(blocking_reserve(r) for r in reserves):
        return "BLOQUEE"
    if any(not r.get("plan_de_levee") for r in reserves):
        return "A_CORRIGER"
    return "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES"


def delivery_definitive(status, required_proofs_executed):
    """Interdiction de declarer une livraison definitive sans les preuves
    requises : la fusion reste au pilote et exige des preuves relevees."""
    if status == "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES" and not required_proofs_executed:
        return False
    return status in ("ACCEPTEE", "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES")


# ---- C6 : sobriete documentaire ----
MAX_REPORT_FILES = 4  # specification, rapport, annexes/preuves, registre


def sober(file_count, new_relays=0, new_issues=0):
    """Sobriete : au plus une PR Draft unique, pas de nouveau relais ni issue
    si l'existant suffit ; enonce normatif verificable."""
    return (new_relays == 0 and new_issues == 0
            and 0 < file_count <= MAX_REPORT_FILES)


# ---- Enregistrement v1.3 (delegation stricte, non active) ----
def validate_record_v13(record):
    """Etend la validation v1.2 : l'enregistrement v1.3 porte en plus
    preflight, registre des reserves, executions normalisees, ancrage SHA et
    statut d'arret. Ne modifie NI la validation v1.1, NI la v1.2 : les champs
    v1.2 sont verifies par delegation sur copie portant la version v1.2, la
    version v1.3 etant controlee separement (la verification stricte
    d'egalite de version de coord06_v12 ne doit pas rejeter un v1.3)."""
    if not isinstance(record, dict):
        return False, ["enregistrement non dict"]
    errors = []
    if record.get("protocol") != PROTOCOL_VERSION_V13:
        errors.append("version v1.3 attendue")
    tmp = dict(record)
    tmp["protocol"] = coord06_v12.PROTOCOL_VERSION_V12
    ok12, errors12 = coord06_v12.validate_record_v12(tmp)
    if not ok12:
        errors.extend(errors12)
    for e in record.get("executions_normalisees", []):
        if not is_normalized_execution(e):
            errors.append("execution normalisee non recevable")
    if record.get("stop_criterion") not in STOP_CRITERIA:
        errors.append("criteres d'arret inconnus")
    if "reception_anchor" not in record or not isinstance(record["reception_anchor"], dict):
        errors.append("ancrage de reception SHA absent")
    ok_reg, reg_errors = validate_reserve_registry(record.get("reserve_registry", []))
    errors.extend(reg_errors)
    return len(errors) == 0, errors


def record_v13_from(mission, issue, sha_git, proofs, model_api, ts_utc,
                    risk_level, executed_proofs, api_calls,
                    executions_normalisees, reserve_registry, stop_criterion,
                    state=None, verdict_url="", verdict_id="", ack=False):
    rec = coord06_v12.record_v12_from(mission, issue, sha_git, proofs, model_api,
                                      ts_utc, risk_level, executed_proofs,
                                      api_calls, state=state,
                                      verdict_url=verdict_url, verdict_id=verdict_id,
                                      ack=ack)
    rec["protocol"] = PROTOCOL_VERSION_V13
    rec["executions_normalisees"] = executions_normalisees
    rec["reserve_registry"] = reserve_registry
    rec["reception_anchor"] = reception_anchor(sha_git, proofs)
    rec["stop_criterion"] = stop_criterion
    return rec

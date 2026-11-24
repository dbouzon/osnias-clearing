#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Coord06 v1.4 (candidate, NON ACTIVEE) — extensions du protocole M-COORD-06.

Module d'extension : importe coord06 (v1.1), coord06_v12 (v1.2 candidate) et
coord06_v13 (v1.3, en vigueur) SANS les modifier et ajoute les regles
candidates v1.4 (issue #157, mission M-COORD-09, amendement D1..D7) :
- D1 continuite autonome imperatif : A_CORRIGER renvoie immediatement a
  l'etape concernee (conception si critique prealable, execution si
  reception), sans rendre la main au pilote ; compteur absolu de rounds
  (critique ciblee au 2e round consecutif, escalade bornee C2 au 3e) ;
  budget d'appels par phase ;
- D2 etat de reprise etendu a support double (Knowledge persistante +
  commentaire d'issue ; GitHub faisant foi) ;
- D3 empreintes automatisees (aucune transcription manuelle, parite
  verifiee par calcul) ;
- D4 faisabilite des tests qualifiee a la conception (FEASIBLE_REEL,
  FEASIBLE_MOCK, INFEASIBLE_ALTERNATIVE) ;
- D5 dossier de reception en six sections normees (formats existants
  reutilises) ;
- D6 reception differentielle liée au SHA precedent (ancre complete :
  SHA precedents/courant, empreinte, criteres valides listes) ;
- D7 mesures de coordination nommees avec unites.

La v1.3 reste la version en vigueur. Python stdlib uniquement.
"""
import hashlib

import coord06
import coord06_v12
import coord06_v13

PROTOCOL_VERSION_V14 = "M-COORD-06 v1.4 (candidat, non activee)"


# ---- D1 : continuite autonome imperative ----
PHASES = ("critique_prealable", "reception")
TARGETED_CRITIQUE_ROUND = 2   # critique ciblee au 2e round A_CORRIGER consecutif
ESCALATION_ROUND = 3          # escalade bornee C2 au 3e round sans convergence
CONTINUATION_VERDICTS = ("ACCEPTEE", "ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES")


def continuity_step(verdict, phase, round_count):
    """Regle impérative de continuite : tout verdict A_CORRIGER entraîne un
    retour immediat a l'etape concernee, dans la meme session.

    - A_CORRIGER en phase critique_prealable -> retour CONCEPTION ;
    - A_CORRIGER en phase reception -> retour EXECUTION ;
    - ACCEPTEE / ACCEPTEE_AVEC_RESERVES_NON_BLOQUANTES -> tranche suivante
      ou preparation de cloture (jamais d'interruption) ;
    - REFUSEE -> arret terminal (v1.0 conserve) ;
    - compteur absolu de rounds consecutifs : critique ciblee demandee au
      2e round, escalade bornee coord06_v13 au 3e (motif
      deux_cycles_sans_convergence) ; l'escalade ne suspend jamais le
      travail deja publie.
    """
    if verdict == "REFUSEE":
        return {"action": "arret", "retour": None, "critique_ciblee": False,
                "escalade": False, "motif": ""}
    if verdict in CONTINUATION_VERDICTS:
        return {"action": "poursuivre", "retour": "tranche_suivante",
                "critique_ciblee": False, "escalade": False, "motif": ""}
    if verdict != "A_CORRIGER":
        return {"action": "refuse", "retour": None, "critique_ciblee": False,
                "escalade": False, "motif": "verdict non normalise"}
    if phase not in PHASES:
        return {"action": "refuse", "retour": None, "critique_ciblee": False,
                "escalade": False, "motif": "phase non normalisee"}
    retour = "conception" if phase == "critique_prealable" else "execution"
    critique_ciblee = round_count >= TARGETED_CRITIQUE_ROUND
    escalade = round_count >= ESCALATION_ROUND
    motif = ""
    if escalade:
        ok, err = coord06_v13.escalate("deux_cycles_sans_convergence")
        motif = "deux_cycles_sans_convergence" if ok else err
    return {"action": "continuer", "retour": retour,
            "critique_ciblee": critique_ciblee, "escalade": escalade,
            "motif": motif}


def pilot_solicitation(blockage_documented=False, alternatives_searched=False,
                       pilot_decision_required=False):
    """Une sollicitation du pilote n'est justifiee que dans deux cas bornes :
    (a) blocage objectif hors autonomie, documente apres recherche
    d'alternatives autorisees ; (b) decision reservee au pilote.
    Tout le reste (y compris un verdict A_CORRIGER ou une reserve non
    bloquante) n'interrompt jamais le travail.
    """
    if pilot_decision_required:
        return True, "decision_pilote_requise"
    if blockage_documented and alternatives_searched:
        return True, "blocage_objectif_documente"
    return False, ""


def phase_api_budget(phase, rounds=1):
    """Budget d'appels PAR PHASE (D1) : critique prealable 2 appels ;
    reception 2 appels par round plus un appel par round de correction
    supplementaire ; un appel de verification du pont est admis apres une
    reinitialisation de sandbox. Tout appel est comptabilise (D7) ; aucun
    appel dejoue n'est jamais rejoue (B4/B5 conserves).
    """
    if phase not in PHASES:
        return 0
    if phase == "critique_prealable":
        return 2
    return 2 * rounds + max(0, rounds - 1)


# ---- D2 : etat de reprise etendu, support double ----
RESUME_REQUIRED_FIELDS = (
    "mission",        # identifiant de mission (ex. "M-COORD-09")
    "tranche",        # identifiant de tranche (ex. "T2")
    "round",          # compteur de round de la tranche (int >= 0)
    "issue",          # issue GitHub de reference (ex. "#157")
    "branche",        # branche de mission
    "sha_base",       # SHA Git de la base (40 hex)
    "sha_head",       # SHA Git de tete verifier avant reprise (40 hex ou "")
    "curseur_relais", # dernier id de commentaire traite sur le relais #106 (int)
    "reserves_ouvertes",  # liste des reserves ouvertes (registre C2)
    "prochaine_action",  # action precise a executer a la reprise
    "ts_utc",         # horodatage UTC ISO 8601 (Z)
)

RESUME_SUPPORTS = ("knowledge", "github")


def resume_record(mission, tranche, round_, issue, branche, sha_base,
                  curseur_relais, reserves_ouvertes, prochaine_action, ts_utc,
                  sha_head=""):
    return {
        "mission": mission,
        "tranche": tranche,
        "round": round_,
        "issue": issue,
        "branche": branche,
        "sha_base": sha_base,
        "sha_head": sha_head,
        "curseur_relais": curseur_relais,
        "reserves_ouvertes": reserves_ouvertes,
        "prochaine_action": prochaine_action,
        "ts_utc": ts_utc,
    }


def validate_resume_record(record):
    """Validation fail closed de l'enregistrement de reprise (D2).
    Rejet explicite, jamais de correction silencieuse."""
    import re
    errors = []
    if not isinstance(record, dict):
        return False, ["enregistrement de reprise non dict"]
    for f in RESUME_REQUIRED_FIELDS:
        if f not in record:
            errors.append("champ manquant: " + f)
    if errors:
        return False, errors
    if not isinstance(record["mission"], str) or not record["mission"].strip():
        errors.append("mission absente")
    if not record["issue"].startswith("#"):
        errors.append("issue invalide (prefixe # requis)")
    for f in ("sha_base", "sha_head"):
        v = record[f]
        if f == "sha_head" and v == "":
            continue  # tete non encore publiee : autorisee
        if not re.match(r"^[0-9a-f]{40}$", v or ""):
            errors.append("SHA " + f + " invalide (40 hex)")
    if not isinstance(record["round"], int) or record["round"] < 0:
        errors.append("round invalide")
    if not isinstance(record["curseur_relais"], int) or record["curseur_relais"] < 0:
        errors.append("curseur relais invalide")
    if not isinstance(record["reserves_ouvertes"], list):
        errors.append("reserves ouvertes non list")
    ok_reg, reg_errors = coord06_v13.validate_reserve_registry(record["reserves_ouvertes"])
    if not ok_reg:
        errors.extend(reg_errors)
    if not record["prochaine_action"].strip():
        errors.append("prochaine action absente")
    if not coord06.valid_ts_utc(record["ts_utc"]):
        errors.append("horodatage UTC invalide")
    return len(errors) == 0, errors


def resume_marker(record):
    """Marqueur d'idempotence de publication de l'etat de reprise (regle
    v1.0 §4.2 : jamais de double publication exploitee)."""
    return "RESUME:%s:%s:%d" % (record["mission"], record["tranche"], record["round"])


def persist_resume(record, sinks):
    """Publication idempotente de l'etat de reprise sur les supports durables
    (D2) : support double Knowledge persistante + commentaire d'issue.
    sinks : dict support -> callable(record) -> bool (cree ou non).
    Retourne {support: publie (bool)} ; echec partiel = reserve OUVERTE ;
    GitHub demeure la reference faisant foi.
    """
    marker = resume_marker(record)
    out = {}
    for name, writer in sinks.items():
        if name not in RESUME_SUPPORTS:
            out[name] = False
            continue
        out[name] = bool(writer(record))
    return out


def resume_next_action(record):
    """Reprise automatique : l'action a executer des qu'une session est
    disponible. Il est interdit de pretendre qu'une sandbox arretee continue
    de fonctionner : la reprise repart de l'etat durable (GitHub faisant foi)."""
    ok, errors = validate_resume_record(record)
    if not ok:
        return None, errors
    return record["prochaine_action"], []


# ---- D3 : empreintes automatisees, parite verifiee par calcul ----
def git_blob_sha1(data):
    """Empreinte SHA-1 d'un blob Git (aucune transcription manuelle possible :
    l'empreinte est TOUJOURS calculee par cette fonction)."""
    if isinstance(data, str):
        data = data.encode("utf-8")
    h = hashlib.sha1()
    h.update(b"blob %d\x00" % len(data))
    h.update(data)
    return h.hexdigest()


def verify_parity(expected_manifest, compute_sha):
    """Verification automatique de parite (D3) : compare une liste de
    {chemin, sha1 attendu} aux empreintes calculees par compute_sha(chemin)
    (retourne hex ou None si introuvable). Aucune transcription manuelle :
    les deux cotes sont des empreintes calculees ou publiees.
    Retourne (parite_ok, details {chemin: "ok"/"divergent"/"absent"}).
    """
    details = {}
    ok = True
    for entry in expected_manifest:
        chemin, attendu = entry["chemin"], entry["sha1"]
        actuel = compute_sha(chemin)
        if actuel is None:
            details[chemin] = "absent"
            ok = False
        elif actuel != attendu:
            details[chemin] = "divergent"
            ok = False
        else:
            details[chemin] = "ok"
    return ok, details


def parity_proof(expected_manifest, compute_sha, tool_version, sha_git, ts_utc):
    """Preuve normalisee (C3, etiquette REEL) de la parite verifiee (D3).
    La parite verifiee fait partie des preuves de preflight quand git est
    BLOQUE (plan alternatif C1)."""
    ok, details = verify_parity(expected_manifest, compute_sha)
    divergence = sorted(c for c, s in details.items() if s != "ok")
    sortie = "parity=OK" if ok else "parity=KO divergents=" + ",".join(divergence)
    sortie += " fichiers=" + str(len(details))
    entry = coord06_v13.normalized_execution(
        "verify_parity(%d fichiers)" % len(expected_manifest),
        0 if ok else 1, sortie, tool_version, sha_git, ts_utc, "REEL")
    entry["parity_ok"] = ok
    return entry


# ---- D4 : faisabilite des tests qualifiee a la conception ----
FEASIBILITY_LABELS = ("FEASIBLE_REEL", "FEASIBLE_MOCK", "INFEASIBLE_ALTERNATIVE")


def qualify_test(nom, label, preflight_statut=None, plan_alternatif=""):
    """Qualification d'un test prevu, des la conception (D4)."""
    return {"nom": nom, "label": label,
            "preflight_statut": preflight_statut,
            "plan_alternatif": plan_alternatif}


def validate_feasibility(qualifications):
    """Fail closed : label normalise requis ; FEASIBLE_REEL exige un statut
    de preflight EXECUTABLE ; INFEASIBLE_ALTERNATIVE exige un plan
    alternatif. Un test non qualifie n'est jamais executable ni declare PASS
    (B3 conserve)."""
    errors = []
    if not isinstance(qualifications, list) or not qualifications:
        return False, ["matrice de faisabilite absente"]
    for q in qualifications:
        if not isinstance(q, dict) or q.get("label") not in FEASIBILITY_LABELS:
            errors.append("test sans label de faisabilite normalise: " + str(q.get("nom") if isinstance(q, dict) else q))
            continue
        if q["label"] == "FEASIBLE_REEL" and q.get("preflight_statut") != "EXECUTABLE":
            errors.append("FEASIBLE_REEL sans preflight EXECUTABLE: " + str(q.get("nom")))
        if q["label"] == "INFEASIBLE_ALTERNATIVE" and not q.get("plan_alternatif"):
            errors.append("INFEASIBLE sans plan alternatif: " + str(q.get("nom")))
    return len(errors) == 0, errors


# ---- D5 : dossier de reception rationalise (six sections normees) ----
DOSSIER_SECTIONS = ("mandat", "environnement", "manifeste",
                    "tests_et_preuves", "reserves", "verdict")


def validate_dossier(dossier):
    """Le dossier de reception comporte exactement les six sections normees
    (D5) : mandat, environnement, manifeste, tests_et_preuves, reserves,
    verdict. Les formats existants sont reutilises : la section
    tests_et_preuves contient des enregistrements normalises C3, la section
    reserves le registre C2, la section verdict une ancre C4/D6."""
    errors = []
    if not isinstance(dossier, dict):
        return False, ["dossier non dict"]
    for s in DOSSIER_SECTIONS:
        if s not in dossier or not dossier[s]:
            errors.append("section manquante ou vide: " + s)
    if errors:
        return False, errors
    for e in dossier["tests_et_preuves"]:
        if not coord06_v13.is_normalized_execution(e):
            errors.append("preuve non normalisee (C3)")
    ok_reg, reg_errors = coord06_v13.validate_reserve_registry(dossier["reserves"])
    errors.extend(reg_errors)
    v = dossier["verdict"]
    if not isinstance(v, dict) or "sha_git" not in v or "proof_fp" not in v:
        errors.append("section verdict sans ancre C4")
    return len(errors) == 0, errors


# ---- D6 : reception differentielle liée au SHA precedent ----
def differential_anchor(sha_prev, sha_cur, proofs, validated_criteria):
    """Ancre differentielle complete (D6) : SHA precedent, SHA courant,
    empreinte des preuves et liste des criteres valides lors de la reception
    precedente, chacun avec les fichiers auxquels il s'applique.
    validated_criteria : [{"critere": str, "fichiers": [str, ...]}, ...]
    """
    return coord06_v13.reception_anchor(sha_cur, proofs) | {
        "sha_prev": sha_prev,
        "validated_criteria": [
            {"critere": c["critere"], "fichiers": list(c["fichiers"])}
            for c in validated_criteria
        ],
    }


def is_stale_differential(anchor, sha_prev, sha_cur, proofs):
    """Un verdict rendu sur une ancre D6 est perime des que le SHA precedent,
    le SHA courant ou l'empreinte des preuves change (extension C4)."""
    cur = coord06_v13.reception_anchor(sha_cur, proofs)
    return (anchor.get("sha_prev") != sha_prev
            or anchor.get("sha_git") != cur["sha_git"]
            or anchor.get("proof_fp") != cur["proof_fp"])


def conserve_criteria(anchor, changed_files):
    """Regles de conservation differentielle (D6) : un critere valide lors de
    la reception precedente est conserve seulement si TOUS ses fichiers sont
    inchanges ; sinon il est reevalue. Un critere absent de l'ancre est
    reevalue (jamais presume valide).
    Retourne (conserves, a_reevaluer).
    """
    conserves, reevaluate = [], []
    known = set(changed_files)
    for c in anchor.get("validated_criteria", []):
        fichiers = c.get("fichiers", [])
        if all(f not in known for f in fichiers):
            conserves.append(c["critere"])
        else:
            reevaluate.append(c["critere"])
    return conserves, reevaluate


def reexamen_differential(anchor, corrections_demandees, changed_files):
    """Reexamen explicite obligatoire de toute reception apres correction
    (D6) : (a) corrections demandees appliquees ; (b) impacts collateraux du
    diff ; (c) criteres valides conserves seulement si inchanges.
    Retourne un dict de constats ; la reevaluation proportionnee C4
    (restreinte/completee, coord06_v13.reevaluation_scope) est conservee."""
    portee, _fichiers_concernes = coord06_v13.reevaluation_scope(
        corrections_demandees, changed_files)
    conserves, reevalues = conserve_criteria(anchor, changed_files)
    appliquees = [c for c in corrections_demandees if c in changed_files]
    collateraux = sorted(set(changed_files) - set(corrections_demandees))
    return {
        "corrections_appliquees": len(appliquees) == len(corrections_demandees) and len(corrections_demandees) > 0,
        "corrections_manquantes": [c for c in corrections_demandees if c not in changed_files],
        "impacts_collateraux": collateraux,
        "criteres_conserves": conserves,
        "criteres_a_reevaluer": reevalues,
        "portee_reevaluation": portee,
    }


# ---- D7 : mesures de coordination nommees avec unites ----
METRIC_UNITS = {
    "duree_active_s": "secondes",
    "rounds": "compteur_par_phase",
    "reinitialisations": "compteur",
    "reconstructions": "fichiers_verifies",
    "appels_api": "compteur_par_type",
    "messages_publies": "compteur_par_marqueur",
    "sollicitations_pilote": "compteur",
    "interventions_humaines": "compteur_avec_nature",
}


def coordination_metrics(duree_active_s, rounds, reinitialisations,
                         reconstructions, appels_api, messages_publies,
                         sollicitations_pilote, interventions_humaines):
    """Mesures normalisees D7 : chacune nommee avec son unite, comptee par
    exécution, jamais inventee ni simulee."""
    return {
        "duree_active_s": duree_active_s,
        "rounds": rounds,
        "reinitialisations": reinitialisations,
        "reconstructions": reconstructions,
        "appels_api": appels_api,
        "messages_publies": messages_publies,
        "sollicitations_pilote": sollicitations_pilote,
        "interventions_humaines": interventions_humaines,
    }


def validate_metrics(metrics):
    """Fail closed : les huit mesures nommees sont requises, numeriques ou
    dicts de compteurs, jamais negatives ; toute mesure absente est une
    erreur (jamais presumee nulle)."""
    errors = []
    if not isinstance(metrics, dict):
        return False, ["mesures non dict"]
    for k in METRIC_UNITS:
        if k not in metrics:
            errors.append("mesure absente: " + k)
    if errors:
        return False, errors
    if not isinstance(metrics["duree_active_s"], (int, float)) or metrics["duree_active_s"] < 0:
        errors.append("duree active invalide")
    for k in ("reinitialisations", "sollicitations_pilote"):
        if not isinstance(metrics[k], int) or metrics[k] < 0:
            errors.append("mesure invalide: " + k)
    for k in ("rounds", "reconstructions", "appels_api", "messages_publies"):
        v = metrics[k]
        if not isinstance(v, dict):
            errors.append("mesure non compteur: " + k)
            continue
        for kk, vv in v.items():
            if not isinstance(vv, int) or vv < 0:
                errors.append("compteur invalide: %s.%s" % (k, kk))
    ih = metrics["interventions_humaines"]
    if (not isinstance(ih, dict) or not isinstance(ih.get("nombre"), int)
            or ih["nombre"] < 0 or not isinstance(ih.get("nature", []), list)):
        errors.append("mesure invalide: interventions_humaines")
    return len(errors) == 0, errors


# ---- Enregistrement v1.4 (delegation stricte, non active) ----
def validate_record_v14(record):
    """Etend la validation v1.3 par delegation stricte : la validation v1.3
    est appliquee sur copie portant la version v1.3 (aucun rejet indu par
    l'egalite de version), la version v1.4 etant controlee separement.
    Champs v1.4 requis : resume (D2), faisabilite (D4), ancre differentielle
    (D6), mesures (D7), continuite (D1). Ne modifie RIEN des modules
    anterieurs."""
    if not isinstance(record, dict):
        return False, ["enregistrement non dict"]
    errors = []
    if record.get("protocol") != PROTOCOL_VERSION_V14:
        errors.append("version v1.4 attendue")
    tmp = dict(record)
    tmp["protocol"] = coord06_v13.PROTOCOL_VERSION_V13
    ok13, errors13 = coord06_v13.validate_record_v13(tmp)
    if not ok13:
        errors.extend(errors13)
    ok_resume, resume_errors = validate_resume_record(record.get("resume", {}))
    if not ok_resume:
        errors.extend("resume: " + e for e in resume_errors)
    ok_feas, feas_errors = validate_feasibility(record.get("faisabilite", []))
    if not ok_feas:
        errors.extend("faisabilite: " + e for e in feas_errors)
    anchor = record.get("ancre_differentielle", {})
    if not isinstance(anchor, dict) or "sha_prev" not in anchor or "validated_criteria" not in anchor:
        errors.append("ancre differentielle absente ou incomplete")
    ok_metrics, metrics_errors = validate_metrics(record.get("mesures", {}))
    if not ok_metrics:
        errors.extend("mesures: " + e for e in metrics_errors)
    cont = record.get("continuite", {})
    if not isinstance(cont, dict) or cont.get("phase") not in PHASES:
        errors.append("etat de continuite absent ou phase inconnue")
    if isinstance(cont, dict) and not isinstance(cont.get("round", -1), int):
        errors.append("round de continuite invalide")
    return len(errors) == 0, errors


def record_v14_from(mission, issue, sha_git, proofs, model_api, ts_utc,
                    risk_level, executed_proofs, api_calls,
                    executions_normalisees, reserve_registry, stop_criterion,
                    resume, faisabilite, ancre_differentielle, mesures,
                    continuite, state=None, verdict_url="", verdict_id="",
                    ack=False):
    """Construit un enregistrement v1.4 complet par delegation stricte
    v1.3 (elle-meme delegante v1.2 puis v1.1)."""
    rec = coord06_v13.record_v13_from(
        mission, issue, sha_git, proofs, model_api, ts_utc,
        risk_level, executed_proofs, api_calls,
        executions_normalisees, reserve_registry, stop_criterion,
        state=state, verdict_url=verdict_url, verdict_id=verdict_id, ack=ack)
    rec["protocol"] = PROTOCOL_VERSION_V14
    rec["resume"] = resume
    rec["faisabilite"] = faisabilite
    rec["ancre_differentielle"] = ancre_differentielle
    rec["mesures"] = mesures
    rec["continuite"] = continuite
    return rec

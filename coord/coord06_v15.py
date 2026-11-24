#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Coord06 v1.5 (CANDIDATURE, NON ACTIVEE) — durcissement et conformite independante.

Mission M-COORD-HARDENING-01 (issue #19), plan v2 accepte par critique
préalable OpenAI (verdict PLAN_ACCEPTE, session m-coord-hardening-01).

Module d'extension par delegation stricte : importe coord06 (v1.1) et
coord06_v14 (v1.4, reference operationnelle) SANS les modifier, et ajoute :

Rules H1..H6 (hardening, constats d'audit reproduits dans #19) :
- H1 identites qualifiees a la frontiere des adaptateurs, habilitations
  configurables fail closed, separation executant/receptionnaire :
  l'auto-reception est refusee par le moteur ; aucun fournisseur code en dur ;
- H2 machine a etats v1.5 exhaustive et fermee : chaque transition exige ses
  donnees (verdict normalise, reference de verdict, identite habilitee) ;
  etats ambigus supprimes ; aucune sortie terminale sans verdict reference ;
- H3 ancrage a la revision et a l'empreinte des preuves : obsolescence par
  empreinte (plus de compteur), invalidation du verdict perime ;
- H4 persistance atomique (temporaire + os.replace + fsync) avec points
  d'arret normalises et echec sans perte du contenu precedent ;
- H5 reprise deterministe avec arbitrage des supports (divergence :
  github fait foi), journal de conflits normalise exploitable post-mortem ;
- H6 compatibilite : correspondance etats Python/Go documentee dans la spec
  (annexe), moteur inchange cote Go.

Regles C7..C9 (controle de coherence du contexte, deja definies par la
candidature v1.5 de l'issue #160, reprises ici sans modification
semantique) : qualification des instructions par rapport a l'ordre de
mission et a la provenance declaree, journalisation idempotente des ecarts,
reprise sans interruption.

Le moteur n'effectue AUCUNE authentification : une identite est qualifiee par
son adaptateur a la frontiere ; le moteur refuse toute identite dont
l'adaptateur n'est pas enregistre dans la configuration. Une identite
declaree hors adaptateur enregistre n'est jamais presentee comme authentifiee.

Python stdlib uniquement. Aucun secret. Aucun E2E reel dans ce module.
"""
import hashlib
import json
import os
import re

import coord06
import coord06_v14

PROTOCOL_VERSION_V15 = "M-COORD-06 v1.5 (candidate, non activee)"


# ---- H1 : identites, roles, habilitations (fail closed) ----
ROLES = ("EXECUTANT", "RECEPTIONNAIRE", "PILOTE")
ACTIONS = ("executer", "soumettre", "recevoir", "acker", "cloturer")


def valider_habilitations(config):
    """Validation fail closed de la configuration des habilitations (H1).
    Schema : {"adaptateurs": [id...], "roles": {role: [action...]}}.
    - liste d'adaptateurs vide ou absente : aucune identite ne peut etre
      qualifiee (tout est refuse), mais la validation ne placera jamais
      un identifiant d'adaptateur inconnu ;
    - role inconnu, action inconnue, joker "*", doublon : erreur ;
    - une configuration valide impliquant des refus est legale
      (liste de roles vide = personne n'est habilite).
    Retourne (ok, erreurs)."""
    erreurs = []
    if not isinstance(config, dict):
        return False, ["configuration non dict"]
    adaptateurs = config.get("adaptateurs")
    if not isinstance(adaptateurs, list) or not all(isinstance(a, str) and a for a in adaptateurs):
        erreurs.append("adaptateurs : liste d'identifiants non vides attendue")
    elif len(set(adaptateurs)) != len(adaptateurs):
        erreurs.append("adaptateurs : doublon interdit")
    roles = config.get("roles", {})
    if not isinstance(roles, dict):
        return False, erreurs + ["roles non dict"]
    for role, actions in roles.items():
        if role not in ROLES:
            erreurs.append("role inconnu: " + str(role))
            continue
        if not isinstance(actions, list):
            erreurs.append("actions non list pour le role: " + str(role))
            continue
        if "*" in actions:
            erreurs.append("joker interdit dans les actions du role: " + str(role))
        for a in actions:
            if a not in ACTIONS:
                erreurs.append("action inconnue: " + str(a))
        if len(set(actions)) != len(actions):
            erreurs.append("actions en doublon pour le role: " + str(role))
    return len(erreurs) == 0, erreurs


def identite_qualifiee(identite, config):
    """Une identite est qualifiee si et seulement si son adaptateur
    d'emission est enregistre dans la configuration (H1). Le moteur ne
    verifie RIEN d'autre : la verification du canal releve de l'adaptateur."""
    if not isinstance(identite, dict):
        return False
    for champ in ("id", "role", "qualifiee_par"):
        if not isinstance(identite.get(champ), str) or not identite[champ].strip():
            return False
    if identite["role"] not in ROLES:
        return False
    return identite["qualifiee_par"] in (config.get("adaptateurs") or [])


def habilite(identite, action, config, erreurs_config=None):
    """Habilitation fail closed (H1) : identite qualifiee par un adaptateur
    enregistre, role connu, action autorisee pour ce role. Toute
    defaillance de configuration (y compris rejetee par valider_habilitations)
    doit conduire l'appelant a refuser : habilite retourne False si les
    erreurs de configuration sont fournies non vides."""
    if erreurs_config is None:
        _ok, erreurs_config = valider_habilitations(config)
    if erreurs_config:
        return False
    if not identite_qualifiee(identite, config):
        return False
    actions_role = (config.get("roles") or {}).get(identite["role"], [])
    return action in actions_role


# ---- H2 : machine a etats v1.5 exhaustive et fermee ----
ETATS = ("PLANIFIEE", "EXECUTION", "DECISION_ATTENDUE", "ACCEPTEE",
         "EN_CORRECTION", "ACK_RECU", "REFUSEE", "CLOTUREE")
TERMINEE = ("REFUSEE", "CLOTUREE")
VERDICTS = ("ACCEPTEE", "A_CORRIGER", "REFUSEE")

# Cible attendue, verdict correspondant
VERDICT_PAR_CIBLE = {"ACCEPTEE": "ACCEPTEE", "REFUSEE": "REFUSEE",
                     "EN_CORRECTION": "A_CORRIGER"}
CIBLES_VERDICT = tuple(VERDICT_PAR_CIBLE)

# Table exhaustive : (etat, cible) -> requis {role, action, donnees}.
# Toute paire absente est interdite (table fermee, H2).
TABLE_TRANSITIONS = {
    ("PLANIFIEE", "EXECUTION"): {"role": "EXECUTANT", "action": "executer",
                                 "donnees": ("identite_stable",)},
    ("EXECUTION", "DECISION_ATTENDUE"): {"role": "EXECUTANT", "action": "soumettre",
                                         "donnees": ("revision", "empreinte_preuves")},
    ("DECISION_ATTENDUE", "ACCEPTEE"): {"role": "RECEPTIONNAIRE", "action": "recevoir",
                                        "donnees": ("verdict_id", "verdict_url")},
    ("DECISION_ATTENDUE", "REFUSEE"): {"role": "RECEPTIONNAIRE", "action": "recevoir",
                                       "donnees": ("verdict_id", "verdict_url")},
    ("DECISION_ATTENDUE", "EN_CORRECTION"): {"role": "RECEPTIONNAIRE", "action": "recevoir",
                                             "donnees": ("verdict_id", "verdict_url")},
    ("EN_CORRECTION", "EXECUTION"): {"role": "EXECUTANT", "action": "executer",
                                     "donnees": ("identite_stable",)},
    ("ACCEPTEE", "ACK_RECU"): {"role": "EXECUTANT", "action": "acker",
                               "donnees": ("revision_courante", "empreinte_courante")},
    ("ACK_RECU", "CLOTUREE"): {"role": "PILOTE", "action": "cloturer", "donnees": ()},
}

_REVISION_RE = re.compile(r"^[0-9a-f]{40}$")
_EMPREINTE_RE = re.compile(r"^[0-9a-f]{64}$")


class MissionDurcie:
    """Machine a etats v1.5 durcie (H2/H3/H1). Semantique :

    - PLANIFIEE -> EXECUTION : l'EXECUTANT demarre (liaison stable de
      l'identite executante a la mission) ;
    - EXECUTION -> DECISION_ATTENDUE : l'EXECUTANT soumet (revision + empreinte) ;
    - DECISION_ATTENDUE -> ACCEPTEE/REFUSEE/EN_CORRECTION : un
      RECEPTIONNAIRE DIFFERENT de l'executant rend le verdict (reference
      de verdict obligatoire) ; auto-reception interdite ;
    - EN_CORRECTION -> EXECUTION : reprise par le MEME executant ;
    - ACCEPTEE -> ACK_RECU : l'executant accuse reception si et seulement
      si l'empreinte et la revision n'ont pas change depuis la decision (H3,
      garde par empreinte, jamais par compteur) ;
    - ACK_RECU -> CLOTUREE : SEUL le PILOTE cloture.
    Les etats REFUSEE et CLOTUREE sont terminaux, toute suite est refusee.
    """

    def __init__(self, issue, revision=None):
        self.etat = "PLANIFIEE"
        self.issue = issue
        self.executant = None      # identite (dict) liee a la mission
        self.en_attente = None     # {"revision", "empreinte_preuves"} soumis
        self.decisions = []        # {"verdict","revision","empreinte","recepteur","verdict_id","verdict_url"}

    def transition(self, cible, identite, config, **donnees):
        """Applique une transition fermee. Retourne (True, "") ou (False, motif).
        Aucune transition ne peut aboutir sans identite habilitee ; les
        erreurs de configuration ferment tout (fail closed)."""
        ok_cfg, erreurs_cfg = valider_habilitations(config)
        if not ok_cfg:
            return False, "configuration d'habilitations invalide: " + "; ".join(erreurs_cfg[:3])
        if self.etat in TERMINEE:
            return False, "mission terminee (etat " + self.etat + ")"
        requis = TABLE_TRANSITIONS.get((self.etat, cible))
        if requis is None:
            return False, "transition interdite: " + self.etat + " -> " + str(cible)
        if identite.get("role") != requis["role"]:
            return False, "role non autorise pour cette transition (attendu: " + requis["role"] + ")"
        if not habilite(identite, requis["action"], config, erreurs_cfg):
            return False, "identite non habilitee pour l'action: " + requis["action"]
        if cible == "EXECUTION":
            if self.etat == "PLANIFIEE":
                self.executant = dict(identite)
                self.etat = "EXECUTION"
                return True, ""
            # EN_CORRECTION -> EXECUTION : meme executant uniquement
            if self.executant is None or identite["id"] != self.executant["id"]:
                return False, "identite executante non stabilisee sur la mission"
            self.etat = "EXECUTION"
            return True, ""
        if cible == "DECISION_ATTENDUE":
            revision = donnees.get("revision", "")
            empreinte = donnees.get("empreinte_preuves", "")
            if not _REVISION_RE.match(revision or ""):
                return False, "revision invalide (40 hex attendus)"
            if not _EMPREINTE_RE.match(empreinte or ""):
                return False, "empreinte de preuves invalide (sha256, 64 hex)"
            self.en_attente = {"revision": revision, "empreinte_preuves": empreinte}
            self.etat = "DECISION_ATTENDUE"
            return True, ""
        if cible in CIBLES_VERDICT:
            if self.en_attente is None:
                return False, "aucune soumission en attente"
            if self.executant is not None and identite["id"] == self.executant["id"]:
                return False, "auto-reception interdite (executant = recepteur)"
            verdict_attendu = VERDICT_PAR_CIBLE[cible]
            verdict_id = donnees.get("verdict_id", "")
            verdict_url = donnees.get("verdict_url", "")
            if not verdict_id or not verdict_url:
                return False, "reference de verdict obligatoire (verdict_id + verdict_url)"
            verdict = donnees.get("verdict", "")
            if verdict != verdict_attendu:
                return False, "verdict incoherent avec la cible (attendu: " + verdict_attendu + ")"
            self.decisions.append({
                "verdict": verdict_attendu,
                "revision": self.en_attente["revision"],
                "empreinte": self.en_attente["empreinte_preuves"],
                "recepteur": identite["id"],
                "verdict_id": verdict_id,
                "verdict_url": verdict_url,
            })
            self.en_attente = None
            self.etat = cible
            return True, ""
        if cible == "ACK_RECU":
            if not self.decisions:
                return False, "ACK sans decision"
            decision = self.decisions[-1]
            revision = donnees.get("revision_courante", "")
            empreinte = donnees.get("empreinte_courante", "")
            if empreinte != decision["empreinte"] or revision != decision["revision"]:
                return False, "verdict perime (empreinte ou revision divergentes)"
            self.etat = "ACK_RECU"
            return True, ""
        if cible == "CLOTUREE":
            self.etat = "CLOTUREE"
            return True, ""
        return False, "cible inconnue"


def verdict_perime(decision, empreinte_courante, revision_courante):
    """H3 : un verdict est perime des que l'empreinte des preuves ou la
    revision divergent de celles presentees au moment de la decision.
    Garde par EMPREINTE (jamais par compteur de preuves)."""
    return (decision.get("empreinte") != empreinte_courante
            or decision.get("revision") != revision_courante)


# ---- H4 : persistance atomique et durabilite qualifiee ----
class CrashSimule(Exception):
    """Panne injectee pour les tests de durabilite aux points d'arret."""


POINTS_ARRET = ("ecriture", "avant_remplacement", "apres_remplacement")


def save_record_durci(record, path, crash_a=None):
    """Persistance atomique (H4) :
    1. ecriture complete d'un fichier temporaire (path + ".tmp") ;
    2. fsync du fichier temporaire ;
    3. renommage atomique (os.replace) ;
    4. fsync du repertoire.
    Points d'arret normalises (pour pannes injectees, tests/demonstration) :
    - "ecriture" : le fichier temporaire est laisse tronque, l'original
      est INTACT ; l'exception CrashSimule est levee ;
    - "avant_remplacement" : le temporaire complet existe, l'original est
      INTACT ; CrashSimule est levee ;
    - "apres_remplacement" : le nouveau contenu EST EN PLACE ;
      CrashSimule est levee (crash de la derniere microseconde).
    En usage reel (crash_a=None), aucune injection.
    """
    if crash_a is not None and crash_a not in POINTS_ARRET:
        raise ValueError("point d'arret inconnu: " + str(crash_a))
    tmp = path + ".tmp"
    contenu = coord06.canonical_json(record)
    with open(tmp, "w", encoding="utf-8") as f:
        if crash_a == "ecriture":
            f.write(contenu[:len(contenu) // 2])
            f.flush()
            os.fsync(f.fileno())
            raise CrashSimule("crash pendant l'ecriture du temporaire")
        f.write(contenu)
        f.flush()
        os.fsync(f.fileno())
    if crash_a == "avant_remplacement":
        raise CrashSimule("crash avant le renommage atomique")
    os.replace(tmp, path)
    try:
        dir_fd = os.open(os.path.dirname(os.path.abspath(path)) or ".", os.O_RDONLY)
    except OSError:
        dir_fd = None
    try:
        if dir_fd is not None:
            os.fsync(dir_fd)
    finally:
        if dir_fd is not None:
            os.close(dir_fd)
    if crash_a == "apres_remplacement":
        raise CrashSimule("crash apres le renommage atomique")


def load_record_durci(path):
    """Relecture pour reprise : ignore le temporaire orphelin d'un crash
    anterieur (H4), echoue proprement (None) sur contenu illisible."""
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return None


def nettoyer_temporaire(path):
    """Nettoyage du temporaire orphelin après un crash (idempotent)."""
    tmp2 = path + ".tmp"
    if os.path.exists(tmp2):
        os.remove(tmp2)
        return True
    return False


# ---- H5 : reprise deterministe avec arbitrage des supports ----
REGLE_ARBITRAGE = "github_fait_foi"


def _empreinte_contenu(record):
    return hashlib.sha256(coord06.canonical_json(record).encode("utf-8")).hexdigest()


def champs_divergents(a, b):
    """Champs de premier niveau dont la forme canonique diverge entre deux
    enregistrements de reprise."""
    cles = sorted(set(list(a.keys()) + list(b.keys())))
    return [c for c in cles if coord06.canonical_json(a.get(c)) != coord06.canonical_json(b.get(c))]


def arbitrer_reprise(record_knowledge, record_github, ts_utc, arbitre="moteur-v1.5"):
    """Arbitrage des supports divergents (H5) : github fait foi.
    Retourne un enregistrement normalise de decision d'arbitrage :
      {"elu": "github"|"knowledge"|"supports_concordants",
       "record": <elu complet>,
       "regle": ..., "champs_divergents": [...],
       "empreintes": {"knowledge": sha256, "github": sha256},
       "ts_utc": ..., "arbitre": ...}
    L'enregistrement est suffisant pour une analyse post-mortem : les
    empreintes des DEUX versions integrales y figurent."""
    divergents = champs_divergents(record_knowledge, record_github)
    entry = {
        "elu": "github" if divergents else "supports_concordants",
        "record": record_github,
        "regle": REGLE_ARBITRAGE,
        "champs_divergents": divergents,
        "empreintes": {
            "knowledge": _empreinte_contenu(record_knowledge),
            "github": _empreinte_contenu(record_github),
        },
        "ts_utc": ts_utc,
        "arbitre": arbitre,
    }
    return entry


def journal_conflits(entries, journal_path):
    """Journalisation idempotente des conflits (H5), JSONL canonique
    (coord06.canonical_json), idempotente : une entree identique deja
    presente n'est pas dupliquee. Retourne le nombre de lignes ajoutees."""
    ajoutes = 0
    existants = set()
    if os.path.exists(journal_path):
        with open(journal_path, "r", encoding="utf-8") as f:
            for ligne in f:
                ligne = ligne.strip()
                if ligne:
                    existants.add(ligne)
    with open(journal_path, "a", encoding="utf-8") as f:
        for e in entries:
            ligne = coord06.canonical_json(e)
            if ligne not in existants:
                                f.write(ligne + "\\n")
                existants.add(ligne)
                ajoutes += 1
    return ajoutes


def reprise_apres_arbitrage(entry, sha_head_attendu):
    """Reprise deterministe (H5) : apres arbitrage, la prochaine action n'est
    executee que si la tete verifiee correspond a celle attendue (verif de
    l'etat du monde). Retourne (action, erreurs)."""
    record = entry.get("record")
    if not isinstance(record, dict):
        return None, ["enregistrement arbitre absent"]
    ok, erreurs = coord06_v14.validate_resume_record(record)
    if not ok:
        return None, ["reprise: " + e for e in erreurs]
    sha_head = record.get("sha_head", "")
    if sha_head and sha_head != sha_head_attendu:
        return None, ["sha_head divergent : reprise impossible sans re-verification du monde"]
    return record.get("prochaine_action"), []


# ---- C7..C9 : controle de coherence du contexte (consolidation issue #160) ----
ETAPES_MISSION = ("critique", "execution", "reception", "ack", "cloture")


def _mots(texte):
    """Normalisation minimale : minuscules, sans ponctuation ni accents."""
    texte = texte.lower()
    for a, b in (("é", "e"), ("è", "e"), ("ê", "e"), ("ë", "e"),
                 ("à", "a"), ("â", "a"), ("ä", "a"), ("î", "i"), ("ï", "i"),
                 ("ô", "o"), ("ö", "o"), ("û", "u"), ("ù", "u"), ("ü", "u"),
                 ("ç", "c")):
        texte = texte.replace(a, b)
    return [m for m in re.split(r"[^a-z0-9]+", texte) if m]


def coherence_score(instruction, termes_mission):
    """C7 : proportion de termes de mission presents dans l'instruction
    (termes issus exclusivement de l'ordre de mission publie)."""
    if not termes_mission:
        return 0.0
    mots = set(_mots(instruction))
    couverts = 0
    for terme in termes_mission:
        mots_terme = set(_mots(terme))
        if mots_terme and mots_terme <= mots:
            couverts += 1
    return couverts / len(termes_mission)


def classify_context(instructions, termes_mission):
    """C7 : qualification unitaire COHERENTE / ECART_ISOLE, une instruction
    sans provenance declaree etant qualifiee ECART_ISOLE."""
    resultat = []
    for item in instructions:
        if isinstance(item, dict):
            instruction = item.get("instruction", "")
            provenance = item.get("provenance", "")
        else:
            instruction, provenance = item, ""
        score = coherence_score(instruction, termes_mission)
        coherente = score >= 0.5 and provenance != ""
        resultat.append({
            "instruction": instruction,
            "qualification": "COHERENTE" if coherente else "ECART_ISOLE",
            "score": round(score, 4),
            "provenance": provenance,
        })
    return resultat


def journal_ecarts(ecarts, journal_path):
    """C8 : journalisation idempotente des ecarts, JSONL canonique, empreinte
    de l'instruction calculee (jamais transcrite)."""
    ajoutes = 0
    existants = set()
    if os.path.exists(journal_path):
        with open(journal_path, "r", encoding="utf-8") as f:
            for ligne in f:
                ligne = ligne.strip()
                if ligne:
                    existants.add(ligne)
    with open(journal_path, "a", encoding="utf-8") as f:
        for e in ecarts:
            entree = {
                "ts_utc": e.get("ts_utc"),
                "instruction_hash": coord06.proof_fingerprint([e.get("instruction", "")]),
                "qualification": e.get("qualification", "ECART_ISOLE"),
                "source_alleguee": e.get("source_alleguee", "inconnue"),
                "decision": e.get("decision",
                                  "instruction ecartee, mission non interrompue"),
                "etape": e.get("etape", "execution"),
            }
            ligne = coord06.canonical_json(entree)
            if ligne not in existants:
                                f.write(ligne + "\\n")
                existants.add(ligne)
                ajoutes += 1
    return ajoutes


def reprise_apres_ecart(qualifications, etape_courante):
    """C9 : reprise automatique a l'etape concernee, aucune interruption.
    Un ecart ne peut jamais produire de decision."""
    etape = etape_courante if etape_courante in ETAPES_MISSION else "execution"
    ecarts = sum(1 for q in qualifications if q.get("qualification") != "COHERENTE")
    return {
        "interruption": False,
        "etape_reprise": etape,
        "ecarts_isoles": ecarts,
        "decisions_par_ecart": 0,
    }


# ---- Enregistrement v1.5 (delegation stricte v1.4, non activee) ----
def validate_record_v15(record):
    """Etend la validation v1.4 par delegation stricte, puis verifie :
    - le bloc durcissement (H1/H5) : identite executante, receptionnaires,
      regle d'arbitrage github_fait_foi ;
    - la reference de verdict OBLIGATOIRE pour tout etat de decision
      (en particulier REFUSEE : plus de refus sans verdict, C2 corrige) ;
    - l'empreinte de decision pour les etats post-decision (H3) ;
    - le bloc controle_contexte (C7..C9).
    Fail closed, messages d'erreur jamais masques."""
    if not isinstance(record, dict):
        return False, ["enregistrement non dict"]
    erreurs = []
    if record.get("protocol") != PROTOCOL_VERSION_V15:
        erreurs.append("version v1.5 attendue")
    tmp = dict(record)
    tmp["protocol"] = coord06_v14.PROTOCOL_VERSION_V14
    ok14, erreurs14 = coord06_v14.validate_record_v14(tmp)
    if not ok14:
        erreurs.extend(erreurs14)
    dur = record.get("durcissement")
    if not isinstance(dur, dict):
        return False, erreurs + ["bloc durcissement manquant"]
    if not isinstance(dur.get("executant"), str) or not dur.get("executant").strip():
        erreurs.append("identite executante absente (H1)")
    recpts = dur.get("receptionnaires")
    if not isinstance(recpts, list) or not recpts:
        erreurs.append("receptionnaires absents (liste exigee, ne fut-elle remplise qu'au fil des verdicts)")
    if dur.get("regle_arbitrage") != REGLE_ARBITRAGE:
        erreurs.append("regle d'arbitrage invalide (github_fait_foi attendu)")
    etat = record.get("state", "")
    etats_decision = ("DECISION_API", "ACK_MISTRAL", "REFUSEE")
    if etat in etats_decision or record.get("ack"):
        if not record.get("verdict_id") or not record.get("verdict_url"):
            erreurs.append("reference de verdict obligatoire pour l'etat " + str(etat) + " (H2)")
    empreinte_decision = dur.get("empreinte_decision", "")
    if etat in ("ACK_MISTRAL",) and not _EMPREINTE_RE.match(empreinte_decision or ""):
        erreurs.append("empreinte de decision absente pour un etat post-decision (H3)")
    cc = record.get("controle_contexte")
    if not isinstance(cc, dict):
        return False, erreurs + ["controle_contexte manquant"]
    if not isinstance(cc.get("instructions_qualifiees"), int):
        erreurs.append("instructions_qualifiees doit etre un entier")
    if not isinstance(cc.get("ecarts_journalises"), int):
        erreurs.append("ecarts_journalises doit etre un entier")
    if cc.get("interruptions") != 0:
        erreurs.append("interruptions doit valoir 0 (regle C9)")
    if cc.get("decisions_par_ecart") != 0:
        erreurs.append("decisions_par_ecart doit valoir 0 (regle C7)")
    return len(erreurs) == 0 and ok14, erreurs


def record_v15_from(mission, issue, sha_git, proofs, model_api, ts_utc,
                    risk_level, executed_proofs, api_calls,
                    executions_normalisees, reserve_registry, stop_criterion,
                    resume, faisabilite, ancre_differentielle, mesures,
                    continuite, durcissement, controle_contexte, state=None,
                    verdict_url="", verdict_id="", ack=False):
    """Construit un enregistrement v1.5 complet par delegation stricte v1.4
    (elle-meme delegante v1.3 -> v1.2 -> v1.1)."""
    rec = coord06_v14.record_v14_from(
        mission, issue, sha_git, proofs, model_api, ts_utc,
        risk_level, executed_proofs, api_calls,
        executions_normalisees, reserve_registry, stop_criterion,
        resume, faisabilite, ancre_differentielle, mesures, continuite,
        state=state, verdict_url=verdict_url, verdict_id=verdict_id, ack=ack)
    rec["protocol"] = PROTOCOL_VERSION_V15
    rec["durcissement"] = durcissement
    rec["controle_contexte"] = controle_contexte
    return rec

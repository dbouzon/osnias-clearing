#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Suite de robustesse M-COORD-13-T3 — module canonique coord06.py (v1.1 en vigueur).

Petite mission réelle non critique (essai COORD v1.3, issue #150, T3) :
tests de robustesse adversariaux sur les empreintes de preuves
(proof_fingerprint, canonical_json) et la validation des enregistrements
(validate_record, record_from). Aucun fichier existant n'est modifié ;
la suite est additive et reproductible (Python stdlib uniquement).
Tests UNITAIRES — étiquette REEL lorsqu'ils sont exécutés dans la session
courante, conformément au format normé v1.3 (C3)."""
import os
import sys
import unicodedata

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coord06

PASS, FAIL = [], []


def check(name, cond, detail=""):
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" - " + detail) if detail and not cond else ""))


SHA = "42758576d37b2dfdec063df86396809afc5fa93e"
TS = "2026-11-23T21:30:00Z"

# ---- RB1 : ordre des preuves borne l'empreinte (pas de collision d'ordre) ----
fp_ab = coord06.proof_fingerprint(["a", "b"])
fp_ba = coord06.proof_fingerprint(["b", "a"])
check("RB1 l'ordre des preuves est materiel : permuter deux preuves change l'empreinte (anti-collision d'ordre)",
      fp_ab != fp_ba and len(fp_ab) == 64 and len(fp_ba) == 64)

# ---- RB2 : empreinte insensible a la representation des cles ----
p1 = {"command": "go test ./...", "exit_code": 0, "output": "ok"}
p2 = {"output": "ok", "exit_code": 0, "command": "go test ./..."}
check("RB2 empreinte insensible a l'ordre des cles (serialisation canonique triee)",
      coord06.proof_fingerprint([p1]) == coord06.proof_fingerprint([p2]))

# ---- RB3 : unicite / preuve en double et chaine unique ----
fp_str = coord06.proof_fingerprint("preuve-seule")
fp_list = coord06.proof_fingerprint(["preuve-seule"])
fp_dup = coord06.proof_fingerprint(["preuve-seule", "preuve-seule"])
check("RB3 une chaine et sa liste unitaire donnent la meme empreinte ; un doublon change l'empreinte",
      fp_str == fp_list and fp_dup != fp_str)

# ---- RB4 : unicode et normalisation ----
nfc = unicodedata.normalize("NFC", "preuve exécutée")
nfd = unicodedata.normalize("NFD", "preuve exécutée")
fp_nfc = coord06.proof_fingerprint([nfc])
fp_nfd = coord06.proof_fingerprint([nfd])
check("RB4 l'empreinte distingue NFC et NFD (aucune normalisation implicite silencieuse) et reste stable sur re-calcul",
      fp_nfc != fp_nfd and coord06.proof_fingerprint([nfc]) == fp_nfc)

# ---- RB5 : valeur extreme (chaine vide, longue sortie) ----
fp_empty = coord06.proof_fingerprint([])
fp_emptystr = coord06.proof_fingerprint([""])
long_out = "x" * 100000
check("RB5 cas limites : liste vide et chaine vide distinctes, grande sortie deterministe",
      fp_empty != fp_emptystr
      and coord06.proof_fingerprint([long_out]) == coord06.proof_fingerprint([long_out]))

# ---- RB6 : falsification d'un caractere detectee ----
base = coord06.record_from("M-ROB", "#150", SHA, ["rapport-1"], "gpt-4o", TS)
fp_base = base["proof_fp"]
tampered = coord06.record_from("M-ROB", "#150", SHA, ["rapport-2"], "gpt-4o", TS)
check("RB6 falsification d'un caractere d'une preuve change l'empreinte (verdict perime)",
      fp_base != tampered["proof_fp"]
      and coord06.is_stale_verdict(fp_base, tampered["proof_fp"]))

# ---- RB7 : validation — extremes rejetes, pas de correction silencieuse ----
def v(**over):
    rec = dict(base)
    rec.update(over)
    return coord06.validate_record(rec)

ok_base, _ = v()
seven = "0" * 39 + "1"  # 39 zeros + 1 : 40 hex valides mais pas un SHA de commit reel
upper7 = ("F" * 40)
check("RB7 validation : enregistrement de base valide ; SHA 39 hex, 41 hex et majuscules rejetes",
      ok_base
      and not v(sha_git="0" * 39)[0]
      and not v(sha_git="0" * 41)[0]
      and not v(sha_git=upper7)[0])

# ---- RB8 : bordures d'horodatage et d'issue ----
check("RB8 bordures : TS sans Z, issue sans #, modele vide rejetes ; TS 00:00:00Z accepte",
      not v(ts_utc="2026-11-23T21:30:00")[0]
      and not v(issue="150")[0]
      and not v(model_api="")[0]
      and v(ts_utc="2026-11-23T00:00:00Z")[0])

# ---- RB9 : ACK exige verdict_url ET verdict_id, faiblesses detectees ----
ok_ack, _ = v(ack=True)
ok_ack_url, _ = v(ack=True, verdict_url="https://example/verdict")
ok_ack_full, _ = v(ack=True, verdict_url="https://example/verdict", verdict_id=1)
check("RB9 ACK : sans URL/ID rejete, URL seule rejete, couple URL+ID accepte",
      (not ok_ack) and (not ok_ack_url) and ok_ack_full)

# ---- RB10 : cycle d'ecriture/lecture idempotent et fichier corrompu ----
import tempfile
path = os.path.join(tempfile.mkdtemp(), "record.json")
coord06.save_record(base, path)
loaded = coord06.load_record(path)
with open(path, "a", encoding="utf-8") as f:
    f.write("{corrompu")
corrupt = coord06.load_record(path)
check("RB10 persistance : relecture identique apres ecriture canonique ; fichier corrompu retourne None (aucune reparation silencieuse)",
      loaded == base and corrupt is None)

print()
print("BILAN robustesse T3: PASS=%d FAIL=%d (tests UNITAIRES sur coord06.py v1.1 en vigueur, module non modifie)" % (len(PASS), len(FAIL)))
if FAIL:
    sys.exit(1)

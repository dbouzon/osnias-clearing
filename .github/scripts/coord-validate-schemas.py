#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0

"""Validation externe des schemas JSON de COORD Specification 1.0 (dialecte
2020-12) et d'instances temoins, executee par le workflow coord-go-ci sur un
runner GitHub (quand l'installation de paquets est impossible dans un
environnement local). Validation REEL avec le validateur independant
`jsonschema` (Draft 2020-12).

Sortie : un état par famille de tests ; exit 0 si et seulement si tout est OK.
"""

import json
import os
import sys

from jsonschema import Draft202012Validator
import referencing

registry = referencing.Registry()
BASE = os.path.join(os.path.dirname(__file__), "..", "..", "coord")
SCHEMAS = os.path.join(BASE, "schemas")
VECTORS = os.path.join(BASE, "docs", "conformite-v15")

ok = 0
ko = 0
failures = []


def check(nom, condition, detail=""):
    global ok, ko
    if condition:
        ok += 1
        print("[OK] " + nom)
    else:
        ko += 1
        failures.append((nom, detail))
        print("[KO] " + nom + (" :: " + str(detail) if detail else ""))


def validate_instance(schema, instance):
    v = Draft202012Validator(schema, registry=registry)
    errs = sorted(v.iter_errors(instance), key=lambda e: e.path)
    return errs


# 1. Chaque schema doit etre valide au regard de la metaschema 2020-12.
schema_names = ["identity", "habilitations", "message", "verdict", "vector", "vector-jcs"]
schemas = {}
for name in schema_names:
    path = os.path.join(SCHEMAS, name + ".schema.json")
    with open(path, encoding="utf-8") as f:
        schemas[name] = json.load(f)
    try:
        Draft202012Validator.check_schema(schemas[name])
        check("schema valide 2020-12 : " + name, True)
    except Exception as e:  # noqa: BLE001
        check("schema valide 2020-12 : " + name, False, e)

# 1 bis. Registre : les $ref distants (par $id https://www.osnias-clearing.com/...)
# resolvent vers les schemas locaux publies dans le meme dossier, sans reseau.
for name, schema in schemas.items():
    if "$id" in schema:
        resource = referencing.Resource.from_contents(schema)
        registry = registry.with_resource(schema["$id"], resource)

# 2. Instances temoins : identites.
sid = schemas["identity"]
check("identite valide acceptee",
      not validate_instance(sid, {"id": "agent-1", "role": "EXECUTANT", "qualifiee_par": "adapter-x"}))
check("identite vide refusee",
      bool(validate_instance(sid, {"id": "", "role": "PILOTE", "qualifiee_par": ""})))
check("role inconnu refuse",
      bool(validate_instance(sid, {"id": "x", "role": "CHEF", "qualifiee_par": "a"})))
check("champ inconnu refuse (additionalProperties false)",
      bool(validate_instance(sid, {"id": "x", "role": "PILOTE", "qualifiee_par": "a", "secret": 1})))

# 3. Habilitations (fail closed).
shab = schemas["habilitations"]
hab_valide = {"adaptateurs": ["adapteur_a", "adapteur_b"],
              "roles": {"EXECUTANT": ["executer", "soumettre", "acker"],
                        "RECEPTIONNAIRE": ["recevoir"],
                        "PILOTE": ["cloturer"]}}
check("habilitations valides acceptees", not validate_instance(shab, hab_valide))
check("liste d'adaptateurs vide acceptee par le schema (N-01 : fail-closed au moteur, pas au schema)",
      not validate_instance(shab, {"adaptateurs": [], "roles": {}}))
check("doublon d'adaptateurs refuse (uniqueItems)",
      bool(validate_instance(shab, {"adaptateurs": ["a", "a"], "roles": {}})))
hab_joker = {"adaptateurs": ["a"],
             "roles": {"EXECUTANT": ["executer", "faker", "*"]}}
check("action joker refusee (enum)",
      bool(validate_instance(shab, hab_joker)))
hab_role_inconnu = {"adaptateurs": ["a"], "roles": {"SUPERVISEUR": ["executer"]}}
check("role inconnu refuse (propertyNames)",
      bool(validate_instance(shab, hab_role_inconnu)))
check("champ inconnu refuse au niveau racine",
      bool(validate_instance(shab, {"adaptateurs": ["a"], "roles": {}, "joker": True})))

# 4. Verdicts (reference obligatoire).
sver = schemas["verdict"]
check("verdict valide accepte",
      not validate_instance(sver, {"verdict": "ACCEPTEE", "verdict_id": "v-1",
                                   "verdict_url": "https://exemple.invalid/v-1",
                                   "revision": "a" * 40, "empreinte_preuves": "b" * 64}))
check("verdict sans verdict_id refuse",
      bool(validate_instance(sver, {"verdict": "ACCEPTEE", "verdict_id": "",
                                     "verdict_url": "https://x/1"})))
check("verdict sans URL refuse", bool(validate_instance(
    sver, {"verdict": "REFUSEE", "verdict_id": "v", "verdict_url": "pas-une-url"})))
check("valeur de verdict inconnue refusee", bool(validate_instance(
    sver, {"verdict": "PEUT-ETRE", "verdict_id": "v", "verdict_url": "https://x/1"})))
check("empreinte mal formee refusee", bool(validate_instance(
    sver, {"verdict": "ACCEPTEE", "verdict_id": "v", "verdict_url": "https://x/1",
           "empreinte_preuves": "zz"})))

# 5. Enveloppe de message.
smsg = schemas["message"]
msg_valide = {"type": "verdict", "mission": "M-XXX-01", "issue": "#20",
              "actor": {"id": "agent-1", "role": "RECEPTIONNAIRE", "qualifiee_par": "adapter-x"},
              "state": "DECISION_ATTENDUE", "data": {}, "ts": "2026-11-24T22:00:00Z"}
check("enveloppe valide acceptee", not validate_instance(smsg, msg_valide))
check("horodatage non UTC-Z refuse", bool(validate_instance(
    smsg, {**msg_valide, "ts": "2026-11-24T22:00:00+01:00"})))
check("etat inconnu refuse", bool(validate_instance(
    smsg, {**msg_valide, "state": "EN_attente"})))
check("issue sans diese refusee", bool(validate_instance(
    smsg, {**msg_valide, "issue": "20"})))
check("empreinte d'enveloppe optionnelle mais contrainte", bool(validate_instance(
    smsg, {**msg_valide, "fingerprint": "court"})))

# 6. Vecteurs de conformite publies (familles coord-conformite-v1 et
#    coord-conformite-jcs-v1, mission M-COORD-JCS-01 #85).
svec = schemas["vector"]
svecjcs = schemas["vector-jcs"]
vector_files = [f for f in sorted(os.listdir(VECTORS))
                if f.endswith(".json") and not f.startswith("manifeste")]
n_vec = 0
n_vec_jcs = 0
fichiers_avec_vecteurs = 0
fichiers_jcs = 0
for f in vector_files:
    with open(os.path.join(VECTORS, f), encoding="utf-8") as fh:
        doc = json.load(fh)
    fmt = doc.get("format")
    if fmt == "coord-conformite-v1":
        errs = validate_instance(svec, doc)
        check("vecteurs valides 2020-12 : " + f, not errs,
              "; ".join(e.message for e in errs[:3]))
        fichiers_avec_vecteurs += 1
        n_vec += len(doc.get("vecteurs", []))
    elif fmt == "coord-conformite-jcs-v1":
        errs = validate_instance(svecjcs, doc)
        check("vecteurs valides 2020-12 : " + f, not errs,
              "; ".join(e.message for e in errs[:3]))
        fichiers_jcs += 1
        n_vec_jcs += len(doc.get("vecteurs", []))
check("22 vecteurs couverts (famille coord-conformite-v1)", n_vec == 22,
      "compte=" + str(n_vec))
check("4 fichiers de vecteurs couverts (famille coord-conformite-v1)",
      fichiers_avec_vecteurs == 4, "compte=" + str(fichiers_avec_vecteurs))
check("3 vecteurs JCS couverts (famille coord-conformite-jcs-v1, #85)",
      n_vec_jcs == 3, "compte=" + str(n_vec_jcs))
check("1 fichier de vecteurs JCS couvert (famille coord-conformite-jcs-v1)",
      fichiers_jcs == 1, "compte=" + str(fichiers_jcs))

print()
print("BILAN SCHEMAS: OK=%d KO=%d" % (ok, ko))
if ko:
    for nom, detail in failures:
        print("ECART: " + nom + " :: " + str(detail))
    sys.exit(1)

#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# Lanceur M-COORD-06 — reproducible, Python 3 stdlib uniquement.
# Usage : sh tests/coord06/run.sh
# Sortie : resume par batterie ; exit != 0 si un seul FAIL.
set -e
cd "$(dirname "$0")"
echo "== Batterie v1.0 (machine a etats, idempotence, obsolescence, securite) =="
python3 tests_coord06.py
echo
echo "== Batterie v1.1 (format versionne, empreintes, indisponibilites, reprise) =="
python3 tests_coord06_v11.py
echo
echo "TOUTES BATTERIES: OK"

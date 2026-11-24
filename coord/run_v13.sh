#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# Lanceur M-COORD-06 v1.3 (candidate) — séparé, conformément à la règle de
# maintenance : run.sh (v1.0+v1.1) et run_v12.sh (v1.2) restent INCHANGES.
# Usage : sh tests/coord06/run_v13.sh
# Sortie : resume par batterie ; exit != 0 si un seul FAIL.
set -e
cd "$(dirname "$0")"
echo "== Batterie v1.3 candidate (preflight, boucle interne, recette normalisee, ancrage SHA, arret, sobriete) =="
python3 tests_coord06_v13.py
echo
echo "BATTERIE v1.3: OK (les batteries v1.0/v1.1 restent lancees par run.sh, v1.2 par run_v12.sh, inchangees)"

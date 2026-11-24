#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# Lanceur M-COORD-06 v1.2 (candidate) — separate, conformement a la critique
# prealable M-COORD-08 : run.sh (v1.0+v1.1) reste INCHANGE.
# Usage : sh tests/coord06/run_v12.sh
# Sortie : resume par batterie ; exit != 0 si un seul FAIL.
set -e
cd "$(dirname "$0")"
echo "== Batterie v1.2 candidate (risque proportionne, preuves executees, reprise, budget) =="
python3 tests_coord06_v12.py
echo
echo "BATTERIE v1.2: OK (les batteries v1.0/v1.1 restent lancees par run.sh, inchangees)"

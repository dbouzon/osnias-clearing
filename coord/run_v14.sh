#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# Lanceur M-COORD-09 v1.4 (candidate) — séparé, conformément à la règle de
# sobriété C6 : run.sh (v1.0+v1.1), run_v12.sh (v1.2), run_v13.sh (v1.3) et
# run_robustness.sh restent INCHANGES.
# Usage : sh tests/coord06/run_v14.sh
# Sortie : resume par batterie ; exit != 0 si un seul FAIL.
set -e
cd "$(dirname "$0")"
echo "== Batterie v1.4 candidate (continuite autonome, reprise, empreintes, faisabilite, dossier, reception differentielle, mesures) =="
python3 tests_coord06_v14.py
echo
echo "BATTERIE v1.4: OK (les batteries v1.0..v1.3 restent lancees par run.sh, run_v12.sh, run_v13.sh, inchangees)"

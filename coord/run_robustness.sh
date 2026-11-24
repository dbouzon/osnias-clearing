#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# Lanceur M-COORD-13 T3 (essai v1.3) — suite de robustesse additive sur le
# module canonique coord06.py. Les lanceurs run.sh, run_v12.sh et run_v13.sh
# restent INCHANGES.
# Usage : sh tests/coord06/run_robustness.sh
set -e
cd "$(dirname "$0")"
echo "== Suite de robustesse T3 (empreintes de preuves, validation, persistance) =="
python3 tests_coord06_robustness.py
echo
echo "BATTERIE robustesse T3: OK (module canonique coord06.py non modifie)"

#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
# Lanceur de la batterie v1.5 durcie (M-COORD-HARDENING-01, issue #19).
# Les batteries v1.0..v1.4 restent lancees par run.sh, run_v12.sh, run_v13.sh,
# run_v14.sh, run_robustness.sh (inchanges).
cd "$(dirname "$0")"
python3 tests_coord06_v15.py

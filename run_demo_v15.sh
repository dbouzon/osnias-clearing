#!/bin/sh
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
# Lanceur de la demonstration autonome v1.5 durcie (sans service externe,
# sans compte, sans cle API). Sortie exit 0 ssi scenario integralement conforme.
cd "$(dirname "$0")"
python3 demo/demo_v15.py

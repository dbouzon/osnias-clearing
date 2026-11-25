# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0

"""Contrôles statiques T13..T16 de l'interface fonctionnelle COORD Pilot (MVP).

Python 3 stdlib uniquement, aucun paquet tiers. Chaque contrôle:
  OK = vérifié / KO = écart bloquant.
"""
import re
import sys
from pathlib import Path

FICHIER = Path(__file__).with_name("index.html")
src = FICHIER.read_text(encoding="utf-8")
bilan = []


def controle(nom, ok):
    bilan.append((nom, ok))
    print(("[OK] " if ok else "[KO] ") + nom)


# T13 : AUCUNE ressource externe (pas de CDN, pas de src/href réseau).
externes = re.findall(r'(?:src|href)\s*=\s*"https?://[^"]+"', src, re.I)
externes += re.findall(r'@import\s+url\(', src)
ctrl13 = not externes and "cdn" not in src.lower() and "//unpkg" not in src and "//cdnjs" not in src
controle("T13 : aucune ressource externe (CSP self, aucun CDN)", ctrl13)

# T14 : bandeau de simulation permanent, visible sur tous les écrans
# (élément unique hors des sections d'écran) + rappel dans <title>.
ctrl14 = (
    "ADAPTATEURS SIMULÉS — AUCUN APPEL RÉEL" in src
    and src.count('id="bandeau"') == 1
    and 'ADAPTATEURS SIMULÉS' in re.search(r"<title>(.*?)</title>", src).group(1)
    and 'position:sticky' in src
)
controle("T14 : bandeau ADAPTATEURS SIMULÉS permanent (sticky) + <title>", ctrl14)

# T15 : appels API typés vers le service local uniquement : un fetch générique
# à chemin relatif (variable), tous les chemins littéraux en /api/* ou /health,
# aucun littéral absolu http(s) dans un appel fetch.
appels_litteraux = re.findall(r'api\(\s*"([^"]*)"', src)
ctrl15 = (
    "fetch(chemin" in src
    and not re.findall(r"fetch\([^)]*https?:", src)
    and len(appels_litteraux) >= 5
    and all(c.startswith("/api/") or c == "/health" for c in appels_litteraux)
)
controle("T15 : appels fetch typés /api/* et /health uniquement, aucun appel externe", ctrl15)

# T16 : « Prise en main » accessible depuis le tableau de bord + autorité
# humaine unique (S4 documenté), et les 6 étapes du déroulé présentes.
ctrl16 = (
    'data-ecran="prise"' in src
    and "Prise en main" in src
    and "Déroulé d'une mission en 6 étapes" in src
    and "sha_head" in src
    and "github fait foi" in src
    and 'id="token"' in src
)
controle("T16 : guide Prise en main complet (6 étapes, rôles, N-16/N-17, token local)", ctrl16)

ok = sum(1 for _, v in bilan if v)
print(f"\nBILAN UI STATIQUE: OK={ok} KO={len(bilan)-ok}")
sys.exit(0 if ok == len(bilan) else 1)

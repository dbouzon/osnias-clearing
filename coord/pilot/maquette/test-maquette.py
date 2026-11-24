#!/usr/bin/env python3
"""Contrôles de recette de la maquette COORD Pilot (M-COORD-PILOT-01, issue #25).

Contrôles statiques REEL (parsing HTML/JS) ; le rendu navigateur n'est pas
exécuté ici : les contrôles visuels et d'interaction sont NON_EXECUTE dans ce
cadre et restent à la charge du pilote (ouverture locale du fichier).

Copyright © 2026 Denis Bouzon ; ORCID: https://orcid.org/0009-0007-8894-8902
SPDX-License-Identifier: Apache-2.0
"""
import re
import sys
from html.parser import HTMLParser

MAP = "index.html"

checks = []


def check(name, ok, detail=""):
    checks.append((name, bool(ok), detail))


src = open(MAP, encoding="utf-8").read()

# T1 : bandeau DONNÉES SIMULÉES présent et permanent (hors sections) 
check("T1 bandeau 'DONNÉES SIMULÉES' présent",
      "DONNÉES SIMULÉES" in src and 'class="banner"' in src)

# T2 : les 8 écrans attendus existent comme sections
ecrans = ["prise", "dashboard", "creer", "cycle", "arbitrage", "arret", "couts", "journal"]
manquants = [e for e in ecrans if not re.search(r'<section[^>]*id="%s"' % e, src)]
check("T2 les 8 écrans présents", not manquants, "manquants: %s" % manquants)

# T3 : chaque onglet de navigation pointe vers un écran existant
navs = re.findall(r'data-go="([a-z]+)"', src)
bad_nav = sorted({n for n in navs if n not in ecrans})
check("T3 navigation cohérente", not bad_nav, "cibles inconnues: %s" % bad_nav)

# T4 : aucun appel réseau externe (fetch, XMLHttpRequest, src http, lien script)
externes = re.findall(r'(fetch\(|XMLHttpRequest|<script[^>]+src=|<img[^>]+src=["\']https?://)', src)
check("T4 aucun appel externe ni ressource distante", not externes, str(externes[:3]))

# T5 : les éléments référencés par getElementById existent dans le HTML
ids_js = set(re.findall(r'getElementById\("([^"]+)"\)', src))
ids_html = set(re.findall(r'id="([^"]+)"', src))
absents = sorted(i for i in ids_js if i not in ids_html)
check("T5 identifiants DOM référencés présents", not absents, "absents: %s" % absents)

# T6 : les 8 états normatifs figurent avec leur style pill
etats = ["PLANIFIEE", "EXECUTION", "DECISION_ATTENDUE", "ACCEPTEE",
         "EN_CORRECTION", "ACK_RECU", "REFUSEE", "CLOTUREE"]
manq_etats = [e for e in etats if e not in src or ("st-" + e) not in src]
check("T6 les 8 états de la spec affichables", not manq_etats, str(manq_etats))

# T7 : mentions normatives attendues (invariants illustrés)
inv = ["N-01", "N-05", "N-09", "N-10", "N-12", "N-13", "N-16", "N-17", "N-20"]
manq_inv = [i for i in inv if i not in src]
check("T7 invariants illustrés à l'écran", not manq_inv, str(manq_inv))

# T8 : trois rôles et cinq actions normatifs cités
roles = ["EXECUTANT", "RECEPTIONNAIRE", "PILOTE",
         "executer", "soumettre", "recevoir", "acker", "cloturer"]
manq_roles = [r for r in roles if r not in src]
check("T8 rôles et actions normatifs présents", not manq_roles, str(manq_roles))

# T9 : séparation ARRET_PILOTE / ECART explicite
check("T9 distinction ARRET_PILOTE / ECART (N-20) explicite",
      "ARRET_PILOTE" in src and "ECART" in src and "N-20" in src)

# T10 : copyright + ORCID + licence
check("T10 copyright, ORCID, Apache-2.0",
      "0009-0007-8894-8902" in src and "Apache-2.0" in src and "Denis Bouzon" in src)

# T11 : équilibre des balises section
opens = len(re.findall(r"<section\b", src))
closes = len(re.findall(r"</section>", src))
check("T11 sections équilibrées", opens == closes == 8, "open=%d close=%d" % (opens, closes))

# T12 : HTML globalement bien formé (parse intégral sans erreur)
class P(HTMLParser):
    def error(self, message):
        raise ValueError(message)

try:
    P().feed(src)
    ok12, det12 = True, ""
except Exception as exc:  # pragma: no cover
    ok12, det12 = False, str(exc)
check("T12 parse HTML sans erreur", ok12, det12)

# Bilan
bilan = open("recette-maquette.resultat.txt", "w", encoding="utf-8")
ok = 0
for name, passed, detail in checks:
    line = ("[PASS] " if passed else "[FAIL] ") + name + ((" — " + detail) if detail else "")
    bilan.write(line + "\n")
    ok += passed
bilan.write("\nBILAN MAQUETTE: PASS=%d FAIL=%d (contrôles statiques REEL ; rendu navigateur NON_EXECUTE, à la charge du pilote)\n"
            % (ok, len(checks) - ok))
bilan.close()

for name, passed, detail in checks:
    print(("[PASS] " if passed else "[FAIL] ") + name + ((" — " + detail) if detail else ""))
print("\nBILAN MAQUETTE: PASS=%d FAIL=%d" % (ok, len(checks) - ok))
sys.exit(0 if ok == len(checks) else 1)

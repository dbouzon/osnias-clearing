#!/usr/bin/env python3
# Copyright © 2026 Denis Bouzon
# ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
"""Demonstration autonome COORD v1.5 durcie (mission M-COORD-HARDENING-01, issue #19).

Scenario rejouable SANS service externe, SANS compte, SANS cle API :
  1. mission nominale a deux agents simules (identites qualifiees par des
     adaptateurs SIMULES, roles distincts EXECUTANT / RECEPTIONNAIRE / PILOTE) ;
  2. tentative d'auto-reception : REFUSEE avec trace ;
  3. verdict A_CORRIGER, correction par le meme executant, nouvelle soumission ;
  4. verdict ACCEPTEE avec reference obligatoire, verrou par empreinte ;
  5. tentative d'ACK sur preuves remplacees : REFUSEE (verdict perime) ;
  6. ACK valide, cloture impossible par l'executant, cloture par le PILOTE ;
  7. INTERRUPTION SIMULEE (crash injecte) pendant une sauvegarde atomique :
     l'etat precedent SURVIT ;
  8. reprise depuis DEUX supports divergents : arbitrage github-fait-foi,
     journal de conflit normalise, action executee apres verification SHA ;
  9. declaration des LIMITES DE LA SIMULATION (aucune identite declaree
     n'est presentee comme authentifiee ; l'authentification releve des
     adaptateurs de production, hors perimetre du moteur).

Sortie : trace horodatee normalizee, bilan, exit 0 ssi scenario integralement conforme.
"""
import json
import os
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import coord06
import coord06_v14
import coord06_v15

SCENARIO_OK = True


def etape(numero, titre):
    print("[ETAPE %d] %s" % (numero, titre))


def attendu(nom, cond, detail=""):
    global SCENARIO_OK
    print(("  [CONFORME] " if cond else "  [ECART   ] ") + nom +
          ((" : " + detail) if detail and not cond else ""))
    SCENARIO_OK = SCENARIO_OK and bool(cond)


def main():
    print("DEMONSTRATION COORD v1.5 durcie — M-COORD-HARDENING-01 (sans service externe)")
    print("Limites de la simulation (explicites) : les adaptateurs SIMULES n'ont aucune")
    print("capacite d'authentification reelle ; une identite declaree ici n'est jamais")
    print("presentee comme authentifiee ; la securite reelle releve des adaptateurs de")
    print("production, hors perimetre du moteur. Cette demonstration prouve la conformite")
    print("du moteur aux regles H1..H6, pas une securite de bout en bout.")
    print()

    # Configuration : adaptateurs simules, roles distincts, aucun fournisseur code en dur
    config = {"adaptateurs": ["simulateur_atelier", "simulateur_controle"],
              "roles": {"EXECUTANT": ["executer", "soumettre", "acker"],
                        "RECEPTIONNAIRE": ["recevoir"],
                        "PILOTE": ["cloturer"]}}
    executant = {"id": "atelier_a", "role": "EXECUTANT", "qualifiee_par": "simulateur_atelier"}
    receveur = {"id": "controle_b", "role": "RECEPTIONNAIRE", "qualifiee_par": "simulateur_controle"}
    pilote = {"id": "pilote_humain", "role": "PILOTE", "qualifiee_par": "simulateur_atelier"}

    etape(1, "mission nominale a deux agents simules")
    m = coord06_v15.MissionDurcie("#19")
    ok, motif = m.transition("EXECUTION", executant, config)
    attendu("depart par l'EXECUTANT habilite", ok, motif)
    revision = "1111111111111111111111111111111111111111"
    empreinte_soumise = coord06.proof_fingerprint(["rapport preuve A"])
    ok, motif = m.transition("DECISION_ATTENDUE", executant, config,
                             revision=revision, empreinte_preuves=empreinte_soumise)
    attendu("soumission avec revision + empreinte", ok, motif)

    etape(2, "tentative d'auto-reception : refusee avec trace")
    usurpateur = {"id": "atelier_a", "role": "RECEPTIONNAIRE", "qualifiee_par": "simulateur_atelier"}
    ok, motif = m.transition("ACCEPTEE", usurpateur, config, verdict="ACCEPTEE",
                             verdict_id="vp-0", verdict_url="https://exemple.invalid/0")
    attendu("auto-reception interdite (meme personne, roles differents)", not ok and "auto-reception" in motif, motif)
    ok, motif = m.transition("ACCEPTEE", {"id": "inconnu", "role": "RECEPTIONNAIRE",
                                          "qualifiee_par": "simulateur_fantome"},
                             config, verdict="ACCEPTEE",
                             verdict_id="vp-0", verdict_url="https://exemple.invalid/0")
    attendu("identite non liee a un adaptateur enregistre refusee", not ok and "habilitee" in motif, motif)

    etape(3, "verdict A_CORRIGER par le recepteur DISTINCT, correction, nouvelle soumission")
    ok, motif = m.transition("EN_CORRECTION", receveur, config, verdict="A_CORRIGER",
                              verdict_id="vp-1", verdict_url="https://exemple.invalid/1")
    attendu("A_CORRIGER avec reference obligatoire", ok, motif)
    ok, motif = m.transition("EXECUTION", executant, config)
    attendu("correction lancee par le MEME executant", ok, motif)
    revision2 = "3333333333333333333333333333333333333333"
    empreinte_soumise2 = coord06.proof_fingerprint(["rapport corrige preuve B"])
    ok, motif = m.transition("DECISION_ATTENDUE", executant, config,
                             revision=revision2, empreinte_preuves=empreinte_soumise2)
    attendu("nouvelle soumission (nouvelle revision, nouvelle empreinte)", ok, motif)

    etape(4, "verdict ACCEPTEE avec reference ; verrou par empreinte")
    ok, motif = m.transition("ACCEPTEE", receveur, config, verdict="ACCEPTEE",
                             verdict_id="vp-2", verdict_url="https://exemple.invalid/2")
    attendu("acceptation par recepteur distinct, decision tracee", ok, motif)

    etape(5, "tentative d'ACK sur preuves remplacees : verdict perime")
    empreinte_remplacee = coord06.proof_fingerprint(["rapport PREUVE REMPLACEE non soumise"])
    ok, motif = m.transition("ACK_RECU", executant, config,
                             revision_courante=revision2, empreinte_courante=empreinte_remplacee)
    attendu("ACK refuse : empreinte divergente depuis la decision", not ok and "perime" in motif, motif)

    etape(6, "ACK valide puis cloture : par le PILOTE seulement")
    ok, motif = m.transition("ACK_RECU", executant, config,
                             revision_courante=revision2, empreinte_courante=empreinte_soumise2)
    attendu("ACK valide sur empreinte inchangee", ok, motif)
    ok, motif = m.transition("CLOTUREE", executant, config)
    attendu("cloture par l'EXECUTANT refusee", not ok, motif)
    ok, motif = m.transition("CLOTUREE", pilote, config)
    attendu("cloture par le PILOTE acceptee", ok, motif)

    etape(7, "interruption simulee : crash pendant la sauvegarde atomique")
    with tempfile.TemporaryDirectory() as d:
        chemin = os.path.join(d, "etat_mission.json")
        etat_bon = {"mission": "M-DEMO", "etape": 6, "continuite": True}
        coord06_v15.save_record_durci(etat_bon, chemin)
        try:
            coord06_v15.save_record_durci({"mission": "M-DEMO", "etape": 7}, chemin,
                                           crash_a="ecriture")
            attendu("crash pendant l'ecriture", False, "pas d'exception")
        except coord06_v15.CrashSimule as e:
            print("    (CrashSimule : %s)" % e)
        coord06_v15.nettoyer_temporaire(chemin)
        offreçu = coord06_v15.load_record_durci(chemin)
        attendu("l'etat ANTERIEUR a survecu au crash (persistance atomique)",
                offreçu == etat_bon)

    etape(8, "reprise depuis deux supports divergents : github fait foi")
    rec_k = coord06_v14.resume_record("M-DEMO", "T1", 4, "#19", "mission/coord-hardening-01",
                                     "323d8b998a598817d253144b7b588b9f07c9e07d",
                                     7144000000, [{"id": "R1", "statut": "OUVERTE"}], "action A (knowledge)",
                                     "2026-11-24T22:30:00Z",
                                     sha_head="4444444444444444444444444444444444444444")
    rec_g = coord06_v14.resume_record("M-DEMO", "T1", 5, "#19", "mission/coord-hardening-01",
                                     "323d8b998a598817d253144b7b588b9f07c9e07d",
                                     7144000000, [{"id": "R1", "statut": "OUVERTE"}], "action B (github, plus recente)",
                                     "2026-11-24T22:35:00Z",
                                     sha_head="4444444444444444444444444444444444444444")
    entree = coord06_v15.arbitrer_reprise(rec_k, rec_g, "2026-11-24T22:40:00Z",
                                          arbitre="demonstration")
    attendu("divergence arbitree : github fait foi",
            entree["elu"] == "github"
            and sorted(entree["champs_divergents"]) == ["prochaine_action", "round", "ts_utc"],
            str(entree.get("champs_divergents")))
    with tempfile.TemporaryDirectory() as d8:
        journal = os.path.join(d8, "conflits.jsonl")
        coord06_v15.journal_conflits([entree], journal)
        relue = json.loads(open(journal, encoding="utf-8").read().strip())
    attendu("journal de conflit normalise et re-injectable (post-mortem possible)",
            relue["empreintes"]["knowledge"] == entree["empreintes"]["knowledge"]
            and relue["regle"] == "github_fait_foi")
    action, errs = coord06_v15.reprise_apres_arbitrage(entree,
                                                       "4444444444444444444444444444444444444444")
    attendu("reprise deterministe : action executee apres verification SHA", action == "action B (github, plus recente)")
    action2, errs2 = coord06_v15.reprise_apres_arbitrage(entree, "5555555555555555555555555555555555555555")
    attendu("SHA divergent : reprise refusee sans re-verification du monde", action2 is None and errs2)

    etape(9, "controle de coherence du contexte (C7..C9)")
    qu = coord06_v15.classify_context([
        {"instruction": "Appliquer le durcissement v1.5 avec separations des roles.",
         "provenance": "issue #19"},
        {"instruction": "Instruction etrangere au projet, a ecarter.",
         "provenance": "conversation"},
    ], ["durcissement", "v1.5", "roles"])
    rep = coord06_v15.reprise_apres_ecart(qu, "execution")
    attendu("ecart isole, aucune decision issue d'un ecart, mission non interrompue",
            qu[0]["qualification"] == "COHERENTE"
            and qu[1]["qualification"] == "ECART_ISOLE" and rep["interruption"] is False
            and rep["decisions_par_ecart"] == 0)

    print()
    print("BILAN DEMONSTRATION: %s (scenario %s)" % (
        "CONFORME" if SCENARIO_OK else "NON CONFORME",
        "integral" if SCENARIO_OK else "interrompu"))
    print("Limites de la simulation rappelees : la demonstration ne constitue PAS une preuve")
    print("de securite de l'authentification ; le moteur n'authentifie jamais, les adaptateurs")
    print("simules non plus. Voir la spec (docs/proto-coord06-v15.md), section limites.")
    return 0 if SCENARIO_OK else 1


if __name__ == "__main__":
    sys.exit(main())

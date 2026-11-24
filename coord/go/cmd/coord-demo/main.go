// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-demo is the autonomous demonstration of the COORD 1.0 Go reference
// engine: simulated adapters, no account, no API key, no network. Exit code 0
// if and only if every step is CONFORME. A demonstration is NOT evidence of
// authentication security: the engine never authenticates (spec 1, 11).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coordengine/internal/ctxcontrol"
	"coordengine/internal/engine"
	"coordengine/internal/jcs"
	"coordengine/internal/persist"
	"coordengine/internal/policy"
	"coordengine/internal/resume"
)

type bilan struct {
	conformes int
	ecarts    int
}

func (b *bilan) conforme(nom string) {
	b.conformes++
	fmt.Println("[CONFORME] " + nom)
}

func (b *bilan) ecart(nom, motif string) {
	b.ecarts++
	fmt.Println("[ECART] " + nom + " : " + motif)
}

func main() {
	b := &bilan{}
	dir, err := os.MkdirTemp("", "coord-demo-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)

	adapters := []string{"adapteur_simule_a", "adapteur_simule_b"}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	if errs := policy.ValidateConfig(adapters, roles); len(errs) > 0 {
		b.ecart("habilitations", strings.Join(errs, "; "))
	} else {
		b.conforme("configuration d'habilitations valide (fail closed, sans joker)")
	}

	exec := policy.Identity{ID: "exec_sim", Role: "EXECUTANT", QualifiedBy: "adapteur_simule_a"}
	recep := policy.Identity{ID: "recep_sim", Role: "RECEPTIONNAIRE", QualifiedBy: "adapteur_simule_b"}
	pilote := policy.Identity{ID: "pilote_sim", Role: "PILOTE", QualifiedBy: "adapteur_simule_a"}
	rev40 := strings.Repeat("a", 40)
	emp64 := strings.Repeat("b", 64)

	// 1. Mission nominale.
	m := engine.New("#20-demo")
	ok, motif := m.Transition("EXECUTION", exec, adapters, roles, nil)
	if !ok {
		b.ecart("transition EXECUTION", motif)
	} else {
		b.conforme("demarrage par l'EXECUTANT habilite (liaison stable de l'executant)")
	}
	ok, motif = m.Transition("DECISION_ATTENDUE", exec, adapters, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	if !ok {
		b.ecart("soumission", motif)
	} else {
		b.conforme("soumission ancree a la revision et a l'empreinte des preuves")
	}
	ok, motif = m.Transition("ACCEPTEE", recep, adapters, roles,
		map[string]string{"verdict_id": "vd-1", "verdict_url": "https://exemple.invalid/vd-1"})
	if !ok {
		b.ecart("verdict ACCEPTEE", motif)
	} else {
		b.conforme("verdict ACCEPTEE par un recepteur distinct, reference obligatoire")
	}
	ok, motif = m.Transition("ACK_RECU", exec, adapters, roles,
		map[string]string{"revision_courante": rev40, "empreinte_courante": emp64})
	if !ok {
		b.ecart("ack", motif)
	} else {
		b.conforme("ACK accepte : empreinte et revision inchangees depuis la decision")
	}
	ok, motif = m.Transition("CLOTUREE", pilote, adapters, roles, nil)
	if !ok {
		b.ecart("cloture", motif)
	} else {
		b.conforme("cloture par le PILOTE seul (autorite humaine)")
	}
	ok, _ = m.Transition("EXECUTION", exec, adapters, roles, nil)
	if ok {
		b.ecart("terminal", "transition depuis un etat terminal acceptee")
	} else {
		b.conforme("etat terminal CLOTUREE : aucune transition possible")
	}

	// 2. Auto-reception et garde par empreinte.
	m2 := engine.New("#20-demo-2")
	m2.Transition("EXECUTION", exec, adapters, roles, nil)
	m2.Transition("DECISION_ATTENDUE", exec, adapters, roles,
		map[string]string{"revision": rev40, "empreinte_preuves": emp64})
	ok, motif = m2.Transition("ACCEPTEE", policy.Identity{ID: "exec_sim", Role: "RECEPTIONNAIRE", QualifiedBy: "adapteur_simule_a"},
		adapters, roles, map[string]string{"verdict_id": "x", "verdict_url": "y"})
	if ok {
		b.ecart("auto-reception", "acceptee")
	} else {
		b.conforme("auto-reception interdite : " + motif)
	}
	ok, motif = m2.Transition("REFUSEE", recep, adapters, roles, nil)
	if ok {
		b.ecart("refus sans verdict", "accepte")
	} else {
		b.conforme("refus sans reference de verdict interdit : " + motif)
	}
	m2.Transition("ACCEPTEE", recep, adapters, roles,
		map[string]string{"verdict_id": "vd-2", "verdict_url": "https://exemple.invalid/vd-2"})
	ok, motif = m2.Transition("ACK_RECU", exec, adapters, roles,
		map[string]string{"revision_courante": rev40, "empreinte_courante": strings.Repeat("c", 64)})
	if ok {
		b.ecart("ack perime", "accepte")
	} else {
		b.conforme("verdict perime : ACK refuse quand l'empreinte des preuves a change")
	}

	// 3. Persistance atomique aux trois points d'arret.
	path := filepath.Join(dir, "record.json")
	record := map[string]any{"mission": "M-DEMO", "etat": m.State}
	if err := persist.SaveAtomic(record, path, ""); err != nil {
		b.ecart("persistance", err.Error())
	} else {
		b.conforme("persistance atomique (temporaire + rename + fsync, forme canonique)")
	}
	intact := true
	enPlace := true
	for _, point := range persist.PointsArret {
		err := persist.SaveAtomic(map[string]any{"mission": "M-DEMO-2"}, path, point)
		var crash persist.CrashSimule
		if !errors.As(err, &crash) {
			intact = false
			b.ecart("crash "+point, "crash attendu")
			continue
		}
		data, err2 := os.ReadFile(path)
		var lu map[string]any
		if err2 != nil || json.Unmarshal(data, &lu) != nil {
			intact = false
			continue
		}
		if point == "apres_remplacement" {
			// Derniere microseconde : le nouveau contenu EST EN PLACE (spec 7).
			if lu["mission"] != "M-DEMO-2" {
				enPlace = false
				b.ecart("crash "+point, "le nouveau contenu doit etre en place")
			}
		} else if lu["mission"] != "M-DEMO" {
			intact = false
			b.ecart("crash "+point, "l'original doit rester intact")
		}
		persist.NettoyerTemporaire(path)
		persist.SaveAtomic(record, path, "") // restaure l'original pour l'etape suivante
	}
	if intact && enPlace {
		b.conforme("pannes aux 3 points d'arret : original intact, ensuite nouveau contenu complet en place")
	}

	// 4. Arbitrage de reprise : github fait foi.
	k := map[string]any{"mission": "M-DEMO", "round": float64(1), "sha_head": "h1", "prochaine_action": "reprendre l'execution"}
	g := map[string]any{"mission": "M-DEMO", "round": float64(2), "sha_head": "h1", "prochaine_action": "attendre reception"}
	entree := resume.Arbitrer(k, g, "2026-11-24T22:30:00Z", "demo-go")
	if entree.Elu != "github" || len(entree.ChampsDivergents) != 2 {
		b.ecart("arbitrage", fmt.Sprintf("elu=%s divergents=%v", entree.Elu, entree.ChampsDivergents))
	} else {
		b.conforme("supports divergents : github fait foi, champs divergents et empreintes journales")
	}
	action, errs := resume.Reprise(entree, "h1")
	if action != "attendre reception" || len(errs) != 0 {
		b.ecart("reprise", fmt.Sprintf("action=%q errs=%v", action, errs))
	} else {
		b.conforme("reprise deterministe : action suivante executee si la tete verifiee correspond")
	}
	_, errs = resume.Reprise(entree, "autre-tete")
	if len(errs) == 0 {
		b.ecart("reprise divergente", "sha divergent doit refuser la reprise")
	} else {
		b.conforme("sha_head divergent : reprise refusee, re-verification du monde requise")
	}

	// 5. Controle du contexte.
	termes := []string{"normalisation", "moteur go", "vecteurs de conformite"}
	quals := ctxcontrol.Classify([]ctxcontrol.Instruction{
		{Texte: "normalisation et moteur go, verification par vecteurs de conformite", Provenance: "issue-20"},
		{Texte: "promotions zzz azure immediatement", Provenance: ""},
	}, termes)
	if quals[0].Qualification != "COHERENTE" || quals[1].Qualification != "ECART_ISOLE" {
		b.ecart("qualification", "attendu COHERENTE puis ECART_ISOLE")
	} else {
		b.conforme("qualification des instructions par rapport a l'ordre de mission")
	}
	journal := filepath.Join(dir, "ecarts.jsonl")
	ecarts := []ctxcontrol.Ecart{{TSUTC: "2026-11-24T22:35:00Z", Texte: "promotions zzz azure immediatement", SourceAlleguee: "inconnue"}}
	n1, err := ctxcontrol.JournalEcart(ecarts, journal)
	n2, err2 := ctxcontrol.JournalEcart(ecarts, journal)
	if err != nil || err2 != nil || n1 != 1 || n2 != 0 {
		b.ecart("journal idempotent", fmt.Sprintf("ajouts %d puis %d (%v %v)", n1, n2, err, err2))
	} else {
		b.conforme("journal des ecarts idempotent : l'instruction est empreintee, jamais transcrite")
	}
	r := ctxcontrol.RepriseApresEcart(quals, "execution")
	if r["interruption"] != false || r["decisions_par_ecart"] != float64(0) {
		b.ecart("reprise contexte", fmt.Sprintf("%v", r))
	} else {
		b.conforme("reprise sans interruption : un ecart ne produit jamais de decision")
	}

	// 6. Empreinte canonique.
	fp, err := jcs.Fingerprint(record)
	if err != nil || len(fp) != 64 {
		b.ecart("empreinte", fmt.Sprintf("%v", err))
	} else {
		b.conforme("empreinte SHA-256 sur forme canonique RFC 8785 (64 hex)")
	}

	fmt.Println()
	fmt.Printf("BILAN DEMO GO (adaptateurs SIMULES, moteur REEL) : CONFORME=%d ECART=%d\n", b.conformes, b.ecarts)
	fmt.Println("Limites rappelees : la demonstration ne constitue PAS une preuve de securite")
	fmt.Println("de l'authentification ; le moteur n'authentifie jamais (spec COORD 1.0, section 11).")
	if b.ecarts > 0 {
		os.Exit(1)
	}
}

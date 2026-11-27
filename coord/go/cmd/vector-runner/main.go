// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// vector-runner replays the published COORD conformance vectors
// (coord-conformite-v1 format, plus the coord-conformite-jcs-v1 canonical
// serialization family) against the Go reference engine.
//
// Usage: go run ./cmd/vector-runner -dir ../../docs/conformite-v15
// Exit code 0 if and only if no vector is KO. Output format matches the
// Python reference runner for cross-checks (same OK/KO per vector).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"coordengine/internal/engine"
	"coordengine/internal/jcs"
	"coordengine/internal/policy"
	"coordengine/internal/resume"
)

type vecteur struct {
	Nom        string           `json:"nom"`
	Operations []map[string]any `json:"operations"`
	Attendu    map[string]any   `json:"attendu"`
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func identity(m map[string]any) policy.Identity {
	raw, _ := m["identite"].(map[string]any)
	return policy.Identity{ID: str(raw, "id"), Role: str(raw, "role"), QualifiedBy: str(raw, "qualifiee_par")}
}

func config(m map[string]any) ([]string, map[string][]string) {
	raw, _ := m["config"].(map[string]any)
	adapters := []string{}
	if a, ok := raw["adaptateurs"].([]any); ok {
		for _, x := range a {
			if s, ok := x.(string); ok {
				adapters = append(adapters, s)
			}
		}
	}
	roles := map[string][]string{}
	if r, ok := raw["roles"].(map[string]any); ok {
		for role, acts := range r {
			list := []string{}
			if la, ok := acts.([]any); ok {
				for _, a := range la {
					if s, ok := a.(string); ok {
						list = append(list, s)
					}
				}
			}
			roles[role] = list
		}
	}
	return adapters, roles
}

func donnees(m map[string]any) map[string]string {
	out := map[string]string{}
	if d, ok := m["donnees"].(map[string]any); ok {
		for k, v := range d {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

// rejoue replays one vector and returns (conforme, detail).
func rejoue(v vecteur) (bool, string) {
	var mission *engine.Mission
	lastKind := ""
	var ok bool
	var info string
	var arb resume.Entree
	for _, op := range v.Operations {
		switch str(op, "operation") {
		case "mission":
			mission = engine.New(str(op, "issue"))
			lastKind, ok, info = "mission", true, mission.State
		case "transition":
			if mission == nil {
				return false, "vecteur mal forme : transition sans mission"
			}
			adapters, roles := config(op)
			ok, info = mission.Transition(str(op, "cible"), identity(op), adapters, roles, donnees(op))
			lastKind = "transition"
		case "valider_habilitations":
			adapters, roles := config(op)
			errs := policy.ValidateConfig(adapters, roles)
			ok = len(errs) == 0
			info = strings.Join(errs, "; ")
			lastKind = "habilitations"
		case "arbitrer":
			rk, _ := op["record_knowledge"].(map[string]any)
			rg, _ := op["record_github"].(map[string]any)
			arb = resume.Arbitrer(rk, rg, str(op, "ts_utc"), str(op, "arbitre"))
			info = arb.Elu
			lastKind = "arbitrer"
		case "empreinte":
			// Famille serialisation canonique (format coord-conformite-jcs-v1) :
			// forme canonique RFC 8785 de l'objet, refus fail-closed des non-entiers.
			b, err := jcs.Canonical(op["objet"])
			if err != nil {
				ok, info = false, err.Error()
			} else {
				ok, info = true, string(b)
			}
			lastKind = "empreinte"
		default:
			return false, "operation inconnue: " + str(op, "operation")
		}
	}
	att := v.Attendu
	switch lastKind {
	case "transition":
		if attOk, _ := att["ok"].(bool); ok != attOk {
			return false, fmt.Sprintf("ok=%v attendu ok=%v (motif: %s)", ok, attOk, info)
		}
		if attOk, _ := att["ok"].(bool); attOk {
			if etat, _ := att["etat"].(string); etat != "" && mission != nil && mission.State != etat {
				return false, "etat final " + mission.State + " != attendu " + etat
			}
		} else if cont, _ := att["contient"].(string); cont != "" && !strings.Contains(info, cont) {
			return false, "motif inattendu: " + info + " (contient attendu: " + cont + ")"
		}
	case "habilitations":
		attH, _ := att["habilitations"].(bool)
		if ok != attH {
			return false, fmt.Sprintf("habilitations ok=%v attendu=%v (%s)", ok, attH, info)
		}
		if cont, _ := att["contient"].(string); cont != "" && !strings.Contains(info, cont) {
			return false, "erreur attendue absente: " + cont
		}
	case "arbitrer":
		attA, _ := att["arbitre"].(string)
		if arb.Elu != attA {
			return false, "elu=" + arb.Elu + " attendu=" + attA
		}
		attD, _ := att["champs_divergents"].([]any)
		attendus := map[string]bool{}
		for _, x := range attD {
			if s, ok := x.(string); ok {
				attendus[s] = true
			}
		}
		obtenus := map[string]bool{}
		for _, s := range arb.ChampsDivergents {
			obtenus[s] = true
		}
		if len(attendus) != len(obtenus) {
			return false, "champs divergents " + strings.Join(arb.ChampsDivergents, ",") + " != attendus"
		}
		for k := range attendus {
			if !obtenus[k] {
				return false, "champ divergent manquant: " + k
			}
		}
	case "empreinte":
		if errAtt, _ := att["erreur"].(bool); errAtt {
			if ok {
				return false, "refus attendu mais serialisation produite: " + info
			}
			if cont, _ := att["contient"].(string); cont != "" && !strings.Contains(info, cont) {
				return false, "motif inattendu: " + info + " (contient attendu: " + cont + ")"
			}
		} else {
			if !ok {
				return false, "erreur non attendue: " + info
			}
			if oct, _ := att["octets"].(string); oct != "" && info != oct {
				return false, "octets != attendus: " + info + " != " + oct
			}
			if sha, _ := att["sha256"].(string); sha != "" {
				sum := sha256.Sum256([]byte(info))
				if hex.EncodeToString(sum[:]) != sha {
					return false, "sha256 != attendu pour les octets produits"
				}
			}
		}
	default:
		return false, "aucune operation jouee"
	}
	return true, ""
}

func main() {
	dir := flag.String("dir", "", "dossier des vecteurs JSON (format coord-conformite-v1)")
	flag.Parse()
	d := *dir
	if d == "" {
		d = filepath.Join("..", "..", "docs", "conformite-v15")
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dossier illisible:", err)
		os.Exit(2)
	}
	var fichiers []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			fichiers = append(fichiers, e.Name())
		}
	}
	sort.Strings(fichiers)
	passes, echecs := 0, 0
	for _, nom := range fichiers {
		b, err := os.ReadFile(filepath.Join(d, nom))
		if err != nil {
			fmt.Println("[KO] " + nom + " :: lecture: " + err.Error())
			echecs++
			continue
		}
		var doc struct {
			Vecteurs []vecteur `json:"vecteurs"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			fmt.Println("[KO] " + nom + " :: json: " + err.Error())
			echecs++
			continue
		}
		for _, v := range doc.Vecteurs {
			cle := nom + " :: " + v.Nom
			if ok, detail := rejoue(v); ok {
				passes++
				fmt.Println("[OK] " + cle)
			} else {
				echecs++
				fmt.Println("[KO] " + cle + " - " + detail)
			}
		}
	}
	fmt.Println()
	fmt.Printf("BILAN CONFORMITE: OK=%d KO=%d (vecteurs de %s)\n", passes, echecs, d)
	if echecs > 0 {
		os.Exit(1)
	}
}

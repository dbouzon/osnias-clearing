// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package jcs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOrdreUTF16(t *testing.T) {
	// Ordre par unites UTF-16 (RFC 8785, 3.2.3) :
	// \r (0x000D) < "1" (0x0031) < ö (0x00F6) < € (0x20AC).
	in := map[string]any{"\u20ac": "Euro", "\r": "CR", "1": "One", "\u00f6": "O-dia"}
	got, err := Canonical(in)
	if err != nil {
		t.Fatalf("erreur canonique: %v", err)
	}
	want := `{"\r":"CR","1":"One","ö":"O-dia","€":"Euro"}`
	if string(got) != want {
		t.Fatalf("ordre UTF-16 attendu %s, obtenu %s", want, got)
	}
}

func TestEntierSeul(t *testing.T) {
	// Les nombres non entiers sont refuses, jamais approximes.
	if _, err := Canonical(map[string]any{"x": 1.5}); err == nil {
		t.Fatal("un nombre non entier doit etre une erreur")
	}
	got, err := Canonical([]any{float64(42)})
	if err != nil || string(got) != "[42]" {
		t.Fatalf("entier attendu [42], obtenu %s (err=%v)", got, err)
	}
	// Grand entier (curseur de relais) : rendu sans exposant.
	got, err = Canonical(map[string]any{"curseur": float64(7144000000)})
	if err != nil || !contains(string(got), `7144000000`) {
		t.Fatalf("grand entier attendu, obtenu %s (err=%v)", got, err)
	}
}

func TestEchappementMinimal(t *testing.T) {
	got, err := Canonical(map[string]any{"a\nb\"c\\d": "e\tf"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a\nb\"c\\d":"e\tf"}`
	if string(got) != want {
		t.Fatalf("echappement minimal attendu %s, obtenu %s", want, got)
	}
}

func TestEmpreinteStable(t *testing.T) {
	// Meme contenu, ordre de cle different : meme forme canonique, meme empreinte.
	a, _ := Fingerprint(map[string]any{"b": float64(2), "a": float64(1)})
	b, _ := Fingerprint(map[string]any{"a": float64(1), "b": float64(2)})
	if len(a) != 64 || a != b {
		t.Fatalf("empreintes divergentes: %s != %s", a, b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestVecteursPartagesJCS rejoue le vecteur public coord-conformite-jcs-v1
// (mission M-COORD-JCS-01, issue #85) : le MÊME fichier que le runner Python,
// preuve de parite au même artefact (octets, sha256, refus des non-entiers).
func TestVecteursPartagesJCS(t *testing.T) {
	chemin := filepath.Join("..", "..", "..", "docs", "conformite-v15", "ordre_cles_utf16.json")
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("vecteur public illisible (%s): %v", chemin, err)
	}
	var doc struct {
		Vecteurs []struct {
			Nom        string           `json:"nom"`
			Operations []map[string]any `json:"operations"`
			Attendu    map[string]any   `json:"attendu"`
		} `json:"vecteurs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("vecteur public invalide: %v", err)
	}
	if len(doc.Vecteurs) != 3 {
		t.Fatalf("3 vecteurs JCS attendus, obtenu %d", len(doc.Vecteurs))
	}
	for _, v := range doc.Vecteurs {
		var obj any
		for _, op := range v.Operations {
			if str, _ := op["operation"].(string); str == "empreinte" {
				obj = op["objet"]
			}
		}
		octets, err := Canonical(obj)
		att := v.Attendu
		if errAtt, _ := att["erreur"].(bool); errAtt {
			if err == nil {
				t.Fatalf("%s: refus attendu, serialisation produite %s", v.Nom, octets)
			}
			if cont, _ := att["contient"].(string); !contains(err.Error(), cont) {
				t.Fatalf("%s: motif inattendu %q (contient attendu %q)", v.Nom, err.Error(), cont)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: erreur non attendue: %v", v.Nom, err)
		}
		if oct, _ := att["octets"].(string); string(octets) != oct {
			t.Fatalf("%s: octets %s != attendus %s", v.Nom, octets, oct)
		}
		fp, err := Fingerprint(obj)
		if err != nil {
			t.Fatalf("%s: empreinte: %v", v.Nom, err)
		}
		if sha, _ := att["sha256"].(string); fp != sha {
			t.Fatalf("%s: sha256 %s != attendu %s", v.Nom, fp, sha)
		}
	}
}

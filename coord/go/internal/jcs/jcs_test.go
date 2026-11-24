// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package jcs

import "testing"

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

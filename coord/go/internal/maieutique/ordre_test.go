// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"

	"coordengine/internal/jcs"
)

// jsonDecode / jsonEstCanonique : helpers de preuve (forme canonique).
func jsonDecode(raw []byte, v any) error { return json.Unmarshal(raw, v) }

func jsonEstCanonique(raw []byte) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	canonique, err := jcs.Canonical(v)
	if err != nil {
		return false
	}
	return bytes.Equal(canonique, bytes.TrimSpace(raw))
}

// TestValiderOrdreConforme : l'ordre complet passe SANS aucun code.
func TestValiderOrdreConforme(t *testing.T) {
	if codes := ValiderOrdre(ordreComplet()); len(codes) > 0 {
		t.Fatalf("ordre complet rejeté : %v", codes)
	}
}

// TestValiderOrdreCodes : chaque règle déterministe a son code STABLE.
func TestValiderOrdreCodes(t *testing.T) {
	cas := []struct {
		nom    string
		mutate func(o *Ordre)
		code   string
	}{
		{
			nom: "objectif multiple (et)", code: CodeObjectifNonUnique,
			mutate: func(o *Ordre) { o.Objectif = "Qualifier la mission et publier le rapport" },
		},
		{
			nom: "objectif multiple (puis)", code: CodeObjectifNonUnique,
			mutate: func(o *Ordre) { o.Objectif = "Écrire le plan puis le fusionner" },
		},
		{
			nom: "sources vides", code: CodeSourcesVides,
			mutate: func(o *Ordre) { o.Sources = nil },
		},
		{
			nom: "pas de contrôle intermédiaire", code: CodePasDeControle,
			mutate: func(o *Ordre) {
				for i := range o.Etapes {
					o.Etapes[i].ControleIntermediaire = ""
				}
			},
		},
		{
			nom: "livrable imprécis (court)", code: CodeLivrableImprecis,
			mutate: func(o *Ordre) { o.Livrable = "un rapport" },
		},
		{
			nom: "livrable imprécis (interrogatif)", code: CodeLivrableImprecis,
			mutate: func(o *Ordre) { o.Livrable = "un rapport à définir plus tard ?" },
		},
		{
			nom: "étapes incohérentes (doublon)", code: CodeEtapesIncoherentes,
			mutate: func(o *Ordre) { o.Etapes[1].Nom = o.Etapes[0].Nom },
		},
		{
			nom: "ordre non conforme (champ absent)", code: CodeOrdreNonConforme,
			mutate: func(o *Ordre) { o.ApplicationCoord = "" },
		},
	}
	for _, c := range cas {
		o := ordreComplet()
		c.mutate(o)
		codes := ValiderOrdre(o)
		trouve := false
		for _, x := range codes {
			if x == c.code {
				trouve = true
			}
		}
		if !trouve {
			t.Fatalf("%s : code %s attendu, reçus : %v", c.nom, c.code, codes)
		}
	}
}

// TestObjectifUniqueGardeLexicale : la garde est LEXICALE, liste FIXE de
// séparateurs (jamais un jugement) : « et » dans un mot ne compte pas.
func TestObjectifUniqueGardeLexicale(t *testing.T) {
	o := ordreComplet()
	o.Objectif = "Vérifier l'étanchéité du système"
	if codes := ValiderOrdre(o); len(codes) > 0 {
		t.Fatalf("« étanchéité » ne doit pas déclencher OBJECTIF_NON_UNIQUE : %v", codes)
	}
	o.Objectif = "Vérifier le système, en plus de le documenter"
	if codes := ValiderOrdre(o); codes[0] != CodeObjectifNonUnique {
		t.Fatalf("OBJECTIF_NON_UNIQUE attendu, reçus : %v", codes)
	}
}

// TestEmpreinteOrdre : sha256 de la forme canonique JCS, STABLE.
func TestEmpreinteOrdre(t *testing.T) {
	o := ordreComplet()
	e1, err := o.Empreinte()
	if err != nil {
		t.Fatalf("empreinte : %v", err)
	}
	e2, err := o.Empreinte()
	if err != nil || e1 != e2 {
		t.Fatalf("empreinte instable : %s vs %s (%v)", e1, e2, err)
	}
	if len(e1) != 64 {
		t.Fatalf("empreinte sha256 attendue (64 hex), reçu : %s", e1)
	}
	// Le même ordre sous une autre sérialisation a la MÊME empreinte.
	var m map[string]any
	b, _ := json.Marshal(o)
	_ = json.Unmarshal(b, &m)
	e3, err := EmpreinteMap(m)
	if err != nil || e3 != e1 {
		t.Fatalf("empreinte divergente entre formes : %s vs %s (%v)", e1, e3, err)
	}
	// Un octet de différence change l'empreinte.
	o.Livrable = o.Livrable + " (bis)"
	e4, _ := o.Empreinte()
	if e4 == e1 {
		t.Fatalf("deux ordres distincts ont la même empreinte")
	}
}

// TestSeparateursListeFixe : la liste est fermée et triée (documentation
// vivante : tout ajout est un changement de RÈGLE, pas un jugement).
func TestSeparateursListeFixe(t *testing.T) {
	copie := append([]string(nil), separateursObjectif...)
	sort.Strings(copie)
	if len(separateursObjectif) < 5 {
		t.Fatalf("liste de séparateurs incohérente : %v", separateursObjectif)
	}
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"strings"
)

// Ordre de mission qualifié (plan v2 D3, ordre §5-T3) : les dix champs
// exigés par coord/schemas/ordre.schema.json (JSON Schema 2020-12).
// La validation Go est DÉTERMINISTE et structurellement identique au
// schéma : les mêmes règles, aucun jugement sémantique ici.
type Ordre struct {
	Identification    Identification `json:"identification"`
	EtatDesLieux      string         `json:"etat_des_lieux"`
	Sources           []Source       `json:"sources"`
	Objectif          string         `json:"objectif"`
	Etapes            []Etape        `json:"etapes"`
	Limites           []string       `json:"limites"`
	LiberteExecution  string         `json:"liberte_execution"`
	Livrable          string         `json:"livrable"`
	CriteresReception []string       `json:"criteres_reception"`
	ApplicationCoord  string         `json:"application_coord"`
}

// Identification d'un ordre : mission, version du texte, protocole.
type Identification struct {
	Mission   string `json:"mission"`
	Version   string `json:"version"`
	Protocole string `json:"protocole"`
}

// Etape : nom + contrôle intermédiaire obligatoire (au moins une étape
// doit en porter un, plan v2 D3).
type Etape struct {
	Nom                   string `json:"nom"`
	ControleIntermediaire string `json:"controle_intermediaire"`
}

// separateursObjectif : garde LEXICALE de l'objectif UNIQUE. Liste FIXE :
// tout séparateur de coordination présent dans l'objectif signale des
// objectifs multiples (le rejet reste déterministe, jamais subjectif).
var separateursObjectif = []string{
	" et ", " puis ", " ainsi que ", " en plus de ", " par ailleurs ",
	" + ", " ; ", "; ", " aussi ", " également ", " et/ou ",
}

// champsRequis : les dix champs du schéma (ordre §5-T3).
func champsRequis(o *Ordre) map[string]string {
	return map[string]string{
		"identification":     o.Identification.Mission,
		"etat_des_lieux":     o.EtatDesLieux,
		"sources":            "",
		"objectif":           o.Objectif,
		"etapes":             "",
		"limites":            "",
		"liberte_execution":  o.LiberteExecution,
		"livrable":           o.Livrable,
		"criteres_reception": "",
		"application_coord":  o.ApplicationCoord,
	}
}

// ValiderOrdre applique la validation DÉTERMINISTE complète (schéma +
// règles, plan v2 D3) et retourne la liste des codes d'échec STABLES
// (vide = conforme). Cette fonction ne consulte AUCUN service externe :
// les sondes de sources sont séparées (VerifieSources).
func ValiderOrdre(o *Ordre) []string {
	if o == nil {
		return []string{CodeOrdreNonConforme}
	}
	codes := []string{}

	// Structure (équivalent Go du schéma 2020-12, dix champs exigés).
	if strings.TrimSpace(o.Identification.Mission) == "" ||
		strings.TrimSpace(o.Identification.Version) == "" ||
		strings.TrimSpace(o.Identification.Protocole) == "" {
		codes = append(codes, CodeOrdreNonConforme)
	}
	if strings.TrimSpace(o.EtatDesLieux) == "" ||
		strings.TrimSpace(o.Objectif) == "" ||
		strings.TrimSpace(o.LiberteExecution) == "" ||
		strings.TrimSpace(o.Livrable) == "" ||
		strings.TrimSpace(o.ApplicationCoord) == "" {
		codes = append(codes, CodeOrdreNonConforme)
	}
	if len(o.Sources) == 0 {
		codes = append(codes, CodeSourcesVides)
	}
	if len(o.Etapes) == 0 || len(o.Limites) == 0 || len(o.CriteresReception) == 0 {
		codes = append(codes, CodeOrdreNonConforme)
	}

	// Objectif UNIQUE : garde lexicale, liste fixe (déterministe).
	obj := " " + strings.Join(strings.Fields(strings.ToLower(o.Objectif)), " ") + " "
	for _, sep := range separateursObjectif {
		if strings.Contains(obj, strings.ToLower(sep)) {
			codes = append(codes, CodeObjectifNonUnique)
			break
		}
	}

	// Au moins UN contrôle intermédiaire (plan v2 D3).
	controle := false
	for _, e := range o.Etapes {
		if strings.TrimSpace(e.ControleIntermediaire) != "" {
			controle = true
			break
		}
	}
	if !controle {
		codes = append(codes, CodePasDeControle)
	}

	// Livrable PRÉCIS : un livrable trop court ou purement interrogatif
	// est imprécis (garde déterministe sur la longueur et la forme).
	liv := strings.TrimSpace(o.Livrable)
	if len(liv) < 20 || strings.HasSuffix(liv, "?") {
		codes = append(codes, CodeLivrableImprecis)
	}

	// Étapes COHÉRENTES : nom présent, pas de doublon, pas plus de 12
	// étapes (une mission maïeutique reste bornée).
	vus := map[string]bool{}
	if len(o.Etapes) > 12 {
		codes = append(codes, CodeEtapesIncoherentes)
	}
	for _, e := range o.Etapes {
		nom := strings.TrimSpace(e.Nom)
		if nom == "" || vus[nom] {
			codes = append(codes, CodeEtapesIncoherentes)
			break
		}
		vus[nom] = true
	}

	return codes
}

// EstConforme dit si l'ordre passe la validation déterministe complète.
func EstConforme(o *Ordre) bool { return len(ValiderOrdre(o)) == 0 }

// Empreinte : sha256 de la forme canonique JCS de l'ordre (spec 5.3).
// L'ordre est converti en map générique : jcs.Canonical n'accepte pas
// les structs Go.
func (o *Ordre) Empreinte() (string, error) {
	if o == nil {
		return "", erreur(CodeOrdreAbsent, "empreinte d'un ordre absent")
	}
	m, err := enMap(o)
	if err != nil {
		return "", err
	}
	return EmpreinteMap(m)
}

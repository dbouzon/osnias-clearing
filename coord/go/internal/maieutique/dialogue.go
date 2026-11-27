// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// MethodeV1 : instructions méthodologiques du directeur maïeutique,
// versionnées et EMBARQUÉES dans le binaire (plan v2 D4).
//
//go:embed methode/v1.md
var MethodeV1 string

// Actions de réponse du directeur (JSON strict, D4).
const (
	ReponseQuestion         = "QUESTION"
	ReponseClarification    = "CLARIFICATION"
	ReponsePropositionOrdre = "PROPOSITION_ORDRE"
	ReponseVerdict          = "VERDICT"
)

// Sévérités des points de critique (gabarit hérité du moteur).
const (
	SeveriteBloquante = "BLOQUANT"
	SeveriteMajeure   = "MAJEUR"
	SeveriteReserve   = "RESERVE_NON_BLOQUANTE"
)

// Verdicts admis (gabarit du moteur, D4).
var verdictsAdmis = map[string]bool{
	"PLAN_ACCEPTE":            true,
	"A_CORRIGER":              true,
	"RECEPTION_CONFORME":      true,
	"RESERVES_NON_BLOQUANTES": true,
}

// PointDetail : un point de critique du directeur, sévérité + texte.
type PointDetail struct {
	Severite string `json:"severite"`
	Texte    string `json:"texte"`
}

// ReponseDirecteur : la réponse JSON STRICTE d'un tour du directeur
// (D4, fail closed : champs inconnus refusés, forme non validée ici
// pour l'ordre — le rejet non conforme se fait dans evaluerProposition).
type ReponseDirecteur struct {
	Action  string          `json:"action"`
	Contenu string          `json:"contenu,omitempty"`
	Ordre   json.RawMessage `json:"ordre,omitempty"`
	Verdict string          `json:"verdict,omitempty"`
	Points  []PointDetail   `json:"points_detail,omitempty"`
}

// analyserReponseTour décode la sortie du directeur en JSON STRICT :
// champs inconnus refusés (DisallowUnknownFields), action dans l'ensemble
// admis, et cohérence minimale de la forme PAR ACTION. La forme de
// L'ORDRE n'est PAS validée ici : evaluerProposition le fait avec les
// codes stables. Toute non-conformité = REPONSE_INVALIDE, aucune
// interprétation (D4).
func analyserReponseTour(sortie string) (*ReponseDirecteur, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(sortie)))
	dec.DisallowUnknownFields()
	var r ReponseDirecteur
	if err := dec.Decode(&r); err != nil {
		return nil, erreur(CodeReponseInvalide, "réponse du directeur non JSON strict : "+err.Error())
	}
	switch r.Action {
	case ReponseQuestion, ReponseClarification:
		if strings.TrimSpace(r.Contenu) == "" {
			return nil, erreur(CodeReponseInvalide, r.Action+" exige un contenu non vide")
		}
	case ReponsePropositionOrdre:
		if len(r.Ordre) == 0 || string(bytes.TrimSpace(r.Ordre)) == "null" {
			return nil, erreur(CodeReponseInvalide, "PROPOSITION_ORDRE exige l'objet ordre")
		}
	case ReponseVerdict:
		if !verdictsAdmis[r.Verdict] {
			return nil, erreur(CodeReponseInvalide, "verdict inconnu : "+r.Verdict)
		}
		if len(r.Points) == 0 {
			return nil, erreur(CodeReponseInvalide, "VERDICT exige points_detail")
		}
		for _, pt := range r.Points {
			if pt.Severite != SeveriteBloquante && pt.Severite != SeveriteMajeure &&
				pt.Severite != SeveriteReserve {
				return nil, erreur(CodeReponseInvalide, "sévérité inconnue : "+pt.Severite)
			}
		}
	default:
		return nil, erreur(CodeReponseInvalide, "action inconnue : "+r.Action)
	}
	return &r, nil
}

// critiqueBloquante dit si un VERDICT porte un point BLOQUANT ou MAJEUR
// (retour au dialogue, plan v2 D2).
func critiqueBloquante(r *ReponseDirecteur) bool {
	for _, pt := range r.Points {
		if pt.Severite == SeveriteBloquante || pt.Severite == SeveriteMajeure {
			return true
		}
	}
	return false
}

// fenetreDialogue : la fenêtre de contexte BORNÉE du dialogue (D4) :
// budget en octets = ContexteJetonsMax * 4 / 2 (estimation 4 octets par
// jeton, moitié du budget pour l'historique), lignes les plus récentes
// d'abord conservées. Retourne la charge texte de l'historique.
func (s *Store) fenetreDialogue(p *Projet) (string, error) {
	budget := s.cfg.ContexteJetonsMax * 4 / 2
	raw, err := os.ReadFile(s.cheminDialogue(p.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lignes := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	gardes := []string{}
	total := 0
	for i := len(lignes) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lignes[i])
		if l == "" {
			continue
		}
		if total+len(l) > budget {
			break
		}
		gardes = append([]string{l}, gardes...)
		total += len(l)
	}
	return strings.Join(gardes, "\n"), nil
}

// journaliserTour : ajoute un tour au dialogue persistant (dialogue.jsonl,
// D4 : mémoire du binôme, append-only idempotent). Conversion générique :
// les points_detail sont des structs, non canonisables tels quels.
func (s *Store) journaliserTour(p *Projet, tour map[string]any) error {
	g, err := versJSONGenerique(tour)
	if err != nil {
		return err
	}
	entrees, err := enListe([]any{g})
	if err != nil {
		return err
	}
	_, err = persistJournal(entrees, s.cheminDialogue(p.ID))
	return err
}

// construireCharge : la charge complète d'un tour du directeur : méthode
// embarquée + fenêtre d'historique + éventuelle entrée du pilote et
// documents pertinents ENCADRÉS (D4/D5).
func (s *Store) construireCharge(p *Projet, entreePilote string, documents []ResultatRecherche) (string, error) {
	var b strings.Builder
	b.WriteString(MethodeV1)
	b.WriteString("\n\n--- PROJET ---\n")
	b.WriteString("projet : " + p.ID + "\n")
	b.WriteString("intention du pilote : " + p.Intention + "\n")
	if hist, err := s.fenetreDialogue(p); err == nil && hist != "" {
		b.WriteString("\n--- HISTORIQUE (fenêtre bornée) ---\n")
		b.WriteString(hist)
		b.WriteString("\n")
	}
	for _, doc := range documents {
		contenuBrut, err := os.ReadFile(filepath.Join(s.dir(p.ID), "documents", doc.Fichier))
		if err != nil {
			continue
		}
		contenu := string(contenuBrut)
		tronque := false
		if len(contenu) > s.cfg.TailleMaxDocument {
			contenu, tronque = TronquerDocument(contenu, s.cfg.TailleMaxDocument)
		}
		emp := sha256Hex(contenuBrut)
		b.WriteString("\n" + EncadrerDocument(doc.Fichier, contenu, emp) + "\n")
		if tronque {
			b.WriteString("(DOCUMENT_TRONQUE : " + doc.Fichier + " plafonné)\n")
		}
	}
	if strings.TrimSpace(entreePilote) != "" {
		b.WriteString("\n--- ENTREE DU PILOTE (ce tour) ---\n")
		b.WriteString(entreePilote)
		b.WriteString("\n")
	}
	b.WriteString("\nRéponds en JSON STRICT selon la méthode.\n")
	charge := b.String()
	p.ContexteOctets += len(charge)
	return charge, nil
}

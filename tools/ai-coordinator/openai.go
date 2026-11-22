package aic

import (
	"errors"
	"strings"
)

// ReviewInput est la soumission de révision adressée à l'agent chef de projet.
type ReviewInput struct {
	MissionID string
	Report    string
	Diff      string
}

// ReviewComment est un point de revue rattaché à la livraison.
type ReviewComment struct {
	Ref  string // référence GitHub (PR, fichier)
	Note string
}

// ReviewOutput est la réponse structurée de l'agent chef de projet.
type ReviewOutput struct {
	Decision string // ACCEPTED | CORRECTION | BLOCKED
	Comments []ReviewComment
	Summary  string
}

// Reviewer est l'interface de l'agent chef de projet OpenAI.
type Reviewer interface {
	// Review produit un verdict strictement structuré et un rapport de
	// réception avec références GitHub.
	Review(in ReviewInput) (ReviewOutput, error)
}

// SimulatedOpenAI est l'agent chef de projet simulé, pilotable par marqueurs
// posés dans le rapport pour dirige les tests : [FAIL], [BLOCK], sinon ACCEPTED.
type SimulatedOpenAI struct {
	Reviews int // compteur de révisions
}

// Review implémente Reviewer en mode simulé.
func (s *SimulatedOpenAI) Review(in ReviewInput) (ReviewOutput, error) {
	s.Reviews++
	r := strings.ToLower(in.Report)
	switch {
	case strings.Contains(r, "[block]"):
		return ReviewOutput{
			Decision: VerdictBlocked,
			Comments: []ReviewComment{{Ref: "draft", Note: "réserve bloquante détectée"}},
			Summary:  "réception bloquée (simulateur)",
		}, nil
	case strings.Contains(r, "[fail]"):
		return ReviewOutput{
			Decision: VerdictCorrection,
			Comments: []ReviewComment{{Ref: "draft", Note: "tests ou conformité insuffisants : corriger"}},
			Summary:  "correction demandée (simulateur)",
		}, nil
	default:
		return ReviewOutput{
			Decision: VerdictAccepted,
			Comments: []ReviewComment{{Ref: "draft", Note: "livrable conforme"}},
			Summary:  "réception favorable (simulateur)",
		}, nil
	}
}

// APIOpenAI est le squelette d'adaptateur d'API OpenAI. En T02 il refuse
// toute exécution : l'appel d'API payant est interdit sans autorisation (T03).
type APIOpenAI struct{}

// Review refuse l'exécution tant que T03 n'est pas autorisé.
func (APIOpenAI) Review(in ReviewInput) (ReviewOutput, error) {
	return ReviewOutput{}, errors.New("adaptateur API OpenAI non activé en T02 : mode simulé requis (aucun appel payant autorisé)")
}

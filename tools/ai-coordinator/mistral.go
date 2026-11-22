package aic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MistralResult capture le résultat d'une exécution de l'agent développeur.
type MistralResult struct {
	Report string // rapport de tranche produit par l'agent
	Diff   string // différences lues (résumé, sans push)
	Steps  int    // passages d'outils (budget)
	Iters  int    // itérations effectuées
	Err    error  // incident d'exécution éventuel
}

// MistralAdapter est l'interface de l'agent développeur Mistral.
// En T02, seul l'adaptateur simulé est disponible ; l'adaptateur Vibe CLI
// (mode --prompt non interactif) sera câblé en T03 sous autorisation.
type MistralAdapter interface {
	// Run exécute l'agent dans un répertoire de travail isolé, borné en
	// durée et en itérations. L'agent n'a jamais accès à push ni à main.
	Run(ctx context.Context, m *Mission, workDir string) (MistralResult, error)
}

// SimulatedMistral est l'agent développeur simulé pour les tests locaux.
// Il produit un rapport déterministe contrôlable pour diriger les tests.
type SimulatedMistral struct {
	Runs    int      // compteur d'exécutions
	Reports []string // rapports successifs (le dernier est réutilisé)
	Fail    bool     // simule un incident d'exécution
}

// Run implémente MistralAdapter en mode simulé.
func (s *SimulatedMistral) Run(ctx context.Context, m *Mission, workDir string) (MistralResult, error) {
	s.Runs++
	if _, err := os.Stat(workDir); err != nil {
		if err := os.MkdirAll(workDir, 0o750); err != nil {
			return MistralResult{}, err
		}
	}
	if s.Fail {
		return MistralResult{Err: errors.New("erreur simulée de l'agent Mistral")}, errors.New("erreur simulée de l'agent Mistral")
	}
	report := "Tranche exécutée en mode simulé : tests passés, aucun push effectué."
	if n := len(s.Reports); n > 0 {
		idx := s.Runs - 1
		if idx >= n {
			idx = n - 1
		}
		report = s.Reports[idx]
	}
	// Trace d'exécution dans le répertoire isolé (aucune commande arbitraire).
	trace := fmt.Sprintf("mission=%s state=running runs=%d\n", m.ID, s.Runs)
	if err := os.WriteFile(filepath.Join(workDir, "trace.txt"), []byte(trace), 0o640); err != nil {
		return MistralResult{}, err
	}
	return MistralResult{
		Report: report,
		Diff:   "simulated-diff: +12 -3",
		Steps:  4,
		Iters:  3,
	}, nil
}

// APIMistral est le squelette d'adaptateur d'API. En T02 il refuse toute
// exécution : l'appel d'API payant est interdit sans autorisation (T03).
type APIMistral struct{}

// Run refuse l'exécution tant que T03 n'est pas autorisé.
func (APIMistral) Run(ctx context.Context, m *Mission, workDir string) (MistralResult, error) {
	return MistralResult{}, errors.New("adaptateur API Mistral non activé en T02 : mode simulé requis (aucun appel payant autorisé)")
}

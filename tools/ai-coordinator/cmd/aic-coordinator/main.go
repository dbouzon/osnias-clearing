// Command aic-coordinator démarre le coordinateur multi-agents Osnias.
//
// T02 : mode strictement local et simulé. Aucune clé d'API n'est requise ;
// si un mode api est demandé sans autorisation T03, le coordinateur refuse
// toute exécution (échec explicite, jamais silencieux).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := aic.ConfigFromEnv()
	if cfg.WebhookSecret == "" {
		logger.Error("AIC_WEBHOOK_SECRET est requis : aucun webhook ne sera accepté sans secret")
		os.Exit(2)
	}
	store, err := aic.OpenStore(cfg.DBPath)
	if err != nil {
		logger.Error("ouverture du stockage", "err", err.Error())
		os.Exit(1)
	}
	defer store.Close()

	// Adaptateurs T02 : simulés uniquement. Les adaptateurs API refusent
	// toute exécution tant que T03 n'est pas autorisé. La déduplication de
	// l'adaptateur GitHub simulé est rendue durable (T02-R4).
	var mistral aic.MistralAdapter = &aic.SimulatedMistral{}
	var reviewer aic.Reviewer = &aic.SimulatedOpenAI{}
	githubSim := &aic.SimulatedGitHub{}
	githubSim.UseKeyStore(store)
	switch cfg.MistralMode {
	case aic.ModeSimulated:
		// simulé (défaut)
	case aic.ModeAPI:
		logger.Error("AIC_MISTRAL_MODE=api n'est pas autorisé en T02 : aucun appel payant")
		os.Exit(2)
	default:
		logger.Error("AIC_MISTRAL_MODE inconnu", "mode", cfg.MistralMode)
		os.Exit(2)
	}
	if cfg.OpenAIMode == aic.ModeAPI {
		logger.Error("AIC_OPENAI_MODE=api n'est pas autorisé en T02 : aucun appel payant")
		os.Exit(2)
	}
	// Habilitation (T02-R1) : sans identité autorisée, aucune commande /aic
	// ne sera acceptée. Refus explicite au démarrage plutôt qu'un échec muet.
	if len(cfg.AuthorizedLogins) == 0 {
		logger.Error("AIC_AUTHORIZED_LOGINS est vide : aucune identité habilitée (politique fail closed)")
		os.Exit(2)
	}

	coord := &aic.Coordinator{
		Store: store, Config: cfg,
		Mistral: mistral, Reviewer: reviewer, GitHub: githubSim,
	}
	// Contexte d'exploitation : annulé proprement à l'arrêt.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// T02-R2/T02-R3 : reprise des événements acquittés mais non traités
	// (arrêt entre réponse 2xx et traitement asynchrone), puis relance
	// périodique pendant le fonctionnement normal : les commandes humaines
	// ne sont jamais perdues et n'attendent pas un redémarrage.
	aic.DrainPendingEvents(ctx, coord, logger)
	if err := coord.Resume(ctx); err != nil {
		logger.Error("reprise des missions", "err", err.Error())
		os.Exit(1)
	}
	srv := aic.NewServer(coord, logger)
	go aic.StartPendingRetryLoop(ctx, coord, logger)
	go func() {
		logger.Info("coordinateur démarré", "addr", cfg.ListenAddr, "repo", cfg.GitHubRepo)
		if err := srv.ListenAndServe(); err != nil {
			logger.Error("serveur", "err", err.Error())
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	logger.Info("arrêt du coordinateur")
	cancel()
	_ = srv.Close()
}

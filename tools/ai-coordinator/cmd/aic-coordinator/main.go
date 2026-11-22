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

	// Adaptateurs : Mistral et OpenAI restent simulés (hors périmètre T03) ;
	// GitHub dispose d'un adaptateur réel (T03) désactivé par défaut :
	// AIC_GITHUB_MODE=off (défaut) conserve le simulateur, seul
	// AIC_GITHUB_MODE=real active l'intégration go-github.
	var mistral aic.MistralAdapter = &aic.SimulatedMistral{}
	var reviewer aic.Reviewer = &aic.SimulatedOpenAI{}
	githubSim := &aic.SimulatedGitHub{}
	githubSim.UseKeyStore(store)
	var githubClient aic.GitHubClient = githubSim
	switch cfg.GitHubMode {
	case "off":
		// simulé (défaut) : aucun accès réseau GitHub.
	case "real":
		// T03 : adaptateur réel go-github, déduplication durable sur le Store.
		// Le jeton transite uniquement en mémoire, jamais dans les journaux.
		real, err := aic.NewRealGitHub(aic.RealGitHubOptions{
			Token:   cfg.GitHubToken,
			APIURL:  cfg.GitHubAPIURL,
			Repo:    cfg.GitHubRepo,
			Timeout: cfg.GitHubTimeout,
		}, store)
		if err != nil {
			logger.Error("adaptateur GitHub réel invalide", "err", err.Error())
			os.Exit(2)
		}
		logger.Info("adaptateur GitHub réel activé (T03)", "repo", cfg.GitHubRepo)
		githubClient = real
	default:
		logger.Error("AIC_GITHUB_MODE inconnu", "mode", cfg.GitHubMode)
		os.Exit(2)
	}
	switch cfg.MistralMode {
	case aic.ModeSimulated:
		// simulé (défaut)
	case aic.ModeAPI:
		logger.Error("AIC_MISTRAL_MODE=api n'est pas autorisé en T03 : raccordement Mistral hors périmètre")
		os.Exit(2)
	default:
		logger.Error("AIC_MISTRAL_MODE inconnu", "mode", cfg.MistralMode)
		os.Exit(2)
	}
	// M-AIC-04 : adaptateur réel OpenAI, clé d'API en mémoire uniquement,
	// journalisation SQLite des appels et réponses. Fail closed sans clé.
	switch cfg.OpenAIMode {
	case aic.ModeSimulated:
		// simulé (défaut)
	case aic.ModeAPI:
		if cfg.OpenAIKey == "" {
			logger.Error("AIC_OPENAI_APIKEY est requis en mode api : refus de démarrer sans clé (fail closed)")
			os.Exit(2)
		}
		realReviewer, err := aic.NewRealOpenAI(aic.RealOpenAIOptions{
			APIKey:    cfg.OpenAIKey,
			APIURL:    cfg.OpenAIAPIURL,
			Model:     cfg.OpenAIModel,
			Timeout:   cfg.OpenAITimeout,
			MaxTokens: cfg.OpenAIMaxTok,
			Log:       func(missionID, phase, message string) { _ = store.Log(missionID, "openai", phase, message) },
		})
		if err != nil {
			logger.Error("adaptateur API OpenAI invalide", "err", err.Error())
			os.Exit(2)
		}
		logger.Info("adaptateur API OpenAI activé (M-AIC-04)", "model", cfg.OpenAIModel)
		reviewer = realReviewer
	default:
		logger.Error("AIC_OPENAI_MODE inconnu", "mode", cfg.OpenAIMode)
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
		Mistral: mistral, Reviewer: reviewer, GitHub: githubClient,
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

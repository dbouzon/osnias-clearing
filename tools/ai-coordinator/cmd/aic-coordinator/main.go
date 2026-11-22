// Command aic-coordinator démarre le coordinateur multi-agents Osnias.
//
// Modes simulés par défaut (aucune clé requise). Les modes api (M-AIC-04 /
// M-AIC-05) exigent une configuration explicite : clé fournie par
// l'environnement, jamais journalisée ; à défaut, refus de démarrage
// (fail closed, échec explicite jamais silencieux).
//
// Sous-commande « chat » (M-AIC-10 T02) : dialogue interactif sur le canal
// bidirectionnel ChatGPT <-> Mistral, sans serveur ni recompilation :
//
//	ai-coordinator chat [-mission ID] [-db chemin]
//
// Chaque saisie est acheminée par Channel.Deliver vers UN SEUL destinataire
// au choix (mistral ou chatgpt) ; l'historique SQLite persiste entre les
// échanges et les redémarrages (phase "channel" de agent_logs).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

func main() {
	// Sous-commande locale « chat » (M-AIC-10 T02) : dialogue interactif sur
	// le canal, sans démarrer le serveur. Le fonctionnement du serveur est
	// strictement inchangé : sans argument, la voie serveur ci-dessous
	// s'exécute à l'identique.
	if len(os.Args) > 1 && os.Args[1] == "chat" {
		runChat(os.Args[2:])
		return
	}
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
	// M-AIC-05 : adaptateur réel Mistral, clé d'API en mémoire uniquement.
	// Aucun appel payant n'est possible sans configuration explicite
	// (AIC_MISTRAL_MODE=api ET AIC_MISTRAL_APIKEY fournie : fail closed).
	var realMistral *aic.RealMistral
	switch cfg.MistralMode {
	case aic.ModeSimulated:
		// simulé (défaut)
	case aic.ModeAPI:
		if cfg.MistralAPIKey == "" {
			logger.Error("AIC_MISTRAL_APIKEY est requis en mode api : refus de démarrer sans clé (fail closed)")
			os.Exit(2)
		}
		rm, err := aic.NewRealMistral(aic.RealMistralOptions{
			APIKey:    cfg.MistralAPIKey,
			APIURL:    cfg.MistralAPIURL,
			Model:     cfg.MistralModel,
			Timeout:   cfg.MistralTimeout,
			MaxTokens: cfg.MistralMaxTok,
			Log:       func(missionID, phase, message string) { _ = store.Log(missionID, "mistral", phase, message) },
		})
		if err != nil {
			logger.Error("adaptateur API Mistral invalide", "err", err.Error())
			os.Exit(2)
		}
		logger.Info("adaptateur API Mistral activé (M-AIC-05)", "model", cfg.MistralModel)
		realMistral = rm
		mistral = rm
	default:
		logger.Error("AIC_MISTRAL_MODE inconnu", "mode", cfg.MistralMode)
		os.Exit(2)
	}
	// M-AIC-04 : adaptateur réel OpenAI, clé d'API en mémoire uniquement,
	// journalisation SQLite des appels et réponses. Fail closed sans clé.
	var realReviewer *aic.RealOpenAI
	switch cfg.OpenAIMode {
	case aic.ModeSimulated:
		// simulé (défaut)
	case aic.ModeAPI:
		if cfg.OpenAIKey == "" {
			logger.Error("AIC_OPENAI_APIKEY est requis en mode api : refus de démarrer sans clé (fail closed)")
			os.Exit(2)
		}
		rr, err := aic.NewRealOpenAI(aic.RealOpenAIOptions{
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
		reviewer = rr
		realReviewer = rr
	default:
		logger.Error("AIC_OPENAI_MODE inconnu", "mode", cfg.OpenAIMode)
		os.Exit(2)
	}
	// M-AIC-05 : relais de messagerie authentifié. Actif seulement si les
	// DEUX adaptateurs réels et le jeton dédié AIC_MESSAGE_TOKEN sont fournis ;
	// sinon la route /aic/message répond 501 (aucune exposition non
	// authentifiée, y compris à travers un tunnel public).
	var relay *aic.Relay
	if cfg.MessageToken != "" {
		if realMistral == nil || realReviewer == nil {
			logger.Error("AIC_MESSAGE_TOKEN fourni mais adaptateurs réels incomplets : relais refusé (fail closed)")
			os.Exit(2)
		}
		relay, err = aic.NewRelay(store, realMistral, realReviewer, cfg.MessageToken)
		if err != nil {
			logger.Error("relais invalide", "err", err.Error())
			os.Exit(2)
		}
		logger.Info("relais de messagerie activé (M-AIC-05)", "route", "/aic/message")
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
	var srv *http.Server
	if relay != nil {
		srv = aic.NewServer(coord, logger, relay)
	} else {
		srv = aic.NewServer(coord, logger)
	}
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

// runChat exécute le dialogue interactif du canal bidirectionnel M-AIC-10 :
// réutilise Channel.Deliver au-dessus du relais existant (adaptateurs réels
// Mistral et OpenAI, configuration par environnement). Sans serveur, sans
// recompilation entre les messages ; l'historique SQLite (phase "channel")
// persiste entre les échanges et les redémarrages du programme.
//
// Sécurité : clés uniquement en mémoire, jamais affichées ni journalisées ;
// fail closed sans les deux clés (aucun appel payant possible sinon) ;
// aucune boucle autonome : seul l'appui sur Entrée engage un appel d'API.
// Chaque message reçu un identifiant unique (hex du temps, préfixe chan-)
// garantissant l'idempotence à travers les redémarrages.
func runChat(args []string) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := aic.ConfigFromEnv()

	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	missionID := fs.String("mission", "aic-10-chat", "identifiant de corrélation du dialogue")
	dbPath := fs.String("db", cfg.DBPath, "chemin de la base SQLite")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if cfg.MistralAPIKey == "" || cfg.OpenAIKey == "" {
		logger.Error("AIC_MISTRAL_APIKEY et AIC_OPENAI_APIKEY sont requis : refus d'exécuter sans clés (fail closed)")
		os.Exit(2)
	}

	store, err := aic.OpenStore(*dbPath)
	if err != nil {
		logger.Error("ouverture du stockage", "err", err.Error())
		os.Exit(1)
	}
	// Arrêt propre Ctrl+C : fermeture de la base avant la sortie.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\n[arrêt signalé] dialogue close, base refermée.")
		_ = store.Close()
		os.Exit(0)
	}()
	defer store.Close()

	mistral, err := aic.NewRealMistral(aic.RealMistralOptions{
		APIKey:    cfg.MistralAPIKey,
		APIURL:    cfg.MistralAPIURL,
		Model:     cfg.MistralModel,
		Timeout:   cfg.MistralTimeout,
		MaxTokens: cfg.MistralMaxTok,
		Log:       func(m string, phase, message string) { _ = store.Log(m, "mistral", phase, message) },
	})
	if err != nil {
		logger.Error("adaptateur API Mistral invalide", "err", err.Error())
		os.Exit(2)
	}
	openai, err := aic.NewRealOpenAI(aic.RealOpenAIOptions{
		APIKey:    cfg.OpenAIKey,
		APIURL:    cfg.OpenAIAPIURL,
		Model:     cfg.OpenAIModel,
		Timeout:   cfg.OpenAITimeout,
		MaxTokens: cfg.OpenAIMaxTok,
		Log:       func(m string, phase, message string) { _ = store.Log(m, "openai", phase, message) },
	})
	if err != nil {
		logger.Error("adaptateur API OpenAI invalide", "err", err.Error())
		os.Exit(2)
	}
	// Jeton local : le code de relais est identique à la voie HTTP et ne
	// s'exécute jamais sans secret, même en exécution locale.
	relay, err := aic.NewRelay(store, mistral, openai, "local-chat-canal")
	if err != nil {
		logger.Error("relais invalide", "err", err.Error())
		os.Exit(2)
	}
	channel, err := aic.NewChannel(relay)
	if err != nil {
		logger.Error("canal invalide", "err", err.Error())
		os.Exit(2)
	}

	to := aic.PartyMistral // destinataire courant, modifiable par « /to »
	fmt.Printf("Osnias AI Coordinator — canal ChatGPT <-> Mistral (dialogue %s)\n", *missionID)
	fmt.Println("Commandes : /to mistral ou /to chatgpt (choix du destinataire), /help, /exit.")
	fmt.Printf("Destinataire courant : %s. Base : %s (historique conservé entre les redémarrages).\n", to, *dbPath)

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for {
		fmt.Printf("\n[%s] > ", to)
		if !sc.Scan() {
			break
		}
		body := strings.TrimSpace(sc.Text())
		if body == "" {
			continue
		}
		switch strings.ToLower(body) {
		case "/exit", "/quit", "/quitter":
			fmt.Println("dialogue close par commande.")
			return
		case "/help", "/aide":
			fmt.Println("Saisissez un message pour l'envoyer au destinataire courant ; /to mistral ou /to chatgpt pour le changer ; /exit pour arrêter.")
			continue
		}
		if prefix, rest, ok := strings.Cut(body, " "); ok && strings.EqualFold(prefix, "/to") {
			target := strings.ToLower(strings.TrimSpace(rest))
			if target != aic.PartyMistral && target != aic.PartyChatGPT {
				fmt.Printf("[erreur] destinataire inconnu %q : mistral ou chatgpt\n", target)
				continue
			}
			to = target
			fmt.Printf("destinataire courant : %s\n", to)
			continue
		}
		messageID := fmt.Sprintf("chan-%x", time.Now().UnixNano())
		resp, err := channel.Deliver(aic.ChannelRequest{
			MissionID: *missionID,
			MessageID: messageID,
			From:      aic.PartyHuman,
			To:        to,
			Body:      body,
		}, store.Log)
		if err != nil {
			fmt.Printf("[erreur] %s\n", err.Error())
			if resp != nil {
				fmt.Printf("  statut=%s\n", resp.Status)
			}
			continue
		}
		fmt.Printf("[%s -> %s | message_id=%s]\n%s\n", resp.From, resp.To, resp.MessageID, resp.Reply)
	}
	fmt.Println()
}

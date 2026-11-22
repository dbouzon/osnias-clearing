// Command aic-chat ouvre un dialogue interactif continu Mistral ↔ OpenAI
// dans le terminal (M-AIC-06, issue #10) : chaque saisie déclenche un tour
// complet relais (saisie -> Mistral API -> OpenAI API -> verdict affiché),
// sans relancer le programme. L'historique complet est dans SQLite sous
// l'identifiant de session (mission).
//
// Sécurité identique à aic-dialog : clés uniquement en environnement,
// mémoire uniquement, jamais affichées, journalisées ni persistées ; fail
// closed sans les deux clés ; bornes de tours et de jetons ; aucun appel en
// boucle autonome : seul l'appui sur Entrée engage un tour.
//
// Commandes : exit ou quit pour arrêter, help pour l'aide.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := aic.ConfigFromEnv()

	missionID := flag.String("session", "aic-06-chat", "identifiant de corrélation de la session")
	dbPath := flag.String("db", cfg.DBPath, "chemin de la base SQLite")
	flag.Parse()

	if cfg.MistralAPIKey == "" || cfg.OpenAIKey == "" {
		logger.Error("AIC_MISTRAL_APIKEY et AIC_OPENAI_APIKEY sont requis : refus d'exécuter sans clés (fail closed)")
		os.Exit(2)
	}

	store, err := aic.OpenStore(*dbPath)
	if err != nil {
		logger.Error("ouverture du stockage", "err", err.Error())
		os.Exit(1)
	}
	// Arrêt propre Ctrl+C : le signal déclenche la fermeture du stockage.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\n[arrêt signalé] session close, base refermée.")
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
	relay, err := aic.NewRelay(store, mistral, openai, "local-chat")
	if err != nil {
		logger.Error("relais invalide", "err", err.Error())
		os.Exit(2)
	}

	session, err := aic.NewChatSession(relay, *missionID,
		cfg.SessionMaxTurns, cfg.SessionTokenBudget, cfg.MistralMaxTok, cfg.OpenAIMaxTok)
	if err != nil {
		logger.Error("session invalide", "err", err.Error())
		os.Exit(2)
	}

	fmt.Printf("Osnias AI Coordinator — dialogue interactif (session %s)\n", *missionID)
	fmt.Printf("Bornes : %d tours, budget pire cas %d jetons. Commandes : exit/quit, help.\n",
		session.MaxTurns, session.TokenBudget)

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for {
		fmt.Printf("\n[%d/%d tours, ~%d/%d j] > ", session.Turns(), session.MaxTurns, session.TokensSpent(), session.TokenBudget)
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		switch line {
		case "":
			continue
		case "exit", "quit", "quitter":
			fmt.Println("session close par commande.")
			return
		case "help", "aide":
			fmt.Println("Saisissez un message pour un tour Mistral -> OpenAI ; exit/quit pour arrêter.")
			continue
		}
		resp, err := session.Say(line, "humain", store.Log)
		if err != nil {
			fmt.Printf("[erreur] %s\n", err.Error())
			if resp != nil {
				fmt.Printf("  statut=%s note=%s\n", resp.Status, resp.Note)
				if resp.Mistral != "" {
					fmt.Printf("[ réponse Mistral, tour %d ]\n%s\n", session.Turns(), resp.Mistral)
				}
			}
			continue
		}
		fmt.Printf("[ tour %d — message_id=%s ]\nMistral : %s\n", session.Turns(), resp.MessageID, resp.Mistral)
		if resp.OpenAI != nil {
			fmt.Printf("OpenAI (%s) : %s\n", resp.OpenAI.Decision, resp.OpenAI.Summary)
			for _, c := range resp.OpenAI.Comments {
				fmt.Printf("  - %s : %s\n", c.Ref, c.Note)
			}
		}
	}
	fmt.Println()
}

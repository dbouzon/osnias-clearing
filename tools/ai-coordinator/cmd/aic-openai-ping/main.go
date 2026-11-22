// Command aic-openai-ping réalise le premier appel authentifié vers l'API
// OpenAI et journalise l'échange dans SQLite (mission M-AIC-04).
//
// La clé d'API est lue depuis AIC_OPENAI_APIKEY, reste en mémoire et n'est
// jamais journalisée ni persistée. Sans clé, la commande refuse de s'exécuter
// (fail closed). Aucune réponse n'est fabriquée : en cas d'échec, l'erreur
// est rapportée telle quelle.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

// premierMessage est le message de test fixé par l'ordre de mission M-AIC-04.
const premierMessage = "Bonjour OpenAI. Je suis Mistral Code. Ceci est notre premier test de communication directe via Osnias AI Coordinator. Confirme la réception et indique si nous pouvons poursuivre notre dialogue par API."

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := aic.ConfigFromEnv()

	missionID := flag.String("mission", "aic-04-ping", "identifiant de corrélation de la mission")
	dbPath := flag.String("db", cfg.DBPath, "chemin de la base SQLite")
	timeout := flag.Duration("timeout", cfg.OpenAITimeout, "délai maximal de l'appel API")
	flag.Parse()

	if cfg.OpenAIKey == "" {
		logger.Error("AIC_OPENAI_APIKEY est absent : refus d'exécuter un appel sans clé (fail closed)")
		os.Exit(2)
	}

	store, err := aic.OpenStore(*dbPath)
	if err != nil {
		logger.Error("ouverture du stockage", "err", err.Error())
		os.Exit(1)
	}
	defer store.Close()

	reviewer, err := aic.NewRealOpenAI(aic.RealOpenAIOptions{
		APIKey:    cfg.OpenAIKey,
		APIURL:    cfg.OpenAIAPIURL,
		Model:     cfg.OpenAIModel,
		Timeout:   *timeout,
		MaxTokens: cfg.OpenAIMaxTok,
		Log:       func(m string, phase, message string) { _ = store.Log(m, "openai", phase, message) },
	})
	if err != nil {
		logger.Error("adaptateur API OpenAI invalide", "err", err.Error())
		os.Exit(2)
	}

	if err := store.Log(*missionID, "mistral", "message", premierMessage); err != nil {
		logger.Error("journalisation du message envoyé", "err", err.Error())
		os.Exit(1)
	}

	started := time.Now()
	out, err := reviewer.Call(*missionID, premierMessage)
	elapsed := time.Since(started)
	if err != nil {
		_ = store.Log(*missionID, "mistral", "error", fmt.Sprintf("appel API OpenAI échoué : %v", err))
		logger.Error("appel API OpenAI", "err", err.Error(), "mission", *missionID)
		os.Exit(1)
	}

	if err := store.Log(*missionID, "mistral", "result", fmt.Sprintf("réponse OpenAI reçue en %s : décision=%s résumé=%s", elapsed, out.Decision, out.Summary)); err != nil {
		logger.Error("journalisation du résultat", "err", err.Error())
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"mission": *missionID,
		"model":   cfg.OpenAIModel,
		"elapsed": elapsed.String(),
		"output":  out,
	})
}

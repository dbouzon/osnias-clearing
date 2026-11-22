// Command aic-dialog exécute le test réel M-AIC-05 : un message est transmis
// à l'agent Mistral (API réelle), dont la réponse est soumise à l'agent
// OpenAI (API réelle). L'échange complet est journalisé dans SQLite avec
// un identifiant de corrélation unique.
//
// Les clés sont lues depuis l'environnement (AIC_MISTRAL_APIKEY,
// AIC_OPENAI_APIKEY), restent en mémoire et ne sont jamais journalisées.
// Sans les deux clés, la commande refuse de s'exécuter (fail closed).
// Aucune réponse n'est fabriquée : en cas d'échec, l'erreur est rapportée.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	aic "github.com/dbouzon/osnias-clearing/tools/ai-coordinator"
)

// premierMessage est le message de test fixé par la mission M-AIC-05.
const premierMessage = "Bonjour Mistral Code. Ceci est notre premier test de communication directe via Osnias AI Coordinator. Confirme la réception et indique si nous pouvons poursuivre notre dialogue par API."

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := aic.ConfigFromEnv()

	missionID := flag.String("mission", "aic-05-dialog", "identifiant de corrélation de la mission")
	messageID := flag.String("message-id", "", "identifiant d'idempotence (défaut : mission automatique)")
	dbPath := flag.String("db", cfg.DBPath, "chemin de la base SQLite")
	flag.Parse()

	if cfg.MistralAPIKey == "" || cfg.OpenAIKey == "" {
		logger.Error("AIC_MISTRAL_APIKEY et AIC_OPENAI_APIKEY sont requis : refus d'exécuter sans clés (fail closed)")
		os.Exit(2)
	}
	if *messageID == "" {
		*messageID = *missionID + "-001"
	}

	store, err := aic.OpenStore(*dbPath)
	if err != nil {
		logger.Error("ouverture du stockage", "err", err.Error())
		os.Exit(1)
	}
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
	// Le relais exige un jeton même en exécution locale : le code de relais
	// est identique à la voie HTTP et ne doit jamais s'exécuter sans secret.
	relay, err := aic.NewRelay(store, mistral, openai, "local-dialog")
	if err != nil {
		logger.Error("relais invalide", "err", err.Error())
		os.Exit(2)
	}

	resp, err := relay.Relay(aic.RelayRequest{
		MissionID: *missionID,
		MessageID: *messageID,
		From:      "humain",
		Body:      premierMessage,
	}, store.Log)
	if err != nil {
		logger.Error("relais", "err", err.Error(), "mission", *missionID, "message_id", *messageID)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "vérification : SELECT agent, phase, message FROM agent_logs WHERE mission_id='"+*missionID+"';")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

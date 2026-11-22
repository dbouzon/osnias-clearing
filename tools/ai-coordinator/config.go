// Package aic implémente le coordinateur multi-agents Osnias (M-AIC-01 / T02).
//
// Le coordinateur relie GitHub (Issues = missions, PR Draft = livraisons),
// un agent développeur Mistral (Vibe CLI en mode non interactif) et un agent
// chef de projet OpenAI. Il est indépendant du Core Osnias et portable Linux.
//
// En T02, tous les adaptateurs externes sont simulés : aucun secret réel,
// aucun appel d'API payant, aucune ressource cloud n'est requis.
package aic

import (
	"os"
	"strconv"
	"time"
)

// Constantes d'état de la machine d'états des missions.
const (
	StateOpen             = "OPEN"
	StateRunning          = "RUNNING"
	StateReview           = "REVIEW"
	StateCorrection       = "CORRECTION"
	StateAwaitingApproval = "AWAITING_APPROVAL"
	StateAccepted         = "ACCEPTED"
	StateBlocked          = "BLOCKED"
)

// Verdicts de l'agent de réception OpenAI.
const (
	VerdictAccepted   = "ACCEPTED"
	VerdictCorrection = "CORRECTION"
	VerdictBlocked    = "BLOCKED"
)

// Modes d'adaptateurs.
const (
	ModeSimulated = "simulated"
	ModeAPI       = "api"
)

// Config porte la configuration du coordinateur, exclusivement par variables
// d'environnement. Aucune valeur secrète n'est journalisée ni versionnée.
type Config struct {
	// HTTP
	ListenAddr string // AIC_LISTEN_ADDR (défaut 127.0.0.1:8080)
	// GitHub
	WebhookSecret    string // AIC_WEBHOOK_SECRET (obligatoire en production)
	CoordinatorLogin string // AIC_COORDINATOR_LOGIN, compte bot émetteur (anti-boucle)
	GitHubRepo       string // AIC_GITHUB_REPO (défaut dbouzon/osnias-clearing)
	// Mistral
	MistralMode        string        // AIC_MISTRAL_MODE : simulated | api
	MistralTimeout     time.Duration // AIC_MISTRAL_TIMEOUT (défaut 600s)
	MistralMaxIter     int           // AIC_MISTRAL_MAX_ITER (défaut 40 itérations CLI)
	MistralBudgetSteps int           // AIC_MISTRAL_BUDGET_STEPS (défaut 50 passages d'outils)
	// OpenAI
	OpenAIMode string // AIC_OPENAI_MODE : simulated | api
	// Divers
	DBPath           string        // AIC_DB_PATH (défaut ai-coordinator.sqlite)
	WorkRoot         string        // AIC_WORK_ROOT (défaut /tmp/aic-work)
	MaxCorrections   int           // AIC_MAX_CORRECTIONS (défaut 3)
	RunStaleInterval time.Duration // AIC_RUN_STALE_INTERVAL (défaut 86400s)
}

// fromEnvInt lit un entier depuis l'environnement avec valeur par défaut.
func fromEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// fromEnvDur lit une durée en secondes depuis l'environnement avec défaut.
func fromEnvDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

// fromEnvStr lit une chaîne depuis l'environnement avec valeur par défaut.
func fromEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv construit la configuration depuis l'environnement.
func ConfigFromEnv() Config {
	return Config{
		ListenAddr:         fromEnvStr("AIC_LISTEN_ADDR", "127.0.0.1:8080"),
		WebhookSecret:      os.Getenv("AIC_WEBHOOK_SECRET"),
		CoordinatorLogin:   fromEnvStr("AIC_COORDINATOR_LOGIN", "osnias-aic-bot"),
		GitHubRepo:         fromEnvStr("AIC_GITHUB_REPO", "dbouzon/osnias-clearing"),
		MistralMode:        fromEnvStr("AIC_MISTRAL_MODE", ModeSimulated),
		MistralTimeout:     fromEnvDur("AIC_MISTRAL_TIMEOUT", 600*time.Second),
		MistralMaxIter:     fromEnvInt("AIC_MISTRAL_MAX_ITER", 40),
		MistralBudgetSteps: fromEnvInt("AIC_MISTRAL_BUDGET_STEPS", 50),
		OpenAIMode:         fromEnvStr("AIC_OPENAI_MODE", ModeSimulated),
		DBPath:             fromEnvStr("AIC_DB_PATH", "ai-coordinator.sqlite"),
		WorkRoot:           fromEnvStr("AIC_WORK_ROOT", "/tmp/aic-work"),
		MaxCorrections:     fromEnvInt("AIC_MAX_CORRECTIONS", 3),
		RunStaleInterval:   fromEnvDur("AIC_RUN_STALE_INTERVAL", 86400*time.Second),
	}
}

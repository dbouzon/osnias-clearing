package aic

import (
	"os"
	"strconv"
	"strings"
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
	ModeCLI       = "cli" // M-AIC-02 : adaptateur CLI Mistral Code
)

// Config porte la configuration du coordinateur, exclusivement par variables
// d'environnement. Aucune valeur secrète n'est journalisée ni versionnée.
type Config struct {
	// HTTP
	ListenAddr string // AIC_LISTEN_ADDR (défaut 127.0.0.1:8080)
	// GitHub
	WebhookSecret    string   // AIC_WEBHOOK_SECRET (obligatoire en production)
	CoordinatorLogin string   // AIC_COORDINATOR_LOGIN, compte bot émetteur (anti-boucle)
	AuthorizedLogins []string // AIC_AUTHORIZED_LOGINS, seules identités habilitées à commander (fail closed)
	GitHubRepo       string   // AIC_GITHUB_REPO (défaut dbouzon/osnias-clearing)
	// Mistral
	MistralMode        string        // AIC_MISTRAL_MODE : simulated | cli | api (api refusé)
	MistralTimeout     time.Duration // AIC_MISTRAL_TIMEOUT (défaut 600s)
	MistralMaxIter     int           // AIC_MISTRAL_MAX_ITER (défaut 40 itérations CLI)
	MistralBudgetSteps int           // AIC_MISTRAL_BUDGET_STEPS (défaut 50 passages d'outils)
	MistralBin         string        // AIC_MISTRAL_BIN (défaut vibe) : binaire Mistral Code
	MistralArgs        []string      // AIC_MISTRAL_ARGS (défaut --prompt --trust) : mode non interactif
	MistralAPIKey      string        // AIC_MISTRAL_API_KEY : clé d'API, mémoire uniquement, jamais journalisée
	// OpenAI
	OpenAIMode string // AIC_OPENAI_MODE : simulated | api
	// Divers
	DBPath               string        // AIC_DB_PATH (défaut ai-coordinator.sqlite)
	WorkRoot             string        // AIC_WORK_ROOT (défaut /tmp/aic-work)
	MaxCorrections       int           // AIC_MAX_CORRECTIONS (défaut 3)
	RunStaleInterval     time.Duration // AIC_RUN_STALE_INTERVAL (défaut 86400s)
	PendingRetryInterval time.Duration // AIC_PENDING_RETRY_INTERVAL (défaut 60s, relance à chaud de la file T02-R3)

	GitHubMode    string        // AIC_GITHUB_MODE : off | real (défaut off, adaptateur réel désactivé par défaut)
	GitHubToken   string        // AIC_GITHUB_TOKEN : jeton à permissions minimales (jamais journalisé)
	GitHubAPIURL  string        // AIC_GITHUB_APIURL : racine complète de l'API alternative (tests ; GHE : https://hote/api/v3)
	GitHubTimeout time.Duration // AIC_GITHUB_TIMEOUT (défaut 30s par requête)
}

// IsAuthorized applique la politique d'habilitation des commandes /aic :
// seule une identité explicitement listée dans AIC_AUTHORIZED_LOGINS est
// autorisée. La liste vide refuse tout le monde (fail closed). L'auteur est
// l'identité authentifiée par GitHub dans le webhook (sender), distincte de la
// vérification HMAC qui authentifie le webhook lui-même.
func (c Config) IsAuthorized(login string) bool {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return false
	}
	for _, a := range c.AuthorizedLogins {
		if strings.EqualFold(strings.TrimSpace(a), login) {
			return true
		}
	}
	return false
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

// fromEnvList lit une liste séparée par des virgules (espaces ignorés).
func fromEnvList(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// ConfigFromEnv construit la configuration depuis l'environnement.
func ConfigFromEnv() Config {
	return Config{
		ListenAddr:           fromEnvStr("AIC_LISTEN_ADDR", "127.0.0.1:8080"),
		WebhookSecret:        os.Getenv("AIC_WEBHOOK_SECRET"),
		CoordinatorLogin:     fromEnvStr("AIC_COORDINATOR_LOGIN", "osnias-aic-bot"),
		AuthorizedLogins:     fromEnvList("AIC_AUTHORIZED_LOGINS"),
		GitHubRepo:           fromEnvStr("AIC_GITHUB_REPO", "dbouzon/osnias-clearing"),
		MistralMode:          fromEnvStr("AIC_MISTRAL_MODE", ModeSimulated),
		MistralTimeout:       fromEnvDur("AIC_MISTRAL_TIMEOUT", 600*time.Second),
		MistralMaxIter:       fromEnvInt("AIC_MISTRAL_MAX_ITER", 40),
		MistralBudgetSteps:   fromEnvInt("AIC_MISTRAL_BUDGET_STEPS", 50),
		MistralBin:           fromEnvStr("AIC_MISTRAL_BIN", "vibe"),
		MistralArgs:          defaultMistralArgs(fromEnvList("AIC_MISTRAL_ARGS")),
		MistralAPIKey:        os.Getenv("AIC_MISTRAL_API_KEY"), // jamais journalisé ni persisté
		OpenAIMode:           fromEnvStr("AIC_OPENAI_MODE", ModeSimulated),
		DBPath:               fromEnvStr("AIC_DB_PATH", "ai-coordinator.sqlite"),
		WorkRoot:             fromEnvStr("AIC_WORK_ROOT", "/tmp/aic-work"),
		MaxCorrections:       fromEnvInt("AIC_MAX_CORRECTIONS", 3),
		RunStaleInterval:     fromEnvDur("AIC_RUN_STALE_INTERVAL", 86400*time.Second),
		PendingRetryInterval: fromEnvDur("AIC_PENDING_RETRY_INTERVAL", 60*time.Second),
		GitHubMode:           fromEnvStr("AIC_GITHUB_MODE", "off"),
		GitHubToken:          os.Getenv("AIC_GITHUB_TOKEN"), // jamais journalisé ni persisté
		GitHubAPIURL:         os.Getenv("AIC_GITHUB_APIURL"),
		GitHubTimeout:        fromEnvDur("AIC_GITHUB_TIMEOUT", 30*time.Second),
	}
}

// defaultMistralArgs applique le mode non interactif par défaut (exec)
// lorsque aucun argument n'est fourni.
func defaultMistralArgs(args []string) []string {
	if len(args) == 0 {
		// Contrat vérifié avec la CLI Mistral Code (vibe) : mode programmatique
		// -p lisant le prompt sur stdin sans texte l'accompagnant, confiance
		// accordée au répertoire isolé pour la seule invocation.
		return []string{"--prompt", "--trust"}
	}
	return args
}

package aic

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Server est le récepteur de webhooks GitHub du coordinateur.
type Server struct {
	Coordinator *Coordinator
	Logger      *slog.Logger
}

// signatureHeader est l'en-tête utilisé par GitHub pour la signature HMAC.
const signatureHeader = "X-Hub-Signature-256"

// VerifySignature vérifie la signature HMAC-SHA256 du corps brut du webhook
// en comparaison à temps constant.
func (s *Server) VerifySignature(body []byte, signature string) bool {
	if s.Coordinator.Config.WebhookSecret == "" {
		return false // refus silencieux : sans secret configuré, rien n'est accepté
	}
	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(s.Coordinator.Config.WebhookSecret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(want))
}

// issueCommentPayload est l'extrait minimal du payload issue_comment traité.
type issueCommentPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"issue"`
	Comment struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// HandleWebhook traite une requête de webhook GitHub : vérification de
// signature, déduplication de livraison, anti-boucle, puis déroulé.
// Toutes les exceptions sont journalisées ; aucune n'est dévoilée au client.
func (s *Server) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if !s.VerifySignature(body, r.Header.Get(signatureHeader)) {
		s.Logger.Warn("webhook rejeté : signature invalide")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	eventType := r.Header.Get("X-GitHub-Event")
	// réponses rapides : 200 immédiat, traitement déduit.
	w.WriteHeader(http.StatusOK)
	fresh, err := s.Coordinator.Store.SaveEvent(deliveryID, eventType)
	if err != nil || !fresh {
		s.Logger.Info("événement ignoré (déduplication)", "delivery", deliveryID)
		return
	}
	if eventType != "issue_comment" {
		return
	}
	var p issueCommentPayload
	if err := json.Unmarshal(body, &p); err != nil {
		s.Logger.Error("payload illisible", "err", err.Error())
		return
	}
	if p.Action != "created" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.Coordinator.Config.MistralTimeout+30*time.Second)
		defer cancel()
		if err := s.Coordinator.HandleIssueComment(ctx, int64(p.Issue.Number), p.Issue.Title, senderLogin(p), p.Comment.Body); err != nil {
			s.Logger.Error("traitement mission", "issue", p.Issue.Number, "err", err.Error())
		}
	}()
}

func senderLogin(p issueCommentPayload) string {
	if p.Sender.Login != "" {
		return p.Sender.Login
	}
	return p.Comment.User.Login
}

// NewServer construit le récepteur HTTP avec son journal horodaté.
func NewServer(c *Coordinator, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	s := &Server{Coordinator: c, Logger: logger}
	mux.HandleFunc("/webhook", s.HandleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              c.Config.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
}

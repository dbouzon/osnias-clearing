package aic

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
// en comparaison à temps constant. Cette vérification authentifie le webhook
// GitHub lui-même ; elle ne dit rien de l'habilitation de l'auteur du
// commentaire, qui est contrôlée séparément (Config.IsAuthorized).
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
// signature, identification de livraison, persistance durable AVANT
// acquittement, déduplication, puis traitement asynchrone.
// Ordre imposé (T02-R1/R2) : l'événement est enregistré de façon durable
// avant la réponse 2xx ; en cas d'échec réel de stockage, la réponse est 503
// afin que GitHub rejoue la livraison ; une erreur de stockage n'est jamais
// assimilée à un doublon.
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
	if deliveryID == "" {
		// sans identifiant de livraison, la déduplication et la reprise sont
		// impossibles : la livraison est refusée pour être rejouée complète.
		s.Logger.Warn("webhook rejeté : identifiant de livraison absent")
		http.Error(w, "missing delivery id", http.StatusBadRequest)
		return
	}
	// Persistance durable de l'événement, avec son payload, AVANT tout
	// acquittement 2xx (T02-R2) : le traitement redevient possible après
	// redémarrage même si le processus s'arrête juste après l'acquittement.
	fresh, err := s.Coordinator.Store.SaveEvent(deliveryID, eventType, string(body))
	if err != nil {
		// échec réel de stockage : 503, GitHub rejouera la livraison.
		// L'événement n'est pas marqué traité : aucun risque de perte.
		s.Logger.Error("webhook non persisté : échec de stockage", "delivery", deliveryID, "err", err.Error())
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
		return
	}
	// L'événement est désormais durablement enregistré : acquittement.
	w.WriteHeader(http.StatusOK)
	if !fresh {
		s.Logger.Info("événement ignoré (déduplication)", "delivery", deliveryID)
		return
	}
	if eventType != "issue_comment" {
		// types d'événements non commandés : traités définitivement.
		_ = s.Coordinator.Store.MarkEventProcessed(deliveryID)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.Coordinator.Config.MistralTimeout+30*time.Second)
		defer cancel()
		s.ProcessAndAck(ctx, deliveryID, eventType, string(body))
	}()
}

// ProcessAndAck traite un événement persisté et le marque traité si son
// traitement est définitif. En cas d'échec ou de verrou actif, l'événement
// reste dans la file d'attente durable et sera repris au redémarrage
// (T02-R2 : aucune perte d'événement entre acquittement et traitement).
func (s *Server) ProcessAndAck(ctx context.Context, deliveryID, eventType, payload string) {
	done, err := s.Coordinator.ProcessEvent(ctx, eventType, payload)
	if !done {
		if err == ErrMissionLocked {
			s.Logger.Info("exécution différée : verrou d'exclusivité actif, événement conservé en file d'attente", "delivery", deliveryID)
		} else {
			s.Logger.Error("traitement mission, événement conservé en file d'attente", "delivery", deliveryID, "err", err.Error())
		}
		return
	}
	if err := s.Coordinator.Store.MarkEventProcessed(deliveryID); err != nil {
		s.Logger.Error("marquer l'événement traité", "delivery", deliveryID, "err", err.Error())
	}
}

// DrainPendingEvents rejoue au démarrage les événements enregistrés mais non
// traités (arrêt entre acquittement 2xx et traitement asynchrone, T02-R2).
func DrainPendingEvents(ctx context.Context, c *Coordinator, logger *slog.Logger) {
	events, err := c.Store.PendingEvents()
	if err != nil {
		logger.Error("lire la file d'attente des événements", "err", err.Error())
		return
	}
	if len(events) == 0 {
		return
	}
	logger.Info("file d'attente persistante : reprise", "events", len(events))
	for _, e := range events {
		s := &Server{Coordinator: c, Logger: logger}
		s.ProcessAndAck(ctx, e.DeliveryID, e.EventType, e.Payload)
	}
}

func senderLogin(p issueCommentPayload) string {
	if p.Sender.Login != "" {
		return p.Sender.Login
	}
	return p.Comment.User.Login
}

// StartPendingRetryLoop relance périodiquement, pendant le fonctionnement
// normal, le traitement des événements en attente (T02-R3, risque 1) :
// la reprise ne dépend plus du seul redémarrage. Les événements en erreur
// ou bloqués par un verrou d'exclusivité sont rejoués à chaque intervalle
// (AIC_PENDING_RETRY_INTERVAL, défaut 60 s) jusqu'à réussite ; la boucle
// s'arrête proprement à l'annulation du contexte (arrêt du coordinateur).
func StartPendingRetryLoop(ctx context.Context, c *Coordinator, logger *slog.Logger) {
	ticker := time.NewTicker(c.Config.PendingRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			DrainPendingEvents(ctx, c, logger)
		}
	}
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

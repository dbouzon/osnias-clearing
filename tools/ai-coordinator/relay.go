package aic

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RelayService est la voie de messagerie authentifiée entre les agents
// (M-AIC-05). Elle est DISTINCTE du webhook GitHub : aucun secret GitHub
// n'y est accepté, et sans jeton dédié (AIC_MESSAGE_TOKEN) elle refuse
// toute requête (fail closed).
//
// Cycle réalisé par requête : message émetteur -> agent développeur Mistral
// -> agent chef de projet OpenAI, avec journal SQLite complet et
// identifiant de corrélation (message_id) partagé par tout l'échange.
// Idempotence : un message_id déjà traité (ou en file d'attente) est refusé
// en doublon, sans nouvel appel d'API payant.

// RelayRequest est le corps accepté par POST /aic/message.
type RelayRequest struct {
	MissionID string `json:"mission_id"` // identifiant de corrélation du dialogue
	MessageID string `json:"message_id"` // identifiant d'idempotence, fourni par l'émetteur
	From      string `json:"from"`       // identité de l'émetteur déclarée (openai, mistral, humain)
	Body      string `json:"body"`       // message transmis à l'agent Mistral
}

// RelayResponse est la réponse structurée du relais.
type RelayResponse struct {
	MissionID string        `json:"mission_id"`
	MessageID string        `json:"message_id"`
	Status    string        `json:"status"` // ok | duplicate | error
	From      string        `json:"from,omitempty"`
	Mistral   string        `json:"mistral,omitempty"` // réponse de l'agent Mistral (extrait)
	Note      string        `json:"note,omitempty"`
	OpenAI    *ReviewOutput `json:"openai,omitempty"` // verdict de l'agent OpenAI
}

// ErrRelayDuplicate signale un message_id déjà enregistré (idempotence).
var ErrRelayDuplicate = errors.New("message_id déjà enregistré : requête idempotente ignorée")

// Relay contient les dépendances du relais. Une valeur à Mistral ou OpenAI
// nil refuse toute requête : le relais n'est jamais partiellement actif.
type Relay struct {
	Store   *Store
	Mistral *RealMistral // requis, fail closed à la construction
	OpenAI  *RealOpenAI  // requis, fail closed à la construction
	token   string       // jeton du relais, mémoire uniquement
}

// NewRelay construit le relais : sans jeton, le relais reste construit mais
// toute requête HTTP sera refusée (le raccordement TCP sans authentification
// est interdit par l'architecture).
func NewRelay(store *Store, mistral *RealMistral, openai *RealOpenAI, token string) (*Relay, error) {
	if store == nil || mistral == nil || openai == nil {
		return nil, errors.New("relais incomplet : Store et adaptateurs réels Mistral et OpenAI requis (fail closed)")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("relais : AIC_MESSAGE_TOKEN absent, la voie de messagerie serait sans authentification (fail closed)")
	}
	return &Relay{Store: store, Mistral: mistral, OpenAI: openai, token: token}, nil
}

// authenticate vérifie le jeton du relais en comparaison à temps constant.
func (r *Relay) authenticate(got string) bool {
	got = strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
	if got == "" || r.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(r.token)) == 1
}

// Relay exécute le cycle OpenAI->Mistral->OpenAI pour un message donné.
// Idempotence par message_id (table events) ; journal SQLite de chaque
// étape sous la mission fournie ; aucun secret journalisé.
func (r *Relay) Relay(req RelayRequest, logAgent func(missionID, agent, phase, message string) error) (*RelayResponse, error) {
	req.MissionID = strings.TrimSpace(req.MissionID)
	req.MessageID = strings.TrimSpace(req.MessageID)
	req.Body = strings.TrimSpace(req.Body)
	if req.From == "" {
		req.From = "humain"
	}
	switch req.From {
	case "openai", "mistral", "humain":
	default:
		return nil, fmt.Errorf("émetteur inconnu : %q (openai, mistral ou humain)", req.From)
	}
	if req.MissionID == "" || req.MessageID == "" || req.Body == "" {
		return nil, errors.New("mission_id, message_id et body sont requis")
	}
	log := func(agent, phase, message string) {
		if logAgent != nil {
			_ = logAgent(req.MissionID, agent, phase, message)
		}
	}
	// Idempotence durable AVANT tout appel d'API payant : le message est
	// persisté avec son identifiant ; un redémarrage entre l'acquittement
	// et le traitement ne duplique jamais l'échange (sémantique webhook).
	fresh, err := r.Store.SaveEvent("aicmsg:"+req.MessageID, "aic_message", req.Body)
	if err != nil {
		return nil, fmt.Errorf("persistance du message : %w", err)
	}
	if !fresh {
		log("relay", "duplicate", "message_id déjà enregistré : requête ignorée")
		return &RelayResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "duplicate"}, ErrRelayDuplicate
	}
	log("relay", "receive", fmt.Sprintf("message_id=%s de=%s : %s", req.MessageID, req.From, excerptStr(req.Body, 200)))

	mistralReply, err := r.Mistral.Call(req.MissionID, req.Body)
	if err != nil {
		log("relay", "error", fmt.Sprintf("appel Mistral échoué (message_id=%s) : %v", req.MessageID, err))
		return &RelayResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "error", From: req.From, Note: "appel Mistral échoué : " + err.Error()}, err
	}
	log("relay", "mistral", excerptStr(mistralReply, 300))

	openAIPrompt := "Message de l'émetteur (" + req.From + ") via Osnias AI Coordinator :\n" + req.Body +
		"\n\nRéponse de l'agent développeur Mistral Code :\n" + mistralReply
	verdict, err := r.OpenAI.Call(req.MissionID, openAIPrompt)
	if err != nil {
		log("relay", "error", fmt.Sprintf("appel OpenAI échoué (message_id=%s) : %v", req.MessageID, err))
		return &RelayResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "error", From: req.From, Mistral: excerptStr(mistralReply, 2000), Note: "appel OpenAI échoué : " + err.Error()}, err
	}
	log("relay", "openai", fmt.Sprintf("verdict=%s résumé=%s", verdict.Decision, excerptStr(verdict.Summary, 300)))

	if err := r.Store.MarkEventProcessed("aicmsg:" + req.MessageID); err != nil {
		log("relay", "error", "marquage traité du message : "+err.Error())
	}
	return &RelayResponse{
		MissionID: req.MissionID,
		MessageID: req.MessageID,
		Status:    "ok",
		From:      req.From,
		Mistral:   mistralReply,
		OpenAI:    &verdict,
	}, nil
}

// handleAICMessage est le gestionnaire HTTP de POST /aic/message.
// Refus par défaut : jeton absent ou invalide, corps non JSON, champs
// manquants, relais non construit. Aucun secret n'est renvoyé au client.
func (s *Server) handleAICMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Relay == nil {
		http.Error(w, "relais désactivé", http.StatusNotImplemented)
		return
	}
	if !s.Relay.authenticate(r.Header.Get("Authorization")) {
		s.Logger.Warn("relais : authentification refusée")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var req RelayRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "json invalide", http.StatusBadRequest)
		return
	}
	resp, err := s.Relay.Relay(req, s.Coordinator.Store.Log)
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err != nil {
		if errors.Is(err, ErrRelayDuplicate) {
			enc.Encode(resp) // 200 : idempotence, l'échange initial fait foi
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_ = enc.Encode(resp)
		return
	}
	_ = enc.Encode(resp)
}

package aic

import (
	"errors"
	"fmt"
	"strings"
)

// Canal de discussion bidirectionnel ChatGPT <-> Mistral Code (M-AIC-10,
// issue #12). Réutilise exclusivement les briques existantes : le relais
// M-AIC-05 (Store SQLite, RealMistral, RealOpenAI, idempotence events) et
// l'historique du journal agent_logs.
//
// Différence avec le relais : chaque message est acheminé vers UN SEUL
// destinataire (mistral ou chatgpt), sans appeler systématiquement les deux
// API ; les réponses sont libres (aucun verdict exigé). Le routage, l'ordre
// et la persistance de chaque échange sont journalisés en SQLite.
//
// Aucune boucle autonome : Deliver est appelée par le programmeCmd appelant
// (test ou commande humaine) ; le canal n'émet rien de lui-même.

// parties admises sur le canal (émetteur ou destinataire).
const (
	PartyChatGPT = "chatgpt"
	PartyMistral = "mistral"
	PartyHuman   = "humain"
)

// ErrChannelDuplicate signale un message_id déjà enregistré (idempotence).
var ErrChannelDuplicate = errors.New("message_id déjà enregistré : requête idempotente ignorée")

// ChannelRequest est un message du canal : un émetteur, un destinataire.
type ChannelRequest struct {
	MissionID string `json:"mission_id"` // identifiant du dialogue (corrélation)
	MessageID string `json:"message_id"` // identifiant d'idempotence
	From      string `json:"from"`       // chatgpt | mistral | humain
	To        string `json:"to"`         // chatgpt | mistral (destinataire unique)
	Body      string `json:"body"`       // message libre
}

// ChannelResponse est la réponse du canal : la réponse libre du destinataire.
type ChannelResponse struct {
	MissionID string `json:"mission_id"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"` // ok | duplicate | error
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Reply     string `json:"reply,omitempty"` // réponse libre du destinataire
	Note      string `json:"note,omitempty"`
}

// Channel route les messages vers un seul adaptateur à la fois.
type Channel struct {
	Relay *Relay // dépendances existantes : Store, RealMistral, RealOpenAI
}

// NewChannel construit le canal au-dessus du relais existant : fail closed
// sans relais complet (store et adaptateurs réels vérifiés par NewRelay).
func NewChannel(relay *Relay) (*Channel, error) {
	if relay == nil {
		return nil, errors.New("canal : relais requis, construit via NewRelay (fail closed)")
	}
	if relay.Store == nil || relay.Mistral == nil || relay.OpenAI == nil {
		return nil, errors.New("canal : relais incomplet (fail closed)")
	}
	return &Channel{Relay: relay}, nil
}

// validParty vérifie une identité du canal.
func validParty(s string) bool {
	switch s {
	case PartyChatGPT, PartyMistral, PartyHuman:
		return true
	}
	return false
}

// ChannelHistory relit les derniers échanges du canal depuis agent_logs
// (phase "channel"), dans l'ordre chronologique. Une ligne d'historique a la
// forme "de=<émetteur> vers=<destinataire> corps=<message>".
func (s *Store) ChannelHistory(missionID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 6
	}
	rows, err := s.db.Query(
		`SELECT message FROM agent_logs WHERE mission_id = ? AND phase = 'channel' ORDER BY id DESC LIMIT ?`,
		missionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	// lecture en ordre décroissant : rétablir l'ordre chronologique.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// channelPrompt construit le prompt du destinataire : historique (contexte)
// puis message courant. L'historique borne le contexte aux derniers tours.
func channelPrompt(history []string, req ChannelRequest) string {
	var b strings.Builder
	if len(history) > 0 {
		b.WriteString("Historique du canal, ordre chronologique :\n")
		for _, h := range history {
			b.WriteString("- " + h + "\n")
		}
	}
	b.WriteString("Message de " + req.From + " : " + req.Body)
	return b.String()
}

// Deliver achemine un message vers son unique destinataire et retourne la
// réponse libre. Idempotence durable par message_id AVANT tout appel d'API
// (sémantique identique au relais) ; journal SQLite à chaque étape.
func (c *Channel) Deliver(req ChannelRequest, logAgent func(missionID, agent, phase, message string) error) (*ChannelResponse, error) {
	req.MissionID = strings.TrimSpace(req.MissionID)
	req.MessageID = strings.TrimSpace(req.MessageID)
	req.From = strings.TrimSpace(req.From)
	req.To = strings.TrimSpace(req.To)
	req.Body = strings.TrimSpace(req.Body)
	if req.From == "" {
		req.From = PartyHuman
	}
	if req.MissionID == "" || req.MessageID == "" || req.Body == "" {
		return nil, errors.New("mission_id, message_id et body sont requis")
	}
	if !validParty(req.From) {
		return nil, fmt.Errorf("émetteur inconnu : %q (chatgpt, mistral ou humain)", req.From)
	}
	if req.To != PartyChatGPT && req.To != PartyMistral {
		return nil, fmt.Errorf("destinataire invalide : %q (chatgpt ou mistral, un seul par message)", req.To)
	}
	if req.From == req.To {
		return nil, errors.New("émetteur et destinataire identiques : refus")
	}
	log := func(agent, phase, message string) {
		if logAgent != nil {
			_ = logAgent(req.MissionID, agent, phase, message)
		}
	}
	// Idempotence durable AVANT tout appel d'API payant (réutilisation du
	// mécanisme events du relais).
	fresh, err := c.Relay.Store.SaveEvent("aicmsg:"+req.MessageID, "aic_message", req.Body)
	if err != nil {
		return nil, fmt.Errorf("persistance du message : %w", err)
	}
	if !fresh {
		log("channel", "duplicate", "message_id déjà enregistré : requête ignorée")
		return &ChannelResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "duplicate", From: req.From, To: req.To}, ErrChannelDuplicate
	}
	log("channel", "receive", fmt.Sprintf("message_id=%s de=%s vers=%s : %s", req.MessageID, req.From, req.To, excerptStr(req.Body, 200)))
	// Trace d'historique : consultée par les tours suivants comme contexte.
	log("channel", "channel", fmt.Sprintf("de=%s vers=%s corps=%s", req.From, req.To, excerptStr(req.Body, 400)))

	history, err := c.Relay.Store.ChannelHistory(req.MissionID, 6)
	if err != nil {
		log("channel", "error", "lecture d'historique : "+err.Error())
	}
	prompt := channelPrompt(history, req)

	var reply string
	switch req.To {
	case PartyMistral:
		reply, err = c.Relay.Mistral.Call(req.MissionID, prompt)
	default:
		reply, err = c.Relay.OpenAI.Reply(req.MissionID, prompt)
	}
	if err != nil {
		log("channel", "error", fmt.Sprintf("appel %s échoué (message_id=%s) : %v", req.To, req.MessageID, err))
		return &ChannelResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "error", From: req.From, To: req.To, Note: "appel " + req.To + " échoué : " + err.Error()}, err
	}
	if err := c.Relay.Store.MarkEventProcessed("aicmsg:" + req.MessageID); err != nil {
		log("channel", "error", "marquage traité du message : "+err.Error())
	}
	return &ChannelResponse{MissionID: req.MissionID, MessageID: req.MessageID, Status: "ok", From: req.From, To: req.To, Reply: reply}, nil
}

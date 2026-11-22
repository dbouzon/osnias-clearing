package aic

import (
	"errors"
	"fmt"
	"strings"
)

// ChatSession encode un dialogue interactif continu (M-AIC-06) au-dessus du
// relais M-AIC-05 : chaque tour réutilise le cycle Relay existant (message
// -> Mistral -> OpenAI -> verdict) avec un identifiant de tour unique, sans
// relancer le programme.
//
// Bornes de consommation (aucune dépense non bornée) :
//   - plafond de tours par session (MaxTurns) ;
//   - budget de jetons par session (TokenBudget), compté en pire cas :
//     chaque tour est décompté de la somme des plafonds de réponse Mistral
//     et OpenAI, indépendamment de la réponse réelle ; les limites sont
//     vérifiées AVANT tout appel d'API ;
//   - un tour compté même en cas d'erreur en aval : la consommation réelle
//     des amendes déjà émises n'est jamais sous-estimée.
//
// Aucune exécution autonome : Say est uniquement appelée sur saisie
// humaine (cmd/aic-chat) ; aucune boucle interne n'émet d'appel.

// ErrChatTurnLimit signale le plafond de tours atteint.
var ErrChatTurnLimit = errors.New("plafond de tours de la session atteint : session close, relancer avec un plafond plus élevé si validé")

// ErrChatTokenBudget signale le budget de jetons épuisé.
var ErrChatTokenBudget = errors.New("budget de jetons de la session atteint : session close")

// ChatSession borne et corrélationne les tours de dialogue interactif.
type ChatSession struct {
	Relay         *Relay
	MissionID     string // identifiant de corrélation : une session = une mission
	MaxTurns      int    // plafond de tours (AIC_SESSION_MAX_TURNS, défaut 20)
	TokenBudget   int    // budget de jetons pire cas (AIC_SESSION_TOKEN_BUDGET, défaut 20000)
	perTurnTokens int    // décompte pire cas par tour (MistralMaxTok + OpenAIMaxTok)
	turns         int    // tours consommés
	tokens        int    // jetons décomptés (pire cas)
}

// NewChatSession construit une session. Fail closed : plafonds non positifs
// refusés ; le décompte pire cas par tour sommme les plafonds d'adaptateurs.
func NewChatSession(relay *Relay, missionID string, maxTurns, tokenBudget, mistralMaxTok, openaiMaxTok int) (*ChatSession, error) {
	if relay == nil {
		return nil, errors.New("session de dialogue : relais requis (fail closed)")
	}
	missionID = strings.TrimSpace(missionID)
	if missionID == "" {
		return nil, errors.New("session de dialogue : identifiant de mission requis")
	}
	if maxTurns <= 0 {
		maxTurns = 20
	}
	if tokenBudget <= 0 {
		tokenBudget = 20000
	}
	perTurn := mistralMaxTok + openaiMaxTok
	if perTurn <= 0 {
		perTurn = 1200
	}
	return &ChatSession{
		Relay:         relay,
		MissionID:     missionID,
		MaxTurns:      maxTurns,
		TokenBudget:   tokenBudget,
		perTurnTokens: perTurn,
	}, nil
}

// Turns retourne le nombre de tours consommés.
func (s *ChatSession) Turns() int { return s.turns }

// TokensSpent retourne le décompte pire cas consommé.
func (s *ChatSession) TokensSpent() int { return s.tokens }

// tourID construit l'identifiant unique du tour n (idempotence Relay).
func (s *ChatSession) tourID(n int) string { return fmt.Sprintf("%s-t%03d", s.MissionID, n) }

// TourIDs retourne les identifiants des tours déjà émis (vérification).
func (s *ChatSession) TourIDs() []string {
	ids := make([]string, s.turns)
	for i := 0; i < s.turns; i++ {
		ids[i] = s.tourID(i + 1)
	}
	return ids
}

// Say exécute un tour de dialogue : bornes vérifiées AVANT tout appel, puis
// cycle Relay complet sous un nouvel identifiant de tour. Le tour est
// consommé dès que l'appel est engagé (comptage conservateur).
func (s *ChatSession) Say(body string, from string, logAgent func(missionID, agent, phase, message string) error) (*RelayResponse, error) {
	if s.turns >= s.MaxTurns {
		return &RelayResponse{MissionID: s.MissionID, Status: "error", Note: ErrChatTurnLimit.Error()}, ErrChatTurnLimit
	}
	if s.tokens+s.perTurnTokens > s.TokenBudget {
		return &RelayResponse{MissionID: s.MissionID, Status: "error", Note: ErrChatTokenBudget.Error()}, ErrChatTokenBudget
	}
	s.turns++
	s.tokens += s.perTurnTokens
	n := s.turns
	resp, err := s.Relay.Relay(RelayRequest{
		MissionID: s.MissionID,
		MessageID: s.tourID(n),
		From:      from,
		Body:      body,
	}, logAgent)
	if resp != nil {
		resp.MessageID = s.tourID(n)
	}
	return resp, err
}

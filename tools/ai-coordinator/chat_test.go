package aic

// Tests M-AIC-06 de la session de dialogue interactif (ChatSession) : bornes
// de consommation vérifiées AVANT tout appel d'API, identifiants de tours
// déterministes, cycle complet de deux tours sur serveurs simulés avec
// trace SQLite. Aucun accès réseau réel (niveau 1 : tests internes simulés).

import (
	"path/filepath"
	"strings"
	"testing"
)

// newTestSession monte une session sur relais complet simulé.
func newTestSession(t *testing.T, missionID string, maxTurns, budget int) (*ChatSession, *Relay, *Store) {
	t.Helper()
	mURL, oURL := fakeUpstreams(t, new(string), new(string))
	relay, store := newTestRelay(t, mURL, oURL)
	var chatErr error
	var s *ChatSession
	s, chatErr = NewChatSession(relay, missionID, maxTurns, budget, 400, 400)
	if chatErr != nil {
		t.Fatalf("NewChatSession : %v", chatErr)
	}
	return s, relay, store
}

func TestNewChatSessionFailClosed(t *testing.T) {
	mURL, oURL := fakeUpstreams(t, new(string), new(string))
	relay, _ := newTestRelay(t, mURL, oURL)
	if _, err := NewChatSession(nil, "m", 5, 100, 400, 400); err == nil {
		t.Fatal("sans relais, la session doit refuser la construction (fail closed)")
	}
	if _, err := NewChatSession(relay, "  ", 5, 100, 400, 400); err == nil {
		t.Fatal("sans identifiant de mission, la session doit refuser la construction")
	}
}

func TestChatSessionPlafondDeTours(t *testing.T) {
	s, _, store := newTestSession(t, "aic-06-cap", 1, 100000)
	if _, err := s.Say("premier tour", "humain", store.Log); err != nil {
		t.Fatalf("premier tour : %v", err)
	}
	resp, err := s.Say("second tour", "humain", store.Log)
	if err != ErrChatTurnLimit {
		t.Fatalf("plafond de tours non signalé : %v", err)
	}
	if resp == nil || resp.Status != "error" || resp.Note == "" {
		t.Fatalf("réponse d'erreur attendue : %+v", resp)
	}
	if s.Turns() != 1 {
		t.Fatalf("le tour refusé ne doit pas être consommé : %d", s.Turns())
	}
	if s.TokensSpent() != 800 {
		t.Fatalf("décompte pire cas attendu 800, obtenu %d", s.TokensSpent())
	}
}

func TestChatSessionBudgetJetons(t *testing.T) {
	s, _, store := newTestSession(t, "aic-06-budget", 10, 1500)
	if _, err := s.Say("tour un", "humain", store.Log); err != nil {
		t.Fatalf("tour un : %v", err)
	}
	// 800 + 800 = 1600 > 1500 : le second tour doit être refusé AVANT tout appel.
	if _, err := s.Say("tour deux", "humain", store.Log); err != ErrChatTokenBudget {
		t.Fatalf("budget non signalé : %v", err)
	}
	if s.Turns() != 1 {
		t.Fatalf("seul le tour engagé doit être compté : %d", s.Turns())
	}
}

func TestChatSessionTourIDs(t *testing.T) {
	s, _, store := newTestSession(t, "aic-06-ids", 20, 100000)
	s.Say("un", "humain", store.Log)
	s.Say("deux", "humain", store.Log)
	ids := s.TourIDs()
	if len(ids) != 2 || ids[0] != "aic-06-ids-t001" || ids[1] != "aic-06-ids-t002" {
		t.Fatalf("séquence d'identifiants inattendue : %v", ids)
	}
}

func TestChatSessionDeuxToursAvecTrace(t *testing.T) {
	s, _, store := newTestSession(t, "aic-06-histo", 20, 100000)
	resp1, err := s.Say("première saisie humaine", "humain", store.Log)
	if err != nil {
		t.Fatalf("tour 1 : %v", err)
	}
	resp2, err := s.Say("seconde saisie humaine", "humain", store.Log)
	if err != nil {
		t.Fatalf("tour 2 : %v", err)
	}
	if resp1.MessageID != "aic-06-histo-t001" || resp2.MessageID != "aic-06-histo-t002" {
		t.Fatalf("identifiants de tours : %q, %q", resp1.MessageID, resp2.MessageID)
	}
	if resp1.Mistral == "" || resp2.Mistral == "" {
		t.Fatal("réponses Mistral absentes")
	}
	// Historique persistant : les deux tours complets dans agent_logs.
	rows, err := store.db.Query(`SELECT phase, message FROM agent_logs WHERE mission_id='aic-06-histo' ORDER BY id`)
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	defer rows.Close()
	var relayReceive, mistralCalls int
	var bodies []string
	for rows.Next() {
		var phase, message string
		_ = rows.Scan(&phase, &message)
		switch {
		case phase == "receive":
			relayReceive++
			bodies = append(bodies, message)
		case phase == "call" && strings.Contains(message, "modèle="):
			mistralCalls++
		}
	}
	if relayReceive != 2 {
		t.Fatalf("deux receptions attendues dans l'historique, obtenu %d", relayReceive)
	}
	if mistralCalls < 2 {
		t.Fatalf("deux appels Mistral attendus dans l'historique, obtenu %d", mistralCalls)
	}
	joined := strings.Join(bodies, "|")
	if !strings.Contains(joined, "première saisie humaine") || !strings.Contains(joined, "seconde saisie humaine") {
		t.Fatalf("l'historique doit conserver les deux saisies : %q", joined)
	}
}

func TestChatSessionDefaults(t *testing.T) {
	mURL, oURL := fakeUpstreams(t, new(string), new(string))
	relay, _ := newTestRelay(t, mURL, oURL)
	s, err := NewChatSession(relay, "aic-06-def", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("NewChatSession : %v", err)
	}
	if s.MaxTurns != 20 || s.TokenBudget != 20000 {
		t.Fatalf("défauts attendus 20/20000, obtenus %d/%d", s.MaxTurns, s.TokenBudget)
	}
	if s.perTurnTokens != 1200 {
		t.Fatalf("décompte pire cas par tour attendu 1200, obtenu %d", s.perTurnTokens)
	}
	_ = filepath.Join // import conservé si les besoins évoluent
}

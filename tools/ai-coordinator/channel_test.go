package aic

// Tests M-AIC-10 du canal bidirectionnel (issue #12) : routage vers un seul
// destinataire, réponses libres sans verdict, historique et contexte, ordre
// et persistance SQLite. Serveurs simulés httptest (niveau 1 : tests
// internes simulés ; la connexion des sessions réelles ChatGPT et Mistral
// Code n'est PAS prouvée ici, voir rapport).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// capturent le dernier prompt, pour vérifier le routage ciblé.

func fakeUpstreamsBody(t *testing.T, mistralHits, openAIHits *int, mistralGot, openAIGot *string) (string, string) {
	mSrv := newFakeServer(t, mistralHits, mistralGot, "réponse libre de Mistral")
	oSrv := newFakeServerFree(t, openAIHits, openAIGot, "réponse libre de ChatGPT")
	return mSrv, oSrv
}

// newTestChannel monte un canal complet sur serveurs simulés.
func newTestChannel(t *testing.T) (*Channel, *Relay, *Store, *int, *int, *string, *string) {
	mHits, oHits := 0, 0
	var mGot, oGot string
	mURL, oURL := fakeUpstreamsBody(t, &mHits, &oHits, &mGot, &oGot)
	relay, store := newTestRelayCounts(t, mURL, oURL, &mHits, &oHits)
	ch, err := NewChannel(relay)
	if err != nil {
		t.Fatalf("NewChannel : %v", err)
	}
	return ch, relay, store, &mHits, &oHits, &mGot, &oGot
}

// newTestRelayCounts monte un relais complet sur serveurs donnés.
func newTestRelayCounts(t *testing.T, mistralSrv, openAISrv string, mistralHits, openAIHits *int) (*Relay, *Store) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "channel.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore : %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	mistral, err := NewRealMistral(RealMistralOptions{APIKey: "msk-test", APIURL: mistralSrv,
		Log: func(m, p, msg string) { _ = store.Log(m, "mistral", p, msg) }})
	if err != nil {
		t.Fatalf("NewRealMistral : %v", err)
	}
	openai, err := NewRealOpenAI(RealOpenAIOptions{APIKey: "sk-test", APIURL: openAISrv,
		Log: func(m, p, msg string) { _ = store.Log(m, "openai", p, msg) }})
	if err != nil {
		t.Fatalf("NewRealOpenAI : %v", err)
	}
	relay, err := NewRelay(store, mistral, openai, "token-canal")
	if err != nil {
		t.Fatalf("NewRelay : %v", err)
	}
	return relay, store
}

// TestChannelSequenceTroisMessages vérifie la séquence de l'issue #12 :
// ChatGPT -> Mistral, Mistral -> ChatGPT, ChatGPT -> Mistral, avec
// destinataires, réception, ordre et persistance.
func TestChannelSequenceTroisMessages(t *testing.T) {
	ch, _, store, mHits, oHits, mGot, oGot := newTestChannel(t)

	// Tour 1 : ChatGPT -> Mistral : « Bonjour Mistral. »
	r1, err := ch.Deliver(ChannelRequest{
		MissionID: "aic-10-canal", MessageID: "aic-10-canal-m001",
		From: PartyChatGPT, To: PartyMistral, Body: "Bonjour Mistral.",
	}, store.Log)
	if err != nil {
		t.Fatalf("tour 1 : %v", err)
	}
	if r1.Status != "ok" || r1.To != PartyMistral || r1.Reply != "réponse libre de Mistral" {
		t.Fatalf("tour 1 : %+v", r1)
	}
	if *mHits != 1 || *oHits != 0 {
		t.Fatalf("tour 1 : routage ciblé attendu (Mistral=1, OpenAI=0), obtenu (%d, %d)", *mHits, *oHits)
	}
	if !strings.Contains(*mGot, "Bonjour Mistral.") {
		t.Fatalf("tour 1 : Mistral doit recevoir le message : %q", *mGot)
	}

	// Tour 2 : Mistral -> ChatGPT : « Bonjour ChatGPT. »
	r2, err := ch.Deliver(ChannelRequest{
		MissionID: "aic-10-canal", MessageID: "aic-10-canal-m002",
		From: PartyMistral, To: PartyChatGPT, Body: "Bonjour ChatGPT.",
	}, store.Log)
	if err != nil {
		t.Fatalf("tour 2 : %v", err)
	}
	if r2.Status != "ok" || r2.To != PartyChatGPT || r2.Reply != "réponse libre de ChatGPT" {
		t.Fatalf("tour 2 : %+v", r2)
	}
	if *mHits != 1 || *oHits != 1 {
		t.Fatalf("tour 2 : seul ChatGPT doit être appelé, obtenu Mistral=%d OpenAI=%d", *mHits, *oHits)
	}
	if !strings.Contains(*oGot, "Bonjour ChatGPT.") {
		t.Fatalf("tour 2 : ChatGPT doit recevoir le message : %q", *oGot)
	}

	// Tour 3 : ChatGPT -> Mistral : « Nous pouvons discuter. »
	r3, err := ch.Deliver(ChannelRequest{
		MissionID: "aic-10-canal", MessageID: "aic-10-canal-m003",
		From: PartyChatGPT, To: PartyMistral, Body: "Nous pouvons discuter.",
	}, store.Log)
	if err != nil {
		t.Fatalf("tour 3 : %v", err)
	}
	if r3.Status != "ok" || *mHits != 2 || *oHits != 1 {
		t.Fatalf("tour 3 : %+v, hits (%d, %d)", r3, *mHits, *oHits)
	}

	// Ordre et contexte : le troisième prompt doit contenir l'historique.
	if !strings.Contains(*mGot, "Bonjour Mistral.") || !strings.Contains(*mGot, "Bonjour ChatGPT.") {
		t.Fatalf("tour 3 : le contexte des tours précédents manque : %q", *mGot)
	}

	// Persistance : trois échanges tracés, dans l'ordre d'émission.
	hist, err := store.ChannelHistory("aic-10-canal", 10)
	if err != nil {
		t.Fatalf("historique : %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("trois échanges attendus dans l'historique, obtenu %d : %v", len(hist), hist)
	}
	if !strings.Contains(hist[0], "de=chatgpt vers=mistral") || !strings.Contains(hist[0], "Bonjour Mistral.") {
		t.Fatalf("ordre : premier échange incohérent : %q", hist[0])
	}
	if !strings.Contains(hist[1], "de=mistral vers=chatgpt") || !strings.Contains(hist[1], "Bonjour ChatGPT.") {
		t.Fatalf("ordre : second échange incohérent : %q", hist[1])
	}
	if !strings.Contains(hist[2], "Nous pouvons discuter.") {
		t.Fatalf("ordre : troisième échange incohérent : %q", hist[2])
	}
}

// TestChannelValidations vérifie les refus : fail closed et destinataires.
func TestChannelValidations(t *testing.T) {
	ch, _, store, mHits, oHits, _, _ := newTestChannel(t)
	if _, err := NewChannel(nil); err == nil {
		t.Fatal("sans relais, le canal doit refuser la construction (fail closed)")
	}
	// destinataire inconnu : refus, aucun appel.
	if _, err := ch.Deliver(ChannelRequest{MissionID: "m", MessageID: "x1", From: "chatgpt", To: "inconnu", Body: "b"}, store.Log); err == nil {
		t.Fatal("destinataire inconnu : refus attendu")
	}
	// émetteur = destinataire : refus.
	if _, err := ch.Deliver(ChannelRequest{MissionID: "m", MessageID: "x2", From: "chatgpt", To: "chatgpt", Body: "b"}, store.Log); err == nil {
		t.Fatal("émetteur et destinataire identiques : refus attendu")
	}
	// corps vide : refus.
	if _, err := ch.Deliver(ChannelRequest{MissionID: "m", MessageID: "x3", From: "chatgpt", To: "mistral", Body: "  "}, store.Log); err == nil {
		t.Fatal("corps vide : refus attendu")
	}
	if *mHits != 0 || *oHits != 0 {
		t.Fatalf("aucun appel ne doit être émis sur refus : (%d, %d)", *mHits, *oHits)
	}
}

// TestChannelIdempotence vérifie le rejeu d'un message déjà traité.
func TestChannelIdempotence(t *testing.T) {
	ch, _, store, mHits, _, _, _ := newTestChannel(t)
	req := ChannelRequest{MissionID: "aic-10-idem", MessageID: "aic-10-idem-m001", From: PartyChatGPT, To: PartyMistral, Body: "message unique"}
	if _, err := ch.Deliver(req, store.Log); err != nil {
		t.Fatalf("premier passage : %v", err)
	}
	resp, err := ch.Deliver(req, store.Log)
	if err == nil || resp == nil || resp.Status != "duplicate" {
		t.Fatalf("rejeu : doublon attendu, obtenu %+v (%v)", resp, err)
	}
	if *mHits != 1 {
		t.Fatalf("aucun second appel Mistral attendu sur doublon : %d", *mHits)
	}
}

// TestChannelReponseLibre vérifie que la réponse de ChatGPT via le canal est
// du texte libre : le serveur simulé renvoie du texte NON JSON et le canal
// doit le restituer tel quel, sans verdict ni erreur de structure.
func TestChannelReponseLibre(t *testing.T) {
	ch, _, store, _, oHits, _, oGot := newTestChannel(t)
	resp, err := ch.Deliver(ChannelRequest{
		MissionID: "aic-10-libre", MessageID: "aic-10-libre-m001",
		From: PartyHuman, To: PartyChatGPT, Body: "Réponds librement, sans structure.",
	}, store.Log)
	if err != nil {
		t.Fatalf("Deliver : %v", err)
	}
	if resp.Status != "ok" || resp.Reply != "réponse libre de ChatGPT" {
		t.Fatalf("réponse libre attendue, obtenue %+v", resp)
	}
	if *oHits != 1 {
		t.Fatalf("un seul appel OpenAI attendu : %d", *oHits)
	}
	if !strings.Contains(*oGot, "Réponds librement") {
		t.Fatalf("prompt transmis : %q", *oGot)
	}
}

// TestChannelPersistenceBaseRouverte vérifie la persistance au-delà du
// process : la base réouverte restitue l'historique complet du canal.
func TestChannelPersistenceBaseRouverte(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.sqlite")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore : %v", err)
	}
	mHits, oHits := 0, 0
	var mGot, oGot string
	mURL, oURL := fakeUpstreamsBody(t, &mHits, &oHits, &mGot, &oGot)
	relay, _ := newTestRelayCounts(t, mURL, oURL, &mHits, &oHits)
	relay2, err := NewRelay(store, relay.Mistral, relay.OpenAI, "token-persist")
	if err != nil {
		t.Fatalf("NewRelay persist : %v", err)
	}
	ch, err := NewChannel(relay2)
	if err != nil {
		t.Fatalf("NewChannel : %v", err)
	}
	if _, err := ch.Deliver(ChannelRequest{MissionID: "aic-10-p", MessageID: "p-1", From: PartyChatGPT, To: PartyMistral, Body: "premier"}, store.Log); err != nil {
		t.Fatalf("tour 1 : %v", err)
	}
	if _, err := ch.Deliver(ChannelRequest{MissionID: "aic-10-p", MessageID: "p-2", From: PartyMistral, To: PartyChatGPT, Body: "second"}, store.Log); err != nil {
		t.Fatalf("tour 2 : %v", err)
	}
	_ = store.Close()

	// Réouverture : l'historique est intégralement restitué.
	store2, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("réouverture : %v", err)
	}
	defer store2.Close()
	hist, err := store2.ChannelHistory("aic-10-p", 10)
	if err != nil {
		t.Fatalf("historique réouvert : %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("deux échanges attendus après réouverture, obtenu %d", len(hist))
	}
	joined := strings.Join(hist, "|")
	if !strings.Contains(joined, "premier") || !strings.Contains(joined, "second") {
		t.Fatalf("historique incomplet : %v", hist)
	}
}

// newFakeServer monte un serveur Mistral simulé comptant les appels.
func newFakeServer(t *testing.T, hits *int, got *string, content string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mistralRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		*hits++
		*got = req.Messages[len(req.Messages)-1].Content
		_, _ = w.Write([]byte(mistralJSON(content)))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// newFakeServerFree monte un serveur OpenAI simulé renvoyant du texte LIBRE
// (non JSON) : le canal doit restituer ce texte tel quel, sans verdict.
func newFakeServerFree(t *testing.T, hits *int, got *string, content string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		*hits++
		*got = req.Messages[len(req.Messages)-1].Content
		body, _ := json.Marshal(map[string]any{
			"id": "cmpl-free",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}},
			},
			"usage": map[string]int{"total_tokens": 42},
		})
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

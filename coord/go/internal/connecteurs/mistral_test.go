// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette C1..C9 ADAPTÉE au profil Mistral (mission M-COORD-LLM-03, issue
// #54, plan v2, tranche T2). SERVEURS FACTICES LOOPBACK UNIQUEMENT :
// qualification SIMULÉE, aucune clé Mistral réelle, aucun appel à
// api.mistral.ai, 0 €. Les contrôles communs (voûte avant réseau,
// retraits bornés, expurgation, secrets, idempotence) sont déjà couverts
// par les batteries anthropic/gemini/connecteurs et NE SONT PAS
// dupliqués ici ; seuls le profil et l'analyse stop-only sont testés,
// conformément au plan v2 (pas de duplication de règles).
package connecteurs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// ---- helpers du profil Mistral ----

// reponseMistral : réponse de l'API Mistral (chat completions) avec usages
// natifs prompt_tokens/completion_tokens et finish_reason contrôlé.
func reponseMistral(content, modele string, prompt, completion int, finish string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": modele,
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": content},
			"finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens":     prompt,
			"completion_tokens": completion,
			"total_tokens":      prompt + completion,
		},
	})
	return b
}

// configMistralServeur : configuration SIMULEE d'un fournisseur mistral
// pointant sur un serveur factice loopback (format openai natif).
func configMistralServeur(url string) *Config {
	return &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{{
		ID: "mistral-pt", Endpoint: url, Modele: "mistral-small-latest-test",
		Format: "openai", Profil: ProfilMistral,
		TimeoutMS: 5000, TailleMaxReponse: 65536, RetraitsMax: 0,
	}}}
}

type mistralVu struct {
	corps requeteAPI
	auth  string
	idem  string
}

// serveurMistralFactice : enregistre la requête reçue et répond.
func serveurMistralFactice(t *testing.T, reponse func() []byte) (*httptest.Server, *atomic.Int32, *mistralVu) {
	t.Helper()
	var vu mistralVu
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels.Add(1)
		_ = json.NewDecoder(r.Body).Decode(&vu.corps)
		vu.auth = r.Header.Get("Authorization")
		vu.idem = r.Header.Get("Idempotency-Key")
		w.Write(reponse())
	}))
	t.Cleanup(srv.Close)
	return srv, &appels, &vu
}

// pointMistral : connecteur du profil mistral depuis une configuration.
func pointMistral(t *testing.T, cfg *Config) *HTTPEndPoint {
	t.Helper()
	p, err := cfg.NouveauHTTPEndPoint("mistral-pt")
	if err != nil {
		t.Fatalf("connecteur mistral : %v", err)
	}
	return p
}

// ---- C1 : succès, usages natifs, en-têtes, format {model, messages} ----

func TestMistralC1SuccesUsagesNatifsFormat(t *testing.T) {
	t.Setenv("COORD_CLE_MISTRAL_TEST", "sk-mistral-test-000111")
	srv, appels, vu := serveurMistralFactice(t,
		func() []byte { return reponseMistral("SORTIE-MISTRAL", "mistral-small-latest-test", 215, 521, "stop") })
	cfg := configMistralServeur(srv.URL)
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_MISTRAL_TEST"
	p := pointMistral(t, cfg)

	res, err := p.ExecuterContexte(context.Background(), "executer", "produis un plan", "cle-c1")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if res.Sortie != "SORTIE-MISTRAL" {
		t.Fatalf("sortie inattendue : %q", res.Sortie)
	}
	if appels.Load() != 1 {
		t.Fatalf("un seul appel attendu, %d constatés", appels.Load())
	}
	if !strings.Contains(vu.auth, "Bearer sk-mistral-test-000111") {
		t.Fatalf("Authorization Bearer attendu : %q", vu.auth)
	}
	if vu.idem == "" {
		t.Fatalf("Idempotency-Key exigé")
	}
	// Pas de champ reasoning_effort (contrairement au profil gemini) :
	// requête {model, messages} stricte.
	b, _ := json.Marshal(vu.corps)
	if strings.Contains(string(b), "reasoning_effort") {
		t.Fatalf("aucun reasoning_effort pour le profil mistral : %s", b)
	}
}

// ---- C2 : finish_reason length (troncature) => REFUS, jamais réparé ----

func TestMistralC2TroncatureRefusee(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t,
		func() []byte { return reponseMistral("TRONQUE", "mistral-small-latest-test", 10, 10, "length") })
	p := pointMistral(t, configMistralServeur(srv.URL))
	if _, err := p.ExecuterContexte(context.Background(), "critiquer", "plan", "cle-c2"); err == nil {
		t.Fatalf("finish_reason=length doit être refusé (stop-only)")
	} else if !strings.Contains(err.Error(), "FIN_NON_NOMINAL") {
		t.Fatalf("motif FIN_NON_NOMINAL attendu : %v", err)
	}
}

// ---- C3 : finish_reason tool_calls => REFUS (aucun tool admis) ----

func TestMistralC3ToolCallsRefuses(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t,
		func() []byte { return reponseMistral("TOOL", "mistral-small-latest-test", 10, 10, "tool_calls") })
	p := pointMistral(t, configMistralServeur(srv.URL))
	if _, err := p.ExecuterContexte(context.Background(), "recevoir", "livrable", "cle-c3"); err == nil {
		t.Fatalf("finish_reason=tool_calls doit être refusé")
	}
}

// ---- C4 : aucune réponse candidate => MISTRAL_SANS_CANDIDAT ----

func TestMistralC4SansCandidat(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t, func() []byte {
		b, _ := json.Marshal(map[string]any{"model": "mistral-small-latest-test", "choices": []any{}})
		return b
	})
	p := pointMistral(t, configMistralServeur(srv.URL))
	if _, err := p.ExecuterContexte(context.Background(), "executer", "tâche", "cle-c4"); err == nil ||
		!strings.Contains(err.Error(), "MISTRAL_SANS_CANDIDAT") {
		t.Fatalf("MISTRAL_SANS_CANDIDAT attendu : %v", err)
	}
}

// ---- C5 : identité divergente => MODELE_DIVERGENT (boucle commune) ----

func TestMistralC5ModeleDivergent(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t,
		func() []byte { return reponseMistral("X", "autres-modele-2024", 5, 5, "stop") })
	p := pointMistral(t, configMistralServeur(srv.URL))
	if _, err := p.ExecuterContexte(context.Background(), "executer", "tâche", "cle-c5"); err == nil ||
		!strings.Contains(err.Error(), "MODELE_DIVERGENT") {
		t.Fatalf("MODELE_DIVERGENT attendu : %v", err)
	}
}

// ---- C6 : usage absent => JETONS_INCONNUS (fail closed) ----

func TestMistralC6JetonsInconnus(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t, func() []byte {
		return reponseMistral("X", "mistral-small-latest-test", 0, 0, "stop")
	})
	p := pointMistral(t, configMistralServeur(srv.URL))
	if _, err := p.ExecuterContexte(context.Background(), "executer", "tâche", "cle-c6"); err == nil ||
		!strings.Contains(err.Error(), "JETONS_INCONNUS") {
		t.Fatalf("JETONS_INCONNUS attendu : %v", err)
	}
}

// ---- C7 : profil mistral exige le format openai (refus au chargement) ----

func TestMistralC7FormatExige(t *testing.T) {
	dir := t.TempDir()
	chemin := dir + "/cfg-mistral-c7.json"
	corps := `{"mode":"SIMULE","fournisseurs":[{"id":"mistral-pt",` +
		`"endpoint":"http://127.0.0.1:1/v1","modele":"m","format":"anthropic",` +
		`"profil":"mistral","timeout_ms":5000,"taille_max_reponse":65536,"retraits_max":0}]}`
	if err := os.WriteFile(chemin, []byte(corps), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ChargerConfig(chemin)
	if err == nil || !strings.Contains(err.Error(), "profil mistral exigé au format") {
		t.Fatalf("refus profil/format attendu : %v", err)
	}
}

// ---- C8 : profil inconnu => refus au chargement (fail closed) ----

func TestMistralC8ProfilInconnuRefuse(t *testing.T) {
	dir := t.TempDir()
	chemin := dir + "/cfg-mistral-c8.json"
	corps := `{"mode":"SIMULE","fournisseurs":[{"id":"mistral-pt",` +
		`"endpoint":"http://127.0.0.1:1/v1","modele":"m","format":"openai",` +
		`"profil":"mistral-x","timeout_ms":5000,"taille_max_reponse":65536,"retraits_max":0}]}`
	if err := os.WriteFile(chemin, []byte(corps), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ChargerConfig(chemin)
	if err == nil || !strings.Contains(err.Error(), "profil inconnu") {
		t.Fatalf("refus profil inconnu attendu : %v", err)
	}
}

// ---- C9 : total_tokens de secours utilisé si usages natifs absents ----

func TestMistralC9TotalTokensSecours(t *testing.T) {
	srv, _, _ := serveurMistralFactice(t, func() []byte {
		// Réponse sans prompt/completion tokens MAIS total_tokens présent :
		// le secours est admis (usage transmis par le fournisseur).
		b, _ := json.Marshal(map[string]any{
			"model":   "mistral-small-latest-test",
			"choices": []any{map[string]any{"message": map[string]any{"content": "OK"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"total_tokens": 777},
		})
		return b
	})
	p := pointMistral(t, configMistralServeur(srv.URL))
	res, err := p.ExecuterContexte(context.Background(), "executer", "tâche", "cle-c9")
	if err != nil {
		t.Fatalf("secours total_tokens attendu admis : %v", err)
	}
	if res.Jetons != 777 {
		t.Fatalf("jetons 777 attendus : %d", res.Jetons)
	}
}

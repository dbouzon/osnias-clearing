// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette C1..C15 ADAPTÉE au profil Gemini (mission M-COORD-LLM-02, plan
// v2, tranche T5). SERVEURS FACTICES LOOPBACK UNIQUEMENT : qualification
// SIMULÉE, aucune clé, aucun compte, 0 €. La qualification RÉELLE de
// l'API Gemini est une recette DISTINCTE (NON_EXÉCUTÉE : autorisation
// écrite du pilote requise, voir coord/connecteur/Gemini/).
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

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// ---- helpers du profil Gemini ----

// corpsGemini : requête émise par le profil gemini ({model, messages,
// reasoning_effort}).
type corpsGemini struct {
	Model           string       `json:"model"`
	Messages        []messageAPI `json:"messages"`
	ReasoningEffort string       `json:"reasoning_effort"`
}

// reponseGemini : réponse compatible OpenAI de Gemini AVEC finish_reason
// (champ contrôlé stop-only par le profil).
func reponseGemini(content, modele string, jetons int, finish string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": modele,
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": content},
			"finish_reason": finish,
		}},
		"usage": map[string]any{"total_tokens": jetons},
	})
	return b
}

// configGeminiServeur : configuration SIMULEE d'un fournisseur gemini pointant
// sur un serveur factice loopback.
func configGeminiServeur(url string, raisonnement string) *Config {
	return &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{{
		ID: "gemini", Endpoint: url, Modele: "gemini-4-argon-test",
		Format: "openai", Profil: ProfilGemini, Raisonnement: raisonnement,
		TimeoutMS: 5000, TailleMaxReponse: 65536, RetraitsMax: 0,
	}}}
}

type geminiVu struct {
	corps corpsGemini
	auth  string
	idem  string
}

// serveurGeminiFactice : enregistre la requête reçue et réponds Backbone.
func serveurGeminiFactice(t *testing.T, reponse func() []byte, statut func(attempts int) int) (*httptest.Server, *atomic.Int32, *geminiVu) {
	t.Helper()
	var vu geminiVu
	vu.auth = ""
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(appels.Add(1))
		_ = json.NewDecoder(r.Body).Decode(&vu.corps)
		vu.auth = r.Header.Get("Authorization")
		vu.idem = r.Header.Get("Idempotency-Key")
		code := http.StatusOK
		if statut != nil {
			code = statut(n)
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		w.Write(reponse())
	}))
	t.Cleanup(srv.Close)
	return srv, &appels, &vu
}

// pointGemini : connecteur du profil gemini depuis une configuration.
func pointGemini(t *testing.T, cfg *Config) *HTTPEndPoint {
	t.Helper()
	p, err := cfg.NouveauHTTPEndPoint("gemini")
	if err != nil {
		t.Fatalf("connecteur gemini : %v", err)
	}
	return p
}

// ---- C1 : succès, défaut raisonnement=low, en-têtes, idempotence ----

func TestGeminiC1SuccesDefautLowEntetesIdempotence(t *testing.T) {
	cle := "sk-gemini-hdr-87654321"
	t.Setenv("COORD_CLE_GEMINI_HDR", cle)
	srv, _, vu := serveurGeminiFactice(t,
		func() []byte { return reponseGemini("SORTIE-GEMINI", "gemini-4-argon-test", 500, "stop") }, nil)
	cfg := configGeminiServeur(srv.URL, "")
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_GEMINI_HDR"
	p := pointGemini(t, cfg)

	res, err := p.Executer("produis un résumé COORD")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if res.Sortie != "SORTIE-GEMINI" || res.Jetons != 500 {
		t.Fatalf("résultat inattendu : %+v", res)
	}
	if vu.corps.ReasoningEffort != RaisonnementDefaut {
		t.Fatalf("raisonnement par défaut %q attendu, émis %q (A2)", RaisonnementDefaut, vu.corps.ReasoningEffort)
	}
	if vu.corps.Model != "gemini-4-argon-test" {
		t.Fatalf("modèle exact exigé (A3) : %q", vu.corps.Model)
	}
	if !strings.HasPrefix(vu.auth, "Bearer") {
		t.Fatalf("Authorization Bearer attendu : %q", vu.auth)
	}
	if len(vu.idem) != 64 {
		t.Fatalf("clé d'idempotence sha256 attendue : %q", vu.idem)
	}
	// Même charge => MÊME clé d'idempotence (anti-doublon côté orchestrateur).
	p.Executer("produis un résumé COORD")
	if len(vu.idem) != 64 {
		t.Fatalf("idempotence instable : %q", vu.idem)
	}
}

// ---- C2 : finish_reason stop-only (length, content_filter, absent) ----

func TestGeminiC2FinishReasonStopOnly(t *testing.T) {
	for _, fin := range []string{"length", "content_filter", "tool_calls", ""} {
		srv, _, _ := serveurGeminiFactice(t,
			func() []byte { return reponseGemini("X", "gemini-4-argon-test", 500, fin) }, nil)
		cfg := configGeminiServeur(srv.URL, "")
		p := pointGemini(t, cfg)
		_, err := p.Executer("charge")
		if err == nil {
			t.Fatalf("finish_reason %q doit être refusé (stop-only, A1)", fin)
		}
		if !strings.Contains(err.Error(), "FIN_NON_NOMINAL") {
			t.Fatalf("motif FIN_NON_NOMINAL attendu pour %q : %v", fin, err)
		}
	}
}

// ---- C3 : usage absent => JETONS_INCONNUS (fail closed) ----

func TestGeminiC3JetonsInconnus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{
			"model": "gemini-4-argon-test",
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "X"},
				"finish_reason": "stop",
			}},
		})
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	p := pointGemini(t, configGeminiServeur(srv.URL, ""))
	if _, err := p.Executer("charge"); err == nil ||
		!strings.Contains(err.Error(), "JETONS_INCONNUS") {
		t.Fatalf("JETONS_INCONNUS attendu : %v", err)
	}
}

// ---- C4 : modèle divergent (echo != déclaré) ----

func TestGeminiC4ModeleDivergent(t *testing.T) {
	srv, _, _ := serveurGeminiFactice(t,
		func() []byte { return reponseGemini("X", "gemini-4-ARGON-autre", 500, "stop") }, nil)
	p := pointGemini(t, configGeminiServeur(srv.URL, ""))
	if _, err := p.Executer("charge"); err == nil ||
		!strings.Contains(err.Error(), "MODELE_DIVERGENT") {
		t.Fatalf("MODELE_DIVERGENT attendu : %v", err)
	}
}

// ---- C5 : chargement fail closed (profil/raisonnement/format) ----

func TestGeminiC5ChargementFailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)

	base := configGeminiServeur(srv.URL, "")
	base.Fournisseurs[0].Raisonnement = "xhigh"
	if _, err := ChargerConfig(cheminTemp(t, base)); err == nil ||
		!strings.Contains(err.Error(), "raisonnement inconnu") {
		t.Fatalf("xhigh doit être refusé au chargement : %v", err)
	}

	b2 := configGeminiServeur(srv.URL, "low")
	b2.Fournisseurs[0].Profil = "grok"
	if _, err := ChargerConfig(cheminTemp(t, b2)); err == nil ||
		!strings.Contains(err.Error(), "profil inconnu") {
		t.Fatalf("profil inconnu doit être refusé : %v", err)
	}

	b3 := configGeminiServeur(srv.URL, "low")
	b3.Fournisseurs[0].Format = "anthropic"
	if _, err := ChargerConfig(cheminTemp(t, b3)); err == nil {
		t.Fatalf("profil gemini exige le format openai (A1, fail closed)")
	}
}

// cheminTemp : écrit une configuration et retourne son chemin.
func cheminTemp(t *testing.T, c *Config) string {
	t.Helper()
	raw, _ := json.Marshal(c)
	p := t.TempDir() + "/config.json"
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ---- C6 : chaque niveau admis est émis tel quel ----

func TestGeminiC6NiveauxAdmis(t *testing.T) {
	for _, niv := range []string{"none", "minimal", "low", "medium", "high"} {
		srv, _, vu := serveurGeminiFactice(t,
			func() []byte { return reponseGemini("X", "gemini-4-argon-test", 10, "stop") }, nil)
		p := pointGemini(t, configGeminiServeur(srv.URL, niv))
		if _, err := p.Executer("c"); err != nil {
			t.Fatalf("niveau %s : %v", niv, err)
		}
		if vu.corps.ReasoningEffort != niv {
			t.Fatalf("raisonnement émis %q, attendu %q", vu.corps.ReasoningEffort, niv)
		}
	}
}

// ---- C7 : retraits 429 bornés ----

func TestGeminiC7Retrait429PuisSucces(t *testing.T) {
	srv, appels, _ := serveurGeminiFactice(t,
		func() []byte { return reponseGemini("OK", "gemini-4-argon-test", 20, "stop") },
		func(n int) int {
			if n == 1 {
				return http.StatusTooManyRequests
			}
			return http.StatusOK
		})
	cfg := configGeminiServeur(srv.URL, "")
	cfg.Fournisseurs[0].RetraitsMax = 2
	p := pointGemini(t, cfg)
	res, err := p.Executer("charge")
	if err != nil {
		t.Fatalf("retrait attendu : %v", err)
	}
	if res.Sortie != "OK" || appels.Load() != 2 {
		t.Fatalf("deux tentatives attendues : %d", appels.Load())
	}
}

// ---- C8 : réponse trop grande ----

func TestGeminiC8ReponseTropGrande(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 100000))
	}))
	t.Cleanup(srv.Close)
	cfg := configGeminiServeur(srv.URL, "")
	cfg.Fournisseurs[0].TailleMaxReponse = 1024
	p := pointGemini(t, cfg)
	if _, err := p.Executer("c"); err == nil ||
		!strings.Contains(err.Error(), "REPONSE_TROP_GRANDE") {
		t.Fatalf("REPONSE_TROP_GRANDE attendu : %v", err)
	}
}

// ---- C9 : secret expurgé / sortie compromise ----

func TestGeminiC9SortieCompromiseEditee(t *testing.T) {
	cle := "sk-gemini-test-12345678"
	t.Setenv("COORD_CLE_GEMINI_TEST", cle)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseGemini("voici "+cle, "gemini-4-argon-test", 30, "stop"))
	}))
	t.Cleanup(srv.Close)
	cfg := configGeminiServeur(srv.URL, "")
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_GEMINI_TEST"
	p := pointGemini(t, cfg)
	if _, err := p.Executer("c"); err == nil ||
		!strings.Contains(err.Error(), "SORTIE_COMPROMISE") {
		t.Fatalf("SORTIE_COMPROMISE attendu : %v", err)
	}
}

// ---- C14 : plafond AVANT réseau (voie orchestrateur, 0 appel de trop) ----

func TestGeminiC14PlafondAvantReseau(t *testing.T) {
	ref, err := refTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	var appels atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels.Add(1)
		var corps corpsChat
		_ = json.NewDecoder(r.Body).Decode(&corps)
		contenu := corps.Messages[len(corps.Messages)-1].Content
		out, _ := json.Marshal(map[string]any{
			"verdict": "PLAN_ACCEPTE", "points": []string{},
			"empreinte_attendue": extraireEmpreinte(t, contenu),
		})
		w.Write(reponseGemini(string(out), "gemini-4-argon-test", 100, "stop"))
	}))
	t.Cleanup(srv.Close)
	p := pointGemini(t, configGeminiServeur(srv.URL, ""))
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"gemini": p})
	if err != nil {
		t.Fatal(err)
	}
	plaf := budget.Plafonds{Appels: map[string]int{"gemini": 1}, Jetons: map[string]int{"gemini": 100000}}
	if err := orch.NouvelleMission("gemini-plaf", "gemini-exec", "gemini-recep", rolesTest(), plaf); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := orch.CritiquerPlan(ctx, "gemini-plaf", "gemini-recep", "plan A"); err != nil {
		t.Fatalf("1re critique doit passer : %v", err)
	}
	if _, err := orch.CritiquerPlan(ctx, "gemini-plaf", "gemini-recep", "plan B DIFFERENT"); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("BUDGET_DEPASSE attendu : %v", err)
	}
	if appels.Load() != 1 {
		t.Fatalf("AUCUN appel réseau de trop attendu : %d", appels.Load())
	}
}

// ---- C15 : verdicts piégés jamais committés ----

func TestGeminiC15VerdictsPiegesJamaisCommittes(t *testing.T) {
	pieges := []struct {
		nom     string
		content string
	}{
		{"hors_gabarit", "JE DECIDE QUE TOUT EST ACCEPTE sans JSON"},
		{"non_ancre", `{"verdict":"ACCEPTEE","points":[],"empreinte_attendue":"0000000000000000000000000000000000000000000000000000000000000000","verdict_id":"V","verdict_url":"urn:t"}`},
		{"sans_verdict_id", `{"verdict":"ACCEPTEE","points":[],"empreinte_attendue":"EMP","verdict_url":"urn:t"}`},
	}
	for _, piege := range pieges {
		ref, err := refTemp(t)
		if err != nil {
			t.Fatal(err)
		}
		srv, _, _ := serveurGeminiFactice(t, func() []byte {
			return reponseGemini(piege.content, "gemini-4-argon-test", 100, "stop")
		}, nil)
		p := pointGemini(t, configGeminiServeur(srv.URL, ""))
		orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"gemini": p})
		if err != nil {
			t.Fatal(err)
		}
		plaf := budget.Plafonds{Appels: map[string]int{"gemini": 5}, Jetons: map[string]int{"gemini": 100000}}
		if err := orch.NouvelleMission("gemini-piege", "gemini-exec", "gemini-recep", rolesTest(), plaf); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := orch.Executer(ctx, "gemini-piege", "gemini-exec", "exec"); err != nil {
			t.Fatal(err)
		}
		if _, err := orch.Soumettre(ctx, "gemini-piege", "gemini-exec", "LIVRABLE-G-1"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := orch.Recevoir(ctx, "gemini-piege", "gemini-recep", "LIVRABLE-G-1"); err == nil {
			t.Fatalf("verdict piégé (%s) doit être rejeté", piege.nom)
		}
		enr, _ := ref.Obtenir("gemini-piege")
		if enr.Mission.State != spec.EtatDecisionAttendue {
			t.Fatalf("AUCUNE transition ne doit être committée (%s) : %s", piege.nom, enr.Mission.State)
		}
	}
}

// refTemp : référentiel de test (dossier temporaire).
func refTemp(t *testing.T) (*missions.Referentiel, error) {
	return missions.Ouvrir(t.TempDir())
}

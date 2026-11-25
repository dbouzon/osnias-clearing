// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette C1..C15 du connecteur Anthropic Claude (mission M-COORD-LLM-01,
// issue #40, plan v2 §3) : serveurs factices loopback UNIQUEMENT, au format
// Messages API, HORS RÉSEAU, AUCUNE CLÉ RÉELLE, 0 €. Toute réponse piégée
// doit être refusée (fail closed), jamais réparée.
package connecteurs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// ---- helpers de recette (format Messages API) ----

// corpsAnthropic : corps de requête Messages API observé par les factices.
type corpsAnthropic struct {
	Model     string       `json:"model"`
	System    string       `json:"system"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []messageAPI `json:"messages"`
}

// reponseAnthropic : réponse Messages API émise par les factices.
func emissionAnthropic(blocs []blocAnthropic, modele, stop string, in, out int) []byte {
	b, _ := json.Marshal(map[string]any{
		"model":       modele,
		"content":     blocs,
		"stop_reason": stop,
		"usage":       map[string]any{"input_tokens": in, "output_tokens": out},
	})
	return b
}

func reponseAnthropicTexte(texte, modele, stop string, in, out int) []byte {
	return emissionAnthropic([]blocAnthropic{{Type: "text", Text: texte}}, modele, stop, in, out)
}

func blocTexte(s string) blocAnthropic { return blocAnthropic{Type: "text", Text: s} }

// lireCorpsAnthropic : corps Messages API décodé, fail closed du banc.
func lireCorpsAnthropic(t *testing.T, r *http.Request) corpsAnthropic {
	t.Helper()
	var c corpsAnthropic
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		t.Fatalf("requête factice non JSON : %v", err)
	}
	return c
}

func configAnthropicTest(endpoint string) *Config {
	return &Config{
		Mode: ModeSimule,
		Fournisseurs: []ConfigFournisseur{{
			ID: "anthropic", Endpoint: endpoint, Modele: "claude-test",
			Format:    FormatAnthropic,
			TimeoutMS: 2000, TailleMaxReponse: 65536, RetraitsMax: 1,
			JetonsEntreeMaxParTentative: 1000, JetonsSortieMaxParTentative: 555,
		}},
	}
}

// nouveauConnecteurAnthropicReelFactice : connecteur AnthropicPoint posé
// DIRECTEMENT avec simuleLoc=false (hors verrou REEL, serveur factice
// loopback 127.0.0.1 : 0 €, jamais facturable) pour éprouver la voûte
// monétaire et le bornage à l'émission (C9/C14).
func nouveauConnecteurAnthropicReelFactice(t *testing.T, handler http.HandlerFunc,
	tarifEntree, tarifSortie, tarifRequete, borneEntree, borneSortie, retraits int) (*AnthropicPoint, *int) {
	t.Helper()
	appels := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels++
		if handler != nil {
			handler(w, r)
			return
		}
		w.Write(reponseAnthropicTexte("LIVRABLE FACTICE", "exec-modele", "end_turn", 400, 1100))
	}))
	t.Cleanup(s.Close)
	h := &AnthropicPoint{
		id: "anthropic", endpoint: s.URL, modele: "exec-modele",
		client:    &http.Client{Timeout: 5 * time.Second},
		tailleMax: 65536, retraits: retraits, simuleLoc: false,
		tarifEntreeMillion: tarifEntree, tarifSortieMillion: tarifSortie,
		tarifRequete:    tarifRequete,
		jetonsEntreeMax: borneEntree, jetonsSortieMax: borneSortie,
	}
	return h, &appels
}

// ---- C1/C11/C12 : succès nominal, en-têtes exacts, max_tokens transmis ----

func TestAnthropicC1SuccesEnTetesEtMaxTokens(t *testing.T) {
	cle := "sk-ant-factice-1234567890ABCDEF"
	t.Setenv("COORD_CLE_ANTHROPIC_FACTICE", cle)
	var corpsRecu corpsAnthropic
	var versionRecue, xAPIKey, authorization string
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		corpsRecu = lireCorpsAnthropic(t, r)
		versionRecue = r.Header.Get("anthropic-version")
		xAPIKey = r.Header.Get("x-api-key")
		authorization = r.Header.Get("Authorization")
		// C1 : plusieurs blocs texte, concaténés ; usage complet.
		w.Write(emissionAnthropic([]blocAnthropic{
			blocTexte("PREMIER MORCEAU. "), blocTexte("SECOND MORCEAU."),
		}, "claude-test", "end_turn", 31, 7))
	})
	cfg := configAnthropicTest(s.URL)
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_ANTHROPIC_FACTICE"
	h, err := cfg.NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h.Executer("mandat d'essai")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	if res.Jetons != 38 {
		t.Fatalf("jetons = input+output attendus (38), obtenu %d", res.Jetons)
	}
	if res.Sortie != "PREMIER MORCEAU. SECOND MORCEAU." {
		t.Fatalf("concaténation des blocs attendue : %q", res.Sortie)
	}
	// C11 : en-têtes EXACTS exigés : x-api-key + anthropic-version, JAMAIS Bearer.
	if xAPIKey != cle {
		t.Fatalf("x-api-key attendu, obtenu %q", xAPIKey)
	}
	if authorization != "" {
		t.Fatalf("Authorization Bearer INTERDIT pour Anthropic : %q", authorization)
	}
	if versionRecue != anthropicVersion {
		t.Fatalf("anthropic-version épinglé %q attendu, obtenu %q", anthropicVersion, versionRecue)
	}
	// C12 : max_tokens transmis = borne déclarée jetons_sortie_max_par_tentative.
	if corpsRecu.MaxTokens != 555 {
		t.Fatalf("max_tokens = borne déclarée (555) attendu, obtenu %d", corpsRecu.MaxTokens)
	}
	// Format Messages API : system de premier niveau, messages user.
	if corpsRecu.System == "" || !strings.Contains(corpsRecu.System, "coéquipier COORD") {
		t.Fatalf("gabarit system top-level attendu : %q", corpsRecu.System)
	}
	if len(corpsRecu.Messages) != 1 || corpsRecu.Messages[0].Role != "user" ||
		corpsRecu.Messages[0].Content != "mandat d'essai" {
		t.Fatalf("messages user inattendus : %+v", corpsRecu.Messages)
	}
	if corpsRecu.Model != "claude-test" {
		t.Fatalf("model déclaré attendu, obtenu %q", corpsRecu.Model)
	}
}

// ---- C2 : usage absent ou nul => JETONS_INCONNUS ----

func TestAnthropicC2JetonsInconnus(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("ok", "claude-test", "end_turn", 0, 0))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "JETONS_INCONNUS") {
		t.Fatalf("budget non garanti : %v", err)
	}
}

// ---- C3 : bloc contenu non-texte => refus fail closed, jamais réparé ----

func TestAnthropicC3BlocNonTexteRefuse(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		// réfugié comme bloc tool_use : doit être refusé INTÉGRALEMENT
		w.Write(emissionAnthropic([]blocAnthropic{
			blocTexte("texte valide"), {Type: "tool_use", Text: "id_outil"},
		}, "claude-test", "end_turn", 10, 20))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "fail closed") {
		t.Fatalf("bloc non-texte doit être refusé intégral : %v", err)
	}
	if res.Sortie != "" {
		t.Fatalf("aucune sortie ne doit fuiter d'une réponse refusée : %q", res.Sortie)
	}
}

// ---- C4 : stop_reason=max_tokens => troncature refusée ----

func TestAnthropicC4TroncatureRefusee(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("réponse partielle…", "claude-test", "max_tokens", 10, 555))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "SORTIE_TRONQUEE") {
		t.Fatalf("sortie tronquée refusée attendue : %v", err)
	}
	if strings.Contains(fmt.Sprint(err), "partielle") {
		t.Fatalf("le contenu tronqué ne doit jamais être utilisé ni affiché")
	}
}

// ---- C5 : modèle divergent => MODELE_DIVERGENT ----

func TestAnthropicC5ModeleDivergent(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("ok", "AUTRE-MODELE", "end_turn", 10, 20))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "MODELE_DIVERGENT") {
		t.Fatalf("identité répondante non vérifiée : %v", err)
	}
}

// ---- C3b/D3 : champs de cache non nuls => JETONS_INCONNUS (fail closed) ----

func TestAnthropicCacheNonNulRefuse(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{
			"model": "claude-test", "content": []any{map[string]any{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 20,
				"cache_creation_input_tokens": 5},
		})
		w.Write(b)
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "JETONS_INCONNUS") {
		t.Fatalf("usage de cache inattendu doit être refusé : %v", err)
	}
}

// ---- D3 : stop_reason inconnu => refus (fail closed) ----

func TestAnthropicStopRaisonInconnueRefusee(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("ok", "claude-test", "refusal", 10, 20))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "STOP_RAISON_INCONNUE") {
		t.Fatalf("stop_reason inconnu doit être refusé : %v", err)
	}
}

// ---- C6 : 429/5xx => retraits BORNÉS puis APPEL_ECHOUE ----

func TestAnthropicC6RetraitsBornes(t *testing.T) {
	appels := 0
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		appels++
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("{\"error\":\"indisponible\"}"))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "APPEL_ECHOUE") {
		t.Fatalf("APPEL_ECHOUE attendu après retraits : %v", err)
	}
	if appels != 2 { // retraits_max = 1 : 1 essai + 1 retrait, JAMAIS plus
		t.Fatalf("retraits bornés : 2 appels attendus, obtenu %d", appels)
	}
}

// ---- C7 : délai dépassé, annulation par contexte ----

func TestAnthropicC7DelaiEtAnnulation(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.Write(reponseAnthropicTexte("tardif", "claude-test", "end_turn", 1, 1))
	})
	cfg := configAnthropicTest(s.URL)
	cfg.Fournisseurs[0].TimeoutMS = 50
	cfg.Fournisseurs[0].RetraitsMax = 0
	h, err := cfg.NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil {
		t.Fatal("délai dépassé attendu")
	}
	// Annulation par contexte : l'appel est annulable, jamais bloquant.
	ctx, annuler := context.WithCancel(context.Background())
	annuler()
	_, err = h.ExecuterContexte(ctx, "executer", "mandat", "cle-idem")
	if err == nil {
		t.Fatal("contexte annulé doit refuser l'appel")
	}
}

// ---- C8 : réponse surdimensionnée => REPONSE_TROP_GRANDE ----

func TestAnthropicC8ReponseTropGrande(t *testing.T) {
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 10000))
	})
	cfg := configAnthropicTest(s.URL)
	cfg.Fournisseurs[0].TailleMaxReponse = 512
	h, err := cfg.NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "REPONSE_TROP_GRANDE") {
		t.Fatalf("refus de taille attendu : %v", err)
	}
}

// ---- C9 : charge hors borne refusée À L'ÉMISSION (aucun octet émis) ----

func TestAnthropicC9ChargeHorsBorneAvantEmission(t *testing.T) {
	h, appels := nouveauConnecteurAnthropicReelFactice(t, nil, 100, 300, 1, 10, 1000, 0)
	_, err := h.Executer("mandat très long qui dépasse largement la borne déclarée de dix jetons d'entrée")
	if err == nil || !strings.Contains(err.Error(), "CHARGE_HORS_BORNE") {
		t.Fatalf("CHARGE_HORS_BORNE attendu : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("AUCUN octet n'a dû être émis : %d", *appels)
	}
}

// ---- C10 : clé dans la sortie => SORTIE_COMPROMISE ; expurgation ----

func TestAnthropicC10SortieCompromiseEtExpurge(t *testing.T) {
	cle := "sk-ant-CLETESTAZERTY1234567890"
	t.Setenv("COORD_CLE_ANTHROPIC_FACTICE", cle)
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("voici la cle "+cle, "claude-test", "end_turn", 10, 20))
	})
	cfg := configAnthropicTest(s.URL)
	cfg.Fournisseurs[0].CleEnv = "COORD_CLE_ANTHROPIC_FACTICE"
	h, err := cfg.NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	_, err = h.Executer("mandat")
	if err == nil || !strings.Contains(err.Error(), "SORTIE_COMPROMISE") {
		t.Fatalf("fuite de clé non détectée : %v", err)
	}
	if strings.Contains(err.Error(), cle) {
		t.Fatalf("l'erreur elle-même fuit la clé")
	}
	// Expurgation du motif de clé générique (sk-…).
	s2 := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(reponseAnthropicTexte("cle suspecte sk-abcdefghijklmnopqrstuvwxyz1234", "claude-test", "end_turn", 10, 20))
	})
	h2, err := configAnthropicTest(s2.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	res, err := h2.Executer("mandat")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	if !strings.Contains(res.Sortie, "[EXPURGE]") || strings.Contains(res.Sortie, "sk-abc") {
		t.Fatalf("expurgation attendue : %q", res.Sortie)
	}
}

// ---- C13 : anti-doublon appel_en_cours avec le connecteur Anthropic ----

func TestAnthropicC13AntiDoublonAvecConnecteurAnthropic(t *testing.T) {
	libre := make(chan struct{})
	premier := true
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		if premier {
			premier = false
			<-libre // bloque le premier appel jusqu'au feu vert du banc
		}
		w.Write(reponseAnthropicTexte("LIVRABLE FACTICE", "claude-test", "end_turn", 400, 1100))
	})
	h, err := configAnthropicTest(s.URL).NouveauAnthropicPoint("anthropic")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	ref, _ := missions.Ouvrir(t.TempDir())
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"anthropic": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "anth-doublon-1"
	p := plafondsTest()
	p.Appels["anthropic"] = 20
	p.Jetons["anthropic"] = 200000
	if err := orch.NouvelleMission(issue, "anth-exec", "anth-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	termine := make(chan error, 1)
	go func() {
		_, err := orch.Executer(context.Background(), issue, "anth-exec", "mandat")
		termine <- err
	}()
	// Le double appel est IMPOSSIBLE tant que l'appel en cours n'est pas
	// résolu par le pilote (fail closed).
	// Attendre que l'appel en cours soit PERSISTÉ avant de tenter le double.
	fichier := cheminAppel(ref.Racine(), issue)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(fichier); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Le double appel est IMPOSSIBLE tant que l'appel en cours n'est pas
	// résolu par le pilote (fail closed).
	_, refusErr := orch.Executer(context.Background(), issue, "anth-exec", "double")
	close(libre)
	if err := <-termine; err != nil {
		t.Fatalf("premier appel : %v", err)
	}
	if refusErr == nil || !strings.Contains(refusErr.Error(), "APPEL_EN_COURS_NON_RESOLU") {
		t.Fatalf("refus anti-doublon attendu pendant l'appel en cours : %v", refusErr)
	}
}

// ---- C14 : voûte monétaire, réservation AVANT réseau, BUDGET_DEPASSE ----

func TestAnthropicC14VouteMonetaireAvantReseau(t *testing.T) {
	h, appels := nouveauConnecteurAnthropicReelFactice(t, nil, 100, 300, 1, 4000, 1000, 0)
	ref, _ := missions.Ouvrir(t.TempDir())
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{"anthropic": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "anth-monetaire-1"
	p := plafondsTest()
	p.Appels["anthropic"] = 20
	p.Jetons["anthropic"] = 200000
	p.Centimes = 1 // 1 centime : l'enveloppe (3 centimes) doit être refusée
	if err := orch.NouvelleMission(issue, "anth-exec", "anth-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "anth-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("BUDGET_DEPASSE attendu AVANT réseau : %v", err)
	}
	if *appels != 0 {
		t.Fatalf("AUCUNE requête ne devait être émise : %d", *appels)
	}
	// Avec un plafond suffisant : réservation DURABLE avant l'appel,
	// journalisée, conservée (jamais libérée).
	ref2, _ := missions.Ouvrir(t.TempDir())
	orch2, err := NouvelOrchestrateur(ref2, map[string]adaptateurs.Adaptateur{"anthropic": h})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	p2 := p
	p2.Centimes = 100
	if err := orch2.NouvelleMission(issue, "anth-exec", "anth-recep", rolesTest(), p2); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch2.Executer(context.Background(), issue, "anth-exec", "mandat"); err != nil {
		t.Fatalf("appel autorisé : %v", err)
	}
	enr, _ := ref2.Obtenir(issue)
	if enr.Budget.Etat.CentimesReserves["anthropic"] != 3 {
		t.Fatalf("réservation durable attendue (3 centimes) : %+v", enr.Budget.Etat.CentimesReserves)
	}
	j, err := os.ReadFile(missions.JournalPath(ref2.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	if !strings.Contains(string(j), "APPEL_RESERVE") || !strings.Contains(string(j), "centimes_reserves") {
		t.Fatalf("le journal doit tracer la réservation monétaire")
	}
}

// ---- C15 : soumissions piégées du réceptionnaire => AUCUN commit à tort ----
//
// Banc E2E : Mistral exécutant factice (format openai) + Claude
// réceptionnaire factice (format anthropic). Les ids de connecteur triés
// définissent les rôles (NouvelleMission : premier = exécutant, dernier =
// réceptionnaire) : "exec-mistral" < "recep-claude".

type bancAnthropic struct {
	ref              *missions.Referentiel
	orch             *Orchestrateur
	exec             *httptest.Server
	recep            *httptest.Server
	appelsExec       int
	appelsRecep      int
	verdictPlan      string // "" => PLAN_ACCEPTE
	verdictReception string // "" => ACCEPTEE
	sortieBrutePlan  string // pivot de verdict piégé (critique)
	sortieBruteRecep string // pivot de verdict piégé (réception)
}

func nouveauBancAnthropic(t *testing.T) *bancAnthropic {
	t.Helper()
	b := &bancAnthropic{}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	b.ref = ref

	b.exec = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.appelsExec++
		suffix := r.Header.Get("Idempotency-Key")
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		w.Write(reponseChat("LIVRABLE-V"+suffix+", conforme aux critères", "exec-modele", 1500))
	}))
	t.Cleanup(b.exec.Close)

	b.recep = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.appelsRecep++
		// C11 (anthropic) : en-têtes exacts exigés du réceptionnaire Claude.
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version épinglé attendu")
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization Bearer interdit pour Anthropic")
		}
		var corps corpsAnthropic
		_ = json.NewDecoder(r.Body).Decode(&corps)
		contenu := corps.Messages[len(corps.Messages)-1].Content
		if corps.System == "" {
			t.Errorf("gabarit system top-level attendu")
		}
		if strings.Contains(contenu, "CRITIQUE DE PLAN") {
			if b.sortieBrutePlan != "" {
				w.Write(reponseAnthropicTexte(b.sortieBrutePlan, "recep-modele", "end_turn", 200, 300))
				return
			}
			emp := extraireEmpreinte(t, contenu)
			verdict := b.verdictPlan
			if verdict == "" {
				verdict = "PLAN_ACCEPTE"
			}
			out, _ := json.Marshal(map[string]any{
				"verdict": verdict, "points": []string{"rien de bloquant"},
				"points_detail": []map[string]string{
					{"severite": "faible", "texte": "aucun écart"}},
				"empreinte_attendue": emp,
			})
			w.Write(reponseAnthropicTexte(string(out), "recep-modele", "end_turn", 200, 300))
			return
		}
		if strings.Contains(contenu, "RECEPTION INDEPENDANTE") {
			if b.sortieBruteRecep != "" {
				w.Write(reponseAnthropicTexte(b.sortieBruteRecep, "recep-modele", "end_turn", 200, 300))
				return
			}
			emp := extraireEmpreinte(t, contenu)
			verdict := b.verdictReception
			if verdict == "" {
				verdict = "ACCEPTEE"
			}
			out, _ := json.Marshal(map[string]any{
				"verdict": verdict, "points": []string{"critique factuelle"},
				"points_detail": []map[string]string{
					{"severite": "faible", "texte": "réserves non bloquantes couvertes"}},
				"empreinte_attendue": emp,
				"verdict_id":         "verdict-" + fmt.Sprint(b.appelsRecep),
				"verdict_url":        "urn:test:anth",
			})
			w.Write(reponseAnthropicTexte(string(out), "recep-modele", "end_turn", 200, 300))
			return
		}
		w.Write(reponseAnthropicTexte("SOUMISSION COMPRISE", "recep-modele", "end_turn", 200, 50))
	}))
	t.Cleanup(b.recep.Close)

	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "exec-mistral", Endpoint: b.exec.URL, Modele: "exec-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
		{ID: "recep-claude", Endpoint: b.recep.URL, Modele: "recep-modele",
			Format: FormatAnthropic, TimeoutMS: 5000, TailleMaxReponse: 65536, RetraitsMax: 0},
	}}
	exec, err := cfg.NouveauHTTPEndPoint("exec-mistral")
	if err != nil {
		t.Fatalf("connecteur exécutant : %v", err)
	}
	recep, err := cfg.NouveauAnthropicPoint("recep-claude")
	if err != nil {
		t.Fatalf("connecteur réceptionnaire anthropic : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{
		"exec-mistral": exec, "recep-claude": recep,
	})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	b.orch = orch
	return b
}

func (b *bancAnthropic) etat(t *testing.T, issue string) string {
	t.Helper()
	enr, err := b.ref.Obtenir(issue)
	if err != nil {
		t.Fatalf("mission %s : %v", issue, err)
	}
	return enr.Mission.State
}

func plafondsAnthropicTest() budget.Plafonds {
	p := plafondsTest()
	p.Appels["exec-mistral"] = 20
	p.Jetons["exec-mistral"] = 200000
	p.Appels["recep-claude"] = 20
	p.Jetons["recep-claude"] = 200000
	p.Centimes = 10000
	return p
}

func TestAnthropicC15VerdictsPiegesJamaisCommittes(t *testing.T) {
	b := nouveauBancAnthropic(t)
	issue := "anth-c15"
	// Exécutant qualifié par exec-mistral, réceptionnaire par recep-claude.
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "claude-recep", rolesTest(), plafondsAnthropicTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", "LIVRABLE-A"); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("état DECISION_ATTENDUE attendu : %s", b.etat(t, issue))
	}

	// Livrable non conforme (empreinte divergente) : refus SANS appel réseau.
	av := b.appelsRecep
	if _, _, err := b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-A MODIFIE"); err == nil ||
		!strings.Contains(err.Error(), "livrable non conforme") {
		t.Fatalf("refus empreinte attendu : %v", err)
	}
	if b.appelsRecep != av {
		t.Fatalf("aucun appel ne devait partir pour un livrable non conforme")
	}

	// Piège 1 : verdict hors gabarit JSON strict.
	b.sortieBruteRecep = "ACCEPTEE sans JSON, tout va bien je valide"
	_, _, err := b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-A")
	if err == nil || !strings.Contains(err.Error(), "verdict non JSON") {
		t.Fatalf("verdict hors gabarit devait être refusé : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("aucune transition ne devait être committée : %s", b.etat(t, issue))
	}

	// Piège 2 : faux ACCEPTEE NON ANCRÉ (empreinte divergente).
	b.sortieBruteRecep = `{"verdict":"ACCEPTEE","points":[],"empreinte_attendue":"` +
		strings.Repeat("0", 64) + `","verdict_id":"v2","verdict_url":"urn:x"}`
	_, _, err = b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-A")
	if err == nil || !strings.Contains(err.Error(), "non ancré") {
		t.Fatalf("verdict non ancré devait être refusé : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("aucune transition ne devait être committée : %s", b.etat(t, issue))
	}

	// Piège 3 : ACCEPTEE ancré MAIS sans référence de verdict (sans preuve).
	empA := empreinteCharge("LIVRABLE-A")
	b.sortieBruteRecep = `{"verdict":"ACCEPTEE","points":[],"empreinte_attendue":"` +
		empA + `","verdict_url":"urn:x"}`
	_, _, err = b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-A")
	if err == nil || !strings.Contains(err.Error(), "sans référence") {
		t.Fatalf("verdict sans preuve devait être refusé : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("aucune transition ne devait être committée : %s", b.etat(t, issue))
	}

	// Verdict conforme (gabarit strict + points_detail sévérité) : COMMIT
	// ACCEPTEE, puis ACK_RECU ; la clôture reste refusée au réceptionnaire.
	b.sortieBruteRecep = "" // le factice reprend l'ancre depuis le prompt
	if _, _, err := b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-A"); err != nil {
		t.Fatalf("réception conforme : %v", err)
	}
	if b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("ACCEPTEE committée attendue : %s", b.etat(t, issue))
	}
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("ACK : %v", err)
	}
	if b.etat(t, issue) != spec.EtatACKRecu {
		t.Fatalf("ACK_RECU attendu : %s", b.etat(t, issue))
	}
	if err := b.orch.Cloturer(issue); err == nil ||
		!strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
		t.Fatalf("la clôture reste réservée au pilote : %v", err)
	}
}

// ---- Scénario E2E simulé : Mistral exécutant / Claude réceptionnaire ----
// critique de plan, exécution, soumission, réception A_CORRIGER, correction,
// nouvelle soumission, réception ACCEPTEE, ACK_RECU. 0 €, aucun appel réel.

func TestAnthropicDemoE2ECritiqueCorrectionReceptionACK(t *testing.T) {
	b := nouveauBancAnthropic(t)
	issue := "anth-e2e-1"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "claude-recep", rolesTest(), plafondsAnthropicTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()

	// 1. Critique de plan par Claude : verdict JSON strict ancré.
	vp, err := b.orch.CritiquerPlan(ctx, issue, "claude-recep",
		"Plan v2 : connecteur anthropic.go, tests C1..C15, documentation FR/EN.")
	if err != nil {
		t.Fatalf("critique de plan : %v", err)
	}
	if vp.Verdict != "PLAN_ACCEPTE" {
		t.Fatalf("PLAN_ACCEPTE attendu : %+v", vp)
	}
	if b.etat(t, issue) != spec.EtatPlanifiee {
		t.Fatalf("la critique de plan ne change pas l'état : %s", b.etat(t, issue))
	}

	// 2. Exécution Mistral, soumission.
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", "LIVRABLE-B"); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("état DECISION_ATTENDUE attendu : %s", b.etat(t, issue))
	}

	// 3. Première réception : A_CORRIGER => correction exigée.
	b.verdictReception = "A_CORRIGER"
	if _, _, err := b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-B"); err != nil {
		t.Fatalf("réception 1 : %v", err)
	}
	if b.etat(t, issue) != spec.EtatEnCorrection {
		t.Fatalf("EN_CORRECTION attendue : %s", b.etat(t, issue))
	}

	// 4. Correction par l'exécutant, nouvelle soumission.
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "correction du livrable"); err != nil {
		t.Fatalf("correction : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", "LIVRABLE-B CORRIGE"); err != nil {
		t.Fatalf("nouvelle soumission : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("état DECISION_ATTENDUE attendu : %s", b.etat(t, issue))
	}

	// 5. Seconde réception : ACCEPTEE, ACK_RECU.
	b.verdictReception = "ACCEPTEE"
	if _, _, err := b.orch.Recevoir(ctx, issue, "claude-recep", "LIVRABLE-B CORRIGE"); err != nil {
		t.Fatalf("réception 2 : %v", err)
	}
	if b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("ACCEPTEE attendue : %s", b.etat(t, issue))
	}
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("ACK : %v", err)
	}
	if b.etat(t, issue) != spec.EtatACKRecu {
		t.Fatalf("ACK_RECU attendu : %s", b.etat(t, issue))
	}

	// Traçabilité : chaque appel du réceptionnaire Claude est passé par le
	// connecteur anthropic (en-têtes Messages API) et journalisé.
	j := b.journal(t, issue)
	if !strings.Contains(j, "APPEL_REEL_JOURNALISE") && !strings.Contains(j, "verdict") {
		t.Fatalf("journal incomplet : %s", j)
	}
}

func (b *bancAnthropic) journal(t *testing.T, issue string) string {
	t.Helper()
	j, err := os.ReadFile(missions.JournalPath(b.ref.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	return string(j)
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/missions"
)

// ---- helpers partagés du banc de tests maïeutique (0 €, loopback) ----

func contextBG() context.Context { return context.Background() }

func plafondsTest() budget.Plafonds {
	return budget.Plafonds{
		Appels: map[string]int{"exec-factice": 20, "recep-factice": 20},
		Jetons: map[string]int{"exec-factice": 200000, "recep-factice": 200000},
	}
}

func rolesTest() map[string][]string {
	return map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
}

// paramTransmission : la mission fille témoin (plan v2 §5) : identifiant
// DISTINCT MAIEUTIQUE-TEMOIN-01, binôme exec-factice/recep-factice,
// lien vers #42 journalisé.
func paramTransmission() Transmission {
	return Transmission{
		IssueFille:    "MAIEUTIQUE-TEMOIN-01",
		ExecutantID:   "openai-exec",
		RecepteurID:   "openai-recep",
		QuaExecID:     "exec-factice",
		QuaRecepID:    "recep-factice",
		Roles:         rolesTest(),
		Plafonds:      plafondsTest(),
		LienIssueMere: "dbouzon/osnias-clearing#42",
	}
}

// orchestrateurTest : l'orchestrateur EXISTANT du moteur sur un
// référentiel temporaire, registre = binôme factice (SIMULE, 0 €).
func orchestrateurTest(t *testing.T) (*connecteurs.Orchestrateur, *missions.Referentiel) {
	t.Helper()
	ref, err := missions.Ouvrir(filepath.Join(t.TempDir(), "missions"))
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	reg := map[string]adaptateurs.Adaptateur{}
	for _, id := range []string{"exec-factice", "recep-factice"} {
		s, err := adaptateurs.NouveauSimule(id)
		if err != nil {
			t.Fatalf("simule %s : %v", id, err)
		}
		reg[id] = s
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	return orch, ref
}

// cfgSimule : configuration de test (sondes désactivées, SIMULÉ).
func cfgSimule(racine string, directeur adaptateurs.Adaptateur,
	orch *connecteurs.Orchestrateur, ref *missions.Referentiel) Config {
	cfg := ConfigParDefaut(racine)
	cfg.Adaptateurs = map[string]adaptateurs.Adaptateur{
		"directeur-factice": directeur,
		"exec-factice":      mustSimule("exec-factice"),
		"recep-factice":     mustSimule("recep-factice"),
	}
	cfg.Orchestrateur = orch
	cfg.Referentiel = ref
	cfg.Transmission = paramTransmission()
	return cfg
}

func mustSimule(id string) adaptateurs.Adaptateur {
	s, err := adaptateurs.NouveauSimule(id)
	if err != nil {
		panic(err)
	}
	return s
}

// magasinTest : un Store maïeutique complet, directeur factice HTTP.
func magasinTest(t *testing.T) (*Store, *facticeDirecteur) {
	t.Helper()
	f := nouveauFacticeDirecteur(t)
	orch, ref := orchestrateurTest(t)
	cfg := cfgSimule(filepath.Join(t.TempDir(), "maieutique"),
		f.adaptateur(t), orch, ref)
	store, err := OuvrirStore(cfg)
	if err != nil {
		t.Fatalf("store : %v", err)
	}
	return store, f
}

// projetDialoguer : un projet en DIALOGUE avec la source du mandat #42
// v1.4 déclarée par le pilote (exigée par ordreComplet).
func projetDialoguer(t *testing.T, store *Store, intention string) *Projet {
	t.Helper()
	p, err := store.DeclarerIntention("temoin", intention)
	if err != nil {
		t.Fatalf("déclarer intention : %v", err)
	}
	if _, err := store.DeclarerSources("temoin", []string{
		"issue dbouzon/osnias-clearing#42 version 1.4",
	}); err != nil {
		t.Fatalf("déclarer sources : %v", err)
	}
	p, err = store.OuvrirDialogue("temoin", "directeur-factice", "exec-factice")
	if err != nil {
		t.Fatalf("ouvrir dialogue : %v", err)
	}
	return p
}

// ordreComplet : un ordre CONFORME minimal (source = issue #42 v1.4,
// contrôle intermédiaire présent, livrable précis).
func ordreComplet() *Ordre {
	return &Ordre{
		Identification: Identification{
			Mission:   "M-TEMOIN-01",
			Version:   "v1",
			Protocole: "COORD v1.5 / COORD Specification 1.0",
		},
		EtatDesLieux: "Coord v1.5 opérationnelle, main f6852813, aucun module maïeutique pré-mission.",
		Sources: []Source{
			{Ref: "issue dbouzon/osnias-clearing#42 version 1.4"},
		},
		Objectif: "Qualifier la mission témoin de recette maïeutique",
		Etapes: []Etape{
			{Nom: "T1 inventaire", ControleIntermediaire: "inventaire vérifié par le directeur"},
			{Nom: "T2 développement", ControleIntermediaire: ""},
			{Nom: "T6 recette", ControleIntermediaire: ""},
		},
		Limites:           []string{"0 €", "aucun appel réel", "loopback uniquement"},
		LiberteExecution:  "Le contenu technique de chaque étape relève de l'exécutant.",
		Livrable:          "Rapport de recette maïeutique avec journal et empreintes",
		CriteresReception: []string{"scénarios S1 à S16 vérifiés", "0 double publication", "0 double transmission"},
		ApplicationCoord:  "Protocole COORD v1.5 : une transition par opération, journal complet.",
	}
}

// ordreJSON : la forme JSON brute d'un ordre (charge de proposition).
func ordreJSON(t *testing.T, o *Ordre) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal ordre : %v", err)
	}
	return b
}

// ---- directeur factice : serveur loopback compatible OpenAI ----

// facticeDirecteur : un directeur factice EXIGEANT : il répercute le
// modèle demandé (sinon MODELE_DIVERGENT du connecteur réel), sert une
// FILE de réponses JSON (une par tour), et peut simuler N indisponibilités
// (f.echecs) pour éprouver la borne ATTENTE_PILOTE.
type facticeDirecteur struct {
	srv      *httptest.Server
	modele   string
	mu       sync.Mutex
	reponses []string // file : une réponse JSON par tour du directeur
	echecs   int      // indisponibilités restantes à servir (HTTP 500)
	appels   int
}

func nouveauFacticeDirecteur(t *testing.T) *facticeDirecteur {
	t.Helper()
	f := &facticeDirecteur{modele: "gpt-factice"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.appels++
		restants := f.echecs
		if restants > 0 {
			f.echecs--
		}
		var rep struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
			f.mu.Unlock()
			http.Error(w, "corps illisible", http.StatusBadRequest)
			return
		}
		if restants > 0 {
			f.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"indisponible"}`))
			return
		}
		if len(f.reponses) == 0 {
			f.mu.Unlock()
			http.Error(w, "file de réponses vide", http.StatusInternalServerError)
			return
		}
		suivante := f.reponses[0]
		f.reponses = f.reponses[1:]
		modele := rep.Model
		f.mu.Unlock()
		corps := map[string]any{
			"model": modele, // le modèle demandé est RÉPERCUTÉ (identité vérifiée)
			"choices": []map[string]any{
				{"message": map[string]string{"content": suivante}},
			},
			"usage": map[string]int{"total_tokens": 1234},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(corps)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// ajouter : empile des réponses (JSON du directeur) dans la file.
func (f *facticeDirecteur) ajouter(reponses ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reponses = append(f.reponses, reponses...)
}

// indisponibilites : les N prochains appels répondront 500.
func (f *facticeDirecteur) indisponibilites(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.echecs = n
}

// adaptateur : le connecteur RÉEL (HTTPEndPoint loopback SIMULE) du
// directeur factice — le module maïeutique ne sait pas qu'il est factice.
func (f *facticeDirecteur) adaptateur(t *testing.T) adaptateurs.Adaptateur {
	t.Helper()
	cfg := connecteurs.Config{
		Mode: connecteurs.ModeSimule,
		Fournisseurs: []connecteurs.ConfigFournisseur{{
			ID: "directeur-factice", Endpoint: f.srv.URL, Modele: f.modele,
			TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 0,
		}},
	}
	ep, err := cfg.NouveauHTTPEndPoint("directeur-factice")
	if err != nil {
		t.Fatalf("endpoint factice : %v", err)
	}
	return ep
}

// compteurAppels : nombre d'appels reçus (preuves : zéro second appel).
func (f *facticeDirecteur) compteurAppels() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appels
}

// reponseQuestion/Clarification/Verdict : gabarits de réponses.
func reponseQuestion(contenu string) string {
	b, _ := json.Marshal(map[string]any{"action": "QUESTION", "contenu": contenu})
	return string(b)
}

func reponseClarification(contenu string) string {
	b, _ := json.Marshal(map[string]any{"action": "CLARIFICATION", "contenu": contenu})
	return string(b)
}

func reponseVerdict(verdict string, points ...[2]string) string {
	pts := []map[string]string{}
	for _, p := range points {
		pts = append(pts, map[string]string{"severite": p[0], "texte": p[1]})
	}
	b, _ := json.Marshal(map[string]any{
		"action": "VERDICT", "verdict": verdict, "points_detail": pts,
	})
	return string(b)
}

func reponsePropositionOrdre(o *Ordre) string {
	b, _ := json.Marshal(map[string]any{"action": "PROPOSITION_ORDRE", "ordre": o})
	return string(b)
}

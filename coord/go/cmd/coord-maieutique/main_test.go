// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/maieutique"
	"coordengine/internal/missions"
)

// serviceTest : un service complet sur directeur factice (0 €, loopback),
// token STABLE de test.
func serviceTest(t *testing.T) (*service, *facticeServiceDirecteur) {
	t.Helper()
	f := nouveauFacticeServiceDirecteur(t)
	orch, ref := orchestrateurDeTest(t)
	cfg := maieutique.ConfigParDefaut(t.TempDir())
	cfg.Adaptateurs = map[string]adaptateurs.Adaptateur{
		"directeur-factice": f.adaptateur(t),
		"exec-factice":      simule(t, "exec-factice"),
		"recep-factice":     simule(t, "recep-factice"),
	}
	cfg.Orchestrateur = orch
	cfg.Referentiel = ref
	cfg.Transmission = maieutique.Transmission{
		IssueFille:  "MAIEUTIQUE-TEMOIN-01",
		ExecutantID: "openai-exec",
		RecepteurID: "openai-recep",
		QuaExecID:   "exec-factice",
		QuaRecepID:  "recep-factice",
		Roles: map[string][]string{
			"EXECUTANT":      {"executer", "soumettre", "acker"},
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		Plafonds: plafondsDeTest(),
	}
	store, err := maieutique.OuvrirStore(cfg)
	if err != nil {
		t.Fatalf("store : %v", err)
	}
	s := &service{store: store, token: "token-test"}
	return s, f
}

func simule(t *testing.T, id string) adaptateurs.Adaptateur {
	t.Helper()
	a, err := adaptateurs.NouveauSimule(id)
	if err != nil {
		t.Fatalf("simule : %v", err)
	}
	return a
}

func orchestrateurDeTest(t *testing.T) (*connecteurs.Orchestrateur, *missions.Referentiel) {
	t.Helper()
	ref, err := missions.Ouvrir(filepath.Join(t.TempDir(), "missions"))
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	reg := map[string]adaptateurs.Adaptateur{
		"exec-factice":  simule(t, "exec-factice"),
		"recep-factice": simule(t, "recep-factice"),
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, reg)
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	return orch, ref
}

func plafondsDeTest() budget.Plafonds {
	return budget.Plafonds{
		Appels: map[string]int{"exec-factice": 20, "recep-factice": 20},
		Jetons: map[string]int{"exec-factice": 200000, "recep-factice": 200000},
	}
}

// facticeServiceDirecteur : directeur factice pour le test du SERVICE
// (même contrat que le factice du package, indépendant).
type facticeServiceDirecteur struct {
	srv      *httptest.Server
	reponses []string
}

func nouveauFacticeServiceDirecteur(t *testing.T) *facticeServiceDirecteur {
	t.Helper()
	f := &facticeServiceDirecteur{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(f.reponses) == 0 {
			http.Error(w, "file vide", http.StatusInternalServerError)
			return
		}
		suivante := f.reponses[0]
		f.reponses = f.reponses[1:]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": req.Model,
			"choices": []map[string]any{
				{"message": map[string]string{"content": suivante}},
			},
			"usage": map[string]int{"total_tokens": 500},
		})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *facticeServiceDirecteur) ajouter(reponses ...string) {
	f.reponses = append(f.reponses, reponses...)
}

func (f *facticeServiceDirecteur) adaptateur(t *testing.T) adaptateurs.Adaptateur {
	t.Helper()
	cfg := connecteurs.Config{
		Mode: connecteurs.ModeSimule,
		Fournisseurs: []connecteurs.ConfigFournisseur{{
			ID: "directeur-factice", Endpoint: f.srv.URL, Modele: "gpt-factice",
			TimeoutMS: 5000, TailleMaxReponse: 1 << 20, RetraitsMax: 0,
		}},
	}
	ep, err := cfg.NouveauHTTPEndPoint("directeur-factice")
	if err != nil {
		t.Fatalf("endpoint : %v", err)
	}
	return ep
}

// ---- banc HTTP : mux réel, token auth, codes HTTP stables ----

func appel(t *testing.T, s *service, methode, chemin string, corps any) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	brancherRoutes(mux, s.store, s.token)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	var corpsReader io.Reader
	if corps != nil {
		b, err := json.Marshal(corps)
		if err != nil {
			t.Fatalf("marshal : %v", err)
		}
		corpsReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(methode, srv.URL+chemin, corpsReader)
	if err != nil {
		t.Fatalf("requête : %v", err)
	}
	req.Header.Set(enTeteToken, s.token)
	rep, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer rep.Body.Close()
	raw, _ := io.ReadAll(rep.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return rep.StatusCode, m
}

// TestServiceAuthRefusee : sans token, tout est refusé (401).
func TestServiceAuthRefusee(t *testing.T) {
	s, _ := serviceTest(t)
	mux := http.NewServeMux()
	brancherRoutes(mux, s.store, s.token)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	rep, err := http.Get(srv.URL + "/projets")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("401 attendu, reçu %d", rep.StatusCode)
	}
}

// TestServiceChaineComplete : la chaîne complète PAR LE SERVICE REST :
// codes HTTP stables à chaque étape, 409 sur double publication.
func TestServiceChaineComplete(t *testing.T) {
	s, f := serviceTest(t)
	f.ajouter(
		`{"action":"QUESTION","contenu":"Quel livrable ?"}`,
		`{"action":"PROPOSITION_ORDRE","ordre":{`+
			`"identification":{"mission":"M-TEMOIN-01","version":"v1","protocole":"COORD v1.5"},`+
			`"etat_des_lieux":"Coord v1.5 opérationnelle.",`+
			`"sources":[{"ref":"issue dbouzon/osnias-clearing#42 version 1.4"}],`+
			`"objectif":"Qualifier la mission témoin de recette maïeutique",`+
			`"etapes":[{"nom":"T1","controle_intermediaire":"inventaire vérifié"},{"nom":"T2","controle_intermediaire":""}],`+
			`"limites":["0 EUR"],`+
			`"liberte_execution":"Libre.",`+
			`"livrable":"Rapport de recette maïeutique complet et signé",`+
			`"criteres_reception":["scénarios vérifiés"],`+
			`"application_coord":"Protocole COORD v1.5."}}`,
		`{"action":"VERDICT","verdict":"PLAN_ACCEPTE","points_detail":[{"severite":"RESERVE_NON_BLOQUANTE","texte":"ok"}]}`,
	)

	code, m := appel(t, s, "POST", "/projets", map[string]string{
		"id": "temoin", "intention": "Je veux que COORD m'aide à formuler mes missions"})
	if code != http.StatusCreated {
		t.Fatalf("création : %d %+v", code, m)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/sources", map[string]any{
		"sources": []string{"issue dbouzon/osnias-clearing#42 version 1.4"}})
	if code != http.StatusOK {
		t.Fatalf("sources : %d", code)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/dialogue", map[string]string{
		"directeur": "directeur-factice", "executant": "exec-factice"})
	if code != http.StatusOK {
		t.Fatalf("dialogue : %d", code)
	}
	// Tour 1 : question (intention ambiguë).
	code, _ = appel(t, s, "POST", "/projets/temoin/tour", map[string]string{"entree": ""})
	if code != http.StatusOK {
		t.Fatalf("tour 1 : %d", code)
	}
	// Tour 2 : proposition d'ordre + critique sémantique (second appel DU
	// MÊME tour) → ORDRE_PROPOSE.
	code, m = appel(t, s, "POST", "/projets/temoin/tour", map[string]string{"entree": ""})
	if code != http.StatusOK {
		t.Fatalf("tour 2 : %d %+v", code, m)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/qualifier", nil)
	if code != http.StatusOK {
		t.Fatalf("qualifier : %d", code)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/valider", map[string]string{"jeton_action": "console"})
	if code != http.StatusOK {
		t.Fatalf("valider : %d", code)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/publier", nil)
	if code != http.StatusOK {
		t.Fatalf("publier : %d", code)
	}
	// Double publication : 409 OPERATION_DEJA_EFFECTUEE.
	code, m = appel(t, s, "POST", "/projets/temoin/publier", nil)
	if code != http.StatusConflict || m["code"] != maieutique.CodeOperationDejaFait {
		t.Fatalf("double publication : %d %+v", code, m)
	}
	code, _ = appel(t, s, "POST", "/projets/temoin/transmettre", nil)
	if code != http.StatusOK {
		t.Fatalf("transmettre : %d", code)
	}
	code, m = appel(t, s, "POST", "/projets/temoin/transmettre", nil)
	if code != http.StatusConflict || m["code"] != maieutique.CodeOperationDejaFait {
		t.Fatalf("double transmission : %d %+v", code, m)
	}
	// Projet inconnu : 404.
	code, m = appel(t, s, "GET", "/projets/inconnu", nil)
	if code != http.StatusNotFound || m["code"] != maieutique.CodeProjetInconnu {
		t.Fatalf("inconnu : %d %+v", code, m)
	}
}

// TestServiceMissionFille (M-COORD-FRONT-01, commit « adaptations
// serveur ») : la route additive GET /projets/{id}/fille est en lecture
// seule — 404 AVANT transmission (aucune fuite), 200 avec l'état de la
// mission fille témoin APRÈS transmission.
func TestServiceMissionFille(t *testing.T) {
	s, f := serviceTest(t)
	f.ajouter(
		`{"action":"QUESTION","contenu":"Quel livrable ?"}`,
		`{"action":"PROPOSITION_ORDRE","ordre":{`+
			`"identification":{"mission":"M-TEMOIN-01","version":"v1","protocole":"COORD v1.5"},`+
			`"etat_des_lieux":"Coord v1.5 opérationnelle.",`+
			`"sources":[{"ref":"issue dbouzon/osnias-clearing#42 version 1.4"}],`+
			`"objectif":"Qualifier la mission témoin de recette maïeutique",`+
			`"etapes":[{"nom":"T1","controle_intermediaire":"inventaire vérifié"},{"nom":"T2","controle_intermediaire":""}],`+
			`"limites":["0 EUR"],`+
			`"liberte_execution":"Libre.",`+
			`"livrable":"Rapport de recette maïeutique complet et signé",`+
			`"criteres_reception":["scénarios vérifiés"],`+
			`"application_coord":"Protocole COORD v1.5."}}`,
		`{"action":"VERDICT","verdict":"PLAN_ACCEPTE","points_detail":[{"severite":"RESERVE_NON_BLOQUANTE","texte":"ok"}]}`,
	)

	code, _ := appel(t, s, "POST", "/projets", map[string]string{
		"id": "fille-temoin", "intention": "Vérifier la route mission fille"})
	if code != http.StatusCreated {
		t.Fatalf("création : %d", code)
	}
	code, mSources := appel(t, s, "POST", "/projets/fille-temoin/sources", map[string]any{
		"sources": []string{"issue dbouzon/osnias-clearing#42 version 1.4"}})
	if code != http.StatusOK {
		t.Fatalf("sources : %d %+v", code, mSources)
	}
	code, _ = appel(t, s, "POST", "/projets/fille-temoin/dialogue", map[string]string{
		"directeur": "directeur-factice", "executant": "exec-factice"})
	if code != http.StatusOK {
		t.Fatalf("dialogue : %d", code)
	}
	code, _ = appel(t, s, "POST", "/projets/fille-temoin/tour", map[string]string{"entree": ""})
	if code != http.StatusOK {
		t.Fatalf("tour question : %d", code)
	}
	code, _ = appel(t, s, "POST", "/projets/fille-temoin/tour", map[string]string{"entree": ""})
	if code != http.StatusOK {
		t.Fatalf("tour proposition : %d", code)
	}
	// AVANT transmission : 404, aucune information sur la mission fille.
	code, m := appel(t, s, "GET", "/projets/fille-temoin/fille", nil)
	if code != http.StatusNotFound || m["code"] != maieutique.CodeProjetInconnu {
		t.Fatalf("fille avant transmission : attendu 404 PROJET_INCONNU, reçu %d %+v", code, m)
	}
	for _, etape := range []struct {
		chemin string
		corps  any
	}{
		{"/projets/fille-temoin/qualifier", nil},
		{"/projets/fille-temoin/valider", map[string]string{"jeton_action": "console"}},
		{"/projets/fille-temoin/publier", nil},
		{"/projets/fille-temoin/transmettre", nil},
	} {
		code, m = appel(t, s, "POST", etape.chemin, etape.corps)
		if code != http.StatusOK {
			t.Fatalf("%s : %d %+v", etape.chemin, code, m)
		}
	}
	// APRÈS transmission : 200 avec l'état de la mission fille témoin.
	code, m = appel(t, s, "GET", "/projets/fille-temoin/fille", nil)
	if code != http.StatusOK {
		t.Fatalf("fille après transmission : %d %+v", code, m)
	}
	if m["issue"] != "MAIEUTIQUE-TEMOIN-01" {
		t.Fatalf("issue fille inattendue : %+v", m["issue"])
	}
	if m["executant"] != "openai-exec" || m["empreinte_ordre"] == "" {
		t.Fatalf("réponse fille incomplète : %+v", m)
	}
}

// TestServiceOrdreNonConforme422 : un ordre multi-objectifs proposé par le
// pilote rend 422 OBJECTIF_NON_UNIQUE (code stable).
func TestServiceOrdreNonConforme422(t *testing.T) {
	s, f := serviceTest(t)
	f.ajouter(`{"action":"QUESTION","contenu":"q"}`)
	appel(t, s, "POST", "/projets", map[string]string{"id": "x", "intention": "intention"})
	appel(t, s, "POST", "/projets/x/sources", map[string]any{
		"sources": []string{"issue dbouzon/osnias-clearing#42 version 1.4"}})
	appel(t, s, "POST", "/projets/x/dialogue", map[string]string{
		"directeur": "directeur-factice", "executant": "exec-factice"})
	f.ajouter(`{"action":"VERDICT","verdict":"PLAN_ACCEPTE","points_detail":[{"severite":"RESERVE_NON_BLOQUANTE","texte":"ok"}]}`)
	code, m := appel(t, s, "POST", "/projets/x/ordre", map[string]any{
		"identification":     map[string]string{"mission": "M", "version": "v1", "protocole": "P"},
		"etat_des_lieux":     "lieux",
		"sources":            []map[string]string{{"ref": "issue dbouzon/osnias-clearing#42 version 1.4"}},
		"objectif":           "Faire la maïeutique et publier le rapport",
		"etapes":             []map[string]string{{"nom": "T1", "controle_intermediaire": "ctrl"}},
		"limites":            []string{"0 EUR"},
		"liberte_execution":  "libre",
		"livrable":           "Rapport de recette maïeutique complet et signé",
		"criteres_reception": []string{"ok"},
		"application_coord":  "COORD v1.5",
	})
	if code != http.StatusUnprocessableEntity || m["code"] != maieutique.CodeObjectifNonUnique {
		t.Fatalf("422 OBJECTIF_NON_UNIQUE attendu : %d %+v", code, m)
	}
}

// TestStatutDe : le mapping code → HTTP est STABLE et exhaustif pour les
// codes documentés.
func TestStatutDe(t *testing.T) {
	cas := map[string]int{
		maieutique.CodeProjetInconnu:     http.StatusNotFound,
		maieutique.CodeOperationDejaFait: http.StatusConflict,
		maieutique.CodeActionInterdit:    http.StatusConflict,
		maieutique.CodeOrdreNonConforme:  http.StatusUnprocessableEntity,
		maieutique.CodeObjectifNonUnique: http.StatusUnprocessableEntity,
		maieutique.CodeBinomeIdentique:   http.StatusBadRequest,
		maieutique.CodeJetonPiloteAbsent: http.StatusBadRequest,
		maieutique.CodeReponseInvalide:   http.StatusBadGateway,
		maieutique.CodeIndisponibilite:   http.StatusBadGateway,
		"CODE_INCONNU":                   http.StatusInternalServerError,
	}
	for code, statut := range cas {
		if statutDe(code) != statut {
			t.Fatalf("statutDe(%s) = %d, attendu %d", code, statutDe(code), statut)
		}
	}
}

// TestMultiflag : le flag -fournisseur est répétable.
func TestMultiflag(t *testing.T) {
	var m multiflag
	for _, v := range []string{"a=http://127.0.0.1:1", "b=http://127.0.0.1:2"} {
		if err := m.Set(v); err != nil {
			t.Fatalf("set : %v", err)
		}
	}
	if len(m) != 2 || !strings.Contains(m.String(), "a=") {
		t.Fatalf("multiflag : %+v", m)
	}
}

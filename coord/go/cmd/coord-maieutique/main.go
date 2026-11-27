// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-maieutique est le service HTTP local du dialogue maïeutique de
// COORD (mission M-COORD-MAIEUTIQUE-01, plan v2 D1/D2) : interface REST
// JSON documentée (base du futur frontend), pattern du service Pilot —
// écoute LOOPBACK UNIQUEMENT, token d'authentification locale généré à
// chaque démarrage et affiché sur la console (jamais journalisé), refus
// de toute adresse non loopback (fail closed).
//
// Aucune clé API : les fournisseurs déclarés par -fournisseur id=endpoint
// sont des endpoints loopback (pont factice coord.llm.v1 en recette,
// 0 €). Deux fournisseurs DISTINCTS au minimum sont exigés (directeur et
// exécutant, N-05).
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/maieutique"
	"coordengine/internal/missions"
)

const enTeteToken = "X-Coord-Pilot-Token"

// transmissionTemoin : paramètres de la mission fille témoin (plan v2 §5,
// D7). Les RÔLES portent des ACTIONS normatives (spec 2), jamais des
// identités : la mission fille doit être exécutable (executer, soumettre,
// acker), recevable (recevoir) et clôturable par le pilote (cloturer).
func transmissionTemoin() maieutique.Transmission {
	return maieutique.Transmission{
		IssueFille:  "MAIEUTIQUE-TEMOIN-01",
		ExecutantID: "openai-exec",
		RecepteurID: "openai-recep",
		QuaExecID:   "openai-exec",
		QuaRecepID:  "openai-recep",
		Roles: map[string][]string{
			"EXECUTANT":      {"executer", "soumettre", "acker"},
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		Plafonds: budget.Plafonds{
			Appels: map[string]int{"openai-exec": 10, "openai-recep": 10},
			Jetons: map[string]int{"openai-exec": 100000, "openai-recep": 100000},
		},
		LienIssueMere: "dbouzon/osnias-clearing#42",
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "adresse d'écoute (loopback uniquement, fail closed)")
	racine := flag.String("data", "coord-maieutique-data", "racine des projets maïeutiques (state/maieutique)")
	missionsDir := flag.String("missions", "", "référentiel missions du moteur (mission fille) ; défaut : <data>/missions")
	fournisseurs := multiflag{}
	flag.Var(&fournisseurs, "fournisseur", "fournisseur déclaré id=endpoint (loopback, répétable, 2 minimum)")
	sondes := flag.Bool("sondes", false, "activer les sondes réseau des sources (défaut : désactivé, SIMULÉ)")
	flag.Parse()

	if len(fournisseurs) < 2 {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : au moins deux fournisseurs DISTINCTS requis (directeur et exécutant, -fournisseur id=endpoint)")
		os.Exit(3)
	}
	if *missionsDir == "" {
		*missionsDir = filepath.Join(*racine, "missions")
	}

	registre := map[string]adaptateurs.Adaptateur{}
	var cfgFournisseurs []connecteurs.ConfigFournisseur
	for _, f := range fournisseurs {
		id, endpoint, ok := strings.Cut(f, "=")
		id = strings.TrimSpace(id)
		endpoint = strings.TrimSpace(endpoint)
		if !ok || id == "" || endpoint == "" {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : -fournisseur attend id=endpoint, reçu : "+f)
			os.Exit(3)
		}
		if _, deja := registre[id]; deja {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : fournisseur dupliqué : "+id)
			os.Exit(3)
		}
		cfgFournisseurs = append(cfgFournisseurs, connecteurs.ConfigFournisseur{
			ID: id, Endpoint: endpoint, Modele: "modele-" + id,
			TimeoutMS: 30000, TailleMaxReponse: 4 << 20, RetraitsMax: 0,
		})
	}
	cfgConn := connecteurs.Config{Mode: connecteurs.ModeSimule, Fournisseurs: cfgFournisseurs}
	for _, f := range cfgFournisseurs {
		ep, err := cfgConn.NouveauHTTPEndPoint(f.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "NON_EXECUTE : endpoint impossible pour "+f.ID+" : "+err.Error())
			os.Exit(3)
		}
		registre[f.ID] = ep
	}

	ref, err := missions.Ouvrir(*missionsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : référentiel missions impossible : "+err.Error())
		os.Exit(3)
	}
	orch, err := connecteurs.NouvelOrchestrateur(ref, registre)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : orchestrateur impossible : "+err.Error())
		os.Exit(3)
	}

	cfg := maieutique.ConfigParDefaut(*racine)
	cfg.Adaptateurs = registre
	cfg.Referentiel = ref
	cfg.Orchestrateur = orch
	cfg.SondesReseau = *sondes
	cfg.Transmission = transmissionTemoin()
	store, err := maieutique.OuvrirStore(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : store maïeutique impossible : "+err.Error())
		os.Exit(3)
	}

	listen, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : écoute impossible : "+err.Error())
		os.Exit(3)
	}
	host, _, err := net.SplitHostPort(listen.Addr().String())
	if err != nil || host != "127.0.0.1" && host != "localhost" && host != "::1" {
		_ = listen.Close()
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : écoute refusée : "+listen.Addr().String()+" n'est pas loopback (fail closed)")
		os.Exit(3)
	}

	token := genererToken()
	mux := http.NewServeMux()
	brancherRoutes(mux, store, token)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	fmt.Println("coord-maieutique (M-COORD-MAIEUTIQUE-01) : écoute loopback " + listen.Addr().String())
	fmt.Println("Token local : " + token)
	fmt.Println("Ce token n'est pas journalisé ; il est régénéré à chaque démarrage.")
	if err := srv.Serve(listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "ERREUR : "+err.Error())
		os.Exit(1)
	}
}

func genererToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, "NON_EXECUTE : aléa indisponible")
		os.Exit(3)
	}
	return hex.EncodeToString(b)
}

// ---- routes REST (interface du futur frontend, plan v2 D1) ----

func brancherRoutes(mux *http.ServeMux, store *maieutique.Store, token string) {
	s := &service{store: store, token: token}
	mux.HandleFunc("GET /sante", s.gerer(s.sante))
	mux.HandleFunc("GET /projets", s.gerer(s.listerProjets))
	mux.HandleFunc("POST /projets", s.gerer(s.declarerIntention))
	mux.HandleFunc("GET /projets/{id}", s.gerer(s.obtenirProjet))
	mux.HandleFunc("GET /projets/{id}/journal", s.gerer(s.journal))
	mux.HandleFunc("POST /projets/{id}/dialogue", s.gerer(s.ouvrirDialogue))
	mux.HandleFunc("POST /projets/{id}/tour", s.gerer(s.tourDirecteur))
	mux.HandleFunc("POST /projets/{id}/ordre", s.gerer(s.proposerOrdre))
	mux.HandleFunc("POST /projets/{id}/qualifier", s.gerer(s.qualifier))
	mux.HandleFunc("POST /projets/{id}/valider", s.gerer(s.validerPilote))
	mux.HandleFunc("POST /projets/{id}/publier", s.gerer(s.publier))
	mux.HandleFunc("POST /projets/{id}/transmettre", s.gerer(s.transmettre))
	mux.HandleFunc("POST /projets/{id}/relancer", s.gerer(s.relancer))
	mux.HandleFunc("POST /projets/{id}/rectifier", s.gerer(s.rectifier))
	mux.HandleFunc("POST /projets/{id}/abandonner", s.gerer(s.abandonner))
	mux.HandleFunc("POST /projets/{id}/invalider", s.gerer(s.invalider))
	mux.HandleFunc("POST /projets/{id}/retour-dialogue", s.gerer(s.retourDialogue))
	mux.HandleFunc("POST /projets/{id}/cadrage", s.gerer(s.fixerCadrage))
	mux.HandleFunc("POST /projets/{id}/devis", s.gerer(s.fixerDevis))
	mux.HandleFunc("POST /projets/{id}/accord-devis", s.gerer(s.accorderDevis))
	mux.HandleFunc("POST /projets/{id}/documents", s.gerer(s.deposerDocument))
	mux.HandleFunc("GET /projets/{id}/documents", s.gerer(s.listerDocuments))
	mux.HandleFunc("POST /projets/{id}/recherche", s.gerer(s.rechercher))
	mux.HandleFunc("POST /projets/{id}/sources", s.gerer(s.declarerSources))
	mux.HandleFunc("POST /projets/{id}/urls-acceptees", s.gerer(s.accepterURL))
	mux.HandleFunc("GET /projets/{id}/ordre", s.gerer(s.obtenirOrdre))
}

type service struct {
	store *maieutique.Store
	token string
}

// statutDe : mapping STABLE des codes maïeutique vers HTTP.
func statutDe(code string) int {
	switch code {
	case maieutique.CodeProjetInconnu:
		return http.StatusNotFound
	case maieutique.CodeActionInterdit, maieutique.CodeOperationDejaFait,
		maieutique.CodeProjetExistant, maieutique.CodeProjetTerminal:
		return http.StatusConflict
	case maieutique.CodeOrdreNonConforme, maieutique.CodeOrdreAbsent,
		maieutique.CodeObjectifNonUnique,
		maieutique.CodeSourcesVides, maieutique.CodeSourceNonDeclaree,
		maieutique.CodeSourcesNonVerifiees, maieutique.CodePasDeControle,
		maieutique.CodeLivrableImprecis, maieutique.CodeEtapesIncoherentes,
		maieutique.CodeCritiqueBloquante, maieutique.CodeEmpreinteDivergente:
		return http.StatusUnprocessableEntity
	case maieutique.CodeIntentionVide, maieutique.CodeBinomeIdentique,
		maieutique.CodeDirecteurInconnu, maieutique.CodeExecutantInconnu,
		maieutique.CodeJetonPiloteAbsent, maieutique.CodeDocumentInvalide:
		return http.StatusBadRequest
	case maieutique.CodeReponseInvalide, maieutique.CodeIndisponibilite:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func (s *service) auth(r *http.Request) bool {
	return r.Header.Get(enTeteToken) == s.token
}

// gerer : enveloppe commune : auth locale, décodage borné, erreurs
// structurées {code, motif} avec le statut HTTP STABLE correspondant.
func (s *service) gerer(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(r) {
			repondre(w, http.StatusUnauthorized, map[string]any{
				"code": "AUTH_REFUSEE", "motif": "token d'authentification locale absent ou invalide"})
			return
		}
		if err := fn(w, r); err != nil {
			var e maieutique.Erreur
			if errors.As(err, &e) {
				repondre(w, statutDe(e.Code), map[string]any{"code": e.Code, "motif": e.Motif})
				return
			}
			repondre(w, http.StatusInternalServerError, map[string]any{
				"code": "INTERNE", "motif": err.Error()})
		}
	}
}

func repondre(w http.ResponseWriter, statut int, corps any) {
	b, err := json.Marshal(corps)
	if err != nil {
		http.Error(w, "réponse interne impossible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statut)
	_, _ = w.Write(b)
}

func lireCorps(r *http.Request, v any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("corps de requête vide")
	}
	return json.Unmarshal(raw, v)
}

func (s *service) sante(w http.ResponseWriter, r *http.Request) error {
	repondre(w, http.StatusOK, map[string]any{"etat": "OK", "module": "coord-maieutique"})
	return nil
}

func (s *service) listerProjets(w http.ResponseWriter, r *http.Request) error {
	ids, err := s.store.Lister()
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, map[string]any{"projets": ids})
	return nil
}

func (s *service) declarerIntention(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		ID        string `json:"id"`
		Intention string `json:"intention"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.DeclarerIntention(req.ID, req.Intention)
	if err != nil {
		return err
	}
	repondre(w, http.StatusCreated, p)
	return nil
}

func (s *service) obtenirProjet(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Obtenir(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) journal(w http.ResponseWriter, r *http.Request) error {
	raw, err := s.store.Journal(r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = w.Write(raw)
	return nil
}

func (s *service) ouvrirDialogue(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Directeur string `json:"directeur"`
		Executant string `json:"executant"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.OuvrirDialogue(r.PathValue("id"), req.Directeur, req.Executant)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) tourDirecteur(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Entree string `json:"entree"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, rep, err := s.store.TourDirecteur(r.Context(), r.PathValue("id"), req.Entree)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, map[string]any{"projet": p, "reponse": rep})
	return nil
}

func (s *service) proposerOrdre(w http.ResponseWriter, r *http.Request) error {
	var req json.RawMessage
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.ProposerOrdre(r.Context(), r.PathValue("id"), req)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) qualifier(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Qualifier(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) validerPilote(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		JetonAction string `json:"jeton_action"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.ValiderPilote(r.PathValue("id"), req.JetonAction)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) publier(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Publier(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) transmettre(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Transmettre(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) relancer(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Relancer(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) rectifier(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Intention string `json:"intention"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.Rectifier(r.PathValue("id"), req.Intention)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) abandonner(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Abandonner(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) invalider(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Invalider(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) retourDialogue(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Motif string `json:"motif"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.RetourDialogue(r.PathValue("id"), req.Motif)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) fixerCadrage(w http.ResponseWriter, r *http.Request) error {
	var req map[string]any
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.FixerCadrage(r.PathValue("id"), req)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) fixerDevis(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.FixerDevis(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) accorderDevis(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Decision string `json:"decision"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.AccorderDevis(r.PathValue("id"), req.Decision)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) deposerDocument(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Nom     string `json:"nom"`
		Contenu string `json:"contenu"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.Obtenir(r.PathValue("id"))
	if err != nil {
		return err
	}
	emp, err := s.store.DeposerDocument(p, req.Nom, req.Contenu)
	if err != nil {
		return err
	}
	repondre(w, http.StatusCreated, map[string]any{"nom": req.Nom, "empreinte": emp})
	return nil
}

func (s *service) listerDocuments(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Obtenir(r.PathValue("id"))
	if err != nil {
		return err
	}
	noms, err := s.store.ListerDocuments(p)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, map[string]any{"documents": noms})
	return nil
}

func (s *service) rechercher(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Requete string `json:"requete"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.Obtenir(r.PathValue("id"))
	if err != nil {
		return err
	}
	idx, err := s.store.NouvelIndex(p)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, map[string]any{"resultats": idx.Rechercher(req.Requete)})
	return nil
}

func (s *service) declarerSources(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Sources []string `json:"sources"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.DeclarerSources(r.PathValue("id"), req.Sources)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) accepterURL(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		URL string `json:"url"`
	}
	if err := lireCorps(r, &req); err != nil {
		return err
	}
	p, err := s.store.AccepterURL(r.PathValue("id"), req.URL)
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p)
	return nil
}

func (s *service) obtenirOrdre(w http.ResponseWriter, r *http.Request) error {
	p, err := s.store.Obtenir(r.PathValue("id"))
	if err != nil {
		return err
	}
	repondre(w, http.StatusOK, p.Ordre)
	return nil
}

// multiflag : flag répété (liste de valeurs).
type multiflag []string

func (m *multiflag) String() string { return strings.Join(*m, ", ") }
func (m *multiflag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

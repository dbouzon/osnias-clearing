// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Moteur factice DÉMO (plan v2 §1.1) : mêmes états, mêmes codes d'erreur,
// mêmes formes JSON que le moteur réel (internal/maieutique), pour
// dérouler le parcours complet sans backend ni appel externe, 0 €.
// Le directeur factice suit un script déterministe : QUESTION, puis
// CLARIFICATION, puis PROPOSITION_ORDRE.
//
// Ce moteur vit dans le gateway : le paquet internal/maieutique reste
// INTACT (D1). Toute évolution du moteur réel se reflétera ici par une
// micro-mission dédiée.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	etatIntention     = "INTENTION"
	etatDialogue      = "DIALOGUE"
	etatOrdrePropose  = "ORDRE_PROPOSE"
	etatOrdreQualifie = "ORDRE_QUALIFIE"
	etatValide        = "VALIDE"
	etatPublie        = "PUBLIE"
	etatTransmis      = "TRANSMIS"
	etatAbandonne     = "ABANDONNE"
)

type projetDemo struct {
	ID               string          `json:"id"`
	Etat             string          `json:"etat"`
	Intention        string          `json:"intention"`
	DirecteurID      string          `json:"directeur_id"`
	ExecutantID      string          `json:"executant_id"`
	Version          int             `json:"version"`
	EmpreinteOrdre   string          `json:"empreinte_ordre"`
	Ordre            map[string]any  `json:"ordre,omitempty"`
	SourcesDeclarees []string        `json:"sources_declarees"`
	UrlsAcceptees    map[string]bool `json:"urls_acceptees"`
	Cadrage          map[string]any  `json:"cadrage,omitempty"`
	Devis            map[string]any  `json:"devis,omitempty"`
	AccordDevis      bool            `json:"accord_devis"`
	EchecsDirecteur  int             `json:"echecs_directeur"`
	Appels           int             `json:"appels"`
	ContexteOctets   int             `json:"contexte_octets"`
	InstantUTC       string          `json:"instant_utc"`

	tours     int              `json:"-"`
	journal   []map[string]any `json:"-"`
	documents []string         `json:"-"`
	empreinte string           `json:"-"`
}

type moteurDemo struct {
	mu      sync.Mutex
	projets map[string]*projetDemo
}

func nouveauMoteurDemo() *moteurDemo {
	return &moteurDemo{projets: map[string]*projetDemo{}}
}

func maintenantDemo() string { return time.Now().UTC().Format(time.RFC3339) }

func (m *moteurDemo) journaliser(p *projetDemo, entree map[string]any) {
	entree["instant_utc"] = maintenantDemo()
	p.journal = append(p.journal, entree)
}

// ordreFactice : ordre COORD complet (dix champs du schéma, spec §5-T3),
// objectif UNIQUE, étapes avec contrôles intermédiaires.
func ordreFactice(mission, intention string) map[string]any {
	return map[string]any{
		"identification": map[string]any{
			"mission": mission, "version": "", "protocole": "COORD-MAIEUTIQUE-1",
		},
		"etat_des_lieux": "Aucun livrable existant ; cadrage issu du dialogue : " + intention,
		"sources": []map[string]any{
			{"identifiant": "doc-entree-1", "nature": "document fourni", "controle": "lecture"},
		},
		"objectif":           "Produire le livrable demandé : " + intention,
		"etapes":             []map[string]any{{"nom": "cadrage final", "controle_intermediaire": "revue du plan"}, {"nom": "exécution", "controle_intermediaire": "contrôle qualité"}, {"nom": "synthèse", "controle_intermediaire": "relecture croisée"}},
		"limites":            []string{"aucune modification du corpus documentaire après validation"},
		"liberte_execution":  "liberté de méthode dans le respect des limites",
		"livrable":           mission + "/livrable.md",
		"criteres_reception": []string{"livrable lisible", "empreinte vérifiable"},
		"application_coord":  "coord-maieutique (factice)",
	}
}

// empreinteOrdreFactice : sha256 de la forme canonique compacte triée,
// parité avec p.Ordre.Empreinte() du moteur réel pour la recette.
func empreinteOrdreFactice(ordre map[string]any) string {
	b, _ := json.Marshal(ordre)
	s := strings.ReplaceAll(string(b), ", ", ",")
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// servir : répartit les requêtes du parcours (chemins /api/projets…).
func (m *moteurDemo) servir(w http.ResponseWriter, r *http.Request) {
	chemin := strings.TrimPrefix(r.URL.Path, "/api")
	parts := strings.Split(strings.Trim(chemin, "/"), "/")
	// parts[0] == "projets"
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			m.lister(w)
		case http.MethodPost:
			m.declarerIntention(w, r)
		default:
			repondreErreur(w, http.StatusMethodNotAllowed, "METHODE_REFUSEE", r.Method)
		}
		return
	}
	id := parts[1]
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projets[id]
	if !ok {
		repondreErreur(w, http.StatusNotFound, "PROJET_INCONNU", "projet inconnu : "+id)
		return
	}
	action := ""
	if len(parts) >= 3 {
		action = parts[2]
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		repondreJSON(w, http.StatusOK, p)
	case action == "journal" && r.Method == http.MethodGet:
		m.servirJournal(w, p)
	case action == "dialogue" && r.Method == http.MethodPost:
		m.ouvrirDialogue(w, r, p)
	case action == "tour" && r.Method == http.MethodPost:
		m.tourDirecteur(w, r, p)
	case action == "retour-dialogue" && r.Method == http.MethodPost:
		m.retourDialogue(w, r, p)
	case action == "qualifier" && r.Method == http.MethodPost:
		m.qualifier(w, p)
	case action == "devis" && r.Method == http.MethodPost:
		m.fixerDevis(w, p)
	case action == "accord-devis" && r.Method == http.MethodPost:
		m.accorderDevis(w, r, p)
	case action == "valider" && r.Method == http.MethodPost:
		m.validerPilote(w, r, p)
	case action == "publier" && r.Method == http.MethodPost:
		m.publier(w, p)
	case action == "transmettre" && r.Method == http.MethodPost:
		m.transmettre(w, p)
	case action == "cadrage" && r.Method == http.MethodPost:
		m.fixerCadrage(w, r, p)
	case action == "documents" && r.Method == http.MethodPost:
		m.deposerDocument(w, r, p)
	case action == "documents" && r.Method == http.MethodGet:
		repondreJSON(w, http.StatusOK, map[string]any{"documents": p.documents})
	case action == "fille" && r.Method == http.MethodGet:
		m.missionFille(w, p)
	case action == "ordre" && r.Method == http.MethodGet:
		m.obtenirOrdre(w, p)
	default:
		repondreErreur(w, http.StatusNotFound, "ROUTE_INCONNUE", chemin)
	}
}

func (m *moteurDemo) lister(w http.ResponseWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := []string{}
	for id := range m.projets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	repondreJSON(w, http.StatusOK, map[string]any{"projets": ids})
}

func (m *moteurDemo) declarerIntention(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var req struct {
		ID        string `json:"id"`
		Intention string `json:"intention"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		repondreErreur(w, http.StatusBadRequest, "INTENTION_VIDE", "identifiant de projet vide interdit")
		return
	}
	if strings.TrimSpace(req.Intention) == "" {
		repondreErreur(w, http.StatusBadRequest, "INTENTION_VIDE", "intention vide interdite")
		return
	}
	if _, ok := m.projets[req.ID]; ok {
		repondreErreur(w, http.StatusConflict, "PROJET_EXISTANT", "projet déjà existant : "+req.ID)
		return
	}
	p := &projetDemo{
		ID: req.ID, Etat: etatIntention, Intention: req.Intention,
		UrlsAcceptees: map[string]bool{}, SourcesDeclarees: []string{},
		InstantUTC: maintenantDemo(),
	}
	m.journaliser(p, map[string]any{"type": "DECLARATION_INTENTION", "acteur": "pilote", "intention": req.Intention})
	m.projets[req.ID] = p
	repondreJSON(w, http.StatusCreated, p)
}

func (m *moteurDemo) ouvrirDialogue(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		Directeur string `json:"directeur"`
		Executant string `json:"executant"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	req.Directeur = strings.TrimSpace(req.Directeur)
	req.Executant = strings.TrimSpace(req.Executant)
	if req.Directeur == "" || req.Executant == "" {
		repondreErreur(w, http.StatusBadRequest, "BINOME_IDENTIQUE", "binôme directeur/exécutant requis")
		return
	}
	if req.Directeur == req.Executant {
		repondreErreur(w, http.StatusBadRequest, "BINOME_IDENTIQUE",
			"directeur et exécutant doivent être DISTINCTS : "+req.Directeur)
		return
	}
	if p.Etat != etatIntention {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "dialogue requis depuis INTENTION, pas depuis "+p.Etat)
		return
	}
	p.DirecteurID = req.Directeur
	p.ExecutantID = req.Executant
	p.Etat = etatDialogue
	m.journaliser(p, map[string]any{"type": "OUVERTURE_DIALOGUE",
		"directeur": req.Directeur, "executant": req.Executant})
	repondreJSON(w, http.StatusOK, p)
}

// tourDirecteur : script déterministe du directeur factice. Le troisième
// tour PROPOSE un ordre (état ORDRE_PROPOSE), comme le moteur réel via
// evaluerProposition.
func (m *moteurDemo) tourDirecteur(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		Entree string `json:"entree"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if p.Etat != etatDialogue {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "tour directeur requis depuis DIALOGUE, pas depuis "+p.Etat)
		return
	}
	m.journaliser(p, map[string]any{"type": "TOUR_CLIENT", "acteur": "pilote", "entree": req.Entree})
	p.Appels++
	p.ContexteOctets += len(req.Entree)
	p.tours++
	var rep map[string]any
	switch p.tours {
	case 1:
		rep = map[string]any{"action": "QUESTION",
			"contenu": "Quelle est la finalité précise attendue, et pour quand ?"}
	case 2:
		rep = map[string]any{"action": "CLARIFICATION",
			"contenu": "Précisez le périmètre : quel livrable concret suffira ?"}
	default:
		p.Ordre = ordreFactice(p.ID, p.Intention)
		p.Etat = etatOrdrePropose
		rep = map[string]any{"action": "PROPOSITION_ORDRE", "ordre": p.Ordre}
		m.journaliser(p, map[string]any{"type": "PROPOSITION_ORDRE", "acteur": p.DirecteurID})
		repondreJSON(w, http.StatusOK, map[string]any{"projet": p, "reponse": rep})
		return
	}
	m.journaliser(p, map[string]any{"type": "TOUR_DIRECTEUR",
		"acteur": p.DirecteurID, "reponse": rep["action"], "contenu": rep["contenu"]})
	repondreJSON(w, http.StatusOK, map[string]any{"projet": p, "reponse": rep})
}

func (m *moteurDemo) retourDialogue(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		Motif string `json:"motif"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if p.Etat != etatOrdrePropose && p.Etat != etatOrdreQualifie {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "retour au dialogue requis depuis ORDRE_PROPOSE ou ORDRE_QUALIFIE, pas depuis "+p.Etat)
		return
	}
	p.Etat = etatDialogue
	p.tours = 0 // le script du directeur repart depuis la reprise du dialogue
	m.journaliser(p, map[string]any{"type": "RETOUR_DIALOGUE", "acteur": "pilote", "motif": req.Motif})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) qualifier(w http.ResponseWriter, p *projetDemo) {
	if p.Ordre == nil {
		repondreErreur(w, http.StatusUnprocessableEntity, "ORDRE_ABSENT", "aucun ordre proposé à qualifier")
		return
	}
	if p.Etat != etatOrdrePropose {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "qualifier requis depuis ORDRE_PROPOSE, pas depuis "+p.Etat)
		return
	}
	p.Version++
	idn := p.Ordre["identification"].(map[string]any)
	idn["version"] = fmt.Sprintf("v%d", p.Version)
	p.EmpreinteOrdre = empreinteOrdreFactice(p.Ordre)
	p.Etat = etatOrdreQualifie
	m.journaliser(p, map[string]any{"type": "QUALIFICATION",
		"version": idn["version"], "empreinte": p.EmpreinteOrdre})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) fixerDevis(w http.ResponseWriter, p *projetDemo) {
	if p.Ordre == nil {
		repondreErreur(w, http.StatusUnprocessableEntity, "ORDRE_ABSENT", "aucun ordre : le devis dérive de l'ordre (D8)")
		return
	}
	etapes := []string{}
	for _, e := range p.Ordre["etapes"].([]map[string]any) {
		etapes = append(etapes, e["nom"].(string))
	}
	p.Devis = map[string]any{
		"etapes": etapes, "livrable": p.Ordre["livrable"], "paiement_reel": false,
	}
	m.journaliser(p, map[string]any{"type": "DEVIS", "derive_ordre": true, "paiement_reel": false})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) accorderDevis(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		Decision string `json:"decision"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if p.Devis == nil {
		repondreErreur(w, http.StatusUnprocessableEntity, "ORDRE_ABSENT", "aucun devis établi : fixer le devis d'abord (D8)")
		return
	}
	if strings.TrimSpace(req.Decision) == "" {
		repondreErreur(w, http.StatusBadRequest, "JETON_PILOTE_ABSENT", "décision explicite requise (accord ou refus)")
		return
	}
	p.AccordDevis = strings.EqualFold(strings.TrimSpace(req.Decision), "ACCORD")
	m.journaliser(p, map[string]any{"type": "ACCORD_DEVIS", "acteur": "pilote",
		"decision": strings.ToUpper(strings.TrimSpace(req.Decision)), "paiement_reel": false})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) validerPilote(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		JetonAction string `json:"jeton_action"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if strings.TrimSpace(req.JetonAction) == "" {
		repondreErreur(w, http.StatusBadRequest, "JETON_PILOTE_ABSENT",
			"jeton d'action pilote requis : la validation est un acte explicite")
		return
	}
	if p.EmpreinteOrdre == "" {
		repondreErreur(w, http.StatusUnprocessableEntity, "ORDRE_ABSENT", "aucun ordre qualifié à valider")
		return
	}
	if p.Etat != etatOrdreQualifie {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "validation requise depuis ORDRE_QUALIFIE, pas depuis "+p.Etat)
		return
	}
	p.Etat = etatValide
	m.journaliser(p, map[string]any{"type": "VALIDATION", "acteur": "pilote", "empreinte": p.EmpreinteOrdre})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) publier(w http.ResponseWriter, p *projetDemo) {
	if p.Etat != etatValide {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "publication requise depuis VALIDE, pas depuis "+p.Etat)
		return
	}
	p.Etat = etatPublie
	m.journaliser(p, map[string]any{"type": "PUBLICATION", "acteur": "pilote"})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) transmettre(w http.ResponseWriter, p *projetDemo) {
	if p.Etat == etatTransmis {
		repondreErreur(w, http.StatusConflict, "OPERATION_DEJA_EFFECTUEE", "transmission déjà effectuée")
		return
	}
	if p.Etat != etatPublie {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "transmission requise depuis PUBLIE, pas depuis "+p.Etat)
		return
	}
	p.Etat = etatTransmis
	m.journaliser(p, map[string]any{"type": "TRANSMISSION", "acteur": "pilote",
		"executant": p.ExecutantID, "issue_fille": p.ID + "-EXEC"})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) fixerCadrage(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req map[string]any
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if p.Etat == etatAbandonne || p.Etat == etatTransmis {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "cadrage impossible depuis "+p.Etat)
		return
	}
	p.Cadrage = req
	m.journaliser(p, map[string]any{"type": "CADRAGE", "acteur": "pilote", "gratuit": true})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) deposerDocument(w http.ResponseWriter, r *http.Request, p *projetDemo) {
	var req struct {
		Nom     string `json:"nom"`
		Contenu string `json:"contenu"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	nom := strings.TrimSpace(req.Nom)
	if nom == "" || strings.Contains(nom, "/") || strings.Contains(nom, "\\") ||
		nom == "." || nom == ".." {
		repondreErreur(w, http.StatusBadRequest, "DOCUMENT_INVALIDE", "nom de document interdit : "+nom)
		return
	}
	if p.Etat == etatTransmis {
		repondreErreur(w, http.StatusConflict, "ACTION_INTERDITE", "dépôt impossible depuis "+p.Etat)
		return
	}
	sum := sha256.Sum256([]byte(req.Contenu))
	p.documents = append(p.documents, nom)
	p.ContexteOctets += len(req.Contenu)
	m.journaliser(p, map[string]any{"type": "DEPOT_DOCUMENT", "nom": nom,
		"empreinte": hex.EncodeToString(sum[:])})
	repondreJSON(w, http.StatusOK, p)
}

func (m *moteurDemo) missionFille(w http.ResponseWriter, p *projetDemo) {
	if p.Etat != etatTransmis {
		repondreErreur(w, http.StatusNotFound, "PROJET_INCONNU",
			"aucune mission fille : le projet n'est pas transmis")
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{
		"issue": p.ID + "-EXEC", "etat": "EN_COURS",
		"executant": p.ExecutantID, "receptionnaire": "openai-recep",
		"roles": map[string][]string{
			"EXECUTANT":      {"executer", "soumettre", "acker"},
			"RECEPTIONNAIRE": {"recevoir"},
			"PILOTE":         {"cloturer"},
		},
		"empreinte_ordre": p.EmpreinteOrdre,
	})
}

func (m *moteurDemo) obtenirOrdre(w http.ResponseWriter, p *projetDemo) {
	if p.Ordre == nil {
		repondreErreur(w, http.StatusUnprocessableEntity, "ORDRE_ABSENT", "aucun ordre")
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{
		"ordre": p.Ordre, "version": p.Version, "empreinte": p.EmpreinteOrdre,
	})
}

func (m *moteurDemo) servirJournal(w http.ResponseWriter, p *projetDemo) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, e := range p.journal {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		_, _ = w.Write(append(b, '\n'))
	}
}

// donneesExport : lecture du projet pour l'export du dossier de sortie
// (option A, M-COORD-DRIVE-01). Accesseur ADDITIF, sous mutex : aucun
// comportement du moteur n'est modifié.
func (m *moteurDemo) donneesExport(id string) (donneesExport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projets[id]
	if !ok {
		return donneesExport{}, &erreurDrive{code: "DRIVE_ERREUR",
			motif: "projet inconnu : " + id}
	}
	var journal bytes.Buffer
	for _, e := range p.journal {
		if b, err := json.Marshal(e); err == nil {
			journal.Write(b)
			journal.WriteByte('\n')
		}
	}
	return donneesExport{
		Cadrage:   p.Cadrage,
		Ordre:     p.Ordre,
		Version:   p.Version,
		Empreinte: p.EmpreinteOrdre,
		Devis:     p.Devis,
		Journal:   journal.Bytes(),
	}, nil
}

// repondreJSON / repondreErreur / lireJSON : parité avec le service
// coord-maieutique (mêmes formes {"code","motif"} et mêmes statuts).
func repondreJSON(w http.ResponseWriter, statut int, corps any) {
	b, err := json.Marshal(corps)
	if err != nil {
		http.Error(w, "réponse interne impossible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statut)
	_, _ = w.Write(b)
}

func repondreErreur(w http.ResponseWriter, statut int, code, motif string) {
	repondreJSON(w, statut, map[string]any{"code": code, "motif": motif})
}

func lireJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

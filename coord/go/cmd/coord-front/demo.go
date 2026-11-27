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

	// M-COORD-API-MISTRAL-01 : niveau choisi par le client à la
	// création (spec §3.2 : stocké dans la définition de mission, ne
	// change plus), compteur d'appels moteur et plafond (spec §3.3).
	Niveau        string         `json:"niveau,omitempty"`
	AppelsMoteurs int            `json:"appels_moteurs"`
	PlafondAppels int            `json:"plafond_appels,omitempty"`
	Critique      map[string]any `json:"critique,omitempty"`

	tours     int              `json:"-"`
	journal   []map[string]any `json:"-"`
	documents []string         `json:"-"`
	empreinte string           `json:"-"`
	escalade  bool             `json:"-"`
}

type moteurDemo struct {
	gw      *gateway
	mu      sync.Mutex
	projets map[string]*projetDemo
}

func nouveauMoteurDemo(gw *gateway) *moteurDemo {
	return &moteurDemo{gw: gw, projets: map[string]*projetDemo{}}
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

// ---- M-COORD-API-MISTRAL-01 : prompts et enrichissement serveur ------

// systemeDirecteur : rôle du directeur de mission pour les étapes
// cadrage et rédaction. Aucune donnée secrète, jamais de clé.
const systemeDirecteur = "Tu es le directeur de mission du logiciel COORD. " +
	"Tu cadres les intentions du client avec des questions précises et brèves, " +
	"en français, sans préambule ni formule de politesse."

// historiqueDialogue : reconstitution textuelle des tours déjà joués
// (client puis directeur), à partir du journal de la mission.
func historiqueDialogue(p *projetDemo) string {
	var sb strings.Builder
	sb.WriteString("Historique du dialogue :\n")
	for _, e := range p.journal {
		if e["type"] == "TOUR_CLIENT" {
			fmt.Fprintf(&sb, "Client : %v\n", e["entree"])
		}
		if e["type"] == "TOUR_DIRECTEUR" {
			fmt.Fprintf(&sb, "Directeur : %v\n", e["contenu"])
		}
	}
	return sb.String()
}

// enrichirOrdre : remplace les champs rédigeables de l'ordre factice par
// ceux produits par l'étape rédaction/revision. JSON strict attendu
// (clôtures ``` tolérées) ; échec d'analyse = ordre factice conservé
// (fail-closed, jamais de mission cassée par une réponse hors format).
func enrichirOrdre(ordre map[string]any, texte string) {
	t := strings.TrimSpace(texte)
	if debut := strings.Index(t, "{"); debut > 0 {
		t = t[debut:]
	}
	if fin := strings.LastIndex(t, "}"); fin >= 0 {
		t = t[:fin+1]
	}
	var v struct {
		EtatDesLieux string `json:"etat_des_lieux"`
		Objectif     string `json:"objectif"`
		Etapes       []struct {
			Nom                   string `json:"nom"`
			ControleIntermediaire string `json:"controle_intermediaire"`
		} `json:"etapes"`
		Limites           []string `json:"limites"`
		Livrable          string   `json:"livrable"`
		CriteresReception []string `json:"criteres_reception"`
	}
	if err := json.Unmarshal([]byte(t), &v); err != nil {
		return
	}
	if v.EtatDesLieux != "" {
		ordre["etat_des_lieux"] = v.EtatDesLieux
	}
	if v.Objectif != "" {
		ordre["objectif"] = v.Objectif
	}
	if len(v.Etapes) > 0 {
		etapes := []map[string]any{}
		for _, e := range v.Etapes {
			if e.Nom == "" {
				continue
			}
			etapes = append(etapes, map[string]any{
				"nom": e.Nom, "controle_intermediaire": e.ControleIntermediaire})
		}
		if len(etapes) > 0 {
			ordre["etapes"] = etapes
		}
	}
	if len(v.Limites) > 0 {
		ordre["limites"] = v.Limites
	}
	if v.Livrable != "" {
		ordre["livrable"] = v.Livrable
	}
	if len(v.CriteresReception) > 0 {
		ordre["criteres_reception"] = v.CriteresReception
	}
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
		Niveau    string `json:"niveau"`
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
	// M-COORD-API-MISTRAL-01 : niveau de la mission (défaut "avance"),
	// validé fail-closed contre la grille connue.
	req.Niveau = strings.TrimSpace(req.Niveau)
	if req.Niveau == "" {
		req.Niveau = "avance"
	}
	regime := moteursConf{}
	if m.gw != nil {
		regime = m.gw.regimeMoteurs()
	}
	if !niveauxMoteursConnus[req.Niveau] {
		repondreErreur(w, http.StatusBadRequest, "NIVEAU_INCONNU", "niveau inconnu : "+req.Niveau)
		return
	}
	if len(regime.Niveaux) > 0 {
		if _, ok := regime.Niveaux[req.Niveau]; !ok {
			repondreErreur(w, http.StatusBadRequest, "NIVEAU_INCONNU", "niveau non proposé par la grille : "+req.Niveau)
			return
		}
	}
	p := &projetDemo{
		ID: req.ID, Etat: etatIntention, Intention: req.Intention,
		UrlsAcceptees: map[string]bool{}, SourcesDeclarees: []string{},
		Niveau: req.Niveau, PlafondAppels: regime.plafond(),
		InstantUTC: maintenantDemo(),
	}
	m.journaliser(p, map[string]any{"type": "DECLARATION_INTENTION", "acteur": "pilote", "intention": req.Intention, "niveau": req.Niveau})
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

	// M-COORD-API-MISTRAL-01 : si au moins un moteur IA est activé,
	// les tours sont produits par l'itinéraire de cascade du niveau
	// (étape cadrage, puis rédaction pour la proposition d'ordre).
	// Sans AUCUNE clé : repli sur le script déterministe ci-dessous
	// (comportement actuel, 0 €, rien ne casse).
	if m.gw != nil && m.gw.moteursActifs() {
		action := "QUESTION"
		if p.tours == 2 {
			action = "CLARIFICATION"
		}
		if p.tours >= 3 {
			// Troisième tour : PROPOSITION_ORDRE, champs de l'ordre
			// rédigés par l'étape "redaction" du niveau.
			demande := "Mission : " + p.ID + "\nIntention du client : " + p.Intention + "\n" +
				historiqueDialogue(p) +
				"\nProduis l'ordre de mission en JSON STRICT avec UNIQUEMENT les clés : " +
				"\"etat_des_lieux\" (chaîne), \"objectif\" (chaîne), \"etapes\" (tableau de " +
				"{\"nom\", \"controle_intermediaire\"}), \"limites\" (tableau de chaînes), " +
				"\"livrable\" (chaîne), \"criteres_reception\" (tableau de chaînes). " +
				"Aucun texte hors du JSON."
			res, err := m.gw.appelEtape(p, "redaction", systemeDirecteur, demande)
			if err != nil {
				repondreErreur(w, err.Statut, err.Code, err.Motif)
				return
			}
			ordre := ordreFactice(p.ID, p.Intention)
			enrichirOrdre(ordre, res.Texte)
			p.Ordre = ordre
			p.Etat = etatOrdrePropose
			rep := map[string]any{"action": "PROPOSITION_ORDRE", "ordre": p.Ordre}
			m.journaliser(p, map[string]any{"type": "PROPOSITION_ORDRE", "acteur": p.DirecteurID})
			repondreJSON(w, http.StatusOK, map[string]any{"projet": p, "reponse": rep})
			return
		}
		consigne := "Pose UNE seule question ouverte et brève pour préciser la finalité attendue."
		if p.tours == 2 {
			consigne = "Demande UNE seule clarification courte sur le périmètre et le livrable concret attendu."
		}
		demande := "Intention du client : " + p.Intention + "\n" + historiqueDialogue(p) +
			"\n" + consigne + " Réponds par le message du directeur uniquement, sans préambule."
		res, err := m.gw.appelEtape(p, "cadrage", systemeDirecteur, demande)
		if err != nil {
			repondreErreur(w, err.Statut, err.Code, err.Motif)
			return
		}
		rep := map[string]any{"action": action, "contenu": res.Texte}
		m.journaliser(p, map[string]any{"type": "TOUR_DIRECTEUR",
			"acteur": p.DirecteurID, "reponse": action, "contenu": res.Texte})
		repondreJSON(w, http.StatusOK, map[string]any{"projet": p, "reponse": rep})
		return
	}

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

	// M-COORD-API-MISTRAL-01 : étape "critique" de l'itinéraire (si au
	// moins un moteur activé), puis escalade Premium bornée à UNE
	// reprise (étape "revision") si la critique juge l'ordre insuffisant.
	if m.gw != nil && m.gw.moteursActifs() {
		ordreJSON, _ := json.Marshal(p.Ordre)
		demande := "Voici l'ordre de mission à critiquer (JSON) :\n" + string(ordreJSON) +
			"\nRéponds en JSON STRICT : {\"suffisant\": true ou false, " +
			"\"remarques\": [tableau de chaînes]}. Aucun texte hors du JSON."
		systeme := "Tu es le critique indépendant du logiciel COORD. Tu évalues un ordre " +
			"de mission : clarté de l'objectif, étapes contrôlables, limites et critères " +
			"de réception vérifiables. Tu réponds en JSON strict, en français."
		res, err := m.gw.appelEtape(p, "critique", systeme, demande)
		if err != nil {
			repondreErreur(w, err.Statut, err.Code, err.Motif)
			return
		}
		var verdict struct {
			Suffisant bool     `json:"suffisant"`
			Remarques []string `json:"remarques"`
		}
		critiqueOK := false
		t := strings.TrimSpace(res.Texte)
		if debut := strings.Index(t, "{"); debut >= 0 {
			if fin := strings.LastIndex(t, "}"); fin > debut {
				if json.Unmarshal([]byte(t[debut:fin+1]), &verdict) == nil {
					critiqueOK = true
				}
			}
		}
		if critiqueOK {
			p.Critique = map[string]any{"suffisant": verdict.Suffisant, "remarques": verdict.Remarques}
		} else {
			p.Critique = map[string]any{"suffisant": true, "remarques": []string{}}
		}
		m.journaliser(p, map[string]any{"type": "CRITIQUE", "acteur": p.ExecutantID,
			"suffisant": p.Critique["suffisant"]})
		if critiqueOK && !verdict.Suffisant && p.Niveau == "premium" && !p.escalade {
			// Escalade Premium : UNE seule reprise de rédaction par
			// mission (étape "revision"), comptée au journal.
			demandeRev := "L'ordre de mission suivant a été jugé insuffisant par la critique.\n" +
				"Remarques de la critique : " + strings.Join(verdict.Remarques, " ; ") +
				"\nOrdre actuel (JSON) :\n" + string(ordreJSON) +
				"\nProduis l'ordre corrigé en JSON STRICT avec UNIQUEMENT les clés : " +
				"\"etat_des_lieux\", \"objectif\", \"etapes\", \"limites\", \"livrable\", " +
				"\"criteres_reception\". Aucun texte hors du JSON."
			res2, err2 := m.gw.appelEtape(p, "revision", systemeDirecteur, demandeRev)
			if err2 != nil {
				repondreErreur(w, err2.Statut, err2.Code, err2.Motif)
				return
			}
			enrichirOrdre(p.Ordre, res2.Texte)
			p.Version++
			idn["version"] = fmt.Sprintf("v%d", p.Version)
			p.EmpreinteOrdre = empreinteOrdreFactice(p.Ordre)
			p.escalade = true
			m.journaliser(p, map[string]any{"type": "ESCALADE_PREMIUM", "acteur": p.DirecteurID,
				"version": idn["version"]})
		}
	}
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

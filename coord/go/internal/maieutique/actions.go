// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ActeurPilote : identité locale du pilote humain (journalisée, D10).
const ActeurPilote = "pilote-local"

// ActeurDirecteur : identité du directeur dans le journal (D10).
func ActeurDirecteur(id string) string { return "directeur:" + id }

// DeclarerIntention crée un projet maïeutique depuis une intention
// (plan v2 D2, transition « — » → INTENTION).
func (s *Store) DeclarerIntention(id, intention string) (*Projet, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, erreur(CodeProjetInconnu, "identifiant de projet vide interdit")
	}
	if strings.TrimSpace(intention) == "" {
		return nil, erreur(CodeIntentionVide, "intention vide interdite")
	}
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	if _, err := s.charger(id); err == nil {
		return nil, erreur(CodeProjetExistant, "projet déjà existant : "+id)
	}
	p := &Projet{
		ID:               id,
		Etat:             EtatSansProjet,
		Intention:        intention,
		UrlsAcceptees:    map[string]bool{},
		SourcesDeclarees: []string{},
	}
	if err := os.MkdirAll(s.dir(id), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(s.dir(id), "intention.md"), []byte(intention+"\n"), 0o644); err != nil {
		return nil, err
	}
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionDeclarerIntention, ActeurPilote,
		map[string]any{"intention": intention}); err != nil {
		return nil, err
	}
	return p, nil
}

// Obtenir relit l'état durable d'un projet (reprise de processus, D6).
func (s *Store) Obtenir(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	return s.charger(id)
}

// Lister : les projets existants (identifiants triés).
func (s *Store) Lister() ([]string, error) {
	ents, err := os.ReadDir(s.cfg.Racine)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.cfg.Racine, e.Name(), "projet.json")); err == nil {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// DeclarerSources : le pilote déclare les sources admises (D3, contrôle 1a).
func (s *Store) DeclarerSources(id string, refs []string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	p.SourcesDeclarees = append(p.SourcesDeclarees, refs...)
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	entrees, err := enListe([]map[string]any{{
		"type": "DECLARATION_SOURCES", "acteur": ActeurPilote,
		"sources": refs, "instant_utc": maintenantUTC(),
	}})
	if err != nil {
		return nil, err
	}
	if _, err := persistJournal(entrees, s.cheminJournal(id)); err != nil {
		return nil, err
	}
	return p, nil
}

// AccepterURL : le pilote accepte EXPLICITEMENT une URL proposée par le
// directeur (D3, contrôle 1c : l'acceptation a lieu au tour suivant).
func (s *Store) AccepterURL(id, url string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if !estURL(url) {
		return nil, erreur(CodeDocumentInvalide, "acceptation d'une URL http(s) attendue : "+url)
	}
	if p.UrlsAcceptees == nil {
		p.UrlsAcceptees = map[string]bool{}
	}
	p.UrlsAcceptees[strings.TrimSpace(url)] = true
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	entrees, err := enListe([]map[string]any{{
		"type": "ACCEPTATION_URL", "acteur": ActeurPilote,
		"url": strings.TrimSpace(url), "instant_utc": maintenantUTC(),
	}})
	if err != nil {
		return nil, err
	}
	if _, err := persistJournal(entrees, s.cheminJournal(id)); err != nil {
		return nil, err
	}
	return p, nil
}

// OuvrirDialogue : le pilote choisit le binôme directeur/exécutant
// DISTINCTS (D2/D4, échec BINOME_IDENTIQUE si identiques).
func (s *Store) OuvrirDialogue(id, directeurID, executantID string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	directeurID = strings.TrimSpace(directeurID)
	executantID = strings.TrimSpace(executantID)
	if directeurID == "" || executantID == "" {
		return nil, erreur(CodeBinomeIdentique, "binôme directeur/exécutant requis")
	}
	if directeurID == executantID {
		return nil, erreur(CodeBinomeIdentique,
			"directeur et exécutant doivent être DISTINCTS : "+directeurID)
	}
	if _, ok := s.cfg.Adaptateurs[directeurID]; !ok {
		return nil, erreur(CodeDirecteurInconnu, "directeur inconnu du registre : "+directeurID)
	}
	if _, ok := s.cfg.Adaptateurs[executantID]; !ok {
		return nil, erreur(CodeExecutantInconnu, "exécutant inconnu du registre : "+executantID)
	}
	p.DirecteurID = directeurID
	p.ExecutantID = executantID
	if err := s.transition(p, ActionOuvrirDialogue, ActeurPilote,
		map[string]any{"directeur": directeurID, "executant": executantID}); err != nil {
		return nil, err
	}
	return p, nil
}

// appelerDirecteur : un aller-retour vers le directeur. Retourne la
// réponse STRUCTURÉE (déjà validée en forme) ou l'erreur stable
// (INDISPONIBILITE_DIRECTEUR / REPONSE_INVALIDE sont comptées par
// l'appelant pour la borne ATTENTE_PILOTE).
func (s *Store) appelerDirecteur(ctx context.Context, p *Projet, charge string) (*ReponseDirecteur, error) {
	directeur, ok := s.cfg.Adaptateurs[p.DirecteurID]
	if !ok {
		return nil, erreur(CodeDirecteurInconnu, "directeur inconnu : "+p.DirecteurID)
	}
	res, err := directeur.Executer(charge)
	p.Appels++
	if err != nil {
		return nil, erreur(CodeIndisponibilite, "appel du directeur échoué : "+err.Error())
	}
	return analyserReponseTour(res.Sortie)
}

// echecDirecteur : compteur d'échecs borné (TentativesDirecteur). Sous
// la borne : l'échec est journalisé et l'erreur stable retournée. À la
// borne : le projet passe à ATTENTE_PILOTE (garde du plan v2 D2 :
// « réponse invalide ×2 OU indisponibilité ×2 »), motif et horodatage
// journalisés — aucun blocage permanent, sorties explicites du pilote.
func (s *Store) echecDirecteur(p *Projet, code, motif string) error {
	p.EchecsDirecteur++
	if err := s.journaliser(p.ID, map[string]any{
		"type": "ECHEC_DIRECTEUR", "code": code, "motif": motif,
		"echecs": p.EchecsDirecteur, "instant_utc": maintenantUTC(),
	}); err != nil {
		return err
	}
	if p.EchecsDirecteur < s.cfg.TentativesDirecteur {
		if err := s.sauver(p); err != nil {
			return err
		}
		return erreur(code, motif)
	}
	return s.versAttentePilote(p, code, motif)
}

// versAttentePilote : sortie structurée vers ATTENTE_PILOTE (garde D2
// sur l'action tour_directeur). L'état est sauvé atomiquement et la
// transition journalisée (D10) avec son motif.
func (s *Store) versAttentePilote(p *Projet, code, motif string) error {
	avant := p.Etat
	p.Etat = EtatAttentePilote
	if err := s.sauver(p); err != nil {
		return err
	}
	return s.journaliser(p.ID, entreeJournal(ActeurDirecteur(p.DirecteurID),
		ActionTourDirecteur, avant, EtatAttentePilote, p.Appels, p.ContexteOctets,
		map[string]any{"code_echec": code, "motif": motif, "echecs": p.EchecsDirecteur}))
}

// TourDirecteur : un tour de dialogue (D2). L'échec d'appel ou une
// réponse invalide sont comptés (borne → ATTENTE_PILOTE) ; QUESTION et
// CLARIFICATION journalisent le tour SANS transition autre que la
// boucle DIALOGUE→DIALOGUE ; PROPOSITION_ORDRE déclenche
// evaluerProposition ; un VERDICT direct est journalisé (aucune
// transition d'état sur un verdict isolé : la critique ne vaut
// jamais action).
func (s *Store) TourDirecteur(ctx context.Context, id, entreePilote string) (*Projet, *ReponseDirecteur, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, nil, err
	}
	if p.Etat != EtatDialogue {
		return nil, nil, erreur(CodeActionInterdit, "tour directeur requis depuis DIALOGUE, pas depuis "+p.Etat)
	}
	// Recherche documentaire d'abord (D4) : l'entrée du pilote sert de
	// requête quand le corpus contient des documents.
	documents := []ResultatRecherche{}
	if strings.TrimSpace(entreePilote) != "" {
		if idx, err := s.NouvelIndex(p); err == nil {
			documents = idx.Rechercher(entreePilote)
		}
	}
	charge, err := s.construireCharge(p, entreePilote, documents)
	if err != nil {
		return nil, nil, err
	}
	rep, err := s.appelerDirecteur(ctx, p, charge)
	if err != nil {
		if estErreurCode(err, CodeIndisponibilite) || estErreurCode(err, CodeReponseInvalide) {
			code := CodeIndisponibilite
			if estErreurCode(err, CodeReponseInvalide) {
				code = CodeReponseInvalide
			}
			if err2 := s.echecDirecteur(p, code, err.Error()); err2 != nil {
				return nil, nil, err2
			}
			return nil, nil, err
		}
		return nil, nil, err
	}
	p.EchecsDirecteur = 0
	tour := map[string]any{
		"type": "TOUR_DIRECTEUR", "action": rep.Action,
		"directeur": p.DirecteurID, "instant_utc": maintenantUTC(),
		"appels": p.Appels, "contexte_octets": p.ContexteOctets,
	}
	switch rep.Action {
	case ReponseQuestion, ReponseClarification:
		tour["contenu"] = rep.Contenu
		if err := s.journaliserTour(p, tour); err != nil {
			return nil, nil, err
		}
		if err := s.journaliser(p.ID, tour); err != nil {
			return nil, nil, err
		}
		if err := s.transition(p, ActionTourDirecteur, ActeurDirecteur(p.DirecteurID),
			map[string]any{"reponse": rep.Action}); err != nil {
			return nil, nil, err
		}
		return p, rep, nil
	case ReponsePropositionOrdre:
		if err := s.journaliserTour(p, tour); err != nil {
			return nil, nil, err
		}
		if err := s.journaliser(p.ID, tour); err != nil {
			return nil, nil, err
		}
		if err := s.evaluerProposition(ctx, p, rep.Ordre); err != nil {
			return nil, nil, err
		}
		return p, rep, nil
	case ReponseVerdict:
		tour["verdict"] = rep.Verdict
		tour["points_detail"] = rep.Points
		if err := s.journaliserTour(p, tour); err != nil {
			return nil, nil, err
		}
		if err := s.journaliser(p.ID, tour); err != nil {
			return nil, nil, err
		}
		if critiqueBloquante(rep) {
			if err := s.journaliser(p.ID, map[string]any{
				"type": "CRITIQUE_BLOQUANTE", "verdict": rep.Verdict,
				"instant_utc": maintenantUTC(),
			}); err != nil {
				return nil, nil, err
			}
			return p, rep, erreur(CodeCritiqueBloquante,
				"verdict "+rep.Verdict+" avec point BLOQUANT/MAJEUR : retour au dialogue décidé par le pilote")
		}
		return p, rep, nil
	}
	return nil, nil, erreur(CodeReponseInvalide, "action non traitée : "+rep.Action)
}

// ProposerOrdre : le PILOTE propose lui-même l'ordre (D2, action
// proposer_ordre depuis DIALOGUE) : la validation déterministe passe
// D'ABORD, puis la critique sémantique du directeur est OBLIGATOIRE
// (VERDICT sans BLOQUANT/MAJEUR) avant tout passage à ORDRE_PROPOSE.
func (s *Store) ProposerOrdre(ctx context.Context, id string, ordreJSON json.RawMessage) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.Etat != EtatDialogue {
		return nil, erreur(CodeActionInterdit, "proposer_ordre requis depuis DIALOGUE, pas depuis "+p.Etat)
	}
	if err := s.evaluerProposition(ctx, p, ordreJSON); err != nil {
		return nil, err
	}
	return p, nil
}

// evaluerProposition : la proposition d'ordre passe TROIS portes, dans
// cet ordre déterministe (D2/D3) :
//  1. décodage JSON strict de l'ordre ;
//  2. validation DÉTERMINISTE (schéma + règles, codes stables) :
//     un ordre non conforme ne déclenche AUCUN appel du directeur ;
//  3. critique sémantique OBLIGATOIRE du directeur : VERDICT sans
//     BLOQUANT/MAJEUR. Alors seulement : transition proposer_ordre
//     DIALOGUE → ORDRE_PROPOSE.
func (s *Store) evaluerProposition(ctx context.Context, p *Projet, ordreJSON json.RawMessage) error {
	var o Ordre
	if err := json.Unmarshal(ordreJSON, &o); err != nil {
		return erreur(CodeOrdreNonConforme, "ordre non décodable : "+err.Error())
	}
	codes := ValiderOrdre(&o)
	if len(codes) > 0 {
		if err := s.journaliser(p.ID, map[string]any{
			"type": "REJET_ORDRE", "codes": codes, "instant_utc": maintenantUTC(),
		}); err != nil {
			return err
		}
		return erreur(codes[0], "ordre rejeté par la validation déterministe : "+
			strings.Join(codes, ", "))
	}
	// Critique sémantique OBLIGATOIRE du directeur (D2/D4).
	charge, err := s.construireCharge(p, "Critique sémantique de l'ordre proposé. Réponds par un VERDICT avec points_detail.",
		nil)
	if err != nil {
		return err
	}
	charge = charge + "\n--- ORDRE PROPOSE (critique demandée) ---\n" + string(ordreJSON) + "\n"
	p.ContexteOctets += len(string(ordreJSON))
	rep, err := s.appelerDirecteur(ctx, p, charge)
	if err != nil {
		if estErreurCode(err, CodeIndisponibilite) || estErreurCode(err, CodeReponseInvalide) {
			code := CodeIndisponibilite
			if estErreurCode(err, CodeReponseInvalide) {
				code = CodeReponseInvalide
			}
			if err2 := s.echecDirecteur(p, code, err.Error()); err2 != nil {
				return err2
			}
		}
		return err
	}
	if rep.Action != ReponseVerdict {
		return erreur(CodeReponseInvalide,
			"la critique sémantique exige un VERDICT, reçu : "+rep.Action)
	}
	if critiqueBloquante(rep) {
		if err := s.journaliser(p.ID, map[string]any{
			"type": "CRITIQUE_BLOQUANTE", "verdict": rep.Verdict,
			"points_detail": rep.Points, "instant_utc": maintenantUTC(),
		}); err != nil {
			return err
		}
		return erreur(CodeCritiqueBloquante,
			"critique sémantique BLOQUANTE ("+rep.Verdict+") : ordre renvoyé au dialogue")
	}
	p.Ordre = &o
	p.EchecsDirecteur = 0
	return s.transition(p, ActionProposerOrdre, ActeurPilote,
		map[string]any{"directeur": p.DirecteurID, "verdict": rep.Verdict})
}

// Relancer : décision pilote depuis ATTENTE_PILOTE (re-tentative
// autorisée, compteur d'échecs REMIS, retour à DIALOGUE, D2).
func (s *Store) Relancer(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	p.EchecsDirecteur = 0
	if err := s.transition(p, ActionRelancer, ActeurPilote, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// Rectifier : décision pilote + entrée corrigée (D2).
func (s *Store) Rectifier(id, intentionCorrigee string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(intentionCorrigee) != "" {
		p.Intention = intentionCorrigee
	}
	p.EchecsDirecteur = 0
	if err := s.transition(p, ActionRectifier, ActeurPilote,
		map[string]any{"intention": p.Intention}); err != nil {
		return nil, err
	}
	return p, nil
}

// Abandonner : décision pilote, terminal PROPRE (ABANDONNE, état et
// journal conservés, D2).
func (s *Store) Abandonner(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionAbandonner, ActeurPilote, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Qualifier fait passer l'ordre proposé par la validation DÉTERMINISTE
// COMPLÈTE (plan v2 D3) : structure + règles + les trois contrôles de
// sources (provenance, existence sondée, cohérence). Tout échec est
// classé BLOQUANT : retour au dialogue décidé par le pilote (aucune
// transition automatique). En cas de succès, l'ordre est FIGÉ : version
// vN, sha256, copie canonique dans ordres/ordre-fige.json (D6).
func (s *Store) Qualifier(ctx context.Context, id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.Ordre == nil {
		return nil, erreur(CodeOrdreAbsent, "aucun ordre proposé à qualifier")
	}
	codes := ValiderOrdre(p.Ordre)
	if len(codes) > 0 {
		return nil, erreur(codes[0],
			"qualification refusée (validation déterministe) : "+strings.Join(codes, ", "))
	}
	codesSources := s.VerifieSources(ctx, p)
	if len(codesSources) > 0 {
		if err := s.sauver(p); err != nil {
			return nil, err
		}
		return nil, erreur(codesSources[0],
			"qualification refusée (sources, échec BLOQUANT) : "+strings.Join(codesSources, ", "))
	}
	// Figeage : version vN + sha256 de la forme canonique figée.
	p.Version++
	p.Ordre.Identification.Version = fmt.Sprintf("v%d", p.Version)
	empreinte, err := p.Ordre.Empreinte()
	if err != nil {
		return nil, err
	}
	p.EmpreinteOrdre = empreinte
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	// Copie canonique figée (octet pour octet, D6/D7).
	m, err := enMap(p.Ordre)
	if err != nil {
		return nil, err
	}
	dirOrdres := filepath.Join(s.dir(id), "ordres")
	if err := os.MkdirAll(dirOrdres, 0o755); err != nil {
		return nil, err
	}
	if err := persistSaveAtomic(m, filepath.Join(dirOrdres, "ordre-fige.json")); err != nil {
		return nil, err
	}
	if err := persistSaveAtomic(m, filepath.Join(dirOrdres,
		fmt.Sprintf("ordre-v%d.json", p.Version))); err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionQualifier, ActeurPilote,
		map[string]any{"version": p.Ordre.Identification.Version, "empreinte": empreinte}); err != nil {
		return nil, err
	}
	return p, nil
}

// ValiderPilote : décision EXPLICITE du pilote (jeton d'action fourni par
// l'interface locale, JAMAIS persisté : validation.json ne contient pas
// le jeton). Transition valider_pilote ORDRE_QUALIFIE → VALIDE.
func (s *Store) ValiderPilote(id, jetonAction string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	if strings.TrimSpace(jetonAction) == "" {
		return nil, erreur(CodeJetonPiloteAbsent,
			"jeton d'action pilote requis : la validation est un acte explicite")
	}
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.EmpreinteOrdre == "" {
		return nil, erreur(CodeOrdreAbsent, "aucun ordre qualifié à valider")
	}
	if err := s.transition(p, ActionValiderPilote, ActeurPilote,
		map[string]any{"empreinte": p.EmpreinteOrdre}); err != nil {
		return nil, err
	}
	// validation.json : la PREUVE de décision, sans le jeton.
	validation := map[string]any{
		"decision":    "VALIDE",
		"version":     p.Ordre.Identification.Version,
		"empreinte":   p.EmpreinteOrdre,
		"instant_utc": maintenantUTC(),
	}
	if err := persistSaveAtomic(validation, filepath.Join(s.dir(id), "validation.json")); err != nil {
		return nil, err
	}
	return p, nil
}

// Publier publie l'ordre validé dans le répertoire pilote autorisé
// (livrable/ du projet, D6). Idempotent (D7) : une publication déjà
// effectuée rend OPERATION_DEJA_EFFECTUEE, AUCUN second appel, aucun
// double effet.
func (s *Store) Publier(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.EmpreinteOrdre == "" {
		return nil, erreur(CodeOrdreAbsent, "aucun ordre validé à publier")
	}
	// Opération idempotente : déjà publiée (dans PUBLIE ou TRANSMIS) ?
	if p.Etat == EtatPublie || p.Etat == EtatTransmis {
		if err := s.journaliser(p.ID, map[string]any{
			"type": "REFUS_PUBLICATION", "code": CodeOperationDejaFait,
			"instant_utc": maintenantUTC(),
		}); err != nil {
			return nil, err
		}
		return nil, erreur(CodeOperationDejaFait,
			"publication déjà effectuée (empreinte "+p.EmpreinteOrdre+") : aucun second appel")
	}
	publication := map[string]any{
		"type":        "PUBLICATION",
		"version":     p.Ordre.Identification.Version,
		"empreinte":   p.EmpreinteOrdre,
		"instant_utc": maintenantUTC(),
	}
	// Dossier pilote autorisé : livrable/ du projet uniquement (D6).
	dirLivrable := filepath.Join(s.dir(id), "livrable")
	if err := os.MkdirAll(dirLivrable, 0o755); err != nil {
		return nil, err
	}
	if err := persistSaveAtomic(publication, filepath.Join(s.dir(id), "publication.json")); err != nil {
		return nil, err
	}
	m, err := enMap(p.Ordre)
	if err != nil {
		return nil, err
	}
	if err := persistSaveAtomic(m, filepath.Join(dirLivrable,
		fmt.Sprintf("ordre-%s.json", p.Ordre.Identification.Version))); err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionPublier, ActeurPilote,
		map[string]any{"empreinte": p.EmpreinteOrdre}); err != nil {
		return nil, err
	}
	return p, nil
}

// Transmettre crée la mission fille dans l'orchestrateur EXISTANT du
// moteur (NouvelleMissionExplicit, D7), mandat = ordre validé FIGÉ octet
// pour octet, empreinte validée == publiée == transmise. Idempotent :
// un second appel rend OPERATION_DEJA_EFFECTUEE ; une mission fille
// « déjà existante » est une COMPLÉTION DE REPRISE (interruption entre
// la création mission et l'écriture du état durable), vérifiée puis
// acceptée, jamais une seconde création.
func (s *Store) Transmettre(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	// Contrôle déterministe D7 : l'empreinte PUBLIÉE doit être l'empreinte
	// de l'ordre figé validé.
	pub := map[string]any{}
	raw, err := os.ReadFile(filepath.Join(s.dir(id), "publication.json"))
	if err != nil {
		return nil, erreur(CodeOrdreAbsent, "aucune publication : publier avant transmettre")
	}
	if err := json.Unmarshal(raw, &pub); err != nil {
		return nil, erreur(CodeOrdreAbsent, "publication illisible : "+err.Error())
	}
	if fmt.Sprint(pub["empreinte"]) != p.EmpreinteOrdre {
		return nil, erreur(CodeEmpreinteDivergente,
			"l'empreinte publiée diverge de l'ordre validé : publication interrompue ou altérée")
	}
	if p.Etat == EtatTransmis {
		if err := s.journaliser(p.ID, map[string]any{
			"type": "REFUS_TRANSMISSION", "code": CodeOperationDejaFait,
			"instant_utc": maintenantUTC(),
		}); err != nil {
			return nil, err
		}
		return nil, erreur(CodeOperationDejaFait, "transmission déjà effectuée : aucun second appel")
	}
	tr := s.cfg.Transmission
	if s.cfg.Orchestrateur == nil {
		return nil, erreur(CodeActionInterdit, "aucun orchestrateur configuré (transmission impossible)")
	}
	if p.Etat != EtatPublie {
		return nil, erreur(CodeActionInterdit, "transmettre requis depuis PUBLIE, pas depuis "+p.Etat)
	}
	err = s.cfg.Orchestrateur.NouvelleMissionExplicit(tr.IssueFille,
		tr.ExecutantID, tr.RecepteurID, tr.QuaExecID, tr.QuaRecepID,
		tr.Roles, tr.Plafonds)
	reprise := false
	if err != nil {
		if strings.Contains(err.Error(), "deja existante") {
			// COMPLÉTION DE REPRISE : la mission fille existe déjà
			// (interruption réelle pendant la transmission) ; on ne
			// crée RIEN de nouveau, on vérifie et on complète.
			reprise = true
		} else {
			return nil, err
		}
	}
	// Ancre TRANSMISSION_MAIEUTIQUE dans le journal de la mission fille
	// (lien explicite vers le projet maïeutique, D7/D10).
	if s.cfg.Referentiel != nil {
		if err := s.cfg.Referentiel.Journaliser(tr.IssueFille, map[string]any{
			"type":        "TRANSMISSION_MAIEUTIQUE",
			"projet":      id,
			"version":     p.Ordre.Identification.Version,
			"empreinte":   p.EmpreinteOrdre,
			"reprise":     reprise,
			"instant_utc": maintenantUTC(),
			"lien_mere":   tr.LienIssueMere,
		}); err != nil {
			return nil, err
		}
	}
	transmission := map[string]any{
		"type":        "TRANSMISSION",
		"issue_fille": tr.IssueFille,
		"version":     p.Ordre.Identification.Version,
		"empreinte":   p.EmpreinteOrdre,
		"reprise":     reprise,
		"instant_utc": maintenantUTC(),
	}
	if err := persistSaveAtomic(transmission, filepath.Join(s.dir(id), "transmission.json")); err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionTransmettre, ActeurPilote,
		map[string]any{"issue_fille": tr.IssueFille, "reprise": reprise}); err != nil {
		return nil, err
	}
	return p, nil
}

// Invalider : toute modification après validation/publication mène à
// INVALIDE (D2). Le journal et l'état restent lisibles (preuve conservée).
func (s *Store) Invalider(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionInvalider, ActeurPilote, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// RetourDialogue : refus du pilote en ORDRE_PROPOSE ou ORDRE_QUALIFIE,
// motif journalisé (D2).
func (s *Store) RetourDialogue(id, motif string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if err := s.transition(p, ActionRetourDialogue, ActeurPilote,
		map[string]any{"motif": motif}); err != nil {
		return nil, err
	}
	return p, nil
}

// FixerCadrage : le cadrage client est GRATUIT (D8) ; il ne déclenche
// aucun appel et aucun paiement, juste une écriture journalisée.
func (s *Store) FixerCadrage(id string, cadrage map[string]any) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.Etat == EtatAbandonne || p.Etat == EtatInvalide || p.Etat == EtatTransmis {
		return nil, erreur(CodeActionInterdit, "cadrage impossible depuis "+p.Etat)
	}
	p.Cadrage = cadrage
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	if err := s.journaliser(p.ID, map[string]any{
		"type": "CADRAGE", "acteur": ActeurPilote,
		"gratuit": true, "instant_utc": maintenantUTC(),
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// FixerDevis : le devis est DÉRIVÉ de l'ordre qualifié (étapes, livrable,
// limites — jamais saisi à la main, D8).
func (s *Store) FixerDevis(id string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.Ordre == nil {
		return nil, erreur(CodeOrdreAbsent, "aucun ordre : le devis dérive de l'ordre (D8)")
	}
	etapes := []string{}
	for _, e := range p.Ordre.Etapes {
		etapes = append(etapes, e.Nom)
	}
	p.Devis = map[string]any{
		"etapes":   etapes,
		"livrable": p.Ordre.Livrable,
		"limites":  p.Ordre.Limites,
		"version":  p.Ordre.Identification.Version,
	}
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	if err := s.journaliser(p.ID, map[string]any{
		"type": "DEVIS", "acteur": ActeurPilote,
		"derive_de": "ordre", "instant_utc": maintenantUTC(),
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// AccorderDevis : décision explicite du pilote, REPRÉSENTÉE sans aucun
// paiement réel (D8 : l'accord est un acte de gouvernance locale).
func (s *Store) AccorderDevis(id, decision string) (*Projet, error) {
	v := s.verrou(id)
	v.Lock()
	defer v.Unlock()
	p, err := s.charger(id)
	if err != nil {
		return nil, err
	}
	if p.Devis == nil {
		return nil, erreur(CodeOrdreAbsent, "aucun devis établi : fixer le devis d'abord (D8)")
	}
	if strings.TrimSpace(decision) == "" {
		return nil, erreur(CodeJetonPiloteAbsent, "décision explicite requise (accord ou refus)")
	}
	p.AccordDevis = strings.EqualFold(strings.TrimSpace(decision), "ACCORD")
	if err := s.sauver(p); err != nil {
		return nil, err
	}
	if err := s.journaliser(p.ID, map[string]any{
		"type": "ACCORD_DEVIS", "acteur": ActeurPilote,
		"decision":      strings.ToUpper(strings.TrimSpace(decision)),
		"paiement_reel": false, "instant_utc": maintenantUTC(),
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// Journal : le journal complet du projet (preuves D10), brut.
func (s *Store) Journal(id string) ([]byte, error) {
	raw, err := os.ReadFile(s.cheminJournal(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, err
	}
	return raw, nil
}

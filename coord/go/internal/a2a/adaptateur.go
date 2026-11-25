// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Adaptateur A2A : PONT MINCE entre la batterie COORD et le MODULE RAIL
// AUTONOME coord/a2a (M-COORD-RAIL-01, T2 : le rail vit hors de coordengine,
// dans son propre go.mod ; ce fichier ne contient aucune logique de rail et
// importe uniquement le contrat public du module). L'adaptateur est
// REGISTERED comme tout adaptateur (adaptateurs.Adaptateur) : la machine
// normative, elle seule, décide des transitions. Les contrôles monétaires
// (EnveloppeCentimes/CoutConservateurJetons) sont PARTAGÉS avec les
// connecteurs HTTP (connecteurs, commun.go) : aucune règle dupliquée.
package a2a

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/connecteurs"

	coorda2a "github.com/dbouzon/osnias-clearing/coord/a2a"
)

// attenteMax : borne DURE de la boucle d'attente d'un résultat par appel
// (délégation + suivi) ; le contexte de l'orchestrateur reste PRIORITAIRE
// (l'arrêt pilote n'attend jamais le réseau).
const (
	attenteMax = 20 * time.Second
	attentePas = 25 * time.Millisecond
)

// AdaptateurA2A : délègue une action COORD (executer | soumettre |
// critiquer | recevoir) à un agent A2A via le rail générique.
type AdaptateurA2A struct {
	rail *coorda2a.Rail
	id   string
	sim  bool
	// Monétaire (voûte partagée) : tarifs et bornes DÉCLARÉS par le pilote.
	tarifEntreeMillion int
	tarifSortieMillion int
	tarifRequete       int
	jetonsEntreeMax    int
	jetonsSortieMax    int
}

// NouvelAdaptateurA2AJeton : construit l'adaptateur avec le jeton déjà lu
// par l'appelant depuis la variable d'environnement déclarée (jamais
// versionné, jamais le token console).
func NouvelAdaptateurA2AJeton(ctx context.Context, f connecteurs.ConfigFournisseur, jeton, cheminRegistre string) (*AdaptateurA2A, error) {
	if strings.TrimSpace(jeton) == "" {
		return nil, errors.New("a2a : APPEL_REEL_NON_AUTORISE : jeton d'agent absent (" +
			f.CleEnv + " non renseignée, fail closed)")
	}
	if f.TarifEntreeCentimesParMillion <= 0 || f.TarifSortieCentimesParMillion <= 0 ||
		f.TarifRequeteCentimes <= 0 || f.JetonsEntreeMaxParTentative <= 0 ||
		f.JetonsSortieMaxParTentative <= 0 {
		return nil, errors.New("a2a : TARIF_INCONNU : tarifs (entree/sortie/requete) et bornes " +
			"requis pour " + f.ID + " (voûte monétaire partagée, fail closed)")
	}
	rail, err := coorda2a.NouveauRail(ctx, f.Endpoint, jeton, cheminRegistre)
	if err != nil {
		return nil, err
	}
	return &AdaptateurA2A{
		rail:               rail,
		id:                 f.ID,
		sim:                true, // loopback imposé par NouveauRail dans cette tranche
		tarifEntreeMillion: f.TarifEntreeCentimesParMillion,
		tarifSortieMillion: f.TarifSortieCentimesParMillion,
		tarifRequete:       f.TarifRequeteCentimes,
		jetonsEntreeMax:    f.JetonsEntreeMaxParTentative,
		jetonsSortieMax:    f.JetonsSortieMaxParTentative,
	}, nil
}

// Rail expose le rail générique (réutilisable par d'autres adaptateurs).
func (a *AdaptateurA2A) Rail() *coorda2a.Rail { return a.rail }

// ID : identifiant opaque du fournisseur déclaré.
func (a *AdaptateurA2A) ID() string { return a.id }

// SimuleLocal : agent loopback, banc d'essai factice uniquement (0 €).
func (a *AdaptateurA2A) SimuleLocal() bool { return a.sim }

// cleIdempotence : clé SHA-256 de l'action (délégation unique, E3).
func (a *AdaptateurA2A) cleIdempotence(action, charge string) string {
	s := sha256.New()
	s.Write([]byte(a.id + "|" + action + "|" + charge))
	return hex.EncodeToString(s.Sum(nil))
}

// Executer implémente adaptateurs.Adaptateur.
func (a *AdaptateurA2A) Executer(mandat string) (adaptateurs.Resultat, error) {
	return a.ExecuterContexte(context.Background(), "executer", mandat,
		a.cleIdempotence("executer", mandat))
}

// Soumettre implémente adaptateurs.Adaptateur.
func (a *AdaptateurA2A) Soumettre(livrable string) (adaptateurs.Resultat, error) {
	return a.ExecuterContexte(context.Background(), "soumettre", livrable,
		a.cleIdempotence("soumettre", livrable))
}

// ExecuterContexte : délégation + suivi borné + résultat. Maîtrise totale
// du COORD sur l'agent : pas de retry fournisseur doublonnant une tâche
// incertaine ; état non terminal au bout de la borne => refus, la mission
// n'est jamais artificiallement "réussie".
func (a *AdaptateurA2A) ExecuterContexte(ctx context.Context, action, charge, cleIdempotence string) (adaptateurs.Resultat, error) {
	env, doublon, err := a.rail.Deleguer(ctx, action, charge, cleIdempotence)
	if err != nil {
		if doublon && env != nil {
			// E3 : rejet en doublon — jamais un second débit.
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : %w (tâche %s, action %s)",
				coorda2a.ErrDoublon, env.Tache, env.Action)
		}
		return adaptateurs.Resultat{}, err
	}
	// Résultat capturé à la délégation (registre durable) : usage direct.
	if e, ok := a.rail.ResultatDurable(env.Tache); ok {
		return adaptateurs.Resultat{Jetons: e.Jetons, Sortie: e.Texte, Simule: a.sim}, nil
	}
	// Suivi borné (le contexte pilote prime toujours ; tasks/get cadencé).
	// Sémantique : les états non terminaux ne PRODUISENT jamais de résultat ;
	// un achèvement asynchrone sans capture à l'envoi est REFUSÉ (données
	// non transmises => résolution PILOTE, jamais un résultat inventé).
	deadline := time.Now().Add(attenteMax)
	var derniere string
	for {
		etat, errS := a.rail.Suivre(ctx, env.Tache)
		if errS != nil {
			// Agent injoignable / tâche perdue : échec NET, tâche laissée
			// NON RÉSOLUE => décision PILOTE requise (aucune auto-reprise).
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : delegation incertaine (%s) : %v : "+
				"RÉSOLUTION PILOTE REQUISE (fail closed)", env.Tache, errS)
		}
		if etat == coorda2a.EtatAchevee {
			if e, ok := a.rail.ResultatDurable(env.Tache); ok {
				return adaptateurs.Resultat{Jetons: e.Jetons, Sortie: e.Texte, Simule: a.sim}, nil
			}
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : RESULTAT_NON_TRANSMIS %s : l'agent "+
				"n'a pas livré de résultat exploitable à l'envoi (fail closed, résolution pilote requise)",
				env.Tache)
		}
		if etat == coorda2a.EtatEchouee || etat == coorda2a.EtatAnnulee {
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : TACHE_TERMINALE_NEGATIVE %s (%s)",
				env.Tache, etat)
		}
		if etat == coorda2a.EtatSaisieAttendue {
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : SAISIE_ATTENDUE %s : l'agent "+
				"exige une entrée (aucune transmission automatique de mandat, fail closed)", env.Tache)
		}
		derniere = etat
		if ctx.Err() != nil {
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : appel annulé (%s, dernier état %s)",
				env.Tache, derniere)
		}
		if time.Now().After(deadline) {
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : BORNE_ATTEINTE %s (dernier état %s) : "+
				"tâche laissée au registre, réservation pilote requise", env.Tache, derniere)
		}
		select {
		case <-ctx.Done():
			return adaptateurs.Resultat{}, fmt.Errorf("a2a : appel annulé (%s)", env.Tache)
		case <-time.After(attentePas):
		}
	}
}

// EstimationCentimesSur : enveloppe conservatrice PARTAGÉE (connecteurs).
func (a *AdaptateurA2A) EstimationCentimesSur() (int, error) {
	return connecteurs.EnveloppeCentimes(a.tarifEntreeMillion, a.tarifSortieMillion,
		a.tarifRequete, a.jetonsEntreeMax, a.jetonsSortieMax, 0)
}

// CoutReelCentimes : coût conservateur À POSTERIORI, partagé.
func (a *AdaptateurA2A) CoutReelCentimes(jetonsTotal int) (int, error) {
	return connecteurs.CoutConservateurJetons(a.tarifEntreeMillion, a.tarifSortieMillion,
		a.tarifRequete, jetonsTotal)
}

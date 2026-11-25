// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package budget implements the pre-call budget control of COORD Pilot
// (conception §2.2, recette R3): per-mission, per-adapter counters, ceilings
// fixed at mission creation, verification BEFORE any adapter call, and a
// fail-closed default: an adapter without a ceiling is never allowed to call.
// Monetary control (M-COORD-PILOT-03, correction de la réserve 7154948522):
// ReserverCentimes reserves a DURABLE conservative monetary estimation, in
// integer euro cents, BEFORE any potentially billable attempt, against the
// mission-wide ceiling Plafonds.Centimes, and a reservation is NEVER released
// (late answer, failure, crash: the uncertainty is paid). This LOCAL
// conservative estimation is based on tariffs and bounds DECLARED by the
// pilot: it is NOT a provider-side spending guarantee, the ceiling binds
// only the bytes this software emits itself.
// The MVP is simulated: the token counts and the monetary indicator are
// declared by the simulated adapter; no real cost can be incurred here.
package budget

import (
	"fmt"
	"strings"
)

// Plafonds: ceilings fixed at mission creation. Fail-closed: an adapter
// absent from Appels has NO call authorised, whatever the counters say.
type Plafonds struct {
	Appels  map[string]int `json:"appels"`
	Jetons  map[string]int `json:"jetons"`
	Centime int            `json:"centimes_par_jeton_simule"`
	// Centimes : plafond monétaire GLOBAL de la mission, en centimes
	// d'euro ENTIERS (contre-réception 7154948522, M-COORD-PILOT-03). 0 ou
	// négatif = plafond NON ARMÉ : aucun adaptateur réel ne peut être appelé
	// (fail closed). Les adaptateurs simulés/loopback restent autorisés
	// (0 € par construction).
	Centimes int `json:"centimes"`
}

// Etat: consumed counters, per adapter.
type Etat struct {
	AppelsConsommes map[string]int `json:"appels_consommes"`
	JetonsConsommes map[string]int `json:"jetons_consommes"`
	// CentimesReserves : centimes d'euro RÉSERVÉS durablement par adaptateur
	// AVANT chaque tentative potentiellement facturable (M-COORD-PILOT-03,
	// correction de la réserve 7154948522). Une réservation n'est JAMAIS
	// libérée : l'échec d'un appel, une réponse tardive, un verdict non
	// conforme ou un crash conservent la réservation (l'incertitude est
	// payée, fail closed). ESTIMATION CONSERVATRICE LOCALE fondée sur les
	// tarifs et bornes DÉCLARÉS : ce n'est pas une garantie fournisseur.
	CentimesReserves map[string]int `json:"centimes_reserves"`
}

// Budget: ceilings plus consumed state, persisted with the mission record.
type Budget struct {
	Plafonds Plafonds `json:"plafonds"`
	Etat     Etat     `json:"etat"`
}

// Nouveau builds a Budget with initialised maps.
func Nouveau(plafonds Plafonds) *Budget {
	if plafonds.Appels == nil {
		plafonds.Appels = map[string]int{}
	}
	if plafonds.Jetons == nil {
		plafonds.Jetons = map[string]int{}
	}
	return &Budget{
		Plafonds: plafonds,
		Etat: Etat{
			AppelsConsommes:  map[string]int{},
			JetonsConsommes:  map[string]int{},
			CentimesReserves: map[string]int{},
		},
	}
}

// VerifierAvantAppel checks the budget BEFORE any adapter call (R3).
// Returns (false, motif) on refusal; refusals are explicit, never masked.
func (b *Budget) VerifierAvantAppel(adaptateur string, jetonsEstimes int) (bool, string) {
	plafondAppels, defini := b.Plafonds.Appels[adaptateur]
	if !defini || plafondAppels <= 0 {
		return false, "budget : aucun plafond d'appels defini pour l'adaptateur " +
			adaptateur + " : aucun appel autorise (fail closed)"
	}
	if b.Etat.AppelsConsommes == nil {
		b.Etat.AppelsConsommes = map[string]int{}
	}
	if b.Etat.JetonsConsommes == nil {
		b.Etat.JetonsConsommes = map[string]int{}
	}
	if b.Etat.AppelsConsommes[adaptateur]+1 > plafondAppels {
		return false, fmt.Sprintf("budget : plafond d'appels depasse pour %s (%d/%d)",
			adaptateur, b.Etat.AppelsConsommes[adaptateur], plafondAppels)
	}
	if plafondJetons, ok := b.Plafonds.Jetons[adaptateur]; ok && plafondJetons > 0 {
		if b.Etat.JetonsConsommes[adaptateur]+jetonsEstimes > plafondJetons {
			return false, fmt.Sprintf("budget : plafond de jetons depasse pour %s (%d+%d > %d)",
				adaptateur, b.Etat.JetonsConsommes[adaptateur], jetonsEstimes, plafondJetons)
		}
	}
	return true, ""
}

// Consommer records one adapter call AFTER it happened (never before the
// verification). Called only on successful adapter invocation.
func (b *Budget) Consommer(adaptateur string, jetons int) {
	if b.Etat.AppelsConsommes == nil {
		b.Etat.AppelsConsommes = map[string]int{}
	}
	if b.Etat.JetonsConsommes == nil {
		b.Etat.JetonsConsommes = map[string]int{}
	}
	b.Etat.AppelsConsommes[adaptateur]++
	b.Etat.JetonsConsommes[adaptateur] += jetons
}

// CoutSimule: simulated monetary indicator (cents), display only.
func (b *Budget) CoutSimule(adaptateur string) int {
	return b.Etat.JetonsConsommes[adaptateur] * b.Plafonds.Centime
}

// ResumeCompteurs returns a one-line summary for the UI and journals.
func (b *Budget) ResumeCompteurs() string {
	var parts []string
	for a := range b.Plafonds.Appels {
		parts = append(parts, fmt.Sprintf("%s: %d/%d appels", a,
			b.Etat.AppelsConsommes[a], b.Plafonds.Appels[a]))
	}
	return strings.Join(parts, "; ")
}

// AjusterJetons adjusts the consumed TOKEN counter of an adapter by a real
// surplus measured AFTER the call (M-COORD-PILOT-03, étape 4), WITHOUT
// counting an additional call: the call itself was already counted by the
// durable reservation of TX1. Fail closed: the counter can never go below
// zero. This method never authorises anything: ceilings still govern the
// pre-call verification (VerifierAvantAppel).
func (b *Budget) AjusterJetons(adaptateur string, deltaJetons int) {
	if b.Etat.JetonsConsommes == nil {
		b.Etat.JetonsConsommes = map[string]int{}
	}
	v := b.Etat.JetonsConsommes[adaptateur] + deltaJetons
	if v < 0 {
		v = 0
	}
	b.Etat.JetonsConsommes[adaptateur] = v
}

// ReserverCentimes réserve AVANT toute tentative potentiellement facturable
// une estimation monétaire DURABLE, en centimes d'euro entiers (correction
// de la réserve 7154948522 : le plafond monétaire est désormais OPPOSABLE
// AVANT l'appel, la dépense n'est plus ajoutée après engagement). Fail
// closed : aucun plafond armé => refus ; plafond global dépassé (toutes
// réservations confondues) => refus. La réservation est conservée en cas
// d'échec, de réponse tardive ou de crash : elle n'est jamais libérée
// (l'incertitude est payée).
func (b *Budget) ReserverCentimes(adaptateur string, centimes int) (bool, string) {
	if b.Etat.CentimesReserves == nil {
		b.Etat.CentimesReserves = map[string]int{}
	}
	if b.Plafonds.Centimes <= 0 {
		return false, "budget : aucun plafond monetaire arme pour la mission " +
			"(centimes <= 0) : aucun appel reel autorise (fail closed)"
	}
	if centimes <= 0 {
		return false, "budget : reservation monetaire non positive pour " +
			adaptateur + " : tarif ou borne inconnu (fail closed)"
	}
	if b.CentimesReservesTotal()+centimes > b.Plafonds.Centimes {
		return false, fmt.Sprintf("budget : plafond monetaire depasse (%d+%d > %d centimes)",
			b.CentimesReservesTotal(), centimes, b.Plafonds.Centimes)
	}
	b.Etat.CentimesReserves[adaptateur] += centimes
	return true, ""
}

// CentimesReservesTotal : total réservé sur la mission, tous adaptateurs.
func (b *Budget) CentimesReservesTotal() int {
	t := 0
	if b.Etat.CentimesReserves == nil {
		return 0
	}
	for _, v := range b.Etat.CentimesReserves {
		t += v
	}
	return t
}

// AjusterCentimes enregistre un SURCOÛT réel constaté après l'appel, par
// différence avec la réservation. CONSERVATEUR PAR CONSTRUCTION : un delta
// négatif ou nul est IGNORÉ — une réservation monétaire n'est jamais
// libérée, même si l'usage réel s'avère inférieur à l'enveloppe estimée
// (fail closed ; l'estimation locale ne doit JAMAIS devenir une promesse de
// libération). La méthode n'autorise rien : le plafond est opposé à
// l'émission par ReserverCentimes.
func (b *Budget) AjusterCentimes(adaptateur string, surcoutCentimes int) {
	if surcoutCentimes <= 0 {
		return
	}
	if b.Etat.CentimesReserves == nil {
		b.Etat.CentimesReserves = map[string]int{}
	}
	b.Etat.CentimesReserves[adaptateur] += surcoutCentimes
}

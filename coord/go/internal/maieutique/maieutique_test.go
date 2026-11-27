// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTableTransitionsExhaustive : chaque ligne du plan v2 D2 doit être
// dans la table, et toute action non listée doit être refusée.
func TestTableTransitionsExhaustive(t *testing.T) {
	cas := []struct {
		avant  string
		action string
		apres  string
	}{
		{EtatSansProjet, ActionDeclarerIntention, EtatIntention},
		{EtatIntention, ActionOuvrirDialogue, EtatDialogue},
		{EtatDialogue, ActionTourDirecteur, EtatDialogue},
		{EtatDialogue, ActionProposerOrdre, EtatOrdrePropose},
		{EtatOrdrePropose, ActionQualifier, EtatOrdreQualifie},
		{EtatOrdrePropose, ActionRetourDialogue, EtatDialogue},
		{EtatOrdreQualifie, ActionValiderPilote, EtatValide},
		{EtatOrdreQualifie, ActionRetourDialogue, EtatDialogue},
		{EtatValide, ActionPublier, EtatPublie},
		{EtatValide, ActionInvalider, EtatInvalide},
		{EtatPublie, ActionTransmettre, EtatTransmis},
		{EtatPublie, ActionPublier, EtatPublie},         // boucle propre idempotence
		{EtatPublie, ActionInvalider, EtatInvalide},     //
		{EtatTransmis, ActionTransmettre, EtatTransmis}, // boucle propre idempotence
		{EtatAttentePilote, ActionRelancer, EtatDialogue},
		{EtatAttentePilote, ActionRectifier, EtatDialogue},
		{EtatAttentePilote, ActionAbandonner, EtatAbandonne},
	}
	for _, c := range cas {
		cibles, ok := tableTransitions[c.avant]
		if !ok {
			t.Fatalf("état source absent de la table : %q", c.avant)
		}
		apres, ok := cibles[c.action]
		if !ok {
			t.Fatalf("action %q absente depuis %q", c.action, c.avant)
		}
		if apres != c.apres {
			t.Fatalf("%s --%s--> %s, attendu %s", c.avant, c.action, apres, c.apres)
		}
	}
	// Actions NON listées : refusées (échantillon).
	interdites := []struct {
		etat   string
		action string
	}{
		{EtatIntention, ActionTransmettre},   // pas de transmission prématurée
		{EtatDialogue, ActionQualifier},      // qualifier exige un ordre proposé
		{EtatOrdrePropose, ActionPublier},    // publier exige VALIDE
		{EtatAbandonne, ActionRelancer},      // ABANDONNE est terminal
		{EtatTransmis, ActionOuvrirDialogue}, // TRANSMIS n'accepte que transmettre
		{EtatInvalide, ActionRelancer},       // INVALIDE est terminal
	}
	for _, c := range interdites {
		if _, ok := tableTransitions[c.etat][c.action]; ok {
			t.Fatalf("action %q DOIT être interdite depuis %q", c.action, c.etat)
		}
	}
}

// TestDeclarerIntention : création, doublon refusé, intention vide
// refusée, intention.md écrit, état durable relisible.
func TestDeclarerIntention(t *testing.T) {
	store, _ := magasinTest(t)
	p, err := store.DeclarerIntention("projet-a", "Je veux que COORD m'aide")
	if err != nil {
		t.Fatalf("déclarer : %v", err)
	}
	if p.Etat != EtatIntention {
		t.Fatalf("état : %s", p.Etat)
	}
	relu, err := store.Obtenir("projet-a")
	if err != nil {
		t.Fatalf("relire : %v", err)
	}
	if relu.Intention != "Je veux que COORD m'aide" {
		t.Fatalf("intention relue : %q", relu.Intention)
	}
	b, err := os.ReadFile(filepath.Join(store.cfg.Racine, "projet-a", "intention.md"))
	if err != nil || !strings.Contains(string(b), "COORD") {
		t.Fatalf("intention.md : %v", err)
	}
	if _, err := store.DeclarerIntention("projet-a", "doublon"); !estErreurCode(err, CodeProjetExistant) {
		t.Fatalf("doublon attendu PROJET_EXISTANT, reçu : %v", err)
	}
	if _, err := store.DeclarerIntention("projet-b", "  "); !estErreurCode(err, CodeIntentionVide) {
		t.Fatalf("intention vide attendue INTENTION_VIDE, reçue : %v", err)
	}
}

// TestOuvrirDialogueBinome : binôme DISTINCTS exigé, inconnus refusés.
func TestOuvrirDialogueBinome(t *testing.T) {
	store, _ := magasinTest(t)
	if _, err := store.DeclarerIntention("b", "intention"); err != nil {
		t.Fatalf("déclarer : %v", err)
	}
	if _, err := store.OuvrirDialogue("b", "directeur-factice", "directeur-factice"); !estErreurCode(err, CodeBinomeIdentique) {
		t.Fatalf("identiques attendus BINOME_IDENTIQUE, reçu : %v", err)
	}
	if _, err := store.OuvrirDialogue("b", "inconnu", "exec-factice"); !estErreurCode(err, CodeDirecteurInconnu) {
		t.Fatalf("inconnu attendu DIRECTEUR_INCONNU, reçu : %v", err)
	}
	p, err := store.OuvrirDialogue("b", "directeur-factice", "exec-factice")
	if err != nil {
		t.Fatalf("ouvrir : %v", err)
	}
	if p.Etat != EtatDialogue || p.DirecteurID != "directeur-factice" {
		t.Fatalf("état/binôme : %s / %s", p.Etat, p.DirecteurID)
	}
}

// TestJournalTransitions : chaque transition journalisée porte acteur,
// action, états avant/après, horodatage (D10).
func TestJournalTransitions(t *testing.T) {
	store, _ := magasinTest(t)
	projetDialoguer(t, store, "intention de test")
	raw, err := store.Journal("temoin")
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	texte := string(raw)
	for _, attendu := range []string{
		`"action":"declarer_intention"`,
		`"action":"ouvrir_dialogue"`,
		`"acteur":"pilote-local"`,
		`"etat_avant":"INTENTION"`,
		`"etat_apres":"DIALOGUE"`,
		`"instant_utc"`,
		`"appels_directeur"`,
		`"contexte_octets"`,
	} {
		if !strings.Contains(texte, attendu) {
			t.Fatalf("journal sans %s :\n%s", attendu, texte)
		}
	}
}

// TestRelancerRectifierAbandonner : les trois sorties pilote depuis
// ATTENTE_PILOTE (D2), ABANDONNE terminal.
func TestRelancerRectifierAbandonner(t *testing.T) {
	store, f := magasinTest(t)
	f.indisponibilites(4) // deux tours, chacun échoue ×2 → ATTENTE_PILOTE
	projetDialoguer(t, store, "intention")
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); !estErreurCode(err, CodeIndisponibilite) {
		t.Fatalf("premier échec attendu INDISPONIBILITE_DIRECTEUR, reçu : %v", err)
	}
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); !estErreurCode(err, CodeIndisponibilite) {
		t.Fatalf("second échec attendu, reçu : %v", err)
	}
	p, err := store.Obtenir("temoin")
	if err != nil || p.Etat != EtatAttentePilote {
		t.Fatalf("ATTENTE_PILOTE attendu, reçu : %v / %s", err, p.Etat)
	}
	// Relancer : compteur remis, retour DIALOGUE.
	p, err = store.Relancer("temoin")
	if err != nil || p.Etat != EtatDialogue {
		t.Fatalf("relancer : %v / %s", err, p.Etat)
	}
	// Retour à ATTENTE_PILOTE pour éprouver RECTIFIER (licite uniquement
	// depuis ATTENTE_PILOTE, plan v2 D2).
	f.indisponibilites(2)
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); err == nil {
		t.Fatalf("échec attendu")
	}
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); err == nil {
		t.Fatalf("échec attendu")
	}
	if p, err = store.Obtenir("temoin"); err != nil || p.Etat != EtatAttentePilote {
		t.Fatalf("ATTENTE_PILOTE attendu : %v / %s", err, p.Etat)
	}
	// Rectifier : intention corrigée, retour DIALOGUE.
	p, err = store.Rectifier("temoin", "intention corrigée par le pilote")
	if err != nil || p.Etat != EtatDialogue || p.Intention != "intention corrigée par le pilote" {
		t.Fatalf("rectifier : %v / %s / %s", err, p.Etat, p.Intention)
	}
	// Retour à ATTENTE_PILOTE pour éprouver ABANDONNER.
	f.indisponibilites(2)
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); err == nil {
		t.Fatalf("échec attendu")
	}
	if _, _, err := store.TourDirecteur(contextBG(), "temoin", ""); err == nil {
		t.Fatalf("échec attendu")
	}
	// Abandonner : terminal propre, état durable conservé.
	p, err = store.Abandonner("temoin")
	if err != nil || p.Etat != EtatAbandonne {
		t.Fatalf("abandonner : %v / %s", err, p.Etat)
	}
	if _, err := store.Relancer("temoin"); !estErreurCode(err, CodeProjetTerminal) {
		t.Fatalf("ABANDONNE doit être terminal, reçu : %v", err)
	}
	// Les preuves restent lisibles.
	if _, err := store.Journal("temoin"); err != nil {
		t.Fatalf("journal après abandon : %v", err)
	}
}

// TestEnMapCanonique : l'état persisté est en forme canonique JCS
// (clés triées UTF-16, pas d'indentation) — persist.SaveAtomic passe
// TOUJOURS par enMap.
func TestEnMapCanonique(t *testing.T) {
	store, _ := magasinTest(t)
	projetDialoguer(t, store, "intention canonique")
	raw, err := os.ReadFile(filepath.Join(store.cfg.Racine, "temoin", "projet.json"))
	if err != nil {
		t.Fatalf("projet.json : %v", err)
	}
	if strings.Contains(string(raw), "\n  ") {
		t.Fatalf("projet.json ne doit pas être indenté (forme canonique) : %s", raw)
	}
	var m map[string]any
	if err := jsonDecode(raw, &m); err != nil {
		t.Fatalf("projet.json non JSON : %v", err)
	}
	// Les clés sont triées (ordre UTF-16 de RFC 8785).
	if !jsonEstCanonique(raw) {
		t.Fatalf("projet.json non canonique : %s", raw)
	}
}

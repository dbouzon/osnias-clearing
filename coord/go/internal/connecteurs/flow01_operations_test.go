// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/arret"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// opsDuBanc : lecture du registre d'opérations du banc (fail closed).
func opsDuBanc(t *testing.T, b *banc, issue string) []EvenementOperation {
	t.Helper()
	evs, err := LireOperations(b.ref.Racine(), issue)
	if err != nil {
		t.Fatalf("registre : %v", err)
	}
	return evs
}

func (b *banc) orchestratorReouvert(t *testing.T) *Orchestrateur {
	t.Helper()
	ref2, err := missions.Ouvrir(b.ref.Racine())
	if err != nil {
		t.Fatalf("réouverture : %v", err)
	}
	// vrais connecteurs (loopback) pointant sur les serveurs DU BANC : un
	// rejeu autorisé aboutirait à un appel réel vers le factice concerné.
	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral", Endpoint: b.exec.URL, Modele: "exec-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
		{ID: "openai", Endpoint: b.recep.URL, Modele: "recep-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
	}}
	exec2, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur executant rouvert : %v", err)
	}
	recep2, err := cfg.NouveauHTTPEndPoint("openai")
	if err != nil {
		t.Fatalf("connecteur réceptionnaire rouvert : %v", err)
	}
	orch2, err := NouvelOrchestrateur(ref2, map[string]adaptateurs.Adaptateur{
		"mistral": exec2, "openai": recep2,
	})
	if err != nil {
		t.Fatalf("orchestrateur rouvert : %v", err)
	}
	return orch2
}

func etatOperation(t *testing.T, b *banc, issue, id string) string {
	t.Helper()
	etats := etatsOperations(opsDuBanc(t, b, issue))
	st, ok := etats[id]
	if !ok {
		t.Fatalf("opération %s absente du registre", id)
	}
	return st.Etat
}

// idDerniereOuverte : id de la dernière opération OUVERTE du registre.
func idDerniereOuverte(t *testing.T, b *banc, issue string) string {
	t.Helper()
	id, ok := trouverOperationRejeu(opsDuBanc(t, b, issue), "derniere")
	if !ok {
		t.Fatal("aucune opération ouverte dans le registre")
	}
	return id
}

// D1/D4 : deux opérations VOLONTAIRES au contenu identique ont des identités
// DISTINCTES (mission+action+révision ne suffisent pas, ajustement pilote a).
func TestFlow01DeuxVolontairesContenuIdentiqueIdentitesDistinctes(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-volontaires"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	// deux opérations VOLONTAIRES au contenu IDENTIQUE (la critique de plan
	// ne transitionne pas : elle est répétable à volonté) : deux allers-
	// retours facturables distincts, aucun appel de fusion par empreinte.
	if _, err := b.orch.CritiquerPlan(ctx, issue, "openai-recep", "MEME PLAN"); err != nil {
		t.Fatalf("critique 1 : %v", err)
	}
	// même contenu, NOUVELLE opération volontaire : identité NEUVE (D4).
	if _, err := b.orch.CritiquerPlan(ctx, issue, "openai-recep", "MEME PLAN"); err != nil {
		t.Fatalf("critique 2 : %v", err)
	}
	etats := etatsOperations(opsDuBanc(t, b, issue))
	if len(etats) != 2 {
		t.Fatalf("deux identités distinctes attendues : %d", len(etats))
	}
	for id, st := range etats {
		if st.Etat != OperationTerminee {
			t.Fatalf("opération %s attendue TERMINEE : %s", id, st.Etat)
		}
	}
}

// D5 : rejeu d'une opération TERMINEE => OPERATION_DEJA_TERMINEE, zéro appel,
// zéro budget consommé ; le refus est journalisé (REJETEE, informationnel).
func TestFlow01RejeuOperationTermineeSansEffet(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-rejeu-terminee"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	appelsAvant := b.appelsExec
	enrAvant, _ := b.ref.Obtenir(issue)
	id := idDerniereOuverte(t, b, issue)
	orch2 := b.orchestratorReouvert(t).AvecRejeu(id)
	if _, err := orch2.Executer(ctx, issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "OPERATION_DEJA_TERMINEE") {
		t.Fatalf("OPERATION_DEJA_TERMINEE attendue : %v", err)
	}
	if b.appelsExec != appelsAvant {
		t.Fatalf("zéro appel attendu : %d -> %d", appelsAvant, b.appelsExec)
	}
	enrApres, _ := b.ref.Obtenir(issue)
	if enrApres.Budget.Etat.AppelsConsommes["mistral"] != enrAvant.Budget.Etat.AppelsConsommes["mistral"] {
		t.Fatal("DÉFAUT : budget modifié par un rejeu refusé")
	}
	// le refus est traçable (REJETEE, sans changement d'état de l'opération).
	trouve := false
	for _, ev := range opsDuBanc(t, b, issue) {
		if ev.Type == TypeOperationRejetee && ev.IdOperation == id {
			trouve = true
		}
	}
	if !trouve {
		t.Fatal("refus de rejeu non journalisé (REJETEE attendue)")
	}
	if etatOperation(t, b, issue, id) != OperationTerminee {
		t.Fatal("REJETEE ne doit JAMAIS changer l'état (informationnel)")
	}
}

// D5 : rejeu d'une opération ECHOUEE => relance avec la MÊME identité et une
// clé d'idempotence NEUVE (chaque tentative reste distincte, N-19 et #75).
func TestFlow01RejeuOperationEchoueeMemeIdCleNeuve(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-rejeu-echouee"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	// échec réel : adaptateur executant injoignable.
	b.exec.Close()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat"); err == nil {
		t.Fatal("échec d'appel attendu (serveur fermé)")
	}
	id := idDerniereOuverte(t, b, issue)
	if etatOperation(t, b, issue, id) != OperationEchouee {
		t.Fatalf("opération attendue ECHOUEE : %s", etatOperation(t, b, issue, id))
	}
	// rejeu explicite : MÊME id_operation, clé NEUVE.
	orch2 := b.orchestratorReouvert(t).AvecRejeu(id)
	res, err := orch2.Executer(ctx, issue, "mistral-exec", "mandat")
	if err == nil {
		t.Fatalf("le serveur factice reste fermé : échec attendu, obtenu %s", res.Sortie)
	}
	// clé NEUVE : la ligne APPEL_RESERVE du rejeu porte une autre clé.
	lignes := strings.Split(b.journal(t, issue), "\n")
	cles := map[string]bool{}
	for _, l := range lignes {
		if strings.Contains(l, "\"type\":\"APPEL_RESERVE\"") {
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("ligne illisible : %v", err)
			}
			idOp, _ := m["id_operation"].(string)
			if idOp == id {
				cle, _ := m["cle_idempotence"].(string)
				cles[cle] = true
			}
		}
	}
	if len(cles) != 2 {
		t.Fatalf("deux clés DISTINCTES attendues pour la même opération rejouée : %d", len(cles))
	}
	// la relance conserve la MÊME identité : une seule OUVERTE pour id.
	ouvertes := 0
	for _, ev := range opsDuBanc(t, b, issue) {
		if ev.Type == TypeOperationOuverte && ev.IdOperation == id {
			ouvertes++
		}
	}
	if ouvertes != 1 {
		t.Fatalf("la relance reprend la MÊME identité : 1 OUVERTE attendue, %d", ouvertes)
	}
}

// D5 : rejeu à charge divergente => OPERATION_INCOMPATIBLE AVANT tout appel.
func TestFlow01RejeuChargeDivergenteRefuseeAvantAppel(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-charge-divergente"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "MANDAT ORIGINAL"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	b.exec.Close()
	appelsAvant := b.appelsExec
	id := idDerniereOuverte(t, b, issue)
	orch2 := b.orchestratorReouvert(t).AvecRejeu(id)
	if _, err := orch2.Executer(ctx, issue, "mistral-exec", "AUTRE CHARGE"); err == nil ||
		!strings.Contains(err.Error(), "OPERATION_INCOMPATIBLE") {
		t.Fatalf("OPERATION_INCOMPATIBLE attendue : %v", err)
	}
	if b.appelsExec != appelsAvant {
		t.Fatal("DÉFAUT : un appel a eu lieu malgré l'incompatibilité")
	}
}

// D2 fail closed : un registre corrompu bloque TOUTE nouvelle opération
// (OPERATIONS_ILLISIBLES), aucun appel n'a lieu.
func TestFlow01RegistreCorrompuBloqueTout(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-corrompu"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	chemin := cheminOperations(b.ref.Racine(), issue)
	if err := os.WriteFile(chemin, []byte("{ligne illisible\n"), 0o600); err != nil {
		t.Fatalf("corruption : %v", err)
	}
	appelsAvant := b.appelsExec
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "OPERATIONS_ILLISIBLES") {
		t.Fatalf("OPERATIONS_ILLISIBLES attendue : %v", err)
	}
	if b.appelsExec != appelsAvant {
		t.Fatal("DÉFAUT : un appel a eu lieu malgré un registre illisible")
	}
	if _, err := ResumeOperations(b.ref.Racine(), issue); err == nil ||
		!strings.Contains(err.Error(), "OPERATIONS_ILLISIBLES") {
		t.Fatalf("-consulter doit signaler le registre illisible (fail closed) : %v", err)
	}
}

// D6 : mission ANCIENNE (appel EN_COURS sans identité, sans registre) :
// lisible, blocage inchangé, jamais d'appel automatique nouveau ; le rejeu
// est impossible (OPERATION_INCONNUE) ; la voie normale reste l'opération
// VOLONTAIRE après résolution pilote.
func TestFlow01CompatMissionAncienneSansRegistre(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-ancienne"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	// fabrication d'un état ancien : appel EN_COURS sans id, sans registre.
	dir := filepath.Dir(missions.JournalPath(b.ref.Racine(), issue))
	if err := os.WriteFile(filepath.Join(dir, "appel_en_cours.json"), []byte(
		`{"issue":"flow01-ancienne","action":"executer","empreinte_charge":"x",`+
			`"cle_idempotence":"k","etat":"EN_COURS","instant_utc":"2026-11-27T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("état ancien : %v", err)
	}
	if err := os.Remove(cheminOperations(b.ref.Racine(), issue)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("registre : %v", err)
	}
	// 1) l'appel ancien non résolu BLOQUE (comportement inchangé).
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_EN_COURS_NON_RESOLU") {
		t.Fatalf("blocage inchangé attendu : %v", err)
	}
	// 2) résolution pilote inchangée (aucune identité créée : l'état ancien
	// n'en porte pas et la résolution n'en fabrique jamais).
	if err := b.orch.ResoudreAppelEnCours(issue, "PERDU", "état ancien", ActeurPilote); err != nil {
		t.Fatalf("résolution : %v", err)
	}
	// 3) rejeu impossible sur mission ancienne sans identité (fail closed),
	// AVANT tout nouveau cycle.
	orch2 := b.orchestratorReouvert(t).AvecRejeu("derniere")
	if _, err := orch2.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "OPERATION_INCONNUE") {
		t.Fatalf("OPERATION_INCONNUE attendue sur mission ancienne : %v", err)
	}
	// 4) cycle possible : la voie normale de poursuite est la NOUVELLE
	// opération volontaire, jamais le rejeu.
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("cycle après résolution attendu : %v", err)
	}
}

// D2/D3/D7 : cycle complet — le registre retrouve TOUTES les opérations
// terminées (appel_en_cours.json ne les conserve pas), les compteurs sont
// séparés, et chaque ligne de journal corrélée porte id_operation.
func TestFlow01CycleCompletRegistreEtCorrelation(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-cycle"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	plan := "PLAN FLOW-01 : cycle complet."
	if _, err := b.orch.CritiquerPlan(ctx, issue, "openai-recep", plan); err != nil {
		t.Fatalf("critique : %v", err)
	}
	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "MANDAT")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if _, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie); err != nil {
		t.Fatalf("réception : %v", err)
	}
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("ack : %v", err)
	}
	if b.etat(t, issue) != spec.EtatACKRecu {
		t.Fatalf("état attendu ACK_RECU : %s", b.etat(t, issue))
	}
	st, err := ResumeOperations(b.ref.Racine(), issue)
	if err != nil {
		t.Fatalf("résumé : %v", err)
	}
	// appel_en_cours.json ne garde QUE le dernier appel ; le registre
	// retrouve les QUATRE opérations terminées.
	if st.Terminees != 4 || st.Echouees != 0 || st.Interrompues != 0 || st.EnCours != 0 {
		t.Fatalf("comptage attendu 4/0/0/0 : %+v", st)
	}
	ids := map[string]bool{}
	for _, e := range opsDuBanc(t, b, issue) {
		if e.Type == TypeOperationOuverte {
			ids[e.IdOperation] = true
		}
	}
	if len(ids) != 4 {
		t.Fatalf("quatre identités distinctes attendues : %d", len(ids))
	}
	// corrélation : chaque APPEL_RESERVE / APPEL_REEL du journal porte un
	// id_operation qui existe dans le registre.
	lignes := strings.Split(b.journal(t, issue), "\n")
	reserves, reels := 0, 0
	for _, l := range lignes {
		if strings.Contains(l, "\"type\":\"APPEL_RESERVE\"") {
			reserves++
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("ligne illisible : %v", err)
			}
			id, _ := m["id_operation"].(string)
			if !ids[id] {
				t.Fatalf("APPEL_RESERVE non corrélé à une opération du registre : %s", l)
			}
		}
		if strings.Contains(l, "\"type\":\"APPEL_REEL\"") {
			reels++
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("ligne illisible : %v", err)
			}
			id, _ := m["id_operation"].(string)
			if !ids[id] {
				t.Fatalf("APPEL_REEL non corrélé : %s", l)
			}
		}
	}
	if reserves != 4 || reels != 4 {
		t.Fatalf("quatre réserves et quatre appels attendus : %d/%d", reserves, reels)
	}
}

// D5/D7 : réponse tardive (arrêt pilote pendant l'appel) => opération
// INTERROMPUE dans le registre ; son rejeu est REFUSÉ (la réponse est
// possiblement arrivée sans commit : nouvelle opération requise, jamais
// un renvoi implicite).
func TestFlow01ReponseTardiveOperationInterrompueNonRejouable(t *testing.T) {
	b := nouveauBanc(t)
	issue := "flow01-tardive"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	// TX1 : réservation durable, opération OUVERTE, appel inscrit.
	b.bloquerExecMS = 150
	p, err := b.orch.preparer(issue, spec.EtatExecution, "executer", "mistral-exec", "mandat lent")
	if err != nil {
		t.Fatalf("préparation : %v", err)
	}
	// Arrêt pilote PENDANT l'appel : réponse tardive, aucune transition.
	b.arreter(t, issue, "arrêt d'essai pendant un appel réseau")
	res, err := b.orch.appelerReel(context.Background(), p)
	if err != nil {
		t.Fatalf("l'appel borné devait aboutir : %v", err)
	}
	if err := b.orch.committer(issue, spec.EtatExecution, "executer", p, res, nil, false); err == nil ||
		!strings.Contains(err.Error(), "REPONSE_TARDIVE") {
		t.Fatalf("réponse tardive attendue : %v", err)
	}
	idOp := p.idOperation
	if etatOperation(t, b, issue, idOp) != OperationInterrompue {
		t.Fatalf("opération attendue INTERROMPUE : %s", etatOperation(t, b, issue, idOp))
	}
	// Tant que l'appel orphelin n'est pas résolu : blocage existant.
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "mandat lent"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_EN_COURS_NON_RESOLU") {
		t.Fatalf("blocage anti-doublon attendu : %v", err)
	}
	// Résolution pilote (SIMULÉE, donnée de recette) : la réponse ÉTAIT
	// arrivée => RESOLU_RECU ; le rejeu de cette opération reste refusé
	// (aucun renvoi implicite, nouvelle opération requise).
	if err := b.orch.ResoudreAppelEnCours(issue, "RECU", "essai flow01 : réponse orpheline arrivée", ActeurPilote); err != nil {
		t.Fatalf("résolution : %v", err)
	}
	// Levée d'arrêt + reprise normative SIMULÉES (elles relèvent du moteur
	// N-16/N-17, pas de ce test) : le refus de rejeu doit subsister
	// INDÉPENDAMMENT de l'arrêt, car il tient à l'état INTERROMPUE/RESOLU_RECU.
	enr, errE := b.ref.Obtenir(issue)
	if errE != nil {
		t.Fatalf("obtenir : %v", errE)
	}
	enr.Arret = arret.Etat{}
	enr.RepriseAttendue = false
	if err := b.ref.Sauver(enr); err != nil {
		t.Fatalf("levée simulée : %v", err)
	}
	orch2 := b.orchestratorReouvert(t).AvecRejeu(idOp)
	if _, err := orch2.Executer(context.Background(), issue, "mistral-exec", "mandat lent"); err == nil ||
		!strings.Contains(err.Error(), "OPERATION_NON_REJOUABLE") {
		t.Fatalf("rejeu refusé attendu (résultat arrivé non persisté) : %v", err)
	}
	// La voie de poursuite est la NOUVELLE opération volontaire (D4).
}

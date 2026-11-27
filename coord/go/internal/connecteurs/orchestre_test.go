// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/arret"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// banc : deux serveurs factices loopback (exécutant, réceptionnaire), un
// référentiel temporaire, un orchestrateur réel sur connecteurs factices.
// HORS RÉSEAU, AUCUNE CLÉ, 0 €.
type banc struct {
	ref              *missions.Referentiel
	orch             *Orchestrateur
	exec             *httptest.Server
	recep            *httptest.Server
	appelsExec       int
	appelsRecep      int
	bloquerExecMS    int
	verdictPlan      string
	verdictReception string
	// pointsReception : JSON BRUT du champ points de la réponse du
	// réceptionnaire factice (revue PR #71, tour 3) : nil = "[]" (comportement
	// historique), chaîne vide = champ ABSENT, sinon inséré verbatim
	// (ex. "[null]", "null").
	pointsReception *string
}

func rolesTest() map[string][]string {
	return map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
}

func plafondsTest() budget.Plafonds {
	return budget.Plafonds{
		Appels: map[string]int{"mistral": 20, "openai": 20},
		Jetons: map[string]int{"mistral": 200000, "openai": 200000},
	}
}

func nouveauBanc(t *testing.T) *banc {
	t.Helper()
	b := &banc{}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	b.ref = ref

	b.exec = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.appelsExec++
		if b.appelsExec == 1 && b.bloquerExecMS > 0 {
			time.Sleep(time.Duration(b.bloquerExecMS) * time.Millisecond)
		}
		idem := r.Header.Get("Idempotency-Key")
		suffix := idem
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		w.Write(reponseChat("LIVRABLE-V"+suffix, "exec-modele", 1500))
	}))
	t.Cleanup(b.exec.Close)

	b.recep = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.appelsRecep++
		var corps corpsChat
		_ = json.NewDecoder(r.Body).Decode(&corps)
		contenu := corps.Messages[len(corps.Messages)-1].Content
		emp := extraireEmpreinte(t, contenu)
		if strings.Contains(contenu, "CRITIQUE DE PLAN") {
			verdict := b.verdictPlan
			if verdict == "" {
				verdict = "PLAN_ACCEPTE"
			}
			out, _ := json.Marshal(map[string]any{
				"verdict": verdict, "points": []string{"rien de bloquant"},
				"empreinte_attendue": emp,
			})
			w.Write(reponseChat(string(out), "recep-modele", 300))
			return
		}
		if strings.Contains(contenu, "RECEPTION INDEPENDANTE") {
			verdict := b.verdictReception
			if verdict == "" {
				verdict = "ACCEPTEE"
			}
			reponse := map[string]any{
				"verdict": verdict, "empreinte_attendue": emp,
				"verdict_id":  "verdict-" + r.Header.Get("Idempotency-Key")[:8],
				"verdict_url": "urn:test:reception",
			}
			switch {
			case b.pointsReception == nil:
				reponse["points"] = []string{}
			case *b.pointsReception != "":
				reponse["points"] = json.RawMessage(*b.pointsReception)
			}
			out, _ := json.Marshal(reponse)
			w.Write(reponseChat(string(out), "recep-modele", 300))
			return
		}
		w.Write(reponseChat("SOUMISSION COMPRISE", "recep-modele", 300))
	}))
	t.Cleanup(b.recep.Close)

	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral", Endpoint: b.exec.URL, Modele: "exec-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
		{ID: "openai", Endpoint: b.recep.URL, Modele: "recep-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
	}}
	exec, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur executant : %v", err)
	}
	recep, err := cfg.NouveauHTTPEndPoint("openai")
	if err != nil {
		t.Fatalf("connecteur réceptionnaire : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{
		"mistral": exec, "openai": recep,
	})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	b.orch = orch
	return b
}

func extraireEmpreinte(t *testing.T, contenu string) string {
	t.Helper()
	idx := strings.Index(contenu, `empreinte_attendue":"`)
	if idx < 0 {
		t.Fatalf("empreinte absente du prompt : %.80s", contenu)
	}
	reste := contenu[idx+len(`empreinte_attendue":"`):]
	fin := strings.Index(reste, `"`)
	if fin != 64 || len(reste) < 64 {
		t.Fatalf("empreinte non sha256 : %q", reste)
	}
	return reste[:64]
}

func (b *banc) etat(t *testing.T, issue string) string {
	t.Helper()
	enr, err := b.ref.Obtenir(issue)
	if err != nil {
		t.Fatalf("obtenir %s : %v", issue, err)
	}
	return enr.Mission.State
}

func (b *banc) journal(t *testing.T, issue string) string {
	t.Helper()
	raw, err := os.ReadFile(missions.JournalPath(b.ref.Racine(), issue))
	if err != nil {
		t.Fatalf("journal : %v", err)
	}
	return string(raw)
}

func (b *banc) arreter(t *testing.T, issue, motif string) {
	t.Helper()
	err := b.ref.Transaction(issue, func(c *missions.ContexteMission) error {
		return arret.Demander(&c.Enr.Arret, motif, ActeurPilote,
			missions.JournalPath(b.ref.Racine(), issue))
	}, nil)
	if err != nil {
		t.Fatalf("arrêt : %v", err)
	}
}

// ---- Cycle complet REEL (connecteurs factices loopback) ----

func TestCycleCompletCritiqueExecutionReceptionACK(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-cycle"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()

	// Échange plan/critique : verdict structuré ancré à l'empreinte du plan.
	plan := "PLAN v1 : produire un résumé de 3 lignes sur le protocole COORD."
	v, err := b.orch.CritiquerPlan(ctx, issue, "openai-recep", plan)
	if err != nil {
		t.Fatalf("critique de plan : %v", err)
	}
	if v.Verdict != "PLAN_ACCEPTE" {
		t.Fatalf("verdict inattendu : %+v", v)
	}
	if b.etat(t, issue) != spec.EtatPlanifiee {
		t.Fatalf("la critique de plan ne change pas l'état : %s", b.etat(t, issue))
	}

	// Exécution réelle par le connecteur executant.
	resExec, err := b.orch.Executer(ctx, issue, "mistral-exec",
		"Écris un résumé du protocole COORD en 3 lignes.")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if b.etat(t, issue) != spec.EtatExecution {
		t.Fatalf("état attendu EXECUTION : %s", b.etat(t, issue))
	}
	livrable := resExec.Sortie

	// Soumission : ancrage révision/empreinte calculé côté orchestrateur.
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", livrable); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("état attendu DECISION_ATTENDUE : %s", b.etat(t, issue))
	}

	// Réception indépendante : verdict structuré, ancré, validé.
	verdict, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", livrable)
	if err != nil {
		t.Fatalf("réception : %v", err)
	}
	if verdict.Verdict != "ACCEPTEE" || b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("réception inattendue : %+v / %s", verdict, b.etat(t, issue))
	}

	// ACK de l'exécutant : transition normative ancrée, AUCUN appel, 0 €.
	appelsAvant := b.appelsExec + b.appelsRecep
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("ack : %v", err)
	}
	if b.etat(t, issue) != spec.EtatACKRecu {
		t.Fatalf("état attendu ACK_RECU : %s", b.etat(t, issue))
	}
	if b.appelsExec+b.appelsRecep != appelsAvant {
		t.Fatalf("l'ACK ne consomme aucun appel réseau")
	}

	// Clôture : refusée par construction (aucune clôture automatique).
	if err := b.orch.Cloturer(issue); err == nil ||
		!strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
		t.Fatalf("clôture automatique interdite : %v", err)
	}
	if b.etat(t, issue) != spec.EtatACKRecu {
		t.Fatalf("l'état ne change pas après refus de clôture")
	}

	// Budget réellement comptabilisé (réservations + surplus réels).
	enr, _ := b.ref.Obtenir(issue)
	if enr.Budget.Etat.AppelsConsommes["mistral"] != 2 {
		t.Fatalf("2 appels executant attendus : %+v", enr.Budget.Etat)
	}
	if enr.Budget.Etat.AppelsConsommes["openai"] != 2 {
		t.Fatalf("2 appels réceptionnaire attendus : %+v", enr.Budget.Etat)
	}
	if strings.Contains(b.journal(t, issue), "sk-") {
		t.Fatalf("fuite de clé dans le journal")
	}
}

func TestBoucleDeCorrection(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-correction"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	b.verdictReception = "A_CORRIGER"
	ctx := context.Background()

	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat v1")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err != nil {
		t.Fatalf("soumission 1 : %v", err)
	}
	v1, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie)
	if err != nil {
		t.Fatalf("réception 1 : %v", err)
	}
	if v1.Verdict != "A_CORRIGER" || b.etat(t, issue) != spec.EtatEnCorrection {
		t.Fatalf("correction attendue : %+v / %s", v1, b.etat(t, issue))
	}

	// Corrections appliquées, puis nouveau cycle exécution -> réception.
	b.verdictReception = "ACCEPTEE"
	res2, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat v2 corrigé")
	if err != nil {
		t.Fatalf("exécution corrective : %v", err)
	}
	if b.etat(t, issue) != spec.EtatExecution {
		t.Fatalf("état attendu EXECUTION : %s", b.etat(t, issue))
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res2.Sortie); err != nil {
		t.Fatalf("soumission 2 : %v", err)
	}
	v2, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", res2.Sortie)
	if err != nil {
		t.Fatalf("réception 2 : %v", err)
	}
	if v2.Verdict != "ACCEPTEE" || b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("acceptation attendue : %+v / %s", v2, b.etat(t, issue))
	}
}

// ---- Réponse tardive : arrêt du pilote pendant l'appel réseau ----

func TestReponseTardiveArretPendantAppel(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-tardive"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	b.bloquerExecMS = 400

	// TX1 : réservation durable, appel inscrit (personne ne tient le verrou).
	p, err := b.orch.preparer(issue, spec.EtatExecution, "executer", "mistral-exec", "mandat lent")
	if err != nil {
		t.Fatalf("préparation : %v", err)
	}

	// Le pilote demande l'arrêt PENDANT l'appel : l'appel s'achève (délai
	// borné), la réponse arrive en retard : AUCUNE transition appliquée.
	b.arreter(t, issue, "arrêt d'essai pendant un appel réseau")

	res, err := b.orch.appelerReel(context.Background(), p)
	if err != nil {
		t.Fatalf("l'appel borné devait aboutir : %v", err)
	}
	err = b.orch.committer(issue, spec.EtatExecution, "executer", p, res, nil, false)
	if err == nil || !strings.Contains(err.Error(), "REPONSE_TARDIVE") {
		t.Fatalf("réponse tardive attendue : %v", err)
	}
	if b.etat(t, issue) != spec.EtatPlanifiee {
		t.Fatalf("aucune transition ne devait s'appliquer : %s", b.etat(t, issue))
	}
	if !strings.Contains(b.journal(t, issue), "REPONSE_TARDIVE") {
		t.Fatalf("journal sans REPONSE_TARDIVE")
	}

	// Anti-doublon : toute nouvelle tentative est refusée tant que l'appel
	// orphelin n'est pas résolu PAR LE PILOTE (fail closed).
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "reprise"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_EN_COURS_NON_RESOLU") {
		t.Fatalf("refus anti-doublon attendu : %v", err)
	}
	if err := b.orch.ResoudreAppelEnCours(issue, "PERDU",
		"essai tardive : réponse orpheline écartée", ActeurPilote); err != nil {
		t.Fatalf("résolution pilote : %v", err)
	}
	// La mission reste sous arrêt : l'exécution est refusée par l'arrêt.
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "reprise"); err == nil {
		t.Fatal("l'arrêt pilote doit bloquer l'exécution")
	}
}

// ---- Crash pendant l'appel : anti-doublon persistant ----

func TestCrashPendantAppelAntiDoublons(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-crash"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	// TX1 seule : le processus « meurt » avant l'appel.
	if _, err := b.orch.preparer(issue, spec.EtatExecution, "executer", "mistral-exec", "mandat"); err != nil {
		t.Fatalf("préparation : %v", err)
	}
	// Après « redémarrage » (nouveau référentiel + orchestrateur sur le
	// MÊME dossier), un renouvellement est refusé par défaut.
	ref2, err := missions.Ouvrir(b.ref.Racine())
	if err != nil {
		t.Fatalf("référentiel 2 : %v", err)
	}
	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{
		{ID: "mistral", Endpoint: b.exec.URL, Modele: "exec-modele", TimeoutMS: 5000,
			TailleMaxReponse: 65536, RetraitsMax: 0},
	}}
	execs, err := cfg.NouveauHTTPEndPoint("mistral")
	if err != nil {
		t.Fatalf("connecteur : %v", err)
	}
	orch2, err := NouvelOrchestrateur(ref2, map[string]adaptateurs.Adaptateur{"mistral": execs})
	if err != nil {
		t.Fatalf("orchestrateur 2 : %v", err)
	}
	if _, err := orch2.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_EN_COURS_NON_RESOLU") {
		t.Fatalf("anti-doublon attendu : %v", err)
	}
	if err := orch2.ResoudreAppelEnCours(issue, "PERDU", "crash simulé, réponse inconnue", ActeurPilote); err != nil {
		t.Fatalf("résolution : %v", err)
	}
	// Après résolution, une tentative est possible (nouvelle réservation).
	if _, err := orch2.Executer(context.Background(), issue, "mistral-exec", "mandat"); err != nil {
		t.Fatalf("reprise après décision pilote : %v", err)
	}
	if b.etat(t, issue) != spec.EtatExecution {
		t.Fatalf("état attendu EXECUTION : %s", b.etat(t, issue))
	}
	// Le budget compte bien DEUX appels réservés (crash + reprise), le
	// premier n'ayant jamais été remboursé : fail closed, incertitude payée.
	enr, _ := ref2.Obtenir(issue)
	if enr.Budget.Etat.AppelsConsommes["mistral"] != 2 {
		t.Fatalf("deux tentatives attendues au budget : %+v", enr.Budget.Etat)
	}
}

// ---- Habilitations et budget fail-closed ----

func TestActeurNonHabiliteAucunAppel(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-hab"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	// Le réceptionnaire tente d'exécuter : rôle EXECUTANT attendu.
	err := func() error {
		_, e := b.orch.Executer(context.Background(), issue, "openai-recep", "mandat")
		return e
	}()
	if err == nil {
		t.Fatal("un réceptionnaire ne peut pas exécuter")
	}
	if !strings.Contains(err.Error(), "validation normative") {
		t.Fatalf("motif normatif attendu : %v", err)
	}
	if b.appelsExec != 0 {
		t.Fatalf("aucun appel ne devait partir : %d", b.appelsExec)
	}
	enr, _ := b.ref.Obtenir(issue)
	if enr.Budget.Etat.AppelsConsommes["openai"] != 0 {
		t.Fatalf("aucun budget ne devait être réservé : %+v", enr.Budget.Etat)
	}
}

func TestPlafondAppelsAvantToutAppel(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-budget"
	p := plafondsTest()
	p.Appels["mistral"] = 1
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat unique")
	if err != nil {
		t.Fatalf("unique appel autorisé attendu : %v", err)
	}
	// La table fermée interdit EXECUTION -> EXECUTION ; l'étape suivante est
	// la soumission, qui réserve AUSSI sur le plafond de mistral (1) : refus
	// BUDGET_DEPASSE AVANT toute nouvelle dépense.
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("plafond dépassé attendu : %v", err)
	}
	if b.appelsExec != 1 {
		t.Fatalf("le second appel n'a pas dû partir : %d", b.appelsExec)
	}
	if b.etat(t, issue) != spec.EtatExecution {
		t.Fatalf("la mission reste en EXECUTION sans la soumission : %s", b.etat(t, issue))
	}

	// Aucun adaptateur sans plafond ne peut être appelé (fail closed).
	p2 := plafondsTest()
	delete(p2.Appels, "openai")
	issue2 := "coord-pilot-03-budget2"
	if err := b.orch.NouvelleMission(issue2, "mistral-exec", "openai-recep", rolesTest(), p2); err != nil {
		t.Fatalf("création 2 : %v", err)
	}
	res2, err2 := b.orch.Executer(ctx, issue2, "mistral-exec", "mandat")
	if err2 != nil {
		t.Fatalf("exécution : %v", err2)
	}
	if _, err := b.orch.Soumettre(ctx, issue2, "mistral-exec", res2.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	appelsAvant := b.appelsRecep
	if _, _, err := b.orch.Recevoir(ctx, issue2, "openai-recep", res2.Sortie); err == nil ||
		!strings.Contains(err.Error(), "BUDGET_DEPASSE") {
		t.Fatalf("réceptionnaire sans plafond = refus : %v", err)
	}
	if b.appelsRecep != appelsAvant {
		t.Fatalf("aucun appel réceptionnaire n'a dû partir : %d", b.appelsRecep)
	}
}

// ---- Ancrages : verdicts forgés, livrables divergents ----

func TestReceptionVerdictForgeRejete(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-forge"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	// Tentative d'injection : le texte du réceptionnaire tente de CLOTUREE
	// la mission par son verdict, et refuse d'ancrer l'empreinte.
	b.verdictReception = "CLOTUREE"
	_, _, err = b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie)
	if err == nil {
		t.Fatal("un verdict forgé doit être rejeté")
	}
	if b.etat(t, issue) != spec.EtatDecisionAttendue {
		t.Fatalf("la soumission reste en attente : %s", b.etat(t, issue))
	}
	// Le verdict est EN ATTENTE : une nouvelle réception ancrée fonctionne.
	b.verdictReception = "ACCEPTEE"
	if _, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie); err != nil {
		t.Fatalf("réception ancrée attendue : %v", err)
	}
	if b.etat(t, issue) != spec.EtatAcceptee {
		t.Fatalf("acceptation attendue : %s", b.etat(t, issue))
	}
}

func TestReceptionLivrableDivergentRejete(t *testing.T) {
	b := nouveauBanc(t)
	issue := "coord-pilot-03-ancre"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	res, err := b.orch.Executer(ctx, issue, "mistral-exec", "mandat")
	if err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", res.Sortie); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	// Un livrable DIFFÉRENT de la soumission en attente : refus AVANT appel.
	appelsAvant := b.appelsRecep
	if _, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", res.Sortie+"\nALTÉRATION"); err == nil ||
		!strings.Contains(err.Error(), "empreinte diverge") {
		t.Fatalf("ancrage attendu : %v", err)
	}
	if b.appelsRecep != appelsAvant {
		t.Fatalf("aucun appel n'a dû partir pour un livrable divergent")
	}
}

func TestVerdictPlanNonAncreRejete(t *testing.T) {
	if _, err := VerdictPlanDeSortie(
		`{"verdict":"PLAN_ACCEPTE","points":[],"empreinte_attendue":"f00f"}`,
		"aabbeeff00112233445566778899aabbccddeeff00112233445566778899aabbcc"); err == nil {
		t.Fatal("un verdict non ancré doit être rejeté (fail closed)")
	}
}

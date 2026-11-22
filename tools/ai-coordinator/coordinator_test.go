package aic

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestCoordinator construit un coordinateur simulé sur base temporaire.
func newTestCoordinator(t *testing.T) (*Coordinator, *SimulatedMistral, *SimulatedOpenAI, *SimulatedGitHub, *Server) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := ConfigFromEnv()
	cfg.DBPath = filepath.Join(dir, "test.sqlite")
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.WebhookSecret = "test-secret"
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.MistralTimeout = 20 * time.Second
	mistral := &SimulatedMistral{}
	openai := &SimulatedOpenAI{}
	gh := &SimulatedGitHub{}
	c := &Coordinator{Store: store, Config: cfg, Mistral: mistral, Reviewer: openai, GitHub: gh}
	srv := &Server{Coordinator: c, Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}
	return c, mistral, openai, gh, srv
}

// postComment publie un webhook issue_comment simulé, signé avec le secret.
func postComment(t *testing.T, s *Server, secret, deliveryID string, issue int, sender, body string, validSig bool) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"action":  "created",
		"issue":   map[string]any{"number": issue, "title": "Mission de test"},
		"comment": map[string]any{"body": body, "user": map[string]any{"login": sender}},
		"sender":  map[string]any{"login": sender},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload : %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !validSig {
		sig = "sha256=deadbeef"
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(raw))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-GitHub-Event", "issue_comment")
	rec := httptest.NewRecorder()
	s.HandleWebhook(rec, req)
	return rec
}

// waitFor attend qu'une condition devient vraie (traitements asynchrones).
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout : %s", msg)
}

// Scénarios T02-E : le cycle simulé complet, de bout en bout.
func TestT02EScenarios(t *testing.T) {
	c, mistral, openai, gh, srv := newTestCoordinator(t)
	_ = openai
	issue := 101
	id := MissionID(int64(issue))

	// 1-2. Webhook simulé : signature valide acceptée, invalide refusée.
	if rec := postComment(t, srv, c.Config.WebhookSecret, "d1", issue, "dbouzon", "/aic start", true); rec.Code != http.StatusOK {
		t.Fatalf("[1] webhook valide : code %d", rec.Code)
	}
	if rec := postComment(t, srv, c.Config.WebhookSecret, "dX", issue, "dbouzon", "/aic start", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("[2] webhook invalide : code %d, attendu 401", rec.Code)
	}

	// 3. Mission créée dans SQLite et cycle déroulé (4-6) jusqu'à la revue
	// favorable (rapport sans marqueur d'échec) : AWAITING_APPROVAL.
	waitFor(t, 5*time.Second, func() bool {
		m, _ := c.Store.MissionByID(id)
		return m != nil && m.State == StateAwaitingApproval
	}, "la mission n'atteint pas AWAITING_APPROVAL")
	m, err := c.Store.MissionByID(id)
	if err != nil || m == nil {
		t.Fatalf("[3] mission absente de SQLite : %v", err)
	}
	if mistral.Runs != 1 {
		t.Fatalf("[4] agent Mistral exécuté %d fois, attendu 1", mistral.Runs)
	}
	if len(gh.DraftPRs) != 1 {
		t.Fatalf("[5] livraison Draft absente : %v", gh.DraftPRs)
	}
	if openai.Reviews != 1 {
		t.Fatalf("[6] revue OpenAI absente : %d", openai.Reviews)
	}

	// 7. correction simulée : la seconde exécution (mission 2) livre un
	// rapport imparfait, la revue demande une correction.
	mistral.Fail = false
	correctionIssue := 102
	id2 := MissionID(int64(correctionIssue))
	// les runs sont indicés : run 1 = mission 1 (conforme), run 2 = mission 2 ([fail]),
	// run 3 = correction mission 2 (conforme).
	mistral.Reports = []string{"tranche exécutée : conforme", "première livraison imparfaite [fail]", "deuxième livraison conforme"}
	if rec := postComment(t, srv, c.Config.WebhookSecret, "d2", correctionIssue, "dbouzon", "/aic start", true); rec.Code != http.StatusOK {
		t.Fatalf("[7a] webhook : %d", rec.Code)
	}
	waitFor(t, 5*time.Second, func() bool {
		m, _ := c.Store.MissionByID(id2)
		return m != nil && m.State == StateCorrection
	}, "la mission 2 n'entre pas en CORRECTION")
	m2, _ := c.Store.MissionByID(id2)
	if m2.Corrections != 1 {
		t.Fatalf("[7b] corrections=%d, attendu 1", m2.Corrections)
	}
	if len(gh.ChangeRequests) != 1 {
		t.Fatalf("[7c] demande de correction non rattachée : %v", gh.ChangeRequests)
	}

	// 8. nouvelle soumission : correction relancée -> conforme -> AWAITING_APPROVAL.
	if rec := postComment(t, srv, c.Config.WebhookSecret, "d3", correctionIssue, "dbouzon", "/aic correction", true); rec.Code != http.StatusOK {
		t.Fatalf("[8a] webhook correction : %d", rec.Code)
	}
	waitFor(t, 5*time.Second, func() bool {
		m, _ := c.Store.MissionByID(id2)
		return m != nil && m.State == StateAwaitingApproval
	}, "la mission 2 n'atteint pas AWAITING_APPROVAL après correction")
	m2, _ = c.Store.MissionByID(id2)
	if mistral.Runs != 3 {
		t.Fatalf("[8b] ré-exécution Mistral attendue (3 au total), constatée %d", mistral.Runs)
	}

	// 9. approbation humaine : ACCEPTED.
	if rec := postComment(t, srv, c.Config.WebhookSecret, "d4", correctionIssue, "dbouzon", "/aic approve", true); rec.Code != http.StatusOK {
		t.Fatalf("[9a] webhook approbation : %d", rec.Code)
	}
	waitFor(t, 5*time.Second, func() bool {
		m, _ := c.Store.MissionByID(id2)
		return m != nil && m.State == StateAccepted
	}, "la mission 2 n'est pas ACCEPTED après approbation humaine")

	// 10. rejou du premier événement après redémarrage simulé : aucune double exécution.
	tr, dl, err := c.Store.Counts(id)
	if err != nil {
		t.Fatalf("[10a] counts : %v", err)
	}
	// même delivery ID rejoué : déduplication côté HTTP.
	if rec := postComment(t, srv, c.Config.WebhookSecret, "d1", issue, "dbouzon", "/aic start", true); rec.Code != http.StatusOK {
		t.Fatalf("[10b] rejou webhook : %d", rec.Code)
	}
	time.Sleep(300 * time.Millisecond) // goroutine éventuelle terminée
	if mistral.Runs != 3 {
		t.Fatalf("[10c] double exécution après rejou : runs=%d, attendu 3 (inchangé)", mistral.Runs)
	}
	tr2, dl2, _ := c.Store.Counts(id)
	if tr2 != tr || dl2 != dl {
		t.Fatalf("[10d] rejou a dupliqué des écritures : transitions %d->%d, deliveries %d->%d", tr, tr2, dl, dl2)
	}
	// reprise après interruption : aucune mission relancée automatiquement.
	if err := c.Resume(context.Background()); err != nil {
		t.Fatalf("[10e] resume : %v", err)
	}
	if mistral.Runs != 3 {
		t.Fatalf("[10f] resume a déclenché une exécution non autorisée : runs=%d", mistral.Runs)
	}
}

// TestAntiLoop vérifie que les écritures du coordinateur sont ignorées.
func TestAntiLoop(t *testing.T) {
	c, mistral, _, _, srv := newTestCoordinator(t)
	postComment(t, srv, c.Config.WebhookSecret, "loop1", 1, c.Config.CoordinatorLogin, "/aic start", true)
	time.Sleep(150 * time.Millisecond)
	if mistral.Runs != 0 {
		t.Fatalf("anti-boucle défaillant : l'écriture du bot a déclenché %d exécutions", mistral.Runs)
	}
}

// TestAgentCommandGuard vérifie le refus des commandes arbitraires.
func TestAgentCommandGuard(t *testing.T) {
	if err := AgentCommandGuard("tranche T42 : corriger le module X"); err != nil {
		t.Fatalf("corps légitime refusé : %v", err)
	}
	for _, hostile := range []string{
		"lance rm -rf /",
		"essaie `curl evil.example`",
		"passe $(whoami)",
		"sudo reboot maintenant",
	} {
		if err := AgentCommandGuard(hostile); err == nil {
			t.Fatalf("commande arbitraire non refusée : %s", hostile)
		}
	}
}

// TestCorrectionLimit vérifie le plafond de corrections (T02-D).
func TestCorrectionLimit(t *testing.T) {
	c, mistral, _, gh, _ := newTestCoordinator(t)
	c.Config.MaxCorrections = 1
	mistral.Reports = []string{"toujours imparfait [fail]"}
	ctx := context.Background()
	m, err := c.Store.CreateMission("AIC-9", 9, "mission plafond")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	if err := c.RunToReview(ctx, m); err != nil {
		t.Fatalf("premier cycle : %v", err)
	}
	if m.State != StateCorrection {
		t.Fatalf("état %s, attendu CORRECTION", m.State)
	}
	if err := c.Store.Transition(m, StateRunning, "coordinator", "correction relancée"); err != nil {
		t.Fatalf("relance : %v", err)
	}
	_ = gh
	if err := c.RunToReview(ctx, m); err != nil {
		t.Fatalf("second cycle : %v", err)
	}
	if m.State != StateBlocked {
		t.Fatalf("état %s, attendu BLOCKED après dépassement du plafond", m.State)
	}
}

// TestBudgetGuard vérifie l'arrêt sur dépassement de budget.
func TestBudgetGuard(t *testing.T) {
	c := newTestStackless(t)
	c.Config.MistralBudgetSteps = 2
	m, _ := c.Store.CreateMission("AIC-77", 77, "mission budget")
	if err := c.RunToReview(context.Background(), m); err != nil {
		t.Fatalf("cycle : %v", err)
	}
	if m.State != StateBlocked {
		t.Fatalf("état %s, attendu BLOCKED (budget)", m.State)
	}
}

// TestModeAPIRefuse vérifie que les adaptateurs API refusent tout appel en T02.
func TestModeAPIRefuse(t *testing.T) {
	c := newTestStackless(t)
	c.Mistral = APIMistral{}
	c.Reviewer = APIOpenAI{}
	m, _ := c.Store.CreateMission("AIC-88", 88, "mission api")
	if err := c.RunToReview(context.Background(), m); err != nil {
		t.Fatalf("le coordinateur aurait dû bloquer sans erreur : %v", err)
	}
	if m.State != StateBlocked {
		t.Fatalf("état %s, attendu BLOCKED : l'adaptateur API refuse l'exécution en T02", m.State)
	}
	// l'adaptateur OpenAI refuse également toute exécution en T02.
	out, err := APIOpenAI{}.Review(ReviewInput{})
	if err == nil || out.Decision != "" {
		t.Fatal("l'adaptateur API OpenAI aurait dû refuser l'exécution en T02")
	}
}

// TestStoreEvents vérifie la déduplication des événements directement.
func TestStoreEvents(t *testing.T) {
	store := newTestStore(t)
	fresh, err := store.SaveEvent("abc", "issue_comment", "{}")
	if err != nil || !fresh {
		t.Fatalf("premier enregistrement : fresh=%v err=%v", fresh, err)
	}
	fresh, _ = store.SaveEvent("abc", "issue_comment", "{}")
	if fresh {
		t.Fatal("rejou non détecté : la déduplication est défaillante")
	}
}

// TestReplayAfterRestart rejou un événement après réouverture de la base.
func TestReplayAfterRestart(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	f, _ := store.SaveEvent("d-ro", "issue_comment", "{}")
	if !f {
		t.Fatal("premier enregistrement attendu")
	}
	store.Close()
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	f2, _ := store2.SaveEvent("d-ro", "issue_comment", "{}")
	if f2 {
		t.Fatal("rejou non détecté après redémarrage : la déduplication n'est pas persistante")
	}
}

// helpers sans serveur.

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "s.sqlite"))
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestStackless(t *testing.T) *Coordinator {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := ConfigFromEnv()
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.WebhookSecret = "test-secret"
	return &Coordinator{
		Store: store, Config: cfg,
		Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{},
	}
}

// TestMissionIDStability vérifie la stabilité de l'identifiant de mission.
func TestMissionIDStability(t *testing.T) {
	if got := MissionID(42); got != "AIC-42" {
		t.Fatalf("identifiant instable : %s", got)
	}
}

// TestWorkDirIsolation vérifie l'isolation du répertoire de travail.
func TestWorkDirIsolation(t *testing.T) {
	c, mistral, _, _, _ := newTestCoordinator(t)
	m, _ := c.Store.CreateMission("AIC-5", 5, "m")
	if err := c.RunToReview(context.Background(), m); err != nil {
		t.Fatalf("cycle : %v", err)
	}
	workDir := filepath.Join(c.Config.WorkRoot, m.ID)
	if _, err := os.Stat(workDir); err != nil {
		t.Fatalf("répertoire de travail isolé absent : %v", err)
	}
	trace := fmt.Sprintf("%s/trace.txt", workDir)
	if _, err := os.Stat(trace); err != nil {
		t.Fatalf("trace d'exécution absente : %v", err)
	}
	if mistral.Runs != 1 {
		t.Fatalf("runs=%d", mistral.Runs)
	}
}

// --- Tranche T02-R1 : levée des réserves de robustesse et sécurité ---

// TestT02R1AuthorizationRefusal vérifie (réserve 1) qu'un auteur non habilité
// ne peut démarrer, corriger, valider ni bloquer une mission, même avec un
// webhook authentique (HMAC valide) : l'habilitation porte sur l'identité du
// sender authentifiée GitHub, distincte du HMAC.
func TestT02R1AuthorizationRefusal(t *testing.T) {
	c, mistral, _, gh, srv := newTestCoordinator(t)
	c.Config.AuthorizedLogins = []string{"alice"} // seul alice est habilité

	// non habilité : aucune mission créée, aucune exécution.
	rec := postComment(t, srv, c.Config.WebhookSecret, "a1", 200, "mallory", "/aic start", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook accepté attendu côté HTTP, code %d", rec.Code)
	}
	time.Sleep(200 * time.Millisecond)
	if m, _ := c.Store.MissionByID("AIC-200"); m != nil {
		t.Fatal("un auteur non habilité a créé une mission")
	}
	if mistral.Runs != 0 || len(gh.DraftPRs) != 0 {
		t.Fatalf("un auteur non habilité a déclenché une exécution : runs=%d drafts=%d", mistral.Runs, len(gh.DraftPRs))
	}

	// liste vide (fail closed) : même alice est refusée.
	c.Config.AuthorizedLogins = nil
	postComment(t, srv, c.Config.WebhookSecret, "a2", 201, "alice", "/aic start", true)
	time.Sleep(200 * time.Millisecond)
	if m, _ := c.Store.MissionByID("AIC-201"); m != nil {
		t.Fatal("politique fail closed contournée : mission créée sans habilitation listée")
	}

	// un auteur non habilité ne peut pas valider une mission en attente.
	c.Config.AuthorizedLogins = []string{"alice"}
	if err := newAuthorizedMission(t, c, "AIC-202"); err != nil {
		t.Fatalf("mission témoin : %v", err)
	}
	postComment(t, srv, c.Config.WebhookSecret, "a3", 202, "mallory", "/aic approve", true)
	time.Sleep(200 * time.Millisecond)
	m, _ := c.Store.MissionByID("AIC-202")
	if m.State != StateAwaitingApproval {
		t.Fatalf("approbation non habilitée acceptée : état %s", m.State)
	}
}

// newAuthorizedMission pousse une mission simulée jusqu'à AWAITING_APPROVAL.
func newAuthorizedMission(t *testing.T, c *Coordinator, id string) error {
	t.Helper()
	m, err := c.Store.CreateMission(id, 1, "mission témoin")
	if err != nil {
		return err
	}
	return c.RunToReview(context.Background(), m)
}

// TestT02R1DeliveryIDAndStorageAck vérifie (réserve 2) : identifiant de
// livraison vérifié, persistance AVANT acquittement, 503 sur échec réel de
// stockage sans assimilation à un doublon, reprise par rejou.
func TestT02R1DeliveryIDAndStorageAck(t *testing.T) {
	_, _, _, _, srv := newTestCoordinator(t) // srv partagé ci-dessous
	c := srv.Coordinator
	s := srv

	// identifiant de livraison absent : refus 400.
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(signBody(t, c.Config.WebhookSecret, []byte(`{}`))))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	rec := httptest.NewRecorder()
	s.HandleWebhook(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delivery ID absent : code %d, attendu 400", rec.Code)
	}

	// échec réel de stockage : 503, non assimilé à un doublon.
	dbPath := c.Config.DBPath
	_ = c.Store.Close()
	rec = postComment(t, s, c.Config.WebhookSecret, "st1", 300, "dbouzon", "/aic start", true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("échec de stockage : code %d, attendu 503", rec.Code)
	}
	// reprise : même identifiant rejoué une fois le stockage rétabli.
	store2, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	c.Store = store2
	defer store2.Close()
	fresh, err := store2.SaveEvent("st1", "issue_comment", "{}")
	if err != nil || !fresh {
		t.Fatalf("l'échec de stockage a été assimilé à un doublon : fresh=%v err=%v", fresh, err)
	}
}

// signBody est un utilitaire de signature HMAC pour les payloads bruts.
func signBody(t *testing.T, secret string, body []byte) []byte {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return mac.Sum(nil)
}

// TestT02R1ConcurrentExecutionExclusive vérifie (réserve 3) que deux webhooks
// simultanés visant la même mission ne provoquent ni double passage Mistral,
// ni double livraison : le verrou atomique persistant l'exclut formellement.
func TestT02R1ConcurrentExecutionExclusive(t *testing.T) {
	c, mistral, _, gh, _ := newTestCoordinator(t)
	m, err := c.Store.CreateMission("AIC-400", 400, "mission concurrence")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			errs <- c.RunToReview(context.Background(), m)
		}()
	}
	locked, ok := 0, 0
	for i := 0; i < n; i++ {
		if err := <-errs; err == ErrMissionLocked {
			locked++
		} else if err == nil {
			ok++
		} else {
			t.Fatalf("exécution concurrente : erreur inattendue %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("exécutions effectives=%d, attendu exactement 1", ok)
	}
	if locked != n-1 {
		t.Fatalf("verrou non exclusif : %d appels non rejetés", locked)
	}
	if mistral.Runs != 1 {
		t.Fatalf("agent Mistral exécuté %d fois, attendu 1", mistral.Runs)
	}
	if len(gh.DraftPRs) != 1 {
		t.Fatalf("livraisons=%d, attendu 1", len(gh.DraftPRs))
	}
	// une fois l'exécution terminée, le verrou est libéré : relance possible.
	if claim, err := c.Store.ClaimMission("AIC-400"); err != nil || !claim {
		t.Fatalf("verrou non relâché en fin d'exécution : claim=%v err=%v", claim, err)
	}
	_ = c.Store.ReleaseClaim("AIC-400")
}

// TestT02R1TransitionAtomicity vérifie (réserve 4) l'atomicité état/journal :
// en cas d'échec d'insertion du journal, la transaction est annulée et aucun
// état modifié ni écriture partielle ne subsiste.
func TestT02R1TransitionAtomicity(t *testing.T) {
	store := newTestStore(t)
	m, err := store.CreateMission("AIC-500", 500, "mission atomicité")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	// déclencheur qui fait échouer toute insertion dans transitions.
	if _, err := store.db.Exec(`CREATE TRIGGER fail_transitions BEFORE INSERT ON transitions BEGIN SELECT RAISE(FAIL, 'journal indisponible'); END`); err != nil {
		t.Fatalf("créer déclencheur : %v", err)
	}
	if err := store.Transition(m, StateRunning, "coordinator", "exécution"); err == nil {
		t.Fatal("transition réussie alors que le journal est indisponible : atomicité défaillante")
	}
	// aucun état modifié : la mission est inchangée dans la base.
	m2, err := store.MissionByID("AIC-500")
	if err != nil || m2 == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m2.State != StateOpen {
		t.Fatalf("transition partielle : état %s au lieu de OPEN", m2.State)
	}
	tr, _, _ := store.Counts("AIC-500") // la ligne de création est là, rien d'autre.
	if tr != 1 {
		t.Fatalf("journal incohérent : %d transitions au lieu de 1 (création)", tr)
	}
	// le vérrou d'exclusivité est lui aussi atomique et rejouable.
	fresh, err := store.ClaimMission("AIC-500")
	if err != nil || !fresh {
		t.Fatalf("premier claim : fresh=%v err=%v", fresh, err)
	}
	fresh, _ = store.ClaimMission("AIC-500")
	if fresh {
		t.Fatal("claim non exclusif")
	}
	// verrou récent : jamais repris tant qu'une exécution vivante le tient.
	if ok, err := store.ReclaimStaleClaim("AIC-500", 24*time.Hour); err != nil || ok {
		t.Fatalf("verrou récent ne doit pas être repris : ok=%v err=%v", ok, err)
	}
	// verrou périmé (âge > intervalle) : reprise possible après crash.
	if ok, err := store.ReclaimStaleClaim("AIC-500", time.Nanosecond); err != nil || !ok {
		t.Fatalf("verrou périmé doit être repris : ok=%v err=%v", ok, err)
	}
}

// TestT02R2ReclaimThenClaim vérifie (anomalie A de la revue T02-R2) que la
// reprise d'un verrou périmé supprime la ligne du verrou : le ClaimMission
// suivant doit réussir. La version antérieure actualisait claimed_at et
// l'INSERT OR IGNORE échouait ensuite silencieusement : mission bloquée.
func TestT02R2ReclaimThenClaim(t *testing.T) {
	store := newTestStore(t)
	ok, err := store.ClaimMission("AIC-600")
	if err != nil || !ok {
		t.Fatalf("premier claim : ok=%v err=%v", ok, err)
	}
	// verrou vivant : jamais repris.
	if ok, _ := store.ReclaimStaleClaim("AIC-600", 24*time.Hour); ok {
		t.Fatal("verrou vivant repris")
	}
	time.Sleep(2 * time.Millisecond)
	// verrou périmé : supprimé, puis re-claimable (le correctif).
	if ok, err := store.ReclaimStaleClaim("AIC-600", time.Millisecond); err != nil || !ok {
		t.Fatalf("verrou périmé doit être repris : ok=%v err=%v", ok, err)
	}
	ok, err = store.ClaimMission("AIC-600")
	if err != nil || !ok {
		t.Fatalf("claim après reprise impossible : mission resterait bloquée (ok=%v err=%v)", ok, err)
	}
}

// TestT02R2ResumeAfterCrash vérifie (anomalie A, bout en bout) qu'un crash
// pendant l'exécution laisse la mission RUNNING avec un verrou périmé, et
// que Resume au redémarrage débloque réellement la mission : elle progresse
// au lieu de rester RUNNING avec verrou éternellement actif.
func TestT02R2ResumeAfterCrash(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	m, err := store.CreateMission("AIC-601", 601, "mission crash")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	if err := store.Transition(m, StateRunning, "coordinator", "exécution interrompue (crash simulé)"); err != nil {
		t.Fatalf("passer en RUNNING : %v", err)
	}
	if ok, err := store.ClaimMission("AIC-601"); err != nil || !ok {
		t.Fatalf("claim : ok=%v err=%v", ok, err)
	}
	store.Close()

	// redémarrage : nouvelle base, même fichier.
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	cfg := ConfigFromEnv()
	cfg.DBPath = dbp
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.WebhookSecret = "test-secret"
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.RunStaleInterval = time.Nanosecond // verrou immédiatement périmé
	cfg.MistralTimeout = 20 * time.Second
	c := &Coordinator{Store: store2, Config: cfg, Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{}}
	if err := c.Resume(context.Background()); err != nil {
		t.Fatalf("resume : %v", err)
	}
	m2, err := store2.MissionByID("AIC-601")
	if err != nil || m2 == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m2.State == StateRunning {
		t.Fatal("mission encore RUNNING après reprise : le verrou actualisé bloquait la reprise (anomalie A)")
	}
	if m2.State == StateOpen || m2.State == StateBlocked {
		t.Fatalf("état inattendu après reprise : %s", m2.State)
	}
	_ = m // silence
}

// TestT02R2PendingEventQueue vérifie (anomalie B) qu'un événement acquitté
// (2xx) mais non traité au moment du crash est conservé avec son payload et
// retraité au redémarrage : la commande humaine n'est jamais perdue.
func TestT02R2PendingEventQueue(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	payload := `{"action":"created","issue":{"number":602,"title":"mission file"},"comment":{"body":"/aic start","user":{"login":"dbouzon"}},"sender":{"login":"dbouzon"}}`
	// crash simulé entre l'acquittement 2xx et le traitement asynchrone :
	// SaveEvent a eu lieu, MarkEventProcessed non.
	fresh, err := store.SaveEvent("d-t02r2", "issue_comment", payload)
	if err != nil || !fresh {
		t.Fatalf("save event : fresh=%v err=%v", fresh, err)
	}
	store.Close()

	// redémarrage : l'événement est bien dans la file d'attente persistante.
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	pending, err := store2.PendingEvents()
	if err != nil || len(pending) != 1 {
		t.Fatalf("file d'attente : %d événement(s), err=%v", len(pending), err)
	}
	if pending[0].DeliveryID != "d-t02r2" || pending[0].Payload != payload {
		t.Fatal("payload non restitué : la reprise serait impossible")
	}

	// dévidage au démarrage : la mission est créée puis exécutée.
	cfg := ConfigFromEnv()
	cfg.DBPath = dbp
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.WebhookSecret = "test-secret"
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.MistralTimeout = 20 * time.Second
	c := &Coordinator{Store: store2, Config: cfg, Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{}}
	DrainPendingEvents(context.Background(), c, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	m, err := store2.MissionByID("AIC-602")
	if err != nil || m == nil {
		t.Fatalf("mission perdue après redémarrage : %v (anomalie B)", err)
	}
	if m.State == StateOpen {
		t.Fatal("commande /aic start rejouée sans effet : mission toujours OPEN")
	}
	if m.State != StateReview && m.State != StateCorrection && m.State != StateAwaitingApproval && m.State != StateBlocked {
		t.Fatalf("état inattendu : %s", m.State)
	}
	// file vidée et cloturée : l'événement ne sera pas rejoué deux fois.
	pending, err = store2.PendingEvents()
	if err != nil || len(pending) != 0 {
		t.Fatalf("file d'attente non vidée : %d, err=%v", len(pending), err)
	}
}

// TestT02R2NoLossWhenEventNotCommand vérifie que la file d'attente ne
// s'engorge pas : un événement sans commande restante (action ignorée) est
// clos et marqué traité, pas conservé indéfiniment.
func TestT02R2NoLossWhenEventNotCommand(t *testing.T) {
	c, _, _, _, _ := newTestCoordinator(t)
	payload := `{"action":"edited","issue":{"number":603,"title":"x"},"comment":{"body":"hello","user":{"login":"dbouzon"}},"sender":{"login":"dbouzon"}}`
	fresh, err := c.Store.SaveEvent("d-ed", "issue_comment", payload)
	if err != nil || !fresh {
		t.Fatalf("save event : fresh=%v err=%v", fresh, err)
	}
	DrainPendingEvents(context.Background(), c, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	pending, err := c.Store.PendingEvents()
	if err != nil || len(pending) != 0 {
		t.Fatalf("événement non commandé doit être clos : %d, err=%v", len(pending), err)
	}
}

// TestT02R3GitHubOpStates vérifie les états de la table d'idempotence des
// écritures GitHub (risque 2) : pending, sending, done.
func TestT02R3GitHubOpStates(t *testing.T) {
	store := newTestStore(t)
	fresh, err := store.EnqueueGitHubOp("k1", "AIC-700", "comment")
	if err != nil || !fresh {
		t.Fatalf("enregistrement initial : fresh=%v err=%v", fresh, err)
	}
	if fresh, _ = store.EnqueueGitHubOp("k1", "AIC-700", "comment"); fresh {
		t.Fatal("clé d'idempotence non dédupliquée")
	}
	if st, _ := store.GitHubOpState("k1"); st != "pending" {
		t.Fatalf("état initial attendu pending, obtenu %s", st)
	}
	if err := store.MarkGitHubOpState("k1", "sending"); err != nil {
		t.Fatalf("mark sending : %v", err)
	}
	if st, _ := store.GitHubOpState("k1"); st != "sending" {
		t.Fatalf("état attendu sending, obtenu %s", st)
	}
	if err := store.MarkGitHubOpState("k1", "done"); err != nil {
		t.Fatalf("mark done : %v", err)
	}
	if st, _ := store.GitHubOpState("k1"); st != "done" {
		t.Fatalf("état attendu done, obtenu %s", st)
	}
	if st, _ := store.GitHubOpState("inconnue"); st != "" {
		t.Fatalf("clé inconnue doit retourner une chaîne vide, obtenu %s", st)
	}
}

// TestT02R3NoDuplicateComment vérifie (risque 2) qu'un commentaire GitHub
// réenvoyé après une interruption entre l'envoi et le marquage 'done' n'est
// jamais dupliqué : la même clé déduplique côté adaptateur.
func TestT02R3NoDuplicateComment(t *testing.T) {
	c, _, _, gh, _ := newTestCoordinator(t)
	key := "AIC-701:comment:review-ok"
	// premier envoi normal.
	if err := c.githubPostComment(key, "AIC-701", "réception favorable"); err != nil {
		t.Fatalf("premier envoi : %v", err)
	}
	// fenêtre de crash simulée : l'opération reste en état 'sending'.
	if err := c.Store.MarkGitHubOpState(key, "sending"); err != nil {
		t.Fatalf("réarmer état sending : %v", err)
	}
	// renvoi après interruption : même clé, l'adaptateur déduplique.
	if err := c.githubPostComment(key, "AIC-701", "réception favorable"); err != nil {
		t.Fatalf("renvoi : %v", err)
	}
	if n := len(gh.Comments); n != 1 {
		t.Fatalf("commentaire dupliqué : %d au lieu de 1", n)
	}
	if st, _ := c.Store.GitHubOpState(key); st != "done" {
		t.Fatalf("état final attendu done, obtenu %s", st)
	}
}

// TestT02R3DraftReused vérifie (risque 2) qu'une livraison Draft interrompue
// n'est jamais dupliquée : même clé, même référence, et une livraison déjà
// persistée est réutilisée sans nouveauPR.
func TestT02R3DraftReused(t *testing.T) {
	c, _, _, gh, _ := newTestCoordinator(t)
	m, err := c.Store.CreateMission("AIC-702", 702, "mission draft")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	ref1, err := c.githubDraft(m.ID+":draft", m.ID, "Livraison "+m.ID, "rapport")
	if err != nil || ref1 == "" {
		t.Fatalf("première livraison : ref=%q err=%v", ref1, err)
	}
	// fenêtre de crash : opération laissée en 'sending', puis renvoi.
	if err := c.Store.MarkGitHubOpState(m.ID+":draft", "sending"); err != nil {
		t.Fatalf("réarmer : %v", err)
	}
	ref2, err := c.githubDraft(m.ID+":draft", m.ID, "Livraison "+m.ID, "rapport")
	if err != nil || ref2 != ref1 {
		t.Fatalf("renvoi doit réutiliser la référence : %q vs %q, err=%v", ref2, ref1, err)
	}
	if n := len(gh.DraftPRs); n != 1 {
		t.Fatalf("livraison dupliquée : %d au lieu de 1", n)
	}
	// livraison persistée : réutilisée, aucune nouvelle création.
	if err := c.Store.SaveDraft(m.ID, ref1, "draft"); err != nil {
		t.Fatalf("save draft : %v", err)
	}
	ref3, err := c.Store.DraftForMission(m.ID)
	if err != nil || ref3 != ref1 {
		t.Fatalf("livraison persistée non restituée : %q vs %q, err=%v", ref3, ref1, err)
	}
}

// TestT02R3PendingRetryLoop vérifie (risque 1) que la boucle de relance
// périodique rejoue les événements en attente pendant le fonctionnement
// normal : un événement bloqué par un verrou n'attend plus le redémarrage.
func TestT02R3PendingRetryLoop(t *testing.T) {
	c, _, _, _, _ := newTestCoordinator(t)
	c.Config.PendingRetryInterval = 10 * time.Millisecond
	payload := `{"action":"created","issue":{"number":703,"title":"mission relance"},"comment":{"body":"/aic start","user":{"login":"dbouzon"}},"sender":{"login":"dbouzon"}}`
	if _, err := c.Store.SaveEvent("d-t02r3", "issue_comment", payload); err != nil {
		t.Fatalf("save event : %v", err)
	}
	// verrou détenu : les premiers passages de la boucle échouent
	// (ErrMissionLocked), l'événement reste en file d'attente.
	if ok, err := c.Store.ClaimMission("AIC-703"); err != nil || !ok {
		t.Fatalf("claim : ok=%v err=%v", ok, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartPendingRetryLoop(ctx, c, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	time.Sleep(60 * time.Millisecond)
	pending, err := c.Store.PendingEvents()
	if err != nil || len(pending) != 1 {
		t.Fatalf("l'événement doit rester en attente pendant le verrou : %d, err=%v", len(pending), err)
	}
	m, err := c.Store.MissionByID("AIC-703")
	if err != nil || m == nil {
		t.Fatalf("mission absente malgré l'événement : %v", err)
	}
	if m.State != StateOpen && m.State != StateRunning {
		t.Fatalf("état inattendu sous verrou : %s", m.State)
	}
	// libération du verrou : la boucle doit finir le travail à chaud.
	if err := c.Store.ReleaseClaim("AIC-703"); err != nil {
		t.Fatalf("release : %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, err = c.Store.PendingEvents()
		if err != nil {
			t.Fatalf("pending : %v", err)
		}
		if len(pending) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la boucle de relance n'a pas retraité l'événement après libération du verrou")
		}
		time.Sleep(5 * time.Millisecond)
	}
	m, err = c.Store.MissionByID("AIC-703")
	if err != nil || m == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m.State == StateOpen || m.State == StateRunning {
		t.Fatalf("la mission doit avoir progressé à chaud, état : %s", m.State)
	}
}

// TestT02R4DraftRefRecoveredAfterCrash vérifie (réserve T02-R4) qu'une
// interruption survenue après la création de la PR Draft mais avant son
// enregistrement dans deliveries ne perd pas la référence : la reprise (y
// compris après redémarrage complet, nouvel adaptateur vierge) restitue la
// référence depuis le détail durable de l'opération, sans rappeler GitHub.
func TestT02R4DraftRefRecoveredAfterCrash(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store1, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	gh1 := &SimulatedGitHub{}
	c1 := &Coordinator{Store: store1, Config: ConfigFromEnv(), Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh1}
	c1.Config.WebhookSecret = "test-secret"
	c1.Config.AuthorizedLogins = []string{"dbouzon"}
	if _, err := store1.CreateMission("AIC-800", 800, "mission reprise"); err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	// exécution interrompue : PR créée (opération done), SaveDraft non exécuté.
	ref1, err := c1.githubDraft("AIC-800:draft", "AIC-800", "Livraison AIC-800", "rapport")
	if err != nil || ref1 == "" {
		t.Fatalf("première livraison : ref=%q err=%v", ref1, err)
	}
	if ref, _ := store1.DraftForMission("AIC-800"); ref != "" {
		t.Fatal("pré-condition : la livraison ne doit pas encore être enregistrée")
	}
	store1.Close()

	// redémarrage complet : nouvelle base, même fichier, adaptateur vierge.
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	gh2 := &SimulatedGitHub{}
	gh2.UseKeyStore(store2)
	c2 := &Coordinator{Store: store2, Config: c1.Config, Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh2}
	ref2, err := c2.githubDraft("AIC-800:draft", "AIC-800", "Livraison AIC-800", "rapport")
	if err != nil {
		t.Fatalf("reprise : %v", err)
	}
	if ref2 != ref1 || ref2 == "" {
		t.Fatalf("référence non restituée après interruption : %q vs %q", ref2, ref1)
	}
	if n := len(gh2.DraftPRs); n != 0 {
		t.Fatalf("la reprise ne doit jamais rappeler GitHub : %d création(s)", n)
	}
	// la référence récupérée s'enregistre ensuite normalement.
	if err := store2.SaveDraft("AIC-800", ref2, "draft"); err != nil {
		t.Fatalf("save draft : %v", err)
	}
	if ref, _ := store2.DraftForMission("AIC-800"); ref != ref1 {
		t.Fatal("livraison enregistrée incohérente")
	}
}

// TestT02R4IdempotenceAfterRestart vérifie (réserve T02-R4) que la
// déduplication de l'adaptateur survit à un redémarrage complet du
// processus : un renvoi de commentaire sur une opération interrompue entre
// l'envoi et le marquage 'done' ne produit aucun doublon, l'adaptateur
// étant recréé vide et ne connaissant que le registre durable.
func TestT02R4IdempotenceAfterRestart(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store1, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	gh1 := &SimulatedGitHub{}
	gh1.UseKeyStore(store1)
	c1 := &Coordinator{Store: store1, Config: ConfigFromEnv(), Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh1}
	c1.Config.WebhookSecret = "test-secret"
	c1.Config.AuthorizedLogins = []string{"dbouzon"}
	if _, err := store1.CreateMission("AIC-801", 801, "mission redémarrage"); err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	key := "AIC-801:comment:review-ok"
	if err := c1.githubPostComment(key, "AIC-801", "réception favorable"); err != nil {
		t.Fatalf("premier envoi : %v", err)
	}
	if n := len(gh1.Comments); n != 1 {
		t.Fatalf("pré-condition : 1 commentaire attendu, %d", n)
	}
	// crash simulé entre l'envoi et le marquage 'done'.
	if err := store1.MarkGitHubOpState(key, "sending"); err != nil {
		t.Fatalf("réarmer : %v", err)
	}
	store1.Close()

	// redémarrage complet : registre mémoire vierge, seul le durable parle.
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	gh2 := &SimulatedGitHub{}
	gh2.UseKeyStore(store2)
	c2 := &Coordinator{Store: store2, Config: c1.Config, Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh2}
	if err := c2.githubPostComment(key, "AIC-801", "réception favorable"); err != nil {
		t.Fatalf("renvoi après redémarrage : %v", err)
	}
	if n := len(gh2.Comments); n != 0 {
		t.Fatalf("doublon après redémarrage : %d commentaire(s) de l'adaptateur vierge", n)
	}
	if st, _ := store2.GitHubOpState(key); st != "done" {
		t.Fatalf("opération restée %s après renvoi", st)
	}
}

// failingKeyStore est un KeyStore d'injection de panne (T02-R5) : il délègue
// à un KeyStore sain mais fait échouer à la demande les lectures (seen) et
// les écritures (mark) de clés.
type failingKeyStore struct {
	inner     KeyStore
	failSeen  bool
	failMark  bool
	markCalls int
}

func (f *failingKeyStore) GitHubSeenKey(idemKey string) (bool, error) {
	if f.failSeen {
		return false, errors.New("injection : échec de lecture du registre github_sent")
	}
	return f.inner.GitHubSeenKey(idemKey)
}

func (f *failingKeyStore) GitHubMarkKey(idemKey string) error {
	f.markCalls++
	if f.failMark {
		return errors.New("injection : échec d'écriture dans github_sent")
	}
	return f.inner.GitHubMarkKey(idemKey)
}

// TestT02R5PersistFailureNoEffect vérifie (T02-R5, point 2) qu'un échec de
// persistance de la clé n'est jamais assimilé à une exécution : le commentaire
// n'est pas produit, la mémoire n'est pas marquée, l'erreur est propagée ; le
// rejeu une fois la persistance revenue produit exactement un commentaire.
func TestT02R5PersistFailureNoEffect(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	defer store.Close()
	fks := &failingKeyStore{inner: store, failMark: true}
	gh := &SimulatedGitHub{}
	gh.UseKeyStore(fks)
	if err := gh.PostCommentIdem("AIC-900:comment", "AIC-900", "réception"); err == nil {
		t.Fatal("un échec de persistance doit produire une erreur")
	}
	if n := len(gh.Comments); n != 0 {
		t.Fatalf("aucun effet ne doit être produit : %d commentaire(s)", n)
	}
	seen, err := gh.seenKey("c:AIC-900:comment")
	if err != nil || seen {
		t.Fatalf("la clé mémoire ne doit pas être marquée après échec : seen=%v err=%v", seen, err)
	}
	// rejeu persistance saine : un seul commentaire au total.
	fks.failMark = false
	if err := gh.PostCommentIdem("AIC-900:comment", "AIC-900", "réception"); err != nil {
		t.Fatalf("rejeu : %v", err)
	}
	if err := gh.PostCommentIdem("AIC-900:comment", "AIC-900", "réception"); err != nil {
		t.Fatalf("renvoi : %v", err)
	}
	if n := len(gh.Comments); n != 1 {
		t.Fatalf("non-duplication attendue : %d commentaire(s)", n)
	}
}

// TestT02R5SeenKeyErrorPropagated vérifie (T02-R5, point 1) qu'une erreur
// SQLite en lecture du registre durable est propagée par les opérations
// idempotentes, sans produire d'effet.
func TestT02R5SeenKeyErrorPropagated(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	defer store.Close()
	fks := &failingKeyStore{inner: store, failSeen: true}
	gh := &SimulatedGitHub{}
	gh.UseKeyStore(fks)
	if err := gh.PostCommentIdem("AIC-901:comment", "AIC-901", "réception"); err == nil {
		t.Fatal("une erreur de lecture du registre doit être propagée")
	}
	if _, err := gh.CreateDraftPRIdem("AIC-901:draft", "AIC-901", "Livraison", "rapport"); err == nil {
		t.Fatal("une erreur de lecture du registre doit être propagée (draft)")
	}
	if err := gh.RequestChangesIdem("AIC-901:review", "PR-1", "à corriger"); err == nil {
		t.Fatal("une erreur de lecture du registre doit être propagée (review)")
	}
	if n := len(gh.Comments) + len(gh.DraftPRs) + len(gh.ChangeRequests); n != 0 {
		t.Fatalf("aucun effet ne doit être produit : %d", n)
	}
}

// TestT02R5DraftPersistFailureNoCreation vérifie (T02-R5, point 2) qu'un
// échec de persistance lors d'une PR Draft n'a jamais pour effet une PR
// simulée déjà créée mais non enregistrée : aucune duplication n'est possible
// au rejeu, même après redémarrage complet.
func TestT02R5DraftPersistFailureNoCreation(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	fks := &failingKeyStore{inner: store, failMark: true}
	gh := &SimulatedGitHub{}
	gh.UseKeyStore(fks)
	if _, err := gh.CreateDraftPRIdem("AIC-902:draft", "AIC-902", "Livraison", "rapport"); err == nil {
		t.Fatal("un échec de persistance doit produire une erreur (draft)")
	}
	if n := len(gh.DraftPRs); n != 0 {
		t.Fatalf("aucune PR simulée ne doit exister après échec de persistance : %d", n)
	}
	store.Close()

	// redémarrage complet : persistance saine, adaptateur recréé vierge.
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	gh2 := &SimulatedGitHub{}
	gh2.UseKeyStore(store2)
	ref, err := gh2.CreateDraftPRIdem("AIC-902:draft", "AIC-902", "Livraison", "rapport")
	if err != nil || ref == "" {
		t.Fatalf("rejeu : ref=%q err=%v", ref, err)
	}
	if _, err := gh2.CreateDraftPRIdem("AIC-902:draft", "AIC-902", "Livraison", "rapport"); err != nil {
		t.Fatalf("renvoi : %v", err)
	}
	if n := len(gh2.DraftPRs); n != 1 {
		t.Fatalf("non-duplication attendue : %d PR Draft(s)", n)
	}
}

// TestT02R5RestartNoDuplicationAnyOp vérifie (T02-R5, point 4) la
// non-duplication après redémarrage complet pour les trois types
// d'opérations, y compris le rejeu d'opérations interrompues entre la
// persistance de la clé et le marquage 'done' du registre github_ops.
func TestT02R5RestartNoDuplicationAnyOp(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store1, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	gh1 := &SimulatedGitHub{}
	gh1.UseKeyStore(store1)
	c1 := &Coordinator{Store: store1, Config: ConfigFromEnv(), Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh1}
	c1.Config.WebhookSecret = "test-secret"
	c1.Config.AuthorizedLogins = []string{"dbouzon"}
	if _, err := store1.CreateMission("AIC-903", 903, "mission t02-r5"); err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	kc := "AIC-903:comment:r5"
	kr := "AIC-903:review:r5"
	if err := c1.githubPostComment(kc, "AIC-903", "réception"); err != nil {
		t.Fatalf("commentaire : %v", err)
	}
	if _, err := c1.githubApply(kr, "AIC-903", "review", func() (string, error) {
		return "", gh1.RequestChangesIdem(kr, "PR-903", "corriger")
	}); err != nil {
		t.Fatalf("review : %v", err)
	}
	// opération interrompue : clé durable mais état non 'done'.
	if err := store1.MarkGitHubOpState(kc, "sending"); err != nil {
		t.Fatalf("réarmer : %v", err)
	}
	store1.Close()

	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	gh2 := &SimulatedGitHub{}
	gh2.UseKeyStore(store2)
	c2 := &Coordinator{Store: store2, Config: c1.Config, Mistral: &SimulatedMistral{}, Reviewer: &SimulatedOpenAI{}, GitHub: gh2}
	if err := c2.githubPostComment(kc, "AIC-903", "réception"); err != nil {
		t.Fatalf("renvoi commentaire après redémarrage : %v", err)
	}
	if _, err := c2.githubApply(kr, "AIC-903", "review", func() (string, error) {
		return "", gh2.RequestChangesIdem(kr, "PR-903", "corriger")
	}); err != nil {
		t.Fatalf("renvoi review après redémarrage : %v", err)
	}
	if n := len(gh2.Comments) + len(gh2.ChangeRequests); n != 0 {
		t.Fatalf("doublon après redémarrage : %d effet(s) de l'adaptateur vierge", n)
	}
	if st, _ := store2.GitHubOpState(kc); st != "done" {
		t.Fatalf("opération interrompue restée %s après renvoi", st)
	}
}

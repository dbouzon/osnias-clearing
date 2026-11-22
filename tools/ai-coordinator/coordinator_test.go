package aic

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	fresh, err := store.SaveEvent("abc", "issue_comment")
	if err != nil || !fresh {
		t.Fatalf("premier enregistrement : fresh=%v err=%v", fresh, err)
	}
	fresh, _ = store.SaveEvent("abc", "issue_comment")
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
	f, _ := store.SaveEvent("d-ro", "issue_comment")
	if !f {
		t.Fatal("premier enregistrement attendu")
	}
	store.Close()
	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	f2, _ := store2.SaveEvent("d-ro", "issue_comment")
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
	fresh, err := store2.SaveEvent("st1", "issue_comment")
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

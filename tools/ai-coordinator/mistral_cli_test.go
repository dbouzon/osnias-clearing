package aic

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newStubCLI écrit une fausse CLI Mistral Code : l'entrée standard est
// conservée dans prompt-received.txt et une sortie JSON structurée est
// émise sur la sortie standard. Aucun réseau, aucune commande arbitraire.
func newStubCLI(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub CLI de test non portable sous Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "mistral-code-stub")
	script = "#!/bin/sh\n" + script
	if err := os.WriteFile(bin, []byte(script), 0o750); err != nil {
		t.Fatalf("écrire stub CLI : %v", err)
	}
	return bin
}

// TestMAIC02RunContract vérifie le contrat d'invocation de l'adaptateur CLI :
// prompt transmis sur l'entrée standard, isolation du répertoire de travail,
// sortie JSON structurée décodée, trace conservée, journalisation branchée.
func TestMAIC02RunContract(t *testing.T) {
	bin := newStubCLI(t, `cat > prompt-received.txt
printf '%s' '{"report":"tranche exécutée via CLI","diff":"+5 -1","steps":7,"iters":2}'`)
	logs := map[string]bool{}
	adapter := &CLIMistral{Bin: bin, Args: []string{"--prompt"}, APIKey: "test-key", Log: func(missionID, phase, message string) {
		logs[phase] = true
		if strings.Contains(message, "test-key") {
			t.Errorf("la clé d'API est exposée dans le journal : %s", message)
		}
	}}
	m := &Mission{ID: "AIC-701", IssueNumber: 701, Title: "Mission de test CLI"}
	work := filepath.Join(t.TempDir(), "work")
	res, err := adapter.Run(context.Background(), m, work)
	if err != nil {
		t.Fatalf("run : %v", err)
	}
	if res.Report != "tranche exécutée via CLI" || res.Steps != 7 || res.Iters != 2 || res.Diff != "+5 -1" {
		t.Fatalf("résultat inattendu : %+v", res)
	}
	prompt, err := os.ReadFile(filepath.Join(work, "prompt-received.txt"))
	if err != nil {
		t.Fatalf("prompt reçu introuvable : %v", err)
	}
	if !strings.Contains(string(prompt), "AIC-701") || !strings.Contains(string(prompt), "Mission de test CLI") {
		t.Fatalf("prompt incomplet : %q", prompt)
	}
	trace, err := os.ReadFile(filepath.Join(work, "run-output.txt"))
	if err != nil || !strings.Contains(string(trace), "tranche exécutée via CLI") {
		t.Fatalf("trace de sortie manquante : %v %s", err, trace)
	}
	if !logs["start"] || !logs["done"] {
		t.Fatalf("journalisation incomplète : %v", logs)
	}
}

// TestMAIC02AuthFailClosed vérifie que l'absence de clé d'API refuse toute
// exécution (fail closed) et bloque la mission.
func TestMAIC02AuthFailClosed(t *testing.T) {
	c, _, _, _, _ := newTestCoordinator(t)
	c.Mistral = &CLIMistral{Bin: "vibe", APIKey: ""}
	if err := c.HandleIssueComment(context.Background(), 702, "mission sans clé", "dbouzon", "/aic start"); err != nil {
		t.Fatalf("commande : %v", err)
	}
	m, err := c.Store.MissionByID("AIC-702")
	if err != nil || m == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m.State != StateBlocked {
		t.Fatalf("la mission sans clé doit être BLOCKED, état : %s", m.State)
	}
}

// TestMAIC02FailureBlocksMission vérifie qu'un échec non nul de la CLI
// positionne la mission en BLOCKED avec un journal d'incident.
func TestMAIC02FailureBlocksMission(t *testing.T) {
	bin := newStubCLI(t, `printf '%s' '{"report":"tranche"}' >&2
exit 3`)
	store := newTestStore(t)
	cfg := ConfigFromEnv()
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.WorkRoot = filepath.Join(t.TempDir(), "work")
	cfg.MistralTimeout = 20 * time.Second
	c := &Coordinator{Store: store, Config: cfg, Mistral: &CLIMistral{
		Bin: bin, Args: []string{"--prompt"}, APIKey: "test-key",
		Log: func(missionID, phase, message string) { _ = store.Log(missionID, "mistral-code", phase, message) },
	}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{}}
	if err := c.HandleIssueComment(context.Background(), 703, "mission échec CLI", "dbouzon", "/aic start"); err != nil {
		t.Fatalf("commande : %v", err)
	}
	m, err := store.MissionByID("AIC-703")
	if err != nil || m.State != StateBlocked {
		t.Fatalf("mission attendue BLOCKED : %v %s", err, m.State)
	}
}

// TestMAIC02FullCycle vérifie le critère de réception : une commande humaine
// déclenche l'exécution par l'adaptateur CLI, le rapport est enregistré de
// façon durable (livraison Draft persistée + journal SQLite) et la mission
// atteint AWAITING_APPROVAL.
func TestMAIC02FullCycle(t *testing.T) {
	bin := newStubCLI(t, `cat > prompt-received.txt
printf '%s' '{"report":"rapport de tranche : tests passés, aucun push","diff":"+12 -0","steps":4,"iters":1}'`)
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	defer store.Close()
	cfg := ConfigFromEnv()
	cfg.DBPath = dbp
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.WebhookSecret = "test-secret"
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.MistralTimeout = 30 * time.Second
	c := &Coordinator{Store: store, Config: cfg, Mistral: &CLIMistral{
		Bin: bin, Args: []string{"--prompt"}, APIKey: "test-key",
		Log: func(missionID, phase, message string) { _ = store.Log(missionID, "mistral-code", phase, message) },
	}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{}}
	if err := c.HandleIssueComment(context.Background(), 704, "mission complète CLI", "dbouzon", "/aic start"); err != nil {
		t.Fatalf("commande : %v", err)
	}
	m, err := store.MissionByID("AIC-704")
	if err != nil || m == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m.State != StateAwaitingApproval {
		t.Fatalf("état attendu AWAITING_APPROVAL : %v %s", err, m.State)
	}
	prRef, err := store.DraftForMission("AIC-704")
	if err != nil || prRef == "" {
		t.Fatalf("livraison durable manquante : %v %q", err, prRef)
	}
	trans, deliveries, err := store.Counts("AIC-704")
	if err != nil || trans == 0 || deliveries == 0 {
		t.Fatalf("journal/livraisons vides : trans=%d deliveries=%d err=%v", trans, deliveries, err)
	}
}

// TestMAIC02ResumeAfterInterruption vérifie la reprise après interruption :
// une mission RUNNING au verrou périmé (crash simulé) est reprise par
// Resume et menée à son terme par l'adaptateur CLI, sans double livraison.
func TestMAIC02ResumeAfterInterruption(t *testing.T) {
	bin := newStubCLI(t, `cat > prompt-received.txt
printf '%s' '{"report":"rapport après reprise","steps":3,"iters":1}'`)
	dir := t.TempDir()
	dbp := filepath.Join(dir, "t.sqlite")
	store, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("ouvrir store : %v", err)
	}
	m, err := store.CreateMission("AIC-705", 705, "mission interrompue")
	if err != nil {
		t.Fatalf("créer mission : %v", err)
	}
	if err := store.Transition(m, StateRunning, "coordinator", "exécution interrompue (crash simulé)"); err != nil {
		t.Fatalf("passer en RUNNING : %v", err)
	}
	if ok, err := store.ClaimMission("AIC-705"); err != nil || !ok {
		t.Fatalf("claim : %v", err)
	}
	store.Close()

	store2, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir store : %v", err)
	}
	defer store2.Close()
	cfg := ConfigFromEnv()
	cfg.DBPath = dbp
	cfg.WorkRoot = filepath.Join(dir, "work")
	cfg.AuthorizedLogins = []string{"dbouzon"}
	cfg.RunStaleInterval = time.Nanosecond // verrou immédiatement périmé
	cfg.MistralTimeout = 30 * time.Second
	c := &Coordinator{Store: store2, Config: cfg, Mistral: &CLIMistral{
		Bin: bin, Args: []string{"--prompt"}, APIKey: "test-key",
		Log: func(missionID, phase, message string) { _ = store2.Log(missionID, "mistral-code", phase, message) },
	}, Reviewer: &SimulatedOpenAI{}, GitHub: &SimulatedGitHub{}}
	if err := c.Resume(context.Background()); err != nil {
		t.Fatalf("resume : %v", err)
	}
	m2, err := store2.MissionByID("AIC-705")
	if err != nil || m2 == nil {
		t.Fatalf("relire mission : %v", err)
	}
	if m2.State != StateAwaitingApproval && m2.State != StateReview {
		t.Fatalf("mission non reprise : %s", m2.State)
	}
	if _, err := store2.DraftForMission("AIC-705"); err != nil {
		t.Fatalf("livraison après reprise : %v", err)
	}
	// fermeture de la base et réouverture : le résultat reste durable.
	store3, err := OpenStore(dbp)
	if err != nil {
		t.Fatalf("réouvrir après fermeture : %v", err)
	}
	defer store3.Close()
	if ref, err := store3.DraftForMission("AIC-705"); err != nil || ref == "" {
		t.Fatalf("résultat non durable après réouverture : %v %q", err, ref)
	}
}

// TestMAIC02EnvConfig vérifie la lecture des variables dédiées à l'adaptateur
// CLI, dont les arguments par défaut du mode programmatique de Mistral Code.
func TestMAIC02EnvConfig(t *testing.T) {
	t.Setenv("AIC_MISTRAL_BIN", "/opt/vibe")
	t.Setenv("AIC_MISTRAL_ARGS", "--prompt, --trust, --max-turns, 5")
	t.Setenv("AIC_MISTRAL_API_KEY", "k-test")
	cfg := ConfigFromEnv()
	if cfg.MistralBin != "/opt/vibe" {
		t.Fatalf("AIC_MISTRAL_BIN non lu : %s", cfg.MistralBin)
	}
	if strings.Join(cfg.MistralArgs, ",") != "--prompt,--trust,--max-turns,5" {
		t.Fatalf("AIC_MISTRAL_ARGS non lu : %v", cfg.MistralArgs)
	}
	if cfg.MistralAPIKey != "k-test" {
		t.Fatal("AIC_MISTRAL_API_KEY non lu")
	}
	os.Unsetenv("AIC_MISTRAL_ARGS")
	os.Unsetenv("AIC_MISTRAL_BIN")
	cfg = ConfigFromEnv()
	if strings.Join(cfg.MistralArgs, ",") != "--prompt,--trust" {
		t.Fatalf("arguments par défaut attendus [--prompt --trust] : %v", cfg.MistralArgs)
	}
	if cfg.MistralBin != "vibe" {
		t.Fatalf("binaire par défaut attendu vibe : %s", cfg.MistralBin)
	}
}

// TestMAIC02CompatRealCLI vérifie la compatibilité T02 avec le mode
// programmatique de la CLI officielle Mistral Code (binaire vibe) : le shim
// enregistre ses arguments, le plafond --max-turns est ajouté depuis la
// configuration, le prompt transite sur stdin et le rapport est décodé.
func TestMAIC02CompatRealCLI(t *testing.T) {
	if bin := ConfigFromEnv().MistralBin; bin != "vibe" {
		t.Fatalf("binaire par défaut attendu vibe : %s", bin)
	}
	out := filepath.Join(t.TempDir(), "received-args.txt")
	shimDir := t.TempDir()
	shim := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "\ncat > /dev/null\nprintf '%s' '{\"report\":\"ok\"}'\n"
	if err := os.WriteFile(filepath.Join(shimDir, "vibe"), []byte(shim), 0o750); err != nil {
		t.Fatalf("écrire shim vibe : %v", err)
	}
	old := os.Getenv("PATH")
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+old)
	adapter := &CLIMistral{APIKey: "test-key", MaxTurns: 40} // Bin/Args vides : valeurs par défaut de l'adaptateur
	res, err := adapter.Run(context.Background(), &Mission{ID: "AIC-706", IssueNumber: 706, Title: "compat"}, filepath.Join(t.TempDir(), "w"))
	if err != nil {
		t.Fatalf("run : %v", err)
	}
	if res.Report != "ok" {
		t.Fatalf("rapport inattendu : %q", res.Report)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("arguments reçus introuvables : %v", err)
	}
	got := strings.Fields(string(raw))
	want := []string{"--prompt", "--trust", "--max-turns", "40"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("arguments attendus %v, reçus %v", want, got)
	}
}

// TestMAIC02ExitCodeInError vérifie que le code de sortie réel de la CLI est
// restitué dans le message d'incident (sémantique 0-3 de Mistral Code : 3 =
// arrêt avant achèvement, limite atteinte).
func TestMAIC02ExitCodeInError(t *testing.T) {
	bin := newStubCLI(t, `cat > /dev/null
printf '%s' 'limite atteinte' >&2
exit 3`)
	var logged string
	adapter := &CLIMistral{Bin: bin, Args: []string{"--prompt"}, APIKey: "k", Log: func(missionID, phase, message string) { logged = message }}
	_, err := adapter.Run(context.Background(), &Mission{ID: "AIC-707", IssueNumber: 707, Title: "exit"}, t.TempDir())
	if err == nil {
		t.Fatal("un échec était attendu")
	}
	if !strings.Contains(err.Error(), "exit=3") || !strings.Contains(logged, "exit=3") {
		t.Fatalf("code de sortie absent du message : %v / %s", err, logged)
	}
}

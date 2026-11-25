// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette du SUPERVISEUR DE SOUS-PROCESSUS (plan v2 §5) : S11 (codes
// 0/1/2/3), S12 (export absent/corrompu), S13 (approbation refusée
// TRACÉE sans effet de bord), S14 (SIGTERM coopératif), S15 (SIGKILL du
// GROUPE), S19/S19bis/S19ter (secrets, liste blanche, exfiltration),
// T-LONG (limite de durée), fail-closed des options. 100 % factice :
// aucune clé, aucun réseau, 0 €.
package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// binaireFactice : construit la CLI factice locale UNE fois par run.
var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func factice(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "runner-bin-")
		if err != nil {
			binErr = err
			return
		}
		ext := ""
		if filepath.Separator == '\\' {
			ext = ".exe"
		}
		p := filepath.Join(dir, "vibe-factice"+ext)
		out, err := exec.Command("go", "build", "-o", p,
			"github.com/dbouzon/osnias-clearing/coord/a2a-agent/cmd/vibe-factice").CombinedOutput()
		if err != nil {
			binErr = err
			t.Logf("go build : %s", out)
			return
		}
		binPath = p
	})
	if binErr != nil {
		t.Fatalf("binaire factice : %v", binErr)
	}
	return binPath
}

// lancer : options standards autour de la CLI factice.
func lancer(t *testing.T, ctx context.Context, outputDir string, args []string, mutate func(*Options)) (*Resultat, error) {
	t.Helper()
	dir := outputDir
	o := Options{
		Binaire: factice(t), Args: args, Repertoire: dir,
		LimiteDuree: 10 * time.Second, LimiteGrace: 2 * time.Second,
		ExportChemin: CheminExport(dir), SortieMax: 1 << 16,
	}
	if mutate != nil {
		mutate(&o)
	}
	return Lancer(ctx, o)
}

// argsBase : arguments de la CLI factice pour un run nominal + variantes.
func argsBase(t *testing.T, outputDir string, plus ...string) []string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mandat.txt")
	if err := os.WriteFile(p, []byte("TACHE factice de qualification"), 0o600); err != nil {
		t.Fatal(err)
	}
	return append([]string{
		"--output-dir", outputDir,
		"--worktree", "COORD-t-r1",
		"--prompt-file", p,
	}, plus...)
}

// S11 : traduction des codes de sortie, 100 sans export échoué, jamais
// de réussite sur 3.
func TestS11CodesDeSortie(t *testing.T) {
	for _, cas := range []struct {
		code int
	}{
		{0}, {1}, {2}, {3},
	} {
		dir := t.TempDir()
		res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-code", itoa(cas.code)),
			func(o *Options) { o.ExportChemin = CheminExport(dir) })
		if cas.code != 0 {
			if err == nil && res.ExportValide {
				t.Fatalf("code %d : aucune réussite attendue", cas.code)
			}
			continue
		}
		if err != nil || !res.ExportValide {
			t.Fatalf("code 0 nominal attendu : %v", err)
		}
	}
}

// S12 : code 0 AVEC export absent / corrompu = fail closed.
func TestS12ExportAbsentOuCorrompu(t *testing.T) {
	dir := t.TempDir()
	_, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-code-sec"), nil)
	if err == nil || !strings.Contains(err.Error(), "EXPORT_MANQUANT") {
		t.Fatalf("EXPORT_MANQUANT attendu : %v", err)
	}
	dir2 := t.TempDir()
	_, err = lancer(t, context.Background(), dir2, argsBase(t, dir2, "-factice-export", "corrompu"), nil)
	if err == nil || !strings.Contains(err.Error(), "EXPORT_CORROMPU") {
		t.Fatalf("EXPORT_CORROMPU attendu : %v", err)
	}
}

// S13 : approbation refusée en programmatique => refus TRACÉ dans les
// warnings de l'export, code 3, AUCUN effet de bord.
func TestS13ApprobationRefuseeTrace(t *testing.T) {
	dir := t.TempDir()
	res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-approbation"), nil)
	if err != nil {
		t.Fatalf("l'export doit être récolté même sur refus d'approbation : %v", err)
	}
	if res.Code != 3 {
		t.Fatalf("code 3 attendu (jamais une réussite), code %d", res.Code)
	}
	if !res.ExportPresent || !res.ExportValide {
		t.Fatalf("export exigé")
	}
	refus := false
	for _, w := range res.Export.Warnings {
		if strings.Contains(w, "APPROBATION_REFUSEE") {
			refus = true
		}
	}
	if !refus {
		t.Fatalf("refus d'approbation TRACÉ attendu dans warnings : %+v", res.Export.Warnings)
	}
}

// S14/T-LONG : limite de durée => SIGTERM coopératif, export cancel,
// code 3, durée inférieure à la tâche.
func TestS14LimiteSigtermCooperatif(t *testing.T) {
	// Windows : pas de SIGTERM ; taskkill /T /F force l'arrêt (limite
	// documentée plan v2 §3 T4). Sémantique coopérative explorée POSIX.
	if runtime.GOOS == "windows" {
		t.Skip("S14 : sémantique SIGTERM coopératif hors Windows")
	}
	dir := t.TempDir()
	debut := time.Now()
	res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-duree-ms", "3000"),
		func(o *Options) {
			o.LimiteDuree = 400 * time.Millisecond
			o.LimiteGrace = 2 * time.Second
		})
	if err != nil {
		t.Fatalf("export cancel exigé (arrêt coopératif) : %v", err)
	}
	if res.Code != 3 {
		t.Fatalf("code 3 attendu : %d", res.Code)
	}
	if res.Expulse {
		t.Fatalf("SIGKILL ne doit pas être nécessaire (arrêt coopératif)")
	}
	if time.Since(debut) > 2*time.Second {
		t.Fatalf("durée excessive : %s", time.Since(debut))
	}
	if res.ExportValide && res.Export.Outcome != "canceled" {
		t.Fatalf("outcome canceled attendu : %s", res.Export.Outcome)
	}
}

// S15 : SIGKILL du GROUPE après grâce écoulée (processus muet).
func TestS15SIGKILLDuGroupe(t *testing.T) {
	dir := t.TempDir()
	res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-duree-ms", "10000", "-factice-ignore-sigterm"),
		func(o *Options) {
			o.LimiteDuree = 300 * time.Millisecond
			o.LimiteGrace = 300 * time.Millisecond
		})
	if err == nil || !strings.Contains(err.Error(), "EXPORT_MANQUANT") {
		t.Fatalf("tâche tuée : export attendu absent (consommation inconnue = enveloppe totale) : %v", err)
	}
	if !res.Expulse {
		t.Fatalf("SIGKILL du groupe attendu")
	}
	if res.Code == 0 {
		t.Fatalf("jamais un code 0 après expulsion")
	}
}

// S19 : secret en sortie => EXPURGÉ, jamais propagé en clair.
func TestS19ExpurgationSorties(t *testing.T) {
	dir := t.TempDir()
	res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-export", "fuite"), nil)
	// L'export fautif est refusé (S19ter), MAIS les sorties collectées
	// doivent être expurgées.
	if err == nil || !strings.Contains(err.Error(), "SECRET_DANS_EXPORT") {
		t.Fatalf("SECRET_DANS_EXPORT attendu : %v", err)
	}
	for _, s := range []string{res.Stdout, res.Stderr} {
		if strings.Contains(s, "sk-FACTICEfuite1234567890XY") {
			t.Fatalf("secret propagé en clair : %q", s)
		}
	}
}

// S19bis/T-ENV : liste blanche d'environnement, AUCUNE variable héritée.
func TestS19bisEnvironnementListeBlanche(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_FACTICETOKEN1234567890xyz")
	t.Setenv("COORD_CLE_MISTRAL", "sk-facticeenv9876543210")
	dir := t.TempDir()
	res, err := lancer(t, context.Background(), dir, argsBase(t, dir, "-factice-env-dump"), nil)
	if err != nil {
		t.Fatalf("run nominal attendu : %v", err)
	}
	if strings.Contains(res.Stderr, "GITHUB_TOKEN") ||
		strings.Contains(res.Stderr, "ghp_FACTICETOKEN") ||
		strings.Contains(res.Stderr, "sk-facticeenv") {
		t.Fatalf("variable héritée traversée (liste blanche contournée) : %q", res.Stderr)
	}
}

// Fail closed : options manquantes.
func TestOptionsFailClosed(t *testing.T) {
	if _, err := Lancer(context.Background(), Options{}); err != ErrBinaireAbsent {
		t.Fatalf("BINAIRE_ABSENT attendu : %v", err)
	}
	dir := t.TempDir()
	if _, err := Lancer(context.Background(), Options{Binaire: factice(t), LimiteDuree: 0}); err != ErrLimiteAbsente {
		t.Fatalf("LIMITE_DUREE_ABSENTE attendu : %v", err)
	}
	if _, err := Lancer(context.Background(), Options{Binaire: filepath.Join(dir, "introuvable"), LimiteDuree: time.Second}); err == nil {
		t.Fatalf("BINAIRE_INTROUVABLE attendu")
	}
}

// Annulation par contexte (tasks/cancel côté agent) : même sémantique
// coopérative que la limite (S14 côté A2A).
func TestAnnulationContexteCooperative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("annulation coopérative : sémantique SIGTERM hors Windows")
	}
	dir := t.TempDir()
	ctx, annule := context.WithCancel(context.Background())
	apres := time.AfterFunc(300*time.Millisecond, annule)
	defer apres.Stop()
	res, err := lancer(t, ctx, dir, argsBase(t, dir, "-factice-duree-ms", "3000"),
		func(o *Options) { o.LimiteGrace = 2 * time.Second })
	if err != nil {
		t.Fatalf("export cancel exigé : %v", err)
	}
	if res.Code != 3 || res.Expulse {
		t.Fatalf("arrêt coopératif attendu : code=%d expulse=%v", res.Code, res.Expulse)
	}
	if !res.Annule {
		t.Fatalf("annulation par contexte attendue")
	}
}

// itoa : évite strconv pour la lisibilité des cas.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

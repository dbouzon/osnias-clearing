// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package runner : SUPERVISEUR DE SOUS-PROCESSUS GÉNÉRIQUE (M-COORD-LLM-03,
// plan v2 §2 : brique « ExecGo sous-processus » mutualisable de la note
// v1.1 §4.7). Réutilisable tel quel pour tout exécuteur CLI d'un autre
// fournisseur : lancement borné, groupe de processus, SIGTERM coopératif,
// SIGKILL du groupe, récolte export.json, traduction des codes de sortie
// documentés (0 fini / 1 démarrage impossible / 2 échec API/runtime /
// 3 arrêt avant achèvement — un code 3 ne vaut JAMAIS réussite).
//
// SÉCURITÉ (plan v2 §2-C) :
//   - l'environnement du sous-processus est CONSTRUIT PAR LISTE BLANCHE
//     (T-ENV/S19bis : aucune variable héritée ne traverse, donc aucun
//     identifiant GitHub ni clé fournisseur n'existe dans un run de
//     qualification) ;
//   - aucun secret passé en argument, jamais (visibles dans /proc) ;
//   - sorties (stdout/stderr/export.json) EXPURGÉES avant tout usage :
//     un pseudo-secret émis par un binaire est neutralisé ([EXPURGE])
//     ou fait échouer la récolte avec l'export (S19/S19ter, fail closed) ;
//   - GROUPE de processus : SIGTERM au groupe puis SIGKILL du groupe
//     (S14/S15) ; sous Windows, destruction arborescente taskkill /T /F
//     (équivalent documenté, pas de groupes POSIX).
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Codes de sortie documentés de la CLI en mode programmatique (note
// v1.1 §4.1) : toutautre code = ÉCHEC net non catégorisé.
const (
	CodeFin        = 0 // fini
	CodeDemarrage  = 1 // démarrage impossible (flag/config invalide, rejouable)
	CodeAPI        = 2 // échec API/runtime (échec net, décision pilote)
	CodeAvantAchev = 3 // arrêt avant achèvement (limite, arrêt, refus du modèle)
)

// Erreurs transportées (l'exécuteur appelant les préfixe A2A).
var (
	ErrExportManquant = errors.New("runner : EXPORT_MANQUANT : export.json absent (consommation INCONNUE = enveloppe TOTALE, fail closed)")
	ErrExportCorrompu = errors.New("runner : EXPORT_CORROMPU : export.json non JSON valide (fail closed)")
	ErrSecretExport   = errors.New("runner : SECRET_DANS_EXPORT : secret détecté dans export.json (fail closed)")
	ErrBinaireAbsent  = errors.New("runner : BINAIRE_ABSENT : chemin du binaire vide (fail closed)")
	ErrLimiteAbsente  = errors.New("runner : LIMITE_DUREE_ABSENTE : borne de durée exigée avant lancement (fail closed)")
	ErrBinaireIntrouv = errors.New("runner : BINAIRE_INTROUVABLE : le binaire n'existe pas (fail closed)")
)

// motifSecret : préfixe littéral + suite de n caractères d'une classe.
type motifSecret struct {
	prefixe string
	minimum int
	classe  func(rune) bool
}

// alnum : [0-9A-Za-z_-].
func alnum(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') || r == '_' || r == '-'
}

// alnumTiret : [0-9A-Za-z-] (sans underscore).
func alnumTiret(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') || r == '-'
}

// motifsSecrets contrôlés dans TOUTE sortie collectée (S19) : extension
// additive de celle du rail durci (#52 + AIza de #50) appliquée aux
// artefacts du run ; couvre aussi les jetons GitHub de déploiement
// (jamais présents en qualification : liste blanche d'environnement).
var motifsSecrets = []motifSecret{
	{prefixe: "sk-", minimum: 16, classe: alnum},
	{prefixe: "AIza", minimum: 35, classe: alnum},
	{prefixe: "xoxb-", minimum: 10, classe: alnumTiret},
	{prefixe: "ghp_", minimum: 20, classe: alnum},
	{prefixe: "gho_", minimum: 20, classe: alnum},
	{prefixe: "ghu_", minimum: 20, classe: alnum},
	{prefixe: "ghs_", minimum: 20, classe: alnum},
	{prefixe: "ghr_", minimum: 20, classe: alnum},
}

// contientSecretMulti : itère tous les motifs.
func expurgerMotif(s string, m motifSecret, remplace bool) string {
	repl := "\x00"
	if remplace {
		repl = "[EXPURGE]"
	}
	runes := []rune(s)
	p := []rune(m.prefixe)
	for k := 0; k+len(p) <= len(runes); k++ {
		if !egaleRunes(runes[k:k+len(p)], p) {
			continue
		}
		q := k + len(p)
		n := 0
		for q < len(runes) && m.classe(runes[q]) {
			n++
			q++
		}
		if n < m.minimum {
			continue
		}
		return string(runes[:k]) + repl + expurgerMotif(string(runes[q:]), m, remplace)
	}
	return s
}

// contientSecretMulti : itère tous les motifs.
func contientSecretMulti(s string) bool {
	for _, m := range motifsSecrets {
		if expurgerMotif(s, m, false) != s {
			return true
		}
	}
	return false
}

// expurger : neutralise tout motif connu, récursivement sûr.
func expurger(s string) string {
	if !contientSecretMulti(s) {
		return s
	}
	out := s
	for _, m := range motifsSecrets {
		out = expurgerMotif(out, m, true)
	}
	return out
}

// egaleRunes : comparaison directe.
func egaleRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Options de lancement : toutes bornées, aucune valeur cachée.
type Options struct {
	Binaire      string        // chemin du CLI (factice en qualification)
	Args         []string      // arguments (données, jamais de secret)
	Repertoire   string        // répertoire de travail (dépôt de test local SANS remote)
	Env          []string      // LISTE BLANCHE exclusive "K=V" (jamais héritée)
	LimiteDuree  time.Duration // borne DURE exigée (jamais zéro, fail closed)
	LimiteGrace  time.Duration // délai SIGTERM->SIGKILL (défaut appliqué si zéro)
	ExportChemin string        // export.json attendu (vide = non exigé)
	SortieMax    int64         // plafond octets collectés stdout+stderr
}

// ExportVibe : sous-ensemble EXPURGÉ du export.json documenté (clé de
// comptabilité du plan v2 : session, issue, consommation réelle relevée).
type ExportVibe struct {
	SessionID        string   `json:"session_id"`
	Outcome          string   `json:"outcome"`
	TokensPrompt     int      `json:"tokens_prompt"`
	TokensCompletion int      `json:"tokens_completion"`
	Warnings         []string `json:"warnings"`
}

// Resultat : bilan du run. TROIS sources de vérité SEPARÉES : la sortie
// JSON du processus (Stdout), le export.json (Export), l'état du
// répertoire de travail (à la charge de l'exécuteur appelant). Un code
// 0 ne vaut JAMAIS réception (seule la chaîne COORD empreinte/réception
// décide), plan v2.
type Resultat struct {
	Code          int
	Expulse       bool   // SIGKILL du groupe (limite de durée dépassée)
	Annule        bool   // terminé suite à l'annulation du contexte
	TermineAvant  bool   // le processus s'est terminé seul
	Stdout        string // expurgé, tronqué
	Stderr        string // expurgé, tronqué
	ExportPresent bool
	ExportValide  bool
	Export        ExportVibe // valable seulement si ExportValide
	Duree         time.Duration
}

// Lancer : supervise le sous-processus selon les options. Le contexte
// prime TOUJOURS (SIGTERM coopératif puis SIGKILL du GROUPE). Aucun
// retry, aucune relance automatique : une tâche incertaine reste à la
// décision du PILOTE (plan v2, A-granularité, SEC-01).
func Lancer(ctx context.Context, o Options) (*Resultat, error) {
	if o.Binaire == "" {
		return nil, ErrBinaireAbsent
	}
	if o.LimiteDuree <= 0 {
		return nil, ErrLimiteAbsente
	}
	if _, err := os.Stat(o.Binaire); err != nil {
		return nil, fmt.Errorf("%w : %s", ErrBinaireIntrouv, o.Binaire)
	}
	grace := o.LimiteGrace
	if grace <= 0 {
		grace = 5 * time.Second
	}
	sortieMax := o.SortieMax
	if sortieMax <= 0 {
		sortieMax = 1 << 20
	}

	cmd := exec.Command(o.Binaire, o.Args...)
	if o.Repertoire != "" {
		cmd.Dir = o.Repertoire
	}
	// LISTE BLANCHE exclusive (T-ENV) : ATTENTION, cmd.Env == nil
	// signifierait « hériter de TOUT l'environnement » — on pose donc
	// une liste minimale NON VIDE par construction (aucun identifiant
	// GitHub, aucune clé fournisseur, aucune variable de session).
	cmd.Env = append([]string{"PATH=/usr/bin:/bin"}, o.Env...)
	configurerGroupe(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	debut := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	res := &Resultat{}
	var wg sync.WaitGroup
	collecte := func(r interface{ Read([]byte) (int, error) }, cap int64, dest *string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4096)
			var total int64
			var b strings.Builder
			for {
				n, err := r.Read(buf)
				if n > 0 {
					total += int64(n)
					if total <= cap {
						b.Write(buf[:n])
					}
				}
				if err != nil {
					break
				}
			}
			*dest = b.String()
		}()
	}
	var so, se string
	collecte(stdout, sortieMax, &so)
	collecte(stderr, sortieMax, &se)

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- cmd.Wait()
	}()

	tuer := func(sig int) { tuerProcessus(cmd.Process.Pid, sig) }
	expired := time.NewTimer(o.LimiteDuree)
	annuleParContexte := false
	select {
	case <-done:
		res.TermineAvant = true
	case <-ctx.Done():
		annuleParContexte = true
		tuer(sigTermine())
		gr := time.NewTimer(grace)
		select {
		case <-done:
		case <-gr.C:
			tuer(sigTue())
			<-done
		}
		gr.Stop()
	case <-expired.C:
		tuer(sigTermine())
		gr := time.NewTimer(grace)
		select {
		case <-done:
		case <-gr.C:
			tuer(sigTue())
			<-done
			res.Expulse = true
		}
		gr.Stop()
	}
	expired.Stop()
	res.Annule = annuleParContexte
	res.Code = cmd.ProcessState.ExitCode()
	res.Duree = time.Since(debut)
	res.Stdout = expurger(tronqueUTF8(so, sortieMax))
	res.Stderr = expurger(tronqueUTF8(se, sortieMax))

	if o.ExportChemin != "" {
		b, errL := os.ReadFile(o.ExportChemin)
		if errL != nil || len(b) == 0 {
			return res, ErrExportManquant
		}
		res.ExportPresent = true
		var e ExportVibe
		if err := json.Unmarshal(b, &e); err != nil {
			return res, fmt.Errorf("%w : %v", ErrExportCorrompu, err)
		}
		if contientSecretMulti(string(b)) {
			return res, ErrSecretExport
		}
		res.ExportValide = true
		res.Export = e
	}
	return res, nil
}

// tronqueUTF8 : coupe la chaîne à n octets sans casser un rune.
func tronqueUTF8(s string, n int64) string {
	if int64(len(s)) <= n {
		return s
	}
	c := s[:n]
	for !utf8.ValidString(c) && len(c) > 0 {
		c = c[:len(c)-1]
	}
	return c
}

// CheminExport : chemin standard de export.json sous un répertoire run.
func CheminExport(outputDir string) string {
	return filepath.Join(outputDir, "export.json")
}

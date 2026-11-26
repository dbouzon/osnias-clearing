// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package claudecode : voie Claude Code SUPERVISÉE du serveur agent A2A
// Anthropic (M-COORD-LLM-05, ordre #58, plan v2 PLAN_ACCEPTE). UN agent
// deux voies derrière la même carte :
//
//   - VOIE API/INFÉRENCE (critique de plan, réception) : ExecInférence
//     existant (M-COORD-LLM-04), INCHANGÉ, délégué par l'Agent ici ;
//   - VOIE:CODE (exécution outillée supervisée) : ExecClaudeCode pilote
//     la CLI Claude Code en sous-processus via le runner GÉNÉRIQUE
//     réutilisé tel quel (env liste blanche, expurgation des secrets,
//     SIGTERM coopératif puis SIGKILL du groupe, codes 0/1/2/3), en
//     qualification avec la CLI FACTICE claude-factice (0 €, aucune
//     clé, aucun réseau fournisseur, aucune écriture GitHub).
//
// Sécurité actée (plan v2, réponse aux points bloquants 2 et 3) :
//
//   - PERMISSIONS, triple contrôle : le superviseur n'accepte QUE
//     --permission-mode dontAsk sinon MODE_PERMISSION_REFUSE AVANT tout
//     lancement (bypassPermissions interdit par l'ordre) ; le
//     settings.json est ÉCRIT puis VALIDÉ (deny obligatoires, allow ⊆
//     liste blanche : REGLE_PERMISSION_INCONNUE) ; la CLI factice
//     revérifie mode et outils embarqués ;
//   - VALIDATION, chacun des cas de refus est un MOTIF CONSTANT nominal
//     (RESULTAT_NON_JSON, RESULTAT_ECHEC:<code>, SESSION_ID_ABSENT,
//     RESULT_VIDE, COUT_INCONNU, RESULTAT_COUT_HORS_BORNE,
//     RESULTAT_TROP_GRAND, ENV_HORS_LISTE_BLANCHE) : jamais de
//     réparation silencieuse ; AUCUNE session n'est ouverte sur un run
//     refusé (reprise impossible d'un run invalide) ;
//   - SESSIONS : registre VERROUILLÉ (fichier exclusif non bloquant),
//     écriture atomique post-validation uniquement ;
//   - ISOLEMENT : espace de travail dédié par mission, HOME RÉÉCRIT
//     vers cet espace (aucune lecture du ~/.claude réel), liste blanche
//     d'environnement obtenue du runner (SEUL HOME transmis).
//
// Le budget n'est JAMAIS réservé ici : la voûte TX1/TX2 vit côté
// orchestrateur COORD ; --max-budget-usd n'est qu'une DEUXIÈME borne
// locale du run.
package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/runner"
)

// marqueurVoieCode : tête de charge routant vers la voie Claude Code
// (données opaques : aucune instruction de la charge n'est exécutée).
const marqueurVoieCode = "VOIE:CODE "

// modePermissionSeul : SEUL mode de permission admis (CI verrouillée :
// tout ce qui demanderait une confirmation est REFUSÉ par la CLI).
const modePermissionSeul = "dontAsk"

// listeBlancheOutils : outils admis dans les règles allow du settings ;
// tout autre allow est REFUSÉ (REGLE_PERMISSION_INCONNUE).
var listeBlancheOutils = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "List": true,
	"Bash(ls:*)": true, "Bash(dir:*)": true,
}

// denyObligatoires : règles deny ÉCRITES par le superviseur : réseau et
// écritures tierces interdits par construction (les deny bloquent dans
// TOUS les modes, documentation Claude Code).
var denyObligatoires = []string{"WebFetch", "WebSearch", "WebRequest", "TodoWrite"}

// listeBlancheEnvAttendue : ensemble EXACT attendu de l'environnement
// du processus CLI : HOME réécrit par le superviseur + PATH ajouté par
// le runner (contrat T-ENV, seul chemin d'exécution). Toute autre clé
// reçue (ou manquante) est une rupture d'isolation, refusée nettement.
var listeBlancheEnvAttendue = []string{"HOME", "PATH"}

// JetonsCodeDefaut : usage DECLARÉ de la voie Claude Code (SIMULATION
// documentée, jamais facturée, loopback uniquement).
const JetonsCodeDefaut = 800

// Erreurs de la voie Claude Code (fail closed, motifs NOMINAUX,
// cause et remède documentés dans le rapport FR/EN).
var (
	ErrOptionsIncompletes    = errors.New("claudecode : OPTIONS_INCOMPLETES : binaire, racine de travail, registre, limite de durée, budget USD, outils autorisés et taille max de résultat exigés (fail closed)")
	ErrModePermissionRefuse  = errors.New("claudecode : MODE_PERMISSION_REFUSE : seul --permission-mode dontAsk est admis (bypassPermissions et toute autre valeur interdits avant lancement, fail closed)")
	ErrReglePermissionInconn = errors.New("claudecode : REGLE_PERMISSION_INCONNUE : règle d'allow hors liste blanche ou deny manquant dans settings (fail closed)")
	ErrResultatNonJSON       = errors.New("claudecode : RESULTAT_NON_JSON : sortie CLI non JSON valide (fail closed, aucune session ouverte)")
	ErrSessionIDAbsent       = errors.New("claudecode : SESSION_ID_ABSENT : session_id manquant dans le résultat (fail closed, aucune session ouverte)")
	ErrResultVide            = errors.New("claudecode : RESULT_VIDE : champ result vide (fail closed, aucune session ouverte)")
	ErrCoutInconnu           = errors.New("claudecode : COUT_INCONNU : total_cost_usd absent ou négatif (jamais inconnu = gratuit, fail closed)")
	ErrCoutHorsBorne         = errors.New("claudecode : RESULTAT_COUT_HORS_BORNE : coût déclaré du run hors budget alloué (fail closed)")
	ErrResultatTropGrand     = errors.New("claudecode : RESULTAT_TROP_GRAND : résultat hors enveloppe de taille (fail closed)")
	ErrEnvHorsListeBlanche   = errors.New("claudecode : ENV_HORS_LISTE_BLANCHE : le processus a reçu un environnement hors liste blanche (fail closed, isolation rompue)")
	ErrSessionIncoherente    = errors.New("claudecode : SESSION_INCOHERENTE : session de reprise introuvable ou divergente (fail closed)")
	ErrChargeVide            = errors.New("claudecode : CHARGE_VIDE : charge VOIE:CODE exigée (fail closed)")
)

// OptionsCode : configuration BORNÉE de la voie Claude Code (factice en
// qualification). AUCUN secret : aucune clé n'existe dans un run de
// qualification (liste blanche d'environnement, un seul HOME réécrit).
type OptionsCode struct {
	Binaire        string // chemin de la CLI Claude Code (factice en qualification)
	Racine         string // racine des espaces de travail (un dossier par mission)
	Registre       *RegistreClaude
	LimiteDuree    time.Duration // borne DURE exigée
	LimiteGrace    time.Duration // délai SIGTERM -> SIGKILL
	BudgetUSD      float64       // borne locale --max-budget-usd (deuxième défense ; la voûte COORD prime)
	Outils         []string      // règles allow, ⊆ listeBlancheOutils, non vide
	ResultatMax    int           // taille max (octets) du champ result
	SessionLogging bool          // exigé (reprise --resume, S17)
}

// valider : cohérence AVANT tout lancement (fail closed), TRIPLE
// CONTRÔLE des permissions — n°1 : mode refusé ici même.
func (o OptionsCode) valider() error {
	if o.Binaire == "" || o.Racine == "" || o.Registre == nil ||
		o.LimiteDuree <= 0 || o.BudgetUSD <= 0 || o.ResultatMax <= 0 || len(o.Outils) == 0 {
		return ErrOptionsIncompletes
	}
	if !o.SessionLogging {
		return ErrOptionsIncompletes // journalisation exigée pour la reprise
	}
	if modePermissionSeul != "dontAsk" { // défense figée : jamais compilable autrement
		return ErrModePermissionRefuse
	}
	for _, outil := range o.Outils {
		if !listeBlancheOutils[outil] {
			return fmt.Errorf("%w : %s", ErrReglePermissionInconn, outil)
		}
	}
	return nil
}

// ExecClaudeCode : exécuteur A2A de la voie Claude Code supervisée.
type ExecClaudeCode struct {
	Opts OptionsCode
}

// Cancel : annulation CONFORME (l'événement canceled est EXIGÉ par le
// SDK ; le runner propage SIGTERM puis SIGKILL au groupe).
func (*ExecClaudeCode) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return annulationConforme(ec)
}

func annulationConforme(ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// Execute : déroule un run Claude Code supervisé dans l'appel A2A
// (synchrone dans message/send, contrat inchangé, M-COORD-LLM-03 T3).
func (e *ExecClaudeCode) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}
		sortie, err := e.Run(ctx, charge)
		if err != nil {
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("claudecode : "+err.Error()))), nil)
			return
		}
		if !yield(artefact(ec, sortie), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// Run : lance la CLI supervisée, récolte, VALIDE, enregistre la session.
// EXPORTÉ pour la recette (tests CC) : les invariants de sécurité sont
// démontrés au même niveau que l'exécution réelle.
func (e *ExecClaudeCode) Run(ctx context.Context, charge string) (string, error) {
	if err := e.Opts.valider(); err != nil {
		return "", err
	}
	mission, revision, correction, tache := paramsCode(strings.TrimPrefix(charge, marqueurVoieCode))
	if strings.TrimSpace(tache) == "" {
		return "", ErrChargeVide
	}
	// Espace de travail DÉDIÉ par mission, HOME réécrit : la CLI ne voit
	// ni le HOME réel, ni ~/.claude, ni le moindre identifiant.
	travail := filepath.Join(e.Opts.Racine, mission)
	home := filepath.Join(travail, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	// Permissions — contrôles n°2 : settings ÉCRIT puis VALIDÉ (deny
	// obligatoires présents, allow ⊆ liste blanche).
	parametres, err := e.ecrireParametres(home)
	if err != nil {
		return "", err
	}
	args := []string{
		"-p", tache,
		"--output-format", "json",
		"--permission-mode", modePermissionSeul,
		"--allowedTools", strings.Join(e.Opts.Outils, ","),
		"--settings", parametres,
		"--max-budget-usd", strconv.FormatFloat(e.Opts.BudgetUSD, 'f', -1, 64),
	}
	// Correction : MÊME session + contexte neuf (jamais un retry).
	if correction {
		s, ok, err := e.Opts.Registre.Chercher(mission, revision)
		if err != nil {
			return "", err
		}
		if !ok || s.SessionID == "" {
			return "", fmt.Errorf("%w : %s/%s", ErrSessionIncoherente, mission, revision)
		}
		args = append(args, "--resume", s.SessionID)
	}
	res, err := runner.Lancer(ctx, runner.Options{
		Binaire:     e.Opts.Binaire,
		Args:        args,
		Repertoire:  travail,
		Env:         []string{"HOME=" + home}, // LISTE BLANCHE : SEUL HOME réécrit
		LimiteDuree: e.Opts.LimiteDuree, LimiteGrace: e.Opts.LimiteGrace,
		SortieMax: int64(e.Opts.ResultatMax) * 2,
	})
	if err != nil {
		return "", err
	}
	if res.Annule {
		return "", errors.New("claudecode : ANNULE : run annulé avant achèvement (aucune session ouverte, décision pilote pour toute reprise)")
	}
	if res.Expulse {
		return "", errors.New("claudecode : ARRET_FORCE : limite de durée dépassée, SIGKILL du groupe (aucune session ouverte)")
	}
	if res.Code != 0 {
		return "", fmt.Errorf("claudecode : RESULTAT_ECHEC:%d : %s", res.Code, coupe(res.Stderr))
	}
	sortie, err := validerSortie(res, e.Opts.ResultatMax, e.Opts.BudgetUSD)
	if err != nil {
		return "", err // AUCUNE session ouverte : motif nominal, fail closed
	}
	// Validation PASSÉE : la session est enregistrée SOUS VERROU.
	if err := e.Opts.Registre.Enregistrer(SessionCC{
		Mission: mission, Revision: revision, SessionID: sortie.SessionID,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"jetons":%d,"texte":%s}`, JetonsCodeDefaut, mustJSON(sortie.Result)), nil
}

// sortieCLI : sous-ensemble STRICT du JSON documenté de la CLI
// (--output-format json) ; champs additionnels tolérés à la lecture mais
// vérifiés quand ils engagent la sécurité (environment).
type sortieCLI struct {
	Result    string   `json:"result"`
	SessionID string   `json:"session_id"`
	IsError   bool     `json:"is_error"`
	CoutUSD   *float64 `json:"total_cost_usd"`
	NumTurns  int      `json:"num_turns"`
	EnvRecue  []string `json:"environment"`
}

// validerSortie : VALIDATION STRICTE, motifs NOMINAUX par cas (point
// bloquant n°2) ; aucune écriture de session en cas d'échec (garanti par
// l'appelant, qui n'enregistre qu'après ce retour nil).
func validerSortie(res *runner.Resultat, resultatMax int, budgetUSD float64) (sortieCLI, error) {
	var s sortieCLI
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &s); err != nil {
		return sortieCLI{}, ErrResultatNonJSON
	}
	if s.IsError {
		return sortieCLI{}, fmt.Errorf("claudecode : IS_ERROR_VRAI : %s", coupe(s.Result))
	}
	if s.SessionID == "" {
		return sortieCLI{}, ErrSessionIDAbsent
	}
	if strings.TrimSpace(s.Result) == "" {
		return sortieCLI{}, ErrResultVide
	}
	if len(s.Result) > resultatMax {
		return sortieCLI{}, ErrResultatTropGrand
	}
	if s.CoutUSD == nil || *s.CoutUSD < 0 {
		return sortieCLI{}, ErrCoutInconnu
	}
	if *s.CoutUSD > budgetUSD {
		return sortieCLI{}, ErrCoutHorsBorne
	}
	// Isolation : la CLI DOIT ne recevoir EXACTEMENT la liste blanche
	// attendue (elle déclare ce qu'elle a reçu ; toute divergence —
	// clé héritée, token, variable réelle — prouve une fuite).
	if len(s.EnvRecue) != len(listeBlancheEnvAttendue) {
		return sortieCLI{}, fmt.Errorf("%w : %v", ErrEnvHorsListeBlanche, s.EnvRecue)
	}
	for i, cle := range listeBlancheEnvAttendue {
		if s.EnvRecue[i] != cle {
			return sortieCLI{}, fmt.Errorf("%w : %v", ErrEnvHorsListeBlanche, s.EnvRecue)
		}
	}
	return s, nil
}

// ecrireParametres : ÉCRIT le settings.json (allow bornés, deny
// obligatoires) puis le VALIDE par RELecture (contrôle n°2 du triple
// contrôle des permissions, plan v2).
func (e *ExecClaudeCode) ecrireParametres(home string) (string, error) {
	type permissions struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	parametres := filepath.Join(home, "coord-settings-claude.json")
	corps, err := json.Marshal(struct {
		Permissions permissions `json:"permissions"`
	}{Permissions: permissions{Allow: e.Opts.Outils, Deny: denyObligatoires}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(parametres, corps, 0o600); err != nil {
		return "", err
	}
	// Relecture + validation STRICTE : deny présents, allow ⊆ blanche.
	var relu struct {
		Permissions permissions `json:"permissions"`
	}
	data, err := os.ReadFile(parametres)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &relu); err != nil {
		return "", ErrReglePermissionInconn
	}
	if len(relu.Permissions.Deny) < len(denyObligatoires) {
		return "", ErrReglePermissionInconn
	}
	for _, outil := range relu.Permissions.Allow {
		if !listeBlancheOutils[outil] {
			return "", fmt.Errorf("%w : %s", ErrReglePermissionInconn, outil)
		}
	}
	return parametres, nil
}

// Agent : exécuteur UNIQUE à deux voies derrière la même carte (note
// v1.1 §3) : l'inférence EXISTANTE (ExecInférence, inchangé) pour
// critique/réception, la voie Claude Code pour l'exécution outillée.
// Le marqueur VOIE:CODE est une DONNÉE : aucune instruction de la
// charge ne modifie le comportement (S9).
type Agent struct {
	Inference interface {
		Execute(context.Context, *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error]
		Cancel(context.Context, *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error]
	}
	Code *ExecClaudeCode
}

// Execute : route la charge vers la voie demandée.
func (a Agent) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	charge := texteDeCharge(ec)
	if strings.HasPrefix(charge, marqueurVoieCode) && a.Code != nil {
		return a.Code.Execute(ctx, ec)
	}
	return a.Inference.Execute(ctx, ec)
}

// Cancel : annulation conforme sur les deux voies (événement canceled
// EXIGÉ par le SDK ; le contexte est annulé par le serveur, le runner
// propage SIGTERM au groupe puis SIGKILL).
func (a Agent) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return annulationConforme(ec)
}

// ---- helpers A2A (mêmes conventions que coord/a2a-agent/mistral) ----

// artefact : enveloppe {"jetons", "texte"} (contrat strict du rail).
func artefact(ec *a2asrv.ExecutorContext, texte string) *a2a.TaskArtifactUpdateEvent {
	payload := fmt.Sprintf(`{"jetons":%d,"texte":%s}`, JetonsCodeDefaut, mustJSON(texte))
	return a2a.NewArtifactEvent(ec, a2a.NewTextPart(payload))
}

// mustJSON : marshal JSON minimal d'une chaîne (textes factices
// uniquement ; toute sortie réelle passe par runner, déjà expurgée).
func mustJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r', '\t':
			b.WriteString(" ")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// texteDeCharge : texte de la charge A2A reçue.
func texteDeCharge(ec *a2asrv.ExecutorContext) string {
	if ec.Message == nil || len(ec.Message.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range ec.Message.Parts {
		b.WriteString(p.Text())
	}
	return b.String()
}

// coupe : borne les textes d'erreur (déjà expurgés par le runner).
func coupe(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// paramsCode : extrait MISSION=, REVISION=, CORRECTION=, TACHE= de la
// charge VOIE:CODE (données opaques, aucune instruction exécutée).
func paramsCode(charge string) (mission, revision string, correction bool, tache string) {
	mission, revision, correction = "m", "r0", false
	reste := charge
	for _, marque := range []string{"MISSION=", "REVISION=", "CORRECTION="} {
		mr := marque
		i := strings.Index(reste, mr)
		if i < 0 {
			continue
		}
		suite := reste[i+len(mr):]
		fin := strings.IndexAny(suite, " \t\n")
		if fin < 0 {
			fin = len(suite)
		}
		val := suite[:fin]
		switch mr {
		case "MISSION=":
			mission = val
		case "REVISION=":
			revision = val
		case "CORRECTION=":
			correction = val == "1"
		}
	}
	if j := strings.Index(charge, "TACHE="); j >= 0 {
		tache = charge[j+len("TACHE="):]
	} else {
		tache = charge
	}
	return mission, revision, correction, tache
}

// _ : garde les imports de tri utilisés par d'éventuels diagnostics.
var _ = sort.Strings

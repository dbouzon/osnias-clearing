// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package mistral : PREMIER EXEMPLAIRE DU SOCLE (M-COORD-LLM-03, ordre
// #54, plan v2 PLAN_ACCEPTE). UN agent A2A Mistral, DEUX voies derrière
// la même carte (note v1.1 §3) :
//
//   - VOIE API/INFÉRENCE (critique de plan, réception, direction) :
//     ExecMistralAPI est la traduction A2A -> API d'inférence. En cette
//     mission il est FACTICE DÉTERMINISTE (qualification SIMULÉE, 0 €,
//     aucune clé Mistral) : verdicts JSON stricts ancrés à l'empreinte
//     extraite de la charge COORD, gabarit de la machine normative.
//     L'appel réel vers api.mistral.ai est la qualification B1..B3
//     (NON_EXÉCUTÉE, autorisation écrite pilote requise) et passera par
//     le profil moteur connecteurs.ProfilMistral (T2, mistral.go moteur).
//   - VOIE VIBE CLI (exécution outillée : fichiers, tests, commits) :
//     ExecVibe supervise la CLI en sous-processus via le runner générique
//     (runner.Lancer), factice par défaut (binaire factice exigé : aucun
//     appel fournisseur, 0 €, aucune écriture GitHub par l'agent testé).
//
// Routage de la charge : un marqueur VOIE en tête de charge (données
// opaques ; aucune instruction de la charge n'est exécutée — S9) :
//
//	VOIE:VIBE <mandat>  -> voie Vibe CLI (exécution outillée)
//	<charge COORD>      -> voie inférence (critique/réception)
//
// ARBITRAGES ACTÉS (plan v2) : une session Vibe par RÉVISION ; worktree
// déterministe par (mission, révision) ; CORRECTION = même session +
// contexte neuf à clé d'idempotence neuve (nouvelle délégation, jamais
// un retry) ; REPRISE D'UNE TÂCHE INCERTAINE = PILOTE EXCLUSIVEMENT
// (aucune relance automatique, fail closed) ; export manquant/corrompu =
// consommation TOTALE de l'enveloppe (jamais « inconnu = gratuit ») ;
// --max-tokens BORNANT par construction (cohérence vérifiée AVANT
// lancement, refus BUDGET_DEPASSE sinon) ; --max-price indicatif
// secondaire NON déterminant ; session logging désactivée = refus de
// délégation (S17, fail closed : la reprise ne peut pas être garantie).
//
// BUDGET : ce package ne duplique AUCUNE réservation monétaire : la
// voûte TX1/TX2 vit côté orchestrateur COORD (connecteurs). L'exécuteur
// vérifie seulement la COHÉRENCE locale (max_tokens <= enveloppe de
// jetons allouée à la délégation) et consomme l'usage réel relevé dans
// export.json (ajustement a posteriori côté COORD).
package mistral

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/dbouzon/osnias-clearing/coord/a2a-agent/runner"
)

// JetonsVibeDefaut : usage déclaré par la voie Vibe (SIMULATION
// documentée, jamais facturée, loopback uniquement).
const JetonsVibeDefaut = 750

// marqueurVoieVibe : tête de charge routant vers l'exécuteur Vibe.
const marqueurVoieVibe = "VOIE:VIBE "

// Erreurs de la voie Vibe (fail closed, jamais « inconnu = gratuit »).
var (
	ErrBudgetIncoherent        = errors.New("mistral/vibe : BUDGET_DEPASSE : max_tokens ne borne pas l'enveloppe allouée (cohérence exigée AVANT lancement, fail closed)")
	ErrSessionLoggingDesactive = errors.New("mistral/vibe : SESSION_LOGGING_DESACTIVE : journalisation exigée pour la reprise --resume (fail closed, délégation refusée)")
	ErrOptionsIncompletes      = errors.New("mistral/vibe : OPTIONS_INCOMPLETES : binaire, répertoire de test, limite de durée, max_turns et max_tokens > 0 exigés (fail closed)")
	ErrEnveloppeAbsente        = errors.New("mistral/vibe : ENVELOPPE_ABSENTE : enveloppe de jetons allouée exigée (voûte COORD, fail closed)")
	ErrRemotePresent           = errors.New("mistral/vibe : DEPOT_AVEC_REMOTE : le dépôt de test ne doit avoir AUCUN remote (T-GIT, aucune écriture GitHub possible, fail closed)")
)

// OptionsVibe : configuration BORNÉE de la voie Vibe (factice en
// qualification). Aucun secret : la clé fournisseur n'existe pas dans
// un run de qualification (environnement en liste blanche, S19bis).
type OptionsVibe struct {
	Binaire        string        // chemin du CLI Vibe (factice en qualification)
	RepertoireTest string        // dépôt de test LOCAL SANS remote (T-GIT)
	RacineWorktree string        // racine des worktrees déterministes
	LimiteDuree    time.Duration // borne DURE exigée
	LimiteGrace    time.Duration // SIGTERM -> SIGKILL
	MaxTurns       int           // plafond de tours (positif exigé)
	MaxTokens      int           // plafond total prompt+complétion, SOUS-AGENTS INCLUS
	Enveloppe      int           // enveloppe de jetons allouée par la voûte COORD
	SessionLogging bool          // journalisation des sessions (exigée, S17)
	// ArgsScenarios : arguments factices (qualification SIMULÉE
	// uniquement) transmis verbatim à la CLI factice pour piloter les
	// scénarios S11/S12/S13/T-LONG. Aucun secret n'y transite jamais.
	ArgsScenarios []string
}

// valider : cohérence AVANT tout lancement (fail closed).
func (o OptionsVibe) valider() error {
	if o.Binaire == "" || o.RepertoireTest == "" || o.LimiteDuree <= 0 ||
		o.MaxTurns <= 0 || o.MaxTokens <= 0 {
		return ErrOptionsIncompletes
	}
	if !o.SessionLogging {
		return ErrSessionLoggingDesactive
	}
	if o.Enveloppe <= 0 {
		return ErrEnveloppeAbsente
	}
	if o.MaxTokens > o.Enveloppe {
		return ErrBudgetIncoherent
	}
	return nil
}

// Session : lien durable (mission, révision, worktree, session CLI) —
// la paire (session, worktree) est l'ancre de reprise (note v1.1 §4.5).
// Granularité : une session par RÉVISION ; la CORRECTION réutilise la
// session d'origine avec une clé d'idempotence neuve.
type Session struct {
	Mission  string `json:"mission"`
	Revision string `json:"revision"`
	Session  string `json:"session"` // identifiant de session CLI (export.json)
	Worktree string `json:"worktree"`
}

// clé : mission "/" révision.
func cleGen(mission, revision string) string { return mission + "/" + revision }

// RegistreSessions : sessions persistées de l'agent (fichier JSON,
// écriture atomique 0600 — corrélation durable S16). Aucun secret.
type RegistreSessions struct {
	mu       sync.Mutex
	Chemin   string
	Sessions map[string]Session
}

// NouveauRegistreSessions : charge (ou crée) le registre durable.
func NouveauRegistreSessions(chemin string) (*RegistreSessions, error) {
	r := &RegistreSessions{Chemin: chemin, Sessions: map[string]Session{}}
	if b, err := os.ReadFile(chemin); err == nil {
		if err := json.Unmarshal(b, &r.Sessions); err != nil {
			return nil, fmt.Errorf("registre sessions corrompu : %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return r, nil
}

// Chercher : session d'une (mission, révision), sinon "".
func (r *RegistreSessions) Chercher(mission, revision string) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.Sessions[cleGen(mission, revision)]
	return s, ok
}

// Enregistrer : persiste atomiquement (session -> worktree -> registre).
func (r *RegistreSessions) Enregistrer(s Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Sessions[cleGen(s.Mission, s.Revision)] = s
	b, err := json.MarshalIndent(r.Sessions, "", "  ")
	if err != nil {
		return err
	}
	return ecrireAtomique(r.Chemin, b)
}

// ecrireAtomique : temporaire + rename, 0600 (modèle rail/registre).
func ecrireAtomique(chemin string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(chemin), ".sessions-*")
	if err != nil {
		return err
	}
	nom := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(nom)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(nom)
		return err
	}
	if err := os.Chmod(nom, 0o600); err != nil {
		os.Remove(nom)
		return err
	}
	return os.Rename(nom, chemin)
}

// ExecMistralAPI : exécuteur FACTICE déterministe de la voie inférence
// (S8 verifier) : verdicts JSON stricts ANCRÉS à l'empreinte extraite
// de la charge (machine normative COORD), rétroaction pilotable par les
// tests (VerdictReception,VERDICT_REDIRECTION pour le cycle A_CORRIGER).
type ExecMistralAPI struct {
	VerdictReception string // ACCEPTEE par défaut ; A_CORRIGER injectable
	Partage          *atomic.Bool
}

// Cancel : annulation CONFORME (l'événement canceled est EXIGÉ par le
// SDK ; une séquence vide ferait bloquer tasks/cancel — piège du POC).
func (ExecMistralAPI) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return cancelSeq(ec)
}

func cancelSeq(ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// Execute : critique de plan / réception, verdict ancré, JSON strict,
// enveloppe d'artefact {"jetons": N, "texte": "..."} (contrat du rail).
func (e ExecMistralAPI) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		emp := empDuPrompt(charge)
		if emp == "" {
			// Charge NON conforme au gabarit => ÉCHEC net, données
			// opaques (S9 injection : aucune instruction exécutée).
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("CHARGE_NON_CONFORME"))), nil)
			return
		}
		var texte string
		switch {
		case strings.Contains(charge, "CRITIQUE DE PLAN"):
			texte = fmt.Sprintf(`{"verdict":"PLAN_ACCEPTE","points":[],"empreinte_attendue":%q}`, emp)
		case strings.Contains(charge, "RECEPTION INDEPENDANTE"):
			v := "ACCEPTEE"
			if e.VerdictReception != "" {
				v = e.VerdictReception
			}
			texte = fmt.Sprintf(`{"verdict":%q,"points":[],"empreinte_attendue":%q,`+
				`"verdict_id":"MISTRALFICT01","verdict_url":"https://example.invalid/mistral-fictice"}`,
				v, emp)
		default:
			// Exécution générique SIMULÉE (contenu opaque).
			s := "MISTRAL_API_SIMULE : " + charge
			if len(s) > 200 {
				s = s[:200]
			}
			texte = s
		}
		if !yield(artefact(ec, texte), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// ExecVibe : exécuteur de la voie Vibe CLI, SUPERVISÉ par le runner
// générique. Le run se déroule DANS l'appel message/send (le client
// HTTP reste bloqué : capture de l'artefact à l'envoi, sémantique du
// pont). L'annulation tasks/cancel annule le contexte -> SIGTERM au
// groupe (S14) ; l'événement canceled est émis par Cancel (SDK).
type ExecVibe struct {
	Opts       OptionsVibe
	Registre   *RegistreSessions
	ErreursLog *atomic.Bool // trace les échecs (S11/S12 observabilité)
}

// Cancel : annulation conforme (événement canceled EXIGÉ).
func (v *ExecVibe) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return cancelSeq(ec)
}

// Execute : déroule un run Vibe supervisé dans l'appel A2A.
func (v *ExecVibe) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}
		// T-GIT de fait : le dépôt de test ne doit PAS avoir de remote
		// (aucune écriture GitHub possible par construction).
		if err := verifierSansRemote(v.Opts.RepertoireTest); err != nil {
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
			return
		}
		texte, err := v.run(ctx, charge)
		if err != nil {
			if v.ErreursLog != nil {
				v.ErreursLog.Store(true)
			}
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("A2A : "+err.Error()))), nil)
			return
		}
		if !yield(artefact(ec, texte), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// run : lance la CLI supervisée, récolte, traduit, consomme.
func (v *ExecVibe) run(ctx context.Context, charge string) (string, error) {
	if err := v.Opts.valider(); err != nil {
		return "", err
	}
	// Mandat structuré opaque : VOIE:VIBE MISSION=<m> REVISION=<r> CORRECTION=<0|1> TACHE=<texte>.
	mission, revision, correction, tache := paramsVibe(strings.TrimPrefix(charge, marqueurVoieVibe))
	runID := "run-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	outputDir := filepath.Join(v.Opts.RacineWorktree, runID)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", err
	}
	prompt := filepath.Join(outputDir, "mandat.json")
	if err := os.WriteFile(prompt, []byte(tache), 0o600); err != nil {
		return "", err
	}
	args := []string{
		"--trust", "-p", "--prompt-file", prompt,
		"--worktree", "COORD-" + mission + "-" + revision,
		"--max-turns", strconv.Itoa(v.Opts.MaxTurns),
		"--max-tokens", strconv.Itoa(v.Opts.MaxTokens),
		"--time-limit", strconv.Itoa(int(v.Opts.LimiteDuree.Seconds())),
		"--output-dir", outputDir,
		"--output", "json",
	}
	// Correction : MÊME session + contexte neuf (S16) ; jamais un retry.
	if correction {
		if s, ok := v.Registre.Chercher(mission, revision); ok && s.Session != "" {
			args = append(args, "--resume", s.Session)
		}
	}
	args = append(args, v.Opts.ArgsScenarios...)
	args = append(args, v.Opts.ArgsScenarios...)
	res, err := runner.Lancer(ctx, runner.Options{
		Binaire: v.Opts.Binaire, Args: args,
		Repertoire:  v.Opts.RepertoireTest,
		Env:         []string{}, // LISTE BLANCHE VIDE : aucun identifiant (T-ENV)
		LimiteDuree: v.Opts.LimiteDuree, LimiteGrace: v.Opts.LimiteGrace,
		ExportChemin: runner.CheminExport(outputDir),
	})
	if err != nil {
		// Fail closed transporté (export manquant/corrompu inclus).
		return "", err
	}
	// S11 : traduction des codes : 0 fini, 1/2 échec net, 3 JAMAIS
	// réussite (arrêt avant achèvement) ; on ne qualifie pas le code
	// seul : seul le texte livré compte (code 0 != réception).
	switch {
	case res.Code == runner.CodeFin && res.ExportValide:
		// Enregistre la session (corrélation durable S16).
		_ = v.Registre.Enregistrer(Session{
			Mission: mission, Revision: revision,
			Session: res.Export.SessionID, Worktree: v.Opts.RacineWorktree,
		})
		return fmt.Sprintf(`{"jetons":%d,"texte":%s}`, jetonsDe(res.Export), mustJSON(res.Stdout)), nil
	case res.Code == runner.CodeFin:
		return "", errors.New("EXPORT_INVALIDE_APRES_CODE_0 (code 0 ne vaut jamais réception, fail closed)")
	case res.Code == runner.CodeDemarrage:
		return "", fmt.Errorf("DEMARRAGE_IMPOSSIBLE : %s", res.Stderr)
	case res.Code == runner.CodeAPI:
		return "", fmt.Errorf("ECHEC_API_RUNTIME : %s", res.Stderr)
	case res.Code == runner.CodeAvantAchev:
		return "", errors.New("ARRET_AVANT_ACHEVEMENT : code 3, jamais une réussite (décision pilote)")
	default:
		return "", fmt.Errorf("CODE_NON_CATEGORISE %d", res.Code)
	}
}

// jetonsDe : consommation REELLE relevée dans export.json ; export
// invalide à ce stade = enveloppe TOTALE (manqué par runner sous une
// erreur ; ici re-marqué par le texte a posteriori).
func jetonsDe(e runner.ExportVibe) int {
	if e.TokensPrompt+e.TokensCompletion > 0 {
		return e.TokensPrompt + e.TokensCompletion
	}
	// Export sans consommation : consommation inconnue -> valeur
	// conservatrice élevée ; l'orchestrateur consomme la voûte en
	// conséquence (jamais inconnu = gratuit).
	return 1 << 30
}

// verifierSansRemote : T-GIT de fait : le dépôt de test ne doit avoir
// AUCUN remote (un push y est impossible par construction).
func verifierSansRemote(repertoire string) error {
	if repertoire == "" {
		return ErrOptionsIncompletes
	}
	c := ".git/config"
	data, err := os.ReadFile(filepath.Join(repertoire, c))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // pas un dépôt git : aucun remote par construction
		}
		return err
	}
	if strings.Contains(string(data), "[remote") {
		return ErrRemotePresent
	}
	return nil
}

// paramsVibe : extrait MISSION=, REVISION=, CORRECTION=, TACHE= de la
// charge VOIE:VIBE (données opaques, aucune instruction exécutée).
func paramsVibe(charge string) (mission, revision string, correction bool, tache string) {
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

// Agent : exécuteur UNIQUE à deux voies derrière la même carte (note
// v1.1 §3 : un seul agent, l'API pour critique/réception, Vibe pour
// l'exécution outillée). Le routage est un marqueur DE DONNÉES opaques :
// aucune instruction de la charge ne modifie le comportement (S9).
type Agent struct {
	API  ExecMistralAPI
	Vibe *ExecVibe
}

// Execute : route la charge vers la voie demandée.
func (a Agent) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	charge := texteDeCharge(ec)
	if strings.HasPrefix(charge, marqueurVoieVibe) && a.Vibe != nil {
		return a.Vibe.Execute(ctx, ec)
	}
	return a.API.Execute(ctx, ec)
}

// Cancel : annulation conforme sur les deux voies (événement canceled
// EXIGÉ par le SDK ; le contexte d'exécution est annulé par le serveur,
// le runner propage SIGTERM au groupe).
func (a Agent) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return cancelSeq(ec)
}

// ---- helpers A2A partagés (mêmes conventions que coord/a2a/factices) ----

// artefact : enveloppe {"jetons", "texte"} (contrat strict du rail).
func artefact(ec *a2asrv.ExecutorContext, texte string) *a2a.TaskArtifactUpdateEvent {
	payload := fmt.Sprintf(`{"jetons":%d,"texte":%s}`, JetonsVibeDefaut, mustJSON(texte))
	return a2a.NewArtifactEvent(ec, a2a.NewTextPart(payload))
}

// mustJSON : marshal JSON minimal d'une chaîne (textes factices
// uniquement ; toute sortie réelle passe par runner.expurger).
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

// empDuPrompt : empreinte 64 hex attendue, extraite de la charge COORD
// (marqueur standard du gabarit transporté, sans échappement).
func empDuPrompt(charge string) string {
	marque := `empreinte_attendue":"`
	i := strings.Index(charge, marque)
	if i < 0 {
		return ""
	}
	reste := charge[i+len(marque):]
	if len(reste) < 64 {
		return ""
	}
	suite := reste[:64]
	for _, c := range suite {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return suite
}

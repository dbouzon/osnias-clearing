package aic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Coordinator orchestre le cycle complet : mission -> agent Mistral ->
// livraison Draft -> revue OpenAI -> validation humaine. Les corrections sont
// plafonnées ; tout incident positionne la mission en BLOCKED ; aucune
// transition n'est exécutée sans commande humaine explicite.
type Coordinator struct {
	Store    *Store
	Config   Config
	Mistral  MistralAdapter
	Reviewer Reviewer
	GitHub   GitHubClient
}

// MissionID construit l'identifiant stable d'une mission depuis son Issue.
// Politique : AIC-<issue_number>, stable sur tout le cycle de vie.
func MissionID(issueNumber int64) string {
	return fmt.Sprintf("AIC-%d", issueNumber)
}

// commandes de commentaire acceptées par le coordinateur (vocabulaire contrôlé).
const (
	CmdStart      = "/aic start"
	CmdCorrection = "/aic correction"
	CmdApprove    = "/aic approve"
	CmdBlock      = "/aic block"
)

// HandleIssueComment traite un payload de webhook issue_comment, déjà
// authentifié par le récepteur. sender est l'auteur du commentaire :
// les écritures du coordinateur lui-même sont ignorées (anti-boucle).
func (c *Coordinator) HandleIssueComment(ctx context.Context, issueNumber int64, issueTitle, sender, body string) error {
	if strings.EqualFold(sender, c.Config.CoordinatorLogin) {
		return nil // anti-boucle : ignorance des écritures du bot
	}
	if err := AgentCommandGuard(body); err != nil {
		return err
	}
	// Habilitation de l'auteur (T02-R1) : seule une identité explicite-
	// ment autorisée (AIC_AUTHORIZED_LOGINS) peut démarrer, corriger,
	// valider ou bloquer une mission. Cette vérification porte sur l'iden-
	// tité authentifiée GitHub du sender, distincte du HMAC du webhook.
	// Politique fail closed : liste vide = personne n'est habilité.
	if !c.Config.IsAuthorized(sender) {
		_ = c.Store.Log(MissionID(issueNumber), "coordinator", "authz", "commande refusée : auteur non habilité ("+sender+")")
		return nil
	}
	bodyNorm := strings.ToLower(strings.TrimSpace(body))
	id := MissionID(issueNumber)
	m, err := c.Store.CreateMission(id, issueNumber, issueTitle)
	if err != nil {
		return err
	}
	switch {
	case strings.Contains(bodyNorm, CmdStart):
		// T02-R3 : une mission RUNNING sans verrou vivant (interruption, ou
		// contention résorbée) accepte un nouveau start : l'exclusivité
		// reste garantie par le verrou persistant, ce n'est pas l'état.
		if m.State != StateOpen && m.State != StateRunning {
			return nil // terminée : idempotent
		}
		return c.RunToReview(ctx, m)
	case strings.Contains(bodyNorm, CmdCorrection):
		if m.State != StateCorrection {
			return nil
		}
		if err := c.Store.Transition(m, StateRunning, "coordinator", "correction relancée"); err != nil {
			return err
		}
		return c.RunToReview(ctx, m)
	case strings.Contains(bodyNorm, CmdApprove):
		if m.State != StateAwaitingApproval {
			return nil
		}
		if err := c.Store.Transition(m, StateAccepted, "human", "validation humaine"); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:accepted", m.ID, "mission réceptionnée (ACCEPTED) après validation humaine")
	case strings.Contains(bodyNorm, CmdBlock):
		if Terminal(m.State) {
			return nil
		}
		if err := c.Store.Transition(m, StateBlocked, "human", "blocage demandé"); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:blocked-human", m.ID, "mission bloquée par intervention humaine")
	}
	return nil
}

// githubPostComment réalise un commentaire GitHub avec idempotence par clé
// (T02-R3, risque 2) : l'opération est d'abord enregistrée en base (états
// pending/sending/done) ; un renvoi après interruption renvoie la même clé à
// l'adaptateur, qui déduplique ; une opération déjà 'done' n'est jamais
// renvoyée. Sans IdempotentGitHub, la protection se limite à l'état 'done'.
func (c *Coordinator) githubPostComment(idemKey, missionID, body string) error {
	_, err := c.githubApply(idemKey, missionID, "comment", func() (string, error) {
		if ig, ok := c.GitHub.(IdempotentGitHub); ok {
			return "", ig.PostCommentIdem(idemKey, missionID, body)
		}
		return "", c.GitHub.PostComment(missionID, body)
	})
	return err
}

// githubDraft réalise une livraison Draft idempotente (T02-R3, risque 2).
// T02-R4 : la référence de livraison est persistée avec l'opération (détail
// github_ops) ; une opération déjà 'done' restitue sa référence sans
// rappeler GitHub, y compris si l'interruption est survenue entre la création
// de la PR et l'enregistrement dans deliveries.
func (c *Coordinator) githubDraft(idemKey, missionID, title, body string) (string, error) {
	return c.githubApply(idemKey, missionID, "draft_pr", func() (string, error) {
		if ig, ok := c.GitHub.(IdempotentGitHub); ok {
			return ig.CreateDraftPRIdem(idemKey, missionID, title, body)
		}
		return c.GitHub.CreateDraftPR(missionID, title, body)
	})
}

// githubApply exécute une écriture GitHub sous clé d'idempotence et
// retourne le détail persisté de l'opération (référence de livraison pour
// une PR Draft). Une clé déjà 'done' n'est jamais renvoyée : son détail
// est restitué depuis la base. Une clé 'pending' ou 'sending' (fenêtre de
// crash entre l'envoi et le marquage) est renvoyée : l'adaptateur
// déduplique par clé.
func (c *Coordinator) githubApply(idemKey, missionID, opType string, send func() (string, error)) (string, error) {
	fresh, err := c.Store.EnqueueGitHubOp(idemKey, missionID, opType)
	if err != nil {
		return "", err
	}
	if !fresh {
		st, err := c.Store.GitHubOpState(idemKey)
		if err != nil {
			return "", err
		}
		if st == "done" {
			// opération déjà réalisée : le détail durable remplace tout
			// rappel de l'adaptateur (T02-R4).
			return c.Store.GitHubOpDetail(idemKey)
		}
	}
	if err := c.Store.MarkGitHubOpState(idemKey, "sending"); err != nil {
		return "", err
	}
	detail, err := send()
	if err != nil {
		return "", err
	}
	if err := c.Store.MarkGitHubOpDone(idemKey, detail); err != nil {
		return "", err
	}
	return detail, nil
}

// ProcessEvent traite un événement GitHub persisté à partir de son type et
// de son payload brut (T02-R2). Retourne done=true lorsque l'événement est
// définitivement traité (aucun rejou utile) ; done=false indique un échec
// ou un verrou actif : l'événement doit rester en file d'attente durable.
func (c *Coordinator) ProcessEvent(ctx context.Context, eventType, payload string) (bool, error) {
	if eventType != "issue_comment" {
		return true, nil
	}
	var p issueCommentPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		// payload illisible : le rejou serait identique, l'événement est clos.
		_ = c.Store.Log("unknown", "coordinator", "error", "payload illisible (événement clos) : "+err.Error())
		return true, nil
	}
	if p.Action != "created" {
		return true, nil
	}
	if err := c.HandleIssueComment(ctx, int64(p.Issue.Number), p.Issue.Title, senderLogin(p), p.Comment.Body); err != nil {
		return false, err
	}
	return true, nil
}

// RunToReview exécute l'agent Mistral (répertoire isolé, bornes appliquées),
// enregistre la livraison Draft, puis soumet le résultat à l'agent OpenAI.
// Exclusivité (T02-R1/R3) : un verrou atomique persistant (mission_locks)
// interdit deux exécutions simultanées de la même mission ; si le verrou est
// détenu, ErrMissionLocked est retourné et l'appelant s'abstient. Le verrou
// est relâché à la fin de l'exécution, quel que soit le résultat ; après un
// crash, un verrou périmé (âgé de plus de RunStaleInterval) est repris par
// Resume lors du redémarrage, jamais pendant qu'une exécution vivante le tient.
func (c *Coordinator) RunToReview(ctx context.Context, m *Mission) error {
	workDir := fmt.Sprintf("%s/%s", strings.TrimRight(c.Config.WorkRoot, "/"), m.ID)
	if m.State == StateOpen {
		if err := c.Store.Transition(m, StateRunning, "coordinator", "exécution agent Mistral"); err != nil {
			// course concurrente : une autre exécution vient de faire passer
			// la mission en RUNNING ; on la reprend telle quelle, le verrou
			// d'exclusivité départage ensuite les exécutants.
			st, reread := c.Store.MissionByID(m.ID)
			if reread != nil || st == nil || st.State != StateRunning {
				return err
			}
			m.State = st.State
		}
	}
	if m.State != StateRunning {
		return fmt.Errorf("mission %s : état %s inattendu pour exécution", m.ID, m.State)
	}
	claimed, err := c.Store.ClaimMission(m.ID)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrMissionLocked
	}
	defer func() { _ = c.Store.ReleaseClaim(m.ID) }()

	runCtx, cancel := context.WithTimeout(ctx, c.Config.MistralTimeout)
	defer cancel()

	res, runErr := c.Mistral.Run(runCtx, m, workDir)
	if runErr != nil {
		_ = c.Store.Log(m.ID, "mistral", "error", runErr.Error())
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "incident d'exécution agent"); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:blocked-run", m.ID, "mission bloquée : incident d'exécution de l'agent développeur")
	}
	// contrôles de budget et d'itérations (T02-D)
	if res.Steps > c.Config.MistralBudgetSteps {
		_ = c.Store.Log(m.ID, "mistral", "budget", fmt.Sprintf("steps=%d > budget=%d", res.Steps, c.Config.MistralBudgetSteps))
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "dépassement de budget"); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:blocked-budget", m.ID, "mission bloquée : dépassement du budget d'exécution")
	}
	if res.Iters > c.Config.MistralMaxIter {
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "dépassement d'itérations"); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:blocked-iters", m.ID, "mission bloquée : dépassement du nombre d'itérations")
	}
	// Côté agent : aucun push. La livraison est une Draft, enregistrée ici.
	// T02-R3 (risque 2) : une livraison déjà persistée est réutilisée ; une
	// création interrompue est renvoyée sous la même clé, l'adaptateur
	// déduplique : jamais deux livraisons pour une même mission.
	_ = ErrNoPushByAgent
	prRef, err := c.Store.DraftForMission(m.ID)
	if err != nil {
		return err
	}
	if prRef == "" {
		prRef, err = c.githubDraft(m.ID+":draft", m.ID, "Livraison "+m.ID, res.Report)
		if err != nil {
			return err
		}
	}
	if err := c.Store.SaveDraft(m.ID, prRef, "draft"); err != nil {
		return err
	}
	_ = c.Store.Log(m.ID, "mistral", "run", fmt.Sprintf("steps=%d iters=%d", res.Steps, res.Iters))
	if err := c.Store.Transition(m, StateReview, "coordinator", "livraison soumise en revue : "+prRef); err != nil {
		return err
	}
	return c.ReviewDelivery(m, prRef, res)
}

// ReviewDelivery soumet la livraison à l'agent OpenAI et applique le verdict.
func (c *Coordinator) ReviewDelivery(m *Mission, prRef string, res MistralResult) error {
	out, err := c.Reviewer.Review(ReviewInput{MissionID: m.ID, Report: res.Report, Diff: res.Diff})
	if err != nil {
		_ = c.Store.Log(m.ID, "openai", "error", err.Error())
		return c.Store.Transition(m, StateBlocked, "coordinator", "incident agent de réception")
	}
	switch out.Decision {
	case VerdictAccepted:
		if err := c.Store.Transition(m, StateAwaitingApproval, "openai", "réception favorable : "+out.Summary); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:review-ok", m.ID, "réception favorable ; en attente de validation humaine (/aic approve)")
	case VerdictCorrection:
		n, err := c.Store.BumpCorrections(m)
		if err != nil {
			return err
		}
		if n > c.Config.MaxCorrections {
			if err := c.Store.Transition(m, StateBlocked, "coordinator", "plafond de corrections atteint"); err != nil {
				return err
			}
			return c.githubPostComment(m.ID+":comment:corrections-cap", m.ID, "mission bloquée : plafond de corrections atteint")
		}
		corrKey := m.ID + ":changes:" + strconv.Itoa(m.Corrections)
		if ig, ok := c.GitHub.(IdempotentGitHub); ok {
			if _, err := c.githubApply(corrKey, m.ID, "request_changes", func() (string, error) {
				return "", ig.RequestChangesIdem(corrKey, prRef, joinComments(out.Comments))
			}); err != nil {
				return err
			}
		} else if err := c.GitHub.RequestChanges(prRef, joinComments(out.Comments)); err != nil {
			return err
		}
		if err := c.Store.Transition(m, StateCorrection, "openai", "correction demandée : "+out.Summary); err != nil {
			return err
		}
		return c.githubPostComment(m.ID+":comment:correction:"+strconv.Itoa(m.Corrections), m.ID, "correction demandée (attachée à "+prRef+")")
	case VerdictBlocked:
		return c.Store.Transition(m, StateBlocked, "openai", "réserve bloquante")
	}
	return fmt.Errorf("verdict inconnu de l'agent de réception : %s", out.Decision)
}

// Resume redirige les missions en attente après une interruption. La
// déduplication d'événements et le contrôle d'état garantissent qu'aucune
// étape n'est dupliquée : une mission en REVIEW ne subit pas une nouvelle
// exécution de l'agent Mistral.
func (c *Coordinator) Resume(ctx context.Context) error {
	pending, err := c.Store.PendingMissions()
	if err != nil {
		return err
	}
	for _, m := range pending {
		_ = c.Store.Log(m.ID, "coordinator", "resume", "redémarrage : état "+m.State)
		switch m.State {
		case StateOpen:
			// pas de démarrage automatique : le commandement est humain.
		case StateRunning:
			// exécution interrompue : reprise du verrou abandonné (un verrou
			// vivant n'est jamais doublé), puis relance depuis le début de
			// l'exécution. La persistance des verrous fait qu'un crash ne
			// peut jamais produire deux exécutions concurrentes de la mission.
			if ok, err := c.Store.ReclaimStaleClaim(m.ID, c.Config.RunStaleInterval); err == nil && ok {
				_ = c.Store.Log(m.ID, "coordinator", "resume", "verrou périmé repris après crash")
			}
			if err := c.RunToReview(ctx, m); err != nil {
				if err != ErrMissionLocked {
					return err
				}
				_ = c.Store.Log(m.ID, "coordinator", "resume", "exécution encore verrouillée : relance différée")
			}
		case StateReview, StateCorrection, StateAwaitingApproval:
			// pas de relance automatique : en attente de la prochaine commande humaine.
		}
	}
	return nil
}

func joinComments(cs []ReviewComment) string {
	var b strings.Builder
	for i, cm := range cs {
		if i > 0 {
			b.WriteString(" ; ")
		}
		b.WriteString(cm.Ref)
		b.WriteString(": ")
		b.WriteString(cm.Note)
	}
	return b.String()
}

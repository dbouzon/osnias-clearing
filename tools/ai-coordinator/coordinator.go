package aic

import (
	"context"
	"fmt"
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
	bodyNorm := strings.ToLower(strings.TrimSpace(body))
	id := MissionID(issueNumber)
	m, err := c.Store.CreateMission(id, issueNumber, issueTitle)
	if err != nil {
		return err
	}
	switch {
	case strings.Contains(bodyNorm, CmdStart):
		if m.State != StateOpen {
			return nil // déjà démarrée ou terminée : idempotent
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
		return c.GitHub.PostComment(m.ID, "mission réceptionnée (ACCEPTED) après validation humaine")
	case strings.Contains(bodyNorm, CmdBlock):
		if Terminal(m.State) {
			return nil
		}
		if err := c.Store.Transition(m, StateBlocked, "human", "blocage demandé"); err != nil {
			return err
		}
		return c.GitHub.PostComment(m.ID, "mission bloquée par intervention humaine")
	}
	return nil
}

// RunToReview exécute l'agent Mistral (répertoire isolé, bornes appliquées),
// enregistre la livraison Draft, puis soumet le résultat à l'agent OpenAI.
func (c *Coordinator) RunToReview(ctx context.Context, m *Mission) error {
	workDir := fmt.Sprintf("%s/%s", strings.TrimRight(c.Config.WorkRoot, "/"), m.ID)
	if m.State == StateOpen {
		if err := c.Store.Transition(m, StateRunning, "coordinator", "exécution agent Mistral"); err != nil {
			return err
		}
	}
	if m.State != StateRunning {
		return fmt.Errorf("mission %s : état %s inattendu pour exécution", m.ID, m.State)
	}
	runCtx, cancel := context.WithTimeout(ctx, c.Config.MistralTimeout)
	defer cancel()

	res, runErr := c.Mistral.Run(runCtx, m, workDir)
	if runErr != nil {
		_ = c.Store.Log(m.ID, "mistral", "error", runErr.Error())
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "incident d'exécution agent"); err != nil {
			return err
		}
		return c.GitHub.PostComment(m.ID, "mission bloquée : incident d'exécution de l'agent développeur")
	}
	// contrôles de budget et d'itérations (T02-D)
	if res.Steps > c.Config.MistralBudgetSteps {
		_ = c.Store.Log(m.ID, "mistral", "budget", fmt.Sprintf("steps=%d > budget=%d", res.Steps, c.Config.MistralBudgetSteps))
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "dépassement de budget"); err != nil {
			return err
		}
		return c.GitHub.PostComment(m.ID, "mission bloquée : dépassement du budget d'exécution")
	}
	if res.Iters > c.Config.MistralMaxIter {
		if err := c.Store.Transition(m, StateBlocked, "coordinator", "dépassement d'itérations"); err != nil {
			return err
		}
		return c.GitHub.PostComment(m.ID, "mission bloquée : dépassement du nombre d'itérations")
	}
	// Côté agent : aucun push. La livraison est une Draft, enregistrée ici.
	_ = ErrNoPushByAgent
	prRef, err := c.GitHub.CreateDraftPR(m.ID, "Livraison "+m.ID, res.Report)
	if err != nil {
		return err
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
		return c.GitHub.PostComment(m.ID, "réception favorable ; en attente de validation humaine (/aic approve)")
	case VerdictCorrection:
		n, err := c.Store.BumpCorrections(m)
		if err != nil {
			return err
		}
		if n > c.Config.MaxCorrections {
			if err := c.Store.Transition(m, StateBlocked, "coordinator", "plafond de corrections atteint"); err != nil {
				return err
			}
			return c.GitHub.PostComment(m.ID, "mission bloquée : plafond de corrections atteint")
		}
		if err := c.GitHub.RequestChanges(prRef, joinComments(out.Comments)); err != nil {
			return err
		}
		if err := c.Store.Transition(m, StateCorrection, "openai", "correction demandée : "+out.Summary); err != nil {
			return err
		}
		return c.GitHub.PostComment(m.ID, "correction demandée (attachée à "+prRef+")")
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
			// exécution interrompue : relance depuis le début de l'exécution.
			if err := c.RunToReview(ctx, m); err != nil {
				return err
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

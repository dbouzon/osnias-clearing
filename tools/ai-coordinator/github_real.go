package aic

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v93/github"
)

// RealGitHub est l'adaptateur GitHub réel (T03). Il réalise GitHubClient et
// IdempotentGitHub sur la bibliothèque google/go-github. Il est DÉSACTIVÉ PAR
// DÉFAUT : il n'est construit que si AIC_GITHUB_MODE=real (voir cmd/aic-coordinator).
//
// Règles de sécurité :
//   - authentification par jeton à permissions minimales (lecture Issues/PR et
//     écriture Issues/PR uniquement) ; aucun secret n'est journalisé ;
//   - l'adaptateur n'expose et n'utilise AUCUNE opération de fusion (merge) :
//     la fusion reste une action humaine hors du périmètre du coordinateur ;
//   - chaque écriture porte une clé d'idempotence déterministe, enregistrée
//     durablement AVANT l'effet (write-ahead, T02-R5), et est précédée d'une
//     recherche de l'effet déjà réalisé côté GitHub (search-before-create) : les
//     commentaires et demandes de changement portent un marqueur HTML invisible
//     contenant la clé ; les PR Draft utilisent une branche de tête déterministe,
//     ce qui rend une seconde création impossible (le serveur la refuse).
//
// Garantie et limite (documentées au rapport de sécurité) : la combinaison
// write-ahead + recherche avant création procure une déduplication effective
// (exactly-once observé côté GitHub, at-least-once en appels) ; elle N'EST PAS
// une transaction atomique avec l'API distante : une panne entre deux requêtes
// peut déclencher une réémission, que la recherche avant création transforme
// en reconnaissance de l'effet déjà produit ; en cas d'ambiguïté réseau non
// résolue (ni marqueur trouvé, ni création confirmée), l'opération reste en
// erreur et sera rejouée ; aucun exactly-once absolu n'est revendiqué.

const (
	ghMarkerPrefix = "<!-- osnias-aic-idem: "
	ghMarkerSuffix = " -->"
)

func ghMarker(key string) string { return ghMarkerPrefix + key + ghMarkerSuffix }

func ghMarkerOf(body string) string {
	if i := strings.Index(body, ghMarkerPrefix); i >= 0 {
		if j := strings.Index(body[i:], ghMarkerSuffix); j > len(ghMarkerPrefix) {
			return body[i+len(ghMarkerPrefix) : i+j]
		}
	}
	return ""
}

// RealGitHubOptions regroupe les paramètres de construction de l'adaptateur réel.
type RealGitHubOptions struct {
	Token   string        // jeton à permissions minimales ; jamais journalisé
	APIURL  string        // racine COMPLÈTE de l'API (vide = github.com ; GHE : https://hote/api/v3)
	Repo    string        // "owner/name"
	Timeout time.Duration // délai par requête (défaut 30 s)
}

// RealGitHub réalise GitHubClient et IdempotentGitHub via go-github.
type RealGitHub struct {
	client   *gh.Client
	owner    string
	name     string
	keyStore KeyStore // registre durable de déduplication (T02-R4/R5)
	timeout  time.Duration
}

// NewRealGitHub construit l'adaptateur réel. Le jeton n'est stocké que dans le
// client HTTP, jamais dans les journaux ni la base.
func NewRealGitHub(opts RealGitHubOptions, ks KeyStore) (*RealGitHub, error) {
	repo := strings.TrimSpace(opts.Repo)
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("dépôt GitHub invalide (attendu owner/name) : %q", repo)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	hc := &http.Client{Timeout: timeout}
	optsFns := []gh.ClientOptionsFunc{
		gh.WithHTTPClient(hc),
		gh.WithAuthToken(opts.Token),
		gh.WithUserAgent("osnias-aic-coordinator"),
	}
	// AIC_GITHUB_APIURL désigne la racine COMPLÈTE de l'API (pour GitHub
	// Entreprise : https://hote/api/v3 ; pour github.com : laisser vide).
	if opts.APIURL != "" {
		u := opts.APIURL
		optsFns = append(optsFns, gh.WithURLs(&u, &u))
	}
	cl, err := gh.NewClient(optsFns...)
	if err != nil {
		return nil, fmt.Errorf("client GitHub invalide : %w", err)
	}
	return &RealGitHub{client: cl, owner: parts[0], name: parts[1], keyStore: ks, timeout: timeout}, nil
}

func (g *RealGitHub) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), g.timeout)
}

func (g *RealGitHub) issueOf(missionID string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(missionID, "AIC-"))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("identifiant de mission non convertible en issue : %q", missionID)
	}
	return n, nil
}

// markDurable persiste la clé AVANT l'effet (write-ahead, T02-R5) et propage
// l'échec : aucune écriture distante n'est tentée si la persistance échoue.
func (g *RealGitHub) markDurable(key string) error {
	if g.keyStore == nil {
		return nil
	}
	return g.keyStore.GitHubMarkKey(key)
}

// findIssueComment recherche un commentaire de l'Issue contenant le marqueur
// de la clé (lecture avant écriture : reprise des fenêtres d'interruption).
func (g *RealGitHub) findIssueComment(missionID, key string) (bool, error) {
	issue, err := g.issueOf(missionID)
	if err != nil {
		return false, err
	}
	opts := &gh.IssueListCommentsOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		ctx, cancel := g.ctx()
		page, resp, err := g.client.Issues.ListComments(ctx, g.owner, g.name, issue, opts)
		cancel()
		if err != nil {
			return false, err
		}
		for _, c := range page {
			if ghMarkerOf(c.GetBody()) == key {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			return false, nil
		}
		opts.Page = resp.NextPage
	}
}

func (g *RealGitHub) findDraftPR(missionID string) (*gh.PullRequest, error) {
	head := ghDraftBranch(missionID)
	opts := &gh.PullRequestListOptions{State: "open", ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		ctx, cancel := g.ctx()
		page, resp, err := g.client.PullRequests.List(ctx, g.owner, g.name, opts)
		cancel()
		if err != nil {
			return nil, err
		}
		for _, pr := range page {
			if pr.GetHead().GetRef() == head && pr.GetDraft() {
				return pr, nil
			}
		}
		if resp.NextPage == 0 {
			return nil, nil
		}
		opts.Page = resp.NextPage
	}
}

func (g *RealGitHub) findReview(prRef, key string) (bool, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(prRef, "PR-"))
	if err != nil || n <= 0 {
		return false, fmt.Errorf("référence de PR non convertible : %q", prRef)
	}
	opts := &gh.ListOptions{PerPage: 100}
	for {
		ctx, cancel := g.ctx()
		page, resp, err := g.client.PullRequests.ListReviews(ctx, g.owner, g.name, n, opts)
		cancel()
		if err != nil {
			return false, err
		}
		for _, r := range page {
			if ghMarkerOf(r.GetBody()) == key {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			return false, nil
		}
		opts.Page = resp.NextPage
	}
}

// PostComment publie un commentaire GitHub (entrée GitHubClient ; le
// coordinateur passe toujours par la clé d'idempotence interne).
func (g *RealGitHub) PostComment(missionID, body string) error {
	return g.PostCommentIdem(missionID+":write", missionID, body)
}

// PostCommentIdem publie un commentaire dédupliqué : recherche du marqueur
// de la clé côté GitHub avant toute écriture, write-ahead en base, puis
// création. Les erreurs réseau ambiguës laissent l'opération rejouable.
func (g *RealGitHub) PostCommentIdem(idemKey, missionID, body string) error {
	found, err := g.findIssueComment(missionID, idemKey)
	if err != nil {
		return fmt.Errorf("recherche du commentaire existant : %w", err)
	}
	if found {
		if err := g.markDurable("c:" + idemKey); err != nil {
			return err
		}
		return nil
	}
	// write-ahead : la clé est persistée avant l'effet ; en cas d'échec
	// d'écriture en base, aucune requête distante n'est émise (T02-R5).
	if err := g.markDurable("c:" + idemKey); err != nil {
		return err
	}
	issue, err := g.issueOf(missionID)
	if err != nil {
		return err
	}
	cm := gh.IssueCommentRequest{Body: body + "\n\n" + ghMarker(idemKey)}
	ctx, cancel := g.ctx()
	_, _, err = g.client.Issues.CreateComment(ctx, g.owner, g.name, issue, cm)
	cancel()
	return err
}

// ghDraftBranch : branche de tête déterministe d'une livraison de mission.
// Le déterminisme rend une seconde PR pour la même mission impossible (le
// serveur refuse head+base identiques), même sans registre local.
func ghDraftBranch(missionID string) string {
	return "aic/" + strings.ToLower(missionID) + "/draft"
}

// CreateDraftPR publie une PR Draft (entrée GitHubClient).
func (g *RealGitHub) CreateDraftPR(missionID, title, body string) (string, error) {
	return g.CreateDraftPRIdem(missionID+":write", missionID, title, body)
}

// CreateDraftPRIdem crée une PR Draft dédupliquée : recherche de la
// livraison existante (branche déterministe), write-ahead, puis création ;
// une réponse ambiguë après création enregistrée est résolue par la recherche
// au rejeu, jamais par une seconde création.
func (g *RealGitHub) CreateDraftPRIdem(idemKey, missionID, title, body string) (string, error) {
	pr, err := g.findDraftPR(missionID)
	if err != nil {
		return "", fmt.Errorf("recherche de la PR Draft existante : %w", err)
	}
	if pr != nil {
		if err := g.markDurable("d:" + idemKey); err != nil {
			return "", err
		}
		return prRefOf(pr.GetNumber()), nil
	}
	if err := g.markDurable("d:" + idemKey); err != nil {
		return "", err
	}
	issue, err := g.issueOf(missionID)
	if err != nil {
		return "", err
	}
	newPR := gh.CreatePullRequest{
		Title: gh.String(title),
		Head:  ghDraftBranch(missionID),
		Base:  "main",
		Body:  gh.String(body + "\n\n" + ghMarker(idemKey)),
		Draft: gh.Bool(true),
		// Issue : rattache la livraison à l'Issue de mission.
		Issue: gh.Int(issue),
	}
	ctx, cancel := g.ctx()
	created, _, err := g.client.PullRequests.Create(ctx, g.owner, g.name, newPR)
	cancel()
	if err != nil {
		// un doublon concurrent (rejeu après réponse réseau perdue par
		// exemple) se manifeste par un refus serveur : la recherche
		// l'attribue alors à la livraison déjà créée, sans duplication.
		if ghErr, ok := err.(*gh.ErrorResponse); ok && ghErr.Response != nil &&
			(ghErr.Response.StatusCode == http.StatusUnprocessableEntity || ghErr.Response.StatusCode == http.StatusConflict) {
			pr2, ferr := g.findDraftPR(missionID)
			if ferr == nil && pr2 != nil {
				if derr := g.markDurable("d:" + idemKey); derr != nil {
					return "", derr
				}
				return prRefOf(pr2.GetNumber()), nil
			}
		}
		return "", err
	}
	return prRefOf(created.GetNumber()), nil
}

func prRefOf(n int) string { return fmt.Sprintf("PR-%d", n) }

// RequestChanges rattache une demande de correction (entrée GitHubClient).
func (g *RealGitHub) RequestChanges(prRef, body string) error {
	return g.RequestChangesIdem(prRef+":write", prRef, body)
}

// RequestChangesIdem publie une revue REQUEST_CHANGES dédupliquée par marqueur.
func (g *RealGitHub) RequestChangesIdem(idemKey, prRef, body string) error {
	found, err := g.findReview(prRef, idemKey)
	if err != nil {
		return fmt.Errorf("recherche de la revue existante : %w", err)
	}
	if found {
		if err := g.markDurable("r:" + idemKey); err != nil {
			return err
		}
		return nil
	}
	if err := g.markDurable("r:" + idemKey); err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimPrefix(prRef, "PR-"))
	if err != nil || n <= 0 {
		return fmt.Errorf("référence de PR non convertible : %q", prRef)
	}
	rv := &gh.PullRequestReviewRequest{
		Body:  gh.String(body + "\n\n" + ghMarker(idemKey)),
		Event: gh.String("REQUEST_CHANGES"),
	}
	ctx, cancel := g.ctx()
	_, _, err = g.client.PullRequests.CreateReview(ctx, g.owner, g.name, n, rv)
	cancel()
	return err
}

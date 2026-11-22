package aic

// Tests contractuels T03 de l'adaptateur GitHub réel. Aucun accès réseau
// réel : le contrat go-github est vérifié sur un serveur HTTP simulé
// (httptest), avec injection de pannes (500), réponses ambiguës (traitement
// serveur puis 502), dépassements de délai et rejeux après redémarrage.
// L'objectif n'est PAS l'interopérabilité complète avec l'API GitHub
// réelle (réserves du rapport), mais la maîtrise des doublons distants.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHubServer simule les points d'API utilisés par l'adaptateur :
// Issues.ListComments/CreateComment, PullRequests.List/Create,
// PullRequests.ListReviews/CreateReview.
type fakeGitHubServer struct {
	mu        sync.Mutex
	srv       *httptest.Server
	comments  map[int][]fakeComment // par numéro d'issue
	prs       []fakePR
	reviews   map[int][]fakeComment // par numéro de PR
	nextID    int
	postCount int  // nombre total de requêtes POST reçues
	failPOST  int  // n POST échouent (500) avant succès, décrémenté
	ambiguous bool // POST traité puis réponse 502 (réponse réseau ambiguë)
	delay     time.Duration
	sawAuth   int // requêtes porteuses d'une autorisation valide
	seen      int // requêtes totales
}

type fakeComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

type fakePR struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	Head   *fakeHead `json:"head"`
	Draft  bool      `json:"draft"`
	Issue  int       `json:"issue"`
}
type fakeHead struct {
	Ref string `json:"ref"`
}

func newFakeGitHubServer(t *testing.T) *fakeGitHubServer {
	t.Helper()
	f := &fakeGitHubServer{
		comments: map[int][]fakeComment{},
		reviews:  map[int][]fakeComment{},
		nextID:   100,
	}
	// Répartiteur unique à dispatch par suffixe : les chemins du client
	// réel portent un préfixe BaseURL variable, on cible le suffixe
	// canonique de chaque point d'API.
	mux := http.NewServeMux()
	pullList := func(p string) bool {
		return p == "/repos/o/r/pulls" || strings.HasSuffix(p, "/repos/o/r/pulls")
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/comments"):
			nEnd := strings.LastIndex(p, "/comments")
			issue, err := strconv.Atoi(p[strings.LastIndex(p[:nEnd], "/")+1 : nEnd])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodGet:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				writeJSON(w, f.comments[issue])
			case http.MethodPost:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				if !f.accept(w, r) {
					return
				}
				var req struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				c := fakeComment{ID: int64(f.nextID), Body: req.Body}
				f.nextID++
				f.comments[issue] = append(f.comments[issue], c)
				f.ambiguousReply(w)
				writeJSON(w, c)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case pullList(p):
			switch r.Method {
			case http.MethodGet:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				writeJSON(w, f.prs)
			case http.MethodPost:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				if !f.accept(w, r) {
					return
				}
				var req struct {
					Title string `json:"title"`
					Head  string `json:"head"`
					Base  string `json:"base"`
					Body  string `json:"body"`
					Draft bool   `json:"draft"`
					Issue int    `json:"issue"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				// GitHub refuse une seconde PR ouverte pour une même branche
				// de tête : le simulateur reproduit ce refus 422.
				for _, pr := range f.prs {
					if pr.Head != nil && pr.Head.Ref == req.Head {
						w.WriteHeader(http.StatusUnprocessableEntity)
						fmt.Fprintln(w, `{"message":"A pull request already exists"}`)
						return
					}
				}
				f.nextID++
				newPR := fakePR{Number: f.nextID, Title: req.Title, Head: &fakeHead{Ref: req.Head}, Draft: req.Draft, Issue: req.Issue}
				f.prs = append(f.prs, newPR)
				f.ambiguousReply(w)
				writeJSON(w, newPR)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(p, "/reviews"):
			nEnd := strings.LastIndex(p, "/reviews")
			n, err := strconv.Atoi(p[strings.LastIndex(p[:nEnd], "/")+1 : nEnd])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodGet:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				writeJSON(w, f.reviews[n])
			case http.MethodPost:
				f.mu.Lock()
				defer f.mu.Unlock()
				f.note(r)
				if !f.accept(w, r) {
					return
				}
				var req struct {
					Body  string `json:"body"`
					Event string `json:"event"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if req.Event != "REQUEST_CHANGES" {
					w.WriteHeader(http.StatusUnprocessableEntity)
					return
				}
				c := fakeComment{ID: int64(f.nextID), Body: req.Body}
				f.nextID++
				f.reviews[n] = append(f.reviews[n], c)
				f.ambiguousReply(w)
				writeJSON(w, c)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(w, r)
		}
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// note enregistre la requête et vérifie l'en-tête d'autorification.
func (f *fakeGitHubServer) note(r *http.Request) {
	f.seen++
	if r.Header.Get("Authorization") == "Bearer tok-T03-x" {
		f.sawAuth++
	}
}

// accept compte le POST, applique le délai et échoue les n premiers POST
// (500) si l'injection de panne est armée.
func (f *fakeGitHubServer) accept(w http.ResponseWriter, r *http.Request) bool {
	f.postCount++
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.failPOST > 0 {
		f.failPOST--
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"injected failure"}`)
		return false
	}
	return true
}

// ambiguousReply offre, une fois armée, une réponse 502 ALORS que la
// ressource serveur a déjà été créée : réponse réseau ambiguë.
func (f *fakeGitHubServer) ambiguousReply(w http.ResponseWriter) {
	if f.ambiguous {
		f.ambiguous = false
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintln(w, `{"message":"ambiguous gateway timeout"}`)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// newRealForTest construit un adaptateur réel pointé sur le simulateur.
func newRealForTest(t *testing.T, f *fakeGitHubServer, ks KeyStore) *RealGitHub {
	t.Helper()
	g, err := NewRealGitHub(RealGitHubOptions{
		Token:   "tok-T03-x",
		APIURL:  f.srv.URL,
		Repo:    "o/r",
		Timeout: 30 * time.Second,
	}, ks)
	if err != nil {
		t.Fatalf("NewRealGitHub : %v", err)
	}
	return g
}

// T03-B : l'adaptateur réel est désactivé par défaut (mode "off") et refuse
// un dépôt mal formé.
func TestT03RealDisabledByDefault(t *testing.T) {
	t.Setenv("AIC_GITHUB_MODE", "")
	cfg := ConfigFromEnv()
	if cfg.GitHubMode != "off" {
		t.Fatalf("mode GitHub par défaut attendu off, obtenu %q", cfg.GitHubMode)
	}
	if _, err := NewRealGitHub(RealGitHubOptions{Repo: "invalide"}, nil); err == nil {
		t.Fatal("un dépôt sans owner/name doit être refusé")
	}
}

// T03-C : un commentaire est créé une seule fois par clé, authentification
// présente sur chaque requête, déduplication conservée après un
// redémarrage complet (même base SQLite, nouvel adaptateur).
func TestT03CommentIdempotentAndAuth(t *testing.T) {
	f := newFakeGitHubServer(t)
	db := storePath(t)
	store := openStoreT(t, db)
	g := newRealForTest(t, f, store)

	if err := g.PostCommentIdem("k1", "AIC-5", "premier commentaire"); err != nil {
		t.Fatalf("PostCommentIdem : %v", err)
	}
	// Rejeu immédiat : la recherche avant création évite le doublon.
	if err := g.PostCommentIdem("k1", "AIC-5", "premier commentaire"); err != nil {
		t.Fatalf("rejeu PostCommentIdem : %v", err)
	}
	// Redémarrage complet : même base, nouvel adaptateur.
	store.Close()
	store2 := openStoreT(t, db)
	g2 := newRealForTest(t, f, store2)
	if err := g2.PostCommentIdem("k1", "AIC-5", "premier commentaire"); err != nil {
		t.Fatalf("rejeu après redémarrage : %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.comments[5]) != 1 {
		t.Fatalf("un seul commentaire attendu, obtenu %d", len(f.comments[5]))
	}
	body := f.comments[5][0].Body
	if !strings.Contains(body, ghMarker("k1")) {
		t.Fatalf("marqueur d'idempotence absent : %q", body)
	}
	if f.postCount != 1 {
		t.Fatalf("une seule écriture attendue (recherche = lecture), POST=%d", f.postCount)
	}
	if f.sawAuth != f.seen || f.seen == 0 {
		t.Fatalf("autorification absente : %d requêtes, %d authentifiées", f.seen, f.sawAuth)
	}
}

// T03-C : une erreur serveur 500 laisse l'opération rejouable ; le rejeu ne
// crée pas de doublon.
func TestT03CommentServerErrorThenRetry(t *testing.T) {
	f := newFakeGitHubServer(t)
	f.failPOST = 1 // le premier POST échoue
	store := newTestStore(t)
	g := newRealForTest(t, f, store)

	if err := g.PostCommentIdem("k2", "AIC-5", "commentaire"); err == nil {
		t.Fatal("l'échec serveur 500 doit être propagé")
	}
	if err := g.PostCommentIdem("k2", "AIC-5", "commentaire"); err != nil {
		t.Fatalf("rejeu après 500 : %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.comments[5]) != 1 {
		t.Fatalf("un seul commentaire attendu après rejeu, obtenu %d", len(f.comments[5]))
	}
}

// T03-C : réponse réseau ambiguë (création enregistrée puis 502) : le rejeu
// reconnaît l'effet déjà produit, sans dupliquer.
func TestT03CommentAmbiguousResponse(t *testing.T) {
	f := newFakeGitHubServer(t)
	f.ambiguous = true
	store := newTestStore(t)
	g := newRealForTest(t, f, store)

	if err := g.PostCommentIdem("k3", "AIC-5", "commentaire ambigu"); err == nil {
		t.Fatal("la réponse 502 doit être propagée (résultat incertain)")
	}
	f.mu.Lock()
	if len(f.comments[5]) != 1 {
		t.Fatalf("le simulateur doit avoir enregistré le commentaire : %d", len(f.comments[5]))
	}
	f.mu.Unlock()
	// Rejeu : le marqueur est trouvé côté GitHub, aucun doublon. La seule
	// écriture émise reste celle de l'appel ambigu (POST=1).
	if err := g.PostCommentIdem("k3", "AIC-5", "commentaire ambigu"); err != nil {
		t.Fatalf("rejeu après réponse ambiguë : %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.comments[5]) != 1 {
		t.Fatalf("aucun doublon attendu après rejeu, obtenu %d", len(f.comments[5]))
	}
	if f.postCount != 1 {
		t.Fatalf("une seule écriture émise attendue, obtenu %d", f.postCount)
	}
}

// T03-B/T03-C : PR Draft déterministe : rejeu sans doublon, réponse ambiguë
// résolue par la recherche, même référence restituée.
func TestT03DraftPRIdempotent(t *testing.T) {
	f := newFakeGitHubServer(t)
	store := newTestStore(t)
	g := newRealForTest(t, f, store)

	ref, err := g.CreateDraftPRIdem("k4", "AIC-5", "Livraison M-AIC-01", "corps")
	if err != nil {
		t.Fatalf("CreateDraftPRIdem : %v", err)
	}
	if ref == "" {
		t.Fatal("référence de PR vide")
	}
	ref2, err := g.CreateDraftPRIdem("k4", "AIC-5", "Livraison M-AIC-01", "corps")
	if err != nil {
		t.Fatalf("rejeu CreateDraftPRIdem : %v", err)
	}
	if ref2 != ref {
		t.Fatalf("le rejeu doit restituer la même référence : %q puis %q", ref, ref2)
	}
	f.mu.Lock()
	if len(f.prs) != 1 {
		t.Fatalf("une seule PR attendue, obtenu %d", len(f.prs))
	}
	if f.prs[0].Head.Ref != ghDraftBranch("AIC-5") {
		t.Fatalf("branche de tête déterministe attendue %q, obtenue %q", ghDraftBranch("AIC-5"), f.prs[0].Head.Ref)
	}
	if !f.prs[0].Draft {
		t.Fatal("la PR doit être en Draft")
	}
	f.mu.Unlock()

	// Réponse ambiguë sur une seconde mission : 502 après création.
	f.ambiguous = true
	if _, err := g.CreateDraftPRIdem("k5", "AIC-7", "Livraison 7", "corps"); err == nil {
		t.Fatal("la réponse ambiguë doit être propagée (résultat incertain)")
	}
	f.mu.Lock()
	n := len(f.prs)
	f.mu.Unlock()
	if n != 2 {
		t.Fatalf("la PR a été créée malgré le 502 : %d PR", n)
	}
	ref7, err := g.CreateDraftPRIdem("k5", "AIC-7", "Livraison 7", "corps")
	if err != nil {
		t.Fatalf("rejeu après réponse ambiguë : %v", err)
	}
	if !strings.HasPrefix(ref7, "PR-") {
		t.Fatalf("référence invalide : %q", ref7)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.prs) != 2 {
		t.Fatalf("aucun doublon de PR attendu, obtenu %d", len(f.prs))
	}
}

// T03-B : la demande de changement est une revue REQUEST_CHANGES unique par
// clé.
func TestT03RequestChangesIdempotent(t *testing.T) {
	f := newFakeGitHubServer(t)
	store := newTestStore(t)
	g := newRealForTest(t, f, store)

	if err := g.RequestChangesIdem("k6", "PR-101", "corrections demandées"); err != nil {
		t.Fatalf("RequestChangesIdem : %v", err)
	}
	if err := g.RequestChangesIdem("k6", "PR-101", "corrections demandées"); err != nil {
		t.Fatalf("rejeu RequestChangesIdem : %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reviews[101]) != 1 {
		t.Fatalf("une seule revue attendue, obtenu %d", len(f.reviews[101]))
	}
}

// T03-C : dépassement de délai propagé ; rejeu réussi après normalisation du
// réseau, sans doublon.
func TestT03TimeoutErrors(t *testing.T) {
	f := newFakeGitHubServer(t)
	store := newTestStore(t)
	g, err := NewRealGitHub(RealGitHubOptions{
		Token:   "tok-T03-x",
		APIURL:  f.srv.URL,
		Repo:    "o/r",
		Timeout: 60 * time.Millisecond,
	}, store)
	if err != nil {
		t.Fatalf("NewRealGitHub : %v", err)
	}
	f.delay = 400 * time.Millisecond
	if err := g.PostCommentIdem("k7", "AIC-5", "commentaire lent"); err == nil {
		t.Fatal("le dépassement de délai doit être propagé")
	}
	// Un dépassement de délai signifie un résultat INCERTAIN, pas une
	// absence d'effet : le serveur peut avoir traité la requête. Le rejeu
	// doit donc se remettre de la fenêtre ambiguë par la recherche avant
	// création, sans dupliquer.
	// Le simulateur achève la requête interrompue en arrière-plan (~400 ms
	// en tenant son verrou interne) : on attend la fin observable de cette
	// fenêtre ambiguë avant le rejeu.
	deadlineWait := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.comments[5])
		f.mu.Unlock()
		if n == 1 || time.Now().After(deadlineWait) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.delay = 0
	if err := g.PostCommentIdem("k7", "AIC-5", "commentaire lent"); err != nil {
		t.Fatalf("rejeu après délai : %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.comments[5]) != 1 {
		t.Fatalf("un seul commentaire attendu après rejeu, obtenu %d", len(f.comments[5]))
	}
}

// T03-C : registre local perdu (nouvelle base) alors que la PR Draft existe
// déjà : la recherche avant création restitue la livraison, le refus 422
// n'entraîne aucune duplication.
func TestT03DraftPR422Recovery(t *testing.T) {
	f := newFakeGitHubServer(t)
	// Une PR Draft préexiste côté GitHub (créée lors d'une exécution
	// antérieure dont le registre a été perdu).
	f.mu.Lock()
	f.prs = append(f.prs, fakePR{Number: 7, Title: "Livraison M-AIC-01", Head: &fakeHead{Ref: ghDraftBranch("AIC-9")}, Draft: true})
	f.mu.Unlock()

	g := newRealForTest(t, f, newTestStore(t)) // registre neuf : rien de marqué
	ref, err := g.CreateDraftPRIdem("k8", "AIC-9", "Livraison M-AIC-01", "corps")
	if err != nil {
		t.Fatalf("la livraison existante doit être restituée : %v", err)
	}
	if ref != "PR-7" {
		t.Fatalf("référence attendue PR-7, obtenue %q", ref)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.prs) != 1 {
		t.Fatalf("aucune nouvelle PR attendue, obtenu %d", len(f.prs))
	}
	if f.postCount != 0 {
		t.Fatalf("aucune écriture distante attendue, POST=%d", f.postCount)
	}
}

// --- utilitaires ---------------------------------------------------------

func storePath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/t03.sqlite"
}

func openStoreT(t *testing.T, path string) *Store {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%s) : %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

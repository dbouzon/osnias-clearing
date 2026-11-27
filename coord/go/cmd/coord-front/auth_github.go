// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Connexion GitHub et repo comme répertoire de mission — mission
// M-COORD-SRC-01 (spec v0.3) :
//
//   - GET  /api/auth/github/etat    : état (activé, github connecté) ;
//   - GET  /api/auth/github/login   : consentement GitHub (scope
//     public_repo UNIQUEMENT en v1 : repos publics, spec §3.2) ;
//   - GET  /api/auth/github/callback: échange du code, token stocké
//     SERVEUR (jamais front, jamais repo, jamais logs) ;
//   - GET  /api/github/repos        : repos publics (cache 60 s,
//     entêtes X-RateLimit lus, spec §5) ;
//   - POST /api/github/structure    : création missions/<nom>/entrée
//     et /sortie dans le repo choisi ;
//   - /api/missions/repertoire      : étendu au type github avec
//     contrôle 409 SOURCE_HETEROGENE (homogénéité vérifiée serveur).
//
// Fail-closed : sans section github.active dans gateway.json, les
// routes répondent 503 et le front ne propose pas la source GitHub.
// Stdlib uniquement.
package main

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	githubAuthURL    = "https://github.com/login/oauth/authorize"
	githubTokenURL   = "https://github.com/login/oauth/access_token"
	githubAPI        = "https://api.github.com"
	githubScope      = "public_repo" // v1 : repos publics uniquement (spec §3.2)
	githubAccept     = "application/vnd.github+json"
	githubUserAgent  = "coord-front-m-coord-src-01"
	tailleMaxFichier = 5 << 20 // refus > 5 Mo (spec §5)
)

// githubConf : section "github" de gateway.json, sur le modèle de
// "google". Secrets uniquement dans ce fichier (chmod 640 root:coord).
type githubConf struct {
	Activer      bool   `json:"activer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Redirect     string `json:"redirect"`
}

func (gc githubConf) redirectURL() string {
	if gc.Redirect != "" {
		return gc.Redirect
	}
	return "https://coord.anhydra.fr/api/auth/github/callback"
}

func (g *gateway) githubActif() bool {
	return g.cfg.GitHub.Activer && g.cfg.GitHub.ClientID != "" && g.cfg.GitHub.ClientSecret != ""
}

// ---------------------------------------------------------------------------
// Cache des tokens d'accès GitHub (borné et purgé comme les tokens
// Drive — spec §5, critique OpenAI v0.1) et des listings de repos
// (60 s, spec §5).
// ---------------------------------------------------------------------------

type cacheGitHub struct {
	mu   sync.Mutex
	toks map[string]struct {
		val string
		exp time.Time
	}
	repos map[string]struct {
		liste []map[string]string
		exp   time.Time
	}
}

var cacheGH = &cacheGitHub{
	toks: map[string]struct {
		val string
		exp time.Time
	}{},
	repos: map[string]struct {
		liste []map[string]string
		exp   time.Time
	}{},
}

func (c *cacheGitHub) purger() {
	now := time.Now()
	for s, t := range c.toks {
		if now.After(t.exp) {
			delete(c.toks, s)
		}
	}
	for s, r := range c.repos {
		if now.After(r.exp) {
			delete(c.repos, s)
		}
	}
}

func (g *gateway) tokenGitHub(u *utilisateur) (string, error) {
	cacheGH.mu.Lock()
	cacheGH.purger()
	if t, ok := cacheGH.toks[u.Sub]; ok {
		val := t.val
		cacheGH.mu.Unlock()
		return val, nil
	}
	cacheGH.mu.Unlock()
	if u.GitHubToken == "" {
		return "", fmt.Errorf("GitHub non connecté")
	}
	// Les tokens OAuth GitHub n'expirent pas : cache 24 h, renouvelé
	// en cas de refus 401 lors d'un appel (faite dans appelerGitHub).
	cacheGH.mu.Lock()
	cacheGH.toks[u.Sub] = struct {
		val string
		exp time.Time
	}{u.GitHubToken, time.Now().Add(24 * time.Hour)}
	cacheGH.mu.Unlock()
	return u.GitHubToken, nil
}

// appelerGitHub : appel authentifié avec lecture des entêtes
// X-RateLimit et messages explicites en cas de dépassement (spec §5).
func (g *gateway) appelerGitHub(u *utilisateur, methode, chemin string, corps io.Reader) (*http.Response, error) {
	tok, err := g.tokenGitHub(u)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(methode, githubAPI+chemin, corps)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("User-Agent", githubUserAgent)
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if rep.StatusCode == http.StatusUnauthorized {
		// Token révoqué côté GitHub : invalider le cache et refuser.
		cacheGH.mu.Lock()
		delete(cacheGH.toks, u.Sub)
		cacheGH.mu.Unlock()
		return rep, fmt.Errorf("token GitHub refusé : reconnectez GitHub")
	}
	if restant := rep.Header.Get("X-RateLimit-Remaining"); restant == "0" {
		reset := rep.Header.Get("X-RateLimit-Reset")
		return rep, fmt.Errorf("limite GitHub atteinte, réessayez après %s", reset)
	}
	return rep, nil
}

// ---------------------------------------------------------------------------
// Routes d'authentification GitHub.
// ---------------------------------------------------------------------------

func (g *gateway) authGithubEtat(w http.ResponseWriter, r *http.Request) {
	if !g.githubActif() {
		repondreJSON(w, http.StatusOK, map[string]any{"activer": false})
		return
	}
	connecte := false
	if u, ok := g.utilisateurRequete(r); ok {
		connecte = u.GitHubToken != ""
	}
	repondreJSON(w, http.StatusOK, map[string]any{"activer": true, "github_connecte": connecte})
}

func (g *gateway) authGithubLogin(w http.ResponseWriter, r *http.Request) {
	if !rythmeAutorise(r) {
		repondreErreur(w, http.StatusTooManyRequests, "RYTME_DEPASSE",
			"trop de tentatives, réessayez dans une minute")
		return
	}
	if !g.githubActif() {
		repondreErreur(w, http.StatusServiceUnavailable, "AUTH_GITHUB_DESACTIVE",
			"connexion GitHub non configurée (gateway.json)")
		return
	}
	// L'état signé embarque le sub de SESSION : le consentement GitHub
	// n'a de sens que pour une personne déjà connectée (repertoire de
	// mission, spec §3.2). Le sub est requis au callback.
	subEtat := ""
	if u, ok := g.utilisateurRequete(r); ok {
		subEtat = u.Sub
	}
	nonce := nonceAleatoire()
	etat := signerEtat(g.cfg.SecretSession, subEtat, nonce, time.Now().Add(dureeEtatOIDC))
	http.SetCookie(w, &http.Cookie{
		Name: cookieOIDC, Value: etat, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(dureeEtatOIDC.Seconds()),
	})
	v := url.Values{
		"client_id":    {g.cfg.GitHub.ClientID},
		"redirect_uri": {g.cfg.GitHub.redirectURL()},
		"scope":        {githubScope},
		"state":        {etat},
	}
	http.Redirect(w, r, githubAuthURL+"?"+v.Encode(), http.StatusFound)
}

func (g *gateway) authGithubCallback(w http.ResponseWriter, r *http.Request) {
	if !rythmeAutorise(r) {
		repondreErreur(w, http.StatusTooManyRequests, "RYTME_DEPASSE",
			"trop de tentatives, réessayez dans une minute")
		return
	}
	if !g.githubActif() {
		repondreErreur(w, http.StatusServiceUnavailable, "AUTH_GITHUB_DESACTIVE",
			"connexion GitHub non configurée")
		return
	}
	q := r.URL.Query()
	if err := q.Get("error"); err != "" {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_GITHUB_REFUSEE", "GitHub : "+err)
		return
	}
	cookie, err := r.Cookie(cookieOIDC)
	if err != nil || cookie.Value == "" {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_ABSENT", "état absent")
		return
	}
	subEtat, _, ok := verifierEtat(g.cfg.SecretSession, cookie.Value)
	if !ok {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_INVALIDE", "état invalide ou expiré")
		return
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(q.Get("state"))) != 1 {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_DIVERGENT", "état divergent")
		return
	}
	code := q.Get("code")
	if code == "" {
		repondreErreur(w, http.StatusBadRequest, "CODE_ABSENT", "code d'autorisation absent")
		return
	}
	tok, err := g.echangerCodeGitHub(code)
	if err != nil {
		repondreErreur(w, http.StatusUnauthorized, "ECHANGE_IMPOSSIBLE", err.Error())
		return
	}
	// Cookie d'état : usage unique.
	http.SetCookie(w, &http.Cookie{Name: cookieOIDC, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1})

	identite, err := g.identiteGitHub(tok)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "GITHUB_INJOIGNABLE", err.Error())
		return
	}
	if subEtat == "" {
		// Flux PRIMAIRE (règle du pilote, spec v0.4 §3.2) : la
		// première connexion GitHub crée le compte elle-même, sans
		// passer par Google ; session ouverte, redirection accueil.
		u, err := g.provisionner("gh:"+identite.id, "", identite.login, "")
		if err != nil {
			repondreErreur(w, http.StatusServiceUnavailable, "COMPTE_NON_PROVISIONNE", err.Error())
			return
		}
		if err := g.users.maj(func(m map[string]*utilisateur) error {
			if m[u.Sub] == nil {
				return fmt.Errorf("utilisateur disparu")
			}
			m[u.Sub].GitHubToken = tok
			m[u.Sub].GitHubLogin = identite.login
			return nil
		}); err != nil {
			repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
			return
		}
		g.poserCookieSession(w, u.Compte, u.Sub)
		http.Redirect(w, r, "/#/presentation", http.StatusFound)
		return
	}
	if err := g.users.maj(func(m map[string]*utilisateur) error {
		if m[subEtat] == nil {
			return fmt.Errorf("utilisateur disparu")
		}
		m[subEtat].GitHubToken = tok
		m[subEtat].GitHubLogin = identite.login
		return nil
	}); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	cacheGH.mu.Lock()
	delete(cacheGH.toks, subEtat)
	delete(cacheGH.repos, subEtat)
	cacheGH.mu.Unlock()
	http.Redirect(w, r, "/#/creation", http.StatusFound)
}

func (g *gateway) echangerCodeGitHub(code string) (string, error) {
	form := url.Values{
		"client_id":     {g.cfg.GitHub.ClientID},
		"client_secret": {g.cfg.GitHub.ClientSecret},
		"code":          {code},
		"redirect_uri":  {g.cfg.GitHub.redirectURL()},
	}
	req, _ := http.NewRequest(http.MethodPost, githubTokenURL,
		strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token GitHub : statut %d", rep.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&tok); err != nil {
		return "", err
	}
	if tok.Error != "" {
		return "", fmt.Errorf("GitHub : %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token GitHub absent")
	}
	return tok.AccessToken, nil
}

type identiteGitHubT struct {
	id    string
	login string
}

func (g *gateway) identiteGitHub(token string) (identiteGitHubT, error) {
	req, _ := http.NewRequest(http.MethodGet, githubAPI+"/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("User-Agent", githubUserAgent)
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return identiteGitHubT{}, err
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		return identiteGitHubT{}, fmt.Errorf("utilisateur GitHub : statut %d", rep.StatusCode)
	}
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&u); err != nil {
		return identiteGitHubT{}, err
	}
	if u.ID == 0 || u.Login == "" {
		return identiteGitHubT{}, fmt.Errorf("identité GitHub incomplète")
	}
	return identiteGitHubT{id: fmt.Sprintf("%d", u.ID), login: u.Login}, nil
}

// ---------------------------------------------------------------------------
// Repos publics (cache 60 s) et structure de mission.
// ---------------------------------------------------------------------------

func (g *gateway) githubRepos(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	if u.GitHubToken == "" {
		repondreErreur(w, http.StatusPreconditionRequired, "GITHUB_NON_CONNECTE",
			"connectez votre GitHub depuis l'écran de création de mission")
		return
	}
	cacheGH.mu.Lock()
	if c, ok := cacheGH.repos[u.Sub]; ok && time.Now().Before(c.exp) {
		liste := c.liste
		cacheGH.mu.Unlock()
		repondreJSON(w, http.StatusOK, map[string]any{"repos": liste})
		return
	}
	cacheGH.mu.Unlock()
	rep, err := g.appelerGitHub(u, http.MethodGet,
		"/user/repos?type=owner&sort=updated&per_page=100", nil)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "GITHUB_INJOIGNABLE", err.Error())
		return
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		repondreErreur(w, http.StatusBadGateway, "GITHUB_REFUS",
			fmt.Sprintf("GitHub : statut %d", rep.StatusCode))
		return
	}
	var brut []struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Private  bool   `json:"private"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 8<<20)).Decode(&brut); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	liste := []map[string]string{}
	for _, r := range brut {
		if r.Private {
			continue // v1 : repos publics uniquement (scope public_repo)
		}
		liste = append(liste, map[string]string{"id": r.FullName, "nom": r.FullName})
	}
	cacheGH.mu.Lock()
	cacheGH.repos[u.Sub] = struct {
		liste []map[string]string
		exp   time.Time
	}{liste, time.Now().Add(60 * time.Second)}
	cacheGH.mu.Unlock()
	repondreJSON(w, http.StatusOK, map[string]any{"repos": liste})
}

// githubStructure : POST {mission, repo} → crée missions/<mission>/
// entrée/.gitkeep et missions/<mission>/sortie/.gitkeep dans le repo.
// Fail-closed : toute réponse GitHub autre que 201/200 refuse, avec
// le motif explicite ; jamais de structure partielle silencieuse.
func (g *gateway) githubStructure(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	if u.GitHubToken == "" {
		repondreErreur(w, http.StatusPreconditionRequired, "GITHUB_NON_CONNECTE",
			"connectez votre GitHub d'abord")
		return
	}
	var req struct {
		Mission string `json:"mission"`
		Repo    string `json:"repo"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	mission := strings.TrimSpace(req.Mission)
	repo := strings.TrimSpace(req.Repo)
	if mission == "" || repo == "" {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", "mission et repo requis")
		return
	}
	if len(mission) > 120 || !nomMissionValide(mission) {
		repondreErreur(w, http.StatusBadRequest, "NOM_INVALIDE",
			"nom de mission invalide (lettres, chiffres, tirets, points)")
		return
	}
	if !strings.Contains(repo, "/") || len(repo) > 140 {
		repondreErreur(w, http.StatusBadRequest, "REPO_INVALIDE", "repo invalide (attendu : proprio/repo)")
		return
	}
	for _, sous := range []string{"entrée", "sortie"} {
		if err := g.creerFichierGitHub(u, repo, "missions/"+mission+"/"+sous+"/.gitkeep",
			"COORD : répertoire de mission "+mission+" ("+sous+")"); err != nil {
			repondreErreur(w, http.StatusBadGateway, "GITHUB_REFUS", err.Error())
			return
		}
	}
	dossier := repo + "/missions/" + mission
	repondreJSON(w, http.StatusOK, map[string]any{
		"id":  dossier,
		"nom": dossier,
	})
}

func (g *gateway) creerFichierGitHub(u *utilisateur, repo, chemin, message string) error {
	corps := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte("coord\n")),
	}
	raw, _ := json.Marshal(corps)
	rep, err := g.appelerGitHub(u, http.MethodPut,
		"/repos/"+repo+"/contents/"+url.PathEscape(chemin),
		strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusCreated && rep.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("GitHub : statut %d pour %s", rep.StatusCode, chemin)
		if rep.StatusCode == http.StatusUnprocessableEntity {
			msg = "chemin déjà existant ou refusé par GitHub : " + chemin
		}
		if rep.StatusCode == http.StatusNotFound {
			msg = "repo introuvable ou inaccessible : " + repo
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// nomMissionValide : caractères sûrs pour un chemin GitHub.
func nomMissionValide(nom string) bool {
	if nom == "" {
		return false
	}
	for _, c := range nom {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '.', c == '_':
		default:
			return false
		}
	}
	return true
}

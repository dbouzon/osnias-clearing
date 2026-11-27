// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Connexion Google (OIDC Authorization Code) — mission M-COORD-AUTH-01.
//
//   - GET  /api/auth/google/etat    : état (activé, drive connecté) ;
//   - GET  /api/auth/google/login   : consentement Google (openid email
//     profile ; ?drive=1 → scope drive.file, premier usage spec §3.5) ;
//   - GET  /api/auth/google/callback: échange du code, vérification
//     SERVEUR du id_token (RS256/JWKS, iss/aud/exp/nonce), provisioning
//     automatique, session ouverte ;
//   - GET  /api/moi                 : profil (nom, email, image, solde) ;
//   - GET  /api/drive/dossiers      : dossiers Drive de la personne ;
//   - POST /api/drive/dossier       : création du répertoire de mission ;
//   - *    /api/missions/repertoire : association mission → répertoire.
//
// Fail-closed : sans section google.active (client ID/secret dans
// gateway.json, jamais dans le dépôt), les routes répondent 503 et le
// front n'affiche pas le bouton. Stdlib uniquement.
package main

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	googleJWKSURL  = "https://www.googleapis.com/oauth2/v3/certs"
	googleIssuer1  = "accounts.google.com"
	googleIssuer2  = "https://accounts.google.com"

	dureeEtatOIDC = 10 * time.Minute   // cookie d'état anti-CSRF
	dureeSession  = 7 * 24 * time.Hour // session à expiration glissante

	cookieOIDC = "coord_oidc"
)

// googleConf : section "google" de gateway.json. Secrets uniquement
// dans ce fichier (chmod 600, hors dépôt). CompteNouveau = compte COORD
// attribué aux nouvelles personnes tant que l'automatisation complète
// des instances dédiées n'est pas en place ; Correspondance permet au
// pilote d'attribuer un compte précis par email (fail-closed sinon).
type googleConf struct {
	Activer        bool              `json:"activer"`
	ClientID       string            `json:"client_id"`
	ClientSecret   string            `json:"client_secret"`
	Redirect       string            `json:"redirect"`
	CompteNouveau  string            `json:"compte_nouveau"`
	Correspondance map[string]string `json:"correspondance"`
}

func (gc googleConf) redirectURL() string {
	if gc.Redirect != "" {
		return gc.Redirect
	}
	return "https://coord.anhydra.fr/api/auth/google/callback"
}

// utilisateur : une personne identifiée par Google. Clé = sub Google.
type utilisateur struct {
	Sub          string    `json:"sub"`
	Email        string    `json:"email"`
	Nom          string    `json:"nom"`
	Image        string    `json:"image"`
	Compte       string    `json:"compte"`
	Cree         time.Time `json:"cree"`
	DriveRefresh string    `json:"drive_refresh,omitempty"` // jamais envoyé au front
	GitHubToken  string    `json:"github_token,omitempty"`  // jamais envoyé au front
	GitHubLogin  string    `json:"github_login,omitempty"`
}

// ---------------------------------------------------------------------------
// Base locale des utilisateurs (persistance JSON atomique, chmod 600).
// Le grand livre monnaie reste derrière l'interface Livre (SOLDE-01).
// ---------------------------------------------------------------------------

type banque struct {
	mu     sync.Mutex
	chemin string
	users  map[string]*utilisateur
}

func nouvelleBanque(chemin string) (*banque, error) {
	b := &banque{chemin: chemin, users: map[string]*utilisateur{}}
	raw, err := os.ReadFile(chemin)
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &b.users); err != nil {
		return nil, fmt.Errorf("fichier utilisateurs invalide : %w", err)
	}
	return b, nil
}

func (b *banque) parSub(sub string) (*utilisateur, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	u, ok := b.users[sub]
	return u, ok
}

func (b *banque) maj(fn func(m map[string]*utilisateur) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := fn(b.users); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(b.users, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.chemin + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, b.chemin)
}

// ---------------------------------------------------------------------------
// JWKS Google (cache 24 h) et vérification RS256 du id_token.
// ---------------------------------------------------------------------------

type cleJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksGoogle struct {
	Keys []cleJWK `json:"keys"`
}

type cacheJWKS struct {
	mu   sync.Mutex
	cles map[string]*rsa.PublicKey
	exp  time.Time
}

var jwks = &cacheJWKS{}

func b64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func (c *cacheJWKS) charger() (map[string]*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cles) > 0 && time.Now().Before(c.exp) {
		return c.cles, nil
	}
	rep, err := http.Get(googleJWKSURL)
	if err != nil {
		return nil, err
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS Google : statut %d", rep.StatusCode)
	}
	var j jwksGoogle
	if err := json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&j); err != nil {
		return nil, err
	}
	cles := map[string]*rsa.PublicKey{}
	for _, k := range j.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err := b64url(k.N)
		if err != nil {
			continue
		}
		eb, err := b64url(k.E)
		if err != nil || len(eb) > 8 {
			continue
		}
		var exp uint64
		for _, o := range eb {
			exp = exp<<8 | uint64(o)
		}
		cles[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(exp)}
	}
	if len(cles) == 0 {
		return nil, fmt.Errorf("JWKS Google : aucune clé RSA")
	}
	c.cles = cles
	c.exp = time.Now().Add(24 * time.Hour)
	return cles, nil
}

// verifierJeton : RS256, iss, aud, exp. Fail-closed intégral.
func (g *gateway) verifierJeton(jeton string) (map[string]any, error) {
	parts := strings.Split(jeton, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jeton mal formé")
	}
	headerRaw, err := b64url(parts[0])
	if err != nil {
		return nil, err
	}
	var entete struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &entete); err != nil {
		return nil, err
	}
	if entete.Alg != "RS256" {
		return nil, fmt.Errorf("algorithme inattendu : %s", entete.Alg)
	}
	cles, err := jwks.charger()
	if err != nil {
		return nil, err
	}
	pub, ok := cles[entete.Kid]
	if !ok {
		return nil, fmt.Errorf("clé JWKS inconnue : %s", entete.Kid)
	}
	signe, err := b64url(parts[2])
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], signe); err != nil {
		return nil, fmt.Errorf("signature invalide")
	}
	claimsRaw, err := b64url(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return nil, err
	}
	if iss, _ := claims["iss"].(string); iss != googleIssuer1 && iss != googleIssuer2 {
		return nil, fmt.Errorf("émetteur inattendu : %v", iss)
	}
	if aud, _ := claims["aud"].(string); aud != g.cfg.Google.ClientID {
		return nil, fmt.Errorf("audience inattendue")
	}
	exp, _ := claims["exp"].(float64)
	if time.Now().Unix() > int64(exp) {
		return nil, fmt.Errorf("jeton expiré")
	}
	return claims, nil
}

// ---------------------------------------------------------------------------
// État OIDC signé : "exp.sub.nonce.mac" (anti-CSRF + nonce, usage unique).
// Le sub est EMBARQUÉ dans l'état pour le flux Drive : au retour de Google
// (navigation inter-site), le cookie SameSite=Strict n'est pas envoyé ;
// l'état HMAC-signé (10 min) identifie alors l'utilisateur en toute
// sécurité, sans rien exposer côté navigateur.
// ---------------------------------------------------------------------------

func signerEtat(secret, sub, nonce string, exp time.Time) string {
	charge := fmt.Sprintf("%d.%s.%s", exp.Unix(), sub, nonce)
	mac := hmac.New(sha256.New, []byte(secret+"coord-front-oidc-v1"))
	mac.Write([]byte(charge))
	return charge + "." + fmt.Sprintf("%x", mac.Sum(nil))
}

// verifierEtat : renvoie (sub, nonce) si l'état est valide et vivant.
func verifierEtat(secret, valeur string) (string, string, bool) {
	p := strings.Split(valeur, ".")
	if len(p) != 4 {
		return "", "", false
	}
	var exp int64
	if _, err := fmt.Sscanf(p[0], "%d", &exp); err != nil {
		return "", "", false
	}
	attendu := signerEtat(secret, p[1], p[2], time.Unix(exp, 0))
	if subtle.ConstantTimeCompare([]byte(valeur), []byte(attendu)) != 1 {
		return "", "", false
	}
	if time.Now().Unix() > exp {
		return "", "", false
	}
	return p[1], p[2], true
}

func nonceAleatoire() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("source aléatoire indisponible : " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// ---------------------------------------------------------------------------
// Routes.
// ---------------------------------------------------------------------------

func (g *gateway) googleActif() bool {
	return g.cfg.Google.Activer && g.cfg.Google.ClientID != "" && g.cfg.Google.ClientSecret != ""
}

// ---------------------------------------------------------------------------
// Garde-rythme (critique OpenAI v0.1 de la PR) : les routes
// d'authentification sont limitées à limiteRythme requêtes par fenêtre
// glissante par adresse IP, en mémoire (aucune dépendance). Au-delà :
// 429. Fail-closed côté Google comme côté serveur.
// ---------------------------------------------------------------------------

const (
	limiteRythme  = 10          // requêtes par fenêtre
	fenetreRythme = time.Minute // fenêtre glissante
)

var gardeRythme = struct {
	sync.Mutex
	fenetre map[string][]time.Time
}{fenetre: map[string][]time.Time{}}

func rythmeAutorise(r *http.Request) bool {
	ip := r.RemoteAddr
	gardeRythme.Lock()
	defer gardeRythme.Unlock()
	now := time.Now()
	gardées := gardeRythme.fenetre[ip][:0]
	for _, t := range gardeRythme.fenetre[ip] {
		if now.Sub(t) < fenetreRythme {
			gardées = append(gardées, t)
		}
	}
	if len(gardées) >= limiteRythme {
		gardeRythme.fenetre[ip] = gardées
		return false
	}
	gardeRythme.fenetre[ip] = append(gardées, now)
	return true
}

func (g *gateway) authGoogleEtat(w http.ResponseWriter, r *http.Request) {
	if !g.googleActif() {
		repondreJSON(w, http.StatusOK, map[string]any{"activer": false})
		return
	}
	drive := false
	if u, ok := g.utilisateurSession(w, r); ok {
		drive = u.DriveRefresh != ""
	}
	repondreJSON(w, http.StatusOK, map[string]any{"activer": true, "drive_connecte": drive})
}

// authGoogleLogin : redirection vers le consentement Google.
func (g *gateway) authGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if !rythmeAutorise(r) {
		repondreErreur(w, http.StatusTooManyRequests, "RYTME_DEPASSE",
			"trop de tentatives, réessayez dans une minute")
		return
	}
	if !g.googleActif() {
		repondreErreur(w, http.StatusServiceUnavailable, "AUTH_GOOGLE_DESACTIVE",
			"connexion Google non configurée (gateway.json)")
		return
	}
	drive := r.URL.Query().Get("drive") == "1"
	// Flux Drive : embarquer le sub de session dans l'état signé.
	subEtat := ""
	if drive {
		if u, ok := g.utilisateurRequete(r); ok {
			subEtat = u.Sub
		}
	}
	nonce := nonceAleatoire()
	etat := signerEtat(g.cfg.SecretSession, subEtat, nonce, time.Now().Add(dureeEtatOIDC))
	http.SetCookie(w, &http.Cookie{
		Name: cookieOIDC, Value: etat, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(dureeEtatOIDC.Seconds()),
	})
	scope := "openid email profile"
	v := url.Values{
		"client_id":     {g.cfg.Google.ClientID},
		"redirect_uri":  {g.cfg.Google.redirectURL()},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {etat},
		"nonce":         {nonce},
	}
	if drive {
		v.Set("scope", "https://www.googleapis.com/auth/drive.file")
		v.Set("access_type", "offline")
		v.Set("prompt", "consent")
	}
	http.Redirect(w, r, googleAuthURL+"?"+v.Encode(), http.StatusFound)
}

// authGoogleCallback : deux flux distincts selon le scope demandé :
//   - drive.file → refresh_token : session obligatoire, stockage serveur ;
//   - openid     → id_token : vérification, provisioning, session.
func (g *gateway) authGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !rythmeAutorise(r) {
		repondreErreur(w, http.StatusTooManyRequests, "RYTME_DEPASSE",
			"trop de tentatives, réessayez dans une minute")
		return
	}
	if !g.googleActif() {
		repondreErreur(w, http.StatusServiceUnavailable, "AUTH_GOOGLE_DESACTIVE",
			"connexion Google non configurée")
		return
	}
	q := r.URL.Query()
	if code := q.Get("error"); code != "" {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_GOOGLE_REFUSEE", "Google : "+code)
		return
	}
	cookie, err := r.Cookie(cookieOIDC)
	if err != nil || cookie.Value == "" {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_ABSENT", "état OIDC absent")
		return
	}
	subEtat, nonce, ok := verifierEtat(g.cfg.SecretSession, cookie.Value)
	if !ok {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_INVALIDE", "état OIDC invalide ou expiré")
		return
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(q.Get("state"))) != 1 {
		repondreErreur(w, http.StatusBadRequest, "ETAT_OIDC_DIVERGENT", "état OIDC divergent")
		return
	}
	code := q.Get("code")
	if code == "" {
		repondreErreur(w, http.StatusBadRequest, "CODE_ABSENT", "code d'autorisation absent")
		return
	}
	tok, err := g.echangerCode(code)
	if err != nil {
		repondreErreur(w, http.StatusUnauthorized, "ECHANGE_IMPOSSIBLE", err.Error())
		return
	}
	// Cookie d'état : usage unique, suppression immédiate.
	http.SetCookie(w, &http.Cookie{Name: cookieOIDC, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1})

	if refresh, ok := tok["refresh_token"].(string); ok && refresh != "" {
		// Flux Drive : l'état signé embarquait le sub de session.
		if subEtat == "" {
			repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
				"connectez-vous avant de connecter votre Drive")
			return
		}
		u, ok := g.users.parSub(subEtat)
		if !ok {
			repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE", "utilisateur inconnu")
			return
		}
		_ = u
		if err := g.users.maj(func(m map[string]*utilisateur) error {
			if m[subEtat] == nil {
				return fmt.Errorf("utilisateur disparu")
			}
			m[subEtat].DriveRefresh = refresh
			return nil
		}); err != nil {
			repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
			return
		}
		http.Redirect(w, r, "/#/creation", http.StatusFound)
		return
	}
	idToken, _ := tok["id_token"].(string)
	if idToken == "" {
		repondreErreur(w, http.StatusUnauthorized, "JETON_ABSENT", "id_token absent")
		return
	}
	claims, err := g.verifierJeton(idToken)
	if err != nil {
		repondreErreur(w, http.StatusUnauthorized, "JETON_INVALIDE", err.Error())
		return
	}
	nonceRecu, _ := claims["nonce"].(string)
	if nonceRecu == "" || subtle.ConstantTimeCompare([]byte(nonceRecu), []byte(nonce)) != 1 {
		repondreErreur(w, http.StatusUnauthorized, "NONCE_INVALIDE", "nonce divergent")
		return
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		repondreErreur(w, http.StatusUnauthorized, "SUB_ABSENT", "identité Google absente")
		return
	}
	email, _ := claims["email"].(string)
	nom, _ := claims["name"].(string)
	image, _ := claims["picture"].(string)
	u, err := g.provisionner(sub, email, nom, image)
	if err != nil {
		repondreErreur(w, http.StatusServiceUnavailable, "COMPTE_NON_PROVISIONNE", err.Error())
		return
	}
	g.poserCookieSession(w, u.Compte, u.Sub)
	http.Redirect(w, r, "/#/presentation", http.StatusFound)
}

// provisionner : rattachement par sub ; création à la première connexion.
func (g *gateway) provisionner(sub, email, nom, image string) (*utilisateur, error) {
	if u, ok := g.users.parSub(sub); ok {
		return u, nil
	}
	compte := ""
	if email != "" {
		if c, ok := g.cfg.Google.Correspondance[strings.ToLower(email)]; ok {
			compte = c
		}
	}
	if compte == "" {
		compte = g.cfg.Google.CompteNouveau
	}
	if compte == "" {
		return nil, fmt.Errorf("aucun compte attribué : configurez google.compte_nouveau dans gateway.json")
	}
	if (g.compteParNom(compte) == compteConf{}) {
		return nil, fmt.Errorf("compte inconnu dans gateway.json : %s", compte)
	}
	nouveau := &utilisateur{
		Sub: sub, Email: email, Nom: nom, Image: image,
		Compte: compte, Cree: time.Now().UTC(),
	}
	if err := g.users.maj(func(m map[string]*utilisateur) error {
		m[sub] = nouveau
		return nil
	}); err != nil {
		return nil, err
	}
	return nouveau, nil
}

func (g *gateway) echangerCode(code string) (map[string]any, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {g.cfg.Google.ClientID},
		"client_secret": {g.cfg.Google.ClientSecret},
		"redirect_uri":  {g.cfg.Google.redirectURL()},
		"grant_type":    {"authorization_code"},
	}
	rep, err := http.Post(googleTokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	defer rep.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(rep.Body, 1<<20))
	if rep.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token : statut %d", rep.StatusCode)
	}
	var tok map[string]any
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// ---------------------------------------------------------------------------
// Drive de la personne : listage et création de dossiers.
// ---------------------------------------------------------------------------

// tokenDriveAcces : access token court via refresh token de l'utilisateur.
// Nettoyage : les entrées expirées sont purgées à chaque acquisition
// (critique OpenAI v0.1 de la PR : le cache ne croît pas sans borne).
type cacheTokens struct {
	mu     sync.Mutex
	tokens map[string]struct {
		val string
		exp time.Time
	}
}

var tokensDrive = &cacheTokens{tokens: map[string]struct {
	val string
	exp time.Time
}{}}

func (c *cacheTokens) purger() {
	now := time.Now()
	for sub, t := range c.tokens {
		if now.After(t.exp) {
			delete(c.tokens, sub)
		}
	}
}

func (g *gateway) tokenDrive(u *utilisateur) (string, error) {
	tokensDrive.mu.Lock()
	tokensDrive.purger()
	if t, ok := tokensDrive.tokens[u.Sub]; ok {
		val := t.val
		tokensDrive.mu.Unlock()
		return val, nil
	}
	tokensDrive.mu.Unlock()
	form := url.Values{
		"client_id":     {g.cfg.Google.ClientID},
		"client_secret": {g.cfg.Google.ClientSecret},
		"refresh_token": {u.DriveRefresh},
		"grant_type":    {"refresh_token"},
	}
	rep, err := http.Post(googleTokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	defer rep.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token Drive absent")
	}
	tokensDrive.mu.Lock()
	tokensDrive.tokens[u.Sub] = struct {
		val string
		exp time.Time
	}{tok.AccessToken, time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)}
	tokensDrive.mu.Unlock()
	return tok.AccessToken, nil
}

func (g *gateway) driveDossiers(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	if u.DriveRefresh == "" {
		repondreErreur(w, http.StatusPreconditionRequired, "DRIVE_NON_CONNECTE",
			"connectez votre Drive depuis l'écran de création de mission")
		return
	}
	tok, err := g.tokenDrive(u)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_INJOIGNABLE", err.Error())
		return
	}
	api := "https://www.googleapis.com/drive/v3/files?pageSize=200&fields=files(id,name)&q=" +
		url.QueryEscape("mimeType='application/vnd.google-apps.folder' and trashed=false")
	req, _ := http.NewRequest(http.MethodGet, api, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_INJOIGNABLE", err.Error())
		return
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_REFUS",
			fmt.Sprintf("Drive : statut %d", rep.StatusCode))
		return
	}
	var resultat struct {
		Fichiers []struct {
			ID  string `json:"id"`
			Nom string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 4<<20)).Decode(&resultat); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	dossiers := []map[string]string{}
	for _, f := range resultat.Fichiers {
		dossiers = append(dossiers, map[string]string{"id": f.ID, "nom": f.Nom})
	}
	repondreJSON(w, http.StatusOK, map[string]any{"dossiers": dossiers})
}

func (g *gateway) driveCreerDossier(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	if u.DriveRefresh == "" {
		repondreErreur(w, http.StatusPreconditionRequired, "DRIVE_NON_CONNECTE",
			"connectez votre Drive d'abord")
		return
	}
	var req struct {
		Nom    string `json:"nom"`
		Parent string `json:"parent"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	nom := strings.TrimSpace(req.Nom)
	if nom == "" || len(nom) > 120 {
		repondreErreur(w, http.StatusBadRequest, "NOM_INVALIDE", "nom de dossier absent ou trop long")
		return
	}
	tok, err := g.tokenDrive(u)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_INJOIGNABLE", err.Error())
		return
	}
	corps := map[string]any{
		"name":     nom,
		"mimeType": "application/vnd.google-apps.folder",
	}
	if req.Parent != "" {
		corps["parents"] = []string{req.Parent}
	}
	raw, _ := json.Marshal(corps)
	creq, _ := http.NewRequest(http.MethodPost,
		"https://www.googleapis.com/drive/v3/files?fields=id,name", bytes.NewReader(raw))
	creq.Header.Set("Authorization", "Bearer "+tok)
	creq.Header.Set("Content-Type", "application/json")
	rep, err := http.DefaultClient.Do(creq)
	if err != nil {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_INJOIGNABLE", err.Error())
		return
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK && rep.StatusCode != http.StatusCreated {
		repondreErreur(w, http.StatusBadGateway, "DRIVE_REFUS",
			fmt.Sprintf("Drive : statut %d", rep.StatusCode))
		return
	}
	var dossier struct {
		ID  string `json:"id"`
		Nom string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(rep.Body, 1<<20)).Decode(&dossier); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{"id": dossier.ID, "nom": dossier.Nom})
}

// missionRepertoire : association mission → répertoire dédié Drive.
// Garde-fou : un répertoire ne sert qu'à une mission (spec §3.5).
func (g *gateway) missionRepertoire(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		id := r.URL.Query().Get("mission")
		if id == "" {
			repondreErreur(w, http.StatusBadRequest, "MISSION_ABSENTE", "paramètre mission requis")
			return
		}
		m := g.repertoires.charger(u.Compte, id)
		repondreJSON(w, http.StatusOK, map[string]any{"repertoire": m})
		return
	}
	var req struct {
		Mission    string `json:"mission"`
		Source     string `json:"source"`
		DossierID  string `json:"dossier_id"`
		DossierNom string `json:"dossier_nom"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	if req.Mission == "" || req.DossierID == "" {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", "mission et dossier_id requis")
		return
	}
	if req.Source == "" {
		req.Source = "drive" // compat : entrees creees avant SRC-01
	}
	if req.Source != "drive" && req.Source != "github" {
		repondreErreur(w, http.StatusBadRequest, "SOURCE_INVALIDE", "source inconnue : "+req.Source)
		return
	}
	// Homogeneite verifiee SERVEUR (spec 3.4) : refus AVANT toute
	// ecriture si la mission change de source.
	if existant := g.repertoires.charger(u.Compte, req.Mission); existant != nil {
		sourceExistante, _ := existant["source"].(string)
		if sourceExistante == "" {
			sourceExistante = "drive"
		}
		if sourceExistante != req.Source {
			repondreErreur(w, http.StatusConflict, "SOURCE_HETEROGENE",
				"cette mission vit deja dans une source "+sourceExistante)
			return
		}
	}
	// Garde-fou anti-collision : le dossier ne doit servir à aucune
	// AUTRE mission (la mission elle-même peut se ré-associer).
	for cle, m := range g.repertoires.chargerTout(u.Compte) {
		if cle != cleMission(u.Compte, req.Mission) && m["dossier_id"] == req.DossierID {
			repondreErreur(w, http.StatusConflict, "DOSSIER_DEJA_PRIS",
				"ce répertoire est déjà dédié à une autre mission")
			return
		}
	}
	entree := map[string]string{"dossier_id": req.DossierID, "dossier_nom": req.DossierNom, "source": req.Source}
	if err := g.repertoires.majEntry(u.Compte, req.Mission, entree); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{"repertoire": entree})
}

// ---------------------------------------------------------------------------
// Store mission → répertoire (JSON atomique, clé "compte|mission").
// ---------------------------------------------------------------------------

type storeKV struct {
	mu     sync.Mutex
	chemin string
	data   map[string]map[string]string
}

func nouveauStoreKV(chemin string) (*storeKV, error) {
	s := &storeKV{chemin: chemin, data: map[string]map[string]string{}}
	raw, err := os.ReadFile(chemin)
	if err == nil {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, fmt.Errorf("fichier répertoires invalide : %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func cleMission(compte, mission string) string { return compte + "\x1f" + mission }

func (s *storeKV) charger(compte, mission string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[cleMission(compte, mission)]
	if !ok {
		return nil
	}
	c := map[string]any{"dossier_id": e["dossier_id"], "dossier_nom": e["dossier_nom"], "source": e["source"]}
	return c
}

func (s *storeKV) chargerTout(compte string) map[string]map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := map[string]map[string]string{}
	for k, v := range s.data {
		if strings.HasPrefix(k, compte+"\x1f") {
			c := map[string]string{"dossier_id": v["dossier_id"], "dossier_nom": v["dossier_nom"]}
			res[k] = c
		}
	}
	return res
}

func (s *storeKV) majEntry(compte, mission string, val map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[cleMission(compte, mission)] = val
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.chemin + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.chemin)
}

// apiMoi : profil de la personne + solde (v1 : 0, mission SOLDE-01).
func (g *gateway) apiMoi(w http.ResponseWriter, r *http.Request) {
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{
		"nom":            u.Nom,
		"email":          u.Email,
		"image":          u.Image,
		"compte":         u.Compte,
		"solde":          0,
		"drive_connecte": u.DriveRefresh != "",
	})
}

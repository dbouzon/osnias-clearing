// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Session navigateur v2 (M-COORD-AUTH-01) : cookie HttpOnly + Secure +
// SameSite=Strict, signé HMAC (secret par configuration, jamais dans le
// dépôt), expiration GLISSANTE de 7 jours (spec §5). Le cookie porte
// "compte|subGoogle" : les tokens pilote et les refresh tokens Drive
// restent côté serveur. La connexion manuelle (maintenance) signe un
// cookie sans sub.
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const (
	cookieSession = "coord_session"
	nomCookieHMAC = "coord-front-session-v1"
)

// signerSession : "compte|sub.mac".
func signerSession(secret, nom, sub string) string {
	charge := nom + "|" + sub
	mac := hmac.New(sha256.New, []byte(secret+nomCookieHMAC))
	mac.Write([]byte(charge))
	return charge + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifierSession : comparaison à temps constant sur la valeur complète.
// Renvoie le compte et le sub Google ("" en connexion manuelle).
func verifierSession(secret, valeur string) (nom, sub string, ok bool) {
	i := strings.LastIndexByte(valeur, '.')
	if i <= 0 {
		return "", "", false
	}
	charge := valeur[:i]
	sep := strings.LastIndexByte(charge, '|')
	if sep < 0 {
		return "", "", false
	}
	nom, sub = charge[:sep], charge[sep+1:]
	attendu := signerSession(secret, nom, sub)
	if subtle.ConstantTimeCompare([]byte(valeur), []byte(attendu)) != 1 {
		return "", "", false
	}
	return nom, sub, true
}

// secretAleatoire : repli si la configuration n'en fournit pas (chaque
// démarrage invalide alors les sessions précédentes, ce qui reste sûr).
func secretAleatoire() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("source aléatoire indisponible : " + err.Error())
	}
	return hex.EncodeToString(b)
}

// poserCookieSession : session glissante, 7 jours (spec §5).
func (g *gateway) poserCookieSession(w http.ResponseWriter, nom, sub string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSession,
		Value:    signerSession(g.cfg.SecretSession, nom, sub),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})
}

// compteSession : middleware. Renvoie le compte authentifié ou écrit le
// 401. Session à expiration glissante : chaque requête valide étend le
// cookie de 7 jours.
func (g *gateway) compteSession(w http.ResponseWriter, r *http.Request) (compteConf, bool) {
	cookie, err := r.Cookie(cookieSession)
	if err != nil || cookie.Value == "" {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
			"session absente : connectez-vous")
		return compteConf{}, false
	}
	nom, _, ok := verifierSession(g.cfg.SecretSession, cookie.Value)
	if !ok {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
			"session invalide")
		return compteConf{}, false
	}
	for _, c := range g.cfg.Comptes {
		if c.Nom == nom {
			// Expiration glissante : re-signature identique.
			if nom, sub, ok := verifierSession(g.cfg.SecretSession, cookie.Value); ok {
				_ = sub
				g.poserCookieSession(w, nom, sub)
			}
			return c, true
		}
	}
	repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
		"compte inconnu : "+nom)
	return compteConf{}, false
}

// utilisateurSession : utilisateur Google de la session. Le 401 est
// écrit ici (via compteSession) : aucun handler ne répond en double.
func (g *gateway) utilisateurSession(w http.ResponseWriter, r *http.Request) (*utilisateur, bool) {
	if _, ok := g.compteSession(w, r); !ok {
		return nil, false
	}
	u, ok := g.utilisateurRequete(r)
	if !ok {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
			"connexion Google requise pour cette action")
		return nil, false
	}
	return u, true
}

func (g *gateway) utilisateurRequete(r *http.Request) (*utilisateur, bool) {
	cookie, err := r.Cookie(cookieSession)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	nom, sub, ok := verifierSession(g.cfg.SecretSession, cookie.Value)
	if !ok || sub == "" {
		return nil, false
	}
	c := g.compteParNom(nom)
	if (c == compteConf{}) {
		return nil, false
	}
	u, ok := g.users.parSub(sub)
	if !ok || u.Compte != nom {
		return nil, false
	}
	return u, true
}

func (g *gateway) connexion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Compte string `json:"compte"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	for _, c := range g.cfg.Comptes {
		if c.Nom == req.Compte {
			// Connexion manuelle (maintenance) : sans sub Google.
			g.poserCookieSession(w, c.Nom, "")
			repondreJSON(w, http.StatusOK, g.infoPublique(c))
			return
		}
	}
	repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
		"compte inconnu : "+req.Compte)
}

func (g *gateway) deconnexion(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieSession, Value: "", Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	repondreJSON(w, http.StatusOK, map[string]any{"deconnecte": true})
}

// infoSession : le front déduit la bannière DÉMO et l'état Drive de
// cette réponse.
func (g *gateway) infoSession(w http.ResponseWriter, r *http.Request) {
	c, ok := g.compteSession(w, r)
	if !ok {
		return
	}
	repondreJSON(w, http.StatusOK, g.infoPublique(c))
}

func (g *gateway) infoPublique(c compteConf) map[string]any {
	return map[string]any{
		"compte":        c.Nom,
		"demo":          c.Demo,
		"dossier_drive": c.DossierDrive,
	}
}

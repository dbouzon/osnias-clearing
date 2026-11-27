// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Session navigateur : cookie HttpOnly + SameSite=Strict, signé HMAC
// (secret par configuration, jamais dans le dépôt). Le cookie ne porte
// QUE le nom du compte : les tokens pilotes restent côté serveur.
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const (
	cookieSession = "coord_session"
	nomCookieHMAC = "coord-front-session-v1"
)

// signerCompte : nom du compte + MAC hexadécimale.
func signerCompte(secret, nom string) string {
	mac := hmac.New(sha256.New, []byte(secret+nomCookieHMAC))
	mac.Write([]byte(nom))
	return nom + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifierCompte : comparaison à temps constant sur la valeur complète.
func verifierCompte(secret, valeur string) (string, bool) {
	i := strings.LastIndexByte(valeur, '.')
	if i <= 0 {
		return "", false
	}
	nom := valeur[:i]
	attendu := signerCompte(secret, nom)
	if subtle.ConstantTimeCompare([]byte(valeur), []byte(attendu)) != 1 {
		return "", false
	}
	return nom, true
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

// compteSession : middleware. Renvoie le compte authentifié ou écrit
// le 401 et renvoie false.
func (g *gateway) compteSession(w http.ResponseWriter, r *http.Request) (compteConf, bool) {
	cookie, err := r.Cookie(cookieSession)
	if err != nil || cookie.Value == "" {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
			"session absente : connectez-vous")
		return compteConf{}, false
	}
	nom, ok := verifierCompte(g.cfg.SecretSession, cookie.Value)
	if !ok {
		repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
			"session invalide")
		return compteConf{}, false
	}
	for _, c := range g.cfg.Comptes {
		if c.Nom == nom {
			return c, true
		}
	}
	repondreErreur(w, http.StatusUnauthorized, "AUTH_REFUSEE",
		"compte inconnu : "+nom)
	return compteConf{}, false
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
			http.SetCookie(w, &http.Cookie{
				Name:     cookieSession,
				Value:    signerCompte(g.cfg.SecretSession, c.Nom),
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
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
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
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

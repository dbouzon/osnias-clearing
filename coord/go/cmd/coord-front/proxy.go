// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Mode RÉEL : proxy vers l'instance coord-maieutique loopback du compte.
// Le token pilote est injecté ICI, côté serveur ; le navigateur ne le
// voit jamais. Aucune logique métier : les méthodes, chemins et corps
// passent tels quels, les réponses repartent telles quelles.
package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

func (g *gateway) proxifier(w http.ResponseWriter, r *http.Request, c compteConf) {
	// Chemin interne : /api/projets… → /projets… (l'instance expose son
	// API à la racine). La requête sortante ne porte JAMAIS le cookie
	// de session navigateur.
	chemin := strings.TrimPrefix(r.URL.Path, "/api")
	cible := strings.TrimRight(c.URL, "/") + chemin
	if r.URL.RawQuery != "" {
		cible += "?" + r.URL.RawQuery
	}
	corps := []byte{}
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
			return
		}
		corps = b
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, cible, bytes.NewReader(corps))
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "INTERNE", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Coord-Pilot-Token", c.Token) // jamais journalisé

	client := &http.Client{Timeout: 120 * time.Second}
	rep, err := client.Do(req)
	if err != nil {
		// Le token ne doit JAMAIS apparaître dans le motif d'erreur.
		repondreErreur(w, http.StatusBadGateway, "INDISPO",
			"instance du compte injoignable")
		return
	}
	defer rep.Body.Close()
	for k := range rep.Header {
		w.Header().Set(k, rep.Header.Get(k))
	}
	w.WriteHeader(rep.StatusCode)
	_, _ = io.Copy(w, rep.Body)
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Compteurs COORD (missions M-COORD-ORPHEUS-01 puis M-COORD-ORPHEUS-02) :
// instrumentation Prometheus du chemin des moteurs IA, alignée sur la
// convention Orpheus des connecteurs Osnias (compteurs par familles,
// fail-closed sur nil), en stdlib UNIQUEMENT. Familles de marque COORD
// depuis M-COORD-ORPHEUS-02 (décision du pilote, 28/11/2026) :
//
//   coord_moteurs_appels_total{moteur,modele,niveau,etape,resultat}
//   coord_moteurs_jetons_entree_total{moteur,modele}
//   coord_moteurs_jetons_sortie_total{moteur,modele}
//   coord_moteurs_duree_seconds{moteur,etape} (histogramme)
//
// Exposition /metrics au format texte Prometheus (version 0.0.4),
// protégée fail-closed : section "metrics" de gateway.json (activer,
// token) désactivée par défaut → 404 ; activée → Bearer token exigé,
// sinon 404 (pas de divulgation d'existence). Token jamais journalisé.
// Les compteurs sont VOLATILS (zéro au redémarrage) : Prometheus =
// temps réel/alerting, le journal durable journal-moteurs.json reste
// le registre facturable (mission sœur M-COORD-SOLDE-01).
package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crypto/subtle"
)

type metricsConf struct {
	Activer bool   `json:"activer"`
	Token   string `json:"token"`
}

// Buckets de l'histogramme de durée (secondes), ordre croissant.
var bucketsDureeMoteurs = []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}

type histMoteurs struct {
	buckets []uint64 // cumulés (bucket le + haut = +Inf)
	somme   float64
	nombre  uint64
}

type compteursMoteurs struct {
	mu           sync.Mutex
	appels       map[string]*uint64
	jetonsEntree map[string]*uint64
	jetonsSortie map[string]*uint64
	durees       map[string]*histMoteurs
}

func nouveauxCompteursMoteurs() *compteursMoteurs {
	return &compteursMoteurs{
		appels:       map[string]*uint64{},
		jetonsEntree: map[string]*uint64{},
		jetonsSortie: map[string]*uint64{},
		durees:       map[string]*histMoteurs{},
	}
}

// echapperLabel : échappement minimal du format texte Prometheus
// (antislash, guillemet, saut de ligne).
func echapperLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

// rendreLabels : paires triées par clé (ordre déterministe).
func rendreLabels(labels map[string]string) string {
	cles := make([]string, 0, len(labels))
	for k := range labels {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	parts := make([]string, 0, len(cles))
	for _, k := range cles {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, echapperLabel(labels[k])))
	}
	return strings.Join(parts, ",")
}

// incrementer : compte de la famille, clé = forme sériée des labels.
func (c *compteursMoteurs) incrementer(famille map[string]*uint64, labels map[string]string) {
	cle := rendreLabels(labels)
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := famille[cle]; ok {
		atomic.AddUint64(p, 1)
		return
	}
	n := uint64(1)
	famille[cle] = &n
}

// ajouter : comme incrementer, avec un delta quelconque (jetons).
func (c *compteursMoteurs) ajouter(famille map[string]*uint64, labels map[string]string, delta uint64) {
	cle := rendreLabels(labels)
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := famille[cle]; ok {
		atomic.AddUint64(p, delta)
		return
	}
	n := delta
	famille[cle] = &n
}

// observer : histogramme de durée par (moteur, etape).
func (c *compteursMoteurs) observer(moteur, etape string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cle := rendreLabels(map[string]string{"moteur": moteur, "etape": etape})
	h, ok := c.durees[cle]
	if !ok {
		h = &histMoteurs{buckets: make([]uint64, len(bucketsDureeMoteurs))}
		c.durees[cle] = h
	}
	s := d.Seconds()
	for i, b := range bucketsDureeMoteurs {
		if s <= b {
			h.buckets[i]++
		}
	}
	h.somme += s
	h.nombre++
}

// ---- Points d'entrée instrumentés (appelés par appelEtape) ----------

// compterAppelMoteur : un appel moteur, résultat ok|erreur|refuse. Les
// refus AVANT réseau (plafond, fournisseur désactivé) comptent avec
// resultat="refuse" (spec §3.1).
func (g *gateway) compterAppelMoteur(moteur, modele, niveau, etape, resultat string) {
	g.compteurs.incrementer(g.compteurs.appels, map[string]string{
		"moteur": moteur, "modele": modele, "niveau": niveau, "etape": etape, "resultat": resultat,
	})
}

func (g *gateway) compterJetonsMoteurs(moteur, modele string, entree, sortie int) {
	if entree > 0 {
		g.compteurs.ajouter(g.compteurs.jetonsEntree,
			map[string]string{"moteur": moteur, "modele": modele}, uint64(entree))
	}
	if sortie > 0 {
		g.compteurs.ajouter(g.compteurs.jetonsSortie,
			map[string]string{"moteur": moteur, "modele": modele}, uint64(sortie))
	}
}

func (g *gateway) observerDureeMoteur(moteur, etape string, d time.Duration) {
	g.compteurs.observer(moteur, etape, d)
}

// ---- Exposition /metrics (fail-closed, Bearer token) ----------------

func (g *gateway) servirMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		repondreErreur(w, http.StatusNotFound, "INTROUVABLE", "ressource inconnue")
		return
	}
	_ = g.regimeMoteurs() // recharge aussi la section metrics (mtime)
	if !g.metrics.Activer || g.metrics.Token == "" {
		http.NotFound(w, r)
		return
	}
	auth := r.Header.Get("Authorization")
	const porte = "Bearer "
	if !strings.HasPrefix(auth, porte) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, porte)), []byte(g.metrics.Token)) != 1 {
		http.NotFound(w, r) // 404 : pas de divulgation d'existence
		return
	}
	corps := g.exposerMetrics()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(corps))
}

// exposerMetrics : rendu du format texte Prometheus (HELP/TYPE, labels
// triés, histogramme bucket/sum/count). Aucune donnée de mission, aucun
// secret : les labels seuls.
func (g *gateway) exposerMetrics() string {
	var sb strings.Builder
	c := g.compteurs
	c.mu.Lock()
	defer c.mu.Unlock()

	type ligne struct {
		labels string
		valeur uint64
	}
	rendre := func(nom, aide, typ string, famille map[string]*uint64) {
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s %s\n", nom, aide, nom, typ)
		cles := make([]string, 0, len(famille))
		for k := range famille {
			cles = append(cles, k)
		}
		sort.Strings(cles)
		for _, k := range cles {
			fmt.Fprintf(&sb, "%s%s %d\n", nom,
				func() string { if k == "" { return "" } ; return "{" + k + "}" }(),
				atomic.LoadUint64(famille[k]))
		}
	}
	rendre("coord_moteurs_appels_total",
		"Appels moteurs IA (resultat: ok, erreur, refuse).", "counter", c.appels)
	rendre("coord_moteurs_jetons_entree_total",
		"Jetons d'entrée facturés par moteur.", "counter", c.jetonsEntree)
	rendre("coord_moteurs_jetons_sortie_total",
		"Jetons de sortie facturés par moteur.", "counter", c.jetonsSortie)

	// Histogramme de durée.
	nomH := "coord_moteurs_duree_seconds"
	fmt.Fprintf(&sb, "# HELP %s Durée des appels moteurs IA.\n# TYPE %s histogram\n", nomH, nomH)
	cles := make([]string, 0, len(c.durees))
	for k := range c.durees {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	for _, k := range cles {
		h := c.durees[k]
		if k == "" {
			continue
		}
		for i, b := range bucketsDureeMoteurs {
			fmt.Fprintf(&sb, "%s_bucket{%s,le=\"%s\"} %d\n", nomH, k, formatBornes(b), h.buckets[i])
		}
		fmt.Fprintf(&sb, "%s_bucket{%s,le=\"+Inf\"} %d\n", nomH, k, atomic.LoadUint64(&h.nombre))
		fmt.Fprintf(&sb, "%s_sum{%s} %s\n", nomH, k, formatSecondes(h.somme))
		fmt.Fprintf(&sb, "%s_count{%s} %d\n", nomH, k, atomic.LoadUint64(&h.nombre))
	}
	return sb.String()
}

func formatBornes(b float64) string {
	if b == float64(int(b)) {
		return fmt.Sprintf("%d", int(b))
	}
	return fmt.Sprintf("%g", b)
}

func formatSecondes(s float64) string {
	return fmt.Sprintf("%g", s)
}

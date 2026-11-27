// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-front est le gateway du parcours client COORD (mission
// M-COORD-FRONT-01, plan v2). Il sert le front statique (coord/front)
// depuis la MÊME ORIGINE que l'API (/api/*), et rien d'autre :
//
//   - mode RÉEL : proxy authentifié vers l'instance coord-maieutique
//     loopback du compte ; le token pilote est injecté CÔTÉ SERVEUR,
//     jamais envoyé au navigateur, jamais journalisé ;
//   - mode DÉMO : moteur factice embarqué (mêmes états, mêmes formes
//     JSON que le moteur réel), sans aucun appel externe, 0 €.
//
// LOOPBACK UNIQUEMENT (fail closed) : toute adresse non loopback est
// refusée au démarrage. Stdlib uniquement, aucune dépendance tierce.
// Aucun secret dans le code : la table compte→token vit dans le fichier
// de configuration (chmod 600, hors dépôt ; exemple factice dans
// docs/front.md).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// compteConf : une entrée = un compte client = une instance
// coord-maieutique loopback (data dir propre) + son token pilote.
// Demo: true bascule ce compte sur le moteur factice embarqué.
type compteConf struct {
	Nom          string `json:"compte"`
	URL          string `json:"url"`
	Token        string `json:"token"`
	Demo         bool   `json:"demo"`
	DossierDrive string `json:"dossier_drive"`
}

type config struct {
	SecretSession string       `json:"secret_session"`
	Comptes       []compteConf `json:"comptes"`
}

// gateway : état du serveur. Les moteurs de démo et les adaptateurs
// Drive sont instanciés PAR COMPTE (isolation inter-comptes).
type gateway struct {
	mu       sync.RWMutex
	cfg      config
	frontDir string
	demos    map[string]*moteurDemo
	drives   map[string]Dossiers
}

func newGateway(cfg config, frontDir string) *gateway {
	g := &gateway{
		cfg:      cfg,
		frontDir: frontDir,
		demos:    map[string]*moteurDemo{},
		drives:   map[string]Dossiers{},
	}
	for _, c := range cfg.Comptes {
		if c.Demo {
			g.demos[c.Nom] = nouveauMoteurDemo()
		}
		if c.DossierDrive != "" {
			g.drives[c.Nom] = nouveauDossierFactice(c.DossierDrive)
		}
	}
	return g
}

func (g *gateway) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sante", g.servirSante)
	mux.HandleFunc("GET /api/comptes", g.listerComptes)
	mux.HandleFunc("POST /api/connexion", g.connexion)
	mux.HandleFunc("POST /api/deconnexion", g.deconnexion)
	mux.HandleFunc("GET /api/session", g.infoSession)
	mux.HandleFunc("/api/drive/", g.apiDrive)
	mux.HandleFunc("/api/projets", g.apiProjets)
	mux.HandleFunc("/api/projets/", g.apiProjets)
	// Le front statique en dernier : tout chemin non-API reconnu
	// (index.html inclus, servi par FileServer à la racine).
	mux.Handle("/", http.FileServer(http.Dir(g.frontDir)))
	return mux
}

func (g *gateway) servirSante(w http.ResponseWriter, r *http.Request) {
	repondreJSON(w, http.StatusOK, map[string]any{"etat": "ok"})
}

// listerComptes : expose UNIQUEMENT les noms publics des comptes
// (écran de connexion). Jamais d'URL, jamais de token.
func (g *gateway) listerComptes(w http.ResponseWriter, r *http.Request) {
	noms := []string{}
	for _, c := range g.cfg.Comptes {
		noms = append(noms, c.Nom)
	}
	repondreJSON(w, http.StatusOK, map[string]any{"comptes": noms})
}

// apiProjets : point d'entrée unique du parcours. Session obligatoire,
// puis aiguillage démo (moteur factice embarqué) ou réel (proxy vers
// l'instance du compte avec injection du token pilote).
func (g *gateway) apiProjets(w http.ResponseWriter, r *http.Request) {
	c, ok := g.compteSession(w, r)
	if !ok {
		return
	}
	if c.Demo {
		demo := g.demos[c.Nom]
		if demo == nil {
			repondreErreur(w, http.StatusInternalServerError, "INTERNE", "moteur démo absent pour "+c.Nom)
			return
		}
		demo.servir(w, r)
		return
	}
	g.proxifier(w, r, c)
}

func main() {
	var configPath, frontDir, addr string
	flag.StringVar(&configPath, "config", "", "fichier de configuration JSON (chmod 600, hors dépôt)")
	flag.StringVar(&frontDir, "front", "front", "dossier du front statique (coord/front)")
	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "adresse d'écoute (loopback uniquement)")
	flag.Parse()

	if configPath == "" {
		log.Fatal("coord-front : -config requis (voir docs/front.md)")
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatal("coord-front : lecture config impossible : ", err)
	}
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Fatal("coord-front : configuration invalide : ", err)
	}
	if len(strings.TrimSpace(cfg.SecretSession)) < 16 {
		log.Fatal("coord-front : secret_session trop court (16 caractères minimum)")
	}
	if len(cfg.Comptes) == 0 {
		log.Fatal("coord-front : aucun compte configuré")
	}
	vus := map[string]bool{}
	for _, c := range cfg.Comptes {
		if strings.TrimSpace(c.Nom) == "" {
			log.Fatal("coord-front : compte sans nom")
		}
		if vus[c.Nom] {
			log.Fatal("coord-front : compte dupliqué : ", c.Nom)
		}
		vus[c.Nom] = true
		if c.Demo {
			continue // le moteur factice n'a pas d'URL à vérifier
		}
		if strings.TrimSpace(c.URL) == "" || strings.TrimSpace(c.Token) == "" {
			log.Fatal("coord-front : compte réel sans url ou token : ", c.Nom)
		}
	}
	if fi, err := os.Stat(frontDir); err != nil || !fi.IsDir() {
		log.Fatal("coord-front : dossier front introuvable : ", frontDir)
	}

	// Fail closed : loopback uniquement, aucun écouteur public.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatal("coord-front : adresse invalide : ", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("coord-front : refus fail-closed, loopback uniquement : ", addr)
	}

	g := newGateway(cfg, frontDir)
	srv := &http.Server{Addr: addr, Handler: g.routes()}
	fmt.Printf("coord-front : écoute loopback %s (front : %s, comptes : %d)\n",
		addr, frontDir, len(cfg.Comptes))
	log.Fatal(srv.ListenAndServe())
}

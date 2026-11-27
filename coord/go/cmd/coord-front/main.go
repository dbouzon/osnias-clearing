// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// coord-front est le gateway du parcours client COORD (missions
// M-COORD-FRONT-01 puis M-COORD-AUTH-01). Il sert le front statique
// (coord/front) depuis la MÊME ORIGINE que l'API (/api/*), et rien
// d'autre :
//
//   - mode RÉEL : proxy authentifié vers l'instance coord-maieutique
//     loopback du compte ; le token pilote est injecté CÔTÉ SERVEUR,
//     jamais envoyé au navigateur, jamais journalisé ;
//   - mode DÉMO : moteur factice embarqué (mêmes états, mêmes formes
//     JSON que le moteur réel), sans aucun appel externe, 0 € ;
//   - depuis M-COORD-AUTH-01 : connexion Google (OIDC), provisioning
//     automatique des personnes, session glissante 7 jours, Drive de
//     la personne et répertoire dédié par mission.
//
// LOOPBACK UNIQUEMENT (fail closed) : toute adresse non loopback est
// refusée au démarrage. Stdlib uniquement, aucune dépendance tierce.
// Aucun secret dans le code : configuration, comptes et utilisateurs
// vivent dans les fichiers de configuration (chmod 600, hors dépôt ;
// exemple factice dans docs/front.md).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// compteConf : une entrée = un compte client = une instance
// coord-maieutique loopback (data dir propre) + son token pilote.
// Demo: true bascule ce compte sur le moteur factice embarqué.
// RemoteRclone non vide → adaptateur Drive rclone (mission
// M-COORD-DRIVE-01) ; DossierDrive devient alors le chemin racine
// DANS le remote (sinon : racine du dossier factice local).
type compteConf struct {
	Nom          string `json:"compte"`
	URL          string `json:"url"`
	Token        string `json:"token"`
	Demo         bool   `json:"demo"`
	DossierDrive string `json:"dossier_drive"`

	RemoteRclone           string `json:"remote_rclone"`
	TimeoutRclone          int    `json:"timeout_rclone"`
	ExportSortie           bool   `json:"export_sortie"`
	AutoriserEcritureDrive bool   `json:"autoriser_ecriture_drive"`
}

// timeoutDrive : timeout rclone par appel, clampé.
func (c compteConf) timeoutDrive() time.Duration {
	t := c.TimeoutRclone
	if t <= 0 {
		t = 30
	}
	if t < 5 {
		t = 5
	}
	if t > 120 {
		t = 120
	}
	return time.Duration(t) * time.Second
}

type config struct {
	SecretSession       string       `json:"secret_session"`
	Comptes             []compteConf `json:"comptes"`
	Google              googleConf   `json:"google"`
	GitHub              githubConf   `json:"github"`
	Moteurs             moteursConf  `json:"moteurs"`
	Metrics             metricsConf  `json:"metrics"`
	FichierUtilisateurs string       `json:"fichier_utilisateurs"`
	FichierRepertoires  string       `json:"fichier_repertoires"`
}

// gateway : état du serveur. Les moteurs de démo et les adaptateurs
// Drive sont instanciés PAR COMPTE (isolation inter-comptes). La base
// des utilisateurs (M-COORD-AUTH-01) et l'association mission →
// répertoire Drive sont persistées à côté de la configuration.
type gateway struct {
	mu          sync.RWMutex
	cfg         config
	frontDir    string
	demos       map[string]*moteurDemo
	drives      map[string]Dossiers
	exports     map[string]etatExport
	users       *banque
	repertoires *storeKV

	// M-COORD-API-MISTRAL-01 : régime des moteurs IA (relecture à chaud
	// par mtime de gateway.json) et journal de coût atomique 0600.
	// M-COORD-ORPHEUS-01 : section metrics (exposition /metrics) et
	// compteurs Prometheus du chemin moteurs.
	muMoteurs     sync.Mutex
	moteurs       moteursConf
	metrics       metricsConf
	mtimeMoteurs  time.Time
	cheminConfig  string
	cheminJournal string
	muJournal     sync.Mutex
	compteurs     *compteursMoteurs
}

func newGateway(cfg config, frontDir, dirConfig string) *gateway {
	fichierUsers := cfg.FichierUtilisateurs
	if fichierUsers == "" {
		fichierUsers = filepath.Join(dirConfig, "utilisateurs.json")
	}
	fichierRep := cfg.FichierRepertoires
	if fichierRep == "" {
		fichierRep = filepath.Join(dirConfig, "repertoires_missions.json")
	}
	users, err := nouvelleBanque(fichierUsers)
	if err != nil {
		log.Fatal("coord-front : base utilisateurs : ", err)
	}
	stockage, err := nouveauStoreKV(fichierRep)
	if err != nil {
		log.Fatal("coord-front : base répertoires : ", err)
	}
	g := &gateway{
		cfg:           cfg,
		frontDir:      frontDir,
		demos:         map[string]*moteurDemo{},
		drives:        map[string]Dossiers{},
		exports:       map[string]etatExport{},
		users:         users,
		repertoires:   stockage,
		moteurs:       cfg.Moteurs,
		metrics:       cfg.Metrics,
		compteurs:     nouveauxCompteursMoteurs(),
		cheminJournal: filepath.Join(dirConfig, journalMoteurFichier),
	}
	for _, c := range cfg.Comptes {
		if c.Demo {
			g.demos[c.Nom] = nouveauMoteurDemo(g)
		}
		switch {
		case c.RemoteRclone != "":
			g.drives[c.Nom] = nouveauDossierRclone(
				c.RemoteRclone, c.DossierDrive, c.timeoutDrive())
		case c.DossierDrive != "":
			g.drives[c.Nom] = nouveauDossierFactice(c.DossierDrive)
		}
	}
	return g
}

// compteParNom : recherche dans la configuration (fail-closed : le
// résultat vide n'existe pas dans cfg.Comptes).
func (g *gateway) compteParNom(nom string) compteConf {
	for _, c := range g.cfg.Comptes {
		if c.Nom == nom {
			return c
		}
	}
	return compteConf{}
}

func (g *gateway) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sante", g.servirSante)
	mux.HandleFunc("GET /api/comptes", g.listerComptes)
	// M-COORD-AUTH-01 : connexion Google et personne.
	mux.HandleFunc("GET /api/auth/google/etat", g.authGoogleEtat)
	mux.HandleFunc("GET /api/auth/google/login", g.authGoogleLogin)
	mux.HandleFunc("GET /api/auth/google/callback", g.authGoogleCallback)
	mux.HandleFunc("GET /api/moi", g.apiMoi)
	// M-COORD-SRC-01 : source GitHub (repos publics).
	mux.HandleFunc("GET /api/auth/github/etat", g.authGithubEtat)
	mux.HandleFunc("GET /api/auth/github/login", g.authGithubLogin)
	mux.HandleFunc("GET /api/auth/github/callback", g.authGithubCallback)
	mux.HandleFunc("GET /api/github/repos", g.githubRepos)
	mux.HandleFunc("POST /api/github/structure", g.githubStructure)
	mux.HandleFunc("POST /api/connexion", g.connexion)
	mux.HandleFunc("POST /api/deconnexion", g.deconnexion)
	mux.HandleFunc("GET /api/session", g.infoSession)
	mux.HandleFunc("/api/drive/", g.apiDrive)
	mux.HandleFunc("GET /api/drive/dossiers", g.driveDossiers)
	mux.HandleFunc("POST /api/drive/dossier", g.driveCreerDossier)
	mux.HandleFunc("/api/missions/repertoire", g.missionRepertoire)
	// M-COORD-ORPHEUS-01 : exposition Prometheus (fail-closed, token).
	mux.HandleFunc("GET /metrics", g.servirMetrics)
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
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/transmettre") &&
		g.exportActif(c) {
		g.servirTransmission(w, r, c)
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
		if c.RemoteRclone != "" {
			if err := validerNomRemote(c.RemoteRclone); err != nil {
				log.Fatal("coord-front : compte ", c.Nom, " : remote rclone invalide")
			}
		}
		if c.Demo {
			continue
		}
		if strings.TrimSpace(c.URL) == "" || strings.TrimSpace(c.Token) == "" {
			log.Fatal("coord-front : compte réel sans url ou token : ", c.Nom)
		}
	}
	// M-COORD-AUTH-01 : fail-closed sur la configuration Google.
	if cfg.Google.Activer {
		if strings.TrimSpace(cfg.Google.ClientID) == "" ||
			strings.TrimSpace(cfg.Google.ClientSecret) == "" {
			log.Fatal("coord-front : google.activer sans client_id/client_secret")
		}
		if strings.TrimSpace(cfg.Google.CompteNouveau) == "" && len(cfg.Google.Correspondance) == 0 {
			log.Fatal("coord-front : google.activer sans compte_nouveau ni correspondance")
		}
		if cfg.Google.CompteNouveau != "" && (compteParNomStatique(cfg, cfg.Google.CompteNouveau) == compteConf{}) {
			log.Fatal("coord-front : google.compte_nouveau inconnu : ", cfg.Google.CompteNouveau)
		}
		for email, compte := range cfg.Google.Correspondance {
			if (compteParNomStatique(cfg, compte) == compteConf{}) {
				log.Fatal("coord-front : correspondance google pour ", email, " : compte inconnu ", compte)
			}
		}
	}
	// M-COORD-SRC-01 : fail-closed sur la configuration GitHub.
	if cfg.GitHub.Activer {
		if strings.TrimSpace(cfg.GitHub.ClientID) == "" ||
			strings.TrimSpace(cfg.GitHub.ClientSecret) == "" {
			log.Fatal("coord-front : github.activer sans client_id/client_secret")
		}
	}
	// M-COORD-API-MISTRAL-01 : fail-closed sur la section moteurs.
	if err := validerMoteurs(cfg.Moteurs); err != nil {
		log.Fatal("coord-front : section moteurs invalide : ", err)
	}
	// M-COORD-ORPHEUS-01 : fail-closed sur la section metrics.
	if cfg.Metrics.Activer && len(strings.TrimSpace(cfg.Metrics.Token)) < 16 {
		log.Fatal("coord-front : metrics.activer exige un token de 16 caractères minimum")
	}
	if fi, err := os.Stat(frontDir); err != nil || !fi.IsDir() {
		log.Fatal("coord-front : dossier front introuvable : ", frontDir)
	}

	// Fail closed : loopback uniquement, aucun écouteur public.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatal("coord-front : adresse invalide : ", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("coord-front : refus fail-closed, loopback uniquement : ", addr)
	}

	dirConfig := filepath.Dir(configPath)
	g := newGateway(cfg, frontDir, dirConfig)
	g.cheminConfig = configPath
	g.mtimeMoteurs = time.Now()
	srv := &http.Server{Addr: addr, Handler: g.routes()}
	fmt.Printf("coord-front : écoute loopback %s (front : %s, comptes : %d, google : %t, github : %t, moteurs : %t)\n",
		addr, frontDir, len(cfg.Comptes), g.googleActif(), g.githubActif(), g.moteursActifs())
	log.Fatal(srv.ListenAndServe())
}

// compteParNomStatique : contrôle au démarrage, avant l'instanciation.
func compteParNomStatique(cfg config, nom string) compteConf {
	for _, c := range cfg.Comptes {
		if c.Nom == nom {
			return c
		}
	}
	return compteConf{}
}

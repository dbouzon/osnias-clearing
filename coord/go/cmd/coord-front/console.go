// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Console de supervision des cinq moteurs IA (mission
// M-COORD-API-STATUS-01, spec v2). Réservée au pilote/administrateur.
//
// Règles implémentées (spec v2, issue #117) :
//   - fail-closed : section console désactivée par défaut (403
//     CONSOLE_DESACTIVEE) ; habilitation = session authentifiée +
//     drapeau admin dans utilisateurs.json ; sans drapeau : 403
//     CONSOLE_REFUSEE, y compris pour un utilisateur connecté ;
//   - GET /api/console/etat : AUCUN appel réseau fournisseur, aucune
//     dépense ; cartes, grille modèles, agrégats jetons (journal
//     durable), contrats déclaratifs, derniers tests ;
//   - tests : dispo (GET modèles, 0 jeton) TOUJOURS possible seule ;
//     réel (facturable) explicite, plafonné (défaut 20 jetons de
//     sortie), jamais automatique ni périodique ;
//   - anti-doublon serveur (409 TEST_EN_COURS), garde-rythme 10/min ;
//   - empreinte sha256 de {fournisseur, clé, endpoint, modèle}
//     calculée et conservée SERVEUR uniquement : le front ne reçoit
//     jamais l'empreinte ni aucun dérivé du secret ;
//   - invalidation : clé/endpoint/modèle modifiés ou fraîcheur
//     dépassée (défaut 7 jours) → PREUVE_PERIMEE ;
//   - erreurs EXPURGÉES en catégories normalisées, jamais le texte
//     brut fournisseur ; aucun secret dans HTML, JSON, logs ;
//   - jetons des tests réels comptés UNE fois via les points
//     d'instrumentation communs (consignerMoteur + compteurs
//     Orpheus), mission "__test_api__", séparables des missions ;
//   - journal des tests atomique 0600 (tests-moteurs.json, purge 90 j).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	testsMoteurFichier      = "tests-moteurs.json"
	testsMoteurConservation = 90 * 24 * time.Hour
	missionTestAPI          = "__test_api__"
	consoleRythmeMax        = 10 // tests/minute/console
)

// consoleConf : section "console" de gateway.json (fail-closed :
// Activer false par défaut). FraicheurJours et PlafondJetonsTest
// clampés ; modifiables à chaud par le pilote seul (relecture mtime).
type consoleConf struct {
	Activer           bool                   `json:"activer"`
	FraicheurJours    int                    `json:"fraicheur_jours"`
	PlafondJetonsTest int                    `json:"plafond_jetons_test"`
	Contrats          map[string]contratConf `json:"contrats"`
}

// contratConf : déclaration pilote (spec §6) — source et date
// obligatoires pour être affichée ; sinon « non renseigné ».
type contratConf struct {
	Type       string  `json:"type"` // jetons | credit
	Quota      float64 `json:"quota"`
	Periode    string  `json:"periode"`
	Consomme   float64 `json:"consomme"`
	Devise     string  `json:"devise"`
	Source     string  `json:"source"`
	DateSource string  `json:"date_source"`
}

func (c consoleConf) fraicheur() time.Duration {
	j := c.FraicheurJours
	if j <= 0 {
		j = 7
	}
	if j > 90 {
		j = 90
	}
	return time.Duration(j) * 24 * time.Hour
}

func (c consoleConf) plafondJetons() int {
	p := c.PlafondJetonsTest
	if p <= 0 {
		p = 20
	}
	if p > 1024 {
		p = 1024
	}
	return p
}

// entreeTestMoteur : une ligne du journal des tests. Empreinte :
// sha256 serveur de {fournisseur, clé, endpoint, modèle} — jamais
// envoyée au front (l'historique servi la retire explicitement).
type entreeTestMoteur struct {
	Instant     string `json:"instant_utc"`
	Fournisseur string `json:"fournisseur"`
	Type        string `json:"type"` // dispo | reel
	Modele      string `json:"modele,omitempty"`
	Resultat    string `json:"resultat"` // ok | erreur
	DureeMs     int64  `json:"duree_ms"`
	Categorie   string `json:"categorie,omitempty"` // erreur expurgée
	Empreinte   string `json:"empreinte"`           // serveur uniquement
}

type fichierTests struct {
	Entrees []entreeTestMoteur `json:"entrees"`
}

// état anti-doublon et garde-rythme de la console.
type etatConsole struct {
	mu      sync.Mutex
	enCours map[string]time.Time // fournisseur → échéance anti-doublon
	rythme  []time.Time          // horodatages des tests lancés (fenêtre 1 min)
}

// -------------------------------------------------------------------
// Accès console : session authentifiée + drapeau admin. Fail-closed.
// -------------------------------------------------------------------

func (g *gateway) adminSession(w http.ResponseWriter, r *http.Request) bool {
	g.regimeMoteurs() // relecture à chaud unique (mtime), puis :
	if !g.regimeConsole().Activer {
		repondreErreur(w, http.StatusForbidden, "CONSOLE_DESACTIVEE",
			"console désactivée dans la configuration")
		return false
	}
	u, ok := g.utilisateurSession(w, r)
	if !ok {
		return false // 401 déjà répondu par utilisateurSession
	}
	if !u.Admin {
		repondreErreur(w, http.StatusForbidden, "CONSOLE_REFUSEE",
			"console réservée au pilote/administrateur")
		return false
	}
	return true
}

// originAutorisee : contrôle Origin/Referer sur les mutations POST
// (anti-CSRF, même origine uniquement).
func originAutorisee(r *http.Request) bool {
	for _, entete := range []string{"Origin", "Referer"} {
		if v := strings.TrimSpace(r.Header.Get(entete)); v != "" {
			if hote, ok := hoteURL(v); ok && hote == r.Host {
				return true
			}
			return false
		}
	}
	return true // ni Origin ni Referer : requête non-navigateur (curl test)
}

func hoteURL(u string) (string, bool) {
	s := u
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	return s, s != ""
}

// regimeConsole : section console courante. Le rechargement à chaud
// (mtime) est fait par regimeMoteurs — point de relecture unique ;
// cette fonction sert l'état courant sans relecture propre.
func (g *gateway) regimeConsole() consoleConf {
	g.muMoteurs.Lock()
	defer g.muMoteurs.Unlock()
	return g.console
}

// -------------------------------------------------------------------
// Journal des tests (atomique 0600, purge 90 jours).
// -------------------------------------------------------------------

func (g *gateway) lireTests() fichierTests {
	f := fichierTests{}
	g.muJournal.Lock()
	defer g.muJournal.Unlock()
	if raw, err := os.ReadFile(g.cheminTests); err == nil {
		_ = json.Unmarshal(raw, &f)
	}
	return f
}

func (g *gateway) consignerTest(e entreeTestMoteur) {
	g.muJournal.Lock()
	defer g.muJournal.Unlock()
	f := fichierTests{}
	if raw, err := os.ReadFile(g.cheminTests); err == nil {
		_ = json.Unmarshal(raw, &f)
	}
	f.Entrees = append(f.Entrees, e)
	limite := time.Now().Add(-testsMoteurConservation)
	purgees := []entreeTestMoteur{}
	for _, ent := range f.Entrees {
		if t, err := time.Parse(time.RFC3339, ent.Instant); err == nil && t.After(limite) {
			purgees = append(purgees, ent)
		}
	}
	f.Entrees = purgees
	raw, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return
	}
	tmp := g.cheminTests + ".tmp"
	if os.WriteFile(tmp, raw, 0600) == nil {
		_ = os.Rename(tmp, g.cheminTests)
	}
}

// -------------------------------------------------------------------
// Empreinte de configuration (serveur uniquement) et fraîcheur.
// -------------------------------------------------------------------

func empreinteConfig(fournisseur, cle, url, modele string) string {
	h := sha256.Sum256([]byte(fournisseur + "|" + cle + "|" + url + "|" + modele))
	return hex.EncodeToString(h[:])
}

// dernierTest : dernière entrée d'un fournisseur dont l'empreinte
// correspond ENCORE à la configuration courante et dont la fraîcheur
// n'est pas dépassée. Aucune valeur de clé ni empreinte ne sort.
func dernierTestValide(tests fichierTests, fournisseur string, conf fournisseurMoteurConf, modele string, fraicheur time.Duration) (entreeTestMoteur, bool, string) {
	attendue := empreinteConfig(fournisseur, conf.CleAPI, conf.URLAPI, modele)
	var derniere *entreeTestMoteur
	for i := range tests.Entrees {
		e := &tests.Entrees[i]
		if e.Fournisseur != fournisseur {
			continue
		}
		if derniere == nil || e.Instant > derniere.Instant {
			derniere = e
		}
	}
	if derniere == nil {
		return entreeTestMoteur{}, false, "NON_TESTEE"
	}
	if derniere.Resultat != "ok" {
		return *derniere, false, "ERREUR"
	}
	if derniere.Empreinte != attendue {
		return entreeTestMoteur{}, false, "PREUVE_PERIMEE"
	}
	t, err := time.Parse(time.RFC3339, derniere.Instant)
	if err != nil || time.Since(t) > fraicheur {
		return entreeTestMoteur{}, false, "PREUVE_PERIMEE"
	}
	return *derniere, true, "OPERATIONNELLE"
}

// -------------------------------------------------------------------
// Agrégats jetons depuis le journal durable (bornes UTC, purge 90 j).
// -------------------------------------------------------------------

type agregatMoteur struct {
	Moteur       string `json:"moteur"`
	Appels       int    `json:"appels"`
	JetonsEntree int    `json:"jetons_entree"`
	JetonsSortie int    `json:"jetons_sortie"`
}

func (g *gateway) agregerJetons(periode string) ([]agregatMoteur, []agregatMoteur, string, string, int) {
	type fichier struct {
		Entrees []entreeJournalMoteur `json:"entrees"`
	}
	f := fichier{}
	g.muJournal.Lock()
	if raw, err := os.ReadFile(g.cheminJournal); err == nil {
		_ = json.Unmarshal(raw, &f)
	}
	g.muJournal.Unlock()

	maintenant := time.Now().UTC()
	debut := time.Time{}
	if periode == "mois" {
		debut = time.Date(maintenant.Year(), maintenant.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	missions := map[string]*agregatMoteur{}
	tests := map[string]*agregatMoteur{}
	for _, e := range f.Entrees {
		t, err := time.Parse(time.RFC3339, e.Instant)
		if err != nil {
			continue
		}
		if !debut.IsZero() && t.Before(debut) {
			continue
		}
		cible := missions
		if e.Mission == missionTestAPI {
			cible = tests
		}
		a := cible[e.Moteur]
		if a == nil {
			a = &agregatMoteur{Moteur: e.Moteur}
			cible[e.Moteur] = a
		}
		a.Appels++
		a.JetonsEntree += e.JetonsEntree
		a.JetonsSortie += e.JetonsSortie
	}
	vers := func(m map[string]*agregatMoteur) []agregatMoteur {
		out := []agregatMoteur{}
		for _, v := range m {
			out = append(out, *v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Moteur < out[j].Moteur })
		return out
	}
	borneDebut, borneFin := "", ""
	if periode == "mois" {
		borneDebut = debut.Format(time.RFC3339)
		borneFin = maintenant.Format(time.RFC3339)
	}
	// Borne haute de couverture réelle : plus ancienne entrée conservée.
	for _, e := range f.Entrees {
		if t, err := time.Parse(time.RFC3339, e.Instant); err == nil {
			if periode != "mois" || !t.Before(debut) {
				borneDebut = t.Format(time.RFC3339)
				break
			}
		}
	}
	if periode != "mois" {
		borneFin = maintenant.Format(time.RFC3339)
	}
	return vers(missions), vers(tests), borneDebut, borneFin, len(f.Entrees)
}

// -------------------------------------------------------------------
// GET /api/console/etat — aucune dépense, aucun appel fournisseur.
// -------------------------------------------------------------------

func (g *gateway) consoleEtat(w http.ResponseWriter, r *http.Request) {
	if !g.adminSession(w, r) {
		return
	}
	regime := g.regimeMoteurs()
	console := g.regimeConsole()
	periode := r.URL.Query().Get("periode")
	if periode != "mois" {
		periode = "historique"
	}
	fournisseurs := regime.fournisseurs()
	tests := g.lireTests()

	cartes := []map[string]any{}
	for _, nom := range []string{"mistral", "anthropic", "openai", "gemini", "deepseek"} {
		f := fournisseurs[nom]
		modele := premierModeleFournisseur(regime, nom)
		carte := map[string]any{
			"fournisseur":  nom,
			"integre":      true,
			"active":       f.pret(),
			"cle_presente": strings.TrimSpace(f.CleAPI) != "" && strings.TrimSpace(f.CleAPI) != cleMoteurNonRenseignee,
			"modele_teste": modele,
			"contrat":      contratAffichable(console.Contrats[nom]),
		}
		dernier, valide, etat := dernierTestValide(tests, nom, f, modele, console.fraicheur())
		carte["etat"] = etat
		carte["preuve_valide"] = valide
		if etat == "ERREUR" {
			carte["dernier_test"] = map[string]any{
				"instant_utc": dernier.Instant, "type": dernier.Type,
				"duree_ms": dernier.DureeMs, "categorie": dernier.Categorie,
			}
		} else if valide {
			carte["dernier_test"] = map[string]any{
				"instant_utc": dernier.Instant, "type": dernier.Type,
				"duree_ms": dernier.DureeMs,
			}
		}
		cartes = append(cartes, carte)
	}

	// Grille des modèles par niveau/étape + identifiant réellement
	// retourné lors du dernier test réel réussi du modèle.
	grille := map[string]map[string]map[string]any{}
	for _, niveau := range []string{"essentiel", "avance", "premium"} {
		niv, ok := regime.Niveaux[niveau]
		if !ok {
			continue
		}
		grille[niveau] = map[string]map[string]any{}
		for _, etape := range []string{"cadrage", "redaction", "critique", "revision"} {
			e, _ := niv.etape(etape)
			entree := map[string]any{"moteur": e.Moteur, "modele": e.Modele, "version": "non_communicquee"}
			if id, ok := identifiantRetourne(tests, e.Moteur, e.Modele); ok {
				entree["identifiant_retourne"] = id
			}
			grille[niveau][etape] = entree
		}
	}

	missions, testsAg, borneDebut, borneFin, totalEntrees := g.agregerJetons(periode)
	reponse := map[string]any{
		"periode":            periode,
		"borne_debut_utc":    borneDebut,
		"borne_fin_utc":      borneFin,
		"origine":            "journal-moteurs.json (purge 90 jours ; aucun total depuis l'origine)",
		"couverture":         "compteur COORD uniquement (le compte fournisseur entier n'est pas observable)",
		"entrees_conservees": totalEntrees,
		"fournisseurs":       cartes,
		"grille":             grille,
		"jetons_missions":    missions,
		"jetons_tests":       testsAg,
		"actualisation_utc":  time.Now().UTC().Format(time.RFC3339),
	}
	repondreJSON(w, http.StatusOK, reponse)
}

// identifiantRetourne : modèle effectivement renvoyé par l'API lors du
// dernier test réel réussi du modèle configuré (alias ≠ version).
func identifiantRetourne(tests fichierTests, fournisseur, modele string) (string, bool) {
	_ = modele
	for i := len(tests.Entrees) - 1; i >= 0; i-- {
		e := tests.Entrees[i]
		if e.Fournisseur == fournisseur && e.Type == "reel" && e.Resultat == "ok" && e.Modele != "" {
			return e.Modele, true
		}
	}
	return "", false
}

// premierModeleFournisseur : premier modèle de la grille qui pointe ce
// fournisseur (ordre déterministe niveaux puis étapes). Vide si absent.
func premierModeleFournisseur(regime moteursConf, fournisseur string) string {
	for _, niveau := range []string{"essentiel", "avance", "premium"} {
		niv, ok := regime.Niveaux[niveau]
		if !ok {
			continue
		}
		for _, etape := range []string{"cadrage", "redaction", "critique", "revision"} {
			e, _ := niv.etape(etape)
			if e.Moteur == fournisseur {
				return e.Modele
			}
		}
	}
	return ""
}

// contratAffichable : source + date obligatoires, sinon non renseigné
// (jamais zéro ni illimité) ; unité native, aucune conversion.
func contratAffichable(c contratConf) map[string]any {
	if strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.DateSource) == "" {
		return nil
	}
	out := map[string]any{
		"type": c.Type, "quota": nil, "consomme": nil,
		"periode": c.Periode, "source": c.Source, "date_source": c.DateSource,
	}
	if c.Quota > 0 {
		out["quota"] = c.Quota
	}
	if c.Consomme >= 0 {
		out["consomme"] = c.Consomme
	}
	if c.Type == "credit" {
		out["devise"] = c.Devise
	}
	if out["quota"] == nil {
		out["non_renseigne"] = true
	}
	return out
}

// -------------------------------------------------------------------
// POST /api/console/test — dispo (0 €) ou réel (facturable, explicite).
// -------------------------------------------------------------------

func (g *gateway) consoleTest(w http.ResponseWriter, r *http.Request) {
	if !g.adminSession(w, r) {
		return
	}
	if !originAutorisee(r) {
		repondreErreur(w, http.StatusForbidden, "ORIGINE_REFUSEE", "origine non autorisée")
		return
	}
	var corps struct {
		Fournisseur string `json:"fournisseur"`
		Type        string `json:"type"`
	}
	if err := lireJSON(r, &corps); err != nil {
		repondreErreur(w, http.StatusBadRequest, "REQUETE_INVALIDE", "corps JSON invalide")
		return
	}
	regime := g.regimeMoteurs()
	fournisseurs := regime.fournisseurs()
	f, connu := fournisseurs[corps.Fournisseur]
	if !connu {
		repondreErreur(w, http.StatusNotFound, "FOURNISSEUR_INCONNU", "fournisseur inconnu")
		return
	}
	if corps.Type != "dispo" && corps.Type != "reel" {
		repondreErreur(w, http.StatusBadRequest, "TYPE_INVALIDE", "type : dispo ou reel")
		return
	}
	if !f.pret() {
		g.consignerTest(entreeTestMoteur{
			Instant: time.Now().UTC().Format(time.RFC3339), Fournisseur: corps.Fournisseur,
			Type: corps.Type, Resultat: "erreur", Categorie: "desactive",
			Empreinte: empreinteConfig(corps.Fournisseur, f.CleAPI, f.URLAPI, ""),
		})
		repondreErreur(w, http.StatusServiceUnavailable, "MOTEUR_DESACTIVE",
			"fournisseur inactif ou clé absente")
		return
	}
	console := g.regimeConsole()
	modele := premierModeleFournisseur(regime, corps.Fournisseur)
	if corps.Type == "reel" && modele == "" {
		repondreErreur(w, http.StatusConflict, "MODELE_INCONNU",
			"fournisseur absent de la grille des niveaux")
		return
	}

	// Anti-doublon serveur + garde-rythme (avant tout appel réseau).
	g.muConsole.Lock()
	if g.consoleEtatServeur == nil {
		g.consoleEtatServeur = &etatConsole{enCours: map[string]time.Time{}}
	}
	ec := g.consoleEtatServeur
	ec.mu.Lock()
	maintenant := time.Now()
	if echeance, actif := ec.enCours[corps.Fournisseur]; actif && echeance.After(maintenant) {
		ec.mu.Unlock()
		g.muConsole.Unlock()
		repondreErreur(w, http.StatusConflict, "TEST_EN_COURS",
			"un test est déjà en cours pour ce fournisseur")
		return
	}
	// Purge de la fenêtre de rythme.
	fenetre := []time.Time{}
	for _, t := range ec.rythme {
		if maintenant.Sub(t) < time.Minute {
			fenetre = append(fenetre, t)
		}
	}
	if len(fenetre) >= consoleRythmeMax {
		ec.mu.Unlock()
		g.muConsole.Unlock()
		repondreErreur(w, http.StatusTooManyRequests, "CONSOLE_RYTHME",
			"trop de tests par minute")
		return
	}
	timeout := regime.timeout("dialogue") + 5*time.Second
	ec.enCours[corps.Fournisseur] = maintenant.Add(timeout)
	ec.rythme = append(fenetre, maintenant)
	ec.mu.Unlock()
	g.muConsole.Unlock()
	defer func() {
		ec.mu.Lock()
		delete(ec.enCours, corps.Fournisseur)
		ec.mu.Unlock()
	}()

	var resultat map[string]any
	if corps.Type == "dispo" {
		resultat = g.testDispo(corps.Fournisseur, f, modele, console)
	} else {
		resultat = g.testReel(regime, corps.Fournisseur, f, modele, console)
	}
	repondreJSON(w, http.StatusOK, resultat)
}

// testDispo : GET des modèles du fournisseur, 0 jeton facturable.
// Succès = HTTP 200 + liste décodable (jamais de faux succès).
func (g *gateway) testDispo(nom string, f fournisseurMoteurConf, modele string, console consoleConf) map[string]any {
	base := baseFournisseur(nom, f)
	url := strings.TrimSuffix(base, "/") + "/v1/models"
	if nom == "anthropic" {
		url = strings.TrimSuffix(base, "/") + "/v1/models"
	}
	debut := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return g.journaliserTest(nom, "dispo", "", f, console, "erreur", "interne", debut)
	}
	if nom == "anthropic" {
		req.Header.Set("x-api-key", f.CleAPI)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+f.CleAPI)
	}
	rep, err := client.Do(req)
	if err != nil {
		return g.journaliserTest(nom, "dispo", "", f, console, "erreur", categorieErreur(err, 0), debut)
	}
	defer rep.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(rep.Body, 1<<20))
	if rep.StatusCode != http.StatusOK {
		return g.journaliserTest(nom, "dispo", "", f, console, "erreur", categorieErreur(nil, rep.StatusCode), debut)
	}
	return g.journaliserTest(nom, "dispo", "", f, console, "ok", "", debut)
}

// testReel : appel réel plafonné au modèle de la grille, compté UNE
// fois dans le chemin commun (journal + Orpheus), mission __test_api__.
// Succès = 200 + réponse décodable + texte non vide + jetons_sortie > 0.
func (g *gateway) testReel(regime moteursConf, nom string, f fournisseurMoteurConf, modele string, console consoleConf) map[string]any {
	base := baseFournisseur(nom, f)
	plafond := console.plafondJetons()
	systeme := "Test de connexion COORD. Réponds uniquement par : OK."
	demande := "OK ?"
	debut := time.Now()
	timeout := regime.timeout("dialogue")
	var res resultatMoteur
	var errReseau error
	switch {
	case nom == "anthropic":
		corps := map[string]any{
			"model": modele, "max_tokens": plafond, "system": systeme,
			"messages": []messageMoteur{{Role: "user", Contenu: demande}},
		}
		res, errReseau = postMoteur(strings.TrimSuffix(base, "/")+"/v1/messages",
			"x-api-key "+f.CleAPI, corps, timeout)
	case nom == "gemini":
		corps := map[string]any{
			"model": modele, "max_tokens": plafond,
			"messages": []messageMoteur{{Role: "system", Contenu: systeme}, {Role: "user", Contenu: demande}},
		}
		res, errReseau = postMoteur(strings.TrimSuffix(base, "/")+"/chat/completions",
			"Bearer "+f.CleAPI, corps, timeout)
	default:
		corps := map[string]any{
			"model": modele, "max_tokens": plafond,
			"messages": []messageMoteur{{Role: "system", Contenu: systeme}, {Role: "user", Contenu: demande}},
		}
		res, errReseau = postMoteur(strings.TrimSuffix(base, "/")+"/v1/chat/completions",
			"Bearer "+f.CleAPI, corps, timeout)
	}
	if errReseau != nil {
		g.consignerJournalTest(nom, modele, f, "erreur", time.Since(debut).Milliseconds(), categorieErreurReseau(errReseau), 0, 0)
		return g.journaliserTest(nom, "reel", modele, f, console, "erreur", categorieErreurReseau(errReseau), debut)
	}
	if strings.TrimSpace(res.Texte) == "" || res.JetonsSortie <= 0 {
		// Jamais de faux succès : réponse ambiguë = échec.
		g.consignerJournalTest(nom, modele, f, "erreur", time.Since(debut).Milliseconds(), "reponse_ambigue", 0, 0)
		return g.journaliserTest(nom, "reel", modele, f, console, "erreur", "reponse_ambigue", debut)
	}
	g.consignerJournalTest(nom, modele, f, "ok", time.Since(debut).Milliseconds(), "", res.JetonsEntree, res.JetonsSortie)
	return g.journaliserTest(nom, "reel", modele, f, console, "ok", "", debut)
}

// consignerJournalTest : une seule écriture dans le journal commun et
// un seul incrément des compteurs Orpheus (mission __test_api__) —
// les tests sont séparables des missions, jamais comptés deux fois.
func (g *gateway) consignerJournalTest(nom, modele string, f fournisseurMoteurConf, resultat string, dureeMs int64, categorie string, jetonsEntree, jetonsSortie int) {
	entree := entreeJournalMoteur{
		Instant: time.Now().UTC().Format(time.RFC3339), Mission: missionTestAPI,
		Niveau: "test", Etape: "connexion", Moteur: nom, Modele: modele,
		JetonsEntree: jetonsEntree, JetonsSortie: jetonsSortie,
		DureeMs: dureeMs, Resultat: resultat, Essai: 1,
	}
	if resultat != "ok" {
		entree.Erreur = "erreur de test expurgée : " + categorie
	}
	g.consignerMoteur(entree)
	if resultat == "ok" {
		g.compterAppelMoteur(nom, modele, "test", "connexion", "ok")
	} else {
		g.compterAppelMoteur(nom, modele, "test", "connexion", "erreur")
	}
	if jetonsEntree > 0 || jetonsSortie > 0 {
		g.compterJetonsMoteurs(nom, modele, jetonsEntree, jetonsSortie)
	}
}

// journaliserTest : écrit l'entrée dans tests-moteurs.json et renvoie
// la réponse EXPURGÉE servie au front (catégorie, jamais le motif brut ;
// jamais l'empreinte, jamais la clé).
func (g *gateway) journaliserTest(nom, typeTest, modele string, f fournisseurMoteurConf, console consoleConf, resultat, categorie string, debut time.Time) map[string]any {
	e := entreeTestMoteur{
		Instant: time.Now().UTC().Format(time.RFC3339), Fournisseur: nom,
		Type: typeTest, Resultat: resultat, Modele: modele,
		DureeMs: time.Since(debut).Milliseconds(), Categorie: categorie,
		Empreinte: empreinteConfig(nom, f.CleAPI, f.URLAPI, modele),
	}
	g.consignerTest(e)
	out := map[string]any{
		"fournisseur": nom, "type": typeTest, "resultat": resultat,
		"duree_ms": e.DureeMs, "instant_utc": e.Instant,
	}
	if modele != "" {
		out["modele"] = modele
	}
	if categorie != "" {
		out["categorie"] = categorie
	}
	// Fraîcheur recalculée : la preuve vient d'être posée ou invalidée.
	tests := g.lireTests()
	_, valide, etat := dernierTestValide(tests, nom, f, modele, console.fraicheur())
	out["etat"] = etat
	out["preuve_valide"] = valide
	return out
}

// baseFournisseur : endpoint effectif du fournisseur (production ou
// substitution url_api pour les serveurs factices de recette).
func baseFournisseur(nom string, f fournisseurMoteurConf) string {
	if f.URLAPI != "" {
		return f.URLAPI
	}
	switch nom {
	case "mistral":
		return "https://api.mistral.ai"
	case "openai":
		return "https://api.openai.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case "deepseek":
		return "https://api.deepseek.com"
	case "anthropic":
		return "https://api.anthropic.com"
	}
	return ""
}

// categorieErreur : normalisation EXPURGÉE des échecs — le texte brut
// fournisseur (qui peut contenir des fragments de secret ou de requête)
// n'est JAMAIS conservé ni servi.
func categorieErreur(err error, statut int) string {
	if statut == 0 && err != nil {
		return categorieErreurReseau(err)
	}
	switch {
	case statut == 401 || statut == 403:
		return "authentification"
	case statut == 429:
		return "quota"
	case statut == 404:
		return "modele_indisponible"
	case statut >= 500:
		return "interne_fournisseur"
	}
	return "autre"
}

func categorieErreurReseau(err error) string {
	m := strings.ToLower(err.Error())
	switch {
	case strings.Contains(m, "timeout"), strings.Contains(m, "deadline"):
		return "timeout"
	case strings.Contains(m, "connection refused"), strings.Contains(m, "no such host"),
		strings.Contains(m, "connection reset"):
		return "reseau"
	}
	// Erreurs HTTP du fournisseur : statut présent dans le motif.
	if strings.Contains(m, "http 401") || strings.Contains(m, "http 403") {
		return "authentification"
	}
	if strings.Contains(m, "http 429") {
		return "quota"
	}
	if strings.Contains(m, "http 404") {
		return "modele_indisponible"
	}
	if strings.Contains(strings.ToLower(m), "http 5") {
		return "interne_fournisseur"
	}
	return "autre"
}

// -------------------------------------------------------------------
// GET /api/console/historique-tests — expurgé (jamais d'empreinte).
// -------------------------------------------------------------------

func (g *gateway) consoleHistorique(w http.ResponseWriter, r *http.Request) {
	if !g.adminSession(w, r) {
		return
	}
	tests := g.lireTests()
	out := []map[string]any{}
	for _, e := range tests.Entrees {
		l := map[string]any{
			"instant_utc": e.Instant, "fournisseur": e.Fournisseur,
			"type": e.Type, "resultat": e.Resultat, "duree_ms": e.DureeMs,
		}
		if e.Modele != "" {
			l["modele"] = e.Modele
		}
		if e.Categorie != "" {
			l["categorie"] = e.Categorie
		}
		out = append(out, l)
	}
	repondreJSON(w, http.StatusOK, map[string]any{"entrees": out})
}

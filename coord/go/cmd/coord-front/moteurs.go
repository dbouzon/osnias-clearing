// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Moteurs IA via API (mission M-COORD-API-MISTRAL-01, spec v0.3) :
// appels serveur vers Mistral, Anthropic et OpenAI, grille de TROIS
// niveaux = itinéraires de cascade (cadrage → rédaction → critique →
// révision), chaque étape pointant un moteur/modèle DÉFINI EN
// CONFIGURATION (gateway.json, section "moteurs"), jamais en dur.
//
// Règles du pilote implémentées ici :
//   - fail-closed : activer:false par défaut, clé absente ou
//     A_RENSEIGNER_PAR_LE_PILOTE = moteur indisponible (503
//     MOTEUR_DESACTIVE) ; sans AUCUNE clé, repli sur le comportement
//     actuel (script déterministe du moteur démo) ;
//   - vigilance coût : plafond d'appels par mission (défaut 50, refus
//     propre PLAFOND_ATTEINT), alerte préventive à 80 %, journal JSON
//     atomique 0600 (date, mission, niveau, étape, moteur, modèle,
//     jetons, durée, résultat), purge 90 jours, JAMAIS de contenu de
//     mission ni de clé ;
//   - timeouts différenciés (cadrage 60 s, dialogue 30 s), au plus UN
//     nouvel essai en cas d'erreur réseau, essai journalisé ;
//   - relecture de gateway.json (mtime) à chaque création de mission
//     et à chaque appel moteur : la grille se change à chaud ;
//   - escalade Premium bornée à UNE par mission (étape "revision"),
//     comptée au journal ;
//   - stdlib uniquement, clés uniquement dans gateway.json (0640
//     root:coord), jamais dans le front, le dépôt ou les journaux.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	cleMoteurNonRenseignee    = "A_RENSEIGNER_PAR_LE_PILOTE"
	journalMoteurFichier      = "journal-moteurs.json"
	journalMoteurConservation = 90 * 24 * time.Hour
)

// Niveaux connus (spec §3.1) : le client en choisit UN à la création,
// il est stocké dans la définition de mission et ne change plus.
var niveauxMoteursConnus = map[string]bool{"essentiel": true, "avance": true, "premium": true}

// Étapes d'un itinéraire (spec §3.1) : cadrage → rédaction → critique
// → révision (la révision sert d'escalade Premium bornée).
var etapesMoteursConnues = map[string]bool{"cadrage": true, "redaction": true, "critique": true, "revision": true}

type fournisseurMoteurConf struct {
	Activer bool   `json:"activer"`
	CleAPI  string `json:"cle_api"`
	// URLAPI : endpoint de substitution (recette interne, serveurs
	// factices) ; vide = endpoint de production du fournisseur.
	URLAPI string `json:"url_api,omitempty"`
}

func (f fournisseurMoteurConf) pret() bool {
	return f.Activer && strings.TrimSpace(f.CleAPI) != "" &&
		strings.TrimSpace(f.CleAPI) != cleMoteurNonRenseignee
}

type etapeMoteurConf struct {
	Moteur string `json:"moteur"`
	Modele string `json:"modele"`
	// Repli (optionnel) : fournisseur de secours de l'étape si le
	// fournisseur principal est désactivé (spec §3.2).
	Repli string `json:"repli,omitempty"`
}

type niveauMoteurConf struct {
	Cadrage   etapeMoteurConf `json:"cadrage"`
	Redaction etapeMoteurConf `json:"redaction"`
	Critique  etapeMoteurConf `json:"critique"`
	Revision  etapeMoteurConf `json:"revision"`
}

func (n niveauMoteurConf) etape(nom string) (etapeMoteurConf, bool) {
	switch nom {
	case "cadrage":
		return n.Cadrage, true
	case "redaction":
		return n.Redaction, true
	case "critique":
		return n.Critique, true
	case "revision":
		return n.Revision, true
	}
	return etapeMoteurConf{}, false
}

type moteursConf struct {
	Mistral         fournisseurMoteurConf       `json:"mistral"`
	Anthropic       fournisseurMoteurConf       `json:"anthropic"`
	Openai          fournisseurMoteurConf       `json:"openai"`
	Niveaux         map[string]niveauMoteurConf `json:"niveaux"`
	PlafondAppels   int                         `json:"plafond_appels"`
	TimeoutCadrage  int                         `json:"timeout_cadrage"`
	TimeoutDialogue int                         `json:"timeout_dialogue"`
}

// validerMoteurs : fail-closed au démarrage (main) et à chaque
// relecture à chaud : une section invalide n'est JAMAIS activée.
func validerMoteurs(mc moteursConf) error {
	fournisseurs := mc.fournisseurs()
	for nom, f := range fournisseurs {
		if f.Activer {
			if strings.TrimSpace(f.CleAPI) == "" || strings.TrimSpace(f.CleAPI) == cleMoteurNonRenseignee {
				return fmt.Errorf("moteurs.%s.activer sans cle_api réelle", nom)
			}
		}
	}
	for niveau, niv := range mc.Niveaux {
		if !niveauxMoteursConnus[niveau] {
			return fmt.Errorf("niveau inconnu : %s", niveau)
		}
		for _, etape := range []string{"cadrage", "redaction", "critique", "revision"} {
			e, _ := niv.etape(etape)
			if !etapesMoteursConnues[etape] {
				return fmt.Errorf("etape inconnue") // garde statique
			}
			if _, ok := fournisseurs[e.Moteur]; !ok {
				return fmt.Errorf("niveau %s, étape %s : moteur inconnu : %s", niveau, etape, e.Moteur)
			}
			if strings.TrimSpace(e.Modele) == "" {
				return fmt.Errorf("niveau %s, étape %s : modele requis", niveau, etape)
			}
			if e.Repli != "" {
				if _, ok := fournisseurs[e.Repli]; !ok {
					return fmt.Errorf("niveau %s, étape %s : repli inconnu : %s", niveau, etape, e.Repli)
				}
				if e.Repli == e.Moteur {
					return fmt.Errorf("niveau %s, étape %s : repli identique au moteur", niveau, etape)
				}
			}
		}
	}
	return nil
}

func (mc moteursConf) fournisseurs() map[string]fournisseurMoteurConf {
	return map[string]fournisseurMoteurConf{
		"mistral":   mc.Mistral,
		"anthropic": mc.Anthropic,
		"openai":    mc.Openai,
	}
}

// actifs : au moins un fournisseur réellement activé. Sinon, repli
// global sur le comportement actuel (script déterministe, 0 €).
func (mc moteursConf) actifs() bool {
	for _, f := range mc.fournisseurs() {
		if f.pret() {
			return true
		}
	}
	return false
}

// plafond : plafond d'appels par mission, clampé (défaut 50).
func (mc moteursConf) plafond() int {
	p := mc.PlafondAppels
	if p <= 0 {
		p = 50
	}
	if p > 1000 {
		p = 1000
	}
	return p
}

// timeout : cadrage plus long que le dialogue, clampés (60/30 s).
func (mc moteursConf) timeout(etape string) time.Duration {
	defaut := 30
	if etape == "cadrage" {
		defaut = 60
	}
	t := defaut
	if etape == "cadrage" && mc.TimeoutCadrage > 0 {
		t = mc.TimeoutCadrage
	}
	if etape != "cadrage" && mc.TimeoutDialogue > 0 {
		t = mc.TimeoutDialogue
	}
	if t < 5 {
		t = 5
	}
	if t > 300 {
		t = 300
	}
	return time.Duration(t) * time.Second
}

// ---- Régime : relecture à chaud de gateway.json (mtime) ------------

// regimeMoteurs : section moteurs courante. Recharge gateway.json si
// son mtime a changé (spec §3.2 : configuration modifiable sans
// redéploiement). Échec de relecture = on garde l'ancien régime
// (fail-closed), l'erreur est journalisée côté serveur.
func (g *gateway) regimeMoteurs() moteursConf {
	g.muMoteurs.Lock()
	defer g.muMoteurs.Unlock()
	if g.cheminConfig == "" {
		return g.moteurs
	}
	fi, err := os.Stat(g.cheminConfig)
	if err != nil || !fi.ModTime().After(g.mtimeMoteurs) {
		return g.moteurs
	}
	raw, err := os.ReadFile(g.cheminConfig)
	if err != nil {
		log.Printf("coord-front : relecture moteurs impossible : %v", err)
		return g.moteurs
	}
	var partiel struct {
		Moteurs moteursConf `json:"moteurs"`
	}
	if err := json.Unmarshal(raw, &partiel); err != nil {
		log.Printf("coord-front : relecture moteurs : json invalide : %v", err)
		return g.moteurs
	}
	if err := validerMoteurs(partiel.Moteurs); err != nil {
		log.Printf("coord-front : relecture moteurs refusée (fail-closed) : %v", err)
		return g.moteurs
	}
	g.moteurs = partiel.Moteurs
	g.mtimeMoteurs = fi.ModTime()
	return g.moteurs
}

func (g *gateway) moteursActifs() bool {
	return g.regimeMoteurs().actifs()
}

// ---- Journal de coût (spec §3.3) ------------------------------------

type entreeJournalMoteur struct {
	Instant      string `json:"instant_utc"`
	Mission      string `json:"mission"`
	Niveau       string `json:"niveau"`
	Etape        string `json:"etape"`
	Moteur       string `json:"moteur"`
	Modele       string `json:"modele"`
	JetonsEntree int    `json:"jetons_entree"`
	JetonsSortie int    `json:"jetons_sortie"`
	DureeMs      int64  `json:"duree_ms"`
	Resultat     string `json:"resultat"`
	Essai        int    `json:"essai"`
	Erreur       string `json:"erreur,omitempty"`
}

// consignerMoteur : journal JSON atomique (tmp + rename), 0600, purgé
// au-delà de 90 jours. JAMAIS de contenu de mission ni de clé ici.
func (g *gateway) consignerMoteur(e entreeJournalMoteur) {
	g.muJournal.Lock()
	defer g.muJournal.Unlock()
	type fichier struct {
		Entrees []entreeJournalMoteur `json:"entrees"`
	}
	f := fichier{}
	if raw, err := os.ReadFile(g.cheminJournal); err == nil {
		_ = json.Unmarshal(raw, &f)
	}
	f.Entrees = append(f.Entrees, e)
	limite := time.Now().Add(-journalMoteurConservation)
	purgees := []entreeJournalMoteur{}
	for _, ent := range f.Entrees {
		if t, err := time.Parse(time.RFC3339, ent.Instant); err == nil && t.After(limite) {
			purgees = append(purgees, ent)
		}
	}
	f.Entrees = purgees
	raw, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		log.Printf("coord-front : journal moteurs : marshal : %v", err)
		return
	}
	tmp := g.cheminJournal + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		log.Printf("coord-front : journal moteurs : écriture : %v", err)
		return
	}
	if err := os.Rename(tmp, g.cheminJournal); err != nil {
		log.Printf("coord-front : journal moteurs : renommage : %v", err)
	}
}

// ---- Appel unifié d'une étape ---------------------------------------

type messageMoteur struct {
	Role    string `json:"role"`
	Contenu string `json:"content"`
}

type resultatMoteur struct {
	Texte        string
	JetonsEntree int
	JetonsSortie int
}

type erreurMoteur struct {
	Statut int
	Code   string
	Motif  string
}

func (e *erreurMoteur) Error() string { return e.Code + " : " + e.Motif }

// appelEtape : résout étape du niveau → moteur/modèle en
// configuration, applique plafond et timeouts, passe AU PLUS UN nouvel
// essai sur erreur réseau, journalise chaque appel (succès comme échec).
// system/demande : le prompt serveur de l'étape (contexte de mission
// fourni par l'appelant ; jamais de secret dedans). Le verrou du moteur
// démo peut être tenu par l'appelant : cette fonction ne prend que les
// verrous du gateway (indépendants).
func (g *gateway) appelEtape(p *projetDemo, etape, system, demande string) (resultatMoteur, *erreurMoteur) {
	regime := g.regimeMoteurs()
	if !etapesMoteursConnues[etape] {
		return resultatMoteur{}, &erreurMoteur{http.StatusServiceUnavailable, "MOTEUR_DESACTIVE", "étape inconnue : " + etape}
	}
	niveau := p.Niveau
	if niveau == "" {
		niveau = "avance"
	}
	niv, ok := regime.Niveaux[niveau]
	if !ok {
		return resultatMoteur{}, &erreurMoteur{http.StatusServiceUnavailable, "MOTEUR_DESACTIVE",
			"niveau sans itinéraire : " + niveau}
	}
	e, _ := niv.etape(etape)
	fournisseurs := regime.fournisseurs()

	// Résolution : fournisseur principal, puis repli configuré.
	moteur := ""
	var conf fournisseurMoteurConf
	candidats := []string{e.Moteur}
	if e.Repli != "" {
		candidats = append(candidats, e.Repli)
	}
	for _, c := range candidats {
		if f, ok := fournisseurs[c]; ok && f.pret() {
			moteur = c
			conf = f
			break
		}
	}
	if moteur == "" {
		return resultatMoteur{}, &erreurMoteur{http.StatusServiceUnavailable, "MOTEUR_DESACTIVE",
			"fournisseur inactif pour " + niveau + "/" + etape}
	}

	// Plafond par mission : refus propre AVANT tout appel réseau.
	if p.AppelsMoteurs >= regime.plafond() {
		return resultatMoteur{}, &erreurMoteur{http.StatusTooManyRequests, "PLAFOND_ATTEINT",
			fmt.Sprintf("plafond d'appels atteint : %d", regime.plafond())}
	}

	base := conf.URLAPI
	if base == "" {
		switch moteur {
		case "mistral":
			base = "https://api.mistral.ai"
		case "openai":
			base = "https://api.openai.com"
		}
	}
	timeout := regime.timeout(etape)
	debut := time.Now()
	var res resultatMoteur
	var errReseau error
	essais := 1
	if moteur == "anthropic" {
		urlAnthropic := conf.URLAPI
		if urlAnthropic == "" {
			urlAnthropic = "https://api.anthropic.com"
		}
		res, errReseau = appelAnthropic(urlAnthropic, conf.CleAPI, e.Modele, system, demande, timeout)
		if errReseau != nil {
			// Au plus UN nouvel essai en cas d'erreur réseau.
			essais = 2
			res, errReseau = appelAnthropic(urlAnthropic, conf.CleAPI, e.Modele, system, demande, timeout)
		}
	} else {
		res, errReseau = appelOpenAICompatible(base, conf.CleAPI, e.Modele, system, demande, timeout)
		if errReseau != nil {
			essais = 2
			res, errReseau = appelOpenAICompatible(base, conf.CleAPI, e.Modele, system, demande, timeout)
		}
	}
	duree := time.Since(debut).Milliseconds()
	entree := entreeJournalMoteur{
		Instant: time.Now().UTC().Format(time.RFC3339), Mission: p.ID,
		Niveau: niveau, Etape: etape, Moteur: moteur, Modele: e.Modele,
		DureeMs: duree, Essai: essais,
	}
	if errReseau != nil {
		entree.Resultat = "erreur"
		entree.Erreur = errReseau.Error()
		g.consignerMoteur(entree)
		return resultatMoteur{}, &erreurMoteur{http.StatusBadGateway, "ERREUR_MOTEUR",
			"fournisseur " + moteur + " : " + errReseau.Error()}
	}
	entree.Resultat = "ok"
	entree.JetonsEntree = res.JetonsEntree
	entree.JetonsSortie = res.JetonsSortie
	g.consignerMoteur(entree)
	p.AppelsMoteurs++
	p.PlafondAppels = regime.plafond()
	return res, nil
}

// alerteAppels : vrai dès 80 % du plafond consommé (spec §3.3).
func (p *projetDemo) alerteAppels() bool {
	if p.PlafondAppels <= 0 {
		return false
	}
	return p.AppelsMoteurs*100 >= p.PlafondAppels*80
}

// ---- Adaptateur OpenAI-compatible (Mistral, OpenAI) ------------------

func appelOpenAICompatible(base, cle, modele, system, demande string, timeout time.Duration) (resultatMoteur, error) {
	corps := map[string]any{
		"model": modele,
		"messages": []messageMoteur{
			{Role: "system", Contenu: system},
			{Role: "user", Contenu: demande},
		},
		"max_tokens": 1024,
	}
	return postMoteur(base+"/v1/chat/completions", "Bearer "+cle, corps, timeout)
}

// ---- Adaptateur Anthropic Messages (/v1/messages) --------------------

func appelAnthropic(base, cle, modele, system, demande string, timeout time.Duration) (resultatMoteur, error) {
	corps := map[string]any{
		"model":      modele,
		"max_tokens": 1024,
		"system":     system,
		"messages": []messageMoteur{
			{Role: "user", Contenu: demande},
		},
	}
	return postMoteur(strings.TrimSuffix(base, "/")+"/v1/messages", "x-api-key "+cle, corps, timeout)
}

// postMoteur : HTTP commun aux deux formats. auth non vide est soit
// "Bearer <clé>" (OpenAI-compatible) soit "x-api-key <clé>"
// (Anthropic, avec anthropic-version). Réponse plafonnée à 1 Mo. Les
// erreurs transport/dépendance sont renvoyées en erreur (l'appelant
// décide de l'essai unique).
func postMoteur(url, auth string, corps any, timeout time.Duration) (resultatMoteur, error) {
	brut, err := json.Marshal(corps)
	if err != nil {
		return resultatMoteur{}, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(brut))
	if err != nil {
		return resultatMoteur{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	} else if strings.HasPrefix(auth, "x-api-key ") {
		req.Header.Set("x-api-key", strings.TrimPrefix(auth, "x-api-key "))
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	client := &http.Client{Timeout: timeout}
	rep, err := client.Do(req)
	if err != nil {
		return resultatMoteur{}, err
	}
	defer rep.Body.Close()
	lu, err := io.ReadAll(io.LimitReader(rep.Body, 1<<20))
	if err != nil {
		return resultatMoteur{}, err
	}
	if rep.StatusCode != http.StatusOK {
		motif := strings.TrimSpace(string(lu))
		if len(motif) > 200 {
			motif = motif[:200]
		}
		return resultatMoteur{}, fmt.Errorf("HTTP %d : %s", rep.StatusCode, motif)
	}
	var reponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(lu, &reponse); err != nil {
		return resultatMoteur{}, err
	}
	res := resultatMoteur{
		JetonsEntree: reponse.Usage.PromptTokens + reponse.Usage.InputTokens,
		JetonsSortie: reponse.Usage.CompletionTokens + reponse.Usage.OutputTokens,
	}
	if len(reponse.Choices) > 0 {
		res.Texte = reponse.Choices[0].Message.Content
	}
	for _, b := range reponse.Content {
		if b.Type == "text" {
			res.Texte += b.Text
		}
	}
	if strings.TrimSpace(res.Texte) == "" {
		return resultatMoteur{}, fmt.Errorf("réponse du fournisseur sans contenu textuel")
	}
	return res, nil
}

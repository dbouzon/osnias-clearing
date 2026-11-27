// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package maieutique

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/connecteurs"
	"coordengine/internal/jcs"
	"coordengine/internal/missions"
	"coordengine/internal/persist"
)

// ---- enveloppes canoniques (jcs/persist n'acceptent que des valeurs JSON) ----

func canonique(v any) ([]byte, error) { return jcs.Canonical(v) }

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func persistSaveAtomic(enregistrement any, chemin string) error {
	return persist.SaveAtomic(enregistrement, chemin, "")
}

func persistJournal(entrees []any, chemin string) (int, error) {
	return persist.JournalExecute(entrees, chemin)
}

// ---- configuration (plan v2 D3/D4/D9) ----

// Transmission : paramètres de la mission fille créée à la transmission
// (plan v2 D7 : orchestrateur EXISTANT, NouvelleMissionExplicit).
type Transmission struct {
	IssueFille    string              `json:"issue_fille"`
	ExecutantID   string              `json:"executant_id"`
	RecepteurID   string              `json:"recepteur_id"`
	QuaExecID     string              `json:"qua_exec_id"`
	QuaRecepID    string              `json:"qua_recep_id"`
	Roles         map[string][]string `json:"roles"`
	Plafonds      budget.Plafonds     `json:"plafonds"`
	LienIssueMere string              `json:"lien_issue_mere,omitempty"`
}

// Config du magasin maïeutique.
type Config struct {
	// Racine des projets (D6) : state/maieutique/.
	Racine string `json:"racine"`
	// Referentiel missions du moteur (mission fille, lecture/écriture
	// via Journaliser uniquement pour l'ancre de transmission).
	Referentiel *missions.Referentiel `json:"-"`
	// Orchestrateur EXISTANT du moteur (NouvelleMissionExplicit, D7).
	Orchestrateur *connecteurs.Orchestrateur `json:"-"`
	// Adaptateurs : registre des binômes possibles (directeur/exécutant
	// DISTINCTS, identifiants choisissables par le pilote, D4).
	Adaptateurs map[string]adaptateurs.Adaptateur `json:"-"`
	// ContexteJetonsMax : budget de contexte du dialogue (défaut 8000).
	ContexteJetonsMax int `json:"contexte_jetons_max"`
	// TailleMaxDocument : quota par document ET par charge (défaut 32000).
	TailleMaxDocument int `json:"taille_max_document"`
	// TentativesDirecteur : re-tentatives bornées avant ATTENTE_PILOTE (2).
	TentativesDirecteur int `json:"tentatives_directeur"`
	// SondesReseau : sondes d'existence des sources (défaut FAUX en
	// SIMULÉ ; alors statut NON_SONDE consigné, D3).
	SondesReseau bool `json:"sondes_reseau"`
	// TimeoutSondeMS : borne de la sonde HTTP (défaut 10000 ms, D3).
	TimeoutSondeMS int `json:"timeout_sonde_ms"`
	// UserAgent déclaré pour les sondes (journalisé, D3).
	UserAgent string `json:"user_agent"`
	// Transmission : mission fille (D7).
	Transmission Transmission `json:"transmission"`
}

// ConfigParDefaut : valeurs du plan v2 (8000 jetons, 32000 octets,
// 2 tentatives, sondes réseau désactivées en SIMULÉ, sonde bornée 10 s).
func ConfigParDefaut(racine string) Config {
	return Config{
		Racine:              racine,
		ContexteJetonsMax:   8000,
		TailleMaxDocument:   32000,
		TentativesDirecteur: 2,
		SondesReseau:        false,
		TimeoutSondeMS:      10000,
		UserAgent:           "coord-maieutique/1.0 (sonde de qualification, mission M-COORD-MAIEUTIQUE-01)",
	}
}

// ConfigParDefautAppliquee complète les champs vides avec les défauts.
func ConfigParDefautAppliquee(c *Config) error {
	if c.Racine == "" {
		return errors.New("maieutique : racine requise")
	}
	d := ConfigParDefaut(c.Racine)
	if c.ContexteJetonsMax <= 0 {
		c.ContexteJetonsMax = d.ContexteJetonsMax
	}
	if c.TailleMaxDocument <= 0 {
		c.TailleMaxDocument = d.TailleMaxDocument
	}
	if c.TentativesDirecteur <= 0 {
		c.TentativesDirecteur = d.TentativesDirecteur
	}
	if c.TimeoutSondeMS <= 0 {
		c.TimeoutSondeMS = d.TimeoutSondeMS
	}
	if c.UserAgent == "" {
		c.UserAgent = d.UserAgent
	}
	if c.Adaptateurs == nil {
		c.Adaptateurs = map[string]adaptateurs.Adaptateur{}
	}
	return nil
}

// ---- sources (plan v2 D3 : trois contrôles déterministes) ----

// Provenances admises d'une source.
const (
	ProvenanceDeclaree = "DECLAREE_PILOTE"  // (a) sources déclarées par le pilote
	ProvenanceCorpus   = "CORPUS_DOCUMENTS" // (b) corpus documents/ du projet
	ProvenanceURL      = "URL_DIRECTEUR"    // (c) URL du directeur ACCEPTÉE par le pilote
)

// Statuts de sonde d'une source (D3, contrôle d'existence).
const (
	StatutVerifiee     = "VERIFIEE"     // sonde OK (ou chemin lu + empreinte)
	StatutNonSonde     = "NON_SONDE"    // sondes réseau désactivées (SIMULÉ)
	StatutInverifiable = "INVERIFIABLE" // 404, timeout, fichier absent
)

// Source d'un ordre : référence vérifiable avec provenance et statut.
type Source struct {
	Ref        string `json:"ref"`
	Provenance string `json:"provenance"`
	Statut     string `json:"statut"`
	Detail     string `json:"detail,omitempty"`
}

// estURL dit si une référence est une URL publique http(s).
func estURL(ref string) bool {
	u := strings.TrimSpace(ref)
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")
}

// ressembleChemin : une référence sondable comme FICHIER (chemin relatif
// au corpus, chemin absolu, ou nom avec extension). Une référence
// documentaire libre (« issue repo#42 version 1.4 ») n'est PAS un chemin.
func ressembleChemin(ref string) bool {
	if strings.Contains(ref, "\\") {
		return true
	}
	n := strings.TrimSpace(ref)
	if i := strings.IndexByte(n, '/'); i >= 0 {
		// « a/b » ressemble à un chemin SAUF s'il contient des espaces
		// (alors c'est une référence documentaire libre).
		return !strings.ContainsAny(n, " \t")
	}
	// Nom seul : sondable s'il ressemble à un fichier (extension).
	return strings.Contains(filepath.Base(n), ".")
}

// controlerProvenance (D3, contrôle 1) : une source ne peut être citée que
// si elle est déclarée par le pilote, présente au corpus documents/, ou
// URL du directeur acceptée explicitement par le pilote. Toute autre
// source = SOURCE_NON_DECLAREE (pas d'invention).
func (s *Store) controlerProvenance(p *Projet, src Source) (string, error) {
	ref := strings.TrimSpace(src.Ref)
	for _, d := range p.SourcesDeclarees {
		if strings.TrimSpace(d) == ref {
			return ProvenanceDeclaree, nil
		}
	}
	// (b) corpus : un fichier du même nom existe dans documents/.
	if !estURL(ref) && !strings.Contains(ref, "/") {
		chemin := filepath.Join(s.dir(p.ID), "documents", filepath.Base(ref))
		if _, err := os.Stat(chemin); err == nil {
			return ProvenanceCorpus, nil
		}
	}
	// (c) URL du directeur acceptée par le pilote au tour suivant.
	if estURL(ref) && p.UrlsAcceptees != nil && p.UrlsAcceptees[ref] {
		return ProvenanceURL, nil
	}
	return "", erreur(CodeSourceNonDeclaree,
		"source non déclarée, hors corpus et non acceptée par le pilote : "+ref)
}

// sonderURL (D3, contrôle 2) : requête GET bornée, statut et date consignés
// dans le journal. Résultat = statut + détail.
func (s *Store) sonderURL(ctx context.Context, p *Projet, url string) (string, string) {
	if !s.cfg.SondesReseau {
		return StatutNonSonde, "sondes réseau désactivées (SIMULÉ, D3)"
	}
	ctxSonde, annuler := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSondeMS)*time.Millisecond)
	defer annuler()
	req, err := http.NewRequestWithContext(ctxSonde, http.MethodGet, url, nil)
	if err != nil {
		return StatutInverifiable, "requête impossible : " + err.Error()
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		return StatutInverifiable, "sonde échouée : " + err.Error()
	}
	defer rep.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(rep.Body, 4096))
	detail := fmt.Sprintf("HTTP %d le %s", rep.StatusCode, maintenantUTC())
	if rep.StatusCode >= 200 && rep.StatusCode < 300 {
		return StatutVerifiee, detail
	}
	return StatutInverifiable, detail
}

// sonderChemin (D3, contrôle 2) : existence + lisibilité + empreinte sha256
// du fichier consignée. Toujours actif (aucun réseau).
func (s *Store) sonderChemin(p *Projet, chemin string) (string, string) {
	if !filepath.IsAbs(chemin) && !strings.HasPrefix(chemin, "documents/") {
		// référence relative au corpus du projet
		chemin = filepath.Join(s.dir(p.ID), "documents", chemin)
	} else if strings.HasPrefix(chemin, "documents/") {
		chemin = filepath.Join(s.dir(p.ID), chemin)
	}
	b, err := os.ReadFile(chemin)
	if err != nil {
		return StatutInverifiable, "fichier absent ou illisible : " + chemin
	}
	return StatutVerifiee, fmt.Sprintf("fichier lu (%d octets, sha256 %s) le %s",
		len(b), sha256Hex(b), maintenantUTC())
}

// VerifieSources applique les TROIS contrôles D3 aux sources de l'ordre du
// projet : provenance, existence (sonde), cohérence des citations. Les
// résultats de sonde sont journalisés. Retourne la liste des codes
// d'échec (vide = tout est en ordre).
func (s *Store) VerifieSources(ctx context.Context, p *Projet) []string {
	if p.Ordre == nil {
		return []string{CodeOrdreAbsent}
	}
	codes := []string{}
	if len(p.Ordre.Sources) == 0 {
		return []string{CodeSourcesVides}
	}
	verifiees := map[string]string{} // ref → statut
	for i := range p.Ordre.Sources {
		src := &p.Ordre.Sources[i]
		prov, err := s.controlerProvenance(p, *src)
		if err != nil {
			src.Provenance = ""
			src.Statut = StatutInverifiable
			src.Detail = "provenance refusée"
			codes = append(codes, CodeSourceNonDeclaree)
			continue
		}
		src.Provenance = prov
		ref := strings.TrimSpace(src.Ref)
		if estURL(ref) {
			statut, detail := s.sonderURL(ctx, p, ref)
			src.Statut = statut
			src.Detail = detail
		} else if ressembleChemin(ref) {
			statut, detail := s.sonderChemin(p, ref)
			src.Statut = statut
			src.Detail = detail
		} else {
			// Référence documentaire déclarée (ex. « issue
			// dbouzon/osnias-clearing#42 version 1.4 ») : AUCUNE sonde
			// mécanique n'est définie pour ce type — le statut honnête
			// est NON_SONDE, jamais VERIFIEE (pas d'invention d'existence).
			src.Statut = StatutNonSonde
			src.Detail = "référence documentaire : aucune sonde mécanique applicable (statut NON_SONDE consigné)"
		}
		verifiees[strings.ToLower(ref)] = src.Statut
		if src.Statut == StatutInverifiable {
			codes = append(codes, CodeSourcesNonVerifiees)
		}
		_ = s.journaliser(p.ID, map[string]any{
			"type":        "SONDE_SOURCE",
			"source":      ref,
			"provenance":  src.Provenance,
			"statut":      src.Statut,
			"detail":      src.Detail,
			"instant_utc": maintenantUTC(),
		})
	}
	// Contrôle 3 : cohérence des citations de l'état des lieux et des étapes.
	for _, citation := range extraireURLs(p.Ordre.EtatDesLieux) {
		if _, ok := verifiees[strings.ToLower(strings.TrimSpace(citation))]; !ok {
			codes = append(codes, CodeSourceNonDeclaree)
		}
	}
	for _, et := range p.Ordre.Etapes {
		for _, citation := range extraireURLs(et.Nom + " " + et.ControleIntermediaire) {
			if _, ok := verifiees[strings.ToLower(strings.TrimSpace(citation))]; !ok {
				codes = append(codes, CodeSourceNonDeclaree)
			}
		}
	}
	// Déduplication stable des codes.
	vus := map[string]bool{}
	uniques := []string{}
	for _, c := range codes {
		if !vus[c] {
			vus[c] = true
			uniques = append(uniques, c)
		}
	}
	return uniques
}

// extraireURLs isole les URL http(s) citées dans un texte (contrôle 3).
func extraireURLs(texte string) []string {
	var out []string
	motif := strings.Fields(texte)
	for _, m := range motif {
		m = strings.Trim(m, "(),.;:[]<>\"'")
		if estURL(m) {
			out = append(out, m)
		}
	}
	return out
}

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Adaptateur Drive (plan v2 §1.2) : l'interface Dossiers isole le front
// du stockage documentaire. Deux implémentations : le dossier FACTICE
// local de recette, et l'adaptateur RCLONE (rclone.go, mission
// M-COORD-DRIVE-01). Le moteur n'est jamais touché.
//
// Routes (session obligatoire) :
//
//	GET  /api/drive/?dossier=…[&nom=…]   lister / lire
//	POST /api/drive/document            écrire (encadré, plan v2 §1.4)
//	GET  /api/drive/export/{id}         état de l'export du dossier de sortie
//	POST /api/drive/export/{id}         relancer l'export
//
// Export « option A » (plan v2 §1.3) : après une transmission réussie,
// si le compte a un Drive et export_sortie, le gateway copie ordre,
// devis et journal dans le dossier de sortie DÉCLARÉ DANS LE CADRAGE
// du projet. NON BLOQUANT : une erreur d'export n'annule jamais la
// transmission ; l'état est conservé (bandeau front + relance).
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Dossiers : contrat de stockage documentaire du parcours client.
type Dossiers interface {
	Lister(dossier string) ([]string, error)
	Lire(dossier, nom string) ([]byte, error)
	Ecrire(dossier, nom string, contenu []byte) error
}

// dossierFactice : implémentation de recette. Racine fixée par la
// configuration du compte ; tout chemin qui tente d'en sortir est
// refusé (fail closed).
type dossierFactice struct {
	racine string
}

func nouveauDossierFactice(racine string) Dossiers {
	return &dossierFactice{racine: racine}
}

func (d *dossierFactice) resoudre(dossier, nom string) (string, error) {
	base, err := filepath.Abs(d.racine)
	if err != nil {
		return "", err
	}
	cible := filepath.Clean(filepath.Join(base, dossier, nom))
	if !strings.HasPrefix(cible, base+string(os.PathSeparator)) && cible != base {
		return "", os.ErrPermission
	}
	return cible, nil
}

func (d *dossierFactice) Lister(dossier string) ([]string, error) {
	dir, err := d.resoudre(dossier, "")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	noms := []string{}
	for _, e := range entries {
		noms = append(noms, e.Name())
	}
	sort.Strings(noms)
	return noms, nil
}

func (d *dossierFactice) Lire(dossier, nom string) ([]byte, error) {
	chemin, err := d.resoudre(dossier, nom)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(chemin)
}

func (d *dossierFactice) Ecrire(dossier, nom string, contenu []byte) error {
	chemin, err := d.resoudre(dossier, nom)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		return err
	}
	return os.WriteFile(chemin, contenu, 0o644)
}

// --------------------------------------------------------------- export

// etatExport : état de l'export du dossier de sortie d'un projet.
type etatExport struct {
	Etat       string   `json:"etat"` // "ok" | "echec"
	Motif      string   `json:"motif,omitempty"`
	Dossier    string   `json:"dossier,omitempty"`
	Fichiers   []string `json:"fichiers,omitempty"`
	InstantUTC string   `json:"instant_utc"`
}

// exportActif : l'option A ne s'applique qu'aux comptes qui l'ont
// demandés ET qui ont un Drive (factice ou rclone).
func (g *gateway) exportActif(c compteConf) bool {
	return c.ExportSortie && g.drives[c.Nom] != nil
}

// donneesExport : ce que le gateway copie dans le dossier de sortie.
type donneesExport struct {
	Cadrage   map[string]any
	Ordre     map[string]any
	Version   int
	Empreinte string
	Devis     map[string]any
	Journal   []byte
}

// lireDonneesExport : mode démo → accès direct au moteur factice
// (lectures sous mutex) ; mode réel → GET loopback vers l'instance du
// compte avec le token pilote injecté, comme le proxy.
func (g *gateway) lireDonneesExport(c compteConf, id string) (donneesExport, error) {
	if c.Demo {
		demo := g.demos[c.Nom]
		if demo == nil {
			return donneesExport{}, &erreurDrive{code: "DRIVE_ERREUR", motif: "moteur démo absent"}
		}
		return demo.donneesExport(id)
	}
	var donnees donneesExport
	client := &http.Client{Timeout: c.timeoutDrive()}
	projeter := func(chemin string, cible any) error {
		req, err := http.NewRequest(http.MethodGet,
			strings.TrimRight(c.URL, "/")+chemin, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Coord-Pilot-Token", c.Token) // jamais journalisé
		rep, err := client.Do(req)
		if err != nil {
			return err
		}
		defer rep.Body.Close()
		if rep.StatusCode != http.StatusOK {
			return &erreurDrive{code: "DRIVE_ERREUR",
				motif: "instance du compte : statut " + rep.Status + " pour " + chemin}
		}
		return json.NewDecoder(rep.Body).Decode(cible)
	}
	var projet map[string]any
	if err := projeter("/projets/"+id, &projet); err != nil {
		return donnees, err
	}
	var ordreRep struct {
		Ordre     map[string]any `json:"ordre"`
		Version   int            `json:"version"`
		Empreinte string         `json:"empreinte"`
	}
	if err := projeter("/projets/"+id+"/ordre", &ordreRep); err != nil {
		return donnees, err
	}
	rep, err := client.Get(strings.TrimRight(c.URL, "/") + "/projets/" + id + "/journal")
	if err != nil {
		return donnees, err
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		return donnees, &erreurDrive{code: "DRIVE_ERREUR",
			motif: "instance du compte : statut " + rep.Status + " pour le journal"}
	}
	journal, err := io.ReadAll(rep.Body)
	if err != nil {
		return donnees, err
	}
	donnees.Cadrage, _ = projet["cadrage"].(map[string]any)
	donnees.Devis, _ = projet["devis"].(map[string]any)
	donnees.Ordre = ordreRep.Ordre
	donnees.Version = ordreRep.Version
	donnees.Empreinte = ordreRep.Empreinte
	donnees.Journal = bytes.TrimSpace(journal)
	return donnees, nil
}

// exporterProjet : copie ordre.json, devis.json et journal.ndjson dans
// le dossier de sortie du cadrage. Non bloquant par construction :
// appelé APRÈS une transmission réussie, il ne peut jamais annuler ni
// retarder l'état TRANSMIS côté moteur.
func (g *gateway) exporterProjet(c compteConf, id string) etatExport {
	exp := etatExport{Etat: "echec", InstantUTC: time.Now().UTC().Format(time.RFC3339)}
	donnees, err := g.lireDonneesExport(c, id)
	if err != nil {
		exp.Motif = "lecture projet : " + err.Error()
	} else {
		sortie := ""
		if donnees.Cadrage != nil {
			sortie, _ = donnees.Cadrage["dossier_sortie"].(string)
		}
		sortie = strings.TrimSpace(sortie)
		if sortie == "" {
			exp.Motif = "aucun dossier de sortie déclaré dans le cadrage"
		} else if err := validerDossier(sortie); err != nil {
			exp.Motif = "dossier de sortie interdit : " + err.Error()
		} else {
			exp.Dossier = sortie
			if donnees.Ordre != nil {
				exp.Motif = "ordre absent du projet"
				if b, err := json.Marshal(map[string]any{
					"ordre": donnees.Ordre, "version": donnees.Version,
					"empreinte": donnees.Empreinte,
				}); err == nil {
					exp.Motif = ""
					exp = g.ecrireFichiersExport(c, id, exp, sortie, []fichierExport{
						{"ordre.json", b},
						{"devis.json", marshalerExport(map[string]any{"devis": donnees.Devis})},
						{"journal.ndjson", append(bytes.TrimSpace(donnees.Journal), '\n')},
					})
				} else {
					exp.Motif = "sérialisation ordre : " + err.Error()
				}
			}
		}
	}
	g.mu.Lock()
	g.exports[c.Nom+"/"+id] = exp
	g.mu.Unlock()
	// Journal côté gateway (jamais dans le moteur) : motif complet,
	// aucun secret (la ligne de commande rclone ne porte que des
	// chemins validés).
	log.Printf("coord-front : export sortie %s/%s : %s%s", c.Nom, id, exp.Etat,
		func() string {
			if exp.Motif == "" {
				return ""
			}
			return " (" + exp.Motif + ")"
		}())
	return exp
}

type fichierExport struct {
	nom     string
	contenu []byte
}

func (g *gateway) ecrireFichiersExport(c compteConf, id string, exp etatExport,
	sortie string, fichiers []fichierExport) etatExport {
	drive := g.drives[c.Nom]
	for _, f := range fichiers {
		if err := drive.Ecrire(sortie, f.nom, f.contenu); err != nil {
			exp.Motif = "écriture " + f.nom + " : " + err.Error()
			return exp
		}
		exp.Fichiers = append(exp.Fichiers, f.nom)
	}
	exp.Etat = "ok"
	exp.Motif = ""
	return exp
}

func marshalerExport(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// capteurReponse : intercepte la réponse du moteur (démo ou proxy) pour
// la transmission, afin d'ajouter export_sortie SANS modifier le moteur.
type capteurReponse struct {
	statut int
	entete http.Header
	corps  bytes.Buffer
}

func (c *capteurReponse) Header() http.Header {
	if c.entete == nil {
		c.entete = http.Header{}
	}
	return c.entete
}
func (c *capteurReponse) WriteHeader(s int) { c.statut = s }
func (c *capteurReponse) Write(b []byte) (int, error) {
	if c.statut == 0 {
		c.statut = http.StatusOK
	}
	return c.corps.Write(b)
}

// servirTransmission : transmission + export non bloquant (option A).
func (g *gateway) servirTransmission(w http.ResponseWriter, r *http.Request, c compteConf) {
	cap := &capteurReponse{}
	if c.Demo {
		g.demos[c.Nom].servir(cap, r)
	} else {
		g.proxifier(cap, r, c)
	}
	corps := cap.corps.Bytes()
	if cap.statut != http.StatusOK {
		for k := range cap.entete {
			w.Header().Set(k, cap.entete.Get(k))
		}
		w.WriteHeader(cap.statut)
		_, _ = w.Write(corps)
		return
	}
	// Chemin /api/projets/{id}/transmettre : l'identifiant est
	// l'avant-dernier segment, jamais l'action.
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id := ""
	if len(segments) >= 2 {
		id = segments[len(segments)-2]
	}
	exp := g.exporterProjet(c, id)
	var m map[string]any
	if err := json.Unmarshal(corps, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["export_sortie"] = exp
	repondreJSON(w, http.StatusOK, m)
}

// ---------------------------------------------------------------- routes

func (g *gateway) apiDrive(w http.ResponseWriter, r *http.Request) {
	c, ok := g.compteSession(w, r)
	if !ok {
		return
	}
	reste := strings.TrimPrefix(r.URL.Path, "/api/drive")
	switch {
	case r.Method == http.MethodGet && (reste == "" || reste == "/"):
		g.listerDocumentsDrive(w, r, c)
	case r.Method == http.MethodPost && reste == "/document":
		g.deposerDocumentDrive(w, r, c)
	case reste == "/export" || strings.HasPrefix(reste, "/export/"):
		g.apiExport(w, r, c, strings.TrimPrefix(reste, "/export/"))
	default:
		repondreErreur(w, http.StatusNotFound, "ROUTE_INCONNUE", r.URL.Path)
	}
}

func (g *gateway) driveCompte(w http.ResponseWriter, c compteConf) (Dossiers, bool) {
	drive := g.drives[c.Nom]
	if drive == nil {
		repondreErreur(w, http.StatusNotFound, "DRIVE_ABSENT",
			"aucun dossier Drive configuré pour ce compte")
		return nil, false
	}
	return drive, true
}

// listerDocumentsDrive : lecture seule pour le front (lister, lire).
func (g *gateway) listerDocumentsDrive(w http.ResponseWriter, r *http.Request, c compteConf) {
	drive, ok := g.driveCompte(w, c)
	if !ok {
		return
	}
	dossier := r.URL.Query().Get("dossier")
	nom := r.URL.Query().Get("nom")
	switch {
	case nom == "":
		if err := validerDossier(dossier); err != nil {
			statut, code, motif := statutErreurDrive(err)
			repondreErreur(w, statut, code, motif)
			return
		}
		noms, err := drive.Lister(dossier)
		if err != nil {
			statut, code, motif := statutErreurDrive(err)
			repondreErreur(w, statut, code, motif)
			return
		}
		repondreJSON(w, http.StatusOK, map[string]any{"documents": noms})
	default:
		if err := validerDossier(dossier); err != nil {
			statut, code, motif := statutErreurDrive(err)
			repondreErreur(w, statut, code, motif)
			return
		}
		if err := validerNom(nom); err != nil {
			statut, code, motif := statutErreurDrive(err)
			repondreErreur(w, statut, code, motif)
			return
		}
		contenu, err := drive.Lire(dossier, nom)
		if err != nil {
			statut, code, motif := statutErreurDrive(err)
			repondreErreur(w, statut, code, motif)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(contenu)
	}
}

const tailleMaxDocument = 1 << 20 // 1 Mo (plan v2 §1.4)

// deposerDocumentDrive : SEUL point d'écriture du parcours (plan v2
// §1.4). Encadré : session obligatoire, opt-in par compte
// (autoriser_ecriture_drive), fail-closed strict, 1 Mo maximum.
func (g *gateway) deposerDocumentDrive(w http.ResponseWriter, r *http.Request, c compteConf) {
	drive, ok := g.driveCompte(w, c)
	if !ok {
		return
	}
	if !c.AutoriserEcritureDrive {
		repondreErreur(w, http.StatusForbidden, "DRIVE_REFUS",
			"écriture Drive désactivée pour ce compte (autoriser_ecriture_drive)")
		return
	}
	var req struct {
		Dossier string `json:"dossier"`
		Nom     string `json:"nom"`
		Contenu string `json:"contenu"`
	}
	if err := lireJSON(r, &req); err != nil {
		repondreErreur(w, http.StatusBadRequest, "CORPS_INVALIDE", err.Error())
		return
	}
	req.Dossier = strings.TrimSpace(req.Dossier)
	if err := validerDossier(req.Dossier); err != nil {
		statut, code, motif := statutErreurDrive(err)
		repondreErreur(w, statut, code, motif)
		return
	}
	if err := validerNom(req.Nom); err != nil {
		statut, code, motif := statutErreurDrive(err)
		repondreErreur(w, statut, code, motif)
		return
	}
	if len(req.Contenu) > tailleMaxDocument {
		repondreErreur(w, http.StatusBadRequest, "DOCUMENT_TROP_VOLUMINEUX",
			"contenu limité à 1 Mo")
		return
	}
	if err := drive.Ecrire(req.Dossier, req.Nom, []byte(req.Contenu)); err != nil {
		statut, code, motif := statutErreurDrive(err)
		repondreErreur(w, statut, code, motif)
		return
	}
	repondreJSON(w, http.StatusOK, map[string]any{
		"ecrit": true, "dossier": req.Dossier, "nom": req.Nom,
	})
}

// apiExport : GET = dernier état connu ; POST = relance manuelle
// (l'échec d'export n'annule jamais la transmission déjà faite).
func (g *gateway) apiExport(w http.ResponseWriter, r *http.Request, c compteConf, id string) {
	if id == "" {
		repondreErreur(w, http.StatusNotFound, "ROUTE_INCONNUE", r.URL.Path)
		return
	}
	cle := c.Nom + "/" + id
	switch r.Method {
	case http.MethodGet:
		g.mu.RLock()
		exp, ok := g.exports[cle]
		g.mu.RUnlock()
		if !ok {
			repondreErreur(w, http.StatusNotFound, "EXPORT_INCONNU",
				"aucun export connu pour ce projet")
			return
		}
		repondreJSON(w, http.StatusOK, map[string]any{"export_sortie": exp})
	case http.MethodPost:
		if !g.exportActif(c) {
			repondreErreur(w, http.StatusForbidden, "DRIVE_REFUS",
				"export désactivé pour ce compte (export_sortie)")
			return
		}
		if _, ok := g.driveCompte(w, c); !ok {
			return
		}
		exp := g.exporterProjet(c, id)
		repondreJSON(w, http.StatusOK, map[string]any{"export_sortie": exp})
	default:
		repondreErreur(w, http.StatusMethodNotAllowed, "METHODE_REFUSEE", r.Method)
	}
}

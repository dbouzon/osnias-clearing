// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Adaptateur Drive (plan v2 §1.2) : l'interface Dossiers isole le front
// du stockage documentaire. En recette, l'implémentation FACTICE écrit
// dans un dossier local racine par compte ; quand le pilote fournira
// les accès Google, une seconde implémentation (API Drive) remplacera
// la factice SANS toucher au front ni au moteur.
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// apiDrive : lecture seule pour le front (lister, lire). L'écriture
// reste serveur-seule pour l'instant : aucun besoin du parcours T2.
func (g *gateway) apiDrive(w http.ResponseWriter, r *http.Request) {
	c, ok := g.compteSession(w, r)
	if !ok {
		return
	}
	drive := g.drives[c.Nom]
	if drive == nil {
		repondreErreur(w, http.StatusNotFound, "DRIVE_ABSENT",
			"aucun dossier Drive configuré pour ce compte")
		return
	}
	dossier := r.URL.Query().Get("dossier")
	nom := r.URL.Query().Get("nom")
	switch {
	case r.Method == http.MethodGet && nom == "":
		noms, err := drive.Lister(dossier)
		if err != nil {
			repondreErreur(w, http.StatusForbidden, "DRIVE_REFUS", err.Error())
			return
		}
		repondreJSON(w, http.StatusOK, map[string]any{"documents": noms})
	case r.Method == http.MethodGet && nom != "":
		contenu, err := drive.Lire(dossier, nom)
		if err != nil {
			repondreErreur(w, http.StatusNotFound, "DRIVE_ABSENT", err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(contenu)
	default:
		repondreErreur(w, http.StatusMethodNotAllowed, "METHODE_REFUSEE", r.Method)
	}
}

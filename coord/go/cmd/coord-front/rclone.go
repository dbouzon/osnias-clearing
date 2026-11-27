// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Adaptateur Drive via rclone (mission M-COORD-DRIVE-01, plan v2 §1.1).
// Implémente l'interface Dossiers en appelant le binaire rclone par
// os/exec, SANS shell (arguments en tableaux, aucune interpolation de
// chaîne) et avec un timeout par appel. Décision du pilote : plugin sur
// étagère plutôt qu'un développement OAuth/Drive ad hoc ; le MCP Drive
// officiel Google reste la cible de migration future.
//
// Fail closed STRICT : le nom du remote (configuration serveur) doit
// matcher [A-Za-z0-9_-]+ ; tout dossier/nom contenant ':', '..' ou un
// caractère de contrôle est refusé AVANT tout appel. Aucune option
// rclone ne provient des entrées. Le binaire et son rclone.conf restent
// HORS du dépôt (chemin PATH, config chmod 600).
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// erreurDrive : échec portant un code stable, mappé sur un statut HTTP
// par statutErreurDrive.
type erreurDrive struct {
	code  string
	motif string
}

func (e *erreurDrive) Error() string { return e.code + " : " + e.motif }

func refuserDrive(motif string) error {
	return &erreurDrive{code: "DRIVE_REFUS", motif: motif}
}

var motifRemoteRclone = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validerNomRemote : le remote vient de la configuration du compte,
// jamais d'une entrée ; il doit rester un simple nom de section
// rclone.conf (impossible d'injecter ':' ni un chemin).
func validerNomRemote(remote string) error {
	if !motifRemoteRclone.MatchString(remote) {
		return refuserDrive("nom de remote rclone invalide : " + strconv.Quote(remote))
	}
	return nil
}

// validerDossier : segments non vides, pas de remontée, pas de ':' ni
// de caractère de contrôle. Le dossier VIDE est licite (racine).
func validerDossier(dossier string) error {
	if dossier == "" {
		return nil
	}
	if strings.ContainsRune(dossier, ':') {
		return refuserDrive("caractère ':' interdit dans un dossier : " + strconv.Quote(dossier))
	}
	for _, segment := range strings.Split(dossier, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return refuserDrive("segment de dossier interdit : " + strconv.Quote(dossier))
		}
		for _, r := range segment {
			if r < 0x20 || r == 0x7f {
				return refuserDrive("caractère de contrôle interdit dans un dossier")
			}
		}
	}
	return nil
}

// validerNom : un nom de document est UN segment, jamais un chemin.
func validerNom(nom string) error {
	if strings.TrimSpace(nom) == "" {
		return refuserDrive("nom de document vide")
	}
	if strings.ContainsAny(nom, "/\\:") {
		return refuserDrive("séparateur ou ':' interdit dans un nom : " + strconv.Quote(nom))
	}
	if strings.Contains(nom, "..") || strings.TrimSpace(nom) == "." {
		return refuserDrive("nom de document interdit : " + strconv.Quote(nom))
	}
	for _, r := range nom {
		if r < 0x20 || r == 0x7f {
			return refuserDrive("caractère de contrôle interdit dans un nom")
		}
	}
	return nil
}

// dossierRclone : Dossiers via rclone. racine est le dossier Drive du
// compte DANS le remote (dossier_drive de la configuration) : toute
// opération est préfixée par lui, impossible d'échapper au périmètre
// du compte. binaire est surchargé par les tests (faux binaire).
type dossierRclone struct {
	remote  string
	racine  string
	timeout time.Duration
	binaire string
}

func nouveauDossierRclone(remote, racine string, timeout time.Duration) *dossierRclone {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	binaire, err := exec.LookPath("rclone")
	if err != nil {
		binaire = "" // détecté à chaque appel : DRIVE_INDISPONIBLE propre
	}
	return &dossierRclone{remote: remote, racine: racine, timeout: timeout, binaire: binaire}
}

// cible : remote:racine/dossier[/nom], après validation fail-closed.
func (d *dossierRclone) cible(dossier, nom string) (string, error) {
	if err := validerNomRemote(d.remote); err != nil {
		return "", err
	}
	if err := validerDossier(dossier); err != nil {
		return "", err
	}
	if nom != "" {
		if err := validerNom(nom); err != nil {
			return "", err
		}
	}
	chemin := path.Join(d.racine, dossier, nom)
	if chemin == "/" || chemin == "" {
		return d.remote + ":", nil
	}
	return d.remote + ":" + strings.TrimPrefix(chemin, "/"), nil
}

// appeler : un appel rclone, sans shell, avec timeout par appel. Le
// binaire absent (garde LookPath déterministe sur tout OS : l'erreur
// exec de Windows ne se mappe pas toujours à os.ErrNotExist) et le
// timeout donnent DRIVE_INDISPONIBLE ; une sortie non nulle de rclone
// donne DRIVE_ERREUR (motif tronqué, sans secret : la ligne de
// commande ne contient que des chemins validés).
func (d *dossierRclone) appeler(args []string, entree []byte) ([]byte, error) {
	binaire := d.binaire
	if binaire == "" {
		// Résolution PATH à chaque appel : un rclone installé en cours
		// de route est pris en compte sans redémarrage du gateway.
		binaire = "rclone"
	}
	if _, err := exec.LookPath(binaire); err != nil {
		return nil, &erreurDrive{code: "DRIVE_INDISPONIBLE",
			motif: "binaire rclone introuvable : " + strconv.Quote(binaire)}
	}
	ctx, annuler := context.WithTimeout(context.Background(), d.timeout)
	defer annuler()
	cmd := exec.CommandContext(ctx, binaire, args...)
	// Retour prompt au timeout : un petit-fils du binaire peut garder
	// le tube ouvert après le SIGKILL ; WaitDelay clôt l'appel.
	cmd.WaitDelay = 250 * time.Millisecond
	sortie := &bytes.Buffer{}
	erreurs := &bytes.Buffer{}
	cmd.Stdout = sortie
	cmd.Stderr = erreurs
	if entree != nil {
		cmd.Stdin = bytes.NewReader(entree)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &erreurDrive{code: "DRIVE_INDISPONIBLE",
				motif: "timeout rclone après " + d.timeout.String()}
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, &erreurDrive{code: "DRIVE_INDISPONIBLE",
				motif: "binaire rclone introuvable"}
		}
		motif := strings.TrimSpace(erreurs.String())
		if motif == "" {
			motif = err.Error()
		}
		if len(motif) > 300 {
			motif = motif[:300]
		}
		return nil, &erreurDrive{code: "DRIVE_ERREUR", motif: motif}
	}
	return sortie.Bytes(), nil
}

// absent rclone : motif d'un chemin inexistant (lsf/cat sur un dossier
// ou fichier absent) ; parité avec le comportement de l'adaptateur
// factice (liste vide pour un dossier absent).
func absentRclone(sortie []byte, err error) bool {
	var ed *erreurDrive
	if !errors.As(err, &ed) || ed.code != "DRIVE_ERREUR" {
		return false
	}
	texte := string(sortie) + ed.motif
	return strings.Contains(texte, "directory not found") ||
		strings.Contains(texte, "file not found") ||
		strings.Contains(texte, "doesn't exist") ||
		strings.Contains(texte, "does not exist")
}

func (d *dossierRclone) Lister(dossier string) ([]string, error) {
	cible, err := d.cible(dossier, "")
	if err != nil {
		return nil, err
	}
	sortie, err := d.appeler([]string{"lsf", cible}, nil)
	if err != nil {
		if absentRclone(sortie, err) {
			return []string{}, nil
		}
		return nil, err
	}
	noms := []string{}
	for _, ligne := range strings.Split(strings.TrimRight(string(sortie), "\n"), "\n") {
		if ligne != "" {
			noms = append(noms, ligne)
		}
	}
	return noms, nil
}

func (d *dossierRclone) Lire(dossier, nom string) ([]byte, error) {
	cible, err := d.cible(dossier, nom)
	if err != nil {
		return nil, err
	}
	sortie, err := d.appeler([]string{"cat", cible}, nil)
	if err != nil {
		if absentRclone(sortie, err) {
			return nil, &erreurDrive{code: "DRIVE_ABSENT", motif: "document introuvable : " + nom}
		}
		return nil, err
	}
	return sortie, nil
}

func (d *dossierRclone) Ecrire(dossier, nom string, contenu []byte) error {
	if err := validerDossier(dossier); err != nil {
		return err
	}
	if err := validerNom(nom); err != nil {
		return err
	}
	racineDossier, err := d.cible(dossier, "")
	if err != nil {
		return err
	}
	cibleFichier, err := d.cible(dossier, nom)
	if err != nil {
		return err
	}
	// mkdir puis rcat : le dossier de destination doit exister pour
	// certaines backends ; mkdir est idempotent.
	if _, err := d.appeler([]string{"mkdir", racineDossier}, nil); err != nil {
		return err
	}
	if _, err := d.appeler([]string{"rcat", cibleFichier}, contenu); err != nil {
		return err
	}
	return nil
}

// statutErreurDrive : mapping code Dossiers → statut HTTP.
func statutErreurDrive(err error) (int, string, string) {
	var ed *erreurDrive
	if errors.As(err, &ed) {
		switch ed.code {
		case "DRIVE_REFUS":
			return 400, ed.code, ed.motif
		case "DRIVE_ABSENT":
			return 404, ed.code, ed.motif
		case "DRIVE_INDISPONIBLE":
			return 503, ed.code, ed.motif
		case "DRIVE_ERREUR":
			return 502, ed.code, ed.motif
		}
		return 500, ed.code, ed.motif
	}
	if errors.Is(err, os.ErrPermission) {
		return 403, "DRIVE_REFUS", err.Error()
	}
	return 500, "INTERNE", err.Error()
}

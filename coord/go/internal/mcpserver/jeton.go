// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// ErreurJeton signale un refus de démarrage lié au fichier de jeton de
// lecture. Le message ne contient ni la valeur du jeton, ni le chemin
// complet (seul le nom de fichier est cité pour le diagnostic local).
type ErreurJeton struct{ Motif string }

func (e *ErreurJeton) Error() string { return "jeton de lecture refuse : " + e.Motif }

func erreurJeton(nomFichier, motif string) error {
	return &ErreurJeton{Motif: motif + " [" + filepath.Base(nomFichier) + "]"}
}

// ChargerJeton lit le fichier de jeton de lecture du pilote. Contrôles
// fail-closed AVANT la lecture : le fichier doit exister, être un fichier
// régulier (un lien symbolique n'est jamais suivi), puis posséder la
// protection effective attendue sur le système courant (voir
// verifierProtectionJeton) : POSIX — permissions strictement 0600 et
// propriétaire = utilisateur courant ; Windows — DACL natif restreint à
// l'utilisateur courant, sans héritage. Aucune valeur de jeton n'apparaît
// jamais dans les erreurs.
func ChargerJeton(chemin string) (string, error) {
	if strings.TrimSpace(chemin) == "" {
		return "", &ErreurJeton{Motif: "chemin du fichier de jeton requis"}
	}
	info, err := os.Lstat(chemin)
	if err != nil {
		return "", erreurJeton(chemin, "fichier de jeton introuvable ou illisible")
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return "", erreurJeton(chemin, "le chemin ne designe pas un fichier regulier (lien symbolique refuse)")
	}
	if err := verifierProtectionJeton(chemin, info); err != nil {
		return "", err
	}
	brut, err := os.ReadFile(chemin)
	if err != nil {
		return "", erreurJeton(chemin, "lecture du fichier de jeton impossible")
	}
	jeton := strings.TrimSpace(string(brut))
	if len(jeton) == 0 || len(jeton) > 256 || bytes.ContainsRune(brut, '\x00') {
		return "", erreurJeton(chemin, "contenu du fichier de jeton invalide")
	}
	return jeton, nil
}

// EcrireJeton écrit de façon sécurisée un fichier de jeton de lecture
// (SEC-MCP-01/02) : création EXCLUSIVE d'un fichier temporaire dans le
// même répertoire (jamais de collision, jamais de suivi de lien
// symbolique), protection effective appliquée AVANT le renommage atomique
// (POSIX : chmod 0600 ; Windows : DACL natif restreint à l'utilisateur
// courant, sans héritage), puis remplacement atomique de la cible
// (rotation). Une cible existante non régulière (lien symbolique,
// répertoire) est refusée. Aucune valeur de jeton n'apparaît dans les
// erreurs.
func EcrireJeton(chemin, jeton string) error {
	if strings.TrimSpace(chemin) == "" {
		return &ErreurJeton{Motif: "chemin du fichier de jeton requis"}
	}
	valeur := strings.TrimSpace(jeton)
	if len(valeur) == 0 || len(valeur) > 256 || bytes.ContainsRune([]byte(jeton), '\x00') {
		return &ErreurJeton{Motif: "valeur de jeton invalide"}
	}
	if info, err := os.Lstat(chemin); err == nil && !info.Mode().IsRegular() {
		return erreurJeton(chemin, "cible existante non reguliere (lien symbolique ou repertoire) : remplacement refuse")
	}
	f, err := os.CreateTemp(filepath.Dir(chemin), filepath.Base(chemin)+".tmp-*")
	if err != nil {
		return erreurJeton(chemin, "creation exclusive du fichier temporaire impossible")
	}
	tmp := f.Name()
	passe := false
	defer func() {
		if !passe {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(valeur + "\n"); err != nil {
		return erreurJeton(chemin, "ecriture du fichier temporaire impossible")
	}
	if err := f.Sync(); err != nil {
		return erreurJeton(chemin, "synchronisation du fichier temporaire impossible")
	}
	if err := f.Close(); err != nil {
		return erreurJeton(chemin, "fermeture du fichier temporaire impossible")
	}
	if err := protegerFichierJeton(tmp); err != nil {
		return err
	}
	if err := remplacerFichierJeton(tmp, chemin); err != nil {
		return erreurJeton(chemin, "remplacement atomique du fichier de jeton impossible")
	}
	passe = true
	return nil
}

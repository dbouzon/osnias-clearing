// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
// régulier, appartenir à l'utilisateur courant (hors Windows, où la notion
// d'utilisateur POSIX n'existe pas) et avoir des permissions strictement
// 0600 sur les systèmes qui les appliquent. Aucune valeur de jeton
// n'apparaît jamais dans les erreurs.
func ChargerJeton(chemin string) (string, error) {
	if strings.TrimSpace(chemin) == "" {
		return "", &ErreurJeton{Motif: "chemin du fichier de jeton requis"}
	}
	info, err := os.Stat(chemin)
	if err != nil {
		return "", erreurJeton(chemin, "fichier de jeton introuvable ou illisible")
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return "", erreurJeton(chemin, "le chemin ne designe pas un fichier regulier")
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0o600 {
			return "", erreurJeton(chemin, fmt.Sprintf("permissions %o invalides : exiger 0600 (chmod 0600)", info.Mode().Perm()))
		}
		if os.Getuid() >= 0 && uint64(os.Getuid()) != uidFichier(info) {
			return "", erreurJeton(chemin, "le fichier n'appartient pas a l'utilisateur courant")
		}
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

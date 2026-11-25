// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"bytes"
	"io"
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

// ChargerJeton lit le fichier de jeton de lecture du pilote via un HANDLE
// unique (anti-substitution, le chemin n'est JAMAIS ré-résolu après le
// contrôle). Contrôles fail-closed AVANT la lecture (voir
// ouvrirJetonProtege) : fichier régulier (lien symbolique refusé),
// identité du fichier ouvert vérifiée contre le contrôle initial,
// protection effective exigée (POSIX : 0600 + propriétaire = utilisateur
// courant ; Windows : DACL natif restreint à l'utilisateur courant,
// protégé contre l'héritage). Aucune valeur de jeton n'apparaît jamais
// dans les erreurs.
func ChargerJeton(chemin string) (string, error) {
	if strings.TrimSpace(chemin) == "" {
		return "", &ErreurJeton{Motif: "chemin du fichier de jeton requis"}
	}
	f, err := ouvrirJetonProtege(chemin)
	if err != nil {
		return "", err
	}
	defer f.Close()
	brut, err := io.ReadAll(io.LimitReader(f, 8192))
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
// (SEC-MCP-01/02, ordre résiduel) : le fichier temporaire est créé
// EXCLUSIVEMENT (jamais de collision, jamais de suivi de lien) DÉJÀ
// PROTÉGÉ — Windows : attributs de sécurité à la création (DACL natif
// restreint à l'utilisateur courant, sans héritage) ; POSIX : mode 0600
// à la création (O_EXCL) — puis cette protection effective est VÉRIFIÉE
// sur le handle AVANT toute écriture du secret (fail-closed : refus et
// suppression du temporaire sinon). Le partage est refusé pendant
// l'écriture (Windows : FILE_SHARE_NONE). Remplacement atomique final
// de la cible (rotation) ; une cible existante non régulière (lien
// symbolique, répertoire) est refusée. Aucune valeur de jeton
// n'apparaît dans les erreurs.
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
	f, err := creerTemporaireProtege(chemin)
	if err != nil {
		return erreurJeton(chemin, "creation exclusive du fichier temporaire protege impossible")
	}
	tmp := f.Name()
	passe := false
	defer func() {
		if !passe {
			f.Close()
			os.Remove(tmp)
		}
	}()
	// Fail-closed : la protection effective est vérifiée sur le handle
	// AVANT toute écriture du secret.
	if err := verifierTemporaireProtege(chemin, f); err != nil {
		return err
	}
	if _, err := f.WriteString(valeur + "\n"); err != nil {
		return erreurJeton(chemin, "ecriture du fichier temporaire impossible")
	}
	if err := f.Sync(); err != nil {
		return erreurJeton(chemin, "synchronisation du fichier temporaire impossible")
	}
	if err := f.Close(); err != nil {
		return erreurJeton(chemin, "fermeture du fichier temporaire impossible")
	}
	if err := remplacerFichierJeton(tmp, chemin); err != nil {
		return erreurJeton(chemin, "remplacement atomique du fichier de jeton impossible")
	}
	passe = true
	return nil
}

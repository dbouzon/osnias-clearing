// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// uidFichier extrait l'UID propriétaire du fichier (POSIX), ^uint64(0) si
// le type système est indisponible (contrôle propriétaire alors sans objet).
func uidFichier(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Uid)
	}
	return ^uint64(0)
}

// creerTemporaireProtege (POSIX) : création exclusive O_EXCL, mode 0600 à
// la création (l'umask ne peut que rétrécir les droits : le secret n'a
// jamais existé sous une protection plus large).
func creerTemporaireProtege(cheminCible string) (*os.File, error) {
	return os.CreateTemp(filepath.Dir(cheminCible), filepath.Base(cheminCible)+".tmp-*")
}

// verifierTemporaireProtege (POSIX) : fail-closed AVANT écriture du
// secret : le handle ouvert doit être un fichier régulier en 0600.
func verifierTemporaireProtege(chemin string, f *os.File) error {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return erreurJeton(chemin, "controle du fichier temporaire impossible")
	}
	if info.Mode().Perm() != 0o600 {
		return erreurJeton(chemin, "fichier temporaire non protege a la creation (0600 attendu)")
	}
	return nil
}

// ouvrirJetonProtege (POSIX) : contrôle anti-substitution par handles.
// Le chemin est résolu une seule fois : Lstat initial (refus des liens
// symboliques et répertoires), ouverture, puis TOUTES les vérifications
// et la lecture se font sur le handle ouvert : identité dev/ino exigée
// identique au contrôle initial (sinon substitution), permissions 0600,
// propriétaire = utilisateur courant.
func ouvrirJetonProtege(chemin string) (*os.File, error) {
	linfo, err := os.Lstat(chemin)
	if err != nil {
		return nil, erreurJeton(chemin, "fichier de jeton introuvable ou illisible")
	}
	if !linfo.Mode().IsRegular() {
		return nil, erreurJeton(chemin, "le chemin ne designe pas un fichier regulier (lien symbolique refuse)")
	}
	f, err := os.Open(chemin)
	if err != nil {
		return nil, erreurJeton(chemin, "fichier de jeton introuvable ou illisible")
	}
	finfo, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, erreurJeton(chemin, "controle du fichier ouvert impossible")
	}
	if !finfo.Mode().IsRegular() {
		f.Close()
		return nil, erreurJeton(chemin, "le chemin ne designe pas un fichier regulier")
	}
	ls, ok1 := linfo.Sys().(*syscall.Stat_t)
	fs, ok2 := finfo.Sys().(*syscall.Stat_t)
	if !ok1 || !ok2 || ls.Dev != fs.Dev || ls.Ino != fs.Ino {
		f.Close()
		return nil, erreurJeton(chemin, "substitution du fichier detectee entre le controle et l'ouverture")
	}
	if finfo.Mode().Perm() != 0o600 {
		f.Close()
		return nil, erreurJeton(chemin, fmt.Sprintf("permissions %o invalides : exiger 0600 (chmod 0600)", finfo.Mode().Perm()))
	}
	if os.Getuid() >= 0 && uint64(os.Getuid()) != uidFichier(finfo) {
		f.Close()
		return nil, erreurJeton(chemin, "le fichier n'appartient pas a l'utilisateur courant")
	}
	return f, nil
}

// remplacerFichierJeton (POSIX) : renommage atomique. Un lien symbolique
// éventuel serait remplacé comme entrée de répertoire, jamais suivi.
func remplacerFichierJeton(src, dst string) error {
	return os.Rename(src, dst)
}

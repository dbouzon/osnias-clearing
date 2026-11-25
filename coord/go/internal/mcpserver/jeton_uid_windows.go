// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protection EFFECTIVE du fichier de jeton sous Windows (SEC-MCP-01 et
// ordre résiduel) : les bits POSIX (0600) n'existent pas ; la garantie
// est un DACL natif PROTÉGÉ (sans héritage du répertoire parent)
// restreint au seul utilisateur courant, appliqué AUX ATTRIBUTS DE
// SÉCURITÉ DE LA CRÉATION (aucune fenêtre d'exposition du secret) puis
// vérifié en fail-closed sur les handles.

// uidFichier : sous Windows, la notion d'UID POSIX n'existe pas ; le
// contrôle propriétaire est assuré par la vérification du DACL natif.
func uidFichier(info os.FileInfo) uint64 { return ^uint64(0) }

// sidUtilisateurCourant retourne le SID de l'utilisateur du processus.
func sidUtilisateurCourant() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

// descripteurRestreintUtilisateur construit le descripteur de sécurité
// passé AUX ATTRIBUTS DE CRÉATION : SDDL "D:P(A;;FA;;;<SID>)" — DACL
// protégé (aucun héritage de %TEMP%), accès complet au seul utilisateur
// courant.
func descripteurRestreintUtilisateur() (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := sidUtilisateurCourant()
	if err != nil {
		return nil, err
	}
	sddl := "D:P(A;;FA;;;" + sid.String() + ")"
	return windows.SecurityDescriptorFromString(sddl)
}

// creerTemporaireProtege (Windows) : création exclusive CREATE_NEW
// (aucune collision, aucun suivi de lien) DÉJÀ PROTÉGÉE par les
// attributs de sécurité de création (DACL restreint protégé) et SANS
// partage (FILE_SHARE_NONE) : aucun autre processus ne peut ouvrir le
// fichier pendant l'écriture du secret.
func creerTemporaireProtege(cheminCible string) (*os.File, error) {
	sd, err := descripteurRestreintUtilisateur()
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	repertoire := filepath.Dir(cheminCible)
	base := filepath.Base(cheminCible)
	for essai := 0; essai < 32; essai++ {
		suffixe := make([]byte, 8)
		if _, err := rand.Read(suffixe); err != nil {
			return nil, err
		}
		nom := filepath.Join(repertoire, base+".tmp-"+hex.EncodeToString(suffixe))
		p16, err := windows.UTF16PtrFromString(nom)
		if err != nil {
			return nil, err
		}
		h, err := windows.CreateFile(p16,
			windows.GENERIC_WRITE, 0, sa, windows.CREATE_NEW,
			windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != nil {
			if errors.Is(err, windows.ERROR_FILE_EXISTS) {
				continue
			}
			return nil, err
		}
		return os.NewFile(uintptr(h), nom), nil
	}
	return nil, errors.New("creation exclusive impossible apres 32 essais")
}

// verifierTemporaireProtege (Windows) : fail-closed AVANT écriture du
// secret : le DACL DU HANDLE créé doit être restreint et protégé.
func verifierTemporaireProtege(chemin string, f *os.File) error {
	return verifierDaclHandle(chemin, windows.Handle(f.Fd()))
}

// verifierDaclHandle vérifie SUR LE HANDLE (le chemin n'est jamais
// ré-résolu) que le DACL : existe, est PROTÉGÉ (SE_DACL_PROTECTED, aucun
// héritage), ne contient aucun ACE hérité et n'accorde l'accès qu'à
// l'utilisateur courant — sinon REFUS.
func verifierDaclHandle(chemin string, h windows.Handle) error {
	sid, err := sidUtilisateurCourant()
	if err != nil {
		return erreurJeton(chemin, "identification de l'utilisateur courant impossible")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return erreurJeton(chemin, "lecture du DACL natif impossible")
	}
	ctrl, _, err := sd.Control()
	if err != nil || ctrl&windows.SE_DACL_PROTECTED == 0 {
		return erreurJeton(chemin, "protection native non protegee (DACL sujet a l'heritage du repertoire)")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return erreurJeton(chemin, "aucune protection native (DACL) sur le fichier de jeton")
	}
	if dacl.AceCount == 0 {
		return erreurJeton(chemin, "protection native vide (aucun droit explicite)")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return erreurJeton(chemin, "lecture des droits natifs impossible")
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			return erreurJeton(chemin, "droits natifs herites ou non explicites refuses")
		}
		titulaire := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !windows.EqualSid(titulaire, sid) {
			return erreurJeton(chemin, "fichier accessible par un principal autre que l'utilisateur courant")
		}
	}
	return nil
}

// verifierIdentiteSansSubstitution (Windows) : le handle ouvert doit
// désigner exactement le fichier au chemin contrôlé. Une seconde
// ouverture FILE_FLAG_OPEN_REPARSE_POINT (qui ne suit JAMAIS un point
// d'analyse) détecte tout lien symbolique et toute substitution : les
// identités FileIndex doivent coïncider.
func verifierIdentiteSansSubstitution(chemin string, f *os.File) error {
	var direct windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &direct); err != nil {
		return erreurJeton(chemin, "identification du fichier ouvert impossible")
	}
	p16, err := windows.UTF16PtrFromString(chemin)
	if err != nil {
		return erreurJeton(chemin, "chemin du fichier de jeton invalide")
	}
	h, err := windows.CreateFile(p16, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return erreurJeton(chemin, "controle anti-substitution impossible (lien symbolique refuse)")
	}
	defer windows.CloseHandle(h)
	var auChemin windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &auChemin); err != nil {
		return erreurJeton(chemin, "identification du fichier au chemin impossible")
	}
	if auChemin.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return erreurJeton(chemin, "le chemin ne designe pas un fichier regulier (lien symbolique refuse)")
	}
	if auChemin.FileIndexHigh != direct.FileIndexHigh ||
		auChemin.FileIndexLow != direct.FileIndexLow ||
		auChemin.VolumeSerialNumber != direct.VolumeSerialNumber {
		return erreurJeton(chemin, "substitution du fichier detectee entre le controle et l'ouverture")
	}
	return nil
}

// ouvrirJetonProtege (Windows) : contrôle anti-substitution par handles.
// Lstat initial (refus des liens symboliques et répertoires), ouverture
// unique, puis TOUTES les vérifications et la lecture se font sur le
// handle ouvert : identité croisée (FileIndex, refus des points
// d'analyse), DACL vérifié SUR LE HANDLE.
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
	if err := verifierIdentiteSansSubstitution(chemin, f); err != nil {
		f.Close()
		return nil, err
	}
	if err := verifierDaclHandle(chemin, windows.Handle(f.Fd())); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// remplacerFichierJeton (Windows) : os.Rename n'écrase pas une cible
// existante ; la cible régulière est retirée puis remplacée (le dossier
// personnel %TEMP% n'est pas partagé ; une cible non régulière a déjà
// été refusée en amont par Lstat).
func remplacerFichierJeton(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if _, lerr := os.Lstat(dst); lerr != nil {
		return err
	}
	if rerr := os.Remove(dst); rerr != nil {
		return rerr
	}
	return os.Rename(src, dst)
}

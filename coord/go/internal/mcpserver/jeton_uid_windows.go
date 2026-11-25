// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8892-8902
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package mcpserver

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protection effective du fichier de jeton sous Windows (SEC-MCP-01) : les
// bits POSIX (0600) n'existent pas ; la garantie équivalente est un DACL
// natif PROTEGE (sans héritage) restreint au seul utilisateur courant.

// masqueToutAccesFichier est FILE_ALL_ACCESS (droits fichiers complets).
const masqueToutAccesFichier = 0x1F01FF

// uidFichier : sous Windows, la notion d'UID POSIX n'existe pas ; le
// contrôle propriétaire est assuré par verifierProtectionJeton (DACL
// natif restreint à l'utilisateur courant).
func uidFichier(info os.FileInfo) uint64 { return ^uint64(0) }

// sidUtilisateurCourant retourne le SID de l'utilisateur du processus.
func sidUtilisateurCourant() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

// daclRestreintUtilisateur construit un DACL protégé (aucun héritage du
// répertoire parent) accordant l'accès au seul utilisateur courant.
func daclRestreintUtilisateur() (*windows.ACL, error) {
	sid, err := sidUtilisateurCourant()
	if err != nil {
		return nil, err
	}
	return windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: masqueToutAccesFichier,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, nil)
}

// protegerFichierJeton (Windows) : applique le DACL natif restreint et
// protégé (PROTECTED_DACL : les droits héritables du répertoire parent ne
// sont PAS fusionnés).
func protegerFichierJeton(chemin string) error {
	dacl, err := daclRestreintUtilisateur()
	if err != nil {
		return erreurJeton(chemin, "identification de l'utilisateur courant impossible")
	}
	if err := windows.SetNamedSecurityInfo(chemin, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return erreurJeton(chemin, "application du DACL natif restreint impossible")
	}
	return nil
}

// verifierProtectionJeton (Windows, SEC-MCP-01) : fail-closed. Le DACL doit
// exister, être protégé (aucun ACE hérité) et n'accorder l'accès qu'à
// l'utilisateur courant ; tout autre titulaire ou tout ACE hérité est un
// refus.
func verifierProtectionJeton(chemin string, _ os.FileInfo) error {
	sid, err := sidUtilisateurCourant()
	if err != nil {
		return erreurJeton(chemin, "identification de l'utilisateur courant impossible")
	}
	sd, err := windows.GetNamedSecurityInfo(chemin, windows.SE_FILE_OBJECT,
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

// remplacerFichierJeton (Windows) : os.Rename n'écrase pas une cible
// existante ; la cible régulière est retirée puis remplacée (le dossier
// personnel %TEMP% n'est pas partagé ; une cible non régulière a déjà été
// refusée en amont par Lstat).
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

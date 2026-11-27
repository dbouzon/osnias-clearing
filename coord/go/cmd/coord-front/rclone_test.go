// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests de l'adaptateur Drive rclone (mission M-COORD-DRIVE-01, plan
// v2 §2). Deux niveaux :
//
//   - FAUX BINAIRE (script en TempDir, chemin injecté) : arguments
//     transmis, fail-closed, timeout, mapping d'erreurs — couverts
//     PARTOUT, y compris les runners CI sans rclone (unix seulement :
//     un script shell n'y est pas exécutable) ;
//   - VRAI RCLONE : intégration sur un remote LOCAL dédié (backend
//     alias, racine TempDir) ; SAUTÉ avec message explicite si le
//     binaire est absent du PATH.
package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// dossiersDefauts : Dossiers qui échoue toujours, pour l'option A
// (export défaillant) sans dépendre de rclone ni de l'OS.
type dossiersDefauts struct {
	motif string
}

func (d *dossiersDefauts) Lister(string) ([]string, error) { return nil, errors.New(d.motif) }
func (d *dossiersDefauts) Lire(string, string) ([]byte, error) {
	return nil, errors.New(d.motif)
}
func (d *dossiersDefauts) Ecrire(string, string, []byte) error {
	return errors.New(d.motif)
}

// fauxRclone : script shell journalisant ses arguments et son stdin
// dans $FAUX_RCLONE_JOURNAL, avec des réponses simulées par sous-commande.
func fauxRclone(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("faux binaire shell : non exécutable sur windows")
	}
	chemin := filepath.Join(t.TempDir(), "rclone-faux")
	if err := os.WriteFile(chemin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return chemin
}

func TestRcloneValidationFailClosed(t *testing.T) {
	d := &dossierRclone{remote: "recette", racine: "coord", timeout: time.Second, binaire: "/bin/true"}
	// Dossiers interdits : remontée, ':', segment vide, contrôle.
	for _, dossier := range []string{"..", "../x", "a/../b", "a:b", "/absolu", "a//b", "a\x01b"} {
		if _, err := d.cible(dossier, ""); err == nil {
			t.Fatalf("dossier %q accepté à tort", dossier)
		}
	}
	// Noms interdits : chemin, ':', remontée, contrôle, blanc.
	for _, nom := range []string{"a/b", "a\\b", "a:b", "..", ".", "a\x01b", " "} {
		if _, err := d.cible("", nom); err == nil {
			t.Fatalf("nom %q accepté à tort", nom)
		}
	}
	// Nom vide : refusé comme document (la cible VIDE signifie
	// « lister le dossier », jamais « lire un document vide »).
	if err := validerNom(""); err == nil {
		t.Fatal("nom vide accepté à tort")
	}
	// Le remote vient de la config : un nom injectable est refusé.
	if err := validerNomRemote("rec:ette"); err == nil {
		t.Fatal("remote avec ':' accepté à tort")
	}
	if err := validerNomRemote(""); err == nil {
		t.Fatal("remote vide accepté à tort")
	}
	// Cibles licites : racine du compte, sous-dossier, document.
	cas := map[[2]string]string{
		{"", ""}:           "recette:coord",
		{"sortie", ""}:     "recette:coord/sortie",
		{"sortie", "a.js"}: "recette:coord/sortie/a.js",
	}
	for entree, attendu := range cas {
		cible, err := d.cible(entree[0], entree[1])
		if err != nil || cible != attendu {
			t.Fatalf("cible(%q,%q) = %q, %v ; attendu %q", entree[0], entree[1], cible, err, attendu)
		}
	}
	// Racine vide : le remote seul, sans chemin injectable.
	d2 := &dossierRclone{remote: "recette", racine: "", timeout: time.Second}
	if cible, err := d2.cible("", ""); err != nil || cible != "recette:" {
		t.Fatalf("cible racine vide = %q, %v", cible, err)
	}
}

func TestRcloneAbsent(t *testing.T) {
	d := &dossierRclone{remote: "recette", racine: "coord",
		timeout: time.Second, binaire: filepath.Join(t.TempDir(), "introuvable")}
	if _, err := d.Lister("sortie"); err == nil {
		t.Fatal("binaire absent : Lister doit échouer")
	} else {
		statut, code, _ := statutErreurDrive(err)
		if statut != 503 || code != "DRIVE_INDISPONIBLE" {
			t.Fatalf("binaire absent : attendu 503 DRIVE_INDISPONIBLE, reçu %d %s", statut, code)
		}
	}
}

func TestRcloneFauxBinaireArguments(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "journal")
	t.Setenv("FAUX_RCLONE_JOURNAL", journal)
	binaire := fauxRclone(t, `printf '%s\n' "$@" >> "$FAUX_RCLONE_JOURNAL"
cat >> "$FAUX_RCLONE_JOURNAL"
case "$1" in
  lsf) printf 'a.txt\nb.txt\n' ;;
  cat) printf 'contenu lu' ;;
  rcat) : ;;
  mkdir) : ;;
  *) exit 1 ;;
esac`)
	d := &dossierRclone{remote: "recette", racine: "coord", timeout: 5 * time.Second, binaire: binaire}

	noms, err := d.Lister("sortie")
	if err != nil || !reflect.DeepEqual(noms, []string{"a.txt", "b.txt"}) {
		t.Fatalf("Lister : %v %v", noms, err)
	}
	contenu, err := d.Lire("sortie", "a.txt")
	if err != nil || string(contenu) != "contenu lu" {
		t.Fatalf("Lire : %q %v", contenu, err)
	}
	if err := d.Ecrire("sortie", "note.txt", []byte("bonjour")); err != nil {
		t.Fatal(err)
	}
	trace, _ := os.ReadFile(journal)
	lignes := strings.Split(strings.TrimSpace(string(trace)), "\n")
	// Lister : lsf ; Lire : cat ; Écrire : mkdir puis rcat (stdin
	// journalisé sur sa propre ligne). Toutes les cibles sont
	// préfixées par la racine du compte : impossible d'en sortir.
	attendu := []string{
		"lsf", "recette:coord/sortie",
		"cat", "recette:coord/sortie/a.txt",
		"mkdir", "recette:coord/sortie",
		"rcat", "recette:coord/sortie/note.txt",
		"bonjour",
	}
	if len(lignes) != len(attendu) {
		t.Fatalf("journal du faux rclone : %v", lignes)
	}
	for i := range attendu {
		if lignes[i] != attendu[i] {
			t.Fatalf("journal[%d] = %q, attendu %q (trace %v)", i, lignes[i], attendu[i], lignes)
		}
	}
	// Le dossier et le nom validés AVANT tout appel : un nom interdit
	// ne produit AUCUNE ligne de journal.
	os.Remove(journal)
	if err := d.Ecrire("sortie", "../evil", []byte("x")); err == nil {
		t.Fatal("nom interdit accepté")
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("rclone appelé malgré un nom interdit (fail-closed contourné)")
	}
}

func TestRcloneFauxBinaireErreurEtTimeout(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "journal")
	t.Setenv("FAUX_RCLONE_JOURNAL", journal)
	// Sortie non nulle avec stderr : DRIVE_ERREUR (502).
	binaire := fauxRclone(t, `echo "boom simulé" >&2; exit 1`)
	d := &dossierRclone{remote: "recette", racine: "coord", timeout: 5 * time.Second, binaire: binaire}
	_, err := d.Lister("sortie")
	if err == nil {
		t.Fatal("échec rclone attendu")
	}
	statut, code, motif := statutErreurDrive(err)
	if statut != 502 || code != "DRIVE_ERREUR" || !strings.Contains(motif, "boom") {
		t.Fatalf("attendu 502 DRIVE_ERREUR (boom), reçu %d %s %q", statut, code, motif)
	}
	// Timeout : DRIVE_INDISPONIBLE (503), jamais d'appel pendu.
	lent := fauxRclone(t, `sleep 5`)
	dl := &dossierRclone{remote: "recette", racine: "coord",
		timeout: 150 * time.Millisecond, binaire: lent}
	avant := time.Now()
	_, errLent := dl.Lister("sortie")
	if errLent == nil {
		t.Fatal("timeout attendu")
	}
	statut, code, _ = statutErreurDrive(errLent)
	if statut != 503 || code != "DRIVE_INDISPONIBLE" {
		t.Fatalf("attendu 503 DRIVE_INDISPONIBLE, reçu %d %s", statut, code)
	}
	if time.Since(avant) > 3*time.Second {
		t.Fatal("l'appel a survécu à son timeout")
	}
}

// remoteLocalRclone : remote rclone LOCAL dédié (backend alias, racine
// TempDir), mêmes chemins de code que Google Drive, zéro clé.
func remoteLocalRclone(t *testing.T, nom string) string {
	t.Helper()
	if _, err := exec.LookPath("rclone"); err != nil {
		t.Skip("binaire rclone absent du PATH : test d'intégration sauté (couvert par le faux binaire)")
	}
	racine := t.TempDir()
	t.Setenv("RCLONE_CONFIG_"+strings.ToUpper(nom)+"_TYPE", "alias")
	t.Setenv("RCLONE_CONFIG_"+strings.ToUpper(nom)+"_REMOTE", racine)
	return racine
}

func TestRcloneReelRemoteLocal(t *testing.T) {
	racine := remoteLocalRclone(t, "recette")
	d := nouveauDossierRclone("recette", "coord", 10*time.Second)

	// Écrire → Lister → Lire, dans le périmètre du compte.
	if err := d.Ecrire("sortie", "note.txt", []byte("bonjour drive")); err != nil {
		t.Fatalf("Ecrire : %v", err)
	}
	noms, err := d.Lister("sortie")
	if err != nil || !reflect.DeepEqual(noms, []string{"note.txt"}) {
		t.Fatalf("Lister : %v %v", noms, err)
	}
	contenu, err := d.Lire("sortie", "note.txt")
	if err != nil || string(contenu) != "bonjour drive" {
		t.Fatalf("Lire : %q %v", contenu, err)
	}
	// Le fichier est bien SOUS la racine du compte.
	if _, err := os.Stat(filepath.Join(racine, "coord", "sortie", "note.txt")); err != nil {
		t.Fatalf("fichier exporté introuvable sous la racine : %v", err)
	}
	// Dossier absent : liste vide (parité avec l'adaptateur factice).
	vide, err := d.Lister("inexistant")
	if err != nil || len(vide) != 0 {
		t.Fatalf("dossier absent : %v %v", vide, err)
	}
	// Document absent : DRIVE_ABSENT (404), pas un crash.
	if _, err := d.Lire("inexistant", "x.txt"); err == nil {
		t.Fatal("lecture d'un document absent doit échouer")
	} else {
		statut, code, _ := statutErreurDrive(err)
		if statut != 404 || code != "DRIVE_ABSENT" {
			t.Fatalf("attendu 404 DRIVE_ABSENT, reçu %d %s", statut, code)
		}
	}
	// Timeout config clampé : défaut 30 s, plancher 5 s, plafond 120 s.
	for entree, attendu := range map[int]time.Duration{
		0: 30 * time.Second, 3: 5 * time.Second, 45: 45 * time.Second, 999: 120 * time.Second,
	} {
		if d := (compteConf{TimeoutRclone: entree}).timeoutDrive(); d != attendu {
			t.Fatalf("timeoutDrive(%d) = %v, attendu %v", entree, d, attendu)
		}
	}
}

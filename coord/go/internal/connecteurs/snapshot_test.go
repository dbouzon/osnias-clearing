// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/missions"
)

// relireSnapshot : relecture stricte de la projection définitive (jamais le
// temporaire, jamais une cache mémoire).
func relireSnapshot(t *testing.T, racine, issue string) Snapshot {
	t.Helper()
	b, err := os.ReadFile(CheminSnapshot(racine, issue))
	if err != nil {
		t.Fatalf("snapshot illisible : %v", err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("snapshot non JSON : %v", err)
	}
	if s.SchemaVersion != "1.0" {
		t.Fatalf("schema_version 1.0 attendu, obtenu %s", s.SchemaVersion)
	}
	return s
}

// TestSnapshotCycleReelSimule : cycle sur serveur factice loopback (SIMULÉ,
// 0 €). À CHAQUE point durable (TX2), la projection est régénérée : état
// conforme à l'état machine PERSISTÉ, empreinte du journal = sha256 réelle
// du journal, compteur croissant, atomicité (aucun temporaire résiduel),
// mention conservatoire présente.
func TestSnapshotCycleReelSimule(t *testing.T) {
	h, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 40000, 1000, 0)
	orch, ref := orchestreAvecReel(t, h)
	issue := "harden-snap"
	p := plafondsTest()
	p.Centimes = 1000000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	etapes := []struct {
		nom         string
		etatAttendu string
		appel       func() error
	}{
		{"executer", "EXECUTION", func() error {
			_, e := orch.Executer(context.Background(), issue, "mistral-exec", "MANDAT EXECUTE")
			return e
		}},
		{"soumettre", "DECISION_ATTENDUE", func() error {
			_, e := orch.Soumettre(context.Background(), issue, "mistral-exec", "LIVRABLE sha256:ab")
			return e
		}},
	}
	entreesPrec := 0
	for _, eta := range etapes {
		if err := eta.appel(); err != nil {
			t.Fatalf("%s : %v", eta.nom, err)
		}
		s := relireSnapshot(t, ref.Racine(), issue)
		if s.Issue != issue || s.EmpreinteJournalSHA == "" ||
			s.EntreesJournal <= entreesPrec {
			t.Fatalf("snapshot incohérent après %s : %+v", eta.nom, s)
		}
		enr, err := ref.Obtenir(issue)
		if err != nil || enr.Mission.State != s.EtatMission {
			t.Fatalf("état projeté %s != état persisté %+v (err %v)", s.EtatMission, enr.Mission.State, err)
		}
		jb, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue))
		if err != nil {
			t.Fatalf("journal illisible : %v", err)
		}
		sum := sha256.Sum256(jb)
		if s.EmpreinteJournalSHA != hex.EncodeToString(sum[:]) {
			t.Fatalf("empreinte journal divergente après %s", eta.nom)
		}
		entreesPrec = s.EntreesJournal
		restants, _ := filepath.Glob(filepath.Join(filepath.Dir(CheminSnapshot(ref.Racine(), issue)), ".snapshot-*"))
		if len(restants) != 0 {
			t.Fatalf("temporaire résiduel après %s : %v", eta.nom, restants)
		}
		if !strings.Contains(s.MentionConservatoire, "PAS un journal de rejeu") {
			t.Fatalf("mention conservatoire absente : %s", s.MentionConservatoire)
		}
	}
}

// TestSnapshotErreurEcritureJamaisAutoritaire : une projection impossible
// (répertoire en conflit sur le temporaire) n'échoue JAMAIS la transition
// durable : Executer réussit, l'erreur est journalisée (SNAPSHOT_ERREUR),
// aucune projection n'existe — preuve qu'elle n'est pas une autorité.
func TestSnapshotErreurEcritureJamaisAutoritaire(t *testing.T) {
	h, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 40000, 1000, 0)
	orch, ref := orchestreAvecReel(t, h)
	issue := "harden-snap-err"
	p := plafondsTest()
	p.Centimes = 1000000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	// Conflit portable (Linux/macOS/Windows) : un RÉPERTOIRE à l'emplacement
	// cible fait échouer le renommage atomique APRÈS fsync (le contrat
	// d'écriture durable reste sinon intact).
	if err := os.Mkdir(CheminSnapshot(ref.Racine(), issue), 0o755); err != nil {
		t.Fatalf("préparation du conflit : %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(CheminSnapshot(ref.Racine(), issue)) })
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "MANDAT"); err != nil {
		t.Fatalf("la transition durable doit RÉUSSIR malgré la projection impossible : %v", err)
	}
	if fi, err := os.Stat(CheminSnapshot(ref.Racine(), issue)); err != nil || fi != nil && !fi.IsDir() {
		t.Fatalf("aucune projection (fichier) ne doit exister : %v", err)
	}
	restants, _ := filepath.Glob(filepath.Join(filepath.Dir(CheminSnapshot(ref.Racine(), issue)), ".snapshot-*"))
	if len(restants) != 0 {
		t.Fatalf("temporaire résiduel après échec : %v", restants)
	}
	jb, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue))
	if err != nil || !strings.Contains(string(jb), "SNAPSHOT_ERREUR") {
		t.Fatalf("SNAPSHOT_ERREUR attendu au journal : err=%v", err)
	}
}

// TestSnapshotSansTexteNiSecret : la projection ne contient QUE des données
// dérivées (état, empreinte, compteurs), AUCUN texte de charge ou de verdict :
// ni secret (sk-/AIza), ni mandat ne peut l'atteindre.
func TestSnapshotSansTexteNiSecret(t *testing.T) {
	mandat := "MANDAT sk-facticeabcdef12 AIzaSyabc1234567890abcdefghijklmnopqrsXYZ SECRET_DATA"
	h, _ := nouveauConnecteurReelFactice(t, nil, 100, 300, 1, 40000, 1000, 0)
	orch, ref := orchestreAvecReel(t, h)
	issue := "harden-snap-sec"
	p := plafondsTest()
	p.Centimes = 1000000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", mandat); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	relireSnapshot(t, ref.Racine(), issue) // doit exister et être JSON schema 1.0
	b, _ := os.ReadFile(CheminSnapshot(ref.Racine(), issue))
	corpus := string(b)
	for _, interdit := range []string{"sk-facticeabcdef12", "AIzaSy", "MANDAT ", "SECRET_DATA"} {
		if strings.Contains(corpus, interdit) {
			t.Fatalf("contenu interdit «%s» dans la projection : %s", interdit, corpus)
		}
	}
}

// REPRO P5-2 (réception complémentaire 7176998049, reproduction AVANT
// correction, garde-fou permanent) : une ERREUR de lecture de
// l'enregistrement d'appel non résolu ne doit JAMAIS être présentée comme
// une ABSENCE d'appel dans la projection. Scénario : marqueur appel_en_cours
// illisible (état INCONNU), puis point durable ACK. Code défaillant : la
// projection est régénérée en affirmant l'absence (champ vide) SANS aucune
// trace d'erreur. Code corrigé : la projection ÉCHOUE (état inconnu != état
// absent), l'échec est journalisé SNAPSHOT_ERREUR, la projection reste
// périmée (non régénérée : état antérieur ACCEPTEE conservé, marqueur
// conservatoire « peut être périmée »), et la transition durable ACK
// RÉUSSIT (jamais autoritaire).
func TestReproP5ErreurAppelJamaisPresenteeCommeAbsence(t *testing.T) {
	b := nouveauBanc(t)
	issue := "harden-snap-repro-p5"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "MANDAT EXECUTE"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", "LIVRABLE sha256:ab"); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if _, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", "LIVRABLE sha256:ab"); err != nil {
		t.Fatalf("réception : %v", err)
	}
	// Dernière projection saine : état ACCEPTEE.
	if s := relireSnapshot(t, b.ref.Racine(), issue); s.EtatMission != "ACCEPTEE" {
		t.Fatalf("projection saine attendue à l'état ACCEPTEE : %+v", s)
	}
	// Marqueur d'appel rendu ILLISIBLE : l'état de l'appel est INCONNU.
	if err := os.WriteFile(cheminAppel(b.ref.Racine(), issue), []byte("PAS_DU_JSON"), 0o600); err != nil {
		t.Fatalf("corruption du marqueur : %v", err)
	}
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("la transition durable ACK doit RÉUSSIR (projection jamais autoritaire) : %v", err)
	}
	// Exigence 1 : l'état INCONNU doit être OBSERVABLE, pas silencieux.
	if !strings.Contains(b.journal(t, issue), "SNAPSHOT_ERREUR") {
		t.Fatalf("REPRO P5-2 : l'erreur de lecture de l'appel en cours doit être journalisée " +
			"SNAPSHOT_ERREUR, pas absorbée en silence")
	}
	// Exigence 2 : la projection ne doit PAS être régénérée en affirmant une
	// absence invérifiable : elle reste périmée à ACCEPTEE (mention
	// conservatoire), l'état réel ACK_RECU n'y figure que si lisible.
	s := relireSnapshot(t, b.ref.Racine(), issue)
	if s.EtatMission != "ACCEPTEE" {
		t.Fatalf("REPRO P5-2 : la projection ne doit pas être régénérée sur un état INCONNU "+
			"(réflexion de l'état réel %s = affichage d'absence invérifiable) : %+v", "ACK_RECU", s)
	}
	if b.etat(t, issue) != "ACK_RECU" {
		t.Fatalf("l'état machine réel doit être ACK_RECU : %s", b.etat(t, issue))
	}
}

// TestSnapshotFsyncAvantRenommage : GARDE-FOU PERMANENT P5-1 (réception
// complémentaire 7176998049). Le contrat T1 exige écriture temporaire +
// FSYNC + rename : chaque écriture de projection DOIT passer par
// fsyncFichier exactement une fois (durabilité du contenu AVANT le
// renommage atomique), et aucun temporaire ne doit subsister. Ce test
// échoue si quelqu'un supprime le fsync du chemin d'écriture.
func TestSnapshotFsyncAvantRenommage(t *testing.T) {
	syncs := 0
	ancien := fsyncFichier
	fsyncFichier = func(f *os.File) error {
		syncs++
		return f.Sync()
	}
	t.Cleanup(func() { fsyncFichier = ancien })
	b := nouveauBanc(t)
	issue := "harden-snap-fsync"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := b.orch.Executer(context.Background(), issue, "mistral-exec", "MANDAT"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if syncs != 1 {
		t.Fatalf("P5-1 : exactement 1 fsync attendu par écriture de projection (contrat "+
			"temp+fsync+rename), obtenu %d", syncs)
	}
	relireSnapshot(t, b.ref.Racine(), issue) // projection présente et JSON valide
	restants, _ := filepath.Glob(filepath.Join(filepath.Dir(CheminSnapshot(b.ref.Racine(), issue)), ".snapshot-*"))
	if len(restants) != 0 {
		t.Fatalf("P5-1 : temporaire résiduel : %v", restants)
	}
}

// TestSnapshotACKEtResolution : COUVERTURE des points de projection exigés
// par la réception complémentaire — ACK (projection régénérée à l'état
// ACK_RECU) et résolution d'appel en cours (projection régénérée après
// l'entrée de résolution du journal, empreinte du journal actualisée).
// La projection reste strictement non autoritaire dans les deux cas.
func TestSnapshotACKEtResolution(t *testing.T) {
	// --- Point ACK ---
	b := nouveauBanc(t)
	issue := "harden-snap-ack"
	if err := b.orch.NouvelleMission(issue, "mistral-exec", "openai-recep",
		rolesTest(), plafondsTest()); err != nil {
		t.Fatalf("création : %v", err)
	}
	ctx := context.Background()
	if _, err := b.orch.Executer(ctx, issue, "mistral-exec", "MANDAT EXECUTE"); err != nil {
		t.Fatalf("exécution : %v", err)
	}
	if _, err := b.orch.Soumettre(ctx, issue, "mistral-exec", "LIVRABLE sha256:ab"); err != nil {
		t.Fatalf("soumission : %v", err)
	}
	if _, _, err := b.orch.Recevoir(ctx, issue, "openai-recep", "LIVRABLE sha256:ab"); err != nil {
		t.Fatalf("réception : %v", err)
	}
	if err := b.orch.Acker(issue, "mistral-exec"); err != nil {
		t.Fatalf("ack : %v", err)
	}
	if s := relireSnapshot(t, b.ref.Racine(), issue); s.EtatMission != "ACK_RECU" {
		t.Fatalf("point ACK : projection attendue à l'état ACK_RECU : %+v", s)
	}

	// --- Point résolution d'appel en cours ---
	// Un appel EN_COURS non résolu est ce qu'un crash au milieu d'un appel
	// laisse sur disque (un échec transport PROPRE est résolu par
	// l'orchestrateur lui-même, état RESOLU_ECHOUE : rien à résoudre).
	// On fabrique donc le marqueur EN_COURS exact comme l'interruption
	// réelle le laisse, puis on couvre le point durable de résolution.
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{
		"mistral": pointReelBlanc("http://127.0.0.1:9", 10000),
	})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue2 := "harden-snap-resolution"
	p := plafondsTest()
	p.Centimes = 100000
	if err := orch.NouvelleMission(issue2, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if err := orch.appelEnCoursEcrit(&appelEnCours{
		Issue: issue2, Action: "executer", Etat: appelEnCoursEtatEnCours,
		Empreinte: "aaaa", CleIdemp: "cle-snap-resolution",
	}); err != nil {
		t.Fatalf("marqueur EN_COURS : %v", err)
	}
	if err := orch.ResoudreAppelEnCours(issue2, "RECU", "résolution test", "pilote"); err != nil {
		t.Fatalf("résolution : %v", err)
	}
	s := relireSnapshot(t, ref.Racine(), issue2)
	if s.EmpreinteJournalSHA == "" || s.EntreesJournal == 0 {
		t.Fatalf("point résolution : projection régénérée attendue : %+v", s)
	}
	jb, err := os.ReadFile(missions.JournalPath(ref.Racine(), issue2))
	if err != nil || !strings.Contains(string(jb), "résolu") {
		t.Fatalf("entrée de résolution attendue au journal : err=%v", err)
	}
	restants, _ := filepath.Glob(filepath.Join(filepath.Dir(CheminSnapshot(ref.Racine(), issue2)), ".snapshot-*"))
	if len(restants) != 0 {
		t.Fatalf("temporaire résiduel : %v", restants)
	}
}

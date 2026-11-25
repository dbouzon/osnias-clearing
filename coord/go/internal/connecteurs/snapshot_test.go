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
	"strings"
	"testing"

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
		if _, err := os.Stat(CheminSnapshot(ref.Racine(), issue) + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("temporaire résiduel après %s : %v", eta.nom, err)
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
	if err := os.Mkdir(CheminSnapshot(ref.Racine(), issue)+".tmp", 0o755); err != nil {
		t.Fatalf("préparation du conflit : %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(CheminSnapshot(ref.Racine(), issue) + ".tmp") })
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "MANDAT"); err != nil {
		t.Fatalf("la transition durable doit RÉUSSIR malgré la projection impossible : %v", err)
	}
	if _, err := os.Stat(CheminSnapshot(ref.Racine(), issue)); !os.IsNotExist(err) {
		t.Fatalf("aucune projection ne doit exister : %v", err)
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

// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coordengine/internal/missions"
)

// Observabilité backend (M-COORD-HARDENING-01, recommandation Gemini n° 5) :
// coord-snapshot.json est une PROJECTION LECTURE SEULE du dossier de mission,
// régénérée à chaque point durable (TX2 du committer, ACK, résolution
// d'appel en cours). GARDE-FOUS :
//   - JAMAIS une seconde source de vérité : aucun code normatif ne consulte
//     ce fichier pour décider quoi que ce soit ;
//   - l'échec d'écriture de la projection est JOURNALISÉ mais n'échoue
//     JAMAIS la transition normative qui la produit (une projection ne peut
//     pas devenir une autorité) ;
//   - aucun texte de charge/verdict n'est copié (aucun secret possible),
//     seuls l'état machine, l'empreinte sha256 du journal et des compteurs
//     sont projetés ;
//   - un snapshot seul N'EST PAS un journal de rejeu : le journal source
//     fait foi, la projection peut être périmée (mention explicite).

const SnapshotSchemaVersion = "1.0"
const SnapshotNomFichier = "coord-snapshot.json"

// Snapshot : projection observationnelle du dossier de mission.
type Snapshot struct {
	SchemaVersion        string `json:"schema_version"`
	GenereUTC            string `json:"genere_utc"`
	Issue                string `json:"issue"`
	EtatMission          string `json:"etat_mission"`
	AppelEnCours         string `json:"appel_en_cours_non_resolu"`
	EmpreinteJournalSHA  string `json:"empreinte_journal_sha256"`
	EntreesJournal       int    `json:"entrees_journal"`
	MentionConservatoire string `json:"mention"`
}

// CheminSnapshot : emplacement de la projection, dans le dossier de mission.
func CheminSnapshot(racine, issue string) string {
	return filepath.Join(filepath.Dir(missions.JournalPath(racine, issue)), SnapshotNomFichier)
}

// regenererSnapshot : relit l'état PERSISTÉ (jamais mémoire vive seule) et
// réécrit la projection de façon atomique (temporaire + rename, 0600).
func (o *Orchestrateur) regenererSnapshot(issue string) error {
	enr, err := o.ref.Obtenir(issue)
	if err != nil {
		return err
	}
	appel := ""
	if a, errA := o.appelEnCoursNonResolu(issue); errA == nil && a != nil {
		appel = a.Action
	}
	journal, errJ := os.ReadFile(missions.JournalPath(o.ref.Racine(), issue))
	if errJ != nil && !errors.Is(errJ, os.ErrNotExist) {
		return errJ
	}
	empreinte := ""
	entrees := 0
	if len(journal) > 0 {
		s := sha256.Sum256(journal)
		empreinte = hex.EncodeToString(s[:])
		for _, l := range strings.Split(string(journal), "\n") {
			if strings.TrimSpace(l) != "" {
				entrees++
			}
		}
	}
	snap := Snapshot{
		SchemaVersion:       SnapshotSchemaVersion,
		GenereUTC:           time.Now().UTC().Format(time.RFC3339),
		Issue:               issue,
		EtatMission:         enr.Mission.State,
		AppelEnCours:        appel,
		EmpreinteJournalSHA: empreinte,
		EntreesJournal:      entrees,
		MentionConservatoire: "projection lecture seule issue du journal durable ; " +
			"PAS un journal de rejeu ; peut être périmée entre deux régénérations ; " +
			"jamais consultée par le moteur (aucune décision ne lit ce fichier)",
	}
	return ecrireSnapshot(o.ref.Racine(), issue, &snap)
}

// ecrireSnapshot : écriture atomique (temp + rename, 0600) ; jamais de
// décision fondée sur le contenu précédent.
func ecrireSnapshot(racine, issue string, snap *Snapshot) error {
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	chemin := CheminSnapshot(racine, issue)
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, chemin); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// projeterDurable : point d'appel unique des transitions durables. La
// projection ne peut jamais faire échouer ce qui est déjà durable : son
// échec est journalisé, SANS échouer l'appelant.
func (o *Orchestrateur) projeterDurable(issue string) {
	if err := o.regenererSnapshot(issue); err != nil {
		o.ref.Journaliser(issue, map[string]any{
			"type":   "SNAPSHOT_ERREUR",
			"detail": "projection coord-snapshot.json impossible (transition durable INTACTE, projection non autoritaire) : " + err.Error(),
		})
	}
}

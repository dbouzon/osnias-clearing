package aic

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store assure la persistance SQLite : missions, événements (déduplication),
// transitions et journal des agents.
type Store struct {
	db *sql.DB
}

// Mission représente une mission GitHub (une Issue), portée par un
// identifiant stable indépendant du numéro d'Issue.
type Mission struct {
	ID          string
	IssueNumber int64
	Title       string
	State       string
	Corrections int
	CreatedAt   string
	UpdatedAt   string
}

// Event est un événement GitHub enregistré pour déduplication et reprise.
type Event struct {
	DeliveryID string
	EventType  string
	Received   time.Time
}

// schema porte la définition SQLite (livrable documentaire).
const schema = `
CREATE TABLE IF NOT EXISTS missions (
    id          TEXT PRIMARY KEY,
    issue_number INTEGER NOT NULL UNIQUE,
    title       TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL,
    corrections INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
    delivery_id TEXT PRIMARY KEY,
    event_type  TEXT NOT NULL,
    received_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mission_id TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state   TEXT NOT NULL,
    actor      TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    at         TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mission_id TEXT NOT NULL,
    agent      TEXT NOT NULL,
    phase      TEXT NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    at         TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mission_id TEXT NOT NULL,
    pr_draft   TEXT NOT NULL,
    status     TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_missions_state ON missions(state);
CREATE INDEX IF NOT EXISTS idx_transitions_mission ON transitions(mission_id);
`

// OpenStore ouvre (ou crée) la base SQLite du coordinateur.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("ouvrir sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite : une seule écriture à la fois.
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("appliquer le schéma: %w", err)
	}
	return &Store{db: db}, nil
}

// Close referme la base.
func (s *Store) Close() error { return s.db.Close() }

// SaveEvent enregistre un événement. Retourne false si l'identifiant de
// livraison est déjà connu (déduplication, protection contre les rejouements).
func (s *Store) SaveEvent(deliveryID, eventType string) (bool, error) {
	res, err := s.db.Exec(`INSERT INTO events (delivery_id, event_type, received_at) VALUES (?,?,?)`,
		deliveryID, eventType, nowISO())
	if err != nil {
		return false, nil // violation de clé primaire : rejou
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MissionByID retourne une mission, ou nil si absente.
func (s *Store) MissionByID(id string) (*Mission, error) {
	row := s.db.QueryRow(`SELECT id, issue_number, title, state, corrections, created_at, updated_at FROM missions WHERE id = ?`, id)
	m := &Mission{}
	err := row.Scan(&m.ID, &m.IssueNumber, &m.Title, &m.State, &m.Corrections, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// CreateMission crée une mission en état OPEN (idempotent : si l'Issue a déjà
// une mission, la mission existante est retournée sans modification).
func (s *Store) CreateMission(id string, issue int64, title string) (*Mission, error) {
	if m, err := s.MissionByID(id); err != nil {
		return nil, err
	} else if m != nil {
		return m, nil
	}
	now := nowISO()
	_, err := s.db.Exec(`INSERT INTO missions (id, issue_number, title, state, corrections, created_at, updated_at) VALUES (?,?,?,?,0,?,?)`,
		id, issue, title, StateOpen, now, now)
	if err != nil {
		return nil, fmt.Errorf("créer mission: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO transitions (mission_id, from_state, to_state, actor, detail, at) VALUES (?,?,?,?,?,?)`,
		id, "", StateOpen, "system", "création", now); err != nil {
		return nil, err
	}
	return s.MissionByID(id)
}

// Transition fait passer une mission dans un état autorisé et journalise le
// passage. Refuse toute transition non prévue par la machine d'états.
func (s *Store) Transition(m *Mission, to, actor, detail string) error {
	if !CanTransition(m.State, to) {
		return fmt.Errorf("transition interdite : %s -> %s (mission %s)", m.State, to, m.ID)
	}
	from := m.State
	if _, err := s.db.Exec(`UPDATE missions SET state = ?, updated_at = ? WHERE id = ?`, to, nowISO(), m.ID); err != nil {
		return err
	}
	m.State = to
	_, err := s.db.Exec(`INSERT INTO transitions (mission_id, from_state, to_state, actor, detail, at) VALUES (?,?,?,?,?,?)`,
		m.ID, from, to, actor, detail, nowISO())
	return err
}

// BumpCorrections incrémente le compteur de corrections demandées.
func (s *Store) BumpCorrections(m *Mission) (int, error) {
	if _, err := s.db.Exec(`UPDATE missions SET corrections = corrections + 1, updated_at = ? WHERE id = ?`, nowISO(), m.ID); err != nil {
		return 0, err
	}
	m.Corrections++
	return m.Corrections, nil
}

// Log enregistre une entrée de journal d'agent.
func (s *Store) Log(missionID, agent, phase, message string) error {
	_, err := s.db.Exec(`INSERT INTO agent_logs (mission_id, agent, phase, message, at) VALUES (?,?,?,?,?)`,
		missionID, agent, phase, message, nowISO())
	return err
}

// SaveDraft enregistre une livraison (PR Draft).
func (s *Store) SaveDraft(missionID, prDraft, status string) error {
	_, err := s.db.Exec(`INSERT INTO deliveries (mission_id, pr_draft, status, created_at) VALUES (?,?,?,?)`,
		missionID, prDraft, status, nowISO())
	return err
}

// PendingMissions retourne les missions non terminées (pour la reprise après
// interruption).
func (s *Store) PendingMissions() ([]*Mission, error) {
	rows, err := s.db.Query(`SELECT id, issue_number, title, state, corrections, created_at, updated_at FROM missions WHERE state NOT IN (?,?)`,
		StateAccepted, StateBlocked)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mission
	for rows.Next() {
		m := &Mission{}
		if err := rows.Scan(&m.ID, &m.IssueNumber, &m.Title, &m.State, &m.Corrections, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Counts retourne le nombre de transitions et de livraisons d'une mission,
// utilisé par les tests anti-double-exécution.
func (s *Store) Counts(missionID string) (transitions int, deliveries int, err error) {
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM transitions WHERE mission_id = ?`, missionID).Scan(&transitions); err != nil {
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM deliveries WHERE mission_id = ?`, missionID).Scan(&deliveries)
	return
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

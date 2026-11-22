package aic

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store assure la persistance SQLite : missions, événements (déduplication),
// transitions et journal des agents. Toutes les mises à jour multi-tables
// sont transactionnelles : aucune écriture partielle n'est possible.
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

// PendingEvent est un événement persisté mais non traité (T02-R2).
type PendingEvent struct {
	DeliveryID string
	EventType  string
	Payload    string
}

// ErrMissionLocked signale qu'une exécution est déjà en cours pour une
// mission : l'appelant doit s'abstenir (exclusivité d'exécution T02-R1/R3).
var ErrMissionLocked = errors.New("mission déjà en cours d'exécution (verrou actif)")

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
    payload     TEXT NOT NULL DEFAULT '',
    received_at TEXT NOT NULL,
    processed_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mission_id TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
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
CREATE TABLE IF NOT EXISTS mission_locks (
    mission_id TEXT PRIMARY KEY,
    claimed_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS github_ops (
    idem_key   TEXT PRIMARY KEY,
    mission_id TEXT NOT NULL,
    op_type    TEXT NOT NULL,
    state      TEXT NOT NULL DEFAULT 'pending',
    detail     TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS github_sent (
    idem_key TEXT PRIMARY KEY,
    at      TEXT NOT NULL
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
	// Migration T02-R2 des bases existantes : ajout du payload et de la date
	// de traitement à la table events (file d'attente persistante). Les
	// erreurs de colonne déjà présente sont ignorées (base déjà à jour).
	_, _ = db.Exec(`ALTER TABLE events ADD COLUMN payload TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE events ADD COLUMN processed_at TEXT NOT NULL DEFAULT ''`)
	// Migration T02-R4 : détail (référence de livraison) de l'opération GitHub.
	_, _ = db.Exec(`ALTER TABLE github_ops ADD COLUMN detail TEXT NOT NULL DEFAULT ''`)
	return &Store{db: db}, nil
}

// Close referme la base.
func (s *Store) Close() error { return s.db.Close() }

// SaveEvent enregistre durablement un événement, avec son payload brut,
// avant tout acquittement HTTP. Retourne (false, nil) si l'identifiant de
// livraison est déjà connu (rejou, déduplication), et (false, err) en cas
// d'échec réel de stockage : l'erreur n'est jamais assimilée à un doublon
// (T02-R1/R2). Le payload persisté permet de reprendre le traitement après
// un arrêt survenu entre l'acquittement 2xx et le traitement asynchrone.
func (s *Store) SaveEvent(deliveryID, eventType, payload string) (bool, error) {
	if deliveryID == "" {
		return false, fmt.Errorf("identifiant de livraison vide : événement non persistance")
	}
	res, err := s.db.Exec(`INSERT OR IGNORE INTO events (delivery_id, event_type, payload, received_at) VALUES (?,?,?,?)`,
		deliveryID, eventType, payload, nowISO())
	if err != nil {
		return false, fmt.Errorf("enregistrer événement %s: %w", deliveryID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("compter lignes événement %s: %w", deliveryID, err)
	}
	return n == 1, nil
}

// MarkEventProcessed marque un événement comme définitivement traité : il ne
// sera pas rejoué au redémarrage (T02-R2).
func (s *Store) MarkEventProcessed(deliveryID string) error {
	_, err := s.db.Exec(`UPDATE events SET processed_at = ? WHERE delivery_id = ?`, nowISO(), deliveryID)
	return err
}

// PendingEvents retourne les événements persistés non traités : file
// d'attente durable reprise après redémarrage (T02-R2).
func (s *Store) PendingEvents() ([]PendingEvent, error) {
	rows, err := s.db.Query(`SELECT delivery_id, event_type, payload FROM events WHERE processed_at = '' ORDER BY received_at, delivery_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingEvent
	for rows.Next() {
		var e PendingEvent
		if err := rows.Scan(&e.DeliveryID, &e.EventType, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EnqueueGitHubOp enregistre une écriture GitHub sortante sous sa clé
// d'idempotence (T02-R3, risque 2) : INSERT OR IGNORE, retour false si la
// clé est déjà connue. Les états sont pending (créée), sending (envoi en
// cours), done (réalisée). L'écriture diffère de SaveEvent : un échec réel
// n'est pas un doublon et l'erreur est renvoyée telle quelle.
func (s *Store) EnqueueGitHubOp(idemKey, missionID, opType string) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO github_ops (idem_key, mission_id, op_type, state, updated_at) VALUES (?,?,?,'pending',?)`,
		idemKey, missionID, opType, nowISO())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GitHubOpState retourne l'état d'une écriture GitHub (” si inconnue).
func (s *Store) GitHubOpState(idemKey string) (string, error) {
	var st string
	err := s.db.QueryRow(`SELECT state FROM github_ops WHERE idem_key = ?`, idemKey).Scan(&st)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return st, err
}

// MarkGitHubOpState fait évoluer l'état d'une écriture GitHub.
func (s *Store) MarkGitHubOpState(idemKey, state string) error {
	_, err := s.db.Exec(`UPDATE github_ops SET state = ?, updated_at = ? WHERE idem_key = ?`, state, nowISO(), idemKey)
	return err
}

// MarkGitHubOpDone marque l'opération 'done' en persistant son détail
// (T02-R4 : la référence de la livraison Draft survit à la même transaction
// de clôture ; la reprise retrouve la référence sans rappeler GitHub).
func (s *Store) MarkGitHubOpDone(idemKey, detail string) error {
	_, err := s.db.Exec(`UPDATE github_ops SET state = 'done', detail = ?, updated_at = ? WHERE idem_key = ?`, detail, nowISO(), idemKey)
	return err
}

// GitHubOpDetail retourne le détail persisté d'une opération GitHub (” si
// absent ou inconnu).
func (s *Store) GitHubOpDetail(idemKey string) (string, error) {
	var detail string
	err := s.db.QueryRow(`SELECT detail FROM github_ops WHERE idem_key = ?`, idemKey).Scan(&detail)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return detail, err
}

// GitHubSeenKey indique si une clé d'écriture GitHub a déjà été envoyée de
// façon durable (T02-R4 : déduplication de l'adaptateur résistante au
// redémarrage complet du processus).
func (s *Store) GitHubSeenKey(idemKey string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM github_sent WHERE idem_key = ?`, idemKey).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// GitHubMarkKey enregistre durablement l'envoi d'une clé d'écriture GitHub.
func (s *Store) GitHubMarkKey(idemKey string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO github_sent (idem_key, at) VALUES (?,?)`, idemKey, nowISO())
	return err
}

// DraftForMission retourne la référence de la livraison Draft persistée pour
// une mission (” si aucune) : une reprise après interruption réutilise la
// livraison existante au lieu d'en créer une seconde (T02-R3, risque 2).
func (s *Store) DraftForMission(missionID string) (string, error) {
	var ref string
	err := s.db.QueryRow(`SELECT pr_draft FROM deliveries WHERE mission_id = ? ORDER BY id DESC LIMIT 1`, missionID).Scan(&ref)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return ref, err
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

// CreateMission crée une mission en état OPEN et journalise la création dans
// une même transaction (idempotent : si l'Issue a déjà une mission, la
// mission existante est retournée sans modification).
func (s *Store) CreateMission(id string, issue int64, title string) (*Mission, error) {
	if m, err := s.MissionByID(id); err != nil {
		return nil, err
	} else if m != nil {
		return m, nil
	}
	now := nowISO()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO missions (id, issue_number, title, state, corrections, created_at, updated_at) VALUES (?,?,?,?,0,?,?)`,
		id, issue, title, StateOpen, now, now); err != nil {
		return nil, fmt.Errorf("créer mission: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO transitions (mission_id, from_state, to_state, actor, detail, at) VALUES (?,?,?,?,?,?)`,
		id, "", StateOpen, "system", "création", now); err != nil {
		return nil, fmt.Errorf("journaliser création mission: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.MissionByID(id)
}

// Transition fait passer une mission dans un état autorisé et journalise le
// passage dans une même transaction SQL : en cas d'échec, aucune écriture
// partielle (ni état modifié, ni journal orphelin) n'est possible (T02-R1/R4).
// La mise à jour est conditionnée à l'état de départ observé : si une autre
// exécution a fait progresser la mission entre-temps, la transition échoue.
func (s *Store) Transition(m *Mission, to, actor, detail string) error {
	if !CanTransition(m.State, to) {
		return fmt.Errorf("transition interdite : %s -> %s (mission %s)", m.State, to, m.ID)
	}
	from := m.State
	now := nowISO()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE missions SET state = ?, updated_at = ? WHERE id = ? AND state = ?`, to, now, m.ID, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("transition %s -> %s annulée : la mission %s a changé d'état concurremment (%s constaté)", from, to, m.ID, m.State)
	}
	if _, err := tx.Exec(`INSERT INTO transitions (mission_id, from_state, to_state, actor, detail, at) VALUES (?,?,?,?,?,?)`,
		m.ID, from, to, actor, detail, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.State = to
	return nil
}

// BumpCorrections incrémente le compteur de corrections demandées.
func (s *Store) BumpCorrections(m *Mission) (int, error) {
	if _, err := s.db.Exec(`UPDATE missions SET corrections = corrections + 1, updated_at = ? WHERE id = ?`, nowISO(), m.ID); err != nil {
		return 0, err
	}
	m.Corrections++
	return m.Corrections, nil
}

// ClaimMission tente de prendre le verrou d'exclusivité d'exécution d'une
// mission (atomique, persistant). Retourne false si une exécution est déjà
// en cours : aucun second passage Mistral, ni seconde livraison, n'est
// possible pour une même mission (T02-R1/R3).
func (s *Store) ClaimMission(id string) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO mission_locks (mission_id, claimed_at) VALUES (?,?)`, id, nowNano())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ReleaseClaim relâche le verrou d'exécution d'une mission (appelé à la fin
// de chaque exécution, quel que soit l'issue).
func (s *Store) ReleaseClaim(id string) error {
	_, err := s.db.Exec(`DELETE FROM mission_locks WHERE mission_id = ?`, id)
	return err
}

// ReclaimStaleClaim reprend un verrou abandonné (crash du coordinateur pendant
// l'exécution) si son âge dépasse l'intervalle de péremption. Un verrou
// récent est respecté : une exécution vivante n'est jamais doublée.
// T02-R2 : le verrou périmé est supprimé, et non actualisé, afin que le
// ClaimMission suivant puisse réinsérer la ligne (INSERT OR IGNORE) ; la
// version antérieure laissait la ligne en place et bloquait la mission.
// Les horodatages des verrous sont homogènes (nowNano, RFC3339Nano).
func (s *Store) ReclaimStaleClaim(id string, maxAge time.Duration) (bool, error) {
	cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339Nano)
	res, err := s.db.Exec(`DELETE FROM mission_locks WHERE mission_id = ? AND claimed_at < ?`, id, cutoff)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
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

// nowNano fournit un horodatage haute précision (verrous d'exclusivité).
func nowNano() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// AgentLog est une entrée du journal des agents (lecture seule, recette T02).
type AgentLog struct {
	ID        int64
	MissionID string
	Agent     string
	Phase     string
	Message   string
	At        string
}

// Journal retourne les entrées du journal des agents, chronologiques. Un
// identifiant de mission vide retourne l'ensemble du journal (lecture seule :
// utilisé par l'utilitaire de recette aic-journal).
func (s *Store) Journal(missionID string) ([]AgentLog, error) {
	q := `SELECT id, mission_id, agent, phase, message, at FROM agent_logs`
	args := []any{}
	if missionID != "" {
		q += ` WHERE mission_id = ?`
		args = append(args, missionID)
	}
	q += ` ORDER BY id ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentLog
	for rows.Next() {
		var l AgentLog
		if err := rows.Scan(&l.ID, &l.MissionID, &l.Agent, &l.Phase, &l.Message, &l.At); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

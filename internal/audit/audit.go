// Package audit implements the Xeme OS Audit Trail.
//
// Every action the system takes is logged here: searches, enrichments,
// scores, campaign steps, sends, replies. This is the compliance and
// observability backbone.
package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is a single audit event.
type Entry struct {
	ID        int64         `json:"id"`
	Actor     string        `json:"actor"`      // "system", "user", "agent", "mcp"
	Action    string        `json:"action"`     // search, enrich, score, enroll, send, reply, etc.
	Target    string        `json:"target"`     // email, company, sequence_id, etc.
	Channel   string        `json:"channel"`    // email, linkedin, twitter, sms, etc.
	Details   string        `json:"details"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Gated     bool          `json:"gated"`      // true if this action required human approval
	Approved  bool          `json:"approved"`   // true if a gated action was approved
	Tokens    float64       `json:"tokens"`     // API tokens consumed
	CreatedAt time.Time     `json:"created_at"`
}

// Store is the SQLite-backed audit store.
type Store struct {
	DB *sql.DB
	mu sync.RWMutex
}

// Open creates or opens an audit store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.Exec("PRAGMA journal_mode=WAL")
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT,
		action TEXT,
		target TEXT,
		channel TEXT,
		details TEXT,
		metadata TEXT,
		gated BOOLEAN DEFAULT FALSE,
		approved BOOLEAN DEFAULT FALSE,
		tokens REAL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}
	s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action)`)
	s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_target ON audit_log(target)`)
	s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at)`)
	return nil
}

// Close releases the DB.
func (s *Store) Close() error { return s.DB.Close() }

// Log records an audit entry.
func (s *Store) Log(entry Entry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	_, err := s.DB.Exec(`INSERT INTO audit_log (actor, action, target, channel, details, metadata, gated, approved, tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Actor, entry.Action, entry.Target, entry.Channel, entry.Details,
		string(metaJSON), entry.Gated, entry.Approved, entry.Tokens)
	return err
}

// LogSimple is a convenience for common audit entries.
func (s *Store) LogSimple(actor, action, target, channel, details string) error {
	return s.Log(Entry{Actor: actor, Action: action, Target: target, Channel: channel, Details: details})
}

// Recent returns the last N audit entries.
func (s *Store) Recent(limit int) ([]Entry, error) {
	rows, err := s.DB.Query(`SELECT id, actor, action, target, channel, details, metadata, gated, approved, tokens, created_at
		FROM audit_log ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// ByAction returns audit entries filtered by action type.
func (s *Store) ByAction(action string, limit int) ([]Entry, error) {
	rows, err := s.DB.Query(`SELECT id, actor, action, target, channel, details, metadata, gated, approved, tokens, created_at
		FROM audit_log WHERE action = ? ORDER BY created_at DESC LIMIT ?`, action, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// ByTarget returns audit entries for a specific target (e.g. an email).
func (s *Store) ByTarget(target string, limit int) ([]Entry, error) {
	rows, err := s.DB.Query(`SELECT id, actor, action, target, channel, details, metadata, gated, approved, tokens, created_at
		FROM audit_log WHERE target = ? ORDER BY created_at DESC LIMIT ?`, target, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// Summary returns counts by action type.
func (s *Store) Summary() (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT action, COUNT(*) FROM audit_log GROUP BY action ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var action string
		var count int
		if err := rows.Scan(&action, &count); err != nil {
			continue
		}
		out[action] = count
	}
	return out, nil
}

// TokensUsed returns total tokens consumed in a time range.
func (s *Store) TokensUsed(since time.Time) (float64, error) {
	var total float64
	err := s.DB.QueryRow(`SELECT COALESCE(SUM(tokens), 0) FROM audit_log WHERE created_at >= ?`, since).Scan(&total)
	return total, err
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		var metaJSON string
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.Channel,
			&e.Details, &metaJSON, &e.Gated, &e.Approved, &e.Tokens, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaJSON), &e.Metadata)
		out = append(out, e)
	}
	return out, nil
}

// GatedAction represents a pending human-approval gate.
type GatedAction struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Details   string    `json:"details"`
	Status    string    `json:"status"` // pending, approved, rejected
	CreatedAt time.Time `json:"created_at"`
}

// Gate checks if an action requires human approval.
// Returns true if the action is allowed, false if it needs approval.
func (s *Store) Gate(action, target string, sensitiveActions []string) bool {
	for _, sa := range sensitiveActions {
		if action == sa {
			return false // needs approval
		}
	}
	return true
}

// FormatEntry formats an audit entry for display.
func FormatEntry(e Entry) string {
 gated := ""
 if e.Gated {
  gated = " [GATED]"
 }
 approved := ""
 if e.Approved {
  approved = " [APPROVED]"
 }
 return fmt.Sprintf("%s  %s  %s → %s  %s%s%s",
  e.CreatedAt.Format("2006-01-02 15:04:05"),
  e.Actor, e.Action, e.Target, e.Channel, gated, approved)
}

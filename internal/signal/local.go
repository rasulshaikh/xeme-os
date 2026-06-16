// Package signal — Xeme Signal Engine with local signal store.
//
// Captures engagement events from any source (web search, CSV import, manual)
// and persists them in a local SQLite store. Provides time-series queries
// and CRUD operations over signals.
package signal

import (
	"database/sql"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Local is the local signal store.
type Local struct {
	DB *sql.DB
	mu sync.RWMutex
}

type Signal struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`     // web-search, linkedin, g2, manual
	Kind      string    `json:"kind"`       // comment, like, post, g2-review, job-change
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	Company   string    `json:"company"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func Open(path string) (*Local, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Local{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Local) migrate() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS signals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT,
		kind TEXT,
		author TEXT,
		url TEXT,
		company TEXT,
		title TEXT,
		body TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (s *Local) Add(sig Signal) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sig.CreatedAt.IsZero() {
		sig.CreatedAt = time.Now()
	}
	res, err := s.DB.Exec(`INSERT INTO signals (source, kind, author, url, company, title, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Source, sig.Kind, sig.Author, sig.URL, sig.Company, sig.Title, sig.Body, sig.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Local) Recent(limit int) ([]Signal, error) {
	if limit == 0 {
		limit = 50
	}
	rows, err := s.DB.Query(`SELECT id, source, kind, author, url, company, title, body, created_at FROM signals ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Signal
	for rows.Next() {
		var sig Signal
		if err := rows.Scan(&sig.ID, &sig.Source, &sig.Kind, &sig.Author, &sig.URL, &sig.Company, &sig.Title, &sig.Body, &sig.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sig)
	}
	return out, nil
}

func (s *Local) ByCompany(company string) ([]Signal, error) {
	rows, err := s.DB.Query(`SELECT id, source, kind, author, url, company, title, body, created_at FROM signals WHERE company LIKE ? ORDER BY created_at DESC`, "%"+company+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Signal
	for rows.Next() {
		var sig Signal
		if err := rows.Scan(&sig.ID, &sig.Source, &sig.Kind, &sig.Author, &sig.URL, &sig.Company, &sig.Title, &sig.Body, &sig.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sig)
	}
	return out, nil
}

func (s *Local) Count() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM signals`).Scan(&n)
	return n, err
}

func (s *Local) Close() error { return s.DB.Close() }

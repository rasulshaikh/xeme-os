// Package leads implements the Unified Leads Database for Xeme OS.
//
// SQLite-backed store that holds every lead across all pipeline stages:
// discovered → enriched → scored → enrolled → contacted → replied → meeting_booked.
//
// This is the single source of truth that ties together the discover, enrich,
// score, and sequence stages.
package leads

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Lead represents a person/company record in the pipeline.
type Lead struct {
	ID             int64     `json:"id"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	FullName       string    `json:"full_name"`
	Title          string    `json:"title"`
	Company        string    `json:"company"`
	Domain         string    `json:"domain"`
	Industry       string    `json:"industry"`
	Location       string    `json:"location"`
	LinkedInURL    string    `json:"linkedin_url"`
	TwitterHandle  string    `json:"twitter_handle"`
	Revenue        string    `json:"revenue_range"`
	EmployeeRange  string    `json:"employee_range"`
	Score          int       `json:"score"`
	Tier           string    `json:"tier"`
	Stage          string    `json:"stage"` // discovered, enriched, scored, enrolled, contacted, replied, meeting_booked, lost
	Source         string    `json:"source"` // moltsets_search, signal, scrape, import, api
	EnrichSource   string    `json:"enrich_source"`
	Confidence     float64   `json:"confidence"`
	SequenceID     string    `json:"sequence_id,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	CustomFields   map[string]string `json:"custom_fields,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Store is the SQLite-backed leads database.
type Store struct {
	DB *sql.DB
	mu sync.RWMutex
}

// Open creates or opens a leads store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS leads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT,
			phone TEXT,
			first_name TEXT,
			last_name TEXT,
			full_name TEXT,
			title TEXT,
			company TEXT,
			domain TEXT,
			industry TEXT,
			location TEXT,
			linkedin_url TEXT,
			twitter_handle TEXT,
			revenue_range TEXT,
			employee_range TEXT,
			score INTEGER DEFAULT 0,
			tier TEXT,
			stage TEXT DEFAULT 'discovered',
			source TEXT,
			enrich_source TEXT,
			confidence REAL DEFAULT 0,
			sequence_id TEXT,
			tags TEXT,
			custom_fields TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_email ON leads(email)`,
		`CREATE INDEX IF NOT EXISTS idx_leads_stage ON leads(stage)`,
		`CREATE INDEX IF NOT EXISTS idx_leads_tier ON leads(tier)`,
		`CREATE INDEX IF NOT EXISTS idx_leads_company ON leads(company)`,
		`CREATE INDEX IF NOT EXISTS idx_leads_score ON leads(score)`,
		`CREATE INDEX IF NOT EXISTS idx_leads_source ON leads(source)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("leads migrate: %w", err)
		}
	}
	return nil
}

// Close releases the DB.
func (s *Store) Close() error { return s.DB.Close() }

// Upsert inserts or updates a lead (by email).
func (s *Store) Upsert(lead Lead) (int64, error) {
	tagsJSON, _ := json.Marshal(lead.Tags)
	customJSON, _ := json.Marshal(lead.CustomFields)

	// Try insert first
	res, err := s.DB.Exec(`INSERT INTO leads
		(email, phone, first_name, last_name, full_name, title, company, domain,
		 industry, location, linkedin_url, twitter_handle, revenue_range, employee_range,
		 score, tier, stage, source, enrich_source, confidence, sequence_id,
		 tags, custom_fields, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			phone = COALESCE(excluded.phone, leads.phone),
			first_name = COALESCE(excluded.first_name, leads.first_name),
			last_name = COALESCE(excluded.last_name, leads.last_name),
			full_name = COALESCE(excluded.full_name, leads.full_name),
			title = COALESCE(excluded.title, leads.title),
			company = COALESCE(excluded.company, leads.company),
			domain = COALESCE(excluded.domain, leads.domain),
			industry = COALESCE(excluded.industry, leads.industry),
			location = COALESCE(excluded.location, leads.location),
			linkedin_url = COALESCE(excluded.linkedin_url, leads.linkedin_url),
			twitter_handle = COALESCE(excluded.twitter_handle, leads.twitter_handle),
			revenue_range = COALESCE(excluded.revenue_range, leads.revenue_range),
			employee_range = COALESCE(excluded.employee_range, leads.employee_range),
			score = MAX(excluded.score, leads.score),
			tier = COALESCE(excluded.tier, leads.tier),
			stage = excluded.stage,
			enrich_source = COALESCE(excluded.enrich_source, leads.enrich_source),
			confidence = MAX(excluded.confidence, leads.confidence),
			updated_at = excluded.updated_at`,
		lead.Email, lead.Phone, lead.FirstName, lead.LastName, lead.FullName,
		lead.Title, lead.Company, lead.Domain, lead.Industry, lead.Location,
		lead.LinkedInURL, lead.TwitterHandle, lead.Revenue, lead.EmployeeRange,
		lead.Score, lead.Tier, lead.Stage, lead.Source, lead.EnrichSource,
		lead.Confidence, lead.SequenceID, string(tagsJSON), string(customJSON),
		time.Now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// BatchUpsert inserts or updates multiple leads.
func (s *Store) BatchUpsert(leads []Lead) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, lead := range leads {
		tagsJSON, _ := json.Marshal(lead.Tags)
		customJSON, _ := json.Marshal(lead.CustomFields)
		_, err := tx.Exec(`INSERT INTO leads
			(email, phone, first_name, last_name, full_name, title, company, domain,
			 industry, location, linkedin_url, twitter_handle, revenue_range, employee_range,
			 score, tier, stage, source, enrich_source, confidence, tags, custom_fields, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(email) DO UPDATE SET
				phone = COALESCE(excluded.phone, leads.phone),
				title = COALESCE(excluded.title, leads.title),
				company = COALESCE(excluded.company, leads.company),
				linkedin_url = COALESCE(excluded.linkedin_url, leads.linkedin_url),
				twitter_handle = COALESCE(excluded.twitter_handle, leads.twitter_handle),
				score = MAX(excluded.score, leads.score),
				stage = excluded.stage,
				updated_at = excluded.updated_at`,
			lead.Email, lead.Phone, lead.FirstName, lead.LastName, lead.FullName,
			lead.Title, lead.Company, lead.Domain, lead.Industry, lead.Location,
			lead.LinkedInURL, lead.TwitterHandle, lead.Revenue, lead.EmployeeRange,
			lead.Score, lead.Tier, lead.Stage, lead.Source, lead.EnrichSource,
			lead.Confidence, string(tagsJSON), string(customJSON), time.Now())
		if err != nil {
			continue // skip duplicates with errors
		}
		n++
	}
	return n, tx.Commit()
}

// GetLead fetches a lead by email.
func (s *Store) GetLead(email string) (*Lead, error) {
	row := s.DB.QueryRow(`SELECT id, email, phone, first_name, last_name, full_name,
		title, company, domain, industry, location, linkedin_url, twitter_handle,
		revenue_range, employee_range, score, tier, stage, source, enrich_source,
		confidence, sequence_id, tags, custom_fields, created_at, updated_at
		FROM leads WHERE email = ?`, email)
	return scanLead(row)
}

// ListByStage returns leads filtered by stage.
func (s *Store) ListByStage(stage string, limit int) ([]Lead, error) {
	rows, err := s.DB.Query(`SELECT id, email, phone, first_name, last_name, full_name,
		title, company, domain, industry, location, linkedin_url, twitter_handle,
		revenue_range, employee_range, score, tier, stage, source, enrich_source,
		confidence, sequence_id, tags, custom_fields, created_at, updated_at
		FROM leads WHERE stage = ? ORDER BY score DESC LIMIT ?`, stage, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLeads(rows)
}

// ListByTier returns leads filtered by tier.
func (s *Store) ListByTier(tier string, limit int) ([]Lead, error) {
	rows, err := s.DB.Query(`SELECT id, email, phone, first_name, last_name, full_name,
		title, company, domain, industry, location, linkedin_url, twitter_handle,
		revenue_range, employee_range, score, tier, stage, source, enrich_source,
		confidence, sequence_id, tags, custom_fields, created_at, updated_at
		FROM leads WHERE tier = ? ORDER BY score DESC LIMIT ?`, tier, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLeads(rows)
}

// Search does a basic text search across name, company, title, email.
func (s *Store) Search(query string, limit int) ([]Lead, error) {
	like := "%" + query + "%"
	rows, err := s.DB.Query(`SELECT id, email, phone, first_name, last_name, full_name,
		title, company, domain, industry, location, linkedin_url, twitter_handle,
		revenue_range, employee_range, score, tier, stage, source, enrich_source,
		confidence, sequence_id, tags, custom_fields, created_at, updated_at
		FROM leads WHERE
			first_name LIKE ? OR last_name LIKE ? OR full_name LIKE ?
			OR company LIKE ? OR title LIKE ? OR email LIKE ?
		ORDER BY score DESC LIMIT ?`,
		like, like, like, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLeads(rows)
}

// Stats returns pipeline metrics.
func (s *Store) Stats() (map[string]int, error) {
	stages := []string{"discovered", "enriched", "scored", "enrolled", "contacted", "replied", "meeting_booked", "lost"}
	out := make(map[string]int)
	for _, stage := range stages {
		var count int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM leads WHERE stage = ?`, stage).Scan(&count)
		out[stage] = count
	}
	// Total
	var total int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&total)
	out["total"] = total
	return out, nil
}

// UpdateStage changes a lead's pipeline stage.
func (s *Store) UpdateStage(email, stage string) error {
	_, err := s.DB.Exec(`UPDATE leads SET stage = ?, updated_at = ? WHERE email = ?`, stage, time.Now(), email)
	return err
}

// UpdateScore updates a lead's score and tier.
func (s *Store) UpdateScore(email string, score int, tier string) error {
	_, err := s.DB.Exec(`UPDATE leads SET score = ?, tier = ?, stage = 'scored', updated_at = ? WHERE email = ?`,
		score, tier, time.Now(), email)
	return err
}

func scanLead(row *sql.Row) (*Lead, error) {
	var l Lead
	var tagsJSON, customJSON string
	err := row.Scan(&l.ID, &l.Email, &l.Phone, &l.FirstName, &l.LastName, &l.FullName,
		&l.Title, &l.Company, &l.Domain, &l.Industry, &l.Location,
		&l.LinkedInURL, &l.TwitterHandle, &l.Revenue, &l.EmployeeRange,
		&l.Score, &l.Tier, &l.Stage, &l.Source, &l.EnrichSource,
		&l.Confidence, &l.SequenceID, &tagsJSON, &customJSON,
		&l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &l.Tags)
	_ = json.Unmarshal([]byte(customJSON), &l.CustomFields)
	return &l, nil
}

func scanLeads(rows *sql.Rows) ([]Lead, error) {
	var out []Lead
	for rows.Next() {
		var l Lead
		var tagsJSON, customJSON string
		if err := rows.Scan(&l.ID, &l.Email, &l.Phone, &l.FirstName, &l.LastName, &l.FullName,
			&l.Title, &l.Company, &l.Domain, &l.Industry, &l.Location,
			&l.LinkedInURL, &l.TwitterHandle, &l.Revenue, &l.EmployeeRange,
			&l.Score, &l.Tier, &l.Stage, &l.Source, &l.EnrichSource,
			&l.Confidence, &l.SequenceID, &tagsJSON, &customJSON,
			&l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &l.Tags)
		_ = json.Unmarshal([]byte(customJSON), &l.CustomFields)
		out = append(out, l)
	}
	return out, nil
}

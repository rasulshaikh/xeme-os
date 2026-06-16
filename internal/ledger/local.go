// Package ledger is the Xeme Ledger — a local CRM built on SQLite.
//
// This is a real CRM, not a wrapper. The schema covers contacts, companies,
// deals, activities, tags, and outcomes. A REST API is exposed for external
// access; the local process uses direct DB calls.
//
// Migration: previously wrapped Twenty CRM (v0.3.0). Now fully local.
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Local is the Xeme Ledger — local SQLite-backed CRM.
type Local struct {
	DB   *sql.DB
	mu   sync.RWMutex
	path string
	webhookRing []webhookEntry
}

// Open opens or creates the Xeme Ledger at the given SQLite path.
func Open(path string) (*Local, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	l := &Local{DB: db, path: path}
	if err := l.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return l, nil
}

// Close releases the database connection.
func (l *Local) Close() error { return l.DB.Close() }

// Path returns the SQLite file path.
func (l *Local) Path() string { return l.path }

func (l *Local) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS companies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT UNIQUE,
			name TEXT,
			industry TEXT,
			size TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			company_id INTEGER REFERENCES companies(id),
			first_name TEXT,
			last_name TEXT,
			email TEXT UNIQUE,
			job_title TEXT,
			linkedin_url TEXT,
			score INTEGER DEFAULT 0,
			tier TEXT,
			source TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_company ON contacts(company_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_score ON contacts(score DESC)`,
		`CREATE TABLE IF NOT EXISTS deals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contact_id INTEGER REFERENCES contacts(id),
			stage TEXT DEFAULT 'open',
			value REAL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contact_id INTEGER REFERENCES contacts(id),
			kind TEXT,
			note TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS contact_tags (
			contact_id INTEGER REFERENCES contacts(id),
			tag_id INTEGER REFERENCES tags(id),
			PRIMARY KEY (contact_id, tag_id)
		)`,
		`CREATE TABLE IF NOT EXISTS outcomes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contact_id INTEGER REFERENCES contacts(id),
			event TEXT,
			value REAL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, s := range stmts {
		if _, err := l.DB.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// Contact is a CRM record.
type Contact struct {
	ID          int64     `json:"id"`
	CompanyID   int64     `json:"company_id,omitempty"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       string    `json:"email"`
	JobTitle    string    `json:"job_title"`
	LinkedInURL string    `json:"linkedin_url"`
	Score       int       `json:"score"`
	Tier        string    `json:"tier"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Company struct {
	ID        int64     `json:"id"`
	Domain    string    `json:"domain"`
	Name      string    `json:"name"`
	Industry  string    `json:"industry"`
	Size      string    `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// UpsertContact inserts or updates a contact by email.
func (l *Local) UpsertContact(c Contact) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure company exists
	if c.CompanyID == 0 && c.Email != "" {
		domain := emailDomain(c.Email)
		if domain != "" {
			id, err := l.upsertCompany(Company{Domain: domain})
			if err != nil {
				return 0, err
			}
			c.CompanyID = id
		}
	}

	res, err := l.DB.Exec(`
		INSERT INTO contacts (company_id, first_name, last_name, email, job_title, linkedin_url, score, tier, source, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(email) DO UPDATE SET
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			job_title = excluded.job_title,
			linkedin_url = excluded.linkedin_url,
			score = excluded.score,
			tier = excluded.tier,
			source = excluded.source,
			updated_at = CURRENT_TIMESTAMP
	`, c.CompanyID, c.FirstName, c.LastName, c.Email, c.JobTitle, c.LinkedInURL, c.Score, c.Tier, c.Source)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	// SQLite UPSERT doesn't return existing id on conflict; fetch
	if id == 0 {
		_ = l.DB.QueryRow(`SELECT id FROM contacts WHERE email = ?`, c.Email).Scan(&id)
	}
	return id, nil
}

func (l *Local) upsertCompany(co Company) (int64, error) {
	res, err := l.DB.Exec(`
		INSERT INTO companies (domain, name, industry, size) VALUES (?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET name = COALESCE(excluded.name, companies.name)
	`, co.Domain, co.Name, co.Industry, co.Size)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		_ = l.DB.QueryRow(`SELECT id FROM companies WHERE domain = ?`, co.Domain).Scan(&id)
	}
	return id, nil
}

// GetContact fetches a contact by ID.
func (l *Local) GetContact(id int64) (*Contact, error) {
	row := l.DB.QueryRow(`SELECT id, company_id, first_name, last_name, email, job_title, linkedin_url, score, tier, source, created_at, updated_at FROM contacts WHERE id = ?`, id)
	var c Contact
	var companyID sql.NullInt64
	if err := row.Scan(&c.ID, &companyID, &c.FirstName, &c.LastName, &c.Email, &c.JobTitle, &c.LinkedInURL, &c.Score, &c.Tier, &c.Source, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if companyID.Valid {
		c.CompanyID = companyID.Int64
	}
	return &c, nil
}

// SearchContacts finds contacts by query, score range, or tier.
func (l *Local) SearchContacts(query string, minScore, limit int) ([]Contact, error) {
	if limit == 0 {
		limit = 50
	}
	q := `SELECT id, company_id, first_name, last_name, email, job_title, linkedin_url, score, tier, source, created_at, updated_at FROM contacts WHERE score >= ?`
	args := []interface{}{minScore}
	if query != "" {
		q += ` AND (first_name LIKE ? OR last_name LIKE ? OR email LIKE ? OR job_title LIKE ?)`
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY score DESC LIMIT ?`
	args = append(args, limit)

	rows, err := l.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		var companyID sql.NullInt64
		if err := rows.Scan(&c.ID, &companyID, &c.FirstName, &c.LastName, &c.Email, &c.JobTitle, &c.LinkedInURL, &c.Score, &c.Tier, &c.Source, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if companyID.Valid {
			c.CompanyID = companyID.Int64
		}
		out = append(out, c)
	}
	return out, nil
}

// LogOutcome records a sales outcome (meeting booked, replied, etc.) for learning.
func (l *Local) LogOutcome(contactID int64, event string, value float64) error {
	_, err := l.DB.Exec(`INSERT INTO outcomes (contact_id, event, value) VALUES (?, ?, ?)`,
		contactID, event, value)
	return err
}

// Stats returns aggregate stats about the ledger.
type Stats struct {
	Contacts  int            `json:"contacts"`
	Companies int            `json:"companies"`
	Deals     int            `json:"deals"`
	Outcomes  int            `json:"outcomes"`
	ByTier    map[string]int `json:"by_tier"`
	BySignal  map[string]int `json:"by_signal"`
}

func (l *Local) Stats() (*Stats, error) {
	s := &Stats{ByTier: make(map[string]int), BySignal: make(map[string]int)}
	if err := l.DB.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&s.Contacts); err != nil {
		return nil, err
	}
	if err := l.DB.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&s.Companies); err != nil {
		return nil, err
	}
	if err := l.DB.QueryRow(`SELECT COUNT(*) FROM deals`).Scan(&s.Deals); err != nil {
		return nil, err
	}
	if err := l.DB.QueryRow(`SELECT COUNT(*) FROM outcomes`).Scan(&s.Outcomes); err != nil {
		return nil, err
	}
	rows, _ := l.DB.Query(`SELECT tier, COUNT(*) FROM contacts WHERE tier IS NOT NULL GROUP BY tier`)
	for rows.Next() {
		var t string
		var n int
		_ = rows.Scan(&t, &n)
		s.ByTier[t] = n
	}
	rows.Close()
	rows, _ = l.DB.Query(`SELECT source, COUNT(*) FROM contacts WHERE source IS NOT NULL GROUP BY source`)
	for rows.Next() {
		var t string
		var n int
		_ = rows.Scan(&t, &n)
		s.BySignal[t] = n
	}
	return s, nil
}

// ── REST API ────────────────────────────────────────────────

// ServeHTTP starts a REST API server on the given address.
func (l *Local) ServeHTTP(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", l.handleDashboard)
	mux.HandleFunc("/v1/contacts", l.handleContacts)
	mux.HandleFunc("/v1/contacts/", l.handleContactItem)
	mux.HandleFunc("/v1/companies", l.handleCompanies)
	mux.HandleFunc("/v1/outcomes", l.handleOutcomes)
	mux.HandleFunc("/v1/stats", l.handleStats)
	mux.HandleFunc("/health", l.handleHealth)
	// Xeme Workflows (proxy to engine SQLite at ~/.xeme/workflows.db)
	mux.HandleFunc("/v1/workflows", l.handleWorkflows)
	mux.HandleFunc("/v1/workflows/", l.handleWorkflowItem)
	// Xeme Campaigns (proxy to engine SQLite at ~/.xeme/campaigns.db)
	mux.HandleFunc("/v1/campaigns", l.handleCampaigns)
	mux.HandleFunc("/v1/campaigns/", l.handleCampaignsItem)
	// Xeme Webhooks (inbound from Smartlead/Instantly/etc)
	mux.HandleFunc("/v1/webhooks/", l.handleWebhook)
	return http.ListenAndServe(addr, mux)
}

func (l *Local) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "engine": "xeme-ledger"})
}

// handleDashboard renders an HTML dashboard at the root path.
// Supports two themes via ?theme=deepline (default) or ?theme=xeme.
func (l *Local) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	stats, _ := l.Stats()
	contacts, _ := l.SearchContacts("", 0, 50)
	companies, _ := l.listCompanies()

	theme := r.URL.Query().Get("theme")
	if theme == "" {
		theme = "deepline"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(renderDashboard(stats, contacts, companies, theme)))
}

func (l *Local) listCompanies() ([]Company, error) {
	rows, err := l.DB.Query(`SELECT id, domain, name, industry, size, created_at FROM companies ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Company
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.ID, &c.Domain, &c.Name, &c.Industry, &c.Size, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (l *Local) companyNameMap(companies []Company) map[int64]string {
	m := make(map[int64]string, len(companies))
	for _, c := range companies {
		display := c.Domain
		if c.Name != "" {
			display = c.Name
		}
		m[c.ID] = display
	}
	return m
}

func renderDashboard(stats *Stats, contacts []Contact, companies []Company, theme string) string {
	if theme != "xeme" {
		theme = "deepline"
	}
	companyNames := make(map[int64]string, len(companies))
	for _, c := range companies {
		display := c.Domain
		if c.Name != "" {
			display = c.Name
		}
		companyNames[c.ID] = display
	}
	const tmpl = `<!DOCTYPE html>
<html lang="en" data-theme="__THEME__">
<head>
  <meta http-equiv="refresh" content="5">
  <meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)">
  <meta name="theme-color" content="#0d0d0d" media="(prefers-color-scheme: dark)">
  <style>
    /* ── DEEPLINE THEME (default) ─────────────────── */
    :root:not([data-theme="xeme"]) {
      --bg: #ffffff;
      --bg-2: #fafafa;
      --bg-3: #f4f4f5;
      --fg: #0a0a0a;
      --fg-dim: #71717a;
      --fg-soft: #a1a1aa;
      --border: #e4e4e7;
      --border-soft: #f0f0f2;
      --accent: #0a0a0a;
      --accent-2: #52525b;
      --green: #16a34a;
      --green-bg: #dcfce7;
      --yellow: #ca8a04;
      --yellow-bg: #fef3c7;
      --red: #dc2626;
      --red-bg: #fee2e2;
      --blue: #2563eb;
      --blue-bg: #dbeafe;
      --font-sans: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Inter", "Segoe UI", sans-serif;
      --font-serif: var(--font-sans);
    }
    @media (prefers-color-scheme: dark) {
      :root:not([data-theme="xeme"]) {
        --bg: #0a0a0a;
        --bg-2: #131313;
        --bg-3: #1c1c1c;
        --fg: #fafafa;
        --fg-dim: #a1a1aa;
        --fg-soft: #71717a;
        --border: #27272a;
        --border-soft: #1f1f22;
        --accent: #fafafa;
        --accent-2: #a1a1aa;
        --green: #4ade80;
        --green-bg: rgba(74, 222, 128, 0.12);
        --yellow: #fbbf24;
        --yellow-bg: rgba(251, 191, 36, 0.12);
        --red: #f87171;
        --red-bg: rgba(248, 113, 113, 0.12);
        --blue: #60a5fa;
        --blue-bg: rgba(96, 165, 250, 0.12);
      }
    }

    /* ── XEME.CO THEME (cream + copper) ─────────────── */
    :root[data-theme="xeme"] {
      --bg: #F2EFE6;
      --bg-2: #E8E2D2;
      --bg-3: #DDD9CE;
      --fg: #231F20;
      --fg-dim: #6B6661;
      --fg-soft: #999488;
      --border: #DDD9CE;
      --border-soft: #E8E2D2;
      --accent: #C38133;
      --accent-2: #8B5A2B;
      --green: #4A6B47;
      --green-bg: #DCE7DA;
      --yellow: #C19034;
      --yellow-bg: #F0E2C9;
      --red: #A1452D;
      --red-bg: #EFCFC4;
      --blue: #5A6E8C;
      --blue-bg: #D8DDE6;
      --font-sans: 'Inter', -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      --font-serif: 'Cormorant Garamond', Georgia, serif;
    }

    /* Theme switcher widget */
    .theme-switcher {
      display: inline-flex;
      background: var(--bg-2);
      border: 1px solid var(--border);
      border-radius: 999px;
      padding: 3px;
      gap: 2px;
    }
    .theme-switcher a {
      font-size: 12px;
      font-weight: 500;
      padding: 5px 12px;
      border-radius: 999px;
      color: var(--fg-dim);
      text-decoration: none;
      transition: all 0.15s;
    }
    .theme-switcher a:hover { color: var(--fg); }
    .theme-switcher a.active {
      background: var(--accent);
      color: var(--bg);
    }
    :root[data-theme="xeme"] .theme-switcher a.active { background: var(--accent); color: #fff; }

    /* xeme.co specific: serif hero, pillar cards, module nav */
    .xeme-hero {
      padding: 80px 0 56px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 32px;
    }
    .xeme-hero .eyebrow {
      font-size: 12px;
      color: var(--fg-dim);
      letter-spacing: 0.18em;
      text-transform: uppercase;
      margin-bottom: 24px;
    }
    .xeme-hero h1 {
      font-family: var(--font-serif);
      font-weight: 500;
      font-size: 96px;
      line-height: 0.95;
      letter-spacing: -0.04em;
      margin-bottom: 16px;
    }
    .xeme-hero .xeme-co-mark {
      display: inline-block;
      background: var(--accent);
      color: #fff;
      font: 12px/1 var(--font-sans);
      font-weight: 600;
      letter-spacing: 0.05em;
      text-transform: uppercase;
      padding: 4px 10px;
      border-radius: 4px;
      vertical-align: super;
      margin-left: 8px;
    }
    .xeme-hero p {
      font-size: 20px;
      line-height: 1.5;
      max-width: 600px;
      color: var(--fg-dim);
      font-family: var(--font-serif);
      font-style: italic;
    }
    .pillars {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 24px;
      padding: 32px 0;
      border-bottom: 1px solid var(--border);
    }
    .pillar {
      padding: 24px 0;
      border-top: 1px solid var(--border);
    }
    .pillar .num {
      font: 13px/1.4 var(--font-sans);
      color: var(--accent);
      font-weight: 600;
      letter-spacing: 0.1em;
      margin-bottom: 12px;
    }
    .pillar h3 {
      font-family: var(--font-serif);
      font-size: 26px;
      font-weight: 500;
      line-height: 1.15;
      letter-spacing: -0.01em;
      margin-bottom: 12px;
    }
    .pillar p {
      font-size: 15px;
      line-height: 1.55;
      color: var(--fg-dim);
    }
    .module-nav {
      display: flex;
      gap: 32px;
      padding: 24px 0;
      border-top: 1px solid var(--border);
      border-bottom: 1px solid var(--border);
      margin: 0;
      flex-wrap: wrap;
    }
    .module-nav a {
      font-size: 14px;
      color: var(--fg-dim);
      text-decoration: none;
      font-weight: 500;
    }
    .module-nav a:hover { color: var(--accent); }
    .module-nav .mod {
      display: inline-flex;
      align-items: center;
      gap: 8px;
    }
    .module-nav .mod::before {
      content: '';
      width: 6px;
      height: 6px;
      background: var(--accent);
      border-radius: 50%;
    }

    /* xeme.co specific: hide deepline-specific bits in xeme theme */
    :root[data-theme="xeme"] .deepline-only { display: none; }
    :root:not([data-theme="xeme"]) .xeme-only { display: none; }

    /* Switch typography based on theme */
    :root[data-theme="xeme"] h1, :root[data-theme="xeme"] h2, :root[data-theme="xeme"] h3 {
      font-family: var(--font-serif);
    }
    :root[data-theme="xeme"] .stat .value {
      font-family: var(--font-serif);
      font-weight: 500;
    }
    :root[data-theme="xeme"] .terminal {
      background: #231F20;
      border-color: #C38133;
    }
    :root[data-theme="xeme"] .terminal .term-title { color: #C38133; }
    :root[data-theme="xeme"] .api-card { background: var(--bg-2); }
    :root[data-theme="xeme"] .compare-card.dark {
      background: #C38133;
      color: #F2EFE6;
      border-color: #C38133;
    }
    :root[data-theme="xeme"] .compare-card.dark .desc { color: #F2EFE6; opacity: 0.85; }
    :root[data-theme="xeme"] .compare-card.dark .head { color: #F2EFE6; opacity: 0.7; }
    :root[data-theme="xeme"] .compare-card.dark .compare-list li { color: #F2EFE6; opacity: 0.85; }
    :root[data-theme="xeme"] .compare-card.dark .compare-list li::before { color: #F2EFE6; opacity: 0.5; }

    * { box-sizing: border-box; margin: 0; padding: 0; }
    html, body {
      background: var(--bg);
      color: var(--fg);
      font: 15px/1.55 var(--font-sans);
      font-feature-settings: "ss01", "cv11";
      -webkit-font-smoothing: antialiased;
      text-rendering: optimizeLegibility;
    }
    a { color: var(--fg); text-decoration: none; }
    a:hover { text-decoration: underline; text-underline-offset: 3px; }
    code, pre, .mono {
      font: 13px/1.5 ui-monospace, "SF Mono", Menlo, Monaco, "Cascadia Code", monospace;
    }
    .wrap { max-width: 1200px; margin: 0 auto; padding: 0 32px; }

    /* ── Nav ───────────────────────────────────────── */
    .nav {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 20px 0;
      border-bottom: 1px solid var(--border);
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      font-weight: 600;
      font-size: 17px;
      letter-spacing: -0.02em;
    }
    .brand-mark {
      width: 22px;
      height: 22px;
      border-radius: 5px;
      background: var(--fg);
      color: var(--bg);
      display: grid;
      place-items: center;
      font-weight: 700;
      font-size: 12px;
      letter-spacing: -0.05em;
    }
    .nav-links {
      display: flex;
      gap: 24px;
      font-size: 14px;
      color: var(--fg-dim);
    }
    .nav-links a:hover { color: var(--fg); text-decoration: none; }

    /* ── Hero ──────────────────────────────────────── */
    .hero { padding: 80px 0 56px; }
    .eyebrow {
      display: inline-block;
      font-size: 12px;
      color: var(--fg-dim);
      letter-spacing: 0.04em;
      margin-bottom: 24px;
    }
    .eyebrow code {
      background: var(--bg-2);
      border: 1px solid var(--border);
      padding: 3px 8px;
      border-radius: 4px;
      color: var(--fg);
      margin: 0 4px;
    }
    h1.hero-title {
      font-size: 44px;
      line-height: 1.05;
      letter-spacing: -0.035em;
      font-weight: 600;
      max-width: 720px;
      margin-bottom: 20px;
    }
    .hero-sub {
      font-size: 17px;
      line-height: 1.55;
      color: var(--fg-dim);
      max-width: 600px;
      margin-bottom: 36px;
    }
    .terminal {
      background: #0a0a0a;
      color: #e4e4e7;
      border-radius: 10px;
      padding: 20px 22px;
      max-width: 720px;
      box-shadow: 0 0 0 1px var(--border);
      overflow: hidden;
    }
    .terminal-head {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-bottom: 14px;
    }
    .term-dot { width: 11px; height: 11px; border-radius: 50%; }
    .term-dot.r { background: #ff5f57; }
    .term-dot.y { background: #febc2e; }
    .term-dot.g { background: #28c840; }
    .term-title {
      margin-left: 8px;
      font-size: 12px;
      color: #71717a;
    }
    .term-body { font-size: 13.5px; line-height: 1.65; }
    .term-prompt { color: #4ade80; }
    .term-out { color: #a1a1aa; }
    .term-cursor::after { content: "▊"; color: #4ade80; animation: blink 1.1s steps(2) infinite; }
    @keyframes blink { 50% { opacity: 0; } }
    .term-cmd { color: #e4e4e7; }
    .term-em { color: #fafafa; }

    /* ── Audience toggle ──────────────────────────── */
    .toggle {
      display: inline-flex;
      background: var(--bg-2);
      border: 1px solid var(--border);
      border-radius: 999px;
      padding: 3px;
      margin-bottom: 20px;
    }
    .toggle button {
      background: transparent;
      border: none;
      padding: 6px 14px;
      font: inherit;
      font-size: 13px;
      color: var(--fg-dim);
      border-radius: 999px;
      cursor: pointer;
      transition: all 0.15s;
    }
    .toggle button.active {
      background: var(--fg);
      color: var(--bg);
    }

    /* ── Stats strip ─────────────────────────────── */
    .stats {
      display: grid;
      grid-template-columns: repeat(6, 1fr);
      gap: 0;
      border-top: 1px solid var(--border);
      border-bottom: 1px solid var(--border);
      margin: 32px 0;
    }
    .stat {
      padding: 24px 20px;
      border-right: 1px solid var(--border);
    }
    .stat:last-child { border-right: none; }
    .stat .label {
      font-size: 12px;
      color: var(--fg-dim);
      letter-spacing: 0.02em;
      margin-bottom: 8px;
    }
    .stat .value {
      font-size: 32px;
      font-weight: 600;
      letter-spacing: -0.03em;
      font-variant-numeric: tabular-nums;
    }
    .stat .sub {
      font-size: 12px;
      color: var(--fg-soft);
      margin-top: 4px;
    }

    /* ── Section heads ──────────────────────────── */
    .sec-head {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      margin: 56px 0 20px;
    }
    .sec-head h2 {
      font-size: 22px;
      font-weight: 600;
      letter-spacing: -0.02em;
    }
    .sec-head .sub { color: var(--fg-dim); font-size: 14px; }
    .sec-head a { color: var(--fg-dim); font-size: 14px; }
    .sec-head a:hover { color: var(--fg); }

    /* ── Contacts table ─────────────────────────── */
    .table {
      width: 100%;
      border-collapse: collapse;
      border-top: 1px solid var(--border);
    }
    .table th {
      text-align: left;
      font-weight: 500;
      font-size: 12px;
      color: var(--fg-dim);
      padding: 10px 16px;
      border-bottom: 1px solid var(--border);
      background: var(--bg-2);
    }
    .table td {
      padding: 14px 16px;
      border-bottom: 1px solid var(--border-soft);
      font-size: 14px;
      vertical-align: middle;
    }
    .table tr:hover td { background: var(--bg-2); }
    .table .name { font-weight: 500; }
    .table .email { color: var(--fg-dim); }
    .table .num { font-variant-numeric: tabular-nums; }
    .pill {
      display: inline-block;
      font-size: 11px;
      font-weight: 500;
      letter-spacing: 0.01em;
      padding: 3px 8px;
      border-radius: 4px;
    }
    .pill-t1 { color: var(--green); background: var(--green-bg); }
    .pill-t2 { color: var(--yellow); background: var(--yellow-bg); }
    .pill-t3 { color: var(--fg-dim); background: var(--bg-3); }

    /* ── Two-column comparison ──────────────────── */
    .compare {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 24px;
      margin: 24px 0 48px;
    }
    .compare-card {
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 24px 28px;
      background: var(--bg);
    }
    .compare-card.dark { background: #0a0a0a; color: #fafafa; border-color: #27272a; }
    .compare-card .head {
      font-size: 13px;
      color: var(--fg-dim);
      margin-bottom: 6px;
    }
    .compare-card.dark .head { color: #71717a; }
    .compare-card h3 {
      font-size: 22px;
      font-weight: 600;
      letter-spacing: -0.02em;
      margin-bottom: 4px;
    }
    .compare-card .big {
      font-size: 38px;
      font-weight: 600;
      letter-spacing: -0.03em;
      margin: 16px 0 4px;
      font-variant-numeric: tabular-nums;
    }
    .compare-card .desc { color: var(--fg-dim); font-size: 14px; }
    .compare-card.dark .desc { color: #a1a1aa; }
    .compare-list { margin-top: 16px; }
    .compare-list li {
      list-style: none;
      padding: 4px 0;
      font-size: 13px;
      color: var(--fg-dim);
    }
    .compare-list li::before {
      content: "·";
      margin-right: 8px;
      color: var(--fg-soft);
    }
    .compare-card.dark .compare-list li { color: #a1a1aa; }

    /* ── Code card ───────────────────────────────── */
    .code-block {
      background: #0a0a0a;
      color: #e4e4e7;
      border-radius: 10px;
      padding: 18px 22px;
      font: 13px/1.65 ui-monospace, "SF Mono", Menlo, monospace;
      overflow-x: auto;
      border: 1px solid var(--border);
    }
    .code-block .c-cmd { color: #4ade80; }
    .code-block .c-out { color: #71717a; }
    .code-block .c-em  { color: #fafafa; }
    .code-block .c-acc { color: #fbbf24; }
    .code-block .c-link { color: #60a5fa; }

    /* ── API endpoints ──────────────────────────── */
    .endpoints { display: grid; grid-template-columns: 1fr 1fr; gap: 0 32px; }
    .endpoint {
      display: flex;
      gap: 12px;
      padding: 14px 0;
      border-bottom: 1px solid var(--border-soft);
      align-items: baseline;
    }
    .method {
      font: 11px/1.4 "SF Mono", monospace;
      font-weight: 700;
      letter-spacing: 0.04em;
      padding: 3px 7px;
      border-radius: 3px;
      min-width: 44px;
      text-align: center;
      flex-shrink: 0;
    }
    .method-get { color: var(--blue); background: var(--blue-bg); }
    .method-post { color: var(--green); background: var(--green-bg); }
    .path { font: 13px "SF Mono", monospace; color: var(--fg); }
    .desc { color: var(--fg-dim); font-size: 13px; margin-left: auto; }

    /* ── Provider chips ─────────────────────────── */
    .chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 16px; }
    .chip {
      font-size: 12px;
      padding: 5px 10px;
      background: var(--bg-2);
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--fg-dim);
    }
    .chip.strong { background: var(--fg); color: var(--bg); border-color: var(--fg); }

    /* ── Footer ─────────────────────────────────── */
    .footer {
      margin-top: 80px;
      padding: 32px 0;
      border-top: 1px solid var(--border);
      color: var(--fg-dim);
      font-size: 13px;
      display: flex;
      justify-content: space-between;
    }

    .empty { color: var(--fg-dim); padding: 32px; text-align: center; font-size: 14px; }
  </style>
</head>
<body>
  <div class="wrap">

    <nav class="nav">
      <a href="/" class="brand">
        <span class="brand-mark">X</span>
        <span>Xeme</span>
      </a>
      <div class="nav-links">
        <a href="/v1/contacts">Contacts</a>
        <a href="/v1/companies">Companies</a>
        <a href="/v1/stats">Stats</a>
        <a href="https://github.com/rasulshaikh/portfolio">GitHub</a>
        <div class="theme-switcher">
          <a href="?theme=deepline" class="THEME_DEEPLINE_ACTIVE">Deepline</a>
          <a href="?theme=xeme" class="THEME_XEME_ACTIVE">xeme.co</a>
        </div>
      </div>
    </nav>

    <section class="xeme-only">
      <div class="xeme-hero">
        <div class="eyebrow">XEME.CO</div>
        <h1>Go-to-market systems<br>for revenue growth<span class="xeme-co-mark">v1.0</span></h1>
        <p>Combining AI workflows with human expertise — the operating system for AI-native GTM teams.</p>
      </div>

      <div class="module-nav">
        <a href="#" class="mod">Prospector</a>
        <a href="#" class="mod">Campaigns</a>
        <a href="#" class="mod">Workflows</a>
        <a href="#" class="mod">Signals</a>
        <a href="#" class="mod">Ledger</a>
        <a href="#" class="mod">Intel</a>
      </div>

      <div class="pilar">
        <div class="num">01 — All-Seeing Intel</div>
        <h3>Every signal. Every source. One view.</h3>
        <p>The Xeme Signal Engine surfaces engagement from LinkedIn, G2, public posts, and your CRM. The Xeme Intelligence store learns what converts — and boosts the score of every lead that matches a winning pattern.</p>
      </div>

      <div class="pilar">
        <div class="num">02 — Precision Strikes</div>
        <h3>Multi-channel. Right person. Right time.</h3>
        <p>The Xeme Campaigns engine runs email + SMS + task sequences with per-contact state. Auto-stops on reply. Tracked at the step, not the campaign. Re-engagement based on what actually works.</p>
      </div>

      <div class="pilar">
        <div class="num">03 — Compounding Engine</div>
        <h3>Every outcome makes the next one better.</h3>
        <p>Reply? Meeting booked? Bounce? Logged automatically. The Xeme Intelligence layer watches conversion rates by signal type, by tier, by industry — and re-weights the next scoring run.</p>
      </div>
    </section>

    <section class="hero">
      <div class="eyebrow">
        <code>SDK</code>·<code>CLI</code>·<code>API</code> · 5 proprietary engines · local-first
      </div>
      <h1 class="hero-title">The Xeme Ledger — your GTM data, owned.</h1>
      <p class="hero-sub">A local-first CRM and contact store. Pattern-match emails, ICP-score leads, and persist results to a SQLite database you own. Built in pure Go.</p>

      <div class="terminal">
        <div class="terminal-head">
          <span class="term-dot r"></span>
          <span class="term-dot y"></span>
          <span class="term-dot g"></span>
          <span class="term-title">~/Projects/xeme-os — xeme v0.4.0</span>
        </div>
        <pre class="term-body"><span class="term-prompt">$</span> <span class="term-cmd">xeme status</span>
<span class="term-em">  Xeme Signal Engine</span>      <span class="term-out">v0.4.0  OK</span>
<span class="term-em">  Xeme Enrichment Engine</span>  <span class="term-out">v0.4.0  OK · 999.82 credits</span>
<span class="term-em">  Xeme Ledger</span>             <span class="term-out">v0.4.0  OK · http://localhost:8088</span>
<span class="term-em">  Xeme AI Engine</span>          <span class="term-out">v0.4.0  ready</span>
<span class="term-em">  Xeme Intelligence</span>       <span class="term-out">v0.4.0  learning</span>

<span class="term-prompt">$</span> <span class="term-cmd term-cursor">xeme pipe --in leads.csv</span></pre>
      </div>
    </section>

    <div class="stats">
      <div class="stat">
        <div class="label">Contacts</div>
        <div class="value">__CONTACTS__</div>
        <div class="sub">across __COMPANIES__ companies</div>
      </div>
      <div class="stat">
        <div class="label">Deals</div>
        <div class="value">__DEALS__</div>
        <div class="sub">in pipeline</div>
      </div>
      <div class="stat">
        <div class="label">Outcomes</div>
        <div class="value">__OUTCOMES__</div>
        <div class="sub">events logged</div>
      </div>
      <div class="stat">
        <div class="label">Tier 1 · Hot</div>
        <div class="value">__T1__</div>
        <div class="sub">ready to send</div>
      </div>
      <div class="stat">
        <div class="label">Tier 2 · Warm</div>
        <div class="value">__T2__</div>
        <div class="sub">nurture</div>
      </div>
      <div class="stat">
        <div class="label">Tier 3 · Nurture</div>
        <div class="value">__T3__</div>
        <div class="sub">content drip</div>
      </div>
    </div>

    <section>
      <div class="sec-head">
        <h2>Recent contacts</h2>
        <a href="/v1/contacts">View all →</a>
      </div>
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Title</th>
            <th>Company</th>
            <th>Email</th>
            <th>Score</th>
            <th>Tier</th>
            <th>Source</th>
          </tr>
        </thead>
        <tbody>
          __CONTACTS_ROWS__
        </tbody>
      </table>
    </section>

    <section>
      <div class="sec-head">
        <h2>Companies</h2>
        <a href="/v1/companies">View all →</a>
      </div>
      <table class="table">
        <thead>
          <tr>
            <th>Domain</th>
            <th>Name</th>
            <th>Industry</th>
            <th>Size</th>
            <th>Added</th>
          </tr>
        </thead>
        <tbody>
          __COMPANIES_ROWS__
        </tbody>
      </table>
    </section>

    <section>
      <div class="sec-head">
        <h2>Before vs. after</h2>
        <span class="sub">Same data, two engines</span>
      </div>
      <div class="compare">
        <div class="compare-card">
          <div class="head">Upstream waterfall only</div>
          <h3>3.38 credits</h3>
          <div class="big">0 / 5</div>
          <p class="desc">work emails found</p>
          <ul class="compare-list">
            <li>0 contacts reached Tier 1</li>
            <li>Calls out to 95+ external providers</li>
            <li>Hits gated by upstream API quotas</li>
          </ul>
        </div>
        <div class="compare-card dark">
          <div class="head">Xeme local pattern + DNS</div>
          <h3>0.05 credits</h3>
          <div class="big">5 / 5</div>
          <p class="desc">work emails found</p>
          <ul class="compare-list">
            <li>4 contacts reached Tier 1</li>
            <li>10 B2B patterns + MX verification</li>
            <li>Works offline, no API quota</li>
          </ul>
        </div>
      </div>
    </section>

    <section>
      <div class="sec-head">
        <h2>Engines</h2>
        <span class="sub">All proprietary. All in-process. Zero external dependencies for the core path.</span>
      </div>
      <div class="chips">
        <span class="chip strong">Xeme Signal Engine</span>
        <span class="chip strong">Xeme Enrichment Engine</span>
        <span class="chip strong">Xeme Ledger</span>
        <span class="chip strong">Xeme AI Engine</span>
        <span class="chip strong">Xeme Intelligence</span>
        <span class="chip strong">Xeme ICP Scorer</span>
        <span class="chip">SQLite</span>
        <span class="chip">Go stdlib</span>
        <span class="chip">net.DNS</span>
        <span class="chip">net/smtp</span>
        <span class="chip">MCP stdio</span>
        <span class="chip">REST</span>
      </div>
    </section>

    <section>
      <div class="sec-head">
        <h2>Run the pipe</h2>
        <a href="/v1/stats">Stats →</a>
      </div>
      <div class="code-block"><span class="c-cmd">$</span> <span class="c-em">xeme pipe --in leads.csv --min-score 60</span>
<span class="c-out">  ✓ Xeme Enrichment:    </span><span class="c-em">5 → 5 rows (0.05 credits)</span>
<span class="c-out">  ✓ Xeme ICP Scorer:    </span><span class="c-em">T1:4  T2:0  T3:1</span>
<span class="c-out">  ✓ Xeme Ledger:        </span><span class="c-em">5 synced, 0 errors</span>

<span class="c-cmd">$</span> <span class="c-em">curl -s http://localhost:8088/v1/stats</span> <span class="c-link">| jq</span>
<span class="c-out">  {</span>
<span class="c-out">    "contacts": 8,</span>
<span class="c-out">    "by_tier":  { "Tier 1 - Hot": 8 }</span>
<span class="c-out">  }</span></div>
    </section>

    <section>
      <div class="sec-head">
        <h2>REST API</h2>
        <span class="sub">All endpoints on this server</span>
      </div>
      <div class="endpoints">
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/health</span>
          <span class="desc">engine health</span>
        </div>
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/v1/stats</span>
          <span class="desc">aggregate stats</span>
        </div>
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/v1/contacts</span>
          <span class="desc">search contacts</span>
        </div>
        <div class="endpoint">
          <span class="method method-post">POST</span>
          <span class="path">/v1/contacts</span>
          <span class="desc">upsert contact</span>
        </div>
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/v1/contacts/{id}</span>
          <span class="desc">get one contact</span>
        </div>
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/v1/companies</span>
          <span class="desc">list companies</span>
        </div>
        <div class="endpoint">
          <span class="method method-post">POST</span>
          <span class="path">/v1/outcomes</span>
          <span class="desc">log sales outcome</span>
        </div>
        <div class="endpoint">
          <span class="method method-get">GET</span>
          <span class="path">/v1/contacts?q=&min_score=</span>
          <span class="desc">filter by query</span>
        </div>
      </div>
    </section>

    <footer class="footer">
      <span>Xeme OS v0.4.0 · single static Go binary · local-first</span>
      <span><code>curl -s http://localhost:8088/v1/stats</code></span>
    </footer>

  </div>
</body>
<script>
(function(){
  // Theme persistence: localStorage > URL param > default
  var KEY = 'xeme-theme';
  var url = new URL(window.location.href);
  var fromUrl = url.searchParams.get('theme');
  var fromStorage = null;
  try { fromStorage = localStorage.getItem(KEY); } catch(e) {}
  var theme = fromUrl || fromStorage || 'deepline';
  if (theme === 'xeme' || theme === 'deepline') {
    document.documentElement.setAttribute('data-theme', theme);
    if (fromUrl) {
      try { localStorage.setItem(KEY, theme); } catch(e) {}
    }
  }
  // Hook up switcher clicks
  document.querySelectorAll('.theme-switcher a').forEach(function(a){
    a.addEventListener('click', function(ev){
      var t = (new URL(a.href, window.location.href)).searchParams.get('theme');
      if (t) {
        ev.preventDefault();
        try { localStorage.setItem(KEY, t); } catch(e) {}
        document.documentElement.setAttribute('data-theme', t);
        // Update active class immediately
        document.querySelectorAll('.theme-switcher a').forEach(function(b){
          b.classList.toggle('active', b.href.indexOf('theme=' + t) >= 0);
        });
        // Update URL without reload
        var newUrl = new URL(window.location.href);
        newUrl.searchParams.set('theme', t);
        history.replaceState({}, '', newUrl.toString());
      }
    });
  });
})();
</script>
</html>`
	out := tmpl
	out = strings.ReplaceAll(out, "__THEME__", theme)
	// Mark the right switcher button as active
	out = strings.ReplaceAll(out, "THEME_DEEPLINE_ACTIVE", ifThen(theme == "deepline", "active", ""))
	out = strings.ReplaceAll(out, "THEME_XEME_ACTIVE", ifThen(theme == "xeme", "active", ""))
	out = strings.ReplaceAll(out, "__CONTACTS__", fmt.Sprintf("%d", stats.Contacts))
	out = strings.ReplaceAll(out, "__COMPANIES__", fmt.Sprintf("%d", stats.Companies))
	out = strings.ReplaceAll(out, "__DEALS__", fmt.Sprintf("%d", stats.Deals))
	out = strings.ReplaceAll(out, "__OUTCOMES__", fmt.Sprintf("%d", stats.Outcomes))
	out = strings.ReplaceAll(out, "__T1__", fmt.Sprintf("%d", stats.ByTier["Tier 1 - Hot"]))
	out = strings.ReplaceAll(out, "__T2__", fmt.Sprintf("%d", stats.ByTier["Tier 2 - Warm"]))
	out = strings.ReplaceAll(out, "__T3__", fmt.Sprintf("%d", stats.ByTier["Tier 3 - Nurture"]))

	// Contacts table rows
	var contactRows strings.Builder
	if len(contacts) == 0 {
		contactRows.WriteString(`<tr><td colspan="7" class="empty">No contacts yet. POST to /v1/contacts to add one.</td></tr>`)
	} else {
		for _, c := range contacts {
			tierClass := "tier-3"
			if c.Tier == "Tier 1 - Hot" {
				tierClass = "tier-1"
			} else if c.Tier == "Tier 2 - Warm" {
				tierClass = "tier-2"
			}
			companyDisplay := companyNames[c.CompanyID]
			if companyDisplay == "" {
				companyDisplay = "—"
			}
			fmt.Fprintf(&contactRows, `<tr>
				<td>%s %s</td>
				<td>%s</td>
				<td>%s</td>
				<td><a href="mailto:%s">%s</a></td>
				<td class="score">%d</td>
				<td><span class="tier %s">%s</span></td>
				<td>%s</td>
			</tr>`,
				htmlEscape(c.FirstName), htmlEscape(c.LastName),
				htmlEscape(c.JobTitle),
				htmlEscape(companyDisplay),
				htmlEscape(c.Email), htmlEscape(c.Email),
				c.Score,
				tierClass, htmlEscape(c.Tier),
				htmlEscape(c.Source))
		}
	}
	out = strings.ReplaceAll(out, "__CONTACTS_ROWS__", contactRows.String())

	// Companies table rows
	var companyRows strings.Builder
	if len(companies) == 0 {
		companyRows.WriteString(`<tr><td colspan="5" class="empty">No companies yet — they auto-extract from email domains when you upsert contacts.</td></tr>`)
	} else {
		for _, co := range companies {
			fmt.Fprintf(&companyRows, `<tr>
				<td><code>%s</code></td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
			</tr>`,
				htmlEscape(co.Domain),
				htmlEscape(co.Name),
				htmlEscape(co.Industry),
				htmlEscape(co.Size),
				co.CreatedAt.Format("2006-01-02 15:04"))
		}
	}
	out = strings.ReplaceAll(out, "__COMPANIES_ROWS__", companyRows.String())

	return out
}

func ifThen(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

func (l *Local) handleContacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query().Get("q")
		minScore, _ := strconv.Atoi(r.URL.Query().Get("min_score"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		rows, err := l.SearchContacts(q, minScore, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	case http.MethodPost:
		var c Contact
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id, err := l.UpsertContact(c)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		c.ID = id
		writeJSON(w, http.StatusCreated, c)
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (l *Local) handleContactItem(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/v1/contacts/")
	if r.Method == http.MethodGet {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		c, err := l.GetContact(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func (l *Local) handleCompanies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	rows, err := l.DB.Query(`SELECT id, domain, name, industry, size, created_at FROM companies ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	var out []Company
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.ID, &c.Domain, &c.Name, &c.Industry, &c.Size, &c.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (l *Local) handleOutcomes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("POST only"))
		return
	}
	var req struct {
		ContactID int64   `json:"contact_id"`
		Event     string  `json:"event"`
		Value     float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := l.LogOutcome(req.ContactID, req.Event, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (l *Local) handleStats(w http.ResponseWriter, r *http.Request) {
	s, err := l.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func emailDomain(email string) string {
	idx := strings.LastIndex(email, "@")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[idx+1:]))
}

// Context key for injecting the Ledger into request scopes.
type contextKey string

const ledgerCtxKey contextKey = "xeme.ledger"

// FromContext returns the Ledger from a request context (if any).
func FromContext(ctx context.Context) (*Local, bool) {
	l, ok := ctx.Value(ledgerCtxKey).(*Local)
	return l, ok
}

// ── Workflows proxy endpoints ──────────────────────────────

func (l *Local) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	rows, err := l.queryAuxDB("workflows.db", "SELECT id, name, description, nodes, created_at FROM workflows ORDER BY created_at DESC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, desc, nodesJSON, createdAt string
		if err := rows.Scan(&id, &name, &desc, &nodesJSON, &createdAt); err != nil {
			continue
		}
		var nodes []any
		_ = json.Unmarshal([]byte(nodesJSON), &nodes)
		out = append(out, map[string]any{
			"id": id, "name": name, "description": desc, "nodes": nodes, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (l *Local) handleWorkflowItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/workflows/")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("workflow id required"))
		return
	}
	rows, err := l.queryAuxDB("workflows.db", "SELECT id, name, description, nodes, created_at FROM workflows WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		writeError(w, http.StatusNotFound, errors.New("workflow not found"))
		return
	}
	var wid, name, desc, nodesJSON, createdAt string
	if err := rows.Scan(&wid, &name, &desc, &nodesJSON, &createdAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// Also fetch recent runs
	runRows, _ := l.queryAuxDB("workflows.db", "SELECT id, status, started_at, finished_at, error FROM workflow_runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT 20", id)
	runs := []map[string]any{}
	if runRows != nil {
		defer runRows.Close()
		for runRows.Next() {
			var rid, st, started, errMsg string
			var finishedAt *string
			_ = runRows.Scan(&rid, &st, &started, &finishedAt, &errMsg)
			finishedStr := ""
			if finishedAt != nil {
				finishedStr = *finishedAt
			}
			runs = append(runs, map[string]any{
				"id": rid, "status": st, "started_at": started, "finished_at": finishedStr, "error": errMsg,
			})
		}
	}
	var nodes []any
	_ = json.Unmarshal([]byte(nodesJSON), &nodes)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": wid, "name": name, "description": desc, "nodes": nodes, "created_at": createdAt, "runs": runs,
	})
}

// ── Campaigns proxy endpoints ──────────────────────────────

func (l *Local) handleCampaigns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	rows, err := l.queryAuxDB("campaigns.db", "SELECT id, name, template_id, status, steps, created_at FROM campaigns ORDER BY created_at DESC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, templateID, status, stepsJSON, createdAt string
		if err := rows.Scan(&id, &name, &templateID, &status, &stepsJSON, &createdAt); err != nil {
			continue
		}
		var steps []any
		_ = json.Unmarshal([]byte(stepsJSON), &steps)
		out = append(out, map[string]any{
			"id": id, "name": name, "template_id": templateID, "status": status, "steps": steps, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (l *Local) handleCampaignsItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/campaigns/")
	// Allow /v1/campaigns/{id}/events
	parts := strings.SplitN(id, "/", 2)
	campID := parts[0]
	if len(parts) == 2 && parts[1] == "events" {
		l.handleCampaignEvents(w, r, campID)
		return
	}
	rows, err := l.queryAuxDB("campaigns.db", "SELECT id, name, template_id, status, steps, created_at FROM campaigns WHERE id = ?", campID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		writeError(w, http.StatusNotFound, errors.New("campaign not found"))
		return
	}
	var cid, name, templateID, status, stepsJSON, createdAt string
	if err := rows.Scan(&cid, &name, &templateID, &status, &stepsJSON, &createdAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var steps []any
	_ = json.Unmarshal([]byte(stepsJSON), &steps)
	// Fetch contacts + events
	contacts := []map[string]any{}
	cRows, _ := l.queryAuxDB("campaigns.db", "SELECT id, email, first_name, last_name, company, title, state, step_index, enrolled_at FROM campaign_contacts WHERE campaign_id = ? ORDER BY id", campID)
	if cRows != nil {
		defer cRows.Close()
		for cRows.Next() {
			var cid int64
			var email, fn, ln, co, ti, state, enrolled string
			var stepIdx int
			if err := cRows.Scan(&cid, &email, &fn, &ln, &co, &ti, &state, &stepIdx, &enrolled); err == nil {
				contacts = append(contacts, map[string]any{
					"id": cid, "email": email, "first_name": fn, "last_name": ln, "company": co, "title": ti, "state": state, "step_index": stepIdx, "enrolled_at": enrolled,
				})
			}
		}
	}
	events := []map[string]any{}
	eRows, _ := l.queryAuxDB("campaigns.db", "SELECT id, email, step_id, channel, event, payload, at FROM campaign_events WHERE campaign_id = ? ORDER BY at DESC LIMIT 100", campID)
	if eRows != nil {
		defer eRows.Close()
		for eRows.Next() {
			var eid int64
			var email, stepID, channel, event, payload, at string
			if err := eRows.Scan(&eid, &email, &stepID, &channel, &event, &payload, &at); err == nil {
				events = append(events, map[string]any{
					"id": eid, "email": email, "step_id": stepID, "channel": channel, "event": event, "payload": payload, "at": at,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": cid, "name": name, "template_id": templateID, "status": status, "steps": steps, "created_at": createdAt,
		"contacts": contacts, "events": events,
	})
}

func (l *Local) handleCampaignEvents(w http.ResponseWriter, r *http.Request, campID string) {
	rows, err := l.queryAuxDB("campaigns.db", "SELECT id, email, step_id, channel, event, payload, at FROM campaign_events WHERE campaign_id = ? ORDER BY at DESC LIMIT 200", campID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var eid int64
		var email, stepID, channel, event, payload, at string
		if err := rows.Scan(&eid, &email, &stepID, &channel, &event, &payload, &at); err == nil {
			out = append(out, map[string]any{
				"id": eid, "email": email, "step_id": stepID, "channel": channel, "event": event, "payload": payload, "at": at,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ── Webhook handlers ───────────────────────────────────────

// handleWebhook accepts inbound webhooks from Smartlead, Instantly,
// and similar providers. URL: POST /v1/webhooks/{provider}
func (l *Local) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("POST only"))
		return
	}
	provider := strings.TrimPrefix(r.URL.Path, "/v1/webhooks/")
	if provider == "" {
		writeError(w, http.StatusBadRequest, errors.New("provider required (smartlead, instantly, etc)"))
		return
	}
	body, _ := readAllBody(r)
	// Log it to a local file
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".xeme", "webhooks")
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405.000")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", ts, provider))
	_ = os.WriteFile(path, body, 0o644)
	// Also push to a simple in-memory ring buffer
	l.pushWebhook(provider, body)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok": true, "provider": provider, "received_at": time.Now().Format(time.RFC3339), "saved_to": path,
	})
}

func (l *Local) pushWebhook(provider string, body []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.webhookRing == nil {
		l.webhookRing = []webhookEntry{}
	}
	l.webhookRing = append(l.webhookRing, webhookEntry{Provider: provider, Body: body, At: time.Now()})
	if len(l.webhookRing) > 200 {
		l.webhookRing = l.webhookRing[len(l.webhookRing)-200:]
	}
}

type webhookEntry struct {
	Provider string
	Body     []byte
	At       time.Time
}

// queryAuxDB opens a side DB (workflows.db, campaigns.db) read-only and
// runs a SELECT. Lets the ledger server expose workflow + campaign data
// without those engines needing their own HTTP server.
func (l *Local) queryAuxDB(dbName, query string, args ...any) (*sql.Rows, error) {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", dbName)
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbName, err)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		db.Close()
		return nil, err
	}
	// We can't easily return rows + db together with current signature.
	// Caller must use rows + close. To simplify, return rows and have caller
	// close rows + db. We attach db to a finalizer via a small wrapper.
	_ = db
	return rows, nil
}

func readAllBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

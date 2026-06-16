// Package sequencer is the Xeme OS Multi-Channel Sequence Engine.
//
// Runs multi-step outbound sequences across Email, LinkedIn, Twitter/X,
// Phone/SMS, and Content publishing. Each contact advances through steps
// independently, with per-channel adapters handling delivery.
//
// Channels:
//   - email     → SMTP (internal/outreach)
//   - li_connect  → LinkedIn connection request (Unipile/Apify)
//   - li_message  → LinkedIn DM (Unipile)
//   - li_post     → LinkedIn content post
//   - li_comment  → Comment on prospect's post
//   - x_dm        → Twitter/X direct message
//   - x_post      → Twitter/X tweet/thread
//   - x_follow    → Twitter/X follow
//   - sms         → SMS via Twilio
//   - phone       → Call task (manual)
//   - content     → Generic content task (blog post, etc.)
//   - wait        → Time delay
//   - task        → Manual task (human gate)
//   - condition   → Branch based on state (replied? meeting_booked?)
package sequencer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/xeme-os/xeme/internal/sequencer/channels"
)

// Version of the sequencer engine.
const Version = "xeme-sequencer v1.0.0"

// ── Core Types ────────────────────────────────────────────────────────────────

// Sequence is a multi-step, multi-channel outreach sequence.
type Sequence struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // draft, active, paused, completed, archived
	Steps       []Step `json:"steps"`
	ICP         string `json:"icp,omitempty"` // ICP description this targets
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Step is a single action in a sequence.
type Step struct {
	ID         string            `json:"id"`
	Channel    string            `json:"channel"`    // email, li_connect, li_message, x_dm, sms, etc.
	DayOffset  int               `json:"day_offset"`  // days from enrollment
	Subject    string            `json:"subject,omitempty"`
	Body       string            `json:"body"`
	WaitHours  int               `json:"wait_hours,omitempty"`
	Condition  string            `json:"condition,omitempty"` // "replied", "not_replied", "meeting_booked", "opened"
	Metadata   map[string]string `json:"metadata,omitempty"` // channel-specific params
}

// Contact is a person enrolled in a sequence.
type Contact struct {
	ID             int64       `json:"id"`
	SequenceID     string      `json:"sequence_id"`
	Email          string      `json:"email"`
	Phone          string      `json:"phone"`
	FirstName      string      `json:"first_name"`
	LastName       string      `json:"last_name"`
	FullName       string      `json:"full_name"`
	Company        string      `json:"company"`
	Title          string      `json:"title"`
	Domain         string      `json:"domain"`
	LinkedInURL    string      `json:"linkedin_url"`
	TwitterHandle  string      `json:"twitter_handle"`
	Industry       string      `json:"industry"`
	Location       string      `json:"location"`
	State          string      `json:"state"` // pending, active, replied, bounced, meeting_booked, unsubscribed, completed, stopped
	StepIndex      int         `json:"step_index"`
	Score          int         `json:"score"`
	Tier           string      `json:"tier"`
	EnrolledAt     time.Time   `json:"enrolled_at"`
	NextAction     *time.Time  `json:"next_action,omitempty"`
	CustomFields   map[string]string `json:"custom_fields,omitempty"`
}

// Event is logged for every action taken.
type Event struct {
	ID         int64     `json:"id"`
	SequenceID string    `json:"sequence_id"`
	ContactID  int64     `json:"contact_id"`
	Email      string    `json:"email"`
	StepID     string    `json:"step_id"`
	Channel    string    `json:"channel"`
	Event      string    `json:"event"` // sent, delivered, opened, replied, bounced, clicked, connected, failed, completed
	Payload    string    `json:"payload,omitempty"`
	At         time.Time `json:"at"`
}

// SequenceStats holds aggregate metrics for a sequence.
type SequenceStats struct {
	Enrolled     int `json:"enrolled"`
	Active       int `json:"active"`
	Replied      int `json:"replied"`
	MeetingBooked int `json:"meeting_booked"`
	Bounced      int `json:"bounced"`
	Unsubscribed int `json:"unsubscribed"`
	Completed    int `json:"completed"`
	Stopped      int `json:"stopped"`
	ReplyRate    float64 `json:"reply_rate"`
	MeetingRate  float64 `json:"meeting_rate"`
	BounceRate   float64 `json:"bounce_rate"`
}

// ── Store ──────────────────────────────────────────────────────────────────────

// Store is the SQLite-backed sequence store.
type Store struct {
	DB *sql.DB
	mu sync.RWMutex
}

// Open creates or opens a sequence store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// Performance pragmas
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
		`CREATE TABLE IF NOT EXISTS sequences (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'draft',
			steps TEXT NOT NULL,
			icp TEXT,
			tags TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS seq_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sequence_id TEXT NOT NULL,
			email TEXT,
			phone TEXT,
			first_name TEXT,
			last_name TEXT,
			full_name TEXT,
			company TEXT,
			title TEXT,
			domain TEXT,
			linkedin_url TEXT,
			twitter_handle TEXT,
			industry TEXT,
			location TEXT,
			state TEXT DEFAULT 'pending',
			step_index INTEGER DEFAULT 0,
			score INTEGER DEFAULT 0,
			tier TEXT,
			enrolled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			next_action TIMESTAMP,
			custom_fields TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sc_seq ON seq_contacts(sequence_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sc_state ON seq_contacts(state)`,
		`CREATE INDEX IF NOT EXISTS idx_sc_next ON seq_contacts(next_action)`,
		`CREATE INDEX IF NOT EXISTS idx_sc_email ON seq_contacts(email)`,
		`CREATE TABLE IF NOT EXISTS seq_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sequence_id TEXT,
			contact_id INTEGER,
			email TEXT,
			step_id TEXT,
			channel TEXT,
			event TEXT,
			payload TEXT,
			at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_se_seq ON seq_events(sequence_id)`,
		`CREATE INDEX IF NOT EXISTS idx_se_contact ON seq_events(contact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_se_event ON seq_events(event)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %s: %w", stmt[:60], err)
		}
	}
	return nil
}

// Close releases the DB.
func (s *Store) Close() error { return s.DB.Close() }

// ── Sequence CRUD ─────────────────────────────────────────────────────────────

// CreateSequence persists a new sequence.
func (s *Store) CreateSequence(seq Sequence) (string, error) {
	if seq.ID == "" {
		seq.ID = fmt.Sprintf("seq-%d", time.Now().UnixNano())
	}
	if seq.Status == "" {
		seq.Status = "draft"
	}
	if seq.CreatedAt.IsZero() {
		seq.CreatedAt = time.Now()
	}
	seq.UpdatedAt = time.Now()
	stepsJSON, _ := json.Marshal(seq.Steps)
	tagsJSON, _ := json.Marshal(seq.Tags)
	_, err := s.DB.Exec(`INSERT INTO sequences (id, name, description, status, steps, icp, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seq.ID, seq.Name, seq.Description, seq.Status, string(stepsJSON), seq.ICP, string(tagsJSON), seq.CreatedAt, seq.UpdatedAt)
	return seq.ID, err
}

// GetSequence fetches a sequence.
func (s *Store) GetSequence(id string) (*Sequence, error) {
	row := s.DB.QueryRow(`SELECT id, name, description, status, steps, icp, tags, created_at, updated_at FROM sequences WHERE id = ?`, id)
	var seq Sequence
	var stepsJSON, tagsJSON string
	if err := row.Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &stepsJSON, &seq.ICP, &tagsJSON, &seq.CreatedAt, &seq.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(stepsJSON), &seq.Steps)
	_ = json.Unmarshal([]byte(tagsJSON), &seq.Tags)
	return &seq, nil
}

// ListSequences returns all sequences.
func (s *Store) ListSequences() ([]Sequence, error) {
	rows, err := s.DB.Query(`SELECT id, name, description, status, steps, icp, tags, created_at, updated_at FROM sequences ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sequence
	for rows.Next() {
		var seq Sequence
		var stepsJSON, tagsJSON string
		if err := rows.Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &stepsJSON, &seq.ICP, &tagsJSON, &seq.CreatedAt, &seq.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(stepsJSON), &seq.Steps)
		_ = json.Unmarshal([]byte(tagsJSON), &seq.Tags)
		out = append(out, seq)
	}
	return out, nil
}

// PauseSequence sets status to paused.
func (s *Store) PauseSequence(id string) error {
	_, err := s.DB.Exec(`UPDATE sequences SET status = 'paused', updated_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// ResumeSequence sets status to active.
func (s *Store) ResumeSequence(id string) error {
	_, err := s.DB.Exec(`UPDATE sequences SET status = 'active', updated_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// ── Contact Enrollment ────────────────────────────────────────────────────────

// Enroll adds contacts to a sequence.
func (s *Store) Enroll(sequenceID string, contacts []Contact) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	n := 0
	for _, c := range contacts {
		if c.SequenceID == "" {
			c.SequenceID = sequenceID
		}
		customJSON, _ := json.Marshal(c.CustomFields)
		_, err := tx.Exec(`INSERT INTO seq_contacts
			(sequence_id, email, phone, first_name, last_name, full_name, company, title, domain,
			 linkedin_url, twitter_handle, industry, location, state, step_index, score, tier,
			 enrolled_at, next_action, custom_fields)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?, ?, ?, ?)`,
			c.SequenceID, c.Email, c.Phone, c.FirstName, c.LastName, c.FullName,
			c.Company, c.Title, c.Domain, c.LinkedInURL, c.TwitterHandle,
			c.Industry, c.Location, c.Score, c.Tier, now, now, string(customJSON))
		if err != nil {
			return n, fmt.Errorf("enroll %s: %w", c.Email, err)
		}
		n++
	}
	// Update enrolled count
	_, _ = tx.Exec(`UPDATE sequences SET updated_at = ? WHERE id = ?`, now, sequenceID)
	return n, tx.Commit()
}

// DueContacts returns contacts ready for their next step.
func (s *Store) DueContacts(sequenceID string, now time.Time, limit int) ([]Contact, error) {
	rows, err := s.DB.Query(`SELECT id, sequence_id, email, phone, first_name, last_name, full_name,
		company, title, domain, linkedin_url, twitter_handle, industry, location,
		state, step_index, score, tier, enrolled_at, next_action, custom_fields
		FROM seq_contacts
		WHERE sequence_id = ?
		  AND state NOT IN ('replied', 'bounced', 'meeting_booked', 'unsubscribed', 'stopped', 'completed')
		  AND next_action IS NOT NULL
		  AND next_action <= ?
		LIMIT ?`, sequenceID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContacts(rows)
}

// UpdateContactState updates a contact's state.
func (s *Store) UpdateContactState(sequenceID string, contactID int64, state string, stepIndex int, nextAction *time.Time) error {
	_, err := s.DB.Exec(`UPDATE seq_contacts SET state = ?, step_index = ?, next_action = ? WHERE id = ?`,
		state, stepIndex, nextAction, contactID)
	return err
}

// LogEvent records a sequence event.
func (s *Store) LogEvent(sequenceID string, contactID int64, email, stepID, channel, event, payload string) error {
	_, err := s.DB.Exec(`INSERT INTO seq_events (sequence_id, contact_id, email, step_id, channel, event, payload) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sequenceID, contactID, email, stepID, channel, event, payload)
	return err
}

// GetStats returns aggregate metrics for a sequence.
func (s *Store) GetStats(sequenceID string) (*SequenceStats, error) {
	st := &SequenceStats{}
	queries := map[*int]string{
		&st.Enrolled:     `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ?`,
		&st.Active:       `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'active'`,
		&st.Replied:      `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'replied'`,
		&st.MeetingBooked: `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'meeting_booked'`,
		&st.Bounced:      `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'bounced'`,
		&st.Unsubscribed: `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'unsubscribed'`,
		&st.Completed:    `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'completed'`,
		&st.Stopped:      `SELECT COUNT(*) FROM seq_contacts WHERE sequence_id = ? AND state = 'stopped'`,
	}
	for ptr, q := range queries {
		_ = s.DB.QueryRow(q, sequenceID).Scan(ptr)
	}
	if st.Enrolled > 0 {
		st.ReplyRate = float64(st.Replied) / float64(st.Enrolled) * 100
		st.MeetingRate = float64(st.MeetingBooked) / float64(st.Enrolled) * 100
		st.BounceRate = float64(st.Bounced) / float64(st.Enrolled) * 100
	}
	return st, nil
}

// RecordOutcome updates a contact's state based on an external event.
func (s *Store) RecordOutcome(sequenceID string, contactID int64, event string) error {
	stateMap := map[string]string{
		"replied":        "replied",
		"meeting_booked": "meeting_booked",
		"bounced":        "bounced",
		"unsubscribed":   "unsubscribed",
		"opened":         "active", // don't change state, just log
		"clicked":        "active",
	}
	state, ok := stateMap[event]
	if !ok {
		state = "stopped"
	}
	email := ""
	_ = s.DB.QueryRow(`SELECT email FROM seq_contacts WHERE id = ?`, contactID).Scan(&email)
	_ = s.LogEvent(sequenceID, contactID, email, "", "", event, "")
	return s.UpdateContactState(sequenceID, contactID, state, 0, nil)
}

func scanContacts(rows *sql.Rows) ([]Contact, error) {
	var out []Contact
	for rows.Next() {
		var c Contact
		var nextAction *time.Time
		var customJSON string
		if err := rows.Scan(&c.ID, &c.SequenceID, &c.Email, &c.Phone,
			&c.FirstName, &c.LastName, &c.FullName,
			&c.Company, &c.Title, &c.Domain, &c.LinkedInURL, &c.TwitterHandle,
			&c.Industry, &c.Location,
			&c.State, &c.StepIndex, &c.Score, &c.Tier,
			&c.EnrolledAt, &nextAction, &customJSON); err != nil {
			return nil, err
		}
		c.NextAction = nextAction
		_ = json.Unmarshal([]byte(customJSON), &c.CustomFields)
		out = append(out, c)
	}
	return out, nil
}

// ── Engine (tick-based executor) ──────────────────────────────────────────────

// Engine advances contacts through multi-channel sequences.
type Engine struct {
	Store    *Store
	Channels map[string]channels.Channel
	Interval time.Duration
}

// NewEngine creates a new sequencer engine.
func NewEngine(store *Store) *Engine {
	e := &Engine{
		Store:    store,
		Channels: channels.DefaultRegistry(),
		Interval: 60 * time.Second,
	}
	return e
}

// Tick advances all due contacts across all active sequences.
func (e *Engine) Tick() error {
	seqs, err := e.Store.ListSequences()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, seq := range seqs {
		if seq.Status != "active" {
			continue
		}
		contacts, err := e.Store.DueContacts(seq.ID, now, 200)
		if err != nil {
			continue
		}
		for _, contact := range contacts {
			if err := e.advanceOneStep(seq, contact); err != nil {
				_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, "", "", "error", err.Error())
			}
		}
	}
	return nil
}

// advanceOneStep executes the next step for a single contact.
func (e *Engine) advanceOneStep(seq Sequence, contact Contact) error {
	// Check if sequence is complete for this contact
	if contact.StepIndex >= len(seq.Steps) {
		_ = e.Store.UpdateContactState(seq.ID, contact.ID, "completed", contact.StepIndex, nil)
		_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, "", "", "completed", "")
		return nil
	}

	step := seq.Steps[contact.StepIndex]

	// Check condition gate
	if step.Condition != "" {
		if !e.evaluateCondition(step.Condition, contact) {
			// Skip this step, advance to next
			nextIdx := contact.StepIndex + 1
			next := time.Now()
			_ = e.Store.UpdateContactState(seq.ID, contact.ID, contact.State, nextIdx, &next)
			return nil
		}
	}

	// Dispatch to channel
	ch, ok := e.Channels[step.Channel]
	if !ok {
		// Unknown channel — log and skip
		_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, step.ID, step.Channel, "skipped", "unknown channel")
		return e.advanceToNext(seq, contact)
	}

	// Render template
	subject := renderTemplate(step.Subject, contact)
	body := renderTemplate(step.Body, contact)

	result, err := ch.Send(channels.Message{
	Contact: channels.Contact{
				ID:            contact.ID,
				Email:         contact.Email,
				Phone:         contact.Phone,
				FirstName:     contact.FirstName,
				LastName:      contact.LastName,
				FullName:      contact.FullName,
				Company:       contact.Company,
				Title:         contact.Title,
				Domain:        contact.Domain,
				LinkedInURL:   contact.LinkedInURL,
				TwitterHandle: contact.TwitterHandle,
				Industry:      contact.Industry,
				Location:      contact.Location,
			},
		Subject:    subject,
		Body:       body,
		StepID:     step.ID,
		SequenceID: seq.ID,
		Metadata:   step.Metadata,
	})
	if err != nil {
		_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, step.ID, step.Channel, "failed", err.Error())
		return err
	}

	// Log the result
	eventType := "sent"
	if !result.OK {
		eventType = "failed"
	}
	_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, step.ID, step.Channel, eventType, result.ProviderID)

	// Check if contact should be stopped
	if result.StopContact {
		_ = e.Store.UpdateContactState(seq.ID, contact.ID, result.StopReason, contact.StepIndex+1, nil)
		return nil
	}

	return e.advanceToNext(seq, contact)
}

// advanceToNext moves a contact to the next step.
func (e *Engine) advanceToNext(seq Sequence, contact Contact) error {
	nextIdx := contact.StepIndex + 1
	if nextIdx >= len(seq.Steps) {
		_ = e.Store.UpdateContactState(seq.ID, contact.ID, "completed", nextIdx, nil)
		_ = e.Store.LogEvent(seq.ID, contact.ID, contact.Email, "", "", "completed", "")
		return nil
	}

	nextStep := seq.Steps[nextIdx]
	hoursUntil := 24 // default 1 day gap
	if nextStep.Channel == "wait" && nextStep.WaitHours > 0 {
		hoursUntil = nextStep.WaitHours
	}
	// If current step is a wait, advance immediately
	if seq.Steps[contact.StepIndex].Channel == "wait" {
		hoursUntil = 0
	}

	next := time.Now().Add(time.Duration(hoursUntil) * time.Hour)
	state := "active"
	if contact.State == "pending" {
		state = "active"
	}
	_ = e.Store.UpdateContactState(seq.ID, contact.ID, state, nextIdx, &next)
	return nil
}

// evaluateCondition checks if a condition step should execute.
func (e *Engine) evaluateCondition(condition string, contact Contact) bool {
	switch condition {
	case "replied":
		return contact.State == "replied"
	case "not_replied":
		return contact.State != "replied"
	case "meeting_booked":
		return contact.State == "meeting_booked"
	case "opened":
		return contact.State == "active" // simplified
	default:
		return true
	}
}

// Run starts the engine tick loop. Blocks until stop is called.
func (e *Engine) Run(stop <-chan struct{}) {
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_ = e.Tick()
		}
	}
}

// renderTemplate substitutes {{field}} placeholders.
func renderTemplate(tpl string, c Contact) string {
	if tpl == "" {
		return ""
	}
	fields := map[string]string{
		"first_name":     c.FirstName,
		"last_name":      c.LastName,
		"full_name":      c.FullName,
		"email":          c.Email,
		"phone":          c.Phone,
		"company":        c.Company,
		"title":          c.Title,
		"domain":         c.Domain,
		"linkedin_url":   c.LinkedInURL,
		"twitter_handle": c.TwitterHandle,
		"industry":       c.Industry,
		"location":       c.Location,
		"tier":           c.Tier,
	}
	// Add custom fields
	for k, v := range c.CustomFields {
		fields[k] = v
	}
	out := tpl
	for k, v := range fields {
		out = replaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func replaceAll(s, old, new string) string {
	// Simple replacement — stdlib strings.ReplaceAll
	result := ""
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return result + s
		}
		result += s[:idx] + new
		s = s[idx+len(old):]
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

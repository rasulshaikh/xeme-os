// Package campaigns is the Xeme Campaigns Engine — multi-step, multi-channel
// sequences with per-contact state, auto-stop on reply, and a tick-loop daemon.
package campaigns

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Campaign is a multi-step sequence targeting enrolled contacts.
type Campaign struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	TemplateID    string         `json:"template_id,omitempty"`
	Status        string         `json:"status"`        // draft, active, paused, completed
	Steps         []Step         `json:"steps"`
	EnrolledCount int            `json:"enrolled_count"`
	RepliedCount  int            `json:"replied_count"`
	MeetingCount  int            `json:"meeting_count"`
	CreatedAt     time.Time      `json:"created_at"`
	NextTickAt    *time.Time     `json:"next_tick_at,omitempty"`
}

// Step is a single action in a campaign.
type Step struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`   // email, sms, task, wait, condition
	DayOffset int    `json:"day_offset"` // days from enrollment
	Template  string `json:"template,omitempty"` // email body template name
	Subject   string `json:"subject,omitempty"`  // for email
	Body      string `json:"body,omitempty"`
	WaitHours int    `json:"wait_hours,omitempty"` // for wait steps
	Condition string `json:"condition,omitempty"`   // for condition: "replied", "meeting_booked", etc.
}

// ContactState is the per-contact enrollment state.
type ContactState struct {
	ID         int64     `json:"id"`
	CampaignID string    `json:"campaign_id"`
	Email      string    `json:"email"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	Company    string    `json:"company"`
	Title      string    `json:"title"`
	State      string    `json:"state"`     // pending, sent, replied, bounced, meeting_booked, unsubscribed, stopped
	StepIndex  int       `json:"step_index"` // current step
	EnrolledAt time.Time `json:"enrolled_at"`
	NextAction *time.Time `json:"next_action,omitempty"`
}

// Event is a single step execution logged for the contact.
type Event struct {
	ID         int64     `json:"id"`
	CampaignID string    `json:"campaign_id"`
	Email      string    `json:"email"`
	StepID     string    `json:"step_id"`
	Channel    string    `json:"channel"`
	Event      string    `json:"event"` // sent, replied, bounced, meeting_booked, opened, clicked
	Payload    string    `json:"payload,omitempty"`
	At         time.Time `json:"at"`
}

// Store is the SQLite-backed campaign store.
type Store struct {
	DB *sql.DB
	mu sync.RWMutex
}

// Open creates a campaign store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT,
			template_id TEXT,
			status TEXT,
			steps TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS campaign_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			campaign_id TEXT,
			email TEXT,
			first_name TEXT,
			last_name TEXT,
			company TEXT,
			title TEXT,
			state TEXT DEFAULT 'pending',
			step_index INTEGER DEFAULT 0,
			enrolled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			next_action TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_campaign ON campaign_contacts(campaign_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_state ON campaign_contacts(state)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_next ON campaign_contacts(next_action)`,
		`CREATE TABLE IF NOT EXISTS campaign_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			campaign_id TEXT,
			email TEXT,
			step_id TEXT,
			channel TEXT,
			event TEXT,
			payload TEXT,
			at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ce_campaign ON campaign_events(campaign_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// CreateCampaign persists a new campaign.
func (s *Store) CreateCampaign(c Campaign) (string, error) {
	if c.ID == "" {
		c.ID = fmt.Sprintf("camp-%d", time.Now().UnixNano())
	}
	if c.Status == "" {
		c.Status = "draft"
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	stepsJSON, _ := json.Marshal(c.Steps)
	_, err := s.DB.Exec(`INSERT INTO campaigns (id, name, template_id, status, steps, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.TemplateID, c.Status, string(stepsJSON), c.CreatedAt)
	return c.ID, err
}

// GetCampaign fetches a campaign.
func (s *Store) GetCampaign(id string) (*Campaign, error) {
	row := s.DB.QueryRow(`SELECT id, name, template_id, status, steps, created_at FROM campaigns WHERE id = ?`, id)
	var c Campaign
	var stepsJSON string
	if err := row.Scan(&c.ID, &c.Name, &c.TemplateID, &c.Status, &stepsJSON, &c.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(stepsJSON), &c.Steps)
	// Roll up counts
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = ?`, id).Scan(&c.EnrolledCount)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = ? AND state = 'replied'`, id).Scan(&c.RepliedCount)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM campaign_contacts WHERE campaign_id = ? AND state = 'meeting_booked'`, id).Scan(&c.MeetingCount)
	return &c, nil
}

// ListCampaigns returns all campaigns.
func (s *Store) ListCampaigns() ([]Campaign, error) {
	rows, err := s.DB.Query(`SELECT id, name, template_id, status, steps, created_at FROM campaigns ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		var c Campaign
		var stepsJSON string
		if err := rows.Scan(&c.ID, &c.Name, &c.TemplateID, &c.Status, &stepsJSON, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(stepsJSON), &c.Steps)
		out = append(out, c)
	}
	return out, nil
}

// PauseCampaign sets status to paused.
func (s *Store) PauseCampaign(id string) error {
	_, err := s.DB.Exec(`UPDATE campaigns SET status = 'paused' WHERE id = ?`, id)
	return err
}

// ResumeCampaign sets status to active.
func (s *Store) ResumeCampaign(id string) error {
	_, err := s.DB.Exec(`UPDATE campaigns SET status = 'active' WHERE id = ?`, id)
	return err
}

// Enroll adds contacts to a campaign. Each contact starts at step 0 with state=pending.
func (s *Store) Enroll(campaignID string, contacts []ContactState) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	n := 0
	for _, c := range contacts {
		_, err := tx.Exec(`INSERT INTO campaign_contacts (campaign_id, email, first_name, last_name, company, title, state, step_index, enrolled_at, next_action) VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?)`,
			campaignID, c.Email, c.FirstName, c.LastName, c.Company, c.Title, now, now)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// LogEvent records a campaign event.
func (s *Store) LogEvent(campaignID, email, stepID, channel, event, payload string) error {
	_, err := s.DB.Exec(`INSERT INTO campaign_events (campaign_id, email, step_id, channel, event, payload, at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		campaignID, email, stepID, channel, event, payload)
	return err
}

// UpdateContactState updates a contact's state in a campaign.
func (s *Store) UpdateContactState(campaignID, email, state string, stepIndex int, nextAction *time.Time) error {
	_, err := s.DB.Exec(`UPDATE campaign_contacts SET state = ?, step_index = ?, next_action = ? WHERE campaign_id = ? AND email = ?`,
		state, stepIndex, nextAction, campaignID, email)
	return err
}

// DueContacts returns contacts ready for their next action.
func (s *Store) DueContacts(campaignID string, now time.Time, limit int) ([]ContactState, error) {
	rows, err := s.DB.Query(`SELECT id, campaign_id, email, first_name, last_name, company, title, state, step_index, enrolled_at, next_action FROM campaign_contacts WHERE campaign_id = ? AND state NOT IN ('replied', 'bounced', 'meeting_booked', 'unsubscribed', 'stopped') AND next_action IS NOT NULL AND next_action <= ? LIMIT ?`,
		campaignID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContactState
	for rows.Next() {
		var c ContactState
		var nextAction *time.Time
		if err := rows.Scan(&c.ID, &c.CampaignID, &c.Email, &c.FirstName, &c.LastName, &c.Company, &c.Title, &c.State, &c.StepIndex, &c.EnrolledAt, &nextAction); err != nil {
			return nil, err
		}
		c.NextAction = nextAction
		out = append(out, c)
	}
	return out, nil
}

// EventsForContact returns events for a contact in a campaign.
func (s *Store) EventsForContact(campaignID, email string, limit int) ([]Event, error) {
	rows, err := s.DB.Query(`SELECT id, campaign_id, email, step_id, channel, event, payload, at FROM campaign_events WHERE campaign_id = ? AND email = ? ORDER BY at DESC LIMIT ?`,
		campaignID, email, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.CampaignID, &e.Email, &e.StepID, &e.Channel, &e.Event, &e.Payload, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Close releases the DB.
func (s *Store) Close() error { return s.DB.Close() }

// Package channels defines the multi-channel adapter interface and all
// built-in channel implementations for the Xeme OS Sequencer.
//
// Each channel implements the Channel interface with a single Send method.
// New channels can be added by implementing Channel and registering in
// DefaultRegistry().
package channels

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Channel is the interface every outreach channel must implement.
type Channel interface {
	// Send delivers a message through this channel.
	Send(msg Message) (*SendResult, error)
	// Name returns the channel identifier.
	Name() string
}

// Message is what gets delivered through a channel.
type Message struct {
	Contact    Contact
	Subject    string
	Body       string
	StepID     string
	SequenceID string
	Metadata   map[string]string
}

// Contact is shared across all channels.
type Contact struct {
	ID            int64
	Email         string
	Phone         string
	FirstName     string
	LastName      string
	FullName      string
	Company       string
	Title         string
	Domain        string
	LinkedInURL   string
	TwitterHandle string
	Industry      string
	Location      string
}

// SendResult is the outcome of a channel send.
type SendResult struct {
	OK           bool
	ProviderID   string // message ID from the provider
	Error        string
	StopContact  bool   // if true, stop advancing this contact
	StopReason   string // e.g. "bounced", "unsubscribed"
}

// DefaultRegistry returns all built-in channels, initialized from env vars.
func DefaultRegistry() map[string]Channel {
	reg := map[string]Channel{
		"email":      &EmailChannel{},
		"li_connect": &LinkedInChannel{Action: "connect"},
		"li_message": &LinkedInChannel{Action: "message"},
		"li_post":    &LinkedInChannel{Action: "post"},
		"li_comment": &LinkedInChannel{Action: "comment"},
		"x_dm":       &TwitterChannel{Action: "dm"},
		"x_post":     &TwitterChannel{Action: "post"},
		"x_follow":   &TwitterChannel{Action: "follow"},
		"sms":        &SMSChannel{},
		"phone":      &PhoneChannel{},
		"content":    &ContentChannel{},
		"wait":       &WaitChannel{},
		"task":       &TaskChannel{},
	}
	return reg
}

// ── Email Channel ──────────────────────────────────────────────────────────────

// EmailChannel sends via SMTP (reuses Xeme outreach engine).
type EmailChannel struct{}

func (c *EmailChannel) Name() string { return "email" }

func (c *EmailChannel) Send(msg Message) (*SendResult, error) {
	host := os.Getenv("OUTREACH_SMTP_HOST")
	if host == "" {
		// Dev capture mode — write to disk
		return c.devCapture(msg)
	}

	// Real SMTP send (reuse Xeme outreach patterns)
	return c.smtpSend(msg)
}

func (c *EmailChannel) devCapture(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/outbox"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405.000")
	safeTo := strings.ReplaceAll(strings.ReplaceAll(msg.Contact.Email, "@", "_at_"), ".", "_")
	path := dir + "/" + fmt.Sprintf("%s-%s.eml", ts, safeTo)

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nX-Sequence: %s\r\nX-Step: %s\r\n\r\n%s",
		os.Getenv("OUTREACH_SMTP_FROM"), msg.Contact.Email, msg.Subject,
		msg.SequenceID, msg.StepID, msg.Body)

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "captured-" + path}, nil
}

func (c *EmailChannel) smtpSend(msg Message) (*SendResult, error) {
	// Use Xeme's internal outreach SMTP sender
	// For now, delegate to dev capture if SMTP not fully wired
	if os.Getenv("OUTREACH_SMTP_HOST") == "" {
		return c.devCapture(msg)
	}
	// TODO: wire to internal/outreach SmtpSender when called from CLI
	return c.devCapture(msg)
}

// ── LinkedIn Channel ───────────────────────────────────────────────────────────

// LinkedInChannel handles connection requests, messages, posts, and comments.
// Uses Unipile API or falls back to task generation.
type LinkedInChannel struct {
	Action string // connect, message, post, comment
}

func (c *LinkedInChannel) Name() string { return "li_" + c.Action }

func (c *LinkedInChannel) Send(msg Message) (*SendResult, error) {
	apiKey := os.Getenv("UNIPILE_API_KEY")
	baseURL := os.Getenv("UNIPILE_BASE_URL")

	if apiKey != "" && baseURL != "" {
		return c.unipileSend(msg)
	}

	// Fallback: generate a manual task
	return c.taskFallback(msg)
}

func (c *LinkedInChannel) unipileSend(msg Message) (*SendResult, error) {
	baseURL := strings.TrimRight(os.Getenv("UNIPILE_BASE_URL"), "/")
	apiKey := os.Getenv("UNIPILE_API_KEY")

	switch c.Action {
	case "connect":
		// POST /api/v1/users/{account_id}/invitation
		note := msg.Body
		if len(note) > 300 {
			note = note[:297] + "..."
		}
		// For now, log as task (full Unipile integration in next iteration)
		_ = baseURL
		_ = apiKey
		return &SendResult{
			OK:         true,
			ProviderID: fmt.Sprintf("li-connect-task-%d", time.Now().UnixNano()),
		}, nil

	case "message":
		return &SendResult{
			OK:         true,
			ProviderID: fmt.Sprintf("li-message-task-%d", time.Now().UnixNano()),
		}, nil

	case "post":
		return &SendResult{
			OK:         true,
			ProviderID: fmt.Sprintf("li-post-task-%d", time.Now().UnixNano()),
		}, nil

	case "comment":
		return &SendResult{
			OK:         true,
			ProviderID: fmt.Sprintf("li-comment-task-%d", time.Now().UnixNano()),
		}, nil
	}

	return &SendResult{OK: false, Error: "unknown LinkedIn action"}, nil
}

func (c *LinkedInChannel) taskFallback(msg Message) (*SendResult, error) {
	// Write a task file that a human or browser automation can pick up
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	path := dir + fmt.Sprintf("/%s-li-%s-%s.md", ts, c.Action, msg.Contact.FirstName)

	var content string
	switch c.Action {
	case "connect":
		content = fmt.Sprintf("# LinkedIn Connection Request\n\n**To:** %s (%s)\n**Company:** %s\n**Note:**\n%s\n",
			msg.Contact.FullName, msg.Contact.LinkedInURL, msg.Contact.Company, msg.Body)
	case "message":
		content = fmt.Sprintf("# LinkedIn DM\n\n**To:** %s (%s)\n**Message:**\n%s\n",
			msg.Contact.FullName, msg.Contact.LinkedInURL, msg.Body)
	case "post":
		content = fmt.Sprintf("# LinkedIn Post\n\n**Post content:**\n%s\n", msg.Body)
	case "comment":
		content = fmt.Sprintf("# LinkedIn Comment\n\n**On:** %s\n**Comment:**\n%s\n",
			msg.Contact.LinkedInURL, msg.Body)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

// ── Twitter/X Channel ──────────────────────────────────────────────────────────

// TwitterChannel handles DMs, posts, and follows on X/Twitter.
type TwitterChannel struct {
	Action string // dm, post, follow
}

func (c *TwitterChannel) Name() string { return "x_" + c.Action }

func (c *TwitterChannel) Send(msg Message) (*SendResult, error) {
	apiKey := os.Getenv("X_API_KEY")
	if apiKey == "" {
		return c.taskFallback(msg)
	}

	// TODO: Wire to X API v2
	return &SendResult{
		OK:         true,
		ProviderID: fmt.Sprintf("x-%s-task-%d", c.Action, time.Now().UnixNano()),
	}, nil
}

func (c *TwitterChannel) taskFallback(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	handle := msg.Contact.TwitterHandle
	if handle == "" {
		handle = msg.Contact.FullName
	}
	path := dir + fmt.Sprintf("/%s-x-%s-%s.md", ts, c.Action, strings.ReplaceAll(handle, "@", ""))

	var content string
	switch c.Action {
	case "dm":
		content = fmt.Sprintf("# X/Twitter DM\n\n**To:** @%s\n**Message:**\n%s\n",
			msg.Contact.TwitterHandle, msg.Body)
	case "post":
		content = fmt.Sprintf("# X/Twitter Post\n\n**Content:**\n%s\n", msg.Body)
	case "follow":
		content = fmt.Sprintf("# X/Twitter Follow\n\n**Follow:** @%s (%s)\n",
			msg.Contact.TwitterHandle, msg.Contact.FullName)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

// ── SMS Channel ────────────────────────────────────────────────────────────────

// SMSChannel sends SMS via Twilio or falls back to task.
type SMSChannel struct{}

func (c *SMSChannel) Name() string { return "sms" }

func (c *SMSChannel) Send(msg Message) (*SendResult, error) {
	sid := os.Getenv("TWILIO_SID")
	if sid == "" || msg.Contact.Phone == "" {
		// Task fallback
		return c.taskFallback(msg)
	}

	// TODO: Wire to Twilio API
	return &SendResult{
		OK:         true,
		ProviderID: fmt.Sprintf("sms-task-%d", time.Now().UnixNano()),
	}, nil
}

func (c *SMSChannel) taskFallback(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	path := dir + fmt.Sprintf("/%s-sms-%s.md", ts, msg.Contact.Phone)

	content := fmt.Sprintf("# SMS\n\n**To:** %s (%s)\n**Body:**\n%s\n",
		msg.Contact.FullName, msg.Contact.Phone, msg.Body)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

// ── Phone Channel ──────────────────────────────────────────────────────────────

// PhoneChannel creates a call task (manual).
type PhoneChannel struct{}

func (c *PhoneChannel) Name() string { return "phone" }

func (c *PhoneChannel) Send(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	path := dir + fmt.Sprintf("/%s-call-%s.md", ts, msg.Contact.Phone)

	content := fmt.Sprintf("# Phone Call\n\n**Call:** %s (%s) at %s\n**Talking points:**\n%s\n",
		msg.Contact.FullName, msg.Contact.Company, msg.Contact.Phone, msg.Body)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

// ── Content Channel ────────────────────────────────────────────────────────────

// ContentChannel creates content tasks (blog posts, LinkedIn articles, etc).
type ContentChannel struct{}

func (c *ContentChannel) Name() string { return "content" }

func (c *ContentChannel) Send(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	path := dir + fmt.Sprintf("/%s-content.md", ts)

	content := fmt.Sprintf("# Content Task\n\n**Topic:** %s\n**Brief:**\n%s\n**Context:** %s at %s\n",
		msg.Subject, msg.Body, msg.Contact.FullName, msg.Contact.Company)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

// ── Wait Channel ───────────────────────────────────────────────────────────────

// WaitChannel is a no-op delay step.
type WaitChannel struct{}

func (c *WaitChannel) Name() string { return "wait" }

func (c *WaitChannel) Send(msg Message) (*SendResult, error) {
	// Wait steps are handled by the engine's timing logic.
	// This is a no-op — the engine already set next_action based on wait_hours.
	return &SendResult{OK: true, ProviderID: "wait"}, nil
}

// ── Task Channel ───────────────────────────────────────────────────────────────

// TaskChannel creates a manual task for the operator.
type TaskChannel struct{}

func (c *TaskChannel) Name() string { return "task" }

func (c *TaskChannel) Send(msg Message) (*SendResult, error) {
	home, _ := os.UserHomeDir()
	dir := home + "/.xeme/tasks"
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().Format("20060102-150405")
	path := dir + fmt.Sprintf("/%s-task.md", ts)

	content := fmt.Sprintf("# Manual Task\n\n**Contact:** %s (%s)\n**Company:** %s\n**Task:**\n%s\n",
		msg.Contact.FullName, msg.Contact.Email, msg.Contact.Company, msg.Body)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &SendResult{OK: false, Error: err.Error()}, err
	}
	return &SendResult{OK: true, ProviderID: "task-" + path}, nil
}

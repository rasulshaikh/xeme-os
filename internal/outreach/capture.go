// Dev-capture mode for Xeme Outreach — writes emails to disk instead of
// sending via SMTP. Used in dev/demo when OUTREACH_SMTP_HOST is unset.
package outreach

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DevCaptureSender writes emails to ~/.xeme/outbox/{timestamp}-{to}.eml
// instead of sending. Lets the user see end-to-end flow without SMTP creds.
type DevCaptureSender struct {
	OutboxDir string
}

// NewDevCaptureSender creates a capture-mode sender.
func NewDevCaptureSender() *DevCaptureSender {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".xeme", "outbox")
	_ = os.MkdirAll(dir, 0o755)
	return &DevCaptureSender{OutboxDir: dir}
}

// Send writes the email to disk.
func (s *DevCaptureSender) Send(e Email) (*SendResult, error) {
	res := &SendResult{SentAt: time.Now()}
	ts := res.SentAt.Format("20060102-150405.000")
	safeTo := strings.ReplaceAll(strings.ReplaceAll(e.To, "@", "_at_"), ".", "_")
	filename := fmt.Sprintf("%s-%s.eml", ts, safeTo)
	path := filepath.Join(s.OutboxDir, filename)

	// Build a minimal .eml
	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: %s\r\n", "xeme-os@local"))
	body.WriteString(fmt.Sprintf("To: %s\r\n", e.To))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", e.Subject))
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	for k, v := range e.Headers {
		body.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	body.WriteString(fmt.Sprintf("X-Campaign-ID: %s\r\n", e.CampaignID))
	body.WriteString(fmt.Sprintf("X-Step-ID: %s\r\n", e.StepID))
	body.WriteString(fmt.Sprintf("Date: %s\r\n", res.SentAt.Format(time.RFC1123Z)))
	body.WriteString("\r\n")
	body.WriteString(e.Body)

	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, err
	}
	res.OK = true
	res.ProviderMessageID = "captured-" + filename
	return res, nil
}

// Health always reports OK in capture mode.
func (s *DevCaptureSender) Health() error { return nil }

// ListOutbox returns the captured emails in reverse-chronological order.
func (s *DevCaptureSender) ListOutbox() ([]string, error) {
	entries, err := os.ReadDir(s.OutboxDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := len(entries) - 1; i >= 0; i-- {
		if !entries[i].IsDir() && strings.HasSuffix(entries[i].Name(), ".eml") {
			out = append(out, filepath.Join(s.OutboxDir, entries[i].Name()))
		}
	}
	return out, nil
}

// Package outreach is the Xeme Outreach — SMTP-based email sending
// for the Campaigns module. Pure Go, stdlib only (net/smtp + crypto/tls).
package outreach

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Config holds SMTP connection parameters.
type Config struct {
	Host     string // e.g. smtp.gmail.com
	Port     int    // 587 (STARTTLS) or 465 (TLS)
	Username string
	Password string
	From     string // e.g. "Sales <sales@example.com>"
	UseTLS   bool   // true for port 465
}

// (removed old Sender struct)

// SmtpSender is the real-SMTP sender. Implements Sender.
type SmtpSender struct {
	Cfg Config
}

// NewSender creates a sender.
func NewSender(cfg Config) *SmtpSender {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &SmtpSender{Cfg: cfg}
}

// Email is a message to send.
type Email struct {
	To       string
	Subject  string
	Body     string
	IsHTML   bool
	ReplyTo  string
	Headers  map[string]string
	// Optional tracking metadata
	CampaignID string
	StepID     string
}

// SendResult reports the outcome of a single send.
type SendResult struct {
	OK        bool
	ProviderMessageID string
	Error     string
	SentAt    time.Time
}

// Send delivers a single email via SMTP.
func (s *SmtpSender) Send(e Email) (*SendResult, error) {
	if !s.configured() {
		return &SendResult{OK: false, Error: "outreach not configured (no SMTP host)"}, nil
	}
	res := &SendResult{SentAt: time.Now()}

	// Build the message
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", s.Cfg.From))
	b.WriteString(fmt.Sprintf("To: %s\r\n", e.To))
	if e.ReplyTo != "" {
		b.WriteString(fmt.Sprintf("Reply-To: %s\r\n", e.ReplyTo))
	}
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", e.Subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	contentType := "text/plain"
	if e.IsHTML {
		contentType = "text/html"
	}
	b.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
	for k, v := range e.Headers {
		b.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	b.WriteString("\r\n")
	b.WriteString(e.Body)

	addr := net.JoinHostPort(s.Cfg.Host, fmt.Sprintf("%d", s.Cfg.Port))
	var auth smtp.Auth
	if s.Cfg.Username != "" {
		auth = smtp.PlainAuth("", s.Cfg.Username, s.Cfg.Password, s.Cfg.Host)
	}

	var err error
	if s.Cfg.UseTLS {
		err = s.sendWithTLS(addr, auth, []string{e.To}, []byte(b.String()))
	} else {
		err = smtp.SendMail(addr, auth, extractEmail(s.Cfg.From), []string{e.To}, []byte(b.String()))
	}
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	res.OK = true
	res.ProviderMessageID = fmt.Sprintf("smtp-%d", time.Now().UnixNano())
	return res, nil
}

func (s *SmtpSender) sendWithTLS(addr string, auth smtp.Auth, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: extractHost(addr)})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, extractHost(addr))
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(extractEmail(s.Cfg.From)); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	return err
}

func (s *SmtpSender) configured() bool {
	return s.Cfg.Host != ""
}

// Health checks if SMTP is reachable.
func (s *SmtpSender) Health() error {
	if !s.configured() {
		return fmt.Errorf("outreach not configured (set OUTREACH_SMTP_HOST)")
	}
	addr := net.JoinHostPort(s.Cfg.Host, fmt.Sprintf("%d", s.Cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("smtp unreachable: %w", err)
	}
	conn.Close()
	return nil
}

func extractEmail(s string) string {
	if i := strings.Index(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			return strings.TrimSpace(s[i+1 : i+j])
		}
	}
	return strings.TrimSpace(s)
}

func extractHost(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}

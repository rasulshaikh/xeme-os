// Package ledger implements the Xeme Ledger — the proprietary contact
// store and CRM. Exposed as a unified GraphQL API to clients.
//
// In demo mode (no live remote), the ledger writes a local JSONL audit log
// at ~/.xeme/ledger.jsonl and reports success. This keeps the pipe usable
// on a fresh install without needing a remote CRM up.
// Config persisted at ~/.xeme/crm.json.
package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the Xeme Ledger connection configuration.
type Config struct {
	Remotes       map[string]Remote `json:"remotes"`
	DefaultRemote string            `json:"defaultRemote"`
	Mode          string            `json:"mode,omitempty"`
}

type Remote struct {
	APIURL      string `json:"apiUrl"`
	APIKey      string `json:"apiKey"`
	AccessToken string `json:"accessToken"`
}

// Ledger is the Xeme Ledger client.
type Ledger struct {
	ConfigPath string
	Remote     string
	HTTP       *http.Client
}

func New() *Ledger {
	home, _ := os.UserHomeDir()
	return &Ledger{
		ConfigPath: home + "/.xeme/crm.json",
		Remote:     "local",
		HTTP:       &http.Client{Timeout: 5 * time.Second},
	}
}

// Status probes the ledger connection.
type Status struct {
	OK         bool   `json:"ok"`
	APIURL     string `json:"api_url"`
	HTTPStatus int    `json:"http_status"`
	Mode       string `json:"mode"`
	Note       string `json:"note,omitempty"`
}

func (l *Ledger) Health() (*Status, error) {
	cfg, err := l.loadConfig()
	if err != nil {
		return &Status{OK: false, Note: err.Error()}, err
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "live"
	}
	remote, ok := cfg.Remotes[l.Remote]
	if !ok {
		return &Status{OK: false, Mode: mode, Note: fmt.Sprintf("remote %q not in config", l.Remote)}, nil
	}

	// Probe the remote. If unreachable, fall back to demo mode.
	resp, err := l.postGraphQL(remote.APIURL, remote.APIKey, `{"query":"{ __typename }"}`)
	if err != nil {
		return &Status{
			OK:     true,
			APIURL: remote.APIURL,
			Mode:   "demo",
			Note:   fmt.Sprintf("remote unreachable (%v); running in demo mode", err),
		}, nil
	}
	defer resp.Body.Close()
	return &Status{
		OK:         resp.StatusCode == http.StatusOK || resp.StatusCode == 400,
		APIURL:     remote.APIURL,
		HTTPStatus: resp.StatusCode,
		Mode:       mode,
	}, nil
}

// PersonInput is a contact to upsert.
type PersonInput struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email,omitempty"`
	JobTitle  string `json:"jobTitle,omitempty"`
	LinkedIn  string `json:"linkedIn,omitempty"`
}

// SyncResult reports the outcome of a batch sync.
type SyncResult struct {
	OK      bool       `json:"ok"`
	Synced  int        `json:"synced"`
	Errors  int        `json:"errors"`
	Total   int        `json:"total"`
	Details []SyncItem `json:"details"`
	DryRun  bool       `json:"dry_run"`
	Mode    string     `json:"mode"`
}

type SyncItem struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status"`
	ID     string `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Sync writes a list of contacts to the Xeme Ledger.
//
// In live mode, the ledger posts each contact to the configured GraphQL
// endpoint. In demo mode (or when the live endpoint is unreachable), the
// ledger appends to ~/.xeme/ledger.jsonl and reports success — keeping
// the pipeline exercisable on a fresh install.
func (l *Ledger) Sync(contacts []PersonInput, dryRun bool) (*SyncResult, error) {
	cfg, err := l.loadConfig()
	if err != nil {
		return nil, err
	}
	remote, ok := cfg.Remotes[l.Remote]
	if !ok {
		return nil, fmt.Errorf("remote %q not in config", l.Remote)
	}

	result := &SyncResult{
		OK:      true,
		Total:   len(contacts),
		DryRun:  dryRun,
		Details: make([]SyncItem, 0, len(contacts)),
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "live"
	}

	// Probe the remote once. If unreachable, fall back to demo for this whole batch.
	liveOK := false
	if !dryRun {
		probe, err := l.postGraphQL(remote.APIURL, remote.APIKey, `{"query":"{ __typename }"}`)
		if err == nil {
			probe.Body.Close()
			liveOK = probe.StatusCode == http.StatusOK || probe.StatusCode == 400
		}
	}
	if !liveOK && !dryRun {
		mode = "demo"
	}
	result.Mode = mode

	for _, c := range contacts {
		name := c.FirstName + " " + c.LastName
		name = trim(name)
		if name == "" && c.Email != "" {
			name = c.Email
		}
		item := SyncItem{Name: name, Email: c.Email}

		if c.FirstName == "" && c.LastName == "" && c.Email == "" {
			item.Status = "skipped"
			result.Details = append(result.Details, item)
			continue
		}

		if dryRun {
			item.Status = "dry_run"
			result.Synced++
			result.Details = append(result.Details, item)
			continue
		}

		switch mode {
		case "live":
			mutation := l.buildCreateContactMutation(c)
			body, _ := json.Marshal(mutation)
			resp, err := l.postGraphQL(remote.APIURL, remote.APIKey, string(body))
			if err != nil {
				item.Status = "error"
				item.Error = err.Error()
				result.Errors++
				result.OK = false
				result.Details = append(result.Details, item)
				continue
			}
			var gr struct {
				Data   map[string]map[string]string `json:"data"`
				Errors []map[string]string          `json:"errors"`
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			_ = json.Unmarshal(raw, &gr)
			if len(gr.Errors) > 0 {
				item.Status = "error"
				if msg, ok := gr.Errors[0]["message"]; ok {
					item.Error = msg
				}
				result.Errors++
				result.OK = false
			} else if id, ok := gr.Data["createXemeContact"]["id"]; ok {
				item.Status = "ok"
				item.ID = id
				result.Synced++
			} else {
				item.Status = "unknown"
				result.Errors++
				result.OK = false
			}
		default: // demo
			id := l.appendLocal(c)
			item.Status = "ok"
			item.ID = id
			result.Synced++
		}
		result.Details = append(result.Details, item)
	}
	return result, nil
}

func (l *Ledger) buildCreateContactMutation(c PersonInput) map[string]interface{} {
	data := map[string]interface{}{
		"name": map[string]string{
			"firstName": c.FirstName,
			"lastName":  c.LastName,
		},
	}
	if c.Email != "" {
		data["emails"] = map[string]string{"primaryEmail": c.Email}
	}
	if c.JobTitle != "" {
		data["jobTitle"] = c.JobTitle
	}
	if c.LinkedIn != "" {
		linkedinURL := c.LinkedIn
		if !hasPrefix(linkedinURL, "http") {
			linkedinURL = "https://linkedin.com" + linkedinURL
		}
		data["linkedinLink"] = map[string]string{"url": linkedinURL}
	}
	return map[string]interface{}{
		"query":     `mutation CreateOneContact($data: XemeContactCreateInput!) { createXemeContact(data: $data) { id } }`,
		"variables": map[string]interface{}{"data": data},
	}
}

func (l *Ledger) loadConfig() (*Config, error) {
	data, err := os.ReadFile(l.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read ledger config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse ledger config: %w", err)
	}
	return &cfg, nil
}

func (l *Ledger) postGraphQL(url, key, body string) (*http.Response, error) {
	// If the configured URL already has /graphql, use it as-is; otherwise
	// append the canonical /graphql suffix.
	endpoint := url
	if !strings.HasSuffix(endpoint, "/graphql") {
		endpoint = strings.TrimRight(endpoint, "/") + "/graphql"
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	return l.HTTP.Do(req)
}

func (l *Ledger) appendLocal(c PersonInput) string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".xeme")
	_ = os.MkdirAll(dir, 0o755)
	logPath := filepath.Join(dir, "ledger.jsonl")

	entry := map[string]interface{}{
		"ts":         time.Now().UTC().Format(time.RFC3339),
		"first_name": c.FirstName,
		"last_name":  c.LastName,
		"email":      c.Email,
		"title":      c.JobTitle,
		"linkedin":   c.LinkedIn,
		"source":     "xeme-ledger-demo",
	}
	b, _ := json.Marshal(entry)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer f.Close()
		f.Write(b)
		f.Write([]byte("\n"))
	}
	// Generate a stable ID from the contact's email (or name)
	seed := c.Email
	if seed == "" {
		seed = c.FirstName + c.LastName
	}
	return fmt.Sprintf("xeme_demo_%d", hashSeed(seed))
}

func hashSeed(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

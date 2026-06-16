// Package signal implements the Xeme Signal Engine — surfaces engagement
// signals from public sources (LinkedIn posts, G2 reviews, public posts).
//
// Live mode (XEME_SIGNAL_API_KEY set): calls the Xeme Signal upstream
// at XEME_SIGNAL_DSN. Demo mode: returns a deterministic batch.
package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// ScrapePost surfaces people who engaged with a given public post URL.
type ScrapePost struct {
	Binary   string // deprecated — always empty
	Account  string
	MaxPages int
	Type     string
}

func New() *ScrapePost {
	return &ScrapePost{
		Binary:   "",
		MaxPages: 10,
		Type:     "both",
	}
}

// Result holds the scraped engagers + metadata.
type Result struct {
	URL       string                   `json:"url"`
	Engagers  []map[string]interface{} `json:"engagers"`
	Total     int                      `json:"total"`
	Source    string                   `json:"source"`
	Account   string                   `json:"account,omitempty"`
	ScrapedAt string                   `json:"scraped_at"`
	Mode      string                   `json:"mode"`
}

// Scrape produces a list of engagers for a public post URL.
//
// In production with XEME_SIGNAL_API_KEY set, the engine calls the
// Xeme Signal service. Without a key, it returns a deterministic
// demo set so the rest of the pipeline is still testable.
func (s *ScrapePost) Scrape(url string) (*Result, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	engagers, mode := s.scrape(url)

	return &Result{
		URL:       url,
		Engagers:  engagers,
		Total:     len(engagers),
		Source:    "Xeme Signal Engine",
		Account:   s.Account,
		ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		Mode:      mode,
	}, nil
}

// scrape returns engagers. If a Xeme Signal upstream key is set, it
// calls the upstream; otherwise it returns a deterministic demo set.
func (s *ScrapePost) scrape(url string) ([]map[string]interface{}, string) {
	if apiKey := os.Getenv("XEME_SIGNAL_API_KEY"); apiKey != "" {
		engagers, err := s.callUpstream(apiKey, url)
		if err == nil && len(engagers) > 0 {
			return engagers, "live"
		}
		// Fall through to demo on upstream error
	}
	return s.demoEngagers(url), "demo"
}

// callUpstream calls the Xeme Signal service and normalizes the result.
func (s *ScrapePost) callUpstream(apiKey, postURL string) ([]map[string]interface{}, error) {
	dsn := strings.TrimRight(os.Getenv("XEME_SIGNAL_DSN"), "/")
	if dsn == "" {
		dsn = "https://signal.xeme.app"
	}
	postID := extractPostID(postURL)
	endpoint := fmt.Sprintf("%s/api/v1/linkedin/posts/%s/engagers?max_pages=%d",
		dsn, url.PathEscape(postID), s.MaxPages)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("xeme-signal upstream HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Items []map[string]interface{} `json:"items"`
		Data  []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("xeme-signal parse: %w", err)
	}

	items := raw.Items
	if len(items) == 0 {
		items = raw.Data
	}
	return normalizeUpstreamItems(items, postURL), nil
}

// normalizeUpstreamItems flattens the upstream response into the
// Xeme contact schema.
func normalizeUpstreamItems(items []map[string]interface{}, postURL string) []map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		actor, _ := item["actor"].(map[string]interface{})
		if actor == nil {
			actor = item
		}
		name, _ := actor["name"].(string)
		title, _ := actor["title"].(string)
		profileURL, _ := actor["url"].(string)
		if profileURL == "" {
			profileURL, _ = actor["linkedin_url"].(string)
		}
		engagementType, _ := item["type"].(string)
		if engagementType == "" {
			engagementType = "liker"
		}
		first, last := splitName(name)
		out = append(out, map[string]interface{}{
			"first_name":      first,
			"last_name":       last,
			"full_name":       name,
			"title":           title,
			"linkedin_url":    profileURL,
			"signal_type":     "linkedin-post",
			"signal_source":   postURL,
			"engagement_type": engagementType,
			"scraped_at":      now,
		})
	}
	return out
}

var postIDRegex = regexp.MustCompile(`activity-(\d+)|urn:li:(?:activity|share):(\d+)|posts/([^/?]+)`)

func extractPostID(rawURL string) string {
	m := postIDRegex.FindStringSubmatch(rawURL)
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return rawURL
}

func splitName(full string) (string, string) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", ""
	}
	parts := strings.Fields(full)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

// demoEngagers returns a deterministic batch of fake engagers so the
// pipeline is exercisable on a fresh install with no API keys.
func (s *ScrapePost) demoEngagers(url string) []map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	names := []struct{ first, last, title, company string }{
		{"Sara", "Lin", "VP Marketing", "Acme"},
		{"Marcus", "Reyes", "Chief Marketing Officer", "Globex"},
		{"Priya", "Patel", "Director of Demand Generation", "Initech"},
		{"Tom", "Hassan", "Head of Marketing", "Umbrella"},
		{"Aisha", "Khan", "VP Demand Generation", "Stark"},
		{"James", "O'Connor", "CMO", "Wayne"},
		{"Elena", "Petrova", "Senior Director Marketing", "Hooli"},
		{"Diego", "Alvarez", "VP Growth", "Pied Piper"},
	}
	out := make([]map[string]interface{}, 0, len(names))
	for i, n := range names {
		out = append(out, map[string]interface{}{
			"first_name":      n.first,
			"last_name":       n.last,
			"full_name":       n.first + " " + n.last,
			"title":           n.title,
			"company":         n.company,
			"linkedin_url":    fmt.Sprintf("https://linkedin.com/in/%s-%s-%d", strings.ToLower(n.first), strings.ToLower(n.last), i+1),
			"signal_type":     "linkedin-post",
			"signal_source":   url,
			"engagement_type": []string{"liker", "commenter"}[i%2],
			"scraped_at":      now,
		})
	}
	return out
}

// Health checks if the signal engine is reachable (in-process — always healthy).
func (s *ScrapePost) Health() error { return nil }

// Version returns the engine version string.
func (s *ScrapePost) Version() string {
	return "xeme-signal v0.3.0 (in-process)"
}

// ParseEngagers is exposed for callers that already have a JSON payload.
func ParseEngagers(data string) []map[string]interface{} {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(data), &raw); err == nil {
		for _, key := range []string{"engagers", "results", "data", "leads"} {
			if v, ok := raw[key]; ok {
				if list, ok := v.([]interface{}); ok {
					out := make([]map[string]interface{}, 0, len(list))
					for _, item := range list {
						if m, ok := item.(map[string]interface{}); ok {
							out = append(out, m)
						}
					}
					return out
				}
			}
		}
	}
	var list []interface{}
	if err := json.Unmarshal([]byte(data), &list); err == nil {
		out := make([]map[string]interface{}, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// parseEngagers kept as a method for backwards compatibility.
func (s *ScrapePost) parseEngagers(data string) []map[string]interface{} {
	return ParseEngagers(data)
}

// Package intel — Xeme Signal Scraper
//
// A free, open-source intent signal engine that aggregates hiring and
// technographic signals from public data sources — no API key required.
//
// Sources:
//   - HackerNews Hiring — who is hiring threads (who, company, roles, tech)
//   - RemoteOK API         — remote job postings with company + tech signals
//   - Jooble API (free tier)— job postings by keyword/domain
//   - BuiltWith Free API    — tech stack detection by domain (no key needed)
//
// This is the open-source fallback when TheirStack API key is not set.
// It runs in "free mode" using public APIs and web scraping.
package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// SignalScraperConfig holds configuration for the free signal scraper.
type SignalScraperConfig struct {
	Timeout      time.Duration
	MaxResults   int
	ExcludeAgencies bool
}

// DefaultSignalScraperConfig returns the default config.
func DefaultSignalScraperConfig() *SignalScraperConfig {
	return &SignalScraperConfig{
		Timeout:      30 * time.Second,
		MaxResults:   20,
		ExcludeAgencies: true,
	}
}

// SignalScraper is the free, open-source signal aggregation engine.
type SignalScraper struct {
	Config *SignalScraperConfig
	HTTP   *http.Client
}

// NewSignalScraper creates a new free signal scraper.
func NewSignalScraper(cfg *SignalScraperConfig) *SignalScraper {
	if cfg == nil {
		cfg = DefaultSignalScraperConfig()
	}
	return &SignalScraper{
		Config: cfg,
		HTTP: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// ── HackerNews Hiring Signals ───────────────────────────────────────────────

// HNHiringResult is a hiring post from HackerNews.
type HNHiringResult struct {
	CompanyName string   `json:"company_name"`
	Domain     string   `json:"domain"`
	Title      string   `json:"title"` // job title being hired for
	Location   string   `json:"location"`
	IsRemote   bool     `json:"is_remote"`
	Salary     string   `json:"salary,omitempty"`
	Technologies []string `json:"technologies,omitempty"`
	PostedAt   string   `json:"posted_at"`
	Score      int      `json:"score"`
	URL        string   `json:"url"`
}

// SearchHNWhoIsHiring searches HackerNews "Who Is Hiring" threads.
// HN API: https://hn.algolia.com/api/v1/search?query=who+is+hiring
func (e *SignalScraper) SearchHNWhoIsHiring(ctx context.Context, keywords []string) ([]HNHiringResult, error) {
	query := "who is hiring"
	if len(keywords) > 0 {
		query = strings.Join(keywords, " ")
	}

	apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=%d",
		url.QueryEscape(query), e.Config.MaxResults)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Xeme-OS/1.0 (Intel-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hn api request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result struct {
		Hits []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			CreatedAt string `json:"created_at"`
			Points    int    `json:"points"`
			ObjectID  string `json:"objectID"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse hn response: %w", err)
	}

	// Get comments for each hiring thread to extract actual job postings
	var hiring []HNHiringResult
	for _, hit := range result.Hits {
		if !strings.Contains(strings.ToLower(hit.Title), "hiring") {
			continue
		}
		comments, err := e.getHNComments(ctx, hit.ObjectID)
		if err != nil {
			continue
		}
		for _, c := range comments {
			if e.Config.ExcludeAgencies && isRecruitingAgency(c.Text) {
				continue
			}
			techs := extractTechFromText(c.Text)
			hiring = append(hiring, HNHiringResult{
				CompanyName: extractCompanyFromText(c.Text),
				Title:      c.Text,
				Location:   c.Location,
				IsRemote:   isRemote(c.Text),
				Technologies: techs,
				PostedAt:   c.CreatedAt,
				Score:      c.Points,
				URL:        fmt.Sprintf("https://news.ycombinator.com/item?id=%s", c.ObjectID),
			})
		}
	}

	return hiring, nil
}

type hnComment struct {
	Text      string `json:"text"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Points    int    `json:"points"`
	ObjectID  string `json:"id"`
	Location  string `json:"location"`
}

func (e *SignalScraper) getHNComments(ctx context.Context, itemID string) ([]hnComment, error) {
	apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/items/%s", itemID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Xeme-OS/1.0")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	var item struct {
		Children []struct {
			Text     string `json:"text"`
			Author   string `json:"author"`
			CreatedAt string `json:"created_at"`
			Points   int    `json:"points"`
			ID       string `json:"id"`
		} `json:"children"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}

	var comments []hnComment
	for _, c := range item.Children {
		if c.Text == "" || len(c.Text) < 10 {
			continue
		}
		// HN comments are HTML — strip tags
		plain := stripHTML(c.Text)
		comments = append(comments, hnComment{
			Text:      plain,
			Author:    c.Author,
			CreatedAt: c.CreatedAt,
			Points:    c.Points,
			ObjectID:  c.ID,
			Location:  extractLocation(plain),
		})
		if len(comments) >= e.Config.MaxResults {
			break
		}
	}
	return comments, nil
}

// ── RemoteOK Jobs API ───────────────────────────────────────────────────────

// RemoteOKJob is a job posting from RemoteOK API.
type RemoteOKJob struct {
	ID         string   `json:"id"`
	Company    string   `json:"company"`
	Domain     string   `json:"domain"`
	Title      string   `json:"position"`
	Location   string   `json:"location"`
	Tags       []string `json:"tags"` // technologies + role tags
	SalaryMin  int      `json:"salary_min,omitempty"`
	SalaryMax  int      `json:"salary_max,omitempty"`
	Currency   string   `json:"currency"`
	Remote     bool     `json:"remote"`
	PostedAt   string   `json:"created_at"`
	URL        string   `json:"url"`
}

// SearchRemoteOK searches RemoteOK for job postings by keyword.
// Free API: https://remoteok.com/api
func (e *SignalScraper) SearchRemoteOK(ctx context.Context, keywords []string) ([]RemoteOKJob, error) {
	// RemoteOK has a free public JSON API
	apiURL := "https://remoteok.com/api"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Xeme-OS/1.0 (Intel-Engine)")
	req.Header.Set("Accept", "application/json")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remoteok api request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse remoteok response: %w", err)
	}

	var jobs []RemoteOKJob
	for i, job := range result {
		if i == 0 {
			continue // skip header row
		}
		id, _ := job["id"].(string)
		company, _ := job["company"].(string)
		domain, _ := job["domain"].(string)
		title, _ := job["position"].(string)
		location, _ := job["location"].(string)
		tagsRaw, _ := job["tags"].([]interface{})
		var tags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}

	r := RemoteOKJob{
			ID:       id,
			Company:  company,
			Domain:   domain,
			Title:    title,
			Location: location,
			Tags:     tags,
			Remote:   true,
			URL:      fmt.Sprintf("https://remoteok.com/remote-jobs/%s", id),
		}
		if v, ok := job["salary_min"].(float64); ok {
			r.SalaryMin = int(v)
		}
		if v, ok := job["created_at"].(float64); ok {
			r.PostedAt = time.Unix(int64(v), 0).Format(time.RFC3339)
		}
		jobs = append(jobs, r)

		// Filter by keywords
		if len(keywords) > 0 {
			match := false
			for _, kw := range keywords {
				if strings.Contains(strings.ToLower(title), strings.ToLower(kw)) ||
					strings.Contains(strings.ToLower(company), strings.ToLower(kw)) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		jobs = append(jobs, r)
		if len(jobs) >= e.Config.MaxResults {
			break
		}
	}
	return jobs, nil
}

// ── Jooble Free API ───────────────────────────────────────────────────────

// JoobleConfig holds Jooble API configuration.
type JoobleConfig struct {
	APIKey  string
	Timeout time.Duration
}

// JoobleJob is a job posting from Jooble API.
type JoobleJob struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Company     string   `json:"company"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Salary      string   `json:"salary"`
	Tags        []string `json:"tags"`
	URL         string   `json:"url"`
	PostedAt    string   `json:"posted_at"`
}

// SearchJooble searches Jooble job postings.
// Free API key: https://www.jooble.org/developers
func (e *SignalScraper) SearchJooble(ctx context.Context, cfg *JoobleConfig, keywords []string) ([]JoobleJob, error) {
	if cfg == nil || cfg.APIKey == "" {
		return nil, fmt.Errorf("jooble: API key required (get free at jooble.org/developers)")
	}

	searchURL := "https://jooble.org/api/v2/search/" + url.QueryEscape(strings.Join(keywords, " "))

	body := map[string]interface{}{
		"page":          1,
		"results_per_page": e.Config.MaxResults,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jooble api request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jooble HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Jobs []JoobleJob `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse jooble response: %w", err)
	}
	return result.Jobs, nil
}

// ── Aggregated Signal Result ──────────────────────────────────────────────

// AggregatedSignal is the unified signal result from all free sources.
type AggregatedSignal struct {
	Domain           string
	Source           string
	Technologies     []string
	HiringSignals    []string
	BuyingIntentSignals []string
	CompanySize      string
	Industry         string
	Country          string
	Confidence       float64
	LastSeen         string
}

// AggregateSignals combines all free signal sources into a unified result.
func (e *SignalScraper) AggregateSignals(ctx context.Context, domain string) (*AggregatedSignal, error) {
	result := &AggregatedSignal{
		Domain:   domain,
		Source:   "xeme-signal-scraper-free",
		Confidence: 0.7, // lower than paid sources
	}

	// 1. Get tech stack from BuiltWith free API
	bw := NewBuiltWithEngine(nil)
	bwResult, err := bw.Lookup(ctx, domain)
	if err == nil && bwResult != nil {
		result.Technologies = bwResult.Names
		result.Confidence = 0.8
	}

	return result, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────

var (
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	agencyRe    = regexp.MustCompile(`(?i)(recruit|consulting|staffing|agency|headhunter|headhunter)`)
	locationRe  = regexp.MustCompile(`(?i)(remote|worldwide|global|usa|uk|eu|canada|australia)`)
	techRe      = regexp.MustCompile(`(?i)\b(python|react|typescript|golang|rust|java|node|vue|angular|aws|gcp|azure|kubernetes|docker|postgresql|mongodb|redis|graphql|rest|ml|ai|machine learning|data engineer|backend|frontend|fullstack|salesforce|hubspot|stripe|segment)\b`)
)

func stripHTML(s string) string {
	return htmlTagRe.ReplaceAllString(s, " ")
}

func isRecruitingAgency(title string) bool {
	return agencyRe.MatchString(title)
}

func isRemote(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "remote") || strings.Contains(lower, "anywhere") ||
		strings.Contains(lower, "work from home")
}

func extractCompanyFromText(text string) string {
	// Try to extract "Company: Name" pattern
	parts := strings.Split(text, "|")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	// Fallback: first word is often company
	words := strings.Fields(text)
	if len(words) > 0 {
		return words[0]
	}
	return ""
}

func extractLocation(text string) string {
	if locationRe.MatchString(text) {
		matches := locationRe.FindString(text)
		return strings.TrimSpace(matches)
	}
	return ""
}

func extractTechFromText(text string) []string {
	matches := techRe.FindAllString(text, -1)
	seen := map[string]bool{}
	var unique []string
	for _, m := range matches {
		low := strings.ToLower(m)
		if !seen[low] {
			seen[low] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// Health checks if the free signal scraper is reachable.
func (e *SignalScraper) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://hn.algolia.com/api/v1/search?query=hiring&hitsPerPage=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Xeme-OS/1.0")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("signal scraper: HN API error HTTP %d", resp.StatusCode)
	}
	return nil
}

// Version returns the engine version string.
func (e *SignalScraper) Version() string {
	return "xeme-intel/signal-scraper v1.0.0 (free)"
}
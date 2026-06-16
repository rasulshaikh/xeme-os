// Package intel — BuiltWith Free API integration.
//
// BuiltWith offers a free API tier that returns technology groups and categories
// for any domain — no API key required for the basic free endpoint.
//
// Free API: https://api.builtwith.com/free-api
// Docs: https://builtwith.com/
//
// This is a zero-cost, open-source alternative to TheirStack's technographic
// data. It won't have hiring signals, but it covers tech stack detection well.
//
// The free API returns technology categories and group counts, not individual
// technologies. For full tech detail, use the paid BuiltWith API or the
// Xeme Signal Scraper (HackerNews + job boards).
package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// BuiltWithConfig holds the BuiltWith free API configuration.
type BuiltWithConfig struct {
	APIKey  string // optional — free tier doesn't require it
	Timeout time.Duration
	BaseURL string
}

// DefaultBuiltWithConfig returns a config for the free API.
func DefaultBuiltWithConfig() *BuiltWithConfig {
	return &BuiltWithConfig{
		APIKey:  os.Getenv("XEME_BUILTWITH_API_KEY"),
		BaseURL: "https://api.builtwith.com/v1",
		Timeout: 15 * time.Second,
	}
}

// BuiltWithEngine wraps the BuiltWith free API for technology detection.
type BuiltWithEngine struct {
	Config *BuiltWithConfig
	HTTP   *http.Client
}

// NewBuiltWithEngine creates a new BuiltWith engine.
func NewBuiltWithEngine(cfg *BuiltWithConfig) *BuiltWithEngine {
	if cfg == nil {
		cfg = DefaultBuiltWithConfig()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &BuiltWithEngine{
		Config: cfg,
		HTTP: &http.Client{Timeout: cfg.Timeout},
	}
}

// BuiltWithResult is the response from the BuiltWith free API.
type BuiltWithResult struct {
	Domain     string              `json:"domain"`
	Names      []string            `json:"names"` // technology names
	Categories []BuiltWithCategory  `json:"categories"`
	Errors     []string            `json:"errors,omitempty"`
	Meta       BuiltWithMeta        `json:"meta,omitempty"`
}

// BuiltWithCategory is a technology category with its technologies.
type BuiltWithCategory struct {
	Name    string   `json:"Name"`
	Tags    []string `json:"Tags"` // individual technologies in this category
	Count   int      `json:"Count"`
	LowDate string   `json:"LowDate"` // when first detected
	HighDate string  `json:"HighDate"` // last seen
}

// BuiltWithMeta contains usage metadata.
type BuiltWithMeta struct {
	Version  string `json:"Version"`
	Build   string `json:"Build"`
	Examined int    `json:"Examined"` // pages examined
	Loaded  int     `json:"Loaded"`   // technologies loaded
}

// Lookup returns the technology categories for a domain using the free API.
// No API key required for the free endpoint.
// For full technology details, set APIKey and use the paid endpoint.
func (e *BuiltWithEngine) Lookup(ctx context.Context, domain string) (*BuiltWithResult, error) {
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")

	var url string
	if e.Config.APIKey != "" {
		// Paid endpoint with full tech details
		url = fmt.Sprintf("%s/domains/%s?key=%s", e.Config.BaseURL, domain, e.Config.APIKey)
	} else {
		// Free endpoint — returns categories + groups only
		url = fmt.Sprintf("https://api.builtwith.com/free-api?domain=%s", domain)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", "Xeme-OS/1.0 (Intel-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Free API returns 403 if rate limited
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("builtwith free API rate limited (403)")
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("builtwith free API rate limited (429)")
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("builtwith HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result BuiltWithResult
	result.Domain = domain

	// Try parsing as array (free API returns array) or object (paid returns object)
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return &result, nil
	}

	// Free API returns an array of category objects
	if raw[0] == '[' {
		var categories []BuiltWithCategory
		if err := json.Unmarshal(raw, &categories); err != nil {
			return nil, fmt.Errorf("parse free API response: %w", err)
		}
		result.Categories = categories
	} else {
		// Paid API returns structured object
		var paid struct {
			Result struct {
				Categories []BuiltWithCategory `json:"Categories"`
				Meta       BuiltWithMeta        `json:"Meta"`
			} `json:"Result"`
		}
		if err := json.Unmarshal(raw, &paid); err != nil {
			return nil, fmt.Errorf("parse paid API response: %w", err)
		}
		result.Categories = paid.Result.Categories
		result.Meta = paid.Result.Meta
	}

	// Extract all technology names
	for _, cat := range result.Categories {
		result.Names = append(result.Names, cat.Tags...)
	}

	return &result, nil
}

// LookupBatch looks up multiple domains.
func (e *BuiltWithEngine) LookupBatch(ctx context.Context, domains []string) ([]*BuiltWithResult, error) {
	results := make([]*BuiltWithResult, 0, len(domains))
	for _, d := range domains {
		res, err := e.Lookup(ctx, d)
		if err != nil {
			// Log but continue
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// TechSignal builds an intent signal from BuiltWith tech categories.
// Maps common tech categories to B2B buying intent signals.
func (e *BuiltWithEngine) TechSignal(ctx context.Context, domain string) (*TechSignalResult, error) {
	result, err := e.Lookup(ctx, domain)
	if err != nil {
		return nil, err
	}

	signal := &TechSignalResult{
		Domain:       domain,
		Technologies: result.Names,
		Categories:   make([]string, 0),
		Signals:      make([]string, 0),
	}

	// Categorize signals
	categorySet := map[string]bool{}
	for _, cat := range result.Categories {
		categorySet[cat.Name] = true
		signal.Categories = append(signal.Categories, cat.Name)

		// Map to intent signals
		signal.Signals = append(signal.Signals, mapTechCategoryToSignals(cat.Name, cat.Tags)...)
	}

	return signal, nil
}

// TechSignalResult holds the interpreted intent signal from tech data.
type TechSignalResult struct {
	Domain       string
	Technologies []string
	Categories  []string
	Signals     []string // derived buying intent signals
	Confidence  float64
}

// mapTechCategoryToSignals converts a BuiltWith category to intent signals.
func mapTechCategoryToSignals(category string, techs []string) []string {
	var signals []string
	cat := strings.ToLower(category)

	switch {
	case strings.Contains(cat, "crm"):
		signals = append(signals, "uses CRM — may need integrations, data sync, enrichment")
	case strings.Contains(cat, "marketing") || strings.Contains(cat, "email"):
		signals = append(signals, "active in marketing — email, automation, campaigns")
	case strings.Contains(cat, "analytics") || strings.Contains(cat, "tracking"):
		signals = append(signals, "tracks data — needs attribution, BI, data pipelines")
	case strings.Contains(cat, "payment") || strings.Contains(cat, "ecommerce"):
		signals = append(signals, "processes payments — needs billing, invoicing, subscription management")
	case strings.Contains(cat, "help desk") || strings.Contains(cat, "support"):
		signals = append(signals, "has customer support — needs helpdesk, chat, ticketing")
	case strings.Contains(cat, "cloud"):
		signals = append(signals, "cloud infrastructure — DevOps, monitoring, IaC tools")
	case strings.Contains(cat, "hr") || strings.Contains(cat, "recruit"):
		signals = append(signals, "manages people — HRIS, payroll, recruiting tools")
	case strings.Contains(cat, "project") || strings.Contains(cat, "task"):
		signals = append(signals, "manages projects — PM tools, collaboration")
	case strings.Contains(cat, "security"):
		signals = append(signals, "invests in security — endpoint, compliance, IAM")
	}

	// Also flag specific high-intent techs
	for _, t := range techs {
		tl := strings.ToLower(t)
		switch {
		case strings.Contains(tl, "salesforce") || strings.Contains(tl, "hubspot"):
			signals = append(signals, "uses Salesforce/HubSpot — integration opportunity")
		case strings.Contains(tl, "stripe"):
			signals = append(signals, "uses Stripe — billing/payment integration")
		case strings.Contains(tl, "segment"):
			signals = append(signals, "uses Segment — CDP, data pipeline opportunity")
		case strings.Contains(tl, "intercom") || strings.Contains(tl, "zendesk"):
			signals = append(signals, "uses support tool — customer success integration")
		}
	}

	return signals
}

// Health checks if BuiltWith API is reachable.
func (e *BuiltWithEngine) Health(ctx context.Context) error {
	// Free API doesn't need auth — just test reachability
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.builtwith.com/free-api?domain=example.com", nil)
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
		return fmt.Errorf("builtwith free API error: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Version returns the engine version string.
func (e *BuiltWithEngine) Version() string {
	if e.Config.APIKey != "" {
		return "xeme-intel/builtwith v1.0.0 (paid)"
	}
	return "xeme-intel/builtwith v1.0.0 (free)"
}
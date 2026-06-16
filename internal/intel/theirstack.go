// Package intel implements the Xeme Intelligence Engine — signal providers
// that detect company intent signals (technographics, hiring patterns,
// job postings, buying intent) from both premium APIs and free open-source
// alternatives.
//
// Provider stack (in priority order):
//
//   1. TheirStack  — premium API, 32k technologies, 202M job postings
//      (env: XEME_THEIRSTACK_API_KEY)
//   2. BuiltWith Free — free tier, technology groups/categories
//      (env: XEME_BUILTWITH_API_KEY)
//   3. Xeme Signal Scraper — free, HackerNews + job board aggregation
//      (no key required)
package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// TheirStackConfig holds the TheirStack API configuration.
type TheirStackConfig struct {
	APIKey     string
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	CostCapUSD float64
}

// DefaultTheirStackConfig returns a config pointing to env var secrets.
func DefaultTheirStackConfig() *TheirStackConfig {
	return &TheirStackConfig{
		APIKey:     os.Getenv("XEME_THEIRSTACK_API_KEY"),
		BaseURL:    "https://api.theirstack.com/v1",
		Timeout:    30 * time.Second,
		MaxRetries: 2,
		CostCapUSD: 10.0,
	}
}

// TheirStackEngine wraps the TheirStack REST API.
// Docs: https://theirstack.com/en/docs/api-reference
type TheirStackEngine struct {
	Config   *TheirStackConfig
	HTTP     *http.Client
	TokensUsed float64
}

// NewTheirStackEngine creates a new TheirStack engine.
func NewTheirStackEngine(cfg *TheirStackConfig) *TheirStackEngine {
	if cfg == nil {
		cfg = DefaultTheirStackConfig()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	return &TheirStackEngine{
		Config: cfg,
		HTTP: &http.Client{Timeout: cfg.Timeout},
	}
}

// doPOST posts a JSON body to a TheirStack endpoint and decodes the response.
func (e *TheirStackEngine) doPOST(ctx context.Context, endpoint string, body, result interface{}) (tokensCharged float64, _ error) {
	// Cost guard
	if e.Config.CostCapUSD > 0 && e.TokensUsed >= e.Config.CostCapUSD*100 {
		return 0, fmt.Errorf("theirstack: cost cap %.2f USD reached", e.Config.CostCapUSD)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshal body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= e.Config.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST",
			strings.TrimSuffix(e.Config.BaseURL, "/")+"/"+endpoint,
			bytes.NewReader(reqBody))
		if err != nil {
			return 0, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+e.Config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Xeme-OS/1.0 (Intel-Engine)")

		resp, err := e.HTTP.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := resp.Header.Get("Retry-After")
			wait := 2 * time.Second
			if ra, err := strconv.Atoi(retryAfter); err == nil {
				wait = time.Duration(ra) * time.Second
			}
			time.Sleep(wait)
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("theirstack HTTP %d: %s", resp.StatusCode, string(raw))
			if resp.StatusCode >= 500 {
				continue
			}
			return 0, lastErr
		}

		// Parse envelope for tokens_charged
		var envelope struct {
			Metadata struct {
				TokensCharged    float64 `json:"tokens_charged"`
				TokensRemaining  float64 `json:"tokens_remaining"`
				CreditUsed       float64 `json:"credit_used"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &envelope); err == nil {
			tokensCharged = envelope.Metadata.TokensCharged
			if tokensCharged == 0 {
				tokensCharged = envelope.Metadata.CreditUsed
			}
			e.TokensUsed += tokensCharged
		}

		if result != nil {
			if err := json.Unmarshal(raw, result); err != nil {
				return tokensCharged, fmt.Errorf("parse response: %w", err)
			}
		}
		return tokensCharged, nil
	}
	return 0, fmt.Errorf("theirstack %s: all attempts failed: %w", endpoint, lastErr)
}

// ── Company Search ─────────────────────────────────────────────────────────────

// CompanySearchParams defines filters for TheirStack company search.
type CompanySearchParams struct {
	// Technology filters
	Technologies        []string `json:"technologies,omitempty"`         // e.g. ["react", "typescript"]
	TechnologiesExclude []string `json:"technologies_exclude,omitempty"` // e.g. ["wordpress"]
	TechCategories      []string `json:"technologies_categories,omitempty"`

	// Hiring filters
	JobTitles       []string `json:"job_titles,omitempty"`        // e.g. ["VP of Marketing"]
	JobTitlesExclude []string `json:"job_titles_exclude,omitempty"`
	HiringSinceDays int      `json:"hiring_since_days,omitempty"`  // e.g. 30 = hired in last 30 days
	JobLocation     string   `json:"job_location,omitempty"`      // e.g. "United States"
	MinSalary       int      `json:"min_salary,omitempty"`        // in USD
	ExcludeAgencies  bool     `json:"exclude_agencies,omitempty"`  // exclude recruiting firms

	// Firmographic filters
	CompanyName   string   `json:"company_name,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Industry      string   `json:"industry,omitempty"`
	CompanySize   string   `json:"company_size,omitempty"`   // e.g. "11-50", "201-500"
	Country       string   `json:"country,omitempty"`
	CountryCodes  []string `json:"country_codes,omitempty"`
	Revenue       string   `json:"revenue,omitempty"`        // e.g. "$10M - $20M"
	FundingStage  string   `json:"funding_stage,omitempty"`

	// Output controls
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"` // default 20, max 100

	// Include sub-objects
	IncludeJobs        bool `json:"include_jobs,omitempty"`
	IncludeTechnologies bool `json:"include_technologies,omitempty"`
}

// CompanySearchResult is the API response for company search.
type CompanySearchResult struct {
	Data   []CompanyRecord `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalCount int `json:"total_count"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
	Metadata struct {
		TokensCharged   float64 `json:"tokens_charged"`
		TokensRemaining float64 `json:"tokens_remaining"`
	} `json:"metadata"`
}

// CompanyRecord is a single company record from TheirStack.
type CompanyRecord struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Domain          string   `json:"domain"`
	Website         string   `json:"website"`
	LinkedinURL     string   `json:"linkedin_url"`
	Country         string   `json:"country"`
	CountryCode     string   `json:"country_code"`
	Industry        string   `json:"industry"`
	CompanySize     string   `json:"company_size"`
	FoundedYear     int      `json:"founded_year"`
	FundingStage    string   `json:"funding_stage"`
	Revenue         string   `json:"revenue"`
	Description     string   `json:"description"`
	Technologies    []Technology `json:"technologies,omitempty"`
	Jobs            []JobRecord   `json:"jobs,omitempty"`
	Score           float64  `json:"score"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Technology is a single technology record.
type Technology struct {
	Name   string `json:"name"`
	Category string `json:"category"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
}

// JobRecord is a single job posting record.
type JobRecord struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	Country      string   `json:"country"`
	PostedAt     string   `json:"posted_at"`
	SalaryMin    int      `json:"salary_min"`
	SalaryMax    int      `json:"salary_max"`
	Currency     string   `json:"currency"`
	Source       string   `json:"source"`
	URL          string   `json:"url"`
	IsRemote     bool     `json:"is_remote"`
	Technologies  []string `json:"technologies,omitempty"`
}

// SearchCompanies searches TheirStack's company database.
// Returns companies filtered by tech stack, hiring signals, and firmographics.
func (e *TheirStackEngine) SearchCompanies(ctx context.Context, p CompanySearchParams) (*CompanySearchResult, float64, error) {
	if p.PageSize == 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	var res CompanySearchResult
	tokens, err := e.doPOST(ctx, "search/companies", p, &res)
	return &res, tokens, err
}

// ── Job Search ────────────────────────────────────────────────────────────────

// JobSearchParams defines filters for TheirStack job search.
type JobSearchParams struct {
	// Keyword filters
	Keywords        []string `json:"keywords,omitempty"`
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`

	// Company filters
	CompanyDomain string   `json:"company_domain,omitempty"`
	CompanyName   string   `json:"company_name,omitempty"`

	// Technology filters
	Technologies []string `json:"technologies,omitempty"` // job mentions these techs

	// Location filters
	Location   string   `json:"location,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`

	// Compensation
	MinSalary int `json:"min_salary,omitempty"`

	// Date filters
	PostedWithinDays int `json:"posted_within_days,omitempty"` // e.g. 7 = last 7 days

	// Output controls
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`

	// Exclude
	ExcludeAgencies bool `json:"exclude_agencies,omitempty"`
}

// JobSearchResult is the API response for job search.
type JobSearchResult struct {
	Data []JobRecord `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalCount int `json:"total_count"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
	Metadata struct {
		TokensCharged   float64 `json:"tokens_charged"`
		TokensRemaining float64 `json:"tokens_remaining"`
	} `json:"metadata"`
}

// SearchJobs searches TheirStack's 202M job postings database.
// Great for: finding companies actively hiring for a role (intent signal),
// finding companies by tech in job descriptions (technographic signal).
func (e *TheirStackEngine) SearchJobs(ctx context.Context, p JobSearchParams) (*JobSearchResult, float64, error) {
	if p.PageSize == 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	var res JobSearchResult
	tokens, err := e.doPOST(ctx, "search/jobs", p, &res)
	return &res, tokens, err
}

// ── Company Detail ────────────────────────────────────────────────────────────

// CompanyDetailParams — domain lookup for full company profile.
type CompanyDetailParams struct {
	Domain             string `json:"domain"`
	IncludeTechnologies bool   `json:"include_technologies,omitempty"`
	IncludeJobs        bool   `json:"include_jobs,omitempty"`
	JobsLimit          int    `json:"jobs_limit,omitempty"`
}

// CompanyDetailResult wraps the company detail response.
type CompanyDetailResult struct {
	Data     CompanyRecord `json:"data"`
	Metadata struct {
		TokensCharged   float64 `json:"tokens_charged"`
		TokensRemaining float64 `json:"tokens_remaining"`
	} `json:"metadata"`
}

// GetCompany looks up a company by domain and returns tech stack + jobs.
func (e *TheirStackEngine) GetCompany(ctx context.Context, p CompanyDetailParams) (*CompanyDetailResult, float64, error) {
	if p.JobsLimit == 0 {
		p.JobsLimit = 10
	}
	var res CompanyDetailResult
	tokens, err := e.doPOST(ctx, "companies/detail", p, &res)
	return &res, tokens, err
}

// ── Technologies List ────────────────────────────────────────────────────────

// TechnologiesResult is the list of available technologies.
type TechnologiesResult struct {
	Data []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Count    int    `json:"count"` // number of companies using this tech
	} `json:"data"`
}

// ListTechnologies returns all 32k+ technologies tracked by TheirStack.
func (e *TheirStackEngine) ListTechnologies(ctx context.Context, category string) (*TechnologiesResult, float64, error) {
	body := map[string]interface{}{}
	if category != "" {
		body["category"] = category
	}
	var res TechnologiesResult
	tokens, err := e.doPOST(ctx, "technologies/list", body, &res)
	return &res, tokens, err
}

// ── Enrichment row ───────────────────────────────────────────────────────────

// TheirStackEnrichResult is what the intel pipeline receives from TheirStack.
type TheirStackEnrichResult struct {
	Domain           string
	CompanyName      string
	Country          string
	Industry         string
	CompanySize      string
	FundingStage     string
	Revenue          string
	Technologies     []string
	TechCategories   []string
	HiringSignals    []string // job titles of recent hires
	JobPostingsCount int
	Score            float64
	Source           string
}

// EnrichRow looks up a company by domain and returns all signals.
func (e *TheirStackEngine) EnrichRow(ctx context.Context, domain string) (*TheirStackEnrichResult, float64, error) {
	detail, tokens, err := e.GetCompany(ctx, CompanyDetailParams{
		Domain:             domain,
		IncludeTechnologies: true,
		IncludeJobs:        true,
		JobsLimit:          5,
	})
	if err != nil {
		return nil, tokens, err
	}

	result := &TheirStackEnrichResult{
		Domain:     domain,
		CompanyName: detail.Data.Name,
		Country:    detail.Data.Country,
		Industry:   detail.Data.Industry,
		CompanySize: detail.Data.CompanySize,
		FundingStage: detail.Data.FundingStage,
		Revenue:    detail.Data.Revenue,
		Score:      detail.Data.Score,
		Source:     "theirstack",
	}

	// Extract tech names
	for _, t := range detail.Data.Technologies {
		result.Technologies = append(result.Technologies, t.Name)
	}

	// Extract hiring signals (recent job titles)
	for _, j := range detail.Data.Jobs {
		if j.Title != "" {
			result.HiringSignals = append(result.HiringSignals, j.Title)
		}
	}
	result.JobPostingsCount = len(detail.Data.Jobs)

	return result, tokens, nil
}

// ── Account ────────────────────────────────────────────────────────────────

// AccountInfo is the account response from TheirStack.
type TheirStackAccountInfo struct {
	Email   string `json:"email"`
	Plan    string `json:"plan"`
	Status  string `json:"status"`
	Metadata struct {
		TokensRemaining float64 `json:"tokens_remaining"`
		CreditsRemaining float64 `json:"credits_remaining"`
	} `json:"metadata"`
}

// GetAccount returns account info and token balance (free endpoint).
func (e *TheirStackEngine) GetAccount(ctx context.Context) (*TheirStackAccountInfo, float64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		strings.TrimSuffix(e.Config.BaseURL, "/")+"/account",
		nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+e.Config.APIKey)
	req.Header.Set("User-Agent", "Xeme-OS/1.0 (Intel-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var res TheirStackAccountInfo
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, 0, fmt.Errorf("parse account: %w", err)
	}
	return &res, 0, nil
}

// Health checks if TheirStack API is reachable.
func (e *TheirStackEngine) Health(ctx context.Context) error {
	if e.Config.APIKey == "" {
		return fmt.Errorf("XEME_THEIRSTACK_API_KEY not set")
	}
	_, _, err := e.GetAccount(ctx)
	return err
}

// Version returns the engine version string.
func (e *TheirStackEngine) Version() string {
	return "xeme-intel/theirstack v1.0.0"
}
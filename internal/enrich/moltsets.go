// Package enrich — MoltSets integration.
//
// MoltSets is a B2B contact data API providing 100%-verified emails,
// carrier-verified mobile phones, and people/company search. It uses a
// 20+ vendor waterfall and offers unlimited plans for AI agents.
//
// Docs:  https://developer.moltsets.com
// Base:  https://api.moltsets.com/api/v1/tools/
// Auth:  Bearer token, format ms_XXXXXXXXXXX
// MCP:   https://mcp.moltsets.com/mcp
package enrich

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

// MoltSetsConfig holds the per-run configuration for the MoltSets engine.
type MoltSetsConfig struct {
	APIKey     string // Bearer token, ms_XXXXXXXXXXX
	BaseURL    string // defaults to https://api.moltsets.com/api/v1/tools/
	Timeout    time.Duration
	MaxRetries int
	CostCapUSD float64 // max $ to spend per run;0 = unlimited
}

// DefaultMoltSetsConfig returns a config pointing to the env var secrets.
func DefaultMoltSetsConfig() *MoltSetsConfig {
	return&MoltSetsConfig{
		APIKey:     os.Getenv("XEME_MOLTSETS_API_KEY"),
		BaseURL:    "https://api.moltsets.com/api/v1/tools/",
		Timeout:    30 * time.Second,
		MaxRetries: 2,
		CostCapUSD: 10.0,
	}
}

// MoltSetsEngine wraps the MoltSets REST API.
type MoltSetsEngine struct {
	Config   *MoltSetsConfig
	HTTP     *http.Client
	TokensUsed float64 // running total for this session
}

// NewMoltSetsEngine creates a new MoltSets engine.
func NewMoltSetsEngine(cfg *MoltSetsConfig) *MoltSetsEngine {
	if cfg == nil {
		cfg = DefaultMoltSetsConfig()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	return &MoltSetsEngine{
		Config: cfg,
		HTTP: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// ── Low-level request helper ────────────────────────────────────────────────

// doPOST posts a JSON body to a MoltSets endpoint and decodes the response.
func (e *MoltSetsEngine) doPOST(ctx context.Context, endpoint string, body, result interface{}) (tokensCharged float64, _ error) {
	// Cost guard
	if e.Config.CostCapUSD > 0 {
		// approximate: $0.01 per token on the $27 plan
		if e.TokensUsed >= e.Config.CostCapUSD*100 {
			return 0, fmt.Errorf("moltsets: cost cap %.2f USD reached", e.Config.CostCapUSD)
		}
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
		req.Header.Set("User-Agent", "Xeme-OS/1.0 (MoltSets-Enrich-Engine)")

		resp, err := e.HTTP.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Respect rate limits with Retry-After if present
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
			lastErr = fmt.Errorf("moltsets HTTP %d: %s", resp.StatusCode, string(raw))
			if resp.StatusCode >= 500 {
				continue // retry server errors
			}
			return 0, lastErr
		}

		var envelope struct {
			Status   string `json:"status"`
			Metadata struct {
				TokensCharged float64 `json:"tokens_charged"`
				TokensRemaining float64 `json:"tokens_remaining"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &envelope); err == nil {
			tokensCharged = envelope.Metadata.TokensCharged
			e.TokensUsed += tokensCharged
		}

		if result != nil {
			if err := json.Unmarshal(raw, result); err != nil {
				return tokensCharged, fmt.Errorf("parse response: %w", err)
			}
		}
		return tokensCharged, nil
	}
	return 0, fmt.Errorf("moltsets %s: all attempts failed: %w", endpoint, lastErr)
}

// ── Company Search ────────────────────────────────────────────────────────────

// SearchCompanyParams defines filters for company search.
type SearchCompanyParams struct {
	Query         string `json:"query,omitempty"`          // free-text company name
	Domain        string `json:"domain,omitempty"`        // exact domain (no https://)
	Industry      string `json:"industry,omitempty"`      // e.g. "Information Technology"
	EmployeeRange string `json:"employee_range,omitempty"` // e.g. "51-200"
	RevenueRange  string `json:"revenue_range,omitempty"`  // e.g. "$10M - $20M"
	Limit         int    `json:"limit,omitempty"` // default 10, max 25
	Offset        int    `json:"offset,omitempty"`
}

// SearchCompanyResult is the API response for company search.
type SearchCompanyResult struct {
	Results struct {
		Results []CompanyProfile `json:"results"`
		Total   int              `json:"total"`
	} `json:"results"`
	Status string `json:"status"`
}

// CompanyProfile is a single company record from MoltSets.
type CompanyProfile struct {
	Name            string `json:"name"`
	Type string `json:"type"`
	Domain          string `json:"domain"`
	Industry        string `json:"industry"`
	LinkedInURL     string `json:"linkedin_url"`
	ExactRevenue    int64  `json:"exact_revenue"`
	RevenueRange    string `json:"revenue_range"`
	EmployeeCount   int    `json:"employee_count"`
	EmployeeRange   string `json:"employee_range"`
	FollowerCount   int    `json:"follower_count"`
	LinkedInID      string `json:"linkedin_company_id"`
	ID              string `json:"_id"`
	Score           float64 `json:"_score"`
}

// SearchCompanies searches the MoltSets company database.
// Returns ranked company profiles ordered by _score descending.
func (e *MoltSetsEngine) SearchCompanies(ctx context.Context, p SearchCompanyParams) (*SearchCompanyResult, float64, error) {
	if p.Limit == 0 {
		p.Limit = 10
	}
	if p.Limit > 25 {
		p.Limit = 25
	}
	var res SearchCompanyResult
	tokens, err := e.doPOST(ctx, "search_company_profiles", p, &res)
	return &res, tokens, err
}

// ── People / Business Profile Search ────────────────────────────────────────

// SearchPeopleParams defines filters for people/business profile search.
type SearchPeopleParams struct {
	Query         string `json:"query,omitempty"`          // free-text name
	CompanyDomain string `json:"company_domain,omitempty"` // filter by company domain
	Industry      string `json:"industry,omitempty"`
	EmployeeRange string `json:"employee_range,omitempty"`
	RevenueRange  string `json:"revenue_range,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Offset        int    `json:"offset,omitempty"`
}

// SearchPeopleResult is the API response for people search.
type SearchPeopleResult struct {
	Results struct {
		Results []PersonProfile `json:"results"`
		Total   int             `json:"total"`
	} `json:"results"`
	Status string `json:"status"`
}

// PersonProfile is a single person record from MoltSets.
type PersonProfile struct {
	FullName   string `json:"full_name"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Title      string `json:"title"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	LinkedInURL string `json:"linkedin_url"`
	Company    string `json:"company"`
	Domain     string `json:"domain"`
	Industry   string `json:"industry"`
	Location   string `json:"location"`
	Score      float64 `json:"_score"`
}

// SearchPeople searches the MoltSets people database.
// Returns ranked person profiles ordered by _score descending.
func (e *MoltSetsEngine) SearchPeople(ctx context.Context, p SearchPeopleParams) (*SearchPeopleResult, float64, error) {
	if p.Limit == 0 {
		p.Limit = 10
	}
	if p.Limit > 25 {
		p.Limit = 25
	}
	var res SearchPeopleResult
	tokens, err := e.doPOST(ctx, "search_business_profiles", p, &res)
	return &res, tokens, err
}

// ── Email Enrichment ─────────────────────────────────────────────────────────

// EnrichEmailParams — LinkedIn URL → best available email.
type EnrichEmailParams struct {
	LinkedInURL string `json:"linkedin_url"`
}

// EnrichBusinessEmailParams — LinkedIn URL → verified business email only.
type EnrichBusinessEmailParams struct {
	LinkedInURL string `json:"linkedin_url"`
}

// EnrichPersonalEmailParams — LinkedIn URL → verified personal email only.
type EnrichPersonalEmailParams struct {
	LinkedInURL string `json:"linkedin_url"`
}

// EnrichPhoneParams — LinkedIn URL → carrier-verified mobile phone.
type EnrichPhoneParams struct {
	LinkedInURL  string   `json:"linkedin_url"`
	LinkedInURLs []string `json:"linkedin_urls,omitempty"` // batch, up to 100
}

// EmailEnrichResult is the response for email enrichment.
type EmailEnrichResult struct {
	Results struct {
		Email   string `json:"email"`
		Type    string `json:"type"`    // "business" or "personal"
		Score   string `json:"score"`  // confidence string
		LinkedIn string `json:"linkedin"`
	} `json:"results"`
	Status string `json:"status"`
}

// PhoneEnrichResult is the response for phone enrichment.
type PhoneEnrichResult struct {
	Results struct {
		Phone     string `json:"phone"`
		Carrier   string `json:"carrier"`
		Validated string `json:"validated"`
		LinkedIn  string `json:"linkedin"`
	} `json:"results"`
	Status string `json:"status"`
}

// EnrichEmail fetches the best available email for a LinkedIn profile.
// Prefers business email, falls back to personal.
func (e *MoltSetsEngine) EnrichEmail(ctx context.Context, linkedinURL string) (*EmailEnrichResult, float64, error) {
	var res EmailEnrichResult
	tokens, err := e.doPOST(ctx, "enrich_email", EnrichEmailParams{LinkedInURL: linkedinURL}, &res)
	return &res, tokens, err
}

// EnrichBusinessEmail fetches only the verified business email.
func (e *MoltSetsEngine) EnrichBusinessEmail(ctx context.Context, linkedinURL string) (*EmailEnrichResult, float64, error) {
	var res EmailEnrichResult
	tokens, err := e.doPOST(ctx, "enrich_business_email", EnrichBusinessEmailParams{LinkedInURL: linkedinURL}, &res)
	return &res, tokens, err
}

// EnrichPersonalEmail fetches only the verified personal email.
func (e *MoltSetsEngine) EnrichPersonalEmail(ctx context.Context, linkedinURL string) (*EmailEnrichResult, float64, error) {
	var res EmailEnrichResult
	tokens, err := e.doPOST(ctx, "enrich_personal_email", EnrichPersonalEmailParams{LinkedInURL: linkedinURL}, &res)
	return &res, tokens, err
}

// EnrichPhone fetches a carrier-verified mobile phone for a LinkedIn profile.
// Set LinkedInURLs (batch, up to 100) for batch mode.
func (e *MoltSetsEngine) EnrichPhone(ctx context.Context, p EnrichPhoneParams) (*PhoneEnrichResult, float64, error) {
	var res PhoneEnrichResult
	tokens, err := e.doPOST(ctx, "enrich_phone", p, &res)
	return &res, tokens, err
}

// ── Full Business Profile ───────────────────────────────────────────────────

// LinkedInToProfileParams — LinkedIn URL → full business profile.
type LinkedInToProfileParams struct {
	LinkedInURL string `json:"linkedin_url"`
	IncludePhone bool  `json:"include_phone,omitempty"`
}

// HEMToLinkedInParams — Email → LinkedIn slug.
type HEMToLinkedInParams struct {
	Email string `json:"email"`
}

// HEMToBestLinkedInParams — Email → highest-confidence LinkedIn URL.
type HEMToBestLinkedInParams struct {
	Email string `json:"email"`
}

// BusinessProfile is the full person + company data record.
type BusinessProfile struct {
	FullName    string `json:"full_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Title       string `json:"title"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	LinkedInURL string `json:"linkedin_url"`
	Company     string `json:"company"`
	Domain      string `json:"domain"`
	Industry    string `json:"industry"`
	EmployeeRange string `json:"employee_range"`
	RevenueRange  string `json:"revenue_range"`
	Location    string `json:"location"`
	City        string `json:"city"`
	Country     string `json:"country"`
}

// LinkedInToProfileResult wraps the full profile response.
type LinkedInToProfileResult struct {
	Results BusinessProfile `json:"results"`
	Status  string          `json:"status"`
}

// LinkedInToProfile fetches the full business profile + company data for a LinkedIn URL.
func (e *MoltSetsEngine) LinkedInToProfile(ctx context.Context, linkedinURL string) (*LinkedInToProfileResult, float64, error) {
	var res LinkedInToProfileResult
	tokens, err := e.doPOST(ctx, "linkedin_to_business_profile",
		LinkedInToProfileParams{LinkedInURL: linkedinURL}, &res)
	return &res, tokens, err
}

// HEMToBestLinkedIn fetches the highest-confidence LinkedIn URL for an email.
func (e *MoltSetsEngine) HEMToBestLinkedIn(ctx context.Context, email string) (string, float64, error) {
	body := HEMToBestLinkedInParams{Email: email}
	var res struct {
		Results struct {
			LinkedInURL string `json:"linkedin_url"`
		} `json:"results"`
		Status string `json:"status"`
	}
	tokens, err := e.doPOST(ctx, "hem_to_best_linkedin", body, &res)
	return res.Results.LinkedInURL, tokens, err
}

// HEMToProfile fetches the full business profile for an email.
func (e *MoltSetsEngine) HEMToProfile(ctx context.Context, email string) (*LinkedInToProfileResult, float64, error) {
	body := struct{ Email string }{Email: email}
	var res LinkedInToProfileResult
	tokens, err := e.doPOST(ctx, "hem_to_business_profile", body, &res)
	return &res, tokens, err
}

// ── Account / Billing (free endpoints) ─────────────────────────────────────

// AccountInfo is the response from /get_account.
type AccountInfo struct {
	Email     string `json:"email"`
	Plan      string `json:"plan"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Metadata  struct {
		TokensRemaining float64 `json:"tokens_remaining"`
	} `json:"metadata"`
}

// BillingInfo is the response from /get_billing.
type BillingInfo struct {
	Plan       string `json:"plan"`
	RenewalDate string `json:"renewal_date"`
	Status     string `json:"status"`
	Metadata   struct {
		TokensRemaining float64 `json:"tokens_remaining"`
		PerToolCosts    map[string]float64 `json:"per_tool_costs"`
	} `json:"metadata"`
}

// GetAccount returns account details and token balance (free endpoint).
func (e *MoltSetsEngine) GetAccount(ctx context.Context) (*AccountInfo, float64, error) {
	var res AccountInfo
	tokens, err := e.doPOST(ctx, "get_account", struct{}{}, &res)
	return &res, tokens, err
}

// GetBilling returns billing plan, renewal date, and per-tool credit costs (free endpoint).
func (e *MoltSetsEngine) GetBilling(ctx context.Context) (*BillingInfo, float64, error) {
	var res BillingInfo
	tokens, err := e.doPOST(ctx, "get_billing", struct{}{}, &res)
	return &res, tokens, err
}

// ── Waterfall integration ────────────────────────────────────────────────────

// MoltSetsEnrichResult is what the waterfall receives from MoltSets.
type MoltSetsEnrichResult struct {
	Email       string
	Phone       string
	LinkedInURL string
	Company     string
	Domain      string
	Title       string
	FirstName   string
	LastName    string
	FullName    string
	Industry    string
	Location    string
	Confidence  float64
	Source      string
}

// EnrichRow attempts to enrich a single contact row using MoltSets.
// Tries: LinkedIn URL first (email + phone), then email (full profile), then company search.
func (e *MoltSetsEngine) EnrichRow(ctx context.Context, row map[string]string) (*MoltSetsEnrichResult, float64, error) {
	linkedin := row["linkedin_url"]
	email := row["email"]
	domain := row["domain"]
	company := firstNonEmpty(row["company_name"], row["company"])

	var totalTokens float64

	// Strategy 1: LinkedIn URL → email + phone (most reliable)
	if linkedin != "" {
		var emailRes *EmailEnrichResult
		var phoneRes *PhoneEnrichResult
		var errEmail, errPhone error

		// Enrich email and phone in parallel-ish (sequential on same connection)
		emailRes, _, errEmail = e.EnrichEmail(ctx, linkedin)
		if errEmail == nil && emailRes.Results.Email != "" {
			totalTokens += 1 // approximate
		}

		phoneRes, _, errPhone = e.EnrichPhone(ctx, EnrichPhoneParams{LinkedInURL: linkedin})
		if errPhone == nil && phoneRes.Results.Phone != "" {
			totalTokens += 1
		}

		if errEmail == nil && emailRes.Results.Email != "" {
			return &MoltSetsEnrichResult{
				Email:       emailRes.Results.Email,
				Phone:       phoneRes.Results.Phone,
				LinkedInURL: linkedin,
				Confidence:  0.95,
				Source:      "moltsets-linkedin",
			}, totalTokens, nil
		}
	}

	// Strategy 2: email → LinkedIn → full profile
	if email != "" {
		linkedinURL, _, err := e.HEMToBestLinkedIn(ctx, email)
		if err == nil && linkedinURL != "" {
			profile, _, err := e.LinkedInToProfile(ctx, linkedinURL)
			if err == nil {
				totalTokens += 2
				return &MoltSetsEnrichResult{
					Email:       email,
					Phone:       profile.Results.Phone,
					LinkedInURL: linkedinURL,
					Company:     profile.Results.Company,
					Domain:      profile.Results.Domain,
					Title:       profile.Results.Title,
					FullName:    profile.Results.FullName,
					Industry:    profile.Results.Industry,
					Location:    profile.Results.Location,
					Confidence:  0.90,
					Source:      "moltsets-email-hem",
				}, totalTokens, nil
			}
		}
	}

	// Strategy 3: company search → people search → profile
	if domain != "" || company != "" {
		searchDomain := domain
		if searchDomain == "" {
			searchDomain = inferDomain(company, "")
		}
		if searchDomain != "" {
			compRes, _, err := e.SearchCompanies(ctx, SearchCompanyParams{Domain: searchDomain})
			if err == nil && len(compRes.Results.Results) > 0 {
				companyName := compRes.Results.Results[0].Name
				peopleRes, _, err := e.SearchPeople(ctx, SearchPeopleParams{CompanyDomain: searchDomain})
				if err == nil && len(peopleRes.Results.Results) > 0 {
					person := peopleRes.Results.Results[0]
					totalTokens += 2
					return &MoltSetsEnrichResult{
						Email:       person.Email,
						Phone:       person.Phone,
						LinkedInURL: person.LinkedInURL,
						Company:     firstNonEmpty(person.Company, companyName),
						Domain:      searchDomain,
						Title:       person.Title,
						FullName:    person.FullName,
						Industry:    person.Industry,
						Location:    person.Location,
						Confidence:  0.85,
						Source:      "moltsets-company-search",
					}, totalTokens, nil
				}
			}
		}
	}

	return nil, 0, fmt.Errorf("moltsets: no enrichment path for row")
}

// Health checks if the MoltSets API is reachable.
func (e *MoltSetsEngine) Health(ctx context.Context) error {
	if e.Config.APIKey == "" {
		return fmt.Errorf("XEME_MOLTSETS_API_KEY not set")
	}
	_, _, err := e.GetAccount(ctx)
	return err
}

// Version returns the engine version string.
func (e *MoltSetsEngine) Version() string {
	return "xeme-enrich/moltsets v1.0.0"
}

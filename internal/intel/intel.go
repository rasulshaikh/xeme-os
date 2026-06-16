// Package intel — Xeme Intelligence Engine
//
// Orchestrates all intelligence/signal providers in priority order:
//   TheirStack (paid) → BuiltWith Free (free tech) → Signal Scraper (HN + job boards)
//
// Signal types:
//   - technographic  — what tech does this company use?
//   - hiring          — what roles is this company hiring?
//   - buying intent   — what problems does this company have (from job descriptions)?
//   - firmographic    — company size, industry, country, revenue
package intel

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Engine is the Xeme Intelligence Engine — unified interface for all signal providers.
type Engine struct {
	TheirStack    *TheirStackEngine   // paid, full coverage
	BuiltWith     *BuiltWithEngine    // free, tech stack only
	Scraper       *SignalScraper      // free, HN + job boards
}

// New creates a new intel engine with all available providers.
// Providers are auto-initialized based on env var presence.
func New() *Engine {
	e := &Engine{
		Scraper: NewSignalScraper(nil),
	}
	if apiKey := os.Getenv("XEME_THEIRSTACK_API_KEY"); apiKey != "" {
		e.TheirStack = NewTheirStackEngine(DefaultTheirStackConfig())
	}
	if true { // BuiltWith free tier doesn't need a key
		e.BuiltWith = NewBuiltWithEngine(nil)
	}
	return e
}

// SignalResult is the unified signal output from the intel engine.
type SignalResult struct {
	Domain              string   `json:"domain"`
	CompanyName         string   `json:"company_name"`
	Country             string   `json:"country"`
	Industry            string   `json:"industry"`
	CompanySize         string   `json:"company_size"`
	Revenue             string   `json:"revenue"`
	FundingStage        string   `json:"funding_stage"`
	Technologies        []string `json:"technologies"`
	HiringSignals       []string `json:"hiring_signals"` // job titles
	BuyingIntentSignals []string `json:"buying_intent_signals"`
	JobPostingsCount    int      `json:"job_postings_count"`
	TechCategories      []string `json:"tech_categories"`
	Confidence          float64  `json:"confidence"`
	Source              string   `json:"source"` // theirstack | builtwith-free | signal-scraper
	TokensUsed          float64  `json:"tokens_used"`
}

// GetSignals returns all available signals for a company domain.
// Tries TheirStack → BuiltWith Free → Signal Scraper in order.
func (e *Engine) GetSignals(ctx context.Context, domain string) (*SignalResult, error) {
	result := &SignalResult{
		Domain:     domain,
		Confidence: 0.5,
		Source:     "none",
	}

	// Priority 1: TheirStack (full signals)
	if e.TheirStack != nil {
		tsResult, tokens, err := e.TheirStack.EnrichRow(ctx, domain)
		if err == nil && tsResult != nil {
			result.CompanyName = tsResult.CompanyName
			result.Country = tsResult.Country
			result.Industry = tsResult.Industry
			result.CompanySize = tsResult.CompanySize
			result.Revenue = tsResult.Revenue
			result.FundingStage = tsResult.FundingStage
			result.Technologies = tsResult.Technologies
			result.HiringSignals = tsResult.HiringSignals
			result.JobPostingsCount = tsResult.JobPostingsCount
			result.Confidence = 0.95
			result.Source = "theirstack"
			result.TokensUsed = tokens
			return result, nil
		}
	}

	// Priority 2: BuiltWith Free (tech stack only)
	if e.BuiltWith != nil {
		bwResult, err := e.BuiltWith.TechSignal(ctx, domain)
		if err == nil && bwResult != nil {
			result.Technologies = bwResult.Technologies
			result.TechCategories = bwResult.Categories
			result.BuyingIntentSignals = bwResult.Signals
			result.Confidence = 0.8
			result.Source = "builtwith-free"
			if result.CompanyName == "" {
				result.CompanyName = domainToName(domain)
			}
			return result, nil
		}
	}

	// Priority 3: Signal Scraper (HN + job boards, free)
	if e.Scraper != nil {
		// Search HN for hiring signals
		hnResults, err := e.Scraper.SearchHNWhoIsHiring(ctx, []string{domain})
		if err == nil && len(hnResults) > 0 {
			for _, r := range hnResults {
				if strings.Contains(strings.ToLower(r.Domain), domain) {
					result.HiringSignals = append(result.HiringSignals, r.Title)
				}
			}
		}
		result.Confidence = 0.6
		result.Source = "signal-scraper-free"
	}

	return result, nil
}

// GetTechnologies returns all tracked technologies from TheirStack.
// Only works if TheirStack API key is set.
func (e *Engine) GetTechnologies(ctx context.Context, category string) ([]string, error) {
	if e.TheirStack == nil {
		return nil, fmt.Errorf("theirstack: XEME_THEIRSTACK_API_KEY not set")
	}
	res, _, err := e.TheirStack.ListTechnologies(ctx, category)
	if err != nil {
		return nil, err
	}
	var techs []string
	for _, t := range res.Data {
		techs = append(techs, t.Name)
	}
	return techs, nil
}

// SearchCompanies searches companies by tech + hiring signals.
func (e *Engine) SearchCompanies(ctx context.Context, params CompanySearchParams) ([]CompanyRecord, float64, error) {
	if e.TheirStack == nil {
		return nil, 0, fmt.Errorf("theirstack: XEME_THEIRSTACK_API_KEY not set")
	}
	res, tokens, err := e.TheirStack.SearchCompanies(ctx, params)
	if err != nil {
		return nil, tokens, err
	}
	return res.Data, tokens, nil
}

// SearchJobs searches job postings for intent signals.
func (e *Engine) SearchJobs(ctx context.Context, params JobSearchParams) ([]JobRecord, float64, error) {
	if e.TheirStack == nil {
		return nil, 0, fmt.Errorf("theirstack: XEME_THEIRSTACK_API_KEY not set")
	}
	res, tokens, err := e.TheirStack.SearchJobs(ctx, params)
	if err != nil {
		return nil, tokens, err
	}
	return res.Data, tokens, nil
}

// Status returns the health and availability of all intel providers.
func (e *Engine) Status(ctx context.Context) map[string]interface{} {
	out := map[string]interface{}{}

	if e.TheirStack != nil {
		err := e.TheirStack.Health(ctx)
		out["theirstack"] = map[string]interface{}{
			"available": true,
			"version":   e.TheirStack.Version(),
			"ok":        err == nil,
			"error":     nil,
		}
		if err == nil {
			account, _, _ := e.TheirStack.GetAccount(ctx)
			if account != nil {
				out["theirstack"].(map[string]interface{})["tokens_remaining"] = account.Metadata.TokensRemaining
				out["theirstack"].(map[string]interface{})["plan"] = account.Plan
			}
		}
	} else {
		out["theirstack"] = map[string]interface{}{
			"available": false,
			"reason":    "XEME_THEIRSTACK_API_KEY not set",
		}
	}

	if e.BuiltWith != nil {
		err := e.BuiltWith.Health(ctx)
		out["builtwith_free"] = map[string]interface{}{
			"available": true,
			"version":   e.BuiltWith.Version(),
			"ok":        err == nil,
		}
	} else {
		out["builtwith_free"] = map[string]interface{}{"available": false}
	}

	if e.Scraper != nil {
		err := e.Scraper.Health(ctx)
		out["signal_scraper_free"] = map[string]interface{}{
			"available": true,
			"version":   e.Scraper.Version(),
			"ok":        err == nil,
		}
	} else {
		out["signal_scraper_free"] = map[string]interface{}{"available": false}
	}

	return out
}

// Version returns the intel engine version.
func (e *Engine) Version() string {
	return "xeme-intel v1.0.0"
}

// Health checks all available providers.
func (e *Engine) Health(ctx context.Context) error {
	if e.TheirStack != nil {
		if err := e.TheirStack.Health(ctx); err != nil {
			return err
		}
	}
	if e.Scraper != nil {
		if err := e.Scraper.Health(ctx); err != nil {
			return err
		}
	}
	return nil
}

// domainToName converts a domain to a company name guess.
func domainToName(domain string) string {
	domain = strings.TrimPrefix(domain, "www.")
	domain = strings.Split(domain, ".")[0]
	return strings.Title(strings.ReplaceAll(domain, "-", " "))
}

// (helper funcs removed)
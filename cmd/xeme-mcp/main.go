// Xeme OS — MCP Server
// Exposes Xeme's proprietary engines as Model Context Protocol tools
// over stdio. Compatible with Claude Code, Cursor, and any MCP client.
//
// This is a minimal, stdlib-only implementation of the MCP protocol
// (JSON-RPC 2.0 with specific method names: initialize, tools/list, tools/call).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/xeme-os/xeme/internal/csvkit"
	"github.com/xeme-os/xeme/internal/aeo"
	"github.com/xeme-os/xeme/internal/enrich"
	"github.com/xeme-os/xeme/internal/intel"
	"github.com/xeme-os/xeme/internal/ledger"
	"github.com/xeme-os/xeme/internal/pipe"
	"github.com/xeme-os/xeme/internal/score"
	"github.com/xeme-os/xeme/internal/signal"
)

const (
	serverName    = "xeme-os"
	serverVersion = "0.3.0"
	protocolVer   = "2024-11-05"
)

// JSON-RPC 2.0 envelope
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool definitions
type tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

var tools = []tool{
	{
		Name:        "xeme_status",
		Description: "Health check across all Xeme engines (Signal, Enrichment, Ledger). Returns version, status, and credit balance.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_scrape_post",
		Description: "Scrape engagers from a public post URL via the Xeme Signal Engine. Returns JSON file path with engagers.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"url":   map[string]interface{}{"type": "string", "description": "Public post URL to scrape"},
				"out":   map[string]interface{}{"type": "string", "description": "Output JSON path (optional)"},
				"pages": map[string]interface{}{"type": "number", "description": "Max pagination pages (default 10)"},
			},
			"required": []string{"url"},
		},
	},
	{
		Name:        "xeme_enrich_leads",
		Description: "Waterfall-enrich a CSV via the Xeme Enrichment Engine (includes MoltSets when XEME_MOLTSETS_API_KEY is set). Returns path + summary stats + credits spent.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"input":  map[string]interface{}{"type": "string", "description": "Input CSV path"},
				"output": map[string]interface{}{"type": "string", "description": "Output CSV path (optional, auto-generated)"},
			},
			"required": []string{"input"},
		},
	},
	{
		Name:        "xeme_score_leads",
		Description: "Apply Xeme's 7-gate ICP scoring to a CSV. Returns scored + tier-classified leads.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"input":     map[string]interface{}{"type": "string", "description": "Input CSV path"},
				"output":    map[string]interface{}{"type": "string", "description": "Output CSV path (optional)"},
				"min_score": map[string]interface{}{"type": "number", "description": "Filter to leads above this score"},
			},
			"required": []string{"input"},
		},
	},
	{
		Name:        "xeme_run_pipe",
		Description: "THE end-to-end pipe: scrape signals → waterfall enrich → score → sync to Xeme Ledger. The main GTM workflow.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"input":     map[string]interface{}{"type": "string", "description": "Input CSV path"},
				"min_score": map[string]interface{}{"type": "number", "description": "Minimum ICP score (default 0)"},
				"no_crm":    map[string]interface{}{"type": "boolean", "description": "Skip Ledger sync (default false)"},
				"dry_run":   map[string]interface{}{"type": "boolean", "description": "Preview only (default false)"},
			},
			"required": []string{"input"},
		},
	},
	{
		Name:        "xeme_crm_sync",
		Description: "Sync a CSV of contacts to the Xeme Ledger (GraphQL persistence).",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"input":   map[string]interface{}{"type": "string", "description": "CSV path"},
				"dry_run": map[string]interface{}{"type": "boolean", "description": "Preview only (default false)"},
			},
			"required": []string{"input"},
		},
	},
	{
		Name:        "xeme_crm_status",
		Description: "Probe the Xeme Ledger connection.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},

	// ── MoltSets tools ───────────────────────────────────────

	{
		Name:        "xeme_moltsets_status",
		Description: "Check MoltSets account balance and billing plan. Requires XEME_MOLTSETS_API_KEY env var.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_moltsets_search_company",
		Description: "Search the MoltSets company database by name, domain, industry, employee range, or revenue range. Returns ranked company profiles.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"query":          map[string]interface{}{"type": "string", "description": "Free-text company name search"},
				"domain":         map[string]interface{}{"type": "string", "description": "Exact domain filter (no https://)"},
				"industry":       map[string]interface{}{"type": "string", "description": "Industry filter, e.g. 'Information Technology'"},
				"employee_range": map[string]interface{}{"type": "string", "description": "Employee range, e.g. '51-200'"},
				"revenue_range":  map[string]interface{}{"type": "string", "description": "Revenue range, e.g. '$10M - $20M'"},
				"limit":          map[string]interface{}{"type": "number", "description": "Max results (default 10, max 25)"},
			},
			"required": []string{},
		},
	},
	{
		Name:        "xeme_moltsets_search_people",
		Description: "Search the MoltSets people database by company domain, industry, employee range, or revenue range. Returns ranked person profiles.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"company_domain": map[string]interface{}{"type": "string", "description": "Filter by company domain"},
				"industry":       map[string]interface{}{"type": "string", "description": "Industry filter"},
				"employee_range": map[string]interface{}{"type": "string", "description": "Employee range"},
				"revenue_range":  map[string]interface{}{"type": "string", "description": "Revenue range"},
				"limit":          map[string]interface{}{"type": "number", "description": "Max results (default 10, max 25)"},
			},
			"required": []string{},
		},
	},
	{
		Name:        "xeme_moltsets_enrich_email",
		Description: "Enrich a contact's email via their LinkedIn URL using MoltSets. Returns the best available email (business preferred, falls back to personal) with type and confidence.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"linkedin_url": map[string]interface{}{"type": "string", "description": "LinkedIn profile URL"},
			},
			"required": []string{"linkedin_url"},
		},
	},
	{
		Name:        "xeme_moltsets_enrich_phone",
		Description: "Enrich a contact's carrier-verified mobile phone via their LinkedIn URL using MoltSets. Returns the mobile number (mobiles only, no landlines).",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"linkedin_url": map[string]interface{}{"type": "string", "description": "LinkedIn profile URL"},
			},
			"required": []string{"linkedin_url"},
		},
	},

	// ── TheirStack + Intel tools ──────────────────────────────────

	{
		Name:        "xeme_intel_status",
		Description: "Check availability and health of all intel providers (TheirStack paid, BuiltWith free, Signal Scraper free). Returns which providers are active and their token balances.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_intel_company_signals",
		Description: "Get all signals for a company domain — technographics (tech stack), hiring signals (job titles), and buying intent. Tries TheirStack → BuiltWith Free → Signal Scraper in priority order. No key needed for free tiers.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"domain": map[string]interface{}{"type": "string", "description": "Company domain (e.g. moltsets.com)"},
			},
			"required": []string{"domain"},
		},
	},
	{
		Name:        "xeme_intel_search_companies",
		Description: "Search TheirStack's company database by technology, hiring signals, and firmographics. Only works with XEME_THEIRSTACK_API_KEY set. Returns ranked companies with tech stacks and job postings.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"technologies":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Technologies to filter by (e.g. [\"react\", \"typescript\"])"},
				"technologies_exclude": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Technologies to exclude"},
				"job_titles":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Job titles to filter by (e.g. [\"VP of Marketing\"])"},
				"industry":          map[string]interface{}{"type": "string", "description": "Industry filter (e.g. 'Information Technology')"},
				"company_size":      map[string]interface{}{"type": "string", "description": "Company size (e.g. '51-200')"},
				"country":           map[string]interface{}{"type": "string", "description": "Country name or code"},
				"hiring_since_days":  map[string]interface{}{"type": "number", "description": "Only companies that hired in last N days"},
				"exclude_agencies":  map[string]interface{}{"type": "boolean", "description": "Exclude recruiting/staffing agencies (default true)"},
				"page_size":         map[string]interface{}{"type": "number", "description": "Results per page (default 20, max 100)"},
			},
			"required": []string{},
		},
	},
	{
		Name:        "xeme_intel_search_jobs",
		Description: "Search TheirStack's 202M job postings database for intent signals. Find companies actively hiring for specific roles, technologies, or locations. Great for discovering buying intent.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"keywords":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Job title keywords (e.g. [\"VP Marketing\", \"Head of Growth\"])"},
				"technologies":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Technologies in job descriptions"},
				"company_domain":    map[string]interface{}{"type": "string", "description": "Filter to specific company domain"},
				"country_codes":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Country codes (e.g. [\"US\", \"GB\"])"},
				"posted_within_days": map[string]interface{}{"type": "number", "description": "Jobs posted in last N days"},
				"exclude_agencies":  map[string]interface{}{"type": "boolean", "description": "Exclude recruiting agencies"},
				"min_salary":        map[string]interface{}{"type": "number", "description": "Minimum salary in USD"},
				"page_size":         map[string]interface{}{"type": "number", "description": "Results per page (default 20)"},
			},
			"required": []string{},
		},
	},
	{
		Name:        "xeme_intel_tech_lookup",
		Description: "Look up the technology stack for a domain using BuiltWith's free API (no API key needed). Returns technology categories and inferred buying intent signals. Works even without TheirStack API key.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"domain": map[string]interface{}{"type": "string", "description": "Company domain (e.g. moltsets.com)"},
			},
			"required": []string{"domain"},
		},
	},
	{
		Name:        "xeme_intel_hn_hiring_signals",
		Description: "Get hiring signals from HackerNews 'Who Is Hiring' threads. Free, no API key needed. Returns companies actively hiring with tech stack signals extracted from job postings.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"keywords": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Keywords to search (company, tech, role)"},
				"limit":    map[string]interface{}{"type": "number", "description": "Max results (default 20)"},
			},
			"required": []string{},
		},
	},

	// ── AEO/GEO tools ───────────────────────────────────────

	{
		Name:        "xeme_aeo_status",
		Description: "Check Xeme AEO/GEO engine status and configuration. Shows tracked brand, domain, prompts, competitors, and AI engines. Set XEME_AEO_BRAND, XEME_AEO_DOMAIN, XEME_AEO_PROMPTS (comma-sep), XEME_AEO_COMPETITORS to enable.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_aeo_check_brand",
		Description: "Run all configured prompts against all configured AI engines (You.com, Phind, Komo — free, no API key). Returns which engines mention the brand, position, sentiment, and citation URLs. Use this to track Answer Engine Optimization (AEO) and Generative Engine Optimization (GEO) performance.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_aeo_score",
		Description: "Compute the overall AEO score (0-100) for the configured brand across all AI engines. Score is composed of: 50% mention rate, 30% citation rate, 20% position. Returns per-engine breakdown, top citations, and content gaps.",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []string{}},
	},
	{
		Name:        "xeme_aeo_optimize",
		Description: "Analyze a URL for AEO-readiness — checks word count, FAQ section, schema markup, statistics, quotes, citations, and readability. Returns a list of issues and specific recommendations to make the content more likely to be cited by AI search engines.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{"type": "string", "description": "URL to analyze for AEO-readiness"},
			},
			"required": []string{"url"},
		},
	},
	{
		Name:        "xeme_aeo_check_single",
		Description: "Check a single prompt against a single AI engine. Use this for ad-hoc AEO testing without running the full batch. Engines: you.com, phind, komo, bing_copilot (free).",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string", "description": "Prompt to send to the AI engine"},
				"engine": map[string]interface{}{"type": "string", "description": "Engine to query: you.com, phind, komo"},
			},
			"required": []string{"prompt", "engine"},
		},
	},
}

func main() {
	fmt.Fprintf(os.Stderr, "Xeme OS MCP server v%s (stdio)\n", serverVersion)
	if err := serve(os.Stdin, os.Stdout); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := json.NewEncoder(out)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return io.EOF
			}
			return err
		}
		line = []byte(trim(string(line)))

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(writer, nil, -32700, "parse error: "+err.Error())
			continue
		}

		var resp rpcResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": protocolVer,
				"serverInfo":      map[string]string{"name": serverName, "version": serverVersion},
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			}
		case "notifications/initialized":
			// No-op notification
			continue
		case "tools/list":
			resp.Result = map[string]interface{}{"tools": tools}
		case "tools/call":
			var p struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			result, err := callTool(p.Name, p.Arguments)
			if err != nil {
				resp.Result = map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": "Error: " + err.Error()}},
					"isError": true,
				}
			} else {
				text, _ := json.MarshalIndent(result, "", "  ")
				resp.Result = map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(text)}},
				}
			}
		case "ping":
			resp.Result = map[string]interface{}{}
		default:
			resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
		}

		if err := writer.Encode(&resp); err != nil {
			return err
		}
	}
}

func callTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "xeme_status":
		return toolStatus()
	case "xeme_scrape_post":
		return toolScrape(args)
	case "xeme_enrich_leads":
		return toolEnrich(args)
	case "xeme_score_leads":
		return toolScore(args)
	case "xeme_run_pipe":
		return toolPipe(args)
	case "xeme_crm_sync":
		return toolCRMSync(args)
	case "xeme_crm_status":
		return toolCRMStatus()
	case "xeme_moltsets_status":
		return toolMoltsetsStatus()
	case "xeme_moltsets_search_company":
		return toolMoltsetsSearchCompany(args)
	case "xeme_moltsets_search_people":
		return toolMoltsetsSearchPeople(args)
	case "xeme_moltsets_enrich_email":
		return toolMoltsetsEnrichEmail(args)
	case "xeme_moltsets_enrich_phone":
		return toolMoltsetsEnrichPhone(args)
	case "xeme_intel_status":
		return toolIntelStatus()
	case "xeme_intel_company_signals":
		return toolIntelCompanySignals(args)
	case "xeme_intel_search_companies":
		return toolIntelSearchCompanies(args)
	case "xeme_intel_search_jobs":
		return toolIntelSearchJobs(args)
	case "xeme_intel_tech_lookup":
		return toolIntelTechLookup(args)
	case "xeme_intel_hn_hiring_signals":
		return toolIntelHNHiringSignals(args)
	case "xeme_aeo_status":
		return toolAEOStatus()
	case "xeme_aeo_check_brand":
		return toolAEOCheckBrand()
	case "xeme_aeo_score":
		return toolAEOScore()
	case "xeme_aeo_optimize":
		return toolAEOOptimize(args)
	case "xeme_aeo_check_single":
		return toolAEOCheckSingle(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ── Tool implementations ───────────────────────────────────

func toolStatus() (interface{}, error) {
	sig := signal.New()
	en := enrich.New()
	led := ledger.New()
	out := map[string]interface{}{
		"engines": map[string]interface{}{
			"signal": map[string]interface{}{"version": sig.Version(), "ok": sig.Health() == nil},
			"enrich": map[string]interface{}{"version": en.Version(), "ok": en.Health() == nil, "balance": en.Balance()},
		},
		"xeme_os": map[string]interface{}{"version": serverVersion},
	}
	// MoltSets status
	ms := enrich.NewMoltSetsEngine(nil)
	if ms != nil {
		out["engines"].(map[string]interface{})["moltsets"] = map[string]interface{}{"version": ms.Version()}
	}
	if st, err := led.Health(); err == nil {
		out["engines"].(map[string]interface{})["ledger"] = st
	} else {
		out["engines"].(map[string]interface{})["ledger"] = map[string]interface{}{"ok": false, "error": err.Error()}
	}
	return out, nil
}

func toolScrape(args map[string]interface{}) (interface{}, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	out, _ := args["out"].(string)
	if out == "" {
		out = filepath.Join("/tmp", fmt.Sprintf("xeme-signal-%d.json", time.Now().Unix()))
	}
	pages := 10
	if v, ok := args["pages"].(float64); ok {
		pages = int(v)
	}
	s := signal.New()
	s.MaxPages = pages
	res, err := s.Scrape(url)
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}, nil
	}
	return map[string]interface{}{"ok": true, "output": out, "total": res.Total, "source": res.Source}, nil
}

func toolEnrich(args map[string]interface{}) (interface{}, error) {
	in, _ := args["input"].(string)
	if in == "" {
		return nil, fmt.Errorf("input is required")
	}
	out, _ := args["output"].(string)
	if out == "" {
		out = filepath.Join("/tmp", fmt.Sprintf("xeme-enriched-%d.csv", time.Now().Unix()))
	}
	e := enrich.New()
	res, err := e.Waterfall(in, out)
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}, nil
	}
	return map[string]interface{}{
		"ok":            true,
		"input":         in,
		"output":        res.OutputPath,
		"rows_in":        res.RowsIn,
		"rows_out":       res.RowsOut,
		"credits_spent":  res.CreditsSpent,
		"balance":        map[string]float64{"before": res.BalanceBefore, "after": res.BalanceAfter},
	}, nil
}

func toolScore(args map[string]interface{}) (interface{}, error) {
	in, _ := args["input"].(string)
	if in == "" {
		return nil, fmt.Errorf("input is required")
	}
	out, _ := args["output"].(string)
	if out == "" {
		out = filepath.Join("/tmp", fmt.Sprintf("xeme-scored-%d.csv", time.Now().Unix()))
	}
	minScore := 0
	if v, ok := args["min_score"].(float64); ok {
		minScore = int(v)
	}
	rows, err := csvkit.Read(in)
	if err != nil {
		return nil, err
	}
	e := pipe.New(pipe.Config{WorkspaceDir: "/tmp"})
	leads := make([]score.Lead, 0, len(rows))
	for _, r := range rows {
		leads = append(leads, score.Lead{
			FirstName: r["first_name"],
			LastName:  r["last_name"],
			Title:     r["title"],
			Company:   firstNonEmpty(r["company_name"], r["company"]),
			Domain:    r["domain"],
			Email:     firstNonEmpty(r["extracted_email"], r["email"]),
			LinkedIn:  firstNonEmpty(r["linkedin_url"], r["linkedin"]),
			Signal:    firstNonEmpty(r["signal_source"], r["signal"]),
		})
	}
	scored := e.Scorer.Batch(leads)
	if minScore > 0 {
		filtered := scored[:0]
		for _, l := range scored {
			if l.Score >= minScore {
				filtered = append(filtered, l)
			}
		}
		scored = filtered
	}
	summary := score.Summarize(scored)
	return map[string]interface{}{
		"ok":      true,
		"output":  out,
		"summary": summary,
		"sample":  sample(scored, 5),
	}, nil
}

func toolPipe(args map[string]interface{}) (interface{}, error) {
	in, _ := args["input"].(string)
	if in == "" {
		return nil, fmt.Errorf("input is required")
	}
	noCRM, _ := args["no_crm"].(bool)
	dryRun, _ := args["dry_run"].(bool)
	e := pipe.New(pipe.Config{
		WorkspaceDir: "/tmp",
		Stages:       pipe.StagesConfig{Enrich: true, Score: true, Sync: true},
		NoCRM:        noCRM,
		DryRun:       dryRun,
	})
	return e.Run(in)
}

func toolCRMSync(args map[string]interface{}) (interface{}, error) {
	in, _ := args["input"].(string)
	if in == "" {
		return nil, fmt.Errorf("input is required")
	}
	dryRun, _ := args["dry_run"].(bool)
	rows, err := csvkit.Read(in)
	if err != nil {
		return nil, err
	}
	l := ledger.New()
	contacts := make([]ledger.PersonInput, 0, len(rows))
	for _, r := range rows {
		contacts = append(contacts, ledger.PersonInput{
			FirstName: r["first_name"],
			LastName:  r["last_name"],
			Email:     firstNonEmpty(r["extracted_email"], r["email"]),
			JobTitle:  r["title"],
			LinkedIn:  firstNonEmpty(r["linkedin_url"], r["linkedin"]),
		})
	}
	return l.Sync(contacts, dryRun)
}

func toolCRMStatus() (interface{}, error) {
	l := ledger.New()
	return l.Health()
}

// ── MoltSets tool implementations ──────────────────────────────────

func toolMoltsetsStatus() (interface{}, error) {
	ms := enrich.NewMoltSetsEngine(nil)
	if ms == nil {
		return map[string]interface{}{"ok": false, "error": "XEME_MOLTSETS_API_KEY not set"}, nil
	}
	account, _, err := ms.GetAccount(context.Background())
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}, nil
	}
	billing, _, _ := ms.GetBilling(context.Background())
	return map[string]interface{}{
		"ok":     true,
		"email":  account.Email,
		"plan":   account.Plan,
		"status": account.Status,
		"tokens_remaining": account.Metadata.TokensRemaining,
		"billing": billing,
	}, nil
}

func toolMoltsetsSearchCompany(args map[string]interface{}) (interface{}, error) {
	ms := enrich.NewMoltSetsEngine(nil)
	if ms == nil {
		return nil, fmt.Errorf("XEME_MOLTSETS_API_KEY not set")
	}
	p := enrich.SearchCompanyParams{}
	if v, ok := args["query"].(string); ok {
		p.Query = v
	}
	if v, ok := args["domain"].(string); ok {
		p.Domain = v
	}
	if v, ok := args["industry"].(string); ok {
		p.Industry = v
	}
	if v, ok := args["employee_range"].(string); ok {
		p.EmployeeRange = v
	}
	if v, ok := args["revenue_range"].(string); ok {
		p.RevenueRange = v
	}
	if v, ok := args["limit"].(float64); ok {
		p.Limit = int(v)
	}
	ctx := contextWithTimeout(30)
	result, _, err := ms.SearchCompanies(ctx, p)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":     true,
		"total":  result.Results.Total,
		"results": result.Results.Results,
	}, nil
}

func toolMoltsetsSearchPeople(args map[string]interface{}) (interface{}, error) {
	ms := enrich.NewMoltSetsEngine(nil)
	if ms == nil {
		return nil, fmt.Errorf("XEME_MOLTSETS_API_KEY not set")
	}
	p := enrich.SearchPeopleParams{}
	if v, ok := args["company_domain"].(string); ok {
		p.CompanyDomain = v
	}
	if v, ok := args["industry"].(string); ok {
		p.Industry = v
	}
	if v, ok := args["employee_range"].(string); ok {
		p.EmployeeRange = v
	}
	if v, ok := args["revenue_range"].(string); ok {
		p.RevenueRange = v
	}
	if v, ok := args["limit"].(float64); ok {
		p.Limit = int(v)
	}
	ctx := contextWithTimeout(30)
	result, _, err := ms.SearchPeople(ctx, p)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":     true,
		"total":  result.Results.Total,
		"results": result.Results.Results,
	}, nil
}

func toolMoltsetsEnrichEmail(args map[string]interface{}) (interface{}, error) {
	ms := enrich.NewMoltSetsEngine(nil)
	if ms == nil {
		return nil, fmt.Errorf("XEME_MOLTSETS_API_KEY not set")
	}
	linkedinURL, _ := args["linkedin_url"].(string)
	if linkedinURL == "" {
		return nil, fmt.Errorf("linkedin_url is required")
	}
	ctx := contextWithTimeout(30)
	result, _, err := ms.EnrichEmail(ctx, linkedinURL)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":          true,
		"linkedin_url": linkedinURL,
		"email":        result.Results.Email,
		"type":         result.Results.Type,
		"score":        result.Results.Score,
	}, nil
}

func toolMoltsetsEnrichPhone(args map[string]interface{}) (interface{}, error) {
	ms := enrich.NewMoltSetsEngine(nil)
	if ms == nil {
		return nil, fmt.Errorf("XEME_MOLTSETS_API_KEY not set")
	}
	linkedinURL, _ := args["linkedin_url"].(string)
	if linkedinURL == "" {
		return nil, fmt.Errorf("linkedin_url is required")
	}
	ctx := contextWithTimeout(30)
	result, _, err := ms.EnrichPhone(ctx, enrich.EnrichPhoneParams{LinkedInURL: linkedinURL})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":          true,
		"linkedin_url": linkedinURL,
		"phone":        result.Results.Phone,
		"carrier":      result.Results.Carrier,
		"validated":    result.Results.Validated,
	}, nil
}
// ── Intel tool implementations ──────────────────────────────────

func toolIntelStatus() (interface{}, error) {
	intelEng := intel.New()
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	status := intelEng.Status(ctx)
	// Add engine list to xeme_status output
	out := map[string]interface{}{"providers": status}
	return out, nil
}

func toolIntelCompanySignals(args map[string]interface{}) (interface{}, error) {
	domain, _ := args["domain"].(string)
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	intelEng := intel.New()
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	result, err := intelEng.GetSignals(ctx, domain)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func toolIntelSearchCompanies(args map[string]interface{}) (interface{}, error) {
	intelEng := intel.New()
	if intelEng.TheirStack == nil {
		return nil, fmt.Errorf("theirstack: XEME_THEIRSTACK_API_KEY not set")
	}
	params := intel.CompanySearchParams{
		PageSize: 20,
	}
	if v, ok := args["technologies"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				params.Technologies = append(params.Technologies, s)
			}
		}
	}
	if v, ok := args["technologies_exclude"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				params.TechnologiesExclude = append(params.TechnologiesExclude, s)
			}
		}
	}
	if v, ok := args["job_titles"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				params.JobTitles = append(params.JobTitles, s)
			}
		}
	}
	if v, ok := args["industry"].(string); ok {
		params.Industry = v
	}
	if v, ok := args["company_size"].(string); ok {
		params.CompanySize = v
	}
	if v, ok := args["country"].(string); ok {
		params.Country = v
	}
	if v, ok := args["hiring_since_days"].(float64); ok {
		params.HiringSinceDays = int(v)
	}
	if v, ok := args["exclude_agencies"].(bool); ok {
		params.ExcludeAgencies = v
	} else {
		params.ExcludeAgencies = true
	}
	if v, ok := args["page_size"].(float64); ok {
		params.PageSize = int(v)
	}
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	companies, tokens, err := intelEng.SearchCompanies(ctx, params)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok": true,
		"total": len(companies),
		"tokens_used": tokens,
		"companies": companies,
	}, nil
}

func toolIntelSearchJobs(args map[string]interface{}) (interface{}, error) {
	intelEng := intel.New()
	if intelEng.TheirStack == nil {
		return nil, fmt.Errorf("theirstack: XEME_THEIRSTACK_API_KEY not set")
	}
	params := intel.JobSearchParams{
		PageSize: 20,
	}
	if v, ok := args["keywords"].([]interface{}); ok {
		for _, kw := range v {
			if s, ok := kw.(string); ok {
				params.Keywords = append(params.Keywords, s)
			}
		}
	}
	if v, ok := args["technologies"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				params.Technologies = append(params.Technologies, s)
			}
		}
	}
	if v, ok := args["company_domain"].(string); ok {
		params.CompanyDomain = v
	}
	if v, ok := args["posted_within_days"].(float64); ok {
		params.PostedWithinDays = int(v)
	}
	if v, ok := args["exclude_agencies"].(bool); ok {
		params.ExcludeAgencies = v
	}
	if v, ok := args["min_salary"].(float64); ok {
		params.MinSalary = int(v)
	}
	if v, ok := args["page_size"].(float64); ok {
		params.PageSize = int(v)
	}
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	jobs, tokens, err := intelEng.SearchJobs(ctx, params)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok": true,
		"total": len(jobs),
		"tokens_used": tokens,
		"jobs": jobs,
	}, nil
}

func toolIntelTechLookup(args map[string]interface{}) (interface{}, error) {
	domain, _ := args["domain"].(string)
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	eng := intel.NewBuiltWithEngine(nil)
	ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)
	result, err := eng.TechSignal(ctx, domain)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok": true,
		"domain": domain,
		"technologies": result.Technologies,
		"categories": result.Categories,
		"signals": result.Signals,
		"source": "builtwith-free",
	}, nil
}

func toolIntelHNHiringSignals(args map[string]interface{}) (interface{}, error) {
	eng := intel.NewSignalScraper(nil)
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	var keywords []string
	if v, ok := args["keywords"].([]interface{}); ok {
		for _, kw := range v {
			if s, ok := kw.(string); ok {
				keywords = append(keywords, s)
			}
		}
	}
	limit := 20
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}
	eng.Config.MaxResults = limit
	results, err := eng.SearchHNWhoIsHiring(ctx, keywords)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok": true,
		"total": len(results),
		"source": "hackernews-free",
		"results": results,
	}, nil
}

// ── AEO/GEO tool implementations ──────────────────────────────────

func toolAEOStatus() (interface{}, error) {
	eng := aeo.New(nil)
	out := map[string]interface{}{
		"version":             eng.Version(),
		"ok":                  eng.Health(context.Background()) == nil,
		"brand":               eng.Config.Brand,
		"domain":              eng.Config.Domain,
		"prompts_tracked":     len(eng.Config.Prompts),
		"competitors_tracked": len(eng.Config.Competitors),
		"engines":             eng.Config.Engines,
		"prompts":             eng.Config.Prompts,
		"competitors":         eng.Config.Competitors,
	}
	if eng.Config.Brand == "" {
		out["hint"] = "Set XEME_AEO_BRAND, XEME_AEO_DOMAIN, XEME_AEO_PROMPTS (comma-sep), XEME_AEO_COMPETITORS"
	}
	return out, nil
}

func toolAEOCheckBrand() (interface{}, error) {
	eng := aeo.New(nil)
	if eng.Config.Brand == "" {
		return nil, fmt.Errorf("XEME_AEO_BRAND not set")
	}
	ctx, _ := context.WithTimeout(context.Background(), 90*time.Second)
	results, err := eng.CheckBrand(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":        true,
		"brand":     eng.Config.Brand,
		"checked":   len(results),
		"results":   results,
		"citations": eng.FindCitations(results),
		"gaps":      eng.FindGaps(results),
	}, nil
}

func toolAEOScore() (interface{}, error) {
	eng := aeo.New(nil)
	if eng.Config.Brand == "" {
		return nil, fmt.Errorf("XEME_AEO_BRAND not set")
	}
	ctx, _ := context.WithTimeout(context.Background(), 90*time.Second)
	results, _ := eng.CheckBrand(ctx)
	return eng.Score(results), nil
}

func toolAEOOptimize(args map[string]interface{}) (interface{}, error) {
	target, _ := args["url"].(string)
	if target == "" {
		return nil, fmt.Errorf("url is required")
	}
	eng := aeo.New(nil)
	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
	opt, err := eng.Optimize(ctx, target)
	if err != nil {
		return nil, err
	}
	return opt, nil
}

func toolAEOCheckSingle(args map[string]interface{}) (interface{}, error) {
	prompt, _ := args["prompt"].(string)
	engine, _ := args["engine"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if engine == "" {
		engine = "you.com"
	}
	eng := aeo.New(nil)
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	return eng.CheckMention(ctx, engine, prompt)
}


// contextWithTimeout creates a context with the given timeout in seconds.
func contextWithTimeout(seconds int) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	return ctx
}

// ── Helpers ───────────────────────────────────────────────

func sample(leads []score.Lead, n int) []map[string]interface{} {
	if n > len(leads) {
		n = len(leads)
	}
	out := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		l := leads[i]
		out = append(out, map[string]interface{}{
			"name":    trim(l.FirstName + " " + l.LastName),
			"title":   l.Title,
			"company": l.Company,
			"email":   l.Email,
			"score":   l.Score,
			"tier":    l.Tier,
		})
	}
	return out
}

func sendError(w *json.Encoder, id json.RawMessage, code int, msg string) {
	_ = w.Encode(&rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

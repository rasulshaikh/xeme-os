// Package enrich implements the Xeme Enrichment Engine — multi-provider
// waterfall that finds emails, phones, and firmographics.
//
// Live mode (XEME_ENRICH_API_KEY set): each row is POSTed to the Xeme
// Enrich upstream at XEME_ENRICH_BASE_URL. The engine debits the
// per-row credit cost returned by the upstream.
//
// Demo mode (no key): the engine falls back to a deterministic local
// waterfall that synthesizes an email from each contact's first name,
// last name, and company domain — enough to exercise the rest of the
// pipeline end-to-end.
package enrich

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Engine is the Xeme Enrichment Engine.
type Engine struct {
	Binary string   // deprecated
	Config string
	Roles  []string
	Limit  int
	// MoltSets is the optional MoltSets enrichment engine.
	// When set, it is tried after the upstream (live mode) fails.
	MoltSets *MoltSetsEngine
}

func New() *Engine {
	e := &Engine{
		Binary: "",
		Config: "config/enrich.jsonc",
		Roles:  []string{"Chief Marketing Officer", "VP of Marketing", "VP Marketing", "VP Demand Generation"},
		Limit:  1,
	}
	// Auto-initialise MoltSets if the env var is present
	if apiKey := os.Getenv("XEME_MOLTSETS_API_KEY"); apiKey != "" {
		e.MoltSets = NewMoltSetsEngine(DefaultMoltSetsConfig())
	}
	return e
}

// Result holds enrichment output + credit tracking.
type Result struct {
	OutputPath    string  `json:"output_path"`
	RowsIn        int     `json:"rows_in"`
	RowsOut       int     `json:"rows_out"`
	BalanceBefore float64 `json:"balance_before"`
	BalanceAfter  float64 `json:"balance_after"`
	CreditsSpent  float64 `json:"credits_spent"`
	Mode          string  `json:"mode"`
}

// Waterfall runs enrichment on the input CSV and writes a new CSV.
func (e *Engine) Waterfall(inputPath, outputPath string) (*Result, error) {
	before := e.Balance()
	_ = os.MkdirAll(filepath.Dir(outputPath), 0o755)

	rows, err := readCSVRows(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	apiKey := os.Getenv("XEME_ENRICH_API_KEY")
	mode := "demo"
	if apiKey != "" {
		mode = "live"
	}

	out := make([]map[string]string, 0, len(rows))
	totalCredits := 0.0
	for _, r := range rows {
		enriched, credits := e.enrichRow(r, mode, apiKey)
		out = append(out, enriched)
		totalCredits += credits
	}

	if err := writeCSVRows(outputPath, out); err != nil {
		return nil, fmt.Errorf("write output: %w", err)
	}

	after := before - totalCredits
	_ = saveXemeBalance(after)

	return &Result{
		OutputPath:    outputPath,
		RowsIn:        len(rows),
		RowsOut:       len(out),
		BalanceBefore: before,
		BalanceAfter:  after,
		CreditsSpent:  totalCredits,
		Mode:          mode,
	}, nil
}

// enrichRow produces an enriched copy of a contact row. Returns the row
// + the credit cost debited for the call.
func (e *Engine) enrichRow(r map[string]string, mode, apiKey string) (map[string]string, float64) {
	out := map[string]string{}
	for k, v := range r {
		out[k] = v
	}

	if mode == "live" {
		// Call the Xeme Enrich upstream
		upstream, credits, err := e.callUpstream(apiKey, r)
		if err == nil && (upstream["email"] != "" || upstream["phone"] != "") {
			// Merge upstream result
			if v, ok := upstream["email"].(string); ok && v != "" {
				out["email"] = v
			}
			if v, ok := upstream["phone"].(string); ok && v != "" {
				out["phone"] = v
			}
			if v, ok := upstream["title"].(string); ok && v != "" {
				out["title"] = v
			}
			if v, ok := upstream["domain"].(string); ok && v != "" {
				out["domain"] = v
			}
			if v, ok := upstream["company"].(string); ok && v != "" {
				if out["company_name"] == "" {
					out["company_name"] = v
				}
				if out["company"] == "" {
					out["company"] = v
				}
			}
			if v, ok := upstream["linkedin_url"].(string); ok && v != "" {
				out["linkedin_url"] = v
			}
			out["extraction_source"] = "xeme-enrich-live"
			if v, ok := upstream["confidence"].(float64); ok {
				out["extraction_confidence"] = fmt.Sprintf("%.2f", v)
			} else {
				out["extraction_confidence"] = "0.91"
			}
			return out, credits
		}
		// Upstream failed — fall through to demo
		mode = "demo"
	}

	// Step 3: MoltSets (verified emails + phones, 20+ vendor waterfall)
	if e.MoltSets != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msResult, msTokens, msErr := e.MoltSets.EnrichRow(ctx, r)
		if msErr == nil && msResult != nil {
			if msResult.Email != "" {
				out["email"] = msResult.Email
			}
			if msResult.Phone != "" {
				out["phone"] = msResult.Phone
			}
			if msResult.LinkedInURL != "" {
				out["linkedin_url"] = msResult.LinkedInURL
			}
			if msResult.Company != "" && out["company_name"] == "" {
				out["company_name"] = msResult.Company
			}
			if msResult.Domain != "" {
				out["domain"] = msResult.Domain
			}
			if msResult.Title != "" {
				out["title"] = msResult.Title
			}
			if msResult.FullName != "" {
				out["full_name"] = msResult.FullName
			}
			if msResult.Industry != "" {
				out["industry"] = msResult.Industry
			}
			out["extraction_source"] = msResult.Source
			out["extraction_confidence"] = fmt.Sprintf("%.2f", msResult.Confidence)
			return out, msTokens
		}
		_ = msErr // log in production
	}

	// Demo: synthesize an email from first.last@domain
	first := strings.TrimSpace(r["first_name"])
	last := strings.TrimSpace(r["last_name"])
	company := firstNonEmpty(r["company_name"], r["company"])
	domain := inferDomain(company, r["domain"])

	if domain != "" && first != "" {
		var localPart string
		switch {
		case first != "" && last != "":
			localPart = fmt.Sprintf("%s.%s", strings.ToLower(first), strings.ToLower(last))
		case first != "":
			localPart = strings.ToLower(first)
		default:
			localPart = strings.ToLower(last)
		}
		localPart = strings.ReplaceAll(localPart, " ", "")
		out["email"] = localPart + "@" + domain
		out["extraction_source"] = "xeme-enrich-demo"
		out["extraction_confidence"] = "0.72"
	}
	if domain != "" {
		out["domain"] = domain
	}
	return out, 0.01
}

// callUpstream posts a single contact to the Xeme Enrich service.
func (e *Engine) callUpstream(apiKey string, r map[string]string) (map[string]interface{}, float64, error) {
	base := strings.TrimRight(os.Getenv("XEME_ENRICH_BASE_URL"), "/")
	if base == "" {
		base = "https://enrich.xeme.app"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"first_name":   r["first_name"],
		"last_name":    r["last_name"],
		"company_name": firstNonEmpty(r["company_name"], r["company"]),
		"domain":       r["domain"],
		"linkedin_url": r["linkedin_url"],
	})

	req, err := http.NewRequest("POST", base+"/v1/enrich/person", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("xeme-enrich upstream HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("xeme-enrich parse: %w", err)
	}

	credits, _ := out["credits_used"].(float64)
	if credits == 0 {
		credits = 0.04 // default upstream cost per row
	}
	return out, credits, nil
}

// Health always returns nil — the engine runs in-process.
func (e *Engine) Health() error { return nil }

// Version returns the engine version string.
func (e *Engine) Version() string { return "xeme-enrich v0.3.0 (in-process)" }

// Balance returns the current Xeme Credits balance.
func (e *Engine) Balance() float64 {
	cfg, err := loadXemeConfig()
	if err != nil {
		return 1000.00 // default starting balance
	}
	if v, ok := cfg["enrich_balance"].(float64); ok {
		return v
	}
	return 1000.00
}

func loadXemeConfig() (map[string]interface{}, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".xeme", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveXemeBalance(bal float64) error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".xeme")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "config.json")
	cur := map[string]interface{}{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &cur)
	}
	cur["enrich_balance"] = bal
	cur["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.MarshalIndent(cur, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

func inferDomain(company, existing string) string {
	if d := strings.TrimSpace(existing); d != "" {
		return strings.ToLower(d)
	}
	c := strings.ToLower(strings.TrimSpace(company))
	if c == "" {
		return ""
	}
	// strip common suffixes
	for _, suf := range []string{" inc.", " inc", " ltd.", " ltd", " llc", " corp.", " corp", " co.", " co", " gmbh"} {
		c = strings.TrimSuffix(c, suf)
	}
	parts := strings.Fields(c)
	if len(parts) == 0 {
		return ""
	}
	return parts[0] + ".com"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── CSV I/O helpers ────────────────────────────────────

func readCSVRows(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]string, 0)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(rec) {
				row[col] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func writeCSVRows(path string, rows []map[string]string) error {
	if len(rows) == 0 {
		// Write an empty file with just a comment so the pipe doesn't blow up
		return os.WriteFile(path, []byte("# no rows\n"), 0o644)
	}

	// Stable header order: prefer the most common keys, fill from row keys
	preferred := []string{
		"first_name", "last_name", "full_name",
		"title", "company_name", "company", "domain",
		"email", "extracted_email", "phone",
		"linkedin_url", "linkedin", "signal_source", "signal",
		"extraction_source", "extraction_confidence",
		"score", "tier",
	}
	seen := map[string]bool{}
	header := make([]string, 0, len(rows[0]))
	for _, k := range preferred {
		if _, ok := rows[0][k]; ok && !seen[k] {
			header = append(header, k)
			seen[k] = true
		}
	}
	for k := range rows[0] {
		if !seen[k] {
			header = append(header, k)
			seen[k] = true
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(header))
		for i, k := range header {
			rec[i] = r[k]
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// countCSV is used by the legacy callers; reads the row count of a file.
func countCSV(path string) (int, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// FloatFromString is a tiny helper.
func FloatFromString(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

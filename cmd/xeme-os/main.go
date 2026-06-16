// XEME OS — Local Dashboard
// A self-hosted web dashboard for Xeme OS that runs natively on Apple Silicon (M3).
// Serves a beautiful, dark-themed control panel on http://localhost:4903
//
// Built with the Go stdlib only — no framework, no npm, no node.
//
// Endpoints:
//   GET  /              → dashboard HTML
//   GET  /api/status    → JSON status of all engines
//   GET  /api/engines   → engine health
//   GET  /api/leads     → recent enriched leads
//   GET  /api/runs      → pipeline run history
//   GET  /api/signals   → recent signals
//   GET  /api/tools     → list MCP tools
//   POST /api/run/enrich → run enrichment on a CSV
//   POST /api/run/score  → run scoring on a CSV
//   POST /api/run/pipe   → run full pipe
package main

import (
	_ "embed"
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xeme-os/xeme/internal/aeo"
	"github.com/xeme-os/xeme/internal/csvkit"
	"github.com/xeme-os/xeme/internal/enrich"
	"github.com/xeme-os/xeme/internal/intel"
	"github.com/xeme-os/xeme/internal/ledger"
	"github.com/xeme-os/xeme/internal/pipe"
	"github.com/xeme-os/xeme/internal/score"
	"github.com/xeme-os/xeme/internal/signal"
)

const (
	appName    = "XEME OS"
	appVersion = "2.0.0"
	port       = 4903
)

//go:embed dashboard.html
var dashboardHTML string

func main() {
	host := flag.String("host", "127.0.0.1", "host to bind")
	portF := flag.Int("port", port, "port to bind")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *portF)
	fmt.Printf("🚀 %s v%s (Apple Silicon native)\n", appName, appVersion)
	fmt.Printf("→ Dashboard:  http://%s\n", addr)
	fmt.Printf("→ Health:     http://%s/api/status\n", addr)
	fmt.Println()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/engines", handleEngines)
	mux.HandleFunc("/api/leads", handleLeads)
	mux.HandleFunc("/api/runs", handleRuns)
	mux.HandleFunc("/api/signals", handleSignals)
	mux.HandleFunc("/api/tools", handleTools)
	mux.HandleFunc("/api/run/enrich", handleRunEnrich)
	mux.HandleFunc("/api/run/score", handleRunScore)
	mux.HandleFunc("/api/run/pipe", handleRunPipe)
	mux.HandleFunc("/api/intel/lookup", handleIntelLookup)
	mux.HandleFunc("/api/intel/status", handleIntelStatus)
	mux.HandleFunc("/api/aeo/status", handleAEOStatus)
	mux.HandleFunc("/api/aeo/check", handleAEOCheck)
	mux.HandleFunc("/api/aeo/score", handleAEOScore)
	mux.HandleFunc("/api/aeo/optimize", handleAEOOptimize)

	// Wrap with CORS + logging
	handler := middleware(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS for local dev
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Log requests
		start := time.Now()
		h.ServeHTTP(w, r)
		dur := time.Since(start)
		fmt.Printf("  %s %s %s %s %dms\n",
			time.Now().Format("15:04:05"), r.Method, r.URL.Path, r.RemoteAddr, dur.Milliseconds())
	})
}

// ── Page handler ──────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	t, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, map[string]interface{}{
		"AppName":    appName,
		"AppVersion": appVersion,
		"Arch":       runtime.GOARCH,
		"GOOS":       runtime.GOOS,
		"Port":       port,
	})
}

// ── API handlers ───────────────────────────────────────────

func handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"app":       appName,
		"version":   appVersion,
		"arch":      runtime.GOARCH,
		"os":        runtime.GOOS,
		"goVersion": runtime.Version(),
		"cpus":      runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"uptime":    time.Since(startTime).String(),
		"now":       time.Now().Format(time.RFC3339),
	}
	status["engines"] = getAllEngineStatus(r.Context())
	writeJSON(w, status)
}

func handleEngines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, getAllEngineStatus(r.Context()))
}

func getAllEngineStatus(ctx context.Context) map[string]interface{} {
	out := map[string]interface{}{}

	// Xeme Signal Engine
	sig := signal.New()
	out["signal"] = map[string]interface{}{
		"version": sig.Version(),
		"ok":      sig.Health() == nil,
	}

	// Xeme Enrich Engine (includes MoltSets)
	en := enrich.New()
	enrichStatus := map[string]interface{}{
		"version": en.Version(),
		"ok":      en.Health() == nil,
		"balance": en.Balance(),
	}
	// MoltSets
	if apiKey := os.Getenv("XEME_MOLTSETS_API_KEY"); apiKey != "" {
		ms := enrich.NewMoltSetsEngine(nil)
		enrichStatus["moltsets"] = map[string]interface{}{
			"version": ms.Version(),
			"enabled": true,
		}
	} else {
		enrichStatus["moltsets"] = map[string]interface{}{"enabled": false, "reason": "XEME_MOLTSETS_API_KEY not set"}
	}
	out["enrich"] = enrichStatus

	// Xeme Intel Engine
	intelEng := intel.New()
	intelStatus := map[string]interface{}{
		"version": intelEng.Version(),
	}
	if intelEng.TheirStack != nil {
		intelStatus["theirstack"] = map[string]interface{}{
			"enabled": true,
			"version": intelEng.TheirStack.Version(),
		}
	} else {
		intelStatus["theirstack"] = map[string]interface{}{"enabled": false, "reason": "XEME_THEIRSTACK_API_KEY not set"}
	}
	intelStatus["builtwith"] = map[string]interface{}{
		"enabled": true,
		"version": intelEng.BuiltWith.Version(),
	}
	intelStatus["scraper"] = map[string]interface{}{
		"enabled": true,
		"version": intelEng.Scraper.Version(),
	}
	out["intel"] = intelStatus

	// Xeme AEO/GEO Engine
	aeoEng := aeo.New(nil)
	aeoStatus := map[string]interface{}{
		"version": aeoEng.Version(),
		"ok":      aeoEng.Health(ctx) == nil,
	}
	if aeoEng.Config.Brand != "" {
		aeoStatus["brand"] = aeoEng.Config.Brand
		aeoStatus["domain"] = aeoEng.Config.Domain
		aeoStatus["prompts_tracked"] = len(aeoEng.Config.Prompts)
		aeoStatus["competitors_tracked"] = len(aeoEng.Config.Competitors)
	} else {
		aeoStatus["brand"] = ""
		aeoStatus["hint"] = "set XEME_AEO_BRAND and XEME_AEO_PROMPTS to enable tracking"
	}
	out["aeo"] = aeoStatus

	// Xeme Ledger
	led := ledger.New()
	if st, err := led.Health(); err == nil {
		out["ledger"] = st
	} else {
		out["ledger"] = map[string]interface{}{"ok": false, "error": err.Error()}
	}

	return out
}

func handleLeads(w http.ResponseWriter, r *http.Request) {
	rows, err := loadRecentCSV("final_*.csv", 50)
	if err != nil {
		writeJSON(w, map[string]interface{}{"leads": []interface{}{}, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"leads": rows, "count": len(rows)})
}

func handleRuns(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(filepath.Join(getWorkspace(), "live_*.csv"))
	files = append(files, listFiles(getLogsDir(), "*.log")...)

	type Run struct {
		Name     string    `json:"name"`
		Type     string    `json:"type"`
		Size     int64     `json:"size_bytes"`
		Modified time.Time `json:"modified"`
		Path     string    `json:"path"`
	}

	var runs []Run
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		runs = append(runs, Run{
			Name:     filepath.Base(f),
			Type:     strings.TrimPrefix(filepath.Ext(f), "."),
			Size:     info.Size(),
			Modified: info.ModTime(),
			Path:     f,
		})
	}
	writeJSON(w, map[string]interface{}{"runs": runs, "count": len(runs)})
}

func handleSignals(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(filepath.Join(getWorkspace(), "live_signal.json"))
	signals := []map[string]interface{}{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var sig map[string]interface{}
		if err := json.Unmarshal(data, &sig); err == nil {
			signals = append(signals, sig)
		}
	}
	writeJSON(w, map[string]interface{}{"signals": signals, "count": len(signals)})
}

func handleTools(w http.ResponseWriter, r *http.Request) {
	tools := []map[string]interface{}{
		{"name": "xeme_status", "engine": "core", "description": "Health check across all Xeme engines"},
		{"name": "xeme_scrape_post", "engine": "signal", "description": "Scrape engagers from a public post URL"},
		{"name": "xeme_enrich_leads", "engine": "enrich", "description": "Waterfall-enrich a CSV"},
		{"name": "xeme_score_leads", "engine": "score", "description": "Apply 7-gate ICP scoring"},
		{"name": "xeme_run_pipe", "engine": "pipe", "description": "End-to-end: scrape → enrich → score → sync"},
		{"name": "xeme_crm_sync", "engine": "ledger", "description": "Sync CSV to Xeme Ledger"},
		{"name": "xeme_moltsets_status", "engine": "enrich.moltsets", "description": "Check MoltSets account"},
		{"name": "xeme_moltsets_search_company", "engine": "enrich.moltsets", "description": "Search MoltSets companies"},
		{"name": "xeme_moltsets_enrich_email", "engine": "enrich.moltsets", "description": "MoltSets email enrichment via LinkedIn"},
		{"name": "xeme_intel_status", "engine": "intel", "description": "Check intel providers status"},
		{"name": "xeme_intel_company_signals", "engine": "intel.theirstack", "description": "Get all signals for a company"},
		{"name": "xeme_intel_search_companies", "engine": "intel.theirstack", "description": "Search companies by tech + hiring"},
		{"name": "xeme_intel_search_jobs", "engine": "intel.theirstack", "description": "Search 202M job postings"},
		{"name": "xeme_intel_tech_lookup", "engine": "intel.builtwith", "description": "Tech stack detection (free, no key)"},
		{"name": "xeme_intel_hn_hiring_signals", "engine": "intel.scraper", "description": "HN Who Is Hiring signals"},
	}
	writeJSON(w, map[string]interface{}{"tools": tools, "count": len(tools)})
}

// ── Run handlers ───────────────────────────────────────────

func handleRunEnrich(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	var body struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if body.Input == "" {
		http.Error(w, "input is required", 400)
		return
	}
	if body.Output == "" {
		body.Output = filepath.Join(getWorkspace(), fmt.Sprintf("xeme-enrich-%d.csv", time.Now().Unix()))
	}
	e := enrich.New()
	res, err := e.Waterfall(body.Input, body.Output)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"output":       res.OutputPath,
		"rows_in":      res.RowsIn,
		"rows_out":     res.RowsOut,
		"credits_used": res.CreditsSpent,
		"mode":         res.Mode,
	})
}

func handleRunScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	var body struct {
		Input    string  `json:"input"`
		Output   string  `json:"output"`
		MinScore float64 `json:"min_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if body.Input == "" {
		http.Error(w, "input is required", 400)
		return
	}
	if body.Output == "" {
		body.Output = filepath.Join(getWorkspace(), fmt.Sprintf("xeme-score-%d.csv", time.Now().Unix()))
	}
	rows, err := csvkit.Read(body.Input)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	e := pipe.New(pipe.Config{WorkspaceDir: filepath.Dir(body.Output)})
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
	if body.MinScore > 0 {
		filtered := scored[:0]
		for _, l := range scored {
			if l.Score >= int(body.MinScore) {
				filtered = append(filtered, l)
			}
		}
		scored = filtered
	}
	summary := score.Summarize(scored)
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"output":  body.Output,
		"summary": summary,
		"count":   len(scored),
	})
}

func handleRunPipe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	var body struct {
		Input    string `json:"input"`
		NoCRM    bool   `json:"no_crm"`
		DryRun   bool   `json:"dry_run"`
		MinScore int    `json:"min_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if body.Input == "" {
		http.Error(w, "input is required", 400)
		return
	}
	e := pipe.New(pipe.Config{
		WorkspaceDir: getWorkspace(),
		Stages:       pipe.StagesConfig{Enrich: true, Score: true, Sync: true},
		NoCRM:        body.NoCRM,
		DryRun:       body.DryRun,
	})
	result, err := e.Run(body.Input)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

// ── Intel handlers ─────────────────────────────────────────

func handleIntelLookup(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "domain parameter required", 400)
		return
	}
	intelEng := intel.New()
	ctx, _ := context.WithTimeout(r.Context(), 30*time.Second)
	result, err := intelEng.GetSignals(ctx, domain)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func handleIntelStatus(w http.ResponseWriter, r *http.Request) {
	intelEng := intel.New()
	ctx, _ := context.WithTimeout(r.Context(), 10*time.Second)
	writeJSON(w, intelEng.Status(ctx))
}

// ── AEO/GEO handlers ─────────────────────────────────────────────────

func handleAEOStatus(w http.ResponseWriter, r *http.Request) {
	aeoEng := aeo.New(nil)
	out := map[string]interface{}{
		"version":             aeoEng.Version(),
		"ok":                  aeoEng.Health(r.Context()) == nil,
		"brand":               aeoEng.Config.Brand,
		"domain":              aeoEng.Config.Domain,
		"prompts_tracked":     len(aeoEng.Config.Prompts),
		"competitors_tracked": len(aeoEng.Config.Competitors),
		"engines":             aeoEng.Config.Engines,
		"prompts":             aeoEng.Config.Prompts,
		"competitors":         aeoEng.Config.Competitors,
	}
	if aeoEng.Config.Brand == "" {
		out["hint"] = "Set XEME_AEO_BRAND, XEME_AEO_DOMAIN, XEME_AEO_PROMPTS (comma-sep), XEME_AEO_COMPETITORS"
	}
	writeJSON(w, out)
}

func handleAEOCheck(w http.ResponseWriter, r *http.Request) {
	aeoEng := aeo.New(nil)
	if aeoEng.Config.Brand == "" {
		http.Error(w, "XEME_AEO_BRAND not set", 400)
		return
	}
	ctx, _ := context.WithTimeout(r.Context(), 60*time.Second)
	results, err := aeoEng.CheckBrand(ctx)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":        true,
		"brand":     aeoEng.Config.Brand,
		"checked":   len(results),
		"results":   results,
		"citations": aeoEng.FindCitations(results),
		"gaps":      aeoEng.FindGaps(results),
	})
}

func handleAEOScore(w http.ResponseWriter, r *http.Request) {
	aeoEng := aeo.New(nil)
	if aeoEng.Config.Brand == "" {
		http.Error(w, "XEME_AEO_BRAND not set", 400)
		return
	}
	ctx, _ := context.WithTimeout(r.Context(), 90*time.Second)
	results, _ := aeoEng.CheckBrand(ctx)
	score := aeoEng.Score(results)
	writeJSON(w, score)
}

func handleAEOOptimize(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, "url parameter required", 400)
		return
	}
	aeoEng := aeo.New(nil)
	ctx, _ := context.WithTimeout(r.Context(), 20*time.Second)
	opt, err := aeoEng.Optimize(ctx, target)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, opt)
}

// ── Helpers ────────────────────────────────────────────────

var startTime = time.Now()

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func getWorkspace() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Projects", "xeme-os", "workspace")
	}
	return "/tmp"
}

func getLogsDir() string {
	return filepath.Join(getWorkspace(), "..", "logs")
}

func listFiles(dir, pattern string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	return matches
}

func loadRecentCSV(pattern string, limit int) ([]map[string]string, error) {
	workspace := getWorkspace()
	files, _ := filepath.Glob(filepath.Join(workspace, pattern))
	if len(files) == 0 {
		return []map[string]string{}, nil
	}
	// Most recent first
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if fi, _ := os.Stat(files[j]); fi != nil {
				if fi2, _ := os.Stat(files[i]); fi2 != nil && fi.ModTime().After(fi2.ModTime()) {
					files[i], files[j] = files[j], files[i]
				}
			}
		}
	}
	// Load most recent
	rows, err := readCSVFile(files[0])
	if err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows, nil
}

func readCSVFile(path string) ([]map[string]string, error) {
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
	var rows []map[string]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// suppress unused imports
var _ = bufio.NewReader
var _ = exec.Command
var _ = strconv.Itoa

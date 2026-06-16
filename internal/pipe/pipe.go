// Package pipe is the Xeme end-to-end engine. It orchestrates the three
// proprietary engines (Signal, Enrich, Ledger) with the ICP Scorer.
//
// Flow:
//   1. Acquire signals  (Xeme Signal Engine)
//   2. Filter to roles
//   3. Waterfall enrich  (Xeme Enrichment Engine)
//   4. Extract fields    (name, title, email, linkedin)
//   5. ICP score         (Xeme ICP Scorer)
//   6. Sync to ledger    (Xeme Ledger)
package pipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xeme-os/xeme/internal/csvkit"
	"github.com/xeme-os/xeme/internal/enrich"
	"github.com/xeme-os/xeme/internal/ledger"
	"github.com/xeme-os/xeme/internal/score"
	"github.com/xeme-os/xeme/internal/signal"
)

// Config is the runtime configuration for a pipe run.
type Config struct {
	WorkspaceDir string
	Stages       StagesConfig
	NoCRM        bool
	DryRun       bool
}

type StagesConfig struct {
	Enrich bool
	Score  bool
	Sync   bool
}

// Result is the outcome of a pipe run.
type Result struct {
	OK         bool                  `json:"ok"`
	StartedAt  string                `json:"started_at"`
	FinishedAt string                `json:"finished_at"`
	InputRows  int                   `json:"input_rows"`
	OutputRows int                   `json:"output_rows"`
	Enrich     *enrich.Result        `json:"enrich,omitempty"`
	Summary    score.Summary         `json:"summary"`
	CRMResult  *ledger.SyncResult    `json:"crm,omitempty"`
	FinalCSV   string                `json:"final_csv"`
	Stages     map[string]StageInfo  `json:"stages"`
	Error      string                `json:"error,omitempty"`
}

type StageInfo struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	Duration string `json:"duration_ms,omitempty"`
}

// Engine orchestrates a pipe run.
type Engine struct {
	Cfg    Config
	Enrich *enrich.Engine
	Signal *signal.ScrapePost
	Ledger *ledger.Ledger
	Scorer *score.Scorer
}

func New(cfg Config) *Engine {
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = "./workspace"
	}
	return &Engine{
		Cfg:    cfg,
		Enrich: enrich.New(),
		Signal: signal.New(),
		Ledger: ledger.New(),
		Scorer: score.New(),
	}
}

// Run is the main entry. inputPath must be a CSV with at least:
//   first_name, last_name, company_name, domain
// Optional: linkedin_url, title, signal_source
func (e *Engine) Run(inputPath string) (*Result, error) {
	if err := os.MkdirAll(e.Cfg.WorkspaceDir, 0o755); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	res := &Result{
		StartedAt: startedAt.Format(time.RFC3339),
		Stages:    make(map[string]StageInfo),
	}

	rowsIn, err := csvkit.Count(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	res.InputRows = rowsIn

	stamp := startedAt.Format("20060102_150405")
	enrichedPath := filepath.Join(e.Cfg.WorkspaceDir, "enriched_"+stamp+".csv")
	finalPath := filepath.Join(e.Cfg.WorkspaceDir, "final_"+stamp+".csv")

	// Stage 1: Enrichment
	if e.Cfg.Stages.Enrich {
		t := time.Now()
		enrichRes, err := e.Enrich.Waterfall(inputPath, enrichedPath)
		if err != nil {
			res.Error = err.Error()
			res.Stages["enrich"] = StageInfo{OK: false, Message: err.Error(), Duration: durMS(t)}
			return res, err
		}
		res.Enrich = enrichRes
		res.Stages["enrich"] = StageInfo{
			OK:       true,
			Message:  fmt.Sprintf("rows %d → %d, %.2f credits spent", enrichRes.RowsIn, enrichRes.RowsOut, enrichRes.CreditsSpent),
			Duration: durMS(t),
		}
	} else {
		enrichedPath = inputPath
	}

	// Stage 2: Extract + Score
	if e.Cfg.Stages.Score {
		t := time.Now()
		rows, err := csvkit.Read(enrichedPath)
		if err != nil {
			res.Error = err.Error()
			res.Stages["score"] = StageInfo{OK: false, Message: err.Error(), Duration: durMS(t)}
			return res, err
		}
		leads := e.rowsToLeads(rows)
		scored := e.Scorer.Batch(leads)
		res.Summary = score.Summarize(scored)
		res.Stages["score"] = StageInfo{
			OK:       true,
			Message:  fmt.Sprintf("T1:%d T2:%d T3:%d withEmails:%d", res.Summary.Tier1, res.Summary.Tier2, res.Summary.Tier3, res.Summary.WithEmails),
			Duration: durMS(t),
		}

		// Stage 3: CRM sync
		if e.Cfg.Stages.Sync && !e.Cfg.NoCRM {
			t := time.Now()
			contacts := e.leadsToContacts(scored)
			syncRes, err := e.Ledger.Sync(contacts, e.Cfg.DryRun)
			if err != nil {
				res.Stages["ledger"] = StageInfo{OK: false, Message: err.Error(), Duration: durMS(t)}
			} else {
				res.CRMResult = syncRes
				res.Stages["ledger"] = StageInfo{
					OK:       syncRes.OK,
					Message:  fmt.Sprintf("synced:%d errors:%d", syncRes.Synced, syncRes.Errors),
					Duration: durMS(t),
				}
			}
		}

		// Persist the final scored CSV
		flat := e.leadsToRows(scored)
		if err := csvkit.Write(finalPath, flat); err != nil {
			res.Stages["write"] = StageInfo{OK: false, Message: err.Error()}
		}
		res.FinalCSV = finalPath
		res.OutputRows = len(scored)
	}

	res.OK = true
	res.FinishedAt = time.Now().Format(time.RFC3339)
	return res, nil
}

// rowsToLeads converts CSV rows to Lead structs, extracting fields from the
// enrichment waterfall output.
func (e *Engine) rowsToLeads(rows []map[string]string) []score.Lead {
	leads := make([]score.Lead, 0, len(rows))
	for _, r := range rows {
		lead := score.Lead{
			FirstName: pick(r, "first_name"),
			LastName:  pick(r, "last_name"),
			Title:     pick(r, "title"),
			Company:   firstNonEmpty(pick(r, "company_name"), pick(r, "company")),
			Domain:    pick(r, "domain"),
			Email:     firstNonEmpty(pick(r, "extracted_email"), pick(r, "email")),
			LinkedIn:  firstNonEmpty(pick(r, "extracted_name"), pick(r, "linkedin_url")),
			Signal:    firstNonEmpty(pick(r, "signal_source"), pick(r, "signal")),
		}
		// If extracted_name is a JSON object with title, prefer that
		if nameJSON := pick(r, "extracted_name"); strings.HasPrefix(nameJSON, "{") {
			// Best-effort parse for title field
			lead.Title = firstNonEmpty(extractField(nameJSON, "title"), lead.Title)
			lead.LinkedIn = firstNonEmpty(extractField(nameJSON, "linkedin"), lead.LinkedIn)
		}
		leads = append(leads, lead)
	}
	return leads
}

func (e *Engine) leadsToContacts(leads []score.Lead) []ledger.PersonInput {
	out := make([]ledger.PersonInput, 0, len(leads))
	for _, l := range leads {
		out = append(out, ledger.PersonInput{
			FirstName: l.FirstName,
			LastName:  l.LastName,
			Email:     l.Email,
			JobTitle:  l.Title,
			LinkedIn:  l.LinkedIn,
		})
	}
	return out
}

func (e *Engine) leadsToRows(leads []score.Lead) []map[string]string {
	out := make([]map[string]string, 0, len(leads))
	for _, l := range leads {
		row := map[string]string{
			"first_name": l.FirstName,
			"last_name":  l.LastName,
			"title":      l.Title,
			"company":    l.Company,
			"domain":     l.Domain,
			"email":      l.Email,
			"linkedin":   l.LinkedIn,
			"signal":     l.Signal,
			"score":      fmt.Sprintf("%d", l.Score),
			"tier":       l.Tier,
		}
		out = append(out, row)
	}
	return out
}

func pick(m map[string]string, key string) string {
	return strings.TrimSpace(m[key])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func extractField(jsonStr, field string) string {
	// Naive extraction: "field":"value"
	needle := `"` + field + `":"`
	i := strings.Index(jsonStr, needle)
	if i < 0 {
		return ""
	}
	start := i + len(needle)
	end := strings.Index(jsonStr[start:], `"`)
	if end < 0 {
		return ""
	}
	return jsonStr[start : start+end]
}

func durMS(t time.Time) string {
	return fmt.Sprintf("%.0fms", float64(time.Since(t).Nanoseconds())/1e6)
}

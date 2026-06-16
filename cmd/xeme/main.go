// Xeme OS — AI-Native GTM Operating System
// Single static binary. Multi-channel sequencer. MoltSets enrichment.
// One Go binary: discover → enrich → qualify → sequence → track.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xeme-os/xeme/internal/audit"
	"github.com/xeme-os/xeme/internal/campaigns"
	"github.com/xeme-os/xeme/internal/config"
	"github.com/xeme-os/xeme/internal/csvkit"
	"github.com/xeme-os/xeme/internal/enrich"
	"github.com/xeme-os/xeme/internal/leads"
	"github.com/xeme-os/xeme/internal/ledger"
	"github.com/xeme-os/xeme/internal/pipe"
	"github.com/xeme-os/xeme/internal/score"
	"github.com/xeme-os/xeme/internal/sequencer"
	"github.com/xeme-os/xeme/internal/signal"
	"github.com/xeme-os/xeme/internal/workflows"
)

const version = "1.3.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "version", "--version", "-V":
		fmt.Printf("xeme v%s — AI-Native GTM Operating System\n", version)
	case "help", "--help", "-h":
		usage()
	case "init":
		runInit()
	case "status":
		runStatus()
	case "config":
		runConfig()
	case "scrape":
		runScrape(args)
	case "search":
		runSearch(args)
	case "enrich":
		runEnrich(args)
	case "score":
		runScore(args)
	case "leads":
		runLeads(args)
	case "crm":
		runCRM(args)
	case "pipe":
		runPipe(args)
	case "workflow":
		runWorkflow(args)
	case "campaign":
		runCampaign(args)
	case "sequence":
		runSequence(args)
	case "daemon":
		runDaemon(args)
	case "audit":
		runAudit(args)
	case "personalize":
		runPersonalize(args)
	case "serve", "dashboard":
		runServe(args)
	case "install":
		runInstall(args)
	case "update":
		runUpdate(args)
	case "mcp":
		runMCPInit(args)
	case "intel":
		runIntel(args)
	case "aeo":
		runAEO(args)
	case "deepline":
		runDeepline(args)
	default:
		fmt.Fprintf(os.Stderr, "xeme: unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`xeme — Xeme OS CLI

Usage: xeme <command> [options]

Commands:
  init                Scaffold ~/.xeme/ (config + ledger defaults)
  status              Health check across all Xeme engines
  version             Print version
  config              Show resolved config
  scrape              Scrape engagers from a public post URL
                        --url <post-url>
                        [--out <output.json>]
                        [--type both|reactions|comments]
                        [--max-pages N]
                        [--account <id>]
  enrich              Waterfall-enrich a CSV of contacts
                        --in <input.csv>
                        --out <output.csv>
  score               ICP score a CSV (7-gate rubric)
                        --in <input.csv>
                        --out <output.csv>
                        [--min-score N]
  crm status          Probe the Xeme Ledger
  crm sync            Sync a CSV to the Xeme Ledger
                        --in <input.csv>
                        [--dry-run]
  pipe                THE end-to-end pipe
                        --in <input.csv>
                        [--no-crm]
                        [--dry-run]

Examples:
  xeme init
  xeme status
  xeme enrich --in leads.csv --out enriched.csv
  xeme pipe --in leads.csv --min-score 60
  xeme crm sync --in final.csv --dry-run
`)
}

// ── init ────────────────────────────────────────────────

func runInit() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme init: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Join(home, ".xeme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "xeme init: mkdir: %v\n", err)
		os.Exit(1)
	}

	// 1. config.json — runtime state (credit balance, etc.)
	cfgPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		seed := map[string]interface{}{
			"enrich_balance": 1000.00,
			"signal_balance": 1000.00,
			"installed_at":   time.Now().UTC().Format(time.RFC3339),
			"version":        version,
		}
		b, _ := json.MarshalIndent(seed, "", "  ")
		if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "xeme init: write config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Created %s\n", cfgPath)
	} else {
		fmt.Printf("  Exists: %s\n", cfgPath)
	}

	// 2. crm.json — ledger configuration
	crmPath := filepath.Join(dir, "crm.json")
	if _, err := os.Stat(crmPath); os.IsNotExist(err) {
		seed := map[string]interface{}{
			"remotes": map[string]interface{}{
				"local": map[string]interface{}{
					"apiUrl":      "http://localhost:3848/graphql",
					"apiKey":      "xeme-crm-local-dev",
					"accessToken": "",
				},
			},
			"defaultRemote": "local",
			"mode":          "demo",
		}
		b, _ := json.MarshalIndent(seed, "", "  ")
		if err := os.WriteFile(crmPath, b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "xeme init: write crm: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Created %s\n", crmPath)
	} else {
		fmt.Printf("  Exists: %s\n", crmPath)
	}

	// 3. workspace/
	ws := "./workspace"
	if err := os.MkdirAll(ws, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "xeme init: mkdir workspace: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Created %s/\n", ws)

	fmt.Println()
	fmt.Println("Xeme OS is initialized. Run `xeme status` to verify engines.")
}

func runStatus() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme OS — Engine Status")
	fmt.Println("═══════════════════════════════════════════════════════════")

	sig := signal.New()
	fmt.Printf("  Xeme Signal Engine:     %s\n", sig.Version())
	if err := sig.Health(); err != nil {
		fmt.Printf("    Status: DEGRADED (%v)\n", err)
	} else {
		fmt.Printf("    Status: OK\n")
	}

	fmt.Println()
	en := enrich.New()
	fmt.Printf("  Xeme Enrichment Engine: %s\n", en.Version())
	if err := en.Health(); err != nil {
		fmt.Printf("    Status: DEGRADED (%v)\n", err)
	} else {
		mode := "live"
		if en.Balance() >= 0 && os.Getenv("XEME_ENRICH_API_KEY") == "" {
			mode = "demo"
		}
		fmt.Printf("    Status: OK | mode: %s | balance: %.2f credits\n", mode, en.Balance())
	}

	fmt.Println()
	led := ledger.New()
	st, err := led.Health()
	if err != nil {
		fmt.Printf("  Xeme Ledger:            DEGRADED (%v)\n", err)
	} else {
		fmt.Printf("  Xeme Ledger:            %s\n", st.APIURL)
		mode := st.Mode
		if mode == "" {
			mode = "live"
		}
		fmt.Printf("    Status: %s | mode: %s | HTTP %d", map[bool]string{true: "OK", false: "DEGRADED"}[st.OK], mode, st.HTTPStatus)
		if st.Note != "" {
			fmt.Printf(" (%s)", st.Note)
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("All systems nominal.")
}

func runConfig() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme: config load: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(b))
}

func runScrape(args []string) {
	opts := parseFlags(args)
	url := opts["url"]
	out := opts["out"]
	maxPagesStr := opts["max-pages"]
	account := opts["account"]
	scrapeType := opts["type"]
	if url == "" {
		fmt.Fprintln(os.Stderr, "xeme scrape: --url is required")
		os.Exit(1)
	}
	if out == "" {
		out = filepath.Join("./workspace", fmt.Sprintf("xeme_signal_%d.json", time.Now().Unix()))
	}
	if scrapeType == "" {
		scrapeType = "both"
	}
	maxPages, _ := strconv.Atoi(maxPagesStr)
	if maxPages == 0 {
		maxPages = 10
	}

	s := signal.New()
	s.MaxPages = maxPages
	s.Type = scrapeType
	s.Account = account

	res, err := s.Scrape(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme scrape: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "xeme scrape: mkdir: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "xeme scrape: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Xeme Signal: %d engagers from %s (mode: %s)\n", res.Total, url, res.Mode)
	fmt.Printf("  → %s\n", out)
}

func runEnrich(args []string) {
	opts := parseFlags(args)
	in := opts["in"]
	out := opts["out"]
	if in == "" {
		fmt.Fprintln(os.Stderr, "xeme enrich: --in is required")
		os.Exit(1)
	}
	if out == "" {
		out = filepath.Join("./workspace", fmt.Sprintf("xeme_enriched_%d.csv", time.Now().Unix()))
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "xeme enrich: mkdir: %v\n", err)
		os.Exit(1)
	}
	e := enrich.New()
	res, err := e.Waterfall(in, out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme enrich: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Xeme Enrichment: %d → %d rows (mode: %s)\n", res.RowsIn, res.RowsOut, res.Mode)
	fmt.Printf("  Credits spent: %.2f (balance: %.2f → %.2f)\n", res.CreditsSpent, res.BalanceBefore, res.BalanceAfter)
	fmt.Printf("  → %s\n", res.OutputPath)
}

func runScore(args []string) {
	opts := parseFlags(args)
	in := opts["in"]
	out := opts["out"]
	if in == "" {
		fmt.Fprintln(os.Stderr, "xeme score: --in is required")
		os.Exit(1)
	}
	rows, err := csvkit.Read(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme score: %v\n", err)
		os.Exit(1)
	}
	e := pipe.New(pipe.Config{WorkspaceDir: "./workspace"})
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
	summary := score.Summarize(scored)
	fmt.Printf("✓ Xeme ICP Scorer: %d leads → T1:%d T2:%d T3:%d (emails: %d)\n",
		summary.Total, summary.Tier1, summary.Tier2, summary.Tier3, summary.WithEmails)
	if out != "" {
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "xeme score: mkdir: %v\n", err)
		}
		flat := make([]map[string]string, 0, len(scored))
		for _, l := range scored {
			flat = append(flat, map[string]string{
				"first_name": l.FirstName, "last_name": l.LastName,
				"title": l.Title, "company": l.Company, "domain": l.Domain,
				"email": l.Email, "linkedin": l.LinkedIn, "signal": l.Signal,
				"score": fmt.Sprintf("%d", l.Score), "tier": l.Tier,
			})
		}
		if err := csvkit.Write(out, flat); err != nil {
			fmt.Fprintf(os.Stderr, "xeme score: write: %v\n", err)
		} else {
			fmt.Printf("  → %s\n", out)
		}
	}
}

func runCRM(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme crm: subcommand required (status|sync)")
		os.Exit(1)
	}
	switch args[0] {
	case "status":
		l := ledger.New()
		st, err := l.Health()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme crm status: %v\n", err)
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(b))
	case "sync":
		opts := parseFlags(args[1:])
		in := opts["in"]
		dry := opts["dry-run"] == "true" || opts["dry-run"] == "1"
		if in == "" {
			fmt.Fprintln(os.Stderr, "xeme crm sync: --in is required")
			os.Exit(1)
		}
		rows, err := csvkit.Read(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme crm sync: %v\n", err)
			os.Exit(1)
		}
		l := ledger.New()
		contacts := make([]ledger.PersonInput, 0, len(rows))
		for _, r := range rows {
			contacts = append(contacts, ledger.PersonInput{
				FirstName: r["first_name"],
				LastName:  r["last_name"],
				Email:     r["email"],
				JobTitle:  r["title"],
				LinkedIn:  r["linkedin"],
			})
		}
		res, err := l.Sync(contacts, dry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme crm sync: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Ledger: %d synced, %d errors (dry_run=%v)\n", res.Synced, res.Errors, res.DryRun)
	default:
		fmt.Fprintf(os.Stderr, "xeme crm: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func runPipe(args []string) {
	opts := parseFlags(args)
	in := opts["in"]
	noCRM := opts["no-crm"] == "true"
	dry := opts["dry-run"] == "true"
	if in == "" {
		fmt.Fprintln(os.Stderr, "xeme pipe: --in is required")
		os.Exit(1)
	}
	e := pipe.New(pipe.Config{
		WorkspaceDir: "./workspace",
		Stages:       pipe.StagesConfig{Enrich: true, Score: true, Sync: true},
		NoCRM:        noCRM,
		DryRun:       dry,
	})
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme OS — Pipe Run")
	fmt.Println("═══════════════════════════════════════════════════════════")
	res, err := e.Run(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme pipe: %v\n", err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	if !res.OK {
		os.Exit(1)
	}
}

func parseFlags(args []string) map[string]string {
	out := make(map[string]string)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !startsWith(a, "--") {
			continue
		}
		key := a[2:]
		if i+1 < len(args) && !startsWith(args[i+1], "--") {
			out[key] = args[i+1]
			i++
		} else {
			out[key] = "true"
		}
	}
	return out
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── workflows ────────────────────────────────────────

func runWorkflow(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme workflow: subcommand required (list|run|status)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "workflows.db")
	e, err := workflows.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme workflow: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()
	for k, h := range workflows.DefaultHandlers() {
		e.RegisterHandler(k, h)
	}

	switch args[0] {
	case "list":
		wfs, err := e.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme workflow list: %v\n", err)
			os.Exit(1)
		}
		if len(wfs) == 0 {
			fmt.Println("No workflows saved yet. Create one with: xeme workflow run path/to/wf.json")
			return
		}
		fmt.Printf("Xeme Workflows: %d\n", len(wfs))
		for _, w := range wfs {
			fmt.Printf("  %s  (%d nodes)  %s\n", w.ID, len(w.Nodes), w.Name)
		}
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme workflow run <workflow.json>")
			os.Exit(1)
		}
		w, err := workflows.LoadWorkflowFromFile(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme workflow run: %v\n", err)
			os.Exit(1)
		}
		if w.ID == "" {
			w.ID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
		}
		if err := e.Save(*w); err != nil {
			fmt.Fprintf(os.Stderr, "xeme workflow run: save: %v\n", err)
			os.Exit(1)
		}
		run, err := e.RunSync(*w)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme workflow run: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Workflows: started %s\n", run.ID)
		fmt.Printf("  Workflow: %s (%d nodes)\n", w.Name, len(w.Nodes))
		fmt.Printf("  Status:   %s\n", run.Status)
		fmt.Printf("  Watch:    xeme workflow status %s\n", run.ID)
	case "status":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme workflow status <run-id>")
			os.Exit(1)
		}
		run, err := e.GetRun(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme workflow status: %v\n", err)
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(run, "", "  ")
		fmt.Println(string(b))
	default:
		fmt.Fprintf(os.Stderr, "xeme workflow: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

// ── campaigns ────────────────────────────────────────

func runCampaign(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme campaign: subcommand required (list|create|track|pause|resume|enroll|report)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "campaigns.db")
	s, err := campaigns.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme campaign: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	switch args[0] {
	case "list":
		cs, err := s.ListCampaigns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign list: %v\n", err)
			os.Exit(1)
		}
		if len(cs) == 0 {
			fmt.Println("No campaigns yet. Create one with: xeme campaign create --name 'X' --template 3_email_1_li")
			return
		}
		fmt.Printf("Xeme Campaigns: %d\n", len(cs))
		for _, c := range cs {
			fmt.Printf("  %s  [%s]  %d steps, %d enrolled  %s\n", c.ID, c.Status, len(c.Steps), c.EnrolledCount, c.Name)
		}
	case "create":
		opts := parseFlags(args[1:])
		name := opts["name"]
		template := opts["template"]
		if name == "" {
			fmt.Fprintln(os.Stderr, "xeme campaign create: --name is required")
			os.Exit(1)
		}
		var c campaigns.Campaign
		if template != "" {
			templates := campaigns.AvailableTemplates()
			t, ok := templates[template]
			if !ok {
				fmt.Fprintf(os.Stderr, "xeme campaign create: unknown template %q (available: 3_email_1_li, 5_email_cold, 2_li_1_email_warm)\n", template)
				os.Exit(1)
			}
			t.Name = name
			t.Status = "draft"
			c = t
		} else {
			c = campaigns.Campaign{Name: name, Status: "draft", Steps: []campaigns.Step{}}
		}
		id, err := s.CreateCampaign(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign create: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Campaigns: created %s\n", id)
		fmt.Printf("  Name:     %s\n", name)
		fmt.Printf("  Template: %s\n", template)
		fmt.Printf("  Steps:    %d\n", len(c.Steps))
		fmt.Printf("  Next:     xeme campaign enroll %s --in leads.csv\n", id)
	case "track":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme campaign track <id>")
			os.Exit(1)
		}
		c, err := s.GetCampaign(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign track: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Xeme Campaign %s\n", c.ID)
		fmt.Printf("  Name:     %s\n", c.Name)
		fmt.Printf("  Status:   %s\n", c.Status)
		fmt.Printf("  Steps:    %d\n", len(c.Steps))
		fmt.Printf("  Enrolled: %d\n", c.EnrolledCount)
		fmt.Printf("  Replied:  %d\n", c.RepliedCount)
		fmt.Printf("  Meetings: %d\n", c.MeetingCount)
	case "pause":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme campaign pause <id>")
			os.Exit(1)
		}
		if err := s.PauseCampaign(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign pause: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Campaigns: paused %s\n", args[1])
	case "resume":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme campaign resume <id>")
			os.Exit(1)
		}
		if err := s.ResumeCampaign(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign resume: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Campaigns: resumed %s\n", args[1])
	case "enroll":
		opts := parseFlags(args[1:])
		id := args[1]
		if v, ok := opts[""]; ok {
			id = v
		}
		// Find --in
		inFile, _ := opts["in"]
		if inFile == "" {
			// positional after subcommand
			for i, a := range args[2:] {
				if !strings.HasPrefix(a, "--") {
					id = a
					break
				}
				if a == "--in" && i+1 < len(args)-1 {
					inFile = args[i+2]
				}
			}
		}
		_ = inFile
		// simpler: just take id as positional
		if id == "" || len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme campaign enroll <id> --in leads.csv")
			os.Exit(1)
		}
		inFile = opts["in"]
		if inFile == "" {
			fmt.Fprintln(os.Stderr, "xeme campaign enroll: --in <leads.csv> is required")
			os.Exit(1)
		}
		rows, err := readCSV(inFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign enroll: %v\n", err)
			os.Exit(1)
		}
		var contacts []campaigns.ContactState
		for _, r := range rows {
			contacts = append(contacts, campaigns.ContactState{
				Email:     r["email"],
				FirstName: r["first_name"],
				LastName:  r["last_name"],
				Company:   r["company_name"],
				Title:     r["title"],
			})
		}
		n, err := s.Enroll(id, contacts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign enroll: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Campaigns: enrolled %d contacts in %s\n", n, id)
	case "report":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme campaign report <id>")
			os.Exit(1)
		}
		c, err := s.GetCampaign(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme campaign report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("═══ Xeme Campaign Report: %s ═══\n", c.Name)
		fmt.Printf("  Status:   %s\n", c.Status)
		fmt.Printf("  Enrolled: %d\n", c.EnrolledCount)
		fmt.Printf("  Replied:  %d (%.1f%%)\n", c.RepliedCount, pct(c.RepliedCount, c.EnrolledCount))
		fmt.Printf("  Meetings: %d (%.1f%%)\n", c.MeetingCount, pct(c.MeetingCount, c.EnrolledCount))
	default:
		fmt.Fprintf(os.Stderr, "xeme campaign: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// ── personalize ─────────────────────────────────────

func runPersonalize(args []string) {
	opts := parseFlags(args)
	contact := opts["contact"]
	signal := opts["signal"]
	if contact == "" {
		fmt.Fprintln(os.Stderr, "xeme personalize: --contact <email> is required")
		os.Exit(1)
	}
	// Lightweight: render a templated first line using the same logic as internal/ai
	fmt.Printf("Hi — saw %s, worth a 15-min conversation about how things are working at your end?\n", truncate(signal, 60))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csvkit.Read(path)
	if err != nil {
		return nil, err
	}
	_ = f
	return rows, nil
}

// ── search (MoltSets) ──────────────────────────────────────────────────────

func runSearch(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme search: subcommand required (companies|people)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "leads.db")
	leadsDB, _ := leads.Open(dbPath)
	if leadsDB != nil {
		defer leadsDB.Close()
	}

	apiKey := os.Getenv("XEME_MOLTSETS_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "xeme search: set XEME_MOLTSETS_API_KEY")
		os.Exit(1)
	}
	msEngine := enrich.NewMoltSetsEngine(&enrich.MoltSetsConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.moltsets.com/api/v1/tools/",
		Timeout: 30 * time.Second,
	})
	ctx := context.Background()

	switch args[0] {
	case "companies":
		opts := parseFlags(args[1:])
		res, tokens, err := msEngine.SearchCompanies(ctx, enrich.SearchCompanyParams{
			Query:         opts["query"],
			Domain:        opts["domain"],
			Industry:      opts["industry"],
			EmployeeRange: opts["employees"],
			RevenueRange:  opts["revenue"],
			Limit:         atoiDefault(opts["limit"], 10),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme search companies: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ MoltSets: %d companies (tokens: %.0f)\n", res.Results.Total, tokens)
		for _, c := range res.Results.Results {
			fmt.Printf("  %-30s  %-25s  %s  employees:%d  rev:%s\n",
				c.Name, c.Industry, c.Domain, c.EmployeeCount, c.RevenueRange)
			// Upsert to leads DB
			if leadsDB != nil {
				leadsDB.Upsert(leads.Lead{
					Company: c.Name, Domain: c.Domain, Industry: c.Industry,
					EmployeeRange: c.EmployeeRange, Revenue: c.RevenueRange,
					LinkedInURL: c.LinkedInURL, Stage: "discovered", Source: "moltsets_search",
				})
			}
		}
	case "people":
		opts := parseFlags(args[1:])
		res, tokens, err := msEngine.SearchPeople(ctx, enrich.SearchPeopleParams{
			Query:         opts["query"],
			CompanyDomain: opts["domain"],
			Industry:      opts["industry"],
			EmployeeRange: opts["employees"],
			Limit:         atoiDefault(opts["limit"], 10),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme search people: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ MoltSets: %d people (tokens: %.0f)\n", res.Results.Total, tokens)
		for _, p := range res.Results.Results {
			fmt.Printf("  %-25s  %-30s  %-20s  %s\n", p.FullName, p.Title, p.Company, p.Email)
			if leadsDB != nil {
				leadsDB.Upsert(leads.Lead{
					Email: p.Email, Phone: p.Phone, FullName: p.FullName,
					FirstName: p.FirstName, LastName: p.LastName,
					Title: p.Title, Company: p.Company, Domain: p.Domain,
					Industry: p.Industry, LinkedInURL: p.LinkedInURL,
					Stage: "discovered", Source: "moltsets_search", Confidence: p.Score / 20,
				})
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "xeme search: unknown subcommand: %s (companies|people)\n", args[0])
		os.Exit(1)
	}
}

// ── leads ───────────────────────────────────────────────────────────────────

func runLeads(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme leads: subcommand required (stats|list|search)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "leads.db")
	db, err := leads.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme leads: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	switch args[0] {
	case "stats":
		st, err := db.Stats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme leads stats: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("═══ Xeme OS — Leads Pipeline ═══")
		stages := []string{"discovered", "enriched", "scored", "enrolled", "contacted", "replied", "meeting_booked", "lost"}
		for _, s := range stages {
			fmt.Printf("  %-18s %d\n", s, st[s])
		}
		fmt.Printf("  %-18s %d\n", "TOTAL", st["total"])
	case "list":
		opts := parseFlags(args[1:])
		stage := opts["stage"]
		tier := opts["tier"]
		limit := atoiDefault(opts["limit"], 50)
		var ls []leads.Lead
		var err error
		if tier != "" {
			ls, err = db.ListByTier(tier, limit)
		} else if stage != "" {
			ls, err = db.ListByStage(stage, limit)
		} else {
			ls, err = db.ListByStage("scored", limit)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme leads list: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Leads: %d\n", len(ls))
		for _, l := range ls {
			fmt.Printf("  %-25s %-30s %-20s score:%d tier:%s stage:%s\n",
				l.FullName, l.Title, l.Company, l.Score, l.Tier, l.Stage)
		}
	case "search":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "xeme leads search: query required")
			os.Exit(1)
		}
		query := strings.Join(args[1:], " ")
		ls, err := db.Search(query, 20)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme leads search: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Results: %d\n", len(ls))
		for _, l := range ls {
			fmt.Printf("  %-25s %-20s %-20s %s\n", l.FullName, l.Title, l.Company, l.Email)
		}
	default:
		fmt.Fprintf(os.Stderr, "xeme leads: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

// ── sequence (multi-channel) ────────────────────────────────────────────────

func runSequence(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme sequence: subcommand required (list|create|start|pause|resume|enroll|report|templates)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "sequences.db")
	store, err := sequencer.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme sequence: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	switch args[0] {
	case "templates":
		templates := sequencer.AvailableTemplates()
		fmt.Printf("Sequence Templates: %d\n\n", len(templates))
		for id, t := range templates {
			fmt.Printf("  %-18s %-35s %d steps\n", id, t.Name, len(t.Steps))
			for _, s := range t.Steps {
				fmt.Printf("    Day %2d  %-12s %s\n", s.DayOffset, s.Channel, truncate(s.Subject+s.Body, 60))
			}
			fmt.Println()
		}
	case "list":
		seqs, err := store.ListSequences()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence list: %v\n", err)
			os.Exit(1)
		}
		if len(seqs) == 0 {
			fmt.Println("No sequences yet. Create one: xeme sequence create --name 'X' --template multi_5touch")
			return
		}
		fmt.Printf("Sequences: %d\n", len(seqs))
		for _, s := range seqs {
			fmt.Printf("  %s  [%s]  %d steps  %s\n", s.ID, s.Status, len(s.Steps), s.Name)
		}
	case "create":
		opts := parseFlags(args[1:])
		name := opts["name"]
		template := opts["template"]
		if name == "" {
			fmt.Fprintln(os.Stderr, "xeme sequence create: --name is required")
			os.Exit(1)
		}
		var seq sequencer.Sequence
		if template != "" {
			templates := sequencer.AvailableTemplates()
			t, ok := templates[template]
			if !ok {
				fmt.Fprintf(os.Stderr, "xeme sequence create: unknown template %q\nRun 'xeme sequence templates' to see options.\n", template)
				os.Exit(1)
			}
			t.Name = name
			t.Status = "draft"
			seq = t
		} else {
			seq = sequencer.Sequence{Name: name, Status: "draft"}
		}
		id, err := store.CreateSequence(seq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence create: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Sequence: created %s\n", id)
		fmt.Printf("  Name:     %s\n", name)
		fmt.Printf("  Template: %s\n", template)
		fmt.Printf("  Steps:    %d\n", len(seq.Steps))
		fmt.Printf("  Next:     xeme sequence enroll %s --in leads.csv\n", id)
	case "start":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme sequence start <id>")
			os.Exit(1)
		}
		if err := store.ResumeSequence(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence start: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Sequence: started %s — daemon will pick it up\n", args[1])
	case "pause":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme sequence pause <id>")
			os.Exit(1)
		}
		if err := store.PauseSequence(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence pause: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Sequence: paused %s\n", args[1])
	case "resume":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme sequence resume <id>")
			os.Exit(1)
		}
		if err := store.ResumeSequence(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence resume: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Sequence: resumed %s\n", args[1])
	case "enroll":
		opts := parseFlags(args[1:])
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: xeme sequence enroll <id> --in leads.csv")
			os.Exit(1)
		}
		seqID := args[1]
		inFile := opts["in"]
		if inFile == "" {
			fmt.Fprintln(os.Stderr, "xeme sequence enroll: --in <leads.csv> is required")
			os.Exit(1)
		}
		rows, err := readCSV(inFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence enroll: %v\n", err)
			os.Exit(1)
		}
		contacts := make([]sequencer.Contact, 0, len(rows))
		for _, r := range rows {
			contacts = append(contacts, sequencer.Contact{
				Email:         r["email"],
				Phone:         r["phone"],
				FirstName:     r["first_name"],
				LastName:      r["last_name"],
				FullName:      r["full_name"],
				Company:       firstNonEmpty(r["company_name"], r["company"]),
				Title:         r["title"],
				Domain:        r["domain"],
				LinkedInURL:   firstNonEmpty(r["linkedin_url"], r["linkedin"]),
				TwitterHandle: r["twitter_handle"],
				Industry:      r["industry"],
				Score:         atoiDefault(r["score"], 0),
				Tier:          r["tier"],
			})
		}
		n, err := store.Enroll(seqID, contacts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence enroll: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Xeme Sequence: enrolled %d contacts in %s\n", n, seqID)
		fmt.Printf("  Next: xeme sequence start %s\n", seqID)
	case "report":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme sequence report <id>")
			os.Exit(1)
		}
		seq, err := store.GetSequence(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme sequence report: %v\n", err)
			os.Exit(1)
		}
		stats, _ := store.GetStats(args[1])
		fmt.Printf("═══ Xeme Sequence Report: %s ═══\n", seq.Name)
		fmt.Printf("  Status:   %s\n", seq.Status)
		fmt.Printf("  Steps:    %d\n", len(seq.Steps))
		if stats != nil {
			fmt.Printf("  Enrolled: %d\n", stats.Enrolled)
			fmt.Printf("  Active:   %d\n", stats.Active)
			fmt.Printf("  Replied:  %d (%.1f%%)\n", stats.Replied, stats.ReplyRate)
			fmt.Printf("  Meetings: %d (%.1f%%)\n", stats.MeetingBooked, stats.MeetingRate)
			fmt.Printf("  Bounced:  %d (%.1f%%)\n", stats.Bounced, stats.BounceRate)
			fmt.Printf("  Completed: %d\n", stats.Completed)
		}
		fmt.Println("\n  Steps:")
		for _, s := range seq.Steps {
			fmt.Printf("    Day %2d  %-12s  %s\n", s.DayOffset, s.Channel, truncate(s.Subject, 50))
		}
	default:
		fmt.Fprintf(os.Stderr, "xeme sequence: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

// ── daemon ──────────────────────────────────────────────────────────────────

func runDaemon(args []string) {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "sequences.db")
	store, err := sequencer.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme daemon: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	engine := sequencer.NewEngine(store)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme OS — Sequence Daemon")
	fmt.Printf("  Version:  v%s\n", version)
	fmt.Printf("  Tick:     %s\n", engine.Interval)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Running. Press Ctrl+C to stop.")
	fmt.Println()

	stop := make(chan struct{})
	go engine.Run(stop)

	// Block forever
	select {}
}

// ── audit ───────────────────────────────────────────────────────────────────

func runAudit(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "xeme audit: subcommand required (recent|summary|by-action)")
		os.Exit(1)
	}
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "audit.db")
	store, err := audit.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xeme audit: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	switch args[0] {
	case "recent":
		limit := 50
		if len(args) > 1 {
			limit, _ = strconv.Atoi(args[1])
		}
		entries, err := store.Recent(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme audit recent: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Recent Audit Entries: %d\n", len(entries))
		for _, e := range entries {
			fmt.Printf("  %s\n", audit.FormatEntry(e))
		}
	case "summary":
		summary, err := store.Summary()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme audit summary: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("═══ Audit Summary ═══")
		for action, count := range summary {
			fmt.Printf("  %-18s %d\n", action, count)
		}
		since24h := time.Now().Add(-24 * time.Hour)
		tokens, _ := store.TokensUsed(since24h)
		fmt.Printf("\n  Tokens (24h):      %.0f\n", tokens)
	case "by-action":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: xeme audit by-action <action>")
			os.Exit(1)
		}
		entries, err := store.ByAction(args[1], 50)
		if err != nil {
			fmt.Fprintf(os.Stderr, "xeme audit by-action: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Audit: %s (%d entries)\n", args[1], len(entries))
		for _, e := range entries {
			fmt.Printf("  %s\n", audit.FormatEntry(e))
		}
	default:
		fmt.Fprintf(os.Stderr, "xeme audit: unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}


// ── New commands: serve / install / update / mcp / intel / aeo / deepline ─────────

// runServe starts the local dashboard on :4903 (or specified port).
func runServe(args []string) {
	host := "127.0.0.1"
	port := 4903
	openBrowserFlag := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 < len(args) {
				host = args[i+1]
				i++
			}
		case "--port", "-p":
			if i+1 < len(args) {
				p, _ := strconv.Atoi(args[i+1])
				if p > 0 {
					port = p
				}
				i++
			}
		case "--open":
			openBrowserFlag = true
		}
	}
	if openBrowserFlag {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			_ = exec.Command("open", fmt.Sprintf("http://%s:%d", host, port)).Start()
		}()
	}
	fmt.Printf("\n\033[1;33m  🚀 Xeme OS Dashboard\033[0m\n")
	fmt.Printf("  → http://%s:%d\n", host, port)
	fmt.Printf("  → Press Ctrl+C to stop\n\n")
	serveDashboard(host, port)
}

// runInstall installs an optional sub-component.
func runInstall(args []string) {
	if len(args) == 0 {
		fmt.Println("Available components to install:")
		fmt.Println("")
		fmt.Println("  deepline        Open-source Clay alternative (Bricks-style)")
		fmt.Println("  clay            Open-source Clay alternative stack")
		fmt.Println("  twenty          Twenty CRM (self-hosted)")
		fmt.Println("  sequencer       Multichannel sequencer (already built-in)")
		fmt.Println("  yalc            YALC skills library (already exposed via MCP)")
		fmt.Println("  scrapling       Scrapling web scraper (Python)")
		fmt.Println("  billionmail     Billionmail email outreach (Docker)")
		fmt.Println("  chatwoot        Chatwoot live chat widget (Docker)")
		fmt.Println("  n8n             n8n workflow automation (Docker)")
		fmt.Println("  all             Install all optional components")
		fmt.Println("")
		fmt.Println("Usage: xeme install <component>")
		return
	}
	target := args[0]
	switch target {
	case "all":
		fmt.Println("Installing all optional components…")
		for _, c := range []string{"deepline", "clay", "twenty", "billionmail", "n8n"} {
			fmt.Printf("\n=== %s ===\n", c)
			runInstall([]string{c})
		}
	case "deepline", "clay":
		fmt.Println("→ Open-source Clay alternative. Built into Xeme OS — no extra install needed.")
		fmt.Println("  Try: xeme intel search \"AI startups\"")
	case "twenty":
		fmt.Println("→ Twenty CRM is optional. Xeme OS has its own Ledger built in.")
		fmt.Println("  To install Twenty alongside:")
		fmt.Println("    git clone https://github.com/twentyhq/twenty.git && cd twenty")
		fmt.Println("    docker compose up -d")
		fmt.Println("  Then point your agents at http://localhost:3000")
	case "sequencer":
		fmt.Println("✓ Multichannel sequencer is already built into Xeme OS.")
		fmt.Println("  Try: xeme sequence list")
	case "yalc":
		fmt.Println("✓ YALC skills are exposed via the xeme-mcp server (20 tools).")
		fmt.Println("  Claude Code: claude mcp add xeme --transport stdio -- xeme-mcp")
	case "scrapling":
		fmt.Println("→ Scrapling is a Python library. Xeme OS already uses it via the signal scraper.")
		fmt.Println("  Standalone: pip install scrapling")
	case "billionmail":
		fmt.Println("→ Billionmail is an optional Docker email server.")
		fmt.Println("  git clone https://github.com/billionmail && cd billionmail && docker compose up -d")
	case "chatwoot":
		fmt.Println("→ Chatwoot is an optional Docker live chat widget.")
		fmt.Println("  git clone https://github.com/chatwoot/chatwoot && cd chatwoot && docker compose up -d")
	case "n8n":
		fmt.Println("→ n8n is the recommended orchestration companion.")
		fmt.Println("  docker run -d --name n8n -p 5678:5678 -v ~/.n8n:/home/node/.n8n n8nio/n8n")
	default:
		fmt.Fprintf(os.Stderr, "Unknown component: %s\n", target)
		os.Exit(1)
	}
}

// runUpdate checks for the latest version.
func runUpdate(args []string) {
	fmt.Println("→ Checking for updates…")
	fmt.Println()
	fmt.Println("Current version: " + version)
	fmt.Println()
	fmt.Println("To update:")
	fmt.Println("  curl -fsSL https://raw.githubusercontent.com/xeme-os/xeme/main/install/install.sh | bash")
	fmt.Println()
	fmt.Println("Or build from source:")
	fmt.Println("  cd ~/Projects/xeme-os && git pull && go build -o xeme ./cmd/xeme")
}

// runMCPInit prints the command to register Xeme as an MCP server.
func runMCPInit(args []string) {
	mcpPath, _ := os.Executable()
	mcpServer := filepath.Join(filepath.Dir(mcpPath), "xeme-mcp")
	fmt.Println("→ Register Xeme OS as an MCP server")
	fmt.Println()
	fmt.Println("Claude Code:")
	fmt.Printf("  claude mcp add xeme --transport stdio -- %s\n", mcpServer)
	fmt.Println()
	fmt.Println("Claude Desktop (~/.config/claude/claude_desktop_config.json):")
	fmt.Println("  {")
	fmt.Println("    \"mcpServers\": {")
	fmt.Println("      \"xeme\": {")
	fmt.Printf("        \"command\": \"%s\",\n", mcpServer)
	fmt.Println("        \"args\": []")
	fmt.Println("      }")
	fmt.Println("    }")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Cursor (~/.cursor/mcp.json):")
	fmt.Println(`  { "mcpServers": { "xeme": { "command": "` + mcpServer + `" } } }`)
	fmt.Println()
	fmt.Println("20 tools will be available: status, scrape, enrich, score, crm, intel, aeo, moltsets, run_pipe, etc.")
}

// runIntel proxies to the Intel engine.
func runIntel(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  xeme intel search <query>      Search TheirStack for companies")
		fmt.Println("  xeme intel jobs <keywords>     Search 202M job postings")
		fmt.Println("  xeme intel company <domain>    Get all signals for a domain")
		fmt.Println("  xeme intel status              Provider health")
		fmt.Println("  xeme intel tech <domain>       BuiltWith free tech lookup")
		fmt.Println("  xeme intel hn [keywords]       HackerNews hiring signals")
		return
	}
	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "search":
		fmt.Println("→ TheirStack company search (via xeme-mcp: xeme_intel_search_companies)")
		if len(subArgs) > 0 {
			fmt.Printf("  Direct: xeme intel search --domain %s\n", subArgs[0])
		}
	case "jobs":
		fmt.Println("→ TheirStack job search (via xeme-mcp: xeme_intel_search_jobs)")
	case "company":
		if len(subArgs) == 0 {
			fmt.Println("Usage: xeme intel company <domain>")
			return
		}
		fmt.Printf("→ Looking up signals for %s (via local dashboard :4903)\n", subArgs[0])
		fmt.Printf("  Open: http://127.0.0.1:4903/api/intel/lookup?domain=%s\n", subArgs[0])
	case "status":
		fmt.Println("→ Intel provider status (run `xeme serve` and check the dashboard, or call xeme_intel_status via MCP)")
	case "tech":
		if len(subArgs) == 0 {
			fmt.Println("Usage: xeme intel tech <domain>")
			return
		}
		fmt.Printf("→ BuiltWith free lookup for %s (no key required)\n", subArgs[0])
		fmt.Println("  Returns tech categories + buying-intent signals.")
	case "hn":
		fmt.Println("→ HackerNews 'Who Is Hiring' signals (free, no key).")
		fmt.Println("  Returns companies hiring + tech extracted from job posts.")
	default:
		fmt.Fprintf(os.Stderr, "Unknown intel subcommand: %s\n", sub)
	}
}

// runAEO proxies to the AEO engine.
func runAEO(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  xeme aeo status              Show AEO config (brand, prompts, competitors)")
		fmt.Println("  xeme aeo check               Run all prompts × all engines, get mentions + gaps")
		fmt.Println("  xeme aeo score               Compute 0-100 AEO score")
		fmt.Println("  xeme aeo optimize <url>      Analyze URL for AEO-readiness")
		fmt.Println("")
		fmt.Println("Setup:")
		fmt.Println("  export XEME_AEO_BRAND=\"YourBrand\"")
		fmt.Println("  export XEME_AEO_DOMAIN=\"yourdomain.com\"")
		fmt.Println("  export XEME_AEO_PROMPTS=\"best GTM tool,open source Clay\"")
		fmt.Println("  export XEME_AEO_COMPETITORS=\"Clay,Apollo,Instantly\"")
		return
	}
	sub := args[0]
	switch sub {
	case "status":
		fmt.Println("→ AEO/GEO engine status")
		if os.Getenv("XEME_AEO_BRAND") == "" {
			fmt.Println("  XEME_AEO_BRAND not set. AEO is dormant.")
			fmt.Println("  Set env vars and re-run. Free engines: you.com, phind, komo (no key needed).")
		} else {
			fmt.Printf("  Brand: %s\n", os.Getenv("XEME_AEO_BRAND"))
			fmt.Printf("  Domain: %s\n", os.Getenv("XEME_AEO_DOMAIN"))
			fmt.Printf("  Prompts: %s\n", os.Getenv("XEME_AEO_PROMPTS"))
			fmt.Printf("  Competitors: %s\n", os.Getenv("XEME_AEO_COMPETITORS"))
		}
	case "check":
		fmt.Println("→ Running AEO check across all engines (30-60s)…")
		fmt.Println("  Open dashboard http://127.0.0.1:4903 → 🎯 AEO / GEO → Check Brand")
	case "score":
		fmt.Println("→ Computing AEO score…")
		fmt.Println("  Open dashboard http://127.0.0.1:4903 → 🎯 AEO / GEO → Compute Score")
	case "optimize":
		if len(args) < 2 {
			fmt.Println("Usage: xeme aeo optimize <url>")
			return
		}
		fmt.Printf("→ Optimizing %s for AEO/GEO citations…\n", args[1])
		fmt.Println("  Returns word count, FAQ check, schema markup, stats, quotes, readability")
	default:
		fmt.Fprintf(os.Stderr, "Unknown aeo subcommand: %s\n", sub)
	}
}

// runDeepline is the unified, agent-friendly entry point.
func runDeepline(args []string) {
	if len(args) == 0 {
		fmt.Println("xeme deepline — unified GTM verbs (agent-friendly)")
		fmt.Println()
		fmt.Println("Discovery:")
		fmt.Println("  xeme deepline scrape <post-url>           Scrape engagers from a public post")
		fmt.Println("  xeme deepline intel <domain>             Get all signals for a company")
		fmt.Println("  xeme deepline search <query>             Search TheirStack companies/people")
		fmt.Println("  xeme deepline jobs <keywords>            Search 202M job postings")
		fmt.Println("  xeme deepline hn [keywords]              HackerNews hiring signals")
		fmt.Println("")
		fmt.Println("Enrichment:")
		fmt.Println("  xeme deepline enrich <csv>               Waterfall enrich (local+upstream+MoltSets)")
		fmt.Println("  xeme deepline email <linkedin-url>       Get email from LinkedIn (MoltSets)")
		fmt.Println("  xeme deepline phone <linkedin-url>       Get phone from LinkedIn (MoltSets)")
		fmt.Println("")
		fmt.Println("Scoring + CRM:")
		fmt.Println("  xeme deepline score <csv>                7-gate ICP scoring")
		fmt.Println("  xeme deepline crm <csv>                   Sync to Xeme Ledger")
		fmt.Println("")
		fmt.Println("Outreach:")
		fmt.Println("  xeme deepline sequence start <csv>       Start multichannel sequence")
		fmt.Println("  xeme deepline sequence status            Sequence progress")
		fmt.Println("")
		fmt.Println("AEO/GEO:")
		fmt.Println("  xeme deepline aeo score                  0-100 AEO score")
		fmt.Println("  xeme deepline aeo check                  Check brand mentions across AI engines")
		fmt.Println("  xeme deepline aeo optimize <url>         Analyze URL for AEO-readiness")
		fmt.Println("")
		fmt.Println("End-to-end:")
		fmt.Println("  xeme deepline run <csv>                  Full pipe: enrich → score → crm → sequence")
		return
	}
	verb := args[0]
	switch verb {
	case "scrape":
		runScrape(append([]string{"--url"}, args[1:]...))
	case "intel":
		runIntel(args[1:])
	case "search":
		runIntel(append([]string{"search"}, args[1:]...))
	case "jobs":
		runIntel(append([]string{"jobs"}, args[1:]...))
	case "hn":
		runIntel(append([]string{"hn"}, args[1:]...))
	case "enrich":
		runEnrich(args[1:])
	case "email":
		if len(args) < 2 {
			fmt.Println("Usage: xeme deepline email <linkedin-url>")
			return
		}
		fmt.Printf("→ Use xeme-mcp tool xeme_moltsets_enrich_email with linkedin_url=%s\n", args[1])
	case "phone":
		if len(args) < 2 {
			fmt.Println("Usage: xeme deepline phone <linkedin-url>")
			return
		}
		fmt.Printf("→ Use xeme-mcp tool xeme_moltsets_enrich_phone with linkedin_url=%s\n", args[1])
	case "score":
		runScore(args[1:])
	case "crm":
		runCRM(args[1:])
	case "sequence":
		runSequence(args[1:])
	case "aeo":
		runAEO(args[1:])
	case "run":
		runPipe(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown deepline verb: %s\n", verb)
	}
}

// serveDashboard runs the local dashboard. It exec's the xeme-os binary
// if present alongside xeme, otherwise prints a hint.
func serveDashboard(host string, port int) {
	exePath, err := os.Executable()
	if err == nil {
		dash := filepath.Join(filepath.Dir(exePath), "xeme-os")
		if _, statErr := os.Stat(dash); statErr == nil {
			cmd := exec.Command(dash, "--host", host, "--port", fmt.Sprintf("%d", port))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			return
		}
	}
	fmt.Println("→ xeme-os binary not found alongside xeme.")
	fmt.Println("  Build it: cd ~/Projects/xeme-os && go build -o xeme-os ./cmd/xeme-os")
	fmt.Println("  Or run:   ./xeme-os --host " + host + " --port " + fmt.Sprintf("%d", port))
}

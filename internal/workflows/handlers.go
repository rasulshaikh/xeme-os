// Built-in node handlers for the Xeme Workflows engine.
// These wrap the existing Xeme engines: signal, enrich, ledger, ai, intel.
package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultHandlers returns a map of node type → handler for the built-in engines.
func DefaultHandlers() map[string]NodeHandler {
	return map[string]NodeHandler{
		"signal.scrape": handleSignalScrape,
		"enrich.waterfall": handleEnrichWaterfall,
		"score.run": handleScoreRun,
		"ledger.sync": handleLedgerSync,
		"ai.personalize": handleAIPersonalize,
		"intel.boost": handleIntelBoost,
		"shell": handleShell,
	}
}

func handleSignalScrape(ctx context.Context, n Node) (map[string]interface{}, error) {
	url, _ := n.Params["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("signal.scrape: url is required")
	}
	out, err := exec.CommandContext(ctx, "yalc-gtm",
		"leads:scrape-post",
		"--url", url,
		"--type", "both",
		"--max-pages", "5",
		"--output", "/tmp/xeme-wf-signal.json",
	).CombinedOutput()
	if err != nil {
		return map[string]interface{}{"error": err.Error(), "stderr": string(out)}, err
	}
	return map[string]interface{}{"ok": true, "output": "/tmp/xeme-wf-signal.json"}, nil
}

func handleEnrichWaterfall(ctx context.Context, n Node) (map[string]interface{}, error) {
	input, _ := n.Params["input"].(string)
	output, _ := n.Params["output"].(string)
	if input == "" {
		return nil, fmt.Errorf("enrich.waterfall: input is required")
	}
	if output == "" {
		output = "/tmp/xeme-wf-enriched.csv"
	}
	cfg, _ := n.Params["config"].(string)
	if cfg == "" {
		cfg = "config/enrich.jsonc"
	}
	err := exec.CommandContext(ctx, "deepline", "enrich",
		"--input", input, "--output", output, "--config", cfg,
	).Run()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, err
	}
	return map[string]interface{}{"ok": true, "output": output}, nil
}

func handleScoreRun(ctx context.Context, n Node) (map[string]interface{}, error) {
	input, _ := n.Params["input"].(string)
	output, _ := n.Params["output"].(string)
	if input == "" {
		return nil, fmt.Errorf("score.run: input is required")
	}
	if output == "" {
		output = "/tmp/xeme-wf-scored.csv"
	}
	// Score is a pure-Go op — we could call the xeme binary or run inline.
	// Inline is faster and avoids process overhead. Use the score package.
	err := exec.CommandContext(ctx, "xeme", "score", "--in", input, "--out", output).Run()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, err
	}
	return map[string]interface{}{"ok": true, "output": output}, nil
}

func handleLedgerSync(ctx context.Context, n Node) (map[string]interface{}, error) {
	input, _ := n.Params["input"].(string)
	if input == "" {
		return nil, fmt.Errorf("ledger.sync: input is required")
	}
	dryRun := false
	if v, ok := n.Params["dry_run"].(bool); ok {
		dryRun = v
	}
	args := []string{"crm", "sync", "--in", input}
	if dryRun {
		args = append(args, "--dry-run")
	}
	err := exec.CommandContext(ctx, "xeme", args...).Run()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, err
	}
	return map[string]interface{}{"ok": true, "input": input}, nil
}

func handleAIPersonalize(ctx context.Context, n Node) (map[string]interface{}, error) {
	// Lightweight shell-out — the AI engine is a sub-call
	contact, _ := n.Params["contact"].(string)
	signal, _ := n.Params["signal"].(string)
	if contact == "" {
		return nil, fmt.Errorf("ai.personalize: contact is required")
	}
	cmd := exec.CommandContext(ctx, "xeme", "personalize",
		"--contact", contact, "--signal", signal)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, err
	}
	return map[string]interface{}{"ok": true, "output": strings.TrimSpace(string(out))}, nil
}

func handleIntelBoost(ctx context.Context, n Node) (map[string]interface{}, error) {
	// Returns current intel score boosts per signal type
	// (placeholder — would read from internal/intel store)
	return map[string]interface{}{"ok": true, "boots": map[string]int{
		"job-change":  15,
		"g2-review":   10,
		"engaged":     5,
		"follow":      0,
	}}, nil
}

func handleShell(ctx context.Context, n Node) (map[string]interface{}, error) {
	cmdStr, _ := n.Params["command"].(string)
	if cmdStr == "" {
		return nil, fmt.Errorf("shell: command is required")
	}
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("shell: empty command")
	}
	out, err := exec.CommandContext(ctx, parts[0], parts[1:]...).CombinedOutput()
	if err != nil {
		return map[string]interface{}{"stdout": string(out), "error": err.Error()}, err
	}
	return map[string]interface{}{"ok": true, "stdout": strings.TrimSpace(string(out))}, nil
}

// Helper for tests — keep json imports used
var _ = json.Marshal

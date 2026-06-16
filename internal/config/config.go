// Package config loads Xeme OS configuration from config/xeme.yaml.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config is the resolved Xeme OS configuration.
type Config struct {
	Tools      Tools      `yaml:"tools"`
	Enrich     Enrich     `yaml:"enrich"`
	Signal     Signal     `yaml:"signal"`
	Ledger     Ledger     `yaml:"ledger"`
	ICP        ICP        `yaml:"icp"`
	Output     Output     `yaml:"output"`
}

// Tools holds the internal Xeme engine binaries.
type Tools struct {
	SignalEngine string `yaml:"signal"`
	EnrichEngine string `yaml:"enrich"`
	LedgerURL    string `yaml:"ledger"`
}

type Enrich struct {
	DefaultRoles  []string `yaml:"default_roles"`
	ContactLimit  int      `yaml:"contact_limit"`
	ConfigPath    string   `yaml:"config_path"`
}

type Signal struct {
	ScrapeMaxPages int    `yaml:"scrape_max_pages"`
	ScrapeType     string `yaml:"scrape_type"`
	Account        string `yaml:"account"`
}

type Ledger struct {
	ConfigPath   string `yaml:"config_path"`
	DefaultRemote string `yaml:"default_remote"`
	BatchSize    int    `yaml:"batch_size"`
}

type ICP struct {
	TargetTitles []string         `yaml:"target_titles"`
	Scoring      ScoringConfig    `yaml:"scoring"`
}

type ScoringConfig struct {
	TitleCMO        int `yaml:"title_cmo"`
	TitleVPMarketing int `yaml:"title_vp_marketing"`
	TitleOther      int `yaml:"title_other"`
	SignalEvaluating int `yaml:"signal_evaluating"`
	SignalCommenting int `yaml:"signal_commenting"`
	SignalFollowing  int `yaml:"signal_following"`
	EmailVerified    int `yaml:"email_verified"`
	Tier1Threshold   int `yaml:"tier1_threshold"`
	Tier2Threshold   int `yaml:"tier2_threshold"`
}

type Output struct {
	WorkspaceDir string `yaml:"workspace_dir"`
	LogsDir      string `yaml:"logs_dir"`
	CSVPrefix    string `yaml:"csv_prefix"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Tools: Tools{
		SignalEngine: "xeme-signal",    // Xeme Signal Engine — proprietary binary
		EnrichEngine: "xeme-enrich",    // Xeme Enrichment Engine — proprietary binary
		LedgerURL:    "",                // Xeme Ledger — config-driven
		},
		Enrich: Enrich{
			DefaultRoles: []string{
				"Chief Marketing Officer",
				"VP of Marketing",
				"VP Marketing",
				"VP Demand Generation",
				"SVP Marketing",
				"Head of Marketing",
			},
			ContactLimit: 1,
			ConfigPath:   "config/enrich.jsonc",
		},
		Signal: Signal{
			ScrapeMaxPages: 10,
			ScrapeType:     "both",
		},
		Ledger: Ledger{
			ConfigPath:    expandHome("~/.xeme/crm.json"),
			DefaultRemote: "local",
			BatchSize:     50,
		},
		ICP: ICP{
			TargetTitles: []string{
				"Chief Marketing Officer",
				"VP of Marketing",
				"VP Demand Generation",
				"SVP Marketing",
				"Head of Marketing",
			},
			Scoring: ScoringConfig{
				TitleCMO:         50,
				TitleVPMarketing: 40,
				TitleOther:       20,
				SignalEvaluating: 25,
				SignalCommenting: 20,
				SignalFollowing:  15,
				EmailVerified:    5,
				Tier1Threshold:   70,
				Tier2Threshold:   50,
			},
		},
		Output: Output{
			WorkspaceDir: "./workspace",
			LogsDir:      "./logs",
			CSVPrefix:    "xeme",
		},
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

// Load reads config/xeme.yaml and returns a Config merged with defaults.
func Load(path string) (*Config, error) {
	cfg := Defaults()
	if path == "" {
		path = "config/xeme.yaml"
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	// Minimal YAML parser for flat keys with 1-2 levels of nesting
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	parseYAML(data, cfg)
	return cfg, nil
}

// parseYAML is a minimal YAML parser. Sufficient for our flat config shape.
func parseYAML(data []byte, cfg *Config) {
	lines := splitLines(string(data))
	for _, line := range lines {
		line = trim(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Top-level keys: tools, enrich, signal, ledger, icp, output
		colon := strings.Index(line, ":")
		if colon == -1 {
			continue
		}
		key := trim(line[:colon])
		val := trim(line[colon+1:])
		switch key {
		case "workspace_dir":
			cfg.Output.WorkspaceDir = unquote(val)
		case "logs_dir":
			cfg.Output.LogsDir = unquote(val)
		case "csv_prefix":
			cfg.Output.CSVPrefix = unquote(val)
		}
	}
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func unquote(s string) string {
	s = trim(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

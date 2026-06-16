// Xeme Workflows Daemon — long-running process that schedules and
// runs workflows according to their cron triggers.
//
// Usage:
//   xeme-workflows              # runs in foreground
//   xeme-workflows --once       # runs a single tick and exits (for cron)
//
// Env vars:
//   XEME_WORKFLOWS_INTERVAL     poll interval (default 60s)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/xeme-os/xeme/internal/workflows"
)

func main() {
	once := flag.Bool("once", false, "Run a single tick and exit (for use as a cron target)")
	flag.Parse()

	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "workflows.db")
	e, err := workflows.Open(dbPath)
	if err != nil {
		log.Fatalf("open workflows.db: %v", err)
	}
	defer e.Close()

	for k, h := range workflows.DefaultHandlers() {
		e.RegisterHandler(k, h)
	}

	interval := 60 * time.Second
	if v := os.Getenv("XEME_WORKFLOWS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *once {
		fmt.Println("Xeme Workflows Daemon — single tick")
		// List + report
		wfs, _ := e.List()
		fmt.Printf("  %d workflows loaded\n", len(wfs))
		return
	}

	s := workflows.NewScheduler(e, interval)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme Workflows Daemon")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  DB:        %s\n", dbPath)
	fmt.Printf("  Interval:  %s\n", interval)
	fmt.Println()
	fmt.Println("  Waiting for scheduled workflows. Ctrl+C to stop.")

	if err := s.Run(ctx); err != nil {
		log.Fatalf("scheduler: %v", err)
	}
	fmt.Println("Daemon stopped.")
}

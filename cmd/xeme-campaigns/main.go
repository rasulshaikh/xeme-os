// Xeme Campaigns Daemon — long-running process that ticks every
// XEME_CAMPAIGNS_TICK_INTERVAL (default 60s), finds due contacts in
// active campaigns, and executes their next step (email via SMTP,
// task creation, wait advancement).
//
// Usage:
//   xeme-campaigns              # foreground, ticks forever
//   xeme-campaigns --once       # single tick and exit (for cron)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/xeme-os/xeme/internal/campaigns"
	"github.com/xeme-os/xeme/internal/outreach"
)

func main() {
	once := flag.Bool("once", false, "Single tick and exit (for cron use)")
	flag.Parse()

	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "campaigns.db")
	store, err := campaigns.Open(dbPath)
	if err != nil {
		log.Fatalf("open campaigns.db: %v", err)
	}
	defer store.Close()

	// SMTP config from env
	sender := outreach.NewSender(outreach.Config{
		Host:     os.Getenv("OUTREACH_SMTP_HOST"),
		Port:     getEnvInt("OUTREACH_SMTP_PORT", 587),
		Username: os.Getenv("OUTREACH_SMTP_USER"),
		Password: os.Getenv("OUTREACH_SMTP_PASS"),
		From:     os.Getenv("OUTREACH_FROM"),
		UseTLS:   os.Getenv("OUTREACH_SMTP_TLS") == "true",
	})

	sms := &outreach.Twilio{
		AccountSID: os.Getenv("XEME_TWILIO_SID"),
		AuthToken:  os.Getenv("XEME_TWILIO_AUTH_TOKEN"),
		FromNumber: os.Getenv("XEME_TWILIO_FROM"),
	}

	ex := campaigns.NewExecutor(store)
	ex.Sender = sender
	ex.SMS = sms

	if v := os.Getenv("XEME_CAMPAIGNS_TICK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ex.Interval = d
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *once {
		fmt.Println("Xeme Campaigns Daemon — single tick")
		if err := ex.Tick(ctx); err != nil {
			log.Fatalf("tick: %v", err)
		}
		// Report
		cs, _ := store.ListCampaigns()
		active := 0
		for _, c := range cs {
			if c.Status == "active" {
				active++
			}
		}
		fmt.Printf("  Campaigns: %d total, %d active\n", len(cs), active)
		return
	}

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme Campaigns Daemon")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  DB:        %s\n", dbPath)
	fmt.Printf("  Tick:      %s\n", ex.Interval)
	if sender.Health() == nil {
		fmt.Println("  SMTP:      configured (real send enabled)")
	} else if campaigns.UseDevCapture() {
		fmt.Println("  SMTP:      dev-capture mode (writes to ~/.xeme/outbox/*.eml)")
	} else {
		fmt.Println("  SMTP:      not configured (outreach events will be 'queued')")
	}
	if sms.AccountSID != "" {
		fmt.Println("  Twilio:    configured")
	}
	fmt.Println()
	fmt.Println("  Ticking. Ctrl+C to stop.")

	if err := ex.Run(ctx); err != nil {
		log.Fatalf("executor: %v", err)
	}
	fmt.Println("Daemon stopped.")
}

func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}


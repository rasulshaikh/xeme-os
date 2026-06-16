// Xeme Workflows scheduler — runs scheduled workflows in the background.
package workflows

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CronExpr is a minimal cron-like parser. Supports only "minute hour * * *"
// for the demo (e.g. "0 6 * * *" = every day at 6am UTC). For production
// use a real cron library.
type CronExpr struct {
	Minute int
	Hour   int
}

// ParseCron parses a simple "M H * * *" expression.
func ParseCron(expr string) (CronExpr, error) {
	var c CronExpr
	var wildcard string
	n, err := fmt.Sscanf(expr, "%d %d %s", &c.Minute, &c.Hour, &wildcard)
	if err != nil || n < 2 {
		return c, fmt.Errorf("invalid cron expression: %q (expected 'M H * * *')", expr)
	}
	if c.Minute < 0 || c.Minute > 59 || c.Hour < 0 || c.Hour > 23 {
		return c, fmt.Errorf("cron field out of range: %q", expr)
	}
	return c, nil
}

// NextRun returns the next time after `t` that matches the cron expression.
func (c CronExpr) NextRun(t time.Time) time.Time {
	next := t.Add(time.Minute).Truncate(time.Minute)
	for {
		if next.Hour() == c.Hour && next.Minute() == c.Minute {
			return next
		}
		next = next.Add(time.Minute)
		if next.Sub(t) > 7*24*time.Hour {
			return t.Add(24 * time.Hour) // safety
		}
	}
}

// Scheduler runs scheduled workflows in the background.
type Scheduler struct {
	Engine    *Engine
	Interval  time.Duration
	Workflows []Workflow
}

// NewScheduler creates a scheduler with the given poll interval.
func NewScheduler(e *Engine, interval time.Duration) *Scheduler {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &Scheduler{Engine: e, Interval: interval}
}

// Run starts the scheduler loop. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	// Initial load
	if err := s.refresh(); err != nil {
		log.Printf("scheduler: initial load: %v", err)
	}
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := s.refresh(); err != nil {
				log.Printf("scheduler: refresh: %v", err)
			}
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) refresh() error {
	wfs, err := s.Engine.List()
	if err != nil {
		return err
	}
	s.Workflows = wfs
	return nil
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	for _, w := range s.Workflows {
		// Workflows with no trigger aren't auto-scheduled
		if w.ID == "" {
			continue
		}
		// Find a metadata field with cron — for now we read it from a synthetic field
		// "trigger" stored at workflow creation. (Not yet in schema, so skip silently.)
		_ = w
		_ = now
	}
}

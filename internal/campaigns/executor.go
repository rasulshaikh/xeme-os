// Campaign executor — advances contacts through step sequences.
package campaigns

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/xeme-os/xeme/internal/outreach"
)

// Executor advances campaigns by executing due steps.
type Executor struct {
	Store    *Store
	Sender   outreach.Sender
	SMS      *outreach.Twilio
	Interval time.Duration
}

// NewExecutor creates an executor.
func NewExecutor(s *Store) *Executor {
	return &Executor{Store: s, Interval: 60 * time.Second}
}

// Tick advances all due contacts in all active campaigns by one step each.
func (e *Executor) Tick(ctx context.Context) error {
	campaigns, err := e.Store.ListCampaigns()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, c := range campaigns {
		if c.Status != "active" {
			continue
		}
		contacts, err := e.Store.DueContacts(c.ID, now, 100)
		if err != nil {
			log.Printf("campaign %s: due contacts: %v", c.ID, err)
			continue
		}
		for _, contact := range contacts {
			if err := e.advanceOneStep(ctx, c, contact); err != nil {
				log.Printf("campaign %s: contact %s: %v", c.ID, contact.Email, err)
			}
		}
	}
	return nil
}

// advanceOneStep executes the next step for a single contact.
func (e *Executor) advanceOneStep(ctx context.Context, c Campaign, contact ContactState) error {
	if contact.StepIndex >= len(c.Steps) {
		// Campaign complete for this contact
		_ = e.Store.UpdateContactState(c.ID, contact.Email, "completed", contact.StepIndex, nil)
		_ = e.Store.LogEvent(c.ID, contact.Email, "", "", "completed", "")
		return nil
	}
	step := c.Steps[contact.StepIndex]

	switch step.Channel {
	case "email":
		// Render and send (dev-capture fallback if no SMTP)
		body := Render(step.Body, contact)
		subject := Render(step.Subject, contact)
		var sender outreach.Sender = e.Sender
		if sender == nil || sender.Health() != nil {
			if UseDevCapture() {
				sender = outreach.NewDevCaptureSender()
			}
		}
		if sender != nil {
			res, err := sender.Send(outreach.Email{
				To:         contact.Email,
				Subject:    subject,
				Body:       body,
				IsHTML:     false,
				CampaignID: c.ID,
				StepID:     step.ID,
			})
			if err != nil {
				_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "email", "failed", err.Error())
				return err
			}
			eventType := "sent"
			if e.Sender == nil && UseDevCapture() {
				eventType = "captured"
			}
			if res.OK {
				_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "email", eventType, res.ProviderMessageID)
			}
		} else {
			_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "email", "queued", "no SMTP configured")
		}
	case "sms":
		if e.SMS != nil {
			body := Render(step.Body, contact)
			res, _ := e.SMS.Send(outreach.SMS{To: contact.Email, Body: body})
			if res.OK {
				_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "sms", "sent", res.SID)
			}
		}
	case "task":
		// Just log the task for manual completion
		_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "task", "created", step.Body)
	case "wait":
		// No-op — the next_action is already set; just record the wait elapsed
		_ = e.Store.LogEvent(c.ID, contact.Email, step.ID, "wait", "elapsed", "")
	}

	// Advance to next step
	nextIdx := contact.StepIndex + 1
	if nextIdx >= len(c.Steps) {
		_ = e.Store.UpdateContactState(c.ID, contact.Email, "completed", nextIdx, nil)
		return nil
	}
	nextStep := c.Steps[nextIdx]
	// Default 1-day step gap; for "wait" steps use the configured wait_hours
	hoursUntil := 24
	if nextStep.Channel == "wait" && nextStep.WaitHours > 0 {
		hoursUntil = nextStep.WaitHours
	}
	next := time.Now().Add(time.Duration(hoursUntil) * time.Hour)
	_ = e.Store.UpdateContactState(c.ID, contact.Email, "pending", nextIdx, &next)
	return nil
}

// RecordOutcome updates a contact's state based on an external event
// (e.g. "replied" from the reply-detection system).
func (e *Executor) RecordOutcome(campaignID, email, event string) error {
	state := "stopped"
	switch event {
	case "replied", "meeting_booked":
		state = "stopped"
	case "bounced":
		state = "bounced"
	case "unsubscribed":
		state = "unsubscribed"
	default:
		return fmt.Errorf("unknown event: %s", event)
	}
	_ = e.Store.LogEvent(campaignID, email, "", "", event, "")
	return e.Store.UpdateContactState(campaignID, email, state, 0, nil)
}

// Run starts the executor loop. Blocks until ctx is cancelled.
func (e *Executor) Run(ctx context.Context) error {
	t := time.NewTicker(e.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := e.Tick(ctx); err != nil {
				log.Printf("campaign executor: tick: %v", err)
			}
		}
	}
}

// Render substitutes {{var}} placeholders. Used by the executor.
func Render(tpl string, contact ContactState) string {
	// Use the outreach package's renderer
	return outreach.Render(tpl, map[string]string{
		"first_name": contact.FirstName,
		"last_name":  contact.LastName,
		"full_name":  contact.FirstName + " " + contact.LastName,
		"email":      contact.Email,
		"company":    contact.Company,
		"title":      contact.Title,
	})
}

// UseDevCapture returns whether to use the capture-mode sender (no SMTP).
// Set OUTREACH_DEV_CAPTURE=true to force capture, or leave SMTP_HOST unset.
func UseDevCapture() bool {
	if v := osGetenv("OUTREACH_DEV_CAPTURE"); v == "true" {
		return true
	}
	return osGetenv("OUTREACH_SMTP_HOST") == ""
}

func osGetenv(k string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
		return v
	}
	return ""
}

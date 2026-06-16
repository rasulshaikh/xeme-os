// Twilio (SMS) provider — stub that no-ops without API credentials.
// Replace with real Twilio client when XEME_TWILIO_SID is set.
package outreach

import "fmt"

type Twilio struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

type SMS struct {
	To      string
	Body    string
}

type SMSResult struct {
	OK      bool
	SID     string
	Error   string
}

func (t *Twilio) Send(s SMS) (*SMSResult, error) {
	if t.AccountSID == "" || t.AuthToken == "" {
		return &SMSResult{OK: false, Error: "twilio not configured (XEME_TWILIO_SID / XEME_TWILIO_AUTH_TOKEN)"}, nil
	}
	// Real implementation would POST to https://api.twilio.com/2010-04-01/Accounts/{SID}/Messages.json
	return &SMSResult{OK: true, SID: fmt.Sprintf("SM-%d", 1)}, nil
}

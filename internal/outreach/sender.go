// Sender is the interface implemented by both real SMTP senders and
// the dev-capture sender. The campaigns executor dispatches to any Sender.
package outreach

// Sender sends a single email and returns a result.
type Sender interface {
	Send(e Email) (*SendResult, error)
	Health() error
}

// Compile-time interface checks
var (
	_ Sender = (*SmtpSender)(nil)
	_ Sender = (*DevCaptureSender)(nil)
)

// Email template rendering with {{variable}} substitution.
package outreach

import "strings"

// Render applies a template (with {{var}} placeholders) to a context.
func Render(tpl string, ctx map[string]string) string {
	for k, v := range ctx {
		tpl = strings.ReplaceAll(tpl, "{{"+k+"}}", v)
	}
	return tpl
}

// DefaultContext builds a template context from a contact + company.
func DefaultContext(firstName, lastName, email, company, signal, senderName string) map[string]string {
	return map[string]string{
		"first_name":   firstName,
		"last_name":    lastName,
		"full_name":    strings.TrimSpace(firstName + " " + lastName),
		"email":        email,
		"company":      company,
		"signal":       signal,
		"sender_name":  senderName,
		"topic":        defaultIfEmpty(signal, "your work at "+company),
	}
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

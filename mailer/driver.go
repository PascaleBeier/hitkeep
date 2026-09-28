package mailer

// Driver represents the underlying transport mechanism (SMTP, Vendor, etc.)
type Driver interface {
	// Send transmits the constructed message.
	Send(to []string, subject string, htmlBody string, textBody string) error
	// Close cleans up connections if necessary (e.g., SMTP pool).
	Close() error
}

// HeaderDriver is implemented by transports that can preserve message
// identity and standards-based delivery headers across retries.
type HeaderDriver interface {
	SendWithHeaders(to []string, subject string, htmlBody string, textBody string, messageID string, headers map[string]string) error
}

type SendOptions struct {
	MessageID string
	Headers   map[string]string
}

package mailer

import "hitkeep/mailer/drivers"

// Message is one rendered email ready for transport, including its stable
// identity headers and inline images.
type Message = drivers.Message

// InlineImage is an image embedded in a message and referenced as cid:<CID>.
type InlineImage = drivers.InlineImage

// Driver represents the underlying transport mechanism (SMTP, Vendor, etc.)
type Driver interface {
	// Send transmits the message, preserving its headers and inline images.
	Send(message Message) error
	// Close cleans up connections if necessary (e.g., SMTP pool).
	Close() error
}

type SendOptions struct {
	MessageID string
	Headers   map[string]string
}

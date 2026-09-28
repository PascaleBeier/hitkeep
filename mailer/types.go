package mailer

// Mailable represents a specific email notification.
type Mailable interface {
	// Subject returns the email subject line.
	Subject() string

	// Template returns the filename of the specific MJML template (e.g. "reset.mjml").
	Template() string

	// Data returns the struct or map to be injected into the template.
	Data() any
}

// LocalizedMailable optionally exposes the recipient locale for template rendering.
type LocalizedMailable interface {
	Mailable
	Locale() string
}

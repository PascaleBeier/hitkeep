//go:build billing

package mailables

import "hitkeep/internal/mailer"

type EmailVerification struct {
	LocaleCode string
	Link       string
	TeamName   string
}

func NewEmailVerification(link, teamName, locale string) mailer.Mailable {
	return &EmailVerification{Link: link, TeamName: teamName, LocaleCode: locale}
}

func (m *EmailVerification) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.email_verification")
}

func (m *EmailVerification) Template() string {
	return "email_verification.mjml"
}

func (m *EmailVerification) Data() any {
	return struct {
		Link     string
		TeamName string
	}{
		Link:     m.Link,
		TeamName: m.TeamName,
	}
}

func (m *EmailVerification) Locale() string { return m.LocaleCode }

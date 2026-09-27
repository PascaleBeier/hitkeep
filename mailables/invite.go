package mailables

import "hitkeep/internal/mailer"

// UserInvite implements the mailer.Mailable interface
type UserInvite struct {
	LocaleCode string
	Link       string
	SiteName   string
	Inviter    string
}

func NewUserInvite(link, siteName, inviter, locale string) mailer.Mailable {
	return &UserInvite{
		LocaleCode: locale,
		Link:       link,
		SiteName:   siteName,
		Inviter:    inviter,
	}
}

func (m *UserInvite) Subject() string {
	return mailer.Translatef(m.LocaleCode, "subject.user_invite", m.SiteName)
}

func (m *UserInvite) Template() string {
	return "user_invite.mjml"
}

func (m *UserInvite) Data() any {
	return struct {
		Link     string
		SiteName string
		Inviter  string
	}{
		Link:     m.Link,
		SiteName: m.SiteName,
		Inviter:  m.Inviter,
	}
}

func (m *UserInvite) Locale() string { return m.LocaleCode }

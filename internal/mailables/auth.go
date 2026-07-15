package mailables

import "hitkeep/internal/mailer"

// PasswordReset implements the mailer.Mailable interface
type PasswordReset struct {
	LocaleCode string
	Link       string
}

func NewPasswordReset(link, locale string) mailer.Mailable {
	return &PasswordReset{Link: link, LocaleCode: locale}
}

func (m *PasswordReset) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.password_reset")
}

func (m *PasswordReset) Template() string {
	return "password_reset.mjml"
}

func (m *PasswordReset) Data() any {
	return struct {
		Link string
	}{
		Link: m.Link,
	}
}

func (m *PasswordReset) Locale() string { return m.LocaleCode }

type MFAMagicLink struct {
	LocaleCode       string
	Link             string
	ExpiresInMinutes int
}

func NewMFAMagicLink(link, locale string, expiresInMinutes int) mailer.Mailable {
	return &MFAMagicLink{Link: link, LocaleCode: locale, ExpiresInMinutes: expiresInMinutes}
}

func (m *MFAMagicLink) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.magic_link")
}

func (m *MFAMagicLink) Template() string {
	return "mfa_magic_link.mjml"
}

func (m *MFAMagicLink) Data() any {
	return struct {
		Link             string
		ExpiresInMinutes int
	}{
		Link:             m.Link,
		ExpiresInMinutes: m.ExpiresInMinutes,
	}
}

func (m *MFAMagicLink) Locale() string { return m.LocaleCode }

type SocialConfirmation struct {
	LocaleCode       string
	Link             string
	Provider         string
	ExpiresInMinutes int
}

func NewSocialConfirmation(link, provider, locale string, expiresInMinutes int) mailer.Mailable {
	return &SocialConfirmation{
		Link:             link,
		Provider:         provider,
		LocaleCode:       locale,
		ExpiresInMinutes: expiresInMinutes,
	}
}

func (m *SocialConfirmation) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.social_confirmation")
}

func (m *SocialConfirmation) Template() string {
	return "social_confirmation.mjml"
}

func (m *SocialConfirmation) Data() any {
	return struct {
		Link             string
		Provider         string
		ExpiresInMinutes int
	}{
		Link:             m.Link,
		Provider:         m.Provider,
		ExpiresInMinutes: m.ExpiresInMinutes,
	}
}

func (m *SocialConfirmation) Locale() string { return m.LocaleCode }

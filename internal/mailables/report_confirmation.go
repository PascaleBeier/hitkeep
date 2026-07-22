package mailables

import (
	"strings"

	"hitkeep/internal/api"
	"hitkeep/internal/mailer"
)

type ReportRecipientConfirmation struct {
	LocaleCode    string
	Link          string
	TeamName      string
	ReportName    string
	Frequency     string
	SiteDomains   string
	ExpiresInDays int
}

func NewReportRecipientConfirmation(link, locale string, metadata api.ReportRecipientConfirmation) mailer.Mailable {
	domains := make([]string, 0, len(metadata.Sites))
	for _, site := range metadata.Sites {
		domains = append(domains, site.Domain)
	}
	return &ReportRecipientConfirmation{
		LocaleCode:    locale,
		Link:          link,
		TeamName:      metadata.TeamName,
		ReportName:    metadata.ReportName,
		Frequency:     mailer.Translate(locale, "freq."+string(metadata.Schedule.Frequency)),
		SiteDomains:   strings.Join(domains, ", "),
		ExpiresInDays: 7,
	}
}

func (m *ReportRecipientConfirmation) Subject() string {
	return mailer.Translatef(m.LocaleCode, "subject.report_confirmation", m.ReportName, m.TeamName)
}

func (m *ReportRecipientConfirmation) Template() string { return "report_confirmation.mjml" }

func (m *ReportRecipientConfirmation) Data() any { return m }

func (m *ReportRecipientConfirmation) Locale() string { return m.LocaleCode }

//go:build billing

package mailables

import "hitkeep/internal/mailer"

type CloudLifecycleLinks struct {
	DashboardURL string
	UpgradeURL   string
	FundingURL   string
	DocsURL      string
	WordPressURL string
	FeedbackURL  string
}

type CloudWelcome struct {
	LocaleCode    string
	TeamName      string
	SiteDomain    string
	IsFreePlan    bool
	RetentionDays int
	Links         CloudLifecycleLinks
}

func NewCloudWelcome(locale, teamName, siteDomain string, isFreePlan bool, retentionDays int, links CloudLifecycleLinks) mailer.Mailable {
	return &CloudWelcome{
		LocaleCode:    locale,
		TeamName:      teamName,
		SiteDomain:    siteDomain,
		IsFreePlan:    isFreePlan,
		RetentionDays: retentionDays,
		Links:         links,
	}
}

func (m *CloudWelcome) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.cloud_welcome")
}

func (m *CloudWelcome) Template() string {
	return "cloud_welcome.mjml"
}

func (m *CloudWelcome) Data() any {
	return struct {
		TeamName      string
		SiteDomain    string
		IsFreePlan    bool
		RetentionDays int
		DashboardURL  string
		UpgradeURL    string
		FundingURL    string
		DocsURL       string
		WordPressURL  string
		FeedbackURL   string
	}{
		TeamName:      m.TeamName,
		SiteDomain:    m.SiteDomain,
		IsFreePlan:    m.IsFreePlan,
		RetentionDays: m.RetentionDays,
		DashboardURL:  m.Links.DashboardURL,
		UpgradeURL:    m.Links.UpgradeURL,
		FundingURL:    m.Links.FundingURL,
		DocsURL:       m.Links.DocsURL,
		WordPressURL:  m.Links.WordPressURL,
		FeedbackURL:   m.Links.FeedbackURL,
	}
}

func (m *CloudWelcome) Locale() string { return m.LocaleCode }

type CloudFreeRetentionReminder struct {
	LocaleCode    string
	TeamName      string
	SiteDomain    string
	RetentionDays int
	Links         CloudLifecycleLinks
}

func NewCloudFreeRetentionReminder(locale, teamName, siteDomain string, retentionDays int, links CloudLifecycleLinks) mailer.Mailable {
	return &CloudFreeRetentionReminder{
		LocaleCode:    locale,
		TeamName:      teamName,
		SiteDomain:    siteDomain,
		RetentionDays: retentionDays,
		Links:         links,
	}
}

func (m *CloudFreeRetentionReminder) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.cloud_free_retention_reminder")
}

func (m *CloudFreeRetentionReminder) Template() string {
	return "cloud_free_retention_reminder.mjml"
}

func (m *CloudFreeRetentionReminder) Data() any {
	return struct {
		TeamName      string
		SiteDomain    string
		RetentionDays int
		DashboardURL  string
		UpgradeURL    string
		FundingURL    string
		DocsURL       string
		WordPressURL  string
		FeedbackURL   string
	}{
		TeamName:      m.TeamName,
		SiteDomain:    m.SiteDomain,
		RetentionDays: m.RetentionDays,
		DashboardURL:  m.Links.DashboardURL,
		UpgradeURL:    m.Links.UpgradeURL,
		FundingURL:    m.Links.FundingURL,
		DocsURL:       m.Links.DocsURL,
		WordPressURL:  m.Links.WordPressURL,
		FeedbackURL:   m.Links.FeedbackURL,
	}
}

func (m *CloudFreeRetentionReminder) Locale() string { return m.LocaleCode }

type CloudFreeLimitReminder struct {
	LocaleCode  string
	TeamName    string
	SiteLimit   int
	MemberLimit int
	Links       CloudLifecycleLinks
}

func NewCloudFreeLimitReminder(locale, teamName string, siteLimit, memberLimit int, links CloudLifecycleLinks) mailer.Mailable {
	return &CloudFreeLimitReminder{
		LocaleCode:  locale,
		TeamName:    teamName,
		SiteLimit:   siteLimit,
		MemberLimit: memberLimit,
		Links:       links,
	}
}

func (m *CloudFreeLimitReminder) Subject() string {
	return mailer.Translate(m.LocaleCode, "subject.cloud_free_limit_reminder")
}

func (m *CloudFreeLimitReminder) Template() string {
	return "cloud_free_limit_reminder.mjml"
}

func (m *CloudFreeLimitReminder) Data() any {
	return struct {
		TeamName    string
		SiteLimit   int
		MemberLimit int
		UpgradeURL  string
		FundingURL  string
	}{
		TeamName:    m.TeamName,
		SiteLimit:   m.SiteLimit,
		MemberLimit: m.MemberLimit,
		UpgradeURL:  m.Links.UpgradeURL,
		FundingURL:  m.Links.FundingURL,
	}
}

func (m *CloudFreeLimitReminder) Locale() string { return m.LocaleCode }

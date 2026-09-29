// Package mailpreview holds the deterministic email fixture catalog shared by
// the render gate and the developer preview tooling.
package mailpreview

import (
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/mailables"
	"hitkeep/mailer"
	opportunitysvc "hitkeep/opportunities"
)

// Fixture is one named email variant rendered in every supported locale.
type Fixture struct {
	ID    string
	Build func(locale string) mailer.Mailable
}

// Now is the fixed clock used for every fixture render.
var Now = time.Date(2026, time.September, 29, 8, 0, 0, 0, time.UTC)

const appURL = "https://app.hitkeep.example"

var fixtures = []Fixture{
	{ID: "site_report_weekly", Build: func(locale string) mailer.Mailable {
		return siteReport(locale, "weekly", "shop.example", fullStats(), previousStats(), []int{5200, 6100, 7400, 8100, 7900, 6200, 7313})
	}},
	{ID: "site_report_empty", Build: func(locale string) mailer.Mailable {
		return siteReport(locale, "daily", "new.example", mailables.ReportStats{}, mailables.ReportStats{}, nil)
	}},
	{ID: "site_report_decline_long", Build: func(locale string) mailer.Mailable {
		current := fullStats()
		current.Pageviews, current.Visitors = 9120, 3021
		current.TopPages = metrics("/blog/a-very-long-article-slug-about-privacy-first-analytics-without-cookies-and-consent-banners", 4210, "/", 2040)
		return siteReport(locale, "monthly", "docs.a-rather-long-subdomain.example.com", current, previousStats(), []int{400, 310, 290, 350, 280, 120, 90})
	}},
	{ID: "analytics_digest_weekly", Build: func(locale string) mailer.Mailable {
		return digest(locale, []mailables.DigestSiteEntry{
			{Domain: "shop.example", DashURL: appURL + "/dashboard", Pageviews: 48213, PrevPageviews: 41002, Visitors: 12877, PrevVisitors: 13310, Goals: 400, PrevGoals: 380},
			{Domain: "blog.example.org", DashURL: appURL + "/dashboard", Pageviews: 912, PrevPageviews: 1500, Visitors: 610, PrevVisitors: 900},
			{Domain: "docs.a-rather-long-subdomain.example.com", DashURL: appURL + "/dashboard"},
		})
	}},
	{ID: "analytics_digest_single", Build: func(locale string) mailer.Mailable {
		return digest(locale, []mailables.DigestSiteEntry{
			{Domain: "shop.example", DashURL: appURL + "/dashboard", Pageviews: 48213, PrevPageviews: 41002, Visitors: 12877, PrevVisitors: 13310},
		})
	}},
	{ID: "opportunity_digest_weekly", Build: func(locale string) mailer.Mailable {
		start, end := weekBounds()
		return mailables.NewOpportunityDigestWithSubjectLabel(locale, "shop.example",
			mailables.FormatPeriodLabelForLocale(locale, start, end),
			mailables.LocalizedDigestFrequencyLabel(locale, "weekly"),
			mailables.LocalizedDigestSubjectFrequencyLabel(locale, "weekly"),
			appURL+"/opportunities", appURL+"/settings", opportunityPreview())
	}},
	{ID: "report_confirmation", Build: func(locale string) mailer.Mailable {
		return mailables.NewReportRecipientConfirmation(appURL+"/reports/confirm?token=preview", locale, api.ReportRecipientConfirmation{
			ReportName: "Weekly executive summary",
			TeamName:   "Acme Analytics",
			Preset:     api.ReportPresetSiteSummary,
			Schedule:   api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "Europe/Berlin", LocalTime: "08:00", WeeklyDay: new(1)},
			Sites:      []api.ReportSite{{ID: fixedUUID(1), Domain: "shop.example"}, {ID: fixedUUID(2), Domain: "blog.example.org"}},
			ExpiresAt:  Now.Add(7 * 24 * time.Hour),
		})
	}},
	{ID: "password_reset", Build: func(locale string) mailer.Mailable {
		return mailables.NewPasswordReset(appURL+"/reset-password?token=preview", locale)
	}},
	{ID: "mfa_magic_link", Build: func(locale string) mailer.Mailable {
		return mailables.NewMFAMagicLink(appURL+"/mfa/magic?token=preview", locale, 15)
	}},
	{ID: "social_confirmation", Build: func(locale string) mailer.Mailable {
		return mailables.NewSocialConfirmation(appURL+"/signup/social/confirm?token=preview", "GitHub", locale, 30)
	}},
	{ID: "user_invite", Build: func(locale string) mailer.Mailable {
		return mailables.NewUserInvite(appURL+"/accept-invite?token=preview", "shop.example", "Jane Doe", locale)
	}},
	{ID: "team_invite_new_user", Build: func(locale string) mailer.Mailable {
		return mailables.NewTeamInvite(appURL+"/accept-invite?token=preview", "Acme Analytics", "Jane Doe", "member", true, locale)
	}},
	{ID: "team_invite_existing_user", Build: func(locale string) mailer.Mailable {
		return mailables.NewTeamInvite(appURL+"/dashboard", "Acme Analytics", "Jane Doe", "admin", false, locale)
	}},
}

// Catalog returns every fixture available in the current build.
func Catalog() []Fixture {
	return fixtures
}

func siteReport(locale, freq, domain string, current, previous mailables.ReportStats, daily []int) mailer.Mailable {
	start, end := weekBounds()
	return mailables.NewSiteAnalyticsReport(locale, domain,
		mailables.FormatPeriodLabelForLocale(locale, start, end),
		mailables.LocalizedReportFrequencyLabel(locale, freq),
		appURL+"/dashboard", appURL+"/settings", current, previous, daily)
}

func digest(locale string, sites []mailables.DigestSiteEntry) mailer.Mailable {
	start, end := weekBounds()
	return mailables.NewAnalyticsDigestWithSubjectLabel(locale,
		mailables.FormatPeriodLabelForLocale(locale, start, end),
		mailables.LocalizedDigestFrequencyLabel(locale, "weekly"),
		mailables.LocalizedDigestSubjectFrequencyLabel(locale, "weekly"),
		appURL+"/dashboard", appURL+"/settings", sites)
}

// weekBounds returns the inclusive seven-day period ending the day before Now.
func weekBounds() (time.Time, time.Time) {
	today := time.Date(Now.Year(), Now.Month(), Now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -7), today.Add(-time.Nanosecond)
}

func fullStats() mailables.ReportStats {
	return mailables.ReportStats{
		Pageviews:          48213,
		Visitors:           12877,
		BounceRate:         41.3,
		AvgSessionDuration: 154,
		TopPages:           metrics("/", 12040, "/pricing", 6120, "/docs/getting-started", 3302, "/blog/privacy-first-analytics", 2210, "/about", 1201),
		TopReferrers:       metrics("google.com", 5120, "news.ycombinator.com", 2230, "chatgpt.com", 812, "github.com", 640, "duckduckgo.com", 301),
		Goals: []api.GoalStats{
			{GoalID: fixedUUID(10), Name: "Signup", Conversions: 312, ConversionRate: 2.4},
			{GoalID: fixedUUID(11), Name: "Checkout", Conversions: 88, ConversionRate: 0.7},
		},
	}
}

func previousStats() mailables.ReportStats {
	return mailables.ReportStats{Pageviews: 41002, Visitors: 13310, BounceRate: 44.9, AvgSessionDuration: 140}
}

func metrics(pairs ...any) []api.MetricStat {
	stats := make([]api.MetricStat, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		stats = append(stats, api.MetricStat{Name: pairs[i].(string), Value: pairs[i+1].(int)})
	}
	return stats
}

func opportunityPreview() opportunitysvc.DigestPreview {
	return opportunitysvc.DigestPreview{
		Frequency:  api.ReportFrequencyWeekly,
		ShouldSend: true,
		Reason:     opportunitysvc.DigestPreviewReasonReady,
		Items: []opportunitysvc.DigestItem{{
			ID:          fixedUUID(20).String(),
			SiteID:      fixedUUID(1).String(),
			TypeKey:     "opportunities.types.checkout_conversion",
			TitleKey:    "opportunities.catalog.checkout_conversion.title",
			ActionKey:   "opportunities.catalog.checkout_conversion.action",
			DigestKey:   "opportunities.catalog.checkout_conversion.digest",
			CopyParams:  map[string]any{"conversion_rate": "42%"},
			ImpactValue: "$1,200",
			Confidence:  "high",
			Score:       91,
			Evidence: []api.OpportunityEvidence{
				{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"},
			},
			CitedEvidenceIDs: []string{"conversion_rate"},
		}},
	}
}

func fixedUUID(n byte) uuid.UUID {
	return uuid.UUID{15: n}
}

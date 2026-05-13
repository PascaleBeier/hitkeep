package mailer

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// mockDriver captures every argument passed to Send.
type mockDriver struct {
	to       []string
	subject  string
	htmlBody string
	textBody string
	sendErr  error
}

func (d *mockDriver) Send(to []string, subject string, htmlBody string, textBody string) error {
	d.to = to
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	return d.sendErr
}

func (d *mockDriver) Close() error { return nil }

// stubMailable is a minimal Mailable for tests.
type stubMailable struct {
	subject  string
	template string
	data     any
	locale   string
}

func (m *stubMailable) Subject() string  { return m.subject }
func (m *stubMailable) Template() string { return m.template }
func (m *stubMailable) Data() any        { return m.data }
func (m *stubMailable) Locale() string   { return m.locale }

func newPasswordResetMailable(link string) *stubMailable {
	return &stubMailable{
		subject:  "Reset your HitKeep Password",
		template: "password_reset.mjml",
		data:     struct{ Link string }{Link: link},
	}
}

func newLocalizedPasswordResetMailable(link, locale string) *stubMailable {
	return &stubMailable{
		subject:  Translate(locale, "subject.password_reset"),
		template: "password_reset.mjml",
		data:     struct{ Link string }{Link: link},
		locale:   locale,
	}
}

func newLocalizedSiteReportMailable(locale string) *stubMailable {
	type reportStats struct {
		Pageviews          int
		Visitors           int
		BounceRate         float64
		AvgSessionDuration float64
		TopPages           []struct {
			Name  string
			Value int
		}
		TopReferrers []struct {
			Name  string
			Value int
		}
		Goals []struct {
			Name        string
			Conversions int
		}
	}

	return &stubMailable{
		subject:  Translatef(locale, "subject.site_report", Translate(locale, "freq.report.daily"), "example.com", "31. März 2026"),
		template: "site_report.mjml",
		locale:   locale,
		data: struct {
			SiteDomain     string
			PeriodLabel    string
			FreqLabel      string
			DashURL        string
			SettingsURL    string
			Current        reportStats
			Previous       reportStats
			DailyPageviews []int
		}{
			SiteDomain:  "example.com",
			PeriodLabel: "31. März 2026",
			FreqLabel:   Translate(locale, "freq.report.daily"),
			DashURL:     "https://example.com/dashboard",
			SettingsURL: "https://example.com/settings",
			Current: reportStats{
				Pageviews:          12,
				Visitors:           4,
				BounceRate:         12.5,
				AvgSessionDuration: 65,
			},
			Previous: reportStats{
				Pageviews: 10,
				Visitors:  3,
			},
			DailyPageviews: []int{3, 4, 5},
		},
	}
}

func newLocalizedDigestMailable(locale string) *stubMailable {
	type digestSite struct {
		Domain        string
		DashURL       string
		Pageviews     int
		PrevPageviews int
	}

	return &stubMailable{
		subject:  Translatef(locale, "subject.analytics_digest", Translate(locale, "freq.digest_subject.weekly"), "23–29 mars 2026"),
		template: "analytics_digest.mjml",
		locale:   locale,
		data: struct {
			PeriodLabel string
			FreqLabel   string
			DashURL     string
			SettingsURL string
			Sites       []digestSite
		}{
			PeriodLabel: "23–29 mars 2026",
			FreqLabel:   Translate(locale, "freq.digest.weekly"),
			DashURL:     "https://example.com/dashboard",
			SettingsURL: "https://example.com/settings",
			Sites: []digestSite{
				{
					Domain:        "example.com",
					DashURL:       "https://example.com/dashboard",
					Pageviews:     24,
					PrevPageviews: 20,
				},
			},
		},
	}
}

func newLocalizedOpportunityDigestMailable(locale string) *stubMailable {
	type evidence struct {
		Label string
		Value string
	}
	type item struct {
		ID          string
		Title       string
		Digest      string
		Action      string
		ImpactValue string
		Confidence  string
		Score       int
		URL         string
		Evidence    []evidence
	}

	return &stubMailable{
		subject:  Translatef(locale, "subject.opportunity_digest_one", Translate(locale, "freq.digest_subject.weekly"), "shop.example", "4.-10. Mai 2026"),
		template: "opportunity_digest.mjml",
		locale:   locale,
		data: struct {
			SiteDomain       string
			PeriodLabel      string
			FreqLabel        string
			OpportunitiesURL string
			SettingsURL      string
			Items            []item
		}{
			SiteDomain:       "shop.example",
			PeriodLabel:      "4.-10. Mai 2026",
			FreqLabel:        Translate(locale, "freq.digest.weekly"),
			OpportunitiesURL: "https://example.com/opportunities",
			SettingsURL:      "https://example.com/settings",
			Items: []item{
				{
					ID:          "op-1",
					Title:       Translate(locale, "opportunities.catalog.checkout_conversion.title"),
					Digest:      strings.ReplaceAll(Translate(locale, "opportunities.catalog.checkout_conversion.digest"), "{{conversion_rate}}", "42%"),
					Action:      Translate(locale, "opportunities.catalog.checkout_conversion.action"),
					ImpactValue: "$1,200",
					Confidence:  "high",
					Score:       91,
					URL:         "https://example.com/opportunities#op-1",
					Evidence:    []evidence{{Label: Translate(locale, "opportunities.evidence.checkout_conversion_rate"), Value: "42%"}},
				},
			},
		},
	}
}

func newUserInviteMailable(link, siteName, inviter string) *stubMailable {
	return &stubMailable{
		subject:  "You've been invited to join " + siteName,
		template: "user_invite.mjml",
		data: struct {
			Link     string
			SiteName string
			Inviter  string
		}{Link: link, SiteName: siteName, Inviter: inviter},
	}
}

func newTeamInviteNewUserMailable(link, teamName, inviter, role string) *stubMailable {
	return &stubMailable{
		subject:  "You're invited to join " + teamName + " on HitKeep",
		template: "team_invite.mjml",
		data: struct {
			Link                 string
			TeamName             string
			Inviter              string
			Role                 string
			ActionLabel          string
			HelpText             string
			RequiresAccountSetup bool
		}{
			Link:                 link,
			TeamName:             teamName,
			Inviter:              inviter,
			Role:                 role,
			ActionLabel:          "Set Password & Join Team",
			HelpText:             "Create your password to activate your account and join this team.",
			RequiresAccountSetup: true,
		},
	}
}

func newTeamInviteExistingUserMailable(link, teamName, inviter, role string) *stubMailable {
	return &stubMailable{
		subject:  "You've been added to " + teamName + " on HitKeep",
		template: "team_invite.mjml",
		data: struct {
			Link                 string
			TeamName             string
			Inviter              string
			Role                 string
			ActionLabel          string
			HelpText             string
			RequiresAccountSetup bool
		}{
			Link:                 link,
			TeamName:             teamName,
			Inviter:              inviter,
			Role:                 role,
			ActionLabel:          "Open HitKeep",
			HelpText:             "Sign in to access your team workspace.",
			RequiresAccountSetup: false,
		},
	}
}

// ---------------------------------------------------------------------------
// Disabled / nil guard
// ---------------------------------------------------------------------------

func TestSendNilMailerReturnsDisabledError(t *testing.T) {
	var m *Mailer
	err := m.Send("a@b.com", newPasswordResetMailable("https://x"))
	if !errors.Is(err, ErrMailerDisabled) {
		t.Fatalf("expected ErrMailerDisabled, got %v", err)
	}
}

func TestSendNilDriverReturnsDisabledError(t *testing.T) {
	m := &Mailer{driver: nil}
	err := m.Send("a@b.com", newPasswordResetMailable("https://x"))
	if !errors.Is(err, ErrMailerDisabled) {
		t.Fatalf("expected ErrMailerDisabled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Driver receives correct recipient and subject
// ---------------------------------------------------------------------------

func TestSendPassesRecipientAndSubject(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", newPasswordResetMailable("https://example.com/reset"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(drv.to) != 1 || drv.to[0] != "user@example.com" {
		t.Fatalf("expected to=[user@example.com], got %v", drv.to)
	}
	if drv.subject != "Reset your HitKeep Password" {
		t.Fatalf("expected subject %q, got %q", "Reset your HitKeep Password", drv.subject)
	}
}

// ---------------------------------------------------------------------------
// Driver error propagation
// ---------------------------------------------------------------------------

func TestSendReturnsDriverError(t *testing.T) {
	drv := &mockDriver{sendErr: errors.New("connection refused")}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", newPasswordResetMailable("https://x"))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected driver error to propagate, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Missing template
// ---------------------------------------------------------------------------

func TestSendMissingTemplateReturnsError(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", &stubMailable{
		subject:  "Test",
		template: "nonexistent.mjml",
		data:     struct{}{},
	})
	if err == nil {
		t.Fatalf("expected error for missing template")
	}
}

// ---------------------------------------------------------------------------
// HTML rendering – password reset
// ---------------------------------------------------------------------------

func TestSendHTMLPasswordReset(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/reset/token123"
	err := m.Send("user@example.com", newPasswordResetMailable(link))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.htmlBody, "<!doctype html>") {
		t.Fatalf("expected HTML doctype in output")
	}
	if !strings.Contains(drv.htmlBody, link) {
		t.Fatalf("expected HTML to contain reset link")
	}
	if !strings.Contains(drv.htmlBody, "HitKeep") {
		t.Fatalf("expected HTML to contain branding")
	}
}

// ---------------------------------------------------------------------------
// HTML rendering – user invite
// ---------------------------------------------------------------------------

func TestSendHTMLUserInvite(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/invite/abc"
	err := m.Send("new@example.com", newUserInviteMailable(link, "Acme Corp", "Bob"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.htmlBody, "<!doctype html>") {
		t.Fatalf("expected HTML doctype in output")
	}
	if !strings.Contains(drv.htmlBody, link) {
		t.Fatalf("expected HTML to contain invite link")
	}
	if !strings.Contains(drv.htmlBody, "Acme Corp") {
		t.Fatalf("expected HTML to contain site name")
	}
	if !strings.Contains(drv.htmlBody, "Bob") {
		t.Fatalf("expected HTML to contain inviter name")
	}
}

// ---------------------------------------------------------------------------
// HTML rendering – team invite (new user)
// ---------------------------------------------------------------------------

func TestSendHTMLTeamInviteNewUser(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/accept-invite?token=abc"
	err := m.Send("new@example.com", newTeamInviteNewUserMailable(link, "Acme Corp", "Bob", "admin"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.htmlBody, "<!doctype html>") {
		t.Fatalf("expected HTML doctype in output")
	}
	for _, want := range []string{link, "Acme Corp", "Bob", "admin", "Set Password", "create a password"} {
		if !strings.Contains(drv.htmlBody, want) {
			t.Fatalf("expected HTML to contain %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// HTML rendering – team invite (existing user)
// ---------------------------------------------------------------------------

func TestSendHTMLTeamInviteExistingUser(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/dashboard"
	err := m.Send("existing@example.com", newTeamInviteExistingUserMailable(link, "Acme Corp", "Bob", "viewer"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.htmlBody, "<!doctype html>") {
		t.Fatalf("expected HTML doctype in output")
	}
	for _, want := range []string{link, "Acme Corp", "Bob", "viewer", "Open HitKeep"} {
		if !strings.Contains(drv.htmlBody, want) {
			t.Fatalf("expected HTML to contain %q", want)
		}
	}
	if strings.Contains(drv.htmlBody, "create a password") {
		t.Fatalf("existing user email should NOT contain account setup text")
	}
}

// ---------------------------------------------------------------------------
// Plain-text rendering – password reset
// ---------------------------------------------------------------------------

func TestSendPlainTextPasswordReset(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/reset/abc123"
	err := m.Send("user@example.com", newPasswordResetMailable(link))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.textBody, "Reset Password") {
		t.Fatalf("expected plain-text heading, got:\n%s", drv.textBody)
	}
	if !strings.Contains(drv.textBody, link) {
		t.Fatalf("expected plain-text reset link, got:\n%s", drv.textBody)
	}
	if !strings.Contains(drv.textBody, "HitKeep") {
		t.Fatalf("expected plain-text footer, got:\n%s", drv.textBody)
	}
	if strings.Contains(drv.textBody, "<") {
		t.Fatalf("plain-text body must not contain HTML tags, got:\n%s", drv.textBody)
	}
}

func TestSendLocalizedPasswordResetUsesRecipientLocale(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	link := "https://example.com/reset/de"
	err := m.Send("user@example.com", newLocalizedPasswordResetMailable(link, "de"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Passwort zurücksetzen", "Hallo", link} {
		if !strings.Contains(drv.htmlBody, want) {
			t.Fatalf("expected localized HTML to contain %q, got:\n%s", want, drv.htmlBody)
		}
		if !strings.Contains(drv.textBody, want) {
			t.Fatalf("expected localized text to contain %q, got:\n%s", want, drv.textBody)
		}
	}
	if drv.subject != "Setze dein HitKeep-Passwort zurück" {
		t.Fatalf("expected localized subject, got %q", drv.subject)
	}
}

func TestSendLocalizedSiteReportUsesUTF8TextLabels(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", newLocalizedSiteReportMailable("de"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Kennzahlen", "Seitenaufrufe", "31. März 2026"} {
		if !strings.Contains(drv.textBody, want) {
			t.Fatalf("expected localized report text to contain %q, got:\n%s", want, drv.textBody)
		}
	}
}

func TestSendLocalizedDigestUsesTranslatedTextLabels(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", newLocalizedDigestMailable("fr"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Résumé d'analytique", "Tableau de bord:", "Évolution"} {
		if !strings.Contains(drv.textBody, want) && !strings.Contains(drv.htmlBody, want) {
			t.Fatalf("expected localized digest output to contain %q", want)
		}
	}
	if strings.Contains(drv.textBody, "Dashboard:") {
		t.Fatalf("expected digest text template to avoid hardcoded English label, got:\n%s", drv.textBody)
	}
}

func TestSendOpportunityDigestRendersLocalizedSafeRecommendation(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("user@example.com", newLocalizedOpportunityDigestMailable("de"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"wöchentliche Chance für shop.example", "Checkout-Abbruch prüfen", "Nächste Aktion", "Checkout-Conversion-Rate", "42%"} {
		if !strings.Contains(drv.textBody, want) && !strings.Contains(drv.htmlBody, want) {
			t.Fatalf("expected opportunity digest output to contain %q", want)
		}
	}
	for _, forbidden := range []string{"raw_prompt", "provider_response", "opportunities.catalog.checkout_conversion.title"} {
		if strings.Contains(drv.textBody, forbidden) || strings.Contains(drv.htmlBody, forbidden) {
			t.Fatalf("opportunity digest leaked forbidden value %q", forbidden)
		}
	}
}

func TestMonthNameUsesUTF8LocaleData(t *testing.T) {
	if got := MonthName("de", time.March, false); got != "März" {
		t.Fatalf("expected German March to be März, got %q", got)
	}
	if got := MonthName("fr", time.August, false); got != "août" {
		t.Fatalf("expected French August to be août, got %q", got)
	}
	if got := MonthName("nl", time.March, false); got != "maart" {
		t.Fatalf("expected Dutch March to be maart, got %q", got)
	}
}

func TestSupportedLocalesAreStable(t *testing.T) {
	want := []string{"de", "en", "es", "fr", "it", "nl"}
	got := SupportedLocales()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected supported locales %v, got %v", want, got)
	}
}

// ---------------------------------------------------------------------------
// Plain-text rendering – user invite
// ---------------------------------------------------------------------------

func TestSendPlainTextUserInvite(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("invitee@example.com", newUserInviteMailable(
		"https://example.com/invite/xyz", "My Site", "Alice",
	))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Alice", "My Site", "https://example.com/invite/xyz"} {
		if !strings.Contains(drv.textBody, want) {
			t.Fatalf("expected plain-text to contain %q, got:\n%s", want, drv.textBody)
		}
	}
	if strings.Contains(drv.textBody, "<") {
		t.Fatalf("plain-text body must not contain HTML tags, got:\n%s", drv.textBody)
	}
}

// ---------------------------------------------------------------------------
// Plain-text rendering – team invite (new user)
// ---------------------------------------------------------------------------

func TestSendPlainTextTeamInviteNewUser(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("new@example.com", newTeamInviteNewUserMailable(
		"https://example.com/accept-invite?token=xyz", "DevTeam", "Alice", "editor",
	))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Alice", "DevTeam", "editor", "https://example.com/accept-invite?token=xyz", "Set Password & Join Team", "create a password"} {
		if !strings.Contains(drv.textBody, want) {
			t.Fatalf("expected plain-text to contain %q, got:\n%s", want, drv.textBody)
		}
	}
	if strings.Contains(drv.textBody, "<") {
		t.Fatalf("plain-text body must not contain HTML tags, got:\n%s", drv.textBody)
	}
}

// ---------------------------------------------------------------------------
// Plain-text rendering – team invite (existing user)
// ---------------------------------------------------------------------------

func TestSendPlainTextTeamInviteExistingUser(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("existing@example.com", newTeamInviteExistingUserMailable(
		"https://example.com/dashboard", "DevTeam", "Alice", "viewer",
	))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, want := range []string{"Alice", "DevTeam", "viewer", "https://example.com/dashboard", "Open HitKeep"} {
		if !strings.Contains(drv.textBody, want) {
			t.Fatalf("expected plain-text to contain %q, got:\n%s", want, drv.textBody)
		}
	}
	if strings.Contains(drv.textBody, "create a password") {
		t.Fatalf("existing user plain-text should NOT contain account setup text")
	}
	if strings.Contains(drv.textBody, "<") {
		t.Fatalf("plain-text body must not contain HTML tags, got:\n%s", drv.textBody)
	}
}

// ---------------------------------------------------------------------------
// HTML and plain-text parity: same data appears in both bodies
// ---------------------------------------------------------------------------

func TestSendBothBodiesContainSameData(t *testing.T) {
	tests := []struct {
		name     string
		mailable *stubMailable
		wantIn   []string
	}{
		{
			name:     "password_reset",
			mailable: newPasswordResetMailable("https://example.com/reset/parity"),
			wantIn:   []string{"https://example.com/reset/parity", "HitKeep"},
		},
		{
			name:     "user_invite",
			mailable: newUserInviteMailable("https://example.com/invite/parity", "TestSite", "Charlie"),
			wantIn:   []string{"https://example.com/invite/parity", "TestSite", "Charlie", "HitKeep"},
		},
		{
			name:     "team_invite_new_user",
			mailable: newTeamInviteNewUserMailable("https://example.com/team/parity", "ParityTeam", "Dave", "admin"),
			wantIn:   []string{"https://example.com/team/parity", "ParityTeam", "Dave", "admin", "HitKeep"},
		},
		{
			name:     "team_invite_existing_user",
			mailable: newTeamInviteExistingUserMailable("https://example.com/team/parity2", "ParityTeam", "Eve", "viewer"),
			wantIn:   []string{"https://example.com/team/parity2", "ParityTeam", "Eve", "viewer", "HitKeep"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drv := &mockDriver{}
			m := &Mailer{driver: drv}

			if err := m.Send("x@y.com", tc.mailable); err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			for _, s := range tc.wantIn {
				if !strings.Contains(drv.htmlBody, s) {
					t.Fatalf("HTML body missing %q", s)
				}
				if !strings.Contains(drv.textBody, s) {
					t.Fatalf("plain-text body missing %q", s)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTML contains structural markers from layout
// ---------------------------------------------------------------------------

func TestSendHTMLContainsLayoutStructure(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("a@b.com", newPasswordResetMailable("https://example.com/x"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// MJML always outputs a full HTML document with a <head> and <body>.
	for _, marker := range []string{"<head>", "<body", "</html>"} {
		if !strings.Contains(drv.htmlBody, marker) {
			t.Fatalf("expected HTML to contain %q", marker)
		}
	}
}

// ---------------------------------------------------------------------------
// Subject injected into HTML <title>
// ---------------------------------------------------------------------------

func TestSendHTMLContainsSubjectInTitle(t *testing.T) {
	drv := &mockDriver{}
	m := &Mailer{driver: drv}

	err := m.Send("a@b.com", newPasswordResetMailable("https://example.com/x"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(drv.htmlBody, "<title>Reset your HitKeep Password</title>") {
		t.Fatalf("expected <title> to contain subject line")
	}
}

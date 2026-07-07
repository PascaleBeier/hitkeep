//go:build billing

package mailables

import (
	"strings"
	"testing"

	"hitkeep/internal/mailer"
)

type cloudLifecycleMailDriver struct {
	subject  string
	htmlBody string
	textBody string
}

func (d *cloudLifecycleMailDriver) Send(_ []string, subject, htmlBody, textBody string) error {
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	return nil
}

func (d *cloudLifecycleMailDriver) Close() error { return nil }

func TestCloudLifecycleMailablesRenderForSupportedLocales(t *testing.T) {
	links := CloudLifecycleLinks{
		DashboardURL: "https://cloud.hitkeep.eu/admin/team",
		UpgradeURL:   "https://cloud.hitkeep.eu/admin/team/overview",
		FundingURL:   "https://hitkeep.com/support/funding/",
		DocsURL:      "https://hitkeep.com/guides/introduction/",
		WordPressURL: "https://hitkeep.com/guides/integrations/wordpress/",
		FeedbackURL:  "https://hitkeep.com/support/help/",
	}

	for _, locale := range mailer.SupportedLocales() {
		t.Run(locale+"_welcome", func(t *testing.T) {
			driver := &cloudLifecycleMailDriver{}
			m := mailer.NewWithDriver(driver, nil)
			if err := m.Send("owner@example.com", NewCloudWelcome(locale, "Acme", "example.com", true, 60, links)); err != nil {
				t.Fatalf("send welcome: %v", err)
			}
			assertRenderedLifecycleEmail(t, driver, links.DashboardURL, links.DocsURL, links.WordPressURL, links.FeedbackURL, links.UpgradeURL, links.FundingURL)
			if !strings.Contains(driver.textBody, "60") {
				t.Fatalf("expected free retention note in welcome text body, got %q", driver.textBody)
			}
		})

		t.Run(locale+"_retention", func(t *testing.T) {
			driver := &cloudLifecycleMailDriver{}
			m := mailer.NewWithDriver(driver, nil)
			if err := m.Send("owner@example.com", NewCloudFreeRetentionReminder(locale, "Acme", "example.com", 60, links)); err != nil {
				t.Fatalf("send retention reminder: %v", err)
			}
			// The upgrade email intentionally carries a single primary CTA to the plan comparison.
			assertRenderedLifecycleEmail(t, driver, links.UpgradeURL, links.DashboardURL, links.FundingURL)
			if !strings.Contains(driver.textBody, "60") {
				t.Fatalf("expected retention days in reminder text body, got %q", driver.textBody)
			}
		})
	}
}

func assertRenderedLifecycleEmail(t *testing.T, driver *cloudLifecycleMailDriver, expectedLinks ...string) {
	t.Helper()
	if strings.TrimSpace(driver.subject) == "" {
		t.Fatalf("expected subject")
	}
	for _, link := range expectedLinks {
		if !strings.Contains(driver.htmlBody, link) {
			t.Fatalf("expected html body to contain %q", link)
		}
		if !strings.Contains(driver.textBody, link) {
			t.Fatalf("expected text body to contain %q", link)
		}
	}
}

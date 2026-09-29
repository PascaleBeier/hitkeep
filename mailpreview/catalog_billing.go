//go:build billing

package mailpreview

import (
	"hitkeep/mailables"
	"hitkeep/mailer"
)

var cloudLinks = mailables.CloudLifecycleLinks{
	DashboardURL: appURL + "/dashboard",
	UpgradeURL:   appURL + "/settings/billing",
	FundingURL:   "https://github.com/sponsors/pascalebeier",
	DocsURL:      "https://hitkeep.com/docs/",
	WordPressURL: "https://wordpress.org/plugins/hitkeep/",
	FeedbackURL:  "mailto:feedback@hitkeep.com",
}

func init() {
	fixtures = append(fixtures,
		Fixture{ID: "email_verification", Build: func(locale string) mailer.Mailable {
			return mailables.NewEmailVerification(appURL+"/signup/verify?token=preview", "Acme Analytics", locale)
		}},
		Fixture{ID: "cloud_welcome_free", Build: func(locale string) mailer.Mailable {
			return mailables.NewCloudWelcome(locale, "Acme Analytics", "shop.example", true, 60, cloudLinks)
		}},
		Fixture{ID: "cloud_welcome_paid", Build: func(locale string) mailer.Mailable {
			return mailables.NewCloudWelcome(locale, "Acme Analytics", "shop.example", false, 730, cloudLinks)
		}},
		Fixture{ID: "cloud_free_retention_reminder", Build: func(locale string) mailer.Mailable {
			return mailables.NewCloudFreeRetentionReminder(locale, "Acme Analytics", "shop.example", 60, cloudLinks)
		}},
		Fixture{ID: "cloud_free_retention_pretrim", Build: func(locale string) mailer.Mailable {
			return mailables.NewCloudFreeRetentionPreTrim(locale, "Acme Analytics", "shop.example", 60, "October 6, 2026", cloudLinks)
		}},
		Fixture{ID: "cloud_free_limit_reminder", Build: func(locale string) mailer.Mailable {
			return mailables.NewCloudFreeLimitReminder(locale, "Acme Analytics", 1, 1, cloudLinks)
		}},
	)
}

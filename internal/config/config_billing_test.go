//go:build billing

package config

import "testing"

func TestLoadCloudConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"HITKEEP_CLOUD_HOSTED":                   "true",
		"HITKEEP_CLOUD_SIGNUP_ENABLED":           "true",
		"HITKEEP_CLOUD_JURISDICTION":             "EU",
		"HITKEEP_CLOUD_REGION":                   "eu-central-1",
		"HITKEEP_CLOUD_UPGRADE_URL":              "https://hitkeep.com/cloud/upgrade",
		"HITKEEP_CLOUD_SUPPORT_URL":              "https://hitkeep.com/cloud/support",
		"HITKEEP_CLOUD_PLAN_CODE":                "pro",
		"HITKEEP_CLOUD_PLAN_NAME":                "Pro",
		"HITKEEP_CLOUD_MAX_TEAMS":                "2",
		"HITKEEP_CLOUD_MAX_SITES_PER_TEAM":       "12",
		"HITKEEP_CLOUD_MAX_RETENTION_DAYS":       "365",
		"HITKEEP_CLOUD_MAX_TEAM_MEMBERS":         "15",
		"HITKEEP_CLOUD_ALLOW_SSO":                "true",
		"HITKEEP_CLOUD_ALLOW_CUSTOM_BRANDING":    "true",
		"HITKEEP_STRIPE_SECRET_KEY":              "sk_test_123",
		"HITKEEP_STRIPE_PUBLISHABLE_KEY":         "pk_test_123",
		"HITKEEP_STRIPE_WEBHOOK_SECRET":          "whsec_123",
		"HITKEEP_STRIPE_PORTAL_CONFIGURATION_ID": "bpc_123",
		"HITKEEP_STRIPE_PRICE_PRO_MONTHLY":       "price_pro",
		"HITKEEP_STRIPE_PRICE_BUSINESS_MONTHLY":  "price_business",
		"HITKEEP_STRIPE_PRICE_PRO_ANNUAL":        "price_pro_annual",
		"HITKEEP_STRIPE_PRICE_BUSINESS_ANNUAL":   "price_business_annual",
		"HITKEEP_CLOUD_CHECKOUT_SUCCESS_URL":     "https://cloud.hitkeep.eu/admin/team?checkout=success",
		"HITKEEP_CLOUD_CHECKOUT_CANCEL_URL":      "https://cloud.hitkeep.eu/admin/team?checkout=cancelled",
	}

	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if !conf.CloudHosted || !conf.CloudSignupEnabled {
		t.Fatalf("expected cloud runtime flags to load from env")
	}
	if conf.CloudJurisdiction != "EU" || conf.CloudRegion != "eu-central-1" {
		t.Fatalf("unexpected cloud location config: %+v", conf)
	}
	if conf.CloudPlanCode != "pro" || conf.CloudPlanName != "Pro" {
		t.Fatalf("unexpected cloud plan config: code=%q name=%q", conf.CloudPlanCode, conf.CloudPlanName)
	}
	if !conf.CloudAllowSSO || !conf.CloudAllowCustomBranding {
		t.Fatalf("expected cloud feature flags to load from env")
	}
	if conf.StripeSecretKey == "" || conf.StripePublishableKey == "" || conf.StripeWebhookSecret == "" {
		t.Fatalf("expected stripe secrets to load from env")
	}
	if conf.StripePortalConfigurationID != "bpc_123" {
		t.Fatalf("unexpected stripe portal config: %+v", conf)
	}
	if conf.StripePriceProMonthly != "price_pro" || conf.StripePriceBusinessMonthly != "price_business" ||
		conf.StripePriceProAnnual != "price_pro_annual" || conf.StripePriceBusinessAnnual != "price_business_annual" {
		t.Fatalf("unexpected stripe price config: %+v", conf)
	}
}

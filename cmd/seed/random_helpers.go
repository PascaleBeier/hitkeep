package main

import (
	mrand "math/rand"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func hasAPIClientNamed(clients []api.APIClient, name string) bool {
	for _, client := range clients {
		if strings.EqualFold(strings.TrimSpace(client.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func pickViewport(rng *mrand.Rand, kind string) (vw, vh, sw, sh int) {
	switch kind {
	case "mobile":
		v := mobileViewports[rng.Intn(len(mobileViewports))]
		return v.vw, v.vh, v.sw, v.sh
	case "tablet":
		v := tabletViewports[rng.Intn(len(tabletViewports))]
		return v.vw, v.vh, v.sw, v.sh
	default:
		v := desktopViewports[rng.Intn(len(desktopViewports))]
		return v.vw, v.vh, v.sw, v.sh
	}
}

func randomTimeInDay(rng *mrand.Rand, day time.Time) time.Time {
	hourWeights := []int{
		1, 1, 1, 1, 1, 2,
		4, 7, 10, 12, 14, 14,
		15, 14, 13, 12, 11, 10,
		8, 6, 5, 3, 2, 1,
	}
	total := 0
	for _, w := range hourWeights {
		total += w
	}
	n := rng.Intn(total)
	hour := 0
	for i, w := range hourWeights {
		n -= w
		if n < 0 {
			hour = i
			break
		}
	}
	minute := rng.Intn(60)
	second := rng.Intn(60)
	return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func classifySeedResourceType(contentType string) string {
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(normalized, "text/html"):
		return "html"
	case strings.HasPrefix(normalized, "application/pdf"),
		strings.Contains(normalized, "msword"),
		strings.Contains(normalized, "officedocument"):
		return "document"
	case strings.HasPrefix(normalized, "image/"):
		return "image"
	default:
		return "other"
	}
}

func seedAIReferredVisits(batch *seedWriteBatch, siteID uuid.UUID, fetch *api.AIFetch, target aiFetchTarget, rng *mrand.Rand) (sessions int, hits int) {
	if fetch == nil || target.visitChance <= 0 || target.visitMax <= 0 {
		return 0, 0
	}
	if fetch.Timestamp.After(time.Now().UTC().Add(-30 * time.Minute)) {
		return 0, 0
	}
	if rng.Float64() >= target.visitChance {
		return 0, 0
	}

	visitCount := target.visitMin
	if target.visitMax > target.visitMin {
		visitCount += rng.Intn(target.visitMax - target.visitMin + 1)
	}

	referrer := aiReferrerForFamily(fetch.AssistantFamily, rng)
	hostname := "acme-analytics.io"
	isUnique := true

	for range visitCount {
		sessionID := uuid.New()
		uaEntry := pickWeighted(rng, userAgents)
		country := pickWeighted(rng, countries)
		region, city, provider, asn, asnOrg := seedGeoNetworkMetadata(country, rng)
		lang := pickWeighted(rng, languages)
		vw, vh, sw, sh := pickViewport(rng, uaEntry.kind)
		sessionStart := randomAIFollowupTime(fetch.Timestamp, rng)
		sessionLen := 1 + rng.Intn(3)

		for i := range sessionLen {
			path := fetch.Path
			if i > 0 {
				path = nextPageAfterAIEntry(fetch.Path, rng)
			}

			ts := sessionStart.Add(time.Duration(i*70+rng.Intn(90)) * time.Second)
			h := &api.Hit{
				SiteID:         siteID,
				SessionID:      sessionID,
				PageID:         uuid.New(),
				Timestamp:      ts,
				Path:           path,
				Hostname:       &hostname,
				Referrer:       nil,
				UserAgent:      new(uaEntry.ua),
				CountryCode:    country,
				Region:         region,
				City:           city,
				Provider:       provider,
				ASN:            asn,
				ASNOrg:         asnOrg,
				Language:       lang,
				ViewportWidth:  new(vw),
				ViewportHeight: new(vh),
				ScreenWidth:    new(sw),
				ScreenHeight:   new(sh),
				IsUnique:       new(i == 0),
			}
			if i == 0 {
				h.Referrer = referrer
				h.IsUnique = &isUnique
			}

			batch.addHit(h)
			hits++
		}
		sessions++
	}

	return sessions, hits
}

func seedGeoNetworkMetadata(country *string, rng *mrand.Rand) (*string, *string, *string, *int, *string) {
	if country == nil {
		return nil, nil, nil, nil, nil
	}
	choices, ok := seedGeoNetworkChoices[*country]
	if !ok {
		return nil, nil, nil, nil, nil
	}
	meta := pickWeighted(rng, choices)
	return new(meta.region), new(meta.city), new(meta.provider), new(meta.asn), new(meta.asnOrg)
}

type seedGeoNetwork struct {
	region   string
	city     string
	provider string
	asn      int
	asnOrg   string
}

var seedGeoNetworkChoices = map[string][]weightedEntry[seedGeoNetwork]{
	"US": {
		{seedGeoNetwork{region: "California", city: "Mountain View", provider: "Google LLC", asn: 15169, asnOrg: "Google LLC"}, 4},
		{seedGeoNetwork{region: "New York", city: "New York", provider: "Verizon Business", asn: 701, asnOrg: "Verizon Business"}, 3},
		{seedGeoNetwork{region: "Washington", city: "Seattle", provider: "Comcast Cable", asn: 7922, asnOrg: "Comcast Cable Communications LLC"}, 2},
	},
	"DE": {
		{seedGeoNetwork{region: "Berlin", city: "Berlin", provider: "Deutsche Telekom AG", asn: 3320, asnOrg: "Deutsche Telekom AG"}, 5},
		{seedGeoNetwork{region: "Bavaria", city: "Munich", provider: "Vodafone GmbH", asn: 3209, asnOrg: "Vodafone GmbH"}, 3},
	},
	"GB": {{seedGeoNetwork{region: "England", city: "London", provider: "BT", asn: 2856, asnOrg: "British Telecommunications PLC"}, 1}},
	"FR": {{seedGeoNetwork{region: "Ile-de-France", city: "Paris", provider: "Orange", asn: 3215, asnOrg: "Orange S.A."}, 1}},
	"NL": {{seedGeoNetwork{region: "North Holland", city: "Amsterdam", provider: "KPN", asn: 1136, asnOrg: "KPN B.V."}, 1}},
}

func randomAIFollowupTime(fetchTime time.Time, rng *mrand.Rand) time.Time {
	delayHours := 2 + rng.Intn(72)
	delayMinutes := rng.Intn(60)
	followup := fetchTime.Add(time.Duration(delayHours)*time.Hour + time.Duration(delayMinutes)*time.Minute)
	cutoff := time.Now().UTC().Add(-15 * time.Minute)
	if followup.After(cutoff) {
		return cutoff
	}
	return followup
}

func aiReferrerForFamily(family string, rng *mrand.Rand) *string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "openai":
		return new(pickWeighted(rng, []weightedEntry[string]{
			{value: "https://chatgpt.com/c/hitkeep-demo", weight: 7},
			{value: "https://chat.openai.com/share/hitkeep-demo", weight: 3},
		}))
	case "anthropic":
		return new("https://claude.ai/chat/hitkeep-demo")
	case "perplexity":
		return new("https://www.perplexity.ai/page/hitkeep-demo")
	case "google":
		return new("https://gemini.google.com/app/hitkeep-demo")
	case "deepseek":
		return new("https://chat.deepseek.com/search/hitkeep-demo")
	default:
		return new("https://chatgpt.com/c/hitkeep-demo")
	}
}

func nextPageAfterAIEntry(entryPath string, rng *mrand.Rand) string {
	switch entryPath {
	case "/pricing":
		return pickWeighted(rng, []weightedEntry[string]{
			{"/signup", 45},
			{"/features", 25},
			{"/docs/getting-started", 20},
			{"/contact", 10},
		})
	case "/features":
		return pickWeighted(rng, []weightedEntry[string]{
			{"/pricing", 35},
			{"/signup", 25},
			{"/docs/getting-started", 25},
			{"/contact", 15},
		})
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		return pickWeighted(rng, []weightedEntry[string]{
			{"/docs/getting-started", 20},
			{"/docs/configuration", 25},
			{"/docs/api-reference", 25},
			{"/pricing", 20},
			{"/contact", 10},
		})
	case "/blog/privacy-first-analytics-2025", "/blog/replace-google-analytics":
		return pickWeighted(rng, []weightedEntry[string]{
			{"/pricing", 30},
			{"/features", 25},
			{"/docs/getting-started", 25},
			{"/signup", 20},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"/", 20},
			{"/pricing", 25},
			{"/features", 20},
			{"/docs/getting-started", 20},
			{"/signup", 15},
		})
	}
}

func randomCompanySize(rng *mrand.Rand) string {
	sizes := []string{"1-10", "11-50", "51-200", "201-500", "500+"}
	return sizes[rng.Intn(len(sizes))]
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func randomBilling(rng *mrand.Rand) string {
	if rng.Float64() < 0.65 {
		return "annual"
	}
	return "monthly"
}

func productPrice(product ecommerceProduct, billing string) int {
	if billing == "annual" && product.priceYear > 0 {
		return product.priceYear
	}
	return product.price
}

func randomCoupon(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"", 55},
		{"SPRING25", 18},
		{"ANNUAL20", 15},
		{"WELCOME10", 12},
	})
}

func randomSignupSource(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"footer-form", 40},
		{"blog-post", 25},
		{"exit-intent", 15},
		{"sidebar", 12},
		{"header-cta", 8},
	})
}

func randomNewsletterFormat(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"weekly-digest", 50},
		{"product-updates", 30},
		{"tips-and-tricks", 20},
	})
}

func randomTrialPlan(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"starter", 50},
		{"pro", 35},
		{"business", 15},
	})
}

func randomIndustry(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"saas", 35},
		{"ecommerce", 20},
		{"media", 15},
		{"fintech", 12},
		{"healthcare", 8},
		{"education", 10},
	})
}

func randomDemoSource(rng *mrand.Rand) string {
	return pickWeighted(rng, []weightedEntry[string]{
		{"pricing-page", 40},
		{"demo-button", 30},
		{"contact-form", 20},
		{"live-chat", 10},
	})
}

func randomChatbotSurface(entryPage string, rng *mrand.Rand) string {
	switch entryPage {
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		return pickWeighted(rng, []weightedEntry[string]{
			{"docs-sidebar", 45},
			{"help-center", 35},
			{"inline-docs", 20},
		})
	case "/pricing", "/signup":
		return pickWeighted(rng, []weightedEntry[string]{
			{"pricing-assistant", 60},
			{"checkout-sidebar", 25},
			{"homepage-widget", 15},
		})
	case "/contact":
		return pickWeighted(rng, []weightedEntry[string]{
			{"support-widget", 70},
			{"contact-page", 30},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"homepage-widget", 40},
			{"support-widget", 35},
			{"help-center", 25},
		})
	}
}

func randomChatbotIntent(entryPage string, rng *mrand.Rand) string {
	switch entryPage {
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		return pickWeighted(rng, []weightedEntry[string]{
			{"setup", 35},
			{"api", 25},
			{"installation", 20},
			{"retention", 10},
			{"permissions", 10},
		})
	case "/pricing", "/signup":
		return pickWeighted(rng, []weightedEntry[string]{
			{"pricing", 40},
			{"plan-comparison", 25},
			{"trial", 20},
			{"billing", 15},
		})
	case "/contact":
		return pickWeighted(rng, []weightedEntry[string]{
			{"support", 40},
			{"migration", 25},
			{"sales", 20},
			{"security", 15},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"features", 25},
			{"pricing", 20},
			{"support", 20},
			{"analytics", 20},
			{"migration", 15},
		})
	}
}

func randomCitationURL(entryPage string, rng *mrand.Rand) string {
	switch entryPage {
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		return pickWeighted(rng, []weightedEntry[string]{
			{"/docs/getting-started", 35},
			{"/docs/configuration", 30},
			{"/docs/api-reference", 20},
			{"/blog/privacy-first-analytics-2025", 15},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"/pricing", 30},
			{"/features", 30},
			{"/docs/getting-started", 20},
			{"/contact", 20},
		})
	}
}

func randomHandoffReason(entryPage string, rng *mrand.Rand) string {
	switch entryPage {
	case "/pricing", "/signup":
		return pickWeighted(rng, []weightedEntry[string]{
			{"custom-pricing", 45},
			{"invoice-question", 30},
			{"enterprise-security", 25},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"needs-human-review", 45},
			{"account-question", 30},
			{"complex-integration", 25},
		})
	}
}

func assistedGoalProbability(entryPage string) float64 {
	switch entryPage {
	case "/pricing", "/signup":
		return 0.22
	case "/docs/getting-started", "/docs/configuration", "/docs/api-reference":
		return 0.11
	case "/contact":
		return 0.18
	default:
		return 0.08
	}
}

func randomChatbotGoalName(entryPage string, rng *mrand.Rand) string {
	switch entryPage {
	case "/pricing", "/signup":
		return pickWeighted(rng, []weightedEntry[string]{
			{"trial_started", 45},
			{"purchase_completed", 35},
			{"demo_requested", 20},
		})
	case "/contact":
		return pickWeighted(rng, []weightedEntry[string]{
			{"demo_requested", 55},
			{"newsletter_signup", 25},
			{"trial_started", 20},
		})
	default:
		return pickWeighted(rng, []weightedEntry[string]{
			{"newsletter_signup", 35},
			{"trial_started", 35},
			{"demo_requested", 20},
			{"purchase_completed", 10},
		})
	}
}

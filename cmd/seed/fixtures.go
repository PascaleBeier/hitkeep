package main

var pages = []weightedEntry[string]{
	{"/", 280},
	{"/pricing", 130},
	{"/features", 90},
	{"/blog/privacy-first-analytics-2025", 80},
	{"/blog/self-hosted-vs-cloud-analytics", 70},
	{"/blog/replace-google-analytics", 65},
	{"/blog/gdpr-compliant-analytics", 60},
	{"/docs/getting-started", 70},
	{"/docs/configuration", 50},
	{"/docs/api-reference", 40},
	{"/about", 50},
	{"/changelog", 40},
	{"/signup", 110},
	{"/login", 55},
	{"/contact", 40},
	{"/terms", 20},
}

var referrers = []weightedEntry[*string]{
	{nil, 380},
	{new("https://www.google.com"), 200},
	{new("https://chatgpt.com"), 24},
	{new("https://www.perplexity.ai"), 18},
	{new("https://claude.ai"), 10},
	{new("https://gemini.google.com"), 8},
	{new("https://chat.deepseek.com"), 6},
	{new("https://news.ycombinator.com"), 120},
	{new("https://twitter.com"), 80},
	{new("https://www.reddit.com/r/selfhosted"), 60},
	{new("https://www.linkedin.com"), 50},
	{new("https://github.com/hitkeep/hitkeep"), 50},
	{new("https://dev.to"), 40},
	{new("https://lobste.rs"), 30},
	{new("https://www.producthunt.com"), 20},
}

var countries = []weightedEntry[*string]{
	{new("US"), 350},
	{new("DE"), 100},
	{new("GB"), 80},
	{new("CA"), 60},
	{new("FR"), 50},
	{new("NL"), 50},
	{new("AU"), 40},
	{new("SE"), 30},
	{new("CH"), 30},
	{new("IN"), 35},
	{new("JP"), 25},
	{new("BR"), 25},
	{new("PL"), 20},
	{new("ES"), 20},
	{new("IT"), 20},
	{new("NO"), 15},
	{new("FI"), 15},
	{new("DK"), 15},
	{new("SG"), 15},
}

type viewportPreset struct {
	vw, vh, sw, sh int
}

var desktopViewports = []viewportPreset{
	{1920, 1080, 1920, 1080},
	{1440, 900, 1440, 900},
	{1440, 810, 2560, 1440},
	{1280, 800, 1280, 800},
	{1366, 768, 1366, 768},
	{1536, 864, 1536, 864},
}

var mobileViewports = []viewportPreset{
	{390, 844, 390, 844},
	{430, 932, 430, 932},
	{412, 892, 412, 892},
	{360, 800, 360, 800},
}

var tabletViewports = []viewportPreset{
	{768, 1024, 768, 1024},
	{810, 1080, 810, 1080},
}

type uaGroup struct {
	ua   string
	kind string
}

var userAgents = []weightedEntry[uaGroup]{
	{uaGroup{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", "desktop"}, 280},
	{uaGroup{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", "desktop"}, 230},
	{uaGroup{"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15", "desktop"}, 90},
	{uaGroup{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0", "desktop"}, 80},
	{uaGroup{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0", "desktop"}, 40},
	{uaGroup{"Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36", "mobile"}, 110},
	{uaGroup{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1", "mobile"}, 100},
	{uaGroup{"Mozilla/5.0 (Linux; Android 12; SM-G998B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36", "mobile"}, 50},
	{uaGroup{"Mozilla/5.0 (iPad; CPU OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1", "tablet"}, 60},
	{uaGroup{"Mozilla/5.0 (Linux; Android 11; SM-T870) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", "tablet"}, 40},
}

var languages = []weightedEntry[*string]{
	{new("en-US"), 380},
	{new("en-GB"), 80},
	{new("de-DE"), 100},
	{new("fr-FR"), 50},
	{new("nl-NL"), 50},
	{new("sv-SE"), 30},
	{new("pt-BR"), 25},
	{new("ja-JP"), 25},
	{new("es-ES"), 20},
	{new("pl-PL"), 20},
	{new("it-IT"), 20},
}

type aiFetchBot struct {
	name      string
	family    string
	userAgent string
}

var aiFetchBots = []weightedEntry[aiFetchBot]{
	{aiFetchBot{name: "GPTBot", family: "OpenAI", userAgent: "Mozilla/5.0 (compatible; GPTBot/1.2; +https://openai.com/gptbot)"}, 32},
	{aiFetchBot{name: "ChatGPT-User", family: "OpenAI", userAgent: "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0; +https://openai.com/bot"}, 18},
	{aiFetchBot{name: "PerplexityBot", family: "Perplexity", userAgent: "Mozilla/5.0 (compatible; PerplexityBot/1.0; +https://www.perplexity.ai/perplexitybot)"}, 22},
	{aiFetchBot{name: "ClaudeBot", family: "Anthropic", userAgent: "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://www.anthropic.com/bot)"}, 18},
	{aiFetchBot{name: "Google-Extended", family: "Google", userAgent: "Mozilla/5.0 (compatible; Google-Extended; +http://www.google.com/bot.html)"}, 16},
	{aiFetchBot{name: "DeepSeek", family: "DeepSeek", userAgent: "Mozilla/5.0 (compatible; DeepSeekBot/1.0; +https://deepseek.com/bot)"}, 9},
}

type aiFetchTarget struct {
	path        string
	contentType string
	statusCode  int
	responseMin int
	responseMax int
	bytesMin    int64
	bytesMax    int64
	visitChance float64
	visitMin    int
	visitMax    int
}

var aiFetchTargets = []weightedEntry[aiFetchTarget]{
	{aiFetchTarget{path: "/", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 90, responseMax: 260, bytesMin: 21_000, bytesMax: 54_000, visitChance: 0.14, visitMin: 1, visitMax: 2}, 34},
	{aiFetchTarget{path: "/pricing", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 110, responseMax: 290, bytesMin: 24_000, bytesMax: 60_000, visitChance: 0.34, visitMin: 1, visitMax: 3}, 26},
	{aiFetchTarget{path: "/features", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 100, responseMax: 260, bytesMin: 22_000, bytesMax: 52_000, visitChance: 0.24, visitMin: 1, visitMax: 2}, 20},
	{aiFetchTarget{path: "/blog/privacy-first-analytics-2025", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 120, responseMax: 320, bytesMin: 19_000, bytesMax: 45_000, visitChance: 0.22, visitMin: 1, visitMax: 2}, 18},
	{aiFetchTarget{path: "/blog/replace-google-analytics", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 120, responseMax: 320, bytesMin: 18_000, bytesMax: 43_000, visitChance: 0.26, visitMin: 1, visitMax: 2}, 18},
	{aiFetchTarget{path: "/docs/getting-started", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 80, responseMax: 220, bytesMin: 15_000, bytesMax: 36_000, visitChance: 0.42, visitMin: 1, visitMax: 4}, 28},
	{aiFetchTarget{path: "/docs/configuration", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 85, responseMax: 240, bytesMin: 17_000, bytesMax: 39_000, visitChance: 0.34, visitMin: 1, visitMax: 3}, 24},
	{aiFetchTarget{path: "/docs/api-reference", contentType: "text/html; charset=utf-8", statusCode: 200, responseMin: 140, responseMax: 360, bytesMin: 27_000, bytesMax: 74_000, visitChance: 0.3, visitMin: 1, visitMax: 3}, 20},
	{aiFetchTarget{path: "/docs/api-reference/openapi.json", contentType: "application/json", statusCode: 200, responseMin: 70, responseMax: 180, bytesMin: 70_000, bytesMax: 180_000, visitChance: 0.02, visitMin: 1, visitMax: 1}, 12},
	{aiFetchTarget{path: "/guides/ai-visibility.pdf", contentType: "application/pdf", statusCode: 200, responseMin: 150, responseMax: 420, bytesMin: 240_000, bytesMax: 860_000, visitChance: 0.06, visitMin: 1, visitMax: 1}, 8},
	{aiFetchTarget{path: "/assets/architecture-diagram.png", contentType: "image/png", statusCode: 200, responseMin: 45, responseMax: 120, bytesMin: 42_000, bytesMax: 280_000}, 6},
	{aiFetchTarget{path: "/docs/legacy-sdk", contentType: "text/html; charset=utf-8", statusCode: 404, responseMin: 80, responseMax: 160, bytesMin: 4_000, bytesMax: 12_000}, 5},
	{aiFetchTarget{path: "/blog/ai-overview", contentType: "text/html; charset=utf-8", statusCode: 404, responseMin: 75, responseMax: 150, bytesMin: 3_500, bytesMax: 10_000}, 4},
	{aiFetchTarget{path: "/api/assistant-index", contentType: "application/json", statusCode: 503, responseMin: 300, responseMax: 1_300, bytesMin: 1_000, bytesMax: 4_000}, 3},
}

type utmParams struct {
	source, medium, campaign string
	term, content            *string
}

type ecommerceProduct struct {
	itemID    string
	itemName  string
	plan      string
	category  string
	price     int
	priceYear int
}

type chatbotBot struct {
	botID    string
	provider string
	model    string
}

type autoTrackedOutboundTarget struct {
	host     string
	path     string
	protocol string
}

type autoTrackedDownloadTarget struct {
	host string
	path string
	ext  string
}

type autoTrackedFormTarget struct {
	host       string
	path       string
	method     string
	sameOrigin bool
	formID     string
}

var utmCampaigns = []weightedEntry[*utmParams]{
	{nil, 800},
	{&utmParams{
		source: "twitter", medium: "social", campaign: "product-launch-2025",
	}, 40},
	{&utmParams{
		source: "google", medium: "cpc", campaign: "self-hosted-analytics",
		term: new("self hosted analytics"), content: new("headline-a"),
	}, 35},
	{&utmParams{
		source: "google", medium: "cpc", campaign: "self-hosted-analytics",
		term: new("open source analytics"), content: new("headline-b"),
	}, 25},
	{&utmParams{
		source: "newsletter", medium: "email", campaign: "monthly-digest-jan",
	}, 30},
	{&utmParams{
		source: "newsletter", medium: "email", campaign: "monthly-digest-feb",
	}, 30},
	{&utmParams{
		source: "reddit", medium: "social", campaign: "selfhosted-post",
	}, 20},
	{&utmParams{
		source: "linkedin", medium: "social", campaign: "b2b-outreach",
	}, 20},
	{&utmParams{
		source: "producthunt", medium: "referral", campaign: "ph-launch-day",
	}, 15},
	{&utmParams{
		source: "devto", medium: "content", campaign: "tutorial-series",
	}, 15},
}

var ecommerceProducts = []weightedEntry[ecommerceProduct]{
	{ecommerceProduct{itemID: "starter-plan", itemName: "Starter Plan", plan: "starter", category: "subscription", price: 29, priceYear: 290}, 34},
	{ecommerceProduct{itemID: "pro-plan", itemName: "Pro Plan", plan: "pro", category: "subscription", price: 79, priceYear: 790}, 36},
	{ecommerceProduct{itemID: "business-plan", itemName: "Business Plan", plan: "business", category: "subscription", price: 199, priceYear: 1990}, 20},
	{ecommerceProduct{itemID: "team-seat-pack", itemName: "Team Seat Pack", plan: "business", category: "add-on", price: 49, priceYear: 490}, 7},
	{ecommerceProduct{itemID: "annual-upgrade", itemName: "Annual Upgrade", plan: "pro", category: "upgrade", price: 199, priceYear: 199}, 3},
}

var chatbotBots = []weightedEntry[chatbotBot]{
	{chatbotBot{botID: "support-copilot", provider: "openai", model: "gpt-4.1-mini"}, 42},
	{chatbotBot{botID: "docs-guide", provider: "anthropic", model: "claude-3-7-sonnet"}, 28},
	{chatbotBot{botID: "sales-assistant", provider: "google", model: "gemini-2.0-flash"}, 18},
	{chatbotBot{botID: "checkout-helper", provider: "openai", model: "gpt-4.1"}, 12},
}

var outboundTargets = []weightedEntry[autoTrackedOutboundTarget]{
	{autoTrackedOutboundTarget{host: "github.com", path: "/hitkeep/hitkeep", protocol: "https"}, 28},
	{autoTrackedOutboundTarget{host: "docs.docker.com", path: "/engine/install", protocol: "https"}, 18},
	{autoTrackedOutboundTarget{host: "vercel.com", path: "/templates", protocol: "https"}, 14},
	{autoTrackedOutboundTarget{host: "stripe.com", path: "/billing", protocol: "https"}, 10},
	{autoTrackedOutboundTarget{host: "status.hitkeep.com", path: "/history", protocol: "https"}, 8},
}

var downloadTargets = []weightedEntry[autoTrackedDownloadTarget]{
	{autoTrackedDownloadTarget{host: "acme-analytics.io", path: "/downloads/hitkeep-pricing.pdf", ext: "pdf"}, 32},
	{autoTrackedDownloadTarget{host: "acme-analytics.io", path: "/downloads/security-checklist.xlsx", ext: "xlsx"}, 18},
	{autoTrackedDownloadTarget{host: "acme-analytics.io", path: "/downloads/customer-stories.csv", ext: "csv"}, 16},
	{autoTrackedDownloadTarget{host: "cdn.acme-analytics.io", path: "/assets/hitkeep-demo.zip", ext: "zip"}, 12},
}

var formTargets = []weightedEntry[autoTrackedFormTarget]{
	{autoTrackedFormTarget{host: "acme-analytics.io", path: "/signup", method: "post", sameOrigin: true, formID: "pricing-signup-form"}, 26},
	{autoTrackedFormTarget{host: "acme-analytics.io", path: "/contact", method: "post", sameOrigin: true, formID: "contact-sales-form"}, 18},
	{autoTrackedFormTarget{host: "go.hitkeep.com", path: "/demo-request", method: "post", sameOrigin: false, formID: "demo-request-form"}, 14},
	{autoTrackedFormTarget{host: "api.hsforms.com", path: "/submissions/v3/integration/submit", method: "post", sameOrigin: false, formID: "newsletter-form"}, 10},
}

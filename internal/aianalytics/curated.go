package aianalytics

// curatedAgents preserves HitKeep's original hand-maintained classifications.
// They are merged into every generated bundle with the highest priority, so an
// upstream removal or rename can never regress existing analytics, and the
// embedded-data validation refuses bundles that lost any of these tokens.
//
//nolint:gosec // Public user-agent tokens used for bot classification, not credentials.
var curatedAgents = []AIAgentEntry{
	{Token: "chatgpt-user", Name: "ChatGPT-User", Family: "OpenAI", Category: CategoryAssistant, Respect: RespectYes, URL: "https://platform.openai.com/docs/bots", Sources: []string{sourceHitkeepCurated}},
	{Token: "gptbot", Name: "GPTBot", Family: "OpenAI", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://platform.openai.com/docs/bots", Sources: []string{sourceHitkeepCurated}},
	{Token: "claudebot", Name: "ClaudeBot", Family: "Anthropic", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://support.anthropic.com/en/articles/8896518", Sources: []string{sourceHitkeepCurated}},
	{Token: "claude-web", Name: "Claude-Web", Family: "Anthropic", Category: CategoryAssistant, Respect: RespectUnclear, URL: "https://support.anthropic.com/en/articles/8896518", Sources: []string{sourceHitkeepCurated}},
	{Token: "perplexitybot", Name: "PerplexityBot", Family: "Perplexity", Category: CategorySearchIndexer, Respect: RespectYes, URL: "https://docs.perplexity.ai/guides/bots", Sources: []string{sourceHitkeepCurated}},
	{Token: "google-extended", Name: "Google-Extended", Family: "Google", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers", Sources: []string{sourceHitkeepCurated}},
	{Token: "googleother", Name: "GoogleOther", Family: "Google", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers", Sources: []string{sourceHitkeepCurated}},
	{Token: "google-safety", Name: "Google-Safety", Family: "Google", Category: CategoryOtherAI, Respect: RespectNo, URL: "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers", Sources: []string{sourceHitkeepCurated}},
	{Token: "applebot-extended", Name: "Applebot-Extended", Family: "Apple", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://support.apple.com/en-us/119829", Sources: []string{sourceHitkeepCurated}},
	{Token: "bytespider", Name: "Bytespider", Family: "ByteDance", Category: CategoryTrainingCrawler, Respect: RespectNo, Sources: []string{sourceHitkeepCurated}},
	{Token: "ccbot", Name: "CCBot", Family: "Common Crawl", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://commoncrawl.org/ccbot", Sources: []string{sourceHitkeepCurated}},
	{Token: "meta-externalagent", Name: "Meta-ExternalAgent", Family: "Meta", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://developers.facebook.com/docs/sharing/webmasters/web-crawlers", Sources: []string{sourceHitkeepCurated}},
	{Token: "meta-externalfetcher", Name: "Meta-ExternalFetcher", Family: "Meta", Category: CategoryAssistant, Respect: RespectNo, URL: "https://developers.facebook.com/docs/sharing/webmasters/web-crawlers", Sources: []string{sourceHitkeepCurated}},
	{Token: "amazonbot", Name: "Amazonbot", Family: "Amazon", Category: CategorySearchIndexer, Respect: RespectYes, URL: "https://developer.amazon.com/amazonbot", Sources: []string{sourceHitkeepCurated}},
	{Token: "cohere-ai", Name: "Cohere", Family: "Cohere", Category: CategoryTrainingCrawler, Respect: RespectUnclear, Sources: []string{sourceHitkeepCurated}},
	{Token: "youbot", Name: "YouBot", Family: "You.com", Category: CategorySearchIndexer, Respect: RespectYes, URL: "https://about.you.com/youbot/", Sources: []string{sourceHitkeepCurated}},
	{Token: "ai2bot", Name: "AI2Bot", Family: "AI2", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://allenai.org/crawler", Sources: []string{sourceHitkeepCurated}},
	{Token: "diffbot", Name: "Diffbot", Family: "Diffbot", Category: CategoryTrainingCrawler, Respect: RespectUnclear, URL: "https://www.diffbot.com/", Sources: []string{sourceHitkeepCurated}},
	{Token: "timpibot", Name: "Timpibot", Family: "Timpi", Category: CategorySearchIndexer, Respect: RespectUnclear, Sources: []string{sourceHitkeepCurated}},
	{Token: "imagesiftbot", Name: "ImagesiftBot", Family: "Imagesift", Category: CategoryTrainingCrawler, Respect: RespectYes, URL: "https://imagesift.com/about", Sources: []string{sourceHitkeepCurated}},
	{Token: "deepseekbot", Name: "DeepSeek", Family: "DeepSeek", Category: CategoryTrainingCrawler, Respect: RespectUnclear, Sources: []string{sourceHitkeepCurated}},
	{Token: "petalbot", Name: "PetalBot", Family: "Petal", Category: CategorySearchIndexer, Respect: RespectYes, URL: "https://webmaster.petalsearch.com/site/petalbot", Sources: []string{sourceHitkeepCurated}},
}

// curatedReferrers pins the AI referral surfaces that ship out of the box.
// The updater merges them into the generated bundle; hosts must stay in sync
// with requiredReferrerNames.
var curatedReferrers = []AIReferrerEntry{
	{Name: "Arc Search", Hosts: []string{"arc.net"}},
	{Name: "ChatGPT", Hosts: []string{"chatgpt.com", "chat.openai.com"}},
	{Name: "Claude", Hosts: []string{"claude.ai"}},
	{Name: "Copilot", Hosts: []string{"copilot.microsoft.com"}},
	{Name: "DeepSeek", Hosts: []string{"chat.deepseek.com"}},
	{Name: "Duck.ai", Hosts: []string{"duck.ai"}},
	{Name: "Gemini", Hosts: []string{"gemini.google.com"}},
	{Name: "Grok", Hosts: []string{"grok.com", "grok.x.ai"}},
	{Name: "HuggingChat", Hosts: []string{"huggingface.co"}, PathContains: "/chat"},
	{Name: "Kagi", Hosts: []string{"kagi.com"}},
	{Name: "Meta AI", Hosts: []string{"meta.ai"}},
	{Name: "Mistral", Hosts: []string{"chat.mistral.ai"}},
	{Name: "Perplexity", Hosts: []string{"perplexity.ai"}},
	{Name: "Phind", Hosts: []string{"phind.com"}},
	{Name: "Poe", Hosts: []string{"poe.com"}},
	{Name: "Qwen", Hosts: []string{"chat.qwen.ai"}},
	{Name: "You.com", Hosts: []string{"you.com"}},
}

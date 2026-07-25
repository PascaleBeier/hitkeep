package aianalytics

import "testing"

func TestClassifyBot(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		wantName  string
		wantGroup string
	}{
		{
			name:      "matches GPTBot",
			userAgent: "Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)",
			wantName:  "GPTBot",
			wantGroup: "OpenAI",
		},
		{
			name:      "matches ClaudeBot",
			userAgent: "Mozilla/5.0 (compatible; ClaudeBot/1.0; +https://anthropic.com/claudebot)",
			wantName:  "ClaudeBot",
			wantGroup: "Anthropic",
		},
		{
			name:      "matches DeepSeekBot",
			userAgent: "Mozilla/5.0 (compatible; DeepSeekBot/1.0; +https://deepseek.com/bot)",
			wantName:  "DeepSeek",
			wantGroup: "DeepSeek",
		},
		{
			name:      "does not overmatch DeepSeekBrowser",
			userAgent: "Mozilla/5.0 (compatible; DeepSeekBrowser/1.0; +https://example.com/bot)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			identity := ClassifyBot(tc.userAgent)
			if tc.wantName == "" {
				if identity != nil {
					t.Fatalf("expected nil identity, got %+v", *identity)
				}
				return
			}
			if identity == nil {
				t.Fatal("expected identity, got nil")
			}
			if identity.Name != tc.wantName || identity.Family != tc.wantGroup {
				t.Fatalf("unexpected identity: got %+v", *identity)
			}
		})
	}
}

func TestClassifyBotCategories(t *testing.T) {
	tests := []struct {
		userAgent    string
		wantName     string
		wantCategory string
	}{
		{
			userAgent:    "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
			wantName:     "GPTBot",
			wantCategory: CategoryTrainingCrawler,
		},
		{
			userAgent:    "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; OAI-SearchBot/1.0; +https://openai.com/searchbot)",
			wantName:     "OAI-SearchBot",
			wantCategory: CategorySearchIndexer,
		},
		{
			userAgent:    "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0; +https://openai.com/bot",
			wantName:     "ChatGPT-User",
			wantCategory: CategoryAssistant,
		},
		{
			userAgent:    "Mozilla/5.0 (compatible; Perplexity-User/1.0; +https://perplexity.ai/perplexity-user)",
			wantCategory: CategoryAssistant,
		},
		{
			userAgent:    "Mozilla/5.0 (compatible; Devin/1.0; +https://www.cognition.ai)",
			wantName:     "Devin",
			wantCategory: CategoryCodingAgent,
		},
	}
	for _, tc := range tests {
		identity := ClassifyBot(tc.userAgent)
		if identity == nil {
			t.Errorf("ClassifyBot(%q) = nil, want category %s", tc.userAgent, tc.wantCategory)
			continue
		}
		if tc.wantName != "" && identity.Name != tc.wantName {
			t.Errorf("ClassifyBot(%q) name = %q, want %q", tc.userAgent, identity.Name, tc.wantName)
		}
		if identity.Category != tc.wantCategory {
			t.Errorf("ClassifyBot(%q) category = %q, want %q", tc.userAgent, identity.Category, tc.wantCategory)
		}
	}
}

// TestClassifyBotDoesNotFlagRegularTraffic guards the widened upstream list
// against tokens that would misclassify everyday browsers and non-AI crawlers.
func TestClassifyBotDoesNotFlagRegularTraffic(t *testing.T) {
	corpus := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 OPR/111.0.0.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Vivaldi/6.7",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"DuckDuckBot/1.1; (+http://duckduckgo.com/duckduckbot.html)",
		"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
		"Twitterbot/1.0",
		"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)",
		"curl/8.6.0",
		"okhttp/4.12.0",
	}
	for _, userAgent := range corpus {
		if identity := ClassifyBot(userAgent); identity != nil {
			t.Errorf("ClassifyBot(%q) = %+v, want nil", userAgent, *identity)
		}
	}
}

func TestClassifyResourceType(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{contentType: "", want: "html"},
		{contentType: "text/html; charset=utf-8", want: "html"},
		{contentType: "application/pdf", want: "document"},
		{contentType: "image/png", want: "image"},
		{contentType: "application/json", want: "other"},
	}

	for _, tc := range tests {
		if got := ClassifyResourceType(tc.contentType); got != tc.want {
			t.Fatalf("ClassifyResourceType(%q) = %q, want %q", tc.contentType, got, tc.want)
		}
	}
}

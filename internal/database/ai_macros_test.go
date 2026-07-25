package database

import (
	"context"
	"path/filepath"
	"testing"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/api"
)

func TestEnsureAIClassificationMacrosClassifiesLikeGo(t *testing.T) {
	ctx := context.Background()
	store, err := OpenMigratedTenantStore(ctx, filepath.Join(t.TempDir(), "tenant.db"))
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	defer store.Close()

	corpus := []string{
		// Curated tokens.
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
		"Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0; +https://openai.com/bot",
		// Tokens that exist only in the merged upstream lists, not in the
		// original 22-entry hand-written macro.
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; OAI-SearchBot/1.0; +https://openai.com/searchbot)",
		"Mozilla/5.0 (compatible; Perplexity-User/1.0; +https://perplexity.ai/perplexity-user)",
		"Mozilla/5.0 (compatible; DuckAssistBot/1.0; +http://duckduckgo.com/duckassistbot.html)",
		"Mozilla/5.0 (compatible; Devin/1.0; +https://www.cognition.ai)",
		// Overlapping tokens must resolve identically (longest first).
		"Mozilla/5.0 (compatible; ai2bot-dolma; +https://www.allenai.org/crawler)",
		// Regular traffic must classify as NULL / nil.
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"curl/8.6.0",
		"",
	}

	for _, userAgent := range corpus {
		var sqlName, sqlCategory, sqlCategoryFromName *string
		row := store.db.QueryRowContext(ctx,
			"SELECT hk_ai_bot(?), hk_ai_bot_category(?), hk_ai_bot_category_from_name(hk_ai_bot(?))",
			userAgent, userAgent, userAgent)
		if err := row.Scan(&sqlName, &sqlCategory, &sqlCategoryFromName); err != nil {
			t.Fatalf("query macros for %q: %v", userAgent, err)
		}

		// The stats query derives the category from the already-resolved agent
		// name to avoid a second pattern walk; both paths must agree.
		switch {
		case sqlCategory == nil && sqlCategoryFromName != nil:
			t.Errorf("UA %q: hk_ai_bot_category is NULL, hk_ai_bot_category_from_name is %q", userAgent, *sqlCategoryFromName)
		case sqlCategory != nil && (sqlCategoryFromName == nil || *sqlCategoryFromName != *sqlCategory):
			t.Errorf("UA %q: hk_ai_bot_category %q, hk_ai_bot_category_from_name %v", userAgent, *sqlCategory, sqlCategoryFromName)
		}

		identity := aianalytics.ClassifyBot(userAgent)
		if identity == nil {
			if sqlName != nil {
				t.Errorf("UA %q: Go says nil, SQL says %q", userAgent, *sqlName)
			}
			if sqlCategory != nil {
				t.Errorf("UA %q: Go says nil, SQL category says %q", userAgent, *sqlCategory)
			}
			continue
		}
		if sqlName == nil || *sqlName != identity.Name {
			t.Errorf("UA %q: Go name %q, SQL name %v", userAgent, identity.Name, sqlName)
		}
		if sqlCategory == nil || *sqlCategory != identity.Category {
			t.Errorf("UA %q: Go category %q, SQL category %v", userAgent, identity.Category, sqlCategory)
		}
	}
}

func TestCreateAIFetchPersistsAssistantCategory(t *testing.T) {
	ctx := context.Background()
	store, err := OpenMigratedStore(ctx, filepath.Join(t.TempDir(), "shared.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	userID, err := store.CreateUser(ctx, "ai-category@example.com", "hashed")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "ai-category.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	fetch := &api.AIFetch{
		SiteID:            site.ID,
		AssistantName:     "GPTBot",
		AssistantFamily:   "OpenAI",
		AssistantCategory: aianalytics.CategoryTrainingCrawler,
		Path:              "/docs",
		StatusCode:        200,
		ResourceType:      "html",
	}
	if err := store.CreateAIFetch(ctx, fetch); err != nil {
		t.Fatalf("CreateAIFetch: %v", err)
	}

	var category *string
	if err := store.db.QueryRowContext(ctx, "SELECT assistant_category FROM ai_fetches WHERE site_id = ?", site.ID).Scan(&category); err != nil {
		t.Fatalf("read assistant_category: %v", err)
	}
	if category == nil || *category != aianalytics.CategoryTrainingCrawler {
		t.Fatalf("assistant_category = %v, want %q", category, aianalytics.CategoryTrainingCrawler)
	}
}

func TestEnsureAIClassificationMacrosSourceMacro(t *testing.T) {
	ctx := context.Background()
	store, err := OpenMigratedTenantStore(ctx, filepath.Join(t.TempDir(), "tenant.db"))
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	defer store.Close()

	tests := []struct {
		referrer string
		want     *string
	}{
		{referrer: "https://chatgpt.com/", want: new("ChatGPT")},
		{referrer: "https://www.chatgpt.com/some/path", want: new("ChatGPT")},
		{referrer: "chat.openai.com", want: new("ChatGPT")},
		{referrer: "https://perplexity.ai/search", want: new("Perplexity")},
		{referrer: "https://huggingface.co/chat/conversation", want: new("HuggingChat")},
		{referrer: "https://huggingface.co/models", want: nil},
		{referrer: "https://grok.com/", want: new("Grok")},
		{referrer: "https://duck.ai/", want: new("Duck.ai")},
		{referrer: "https://duckduckgo.com/?q=test", want: nil},
		{referrer: "https://www.google.com/", want: nil},
		{referrer: "", want: nil},
	}
	for _, tc := range tests {
		var got *string
		if err := store.db.QueryRowContext(ctx, "SELECT hk_ai_source(?)", tc.referrer).Scan(&got); err != nil {
			t.Fatalf("query hk_ai_source(%q): %v", tc.referrer, err)
		}
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("hk_ai_source(%q) = %q, want NULL", tc.referrer, *got)
		case tc.want != nil && (got == nil || *got != *tc.want):
			t.Errorf("hk_ai_source(%q) = %v, want %q", tc.referrer, got, *tc.want)
		}
	}
}

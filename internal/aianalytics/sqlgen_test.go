package aianalytics

import (
	"strings"
	"testing"
	"time"
)

func sqlgenTestData() AIAgentData {
	data := AIAgentData{
		GeneratedAt: time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
		Agents: []AIAgentEntry{
			{Token: "gptbot", Name: "GPTBot", Family: "OpenAI", Category: CategoryTrainingCrawler, Sources: []string{sourceHitkeepCurated}},
			{Token: "chatgpt-user", Name: "ChatGPT-User", Family: "OpenAI", Category: CategoryAssistant, Sources: []string{sourceHitkeepCurated}},
			{Token: "ias_crawler", Name: "IAS Crawler", Family: "IAS", Category: CategoryOtherAI, Sources: []string{sourceCrawlerUserAgents}},
			{Token: "o'neil-bot", Name: "O'Neil Bot", Family: "O'Neil", Category: CategoryOtherAI, Sources: []string{sourceCrawlerUserAgents}},
		},
		AIReferrers: []AIReferrerEntry{
			{Name: "ChatGPT", Hosts: []string{"chat.openai.com", "chatgpt.com"}},
			{Name: "HuggingChat", Hosts: []string{"huggingface.co"}, PathContains: "/chat"},
		},
	}
	data.normalize()
	return data
}

func TestAIClassificationMacroStatements(t *testing.T) {
	statements := AIClassificationMacroStatements(sqlgenTestData())
	if len(statements) != 7 {
		t.Fatalf("expected 7 statements, got %d", len(statements))
	}
	joined := strings.Join(statements, "\n---\n")

	for _, macro := range []string{"hk_ai_bot_lc(lua)", "hk_ai_bot(ua)", "hk_ai_bot_category_lc(lua)", "hk_ai_bot_category(ua)", "hk_ai_bot_category_from_name(name)", "hk_ai_source_from_host(h, lref)", "hk_ai_source(ref)"} {
		if !strings.Contains(joined, "CREATE OR REPLACE MACRO "+macro) {
			t.Errorf("missing macro definition for %s", macro)
		}
	}

	botMacro := statements[0]
	// Longest token first: chatgpt-user (12) before ias_crawler (11) before
	// o'neil-bot (10) before gptbot (6).
	order := []string{"chatgpt-user", "ias|_crawler", "o''neil-bot", "gptbot"}
	lastIndex := -1
	for _, token := range order {
		idx := strings.Index(botMacro, token)
		if idx < 0 {
			t.Fatalf("bot macro is missing token %q:\n%s", token, botMacro)
		}
		if idx < lastIndex {
			t.Fatalf("token %q is out of longest-first order:\n%s", token, botMacro)
		}
		lastIndex = idx
	}

	if !strings.Contains(botMacro, "LIKE '%ias|_crawler%' ESCAPE '|'") {
		t.Errorf("underscore token must be LIKE-escaped:\n%s", botMacro)
	}
	if !strings.Contains(botMacro, "THEN 'O''Neil Bot'") {
		t.Errorf("names must be SQL-escaped:\n%s", botMacro)
	}

	categoryMacro := statements[2]
	if !strings.Contains(categoryMacro, "THEN '"+CategoryAssistant+"'") {
		t.Errorf("category macro must return category identifiers:\n%s", categoryMacro)
	}

	categoryFromNameMacro := statements[4]
	if !strings.Contains(categoryFromNameMacro, "WHEN name = 'ChatGPT-User' THEN '"+CategoryAssistant+"'") {
		t.Errorf("category-from-name macro must map agent names to categories:\n%s", categoryFromNameMacro)
	}
	if !strings.Contains(categoryFromNameMacro, "WHEN name = 'O''Neil Bot' THEN") {
		t.Errorf("category-from-name macro must SQL-escape names:\n%s", categoryFromNameMacro)
	}
	if strings.Count(categoryFromNameMacro, "WHEN name = ") != 4 {
		t.Errorf("category-from-name macro must emit one branch per unique name:\n%s", categoryFromNameMacro)
	}

	sourceMacro := statements[5]
	pathRule := strings.Index(sourceMacro, "'%/chat%'")
	plainRule := strings.Index(sourceMacro, "'chatgpt.com'")
	if pathRule < 0 || plainRule < 0 {
		t.Fatalf("source macro is missing rules:\n%s", sourceMacro)
	}
	if pathRule > plainRule {
		t.Errorf("path-conditional referrer rules must come first:\n%s", sourceMacro)
	}

	// Determinism.
	second := AIClassificationMacroStatements(sqlgenTestData())
	for i := range statements {
		if statements[i] != second[i] {
			t.Fatalf("statement %d is not deterministic", i)
		}
	}
}

func TestEmbeddedAIClassificationMacroStatements(t *testing.T) {
	statements := EmbeddedAIClassificationMacroStatements()
	if len(statements) != 7 {
		t.Fatalf("expected 7 statements, got %d", len(statements))
	}
	for _, required := range []string{"%gptbot%", "%claudebot%", "%perplexitybot%"} {
		if !strings.Contains(statements[0], required) {
			t.Errorf("embedded bot macro is missing %q", required)
		}
	}
}

package aianalytics

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeFeedClient struct {
	files          map[string]string
	failures       map[string]bool
	providerErrors map[string]error
}

type feedDoerFunc func(*http.Request) (*http.Response, error)

func (f feedDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type closeTrackingBody struct {
	io.Reader
	closes int
}

func (b *closeTrackingBody) Close() error {
	b.closes++
	return nil
}

func (c *fakeFeedClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if err, ok := c.providerErrors[url]; ok {
		return nil, err
	}
	if c.failures[url] {
		return &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: http.NoBody}, nil
	}
	path, ok := c.files[url]
	if !ok {
		return nil, fmt.Errorf("unexpected URL %s", url)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
}

func newFakeFeedClient() *fakeFeedClient {
	return &fakeFeedClient{
		files: map[string]string{
			aiRobotsTxtURL:       "testdata/ai_robots_txt_fixture.json",
			deviceDetectorURL:    "testdata/device_detector_bots_fixture.yml",
			crawlerUserAgentsURL: "testdata/crawler_user_agents_fixture.json",
		},
		failures:       map[string]bool{},
		providerErrors: map[string]error{},
	}
}

func testAIAgentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFetchAgentFeedBoundsAndClosesSuccessfulBody(t *testing.T) {
	body := &closeTrackingBody{Reader: strings.NewReader(strings.Repeat("x", int(maxAgentFeedResponseBytes)+1))}
	feed, err := fetchAgentFeed(context.Background(), feedDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	}), aiRobotsTxtURL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	contents, err := io.ReadAll(feed)
	if err != nil {
		t.Fatalf("read limited body: %v", err)
	}
	if got, want := len(contents), maxAgentFeedResponseBytes; got != want {
		t.Fatalf("limited body length = %d, want %d", got, want)
	}
	if body.closes != 0 {
		t.Fatalf("body closed before caller closes returned feed: %d", body.closes)
	}
	if err := feed.Close(); err != nil {
		t.Fatalf("close returned feed: %v", err)
	}
	if body.closes != 1 {
		t.Fatalf("underlying body close count = %d, want 1", body.closes)
	}
}

func TestFetchAgentFeedClosesNonSuccessBody(t *testing.T) {
	body := &closeTrackingBody{Reader: strings.NewReader("failure")}
	_, err := fetchAgentFeed(context.Background(), feedDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: body}, nil
	}), aiRobotsTxtURL)
	if err == nil {
		t.Fatal("expected non-success status error")
	}
	if body.closes != 1 {
		t.Fatalf("non-success body close count = %d, want 1", body.closes)
	}
}

func findAgent(t *testing.T, data AIAgentData, token string) AIAgentEntry {
	t.Helper()
	for _, agent := range data.Agents {
		if agent.Token == token {
			return agent
		}
	}
	t.Fatalf("agent %q not found", token)
	return AIAgentEntry{}
}

func hasAgent(data AIAgentData, token string) bool {
	for _, agent := range data.Agents {
		if agent.Token == token {
			return true
		}
	}
	return false
}

func TestFetchAIAgentDataMergesAllSources(t *testing.T) {
	data, err := FetchAIAgentData(context.Background(), newFakeFeedClient(), testAIAgentLogger())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if data.GeneratedAt.IsZero() {
		t.Fatal("generated_at must be set")
	}
	for _, source := range []string{sourceHitkeepCurated, sourceAIRobotsTxt, sourceDeviceDetector, sourceCrawlerUserAgents} {
		if strings.TrimSpace(data.Sources[source]) == "" {
			t.Fatalf("missing source URL for %q", source)
		}
	}
	if data.SourceMetadata[sourceDeviceDetector].License == "" {
		t.Fatal("device detector metadata must carry its LGPL license")
	}

	// Curated entries always survive and win field conflicts.
	gptbot := findAgent(t, data, "gptbot")
	if gptbot.Category != CategoryTrainingCrawler || gptbot.Family != "OpenAI" {
		t.Fatalf("curated fields must win for gptbot, got %+v", gptbot)
	}
	if len(gptbot.Sources) != 2 || gptbot.Sources[0] != sourceAIRobotsTxt || gptbot.Sources[1] != sourceHitkeepCurated {
		t.Fatalf("gptbot sources must union curated + robots.txt, got %v", gptbot.Sources)
	}

	// robots.json category and operator mapping.
	tests := []struct {
		token    string
		category string
		family   string
		respect  string
	}{
		{token: "novaact", category: CategoryAgent, family: "NovaAct", respect: RespectUnclear},
		{token: "devin", category: CategoryCodingAgent, family: "Cognition", respect: RespectYes},
		{token: "quillbot", category: CategoryAgent, family: "Quillbot", respect: RespectUnclear},
		{token: "oai-searchbot", category: CategorySearchIndexer, family: "OpenAI", respect: RespectYes},
		{token: "kangaroo bot", category: CategoryTrainingCrawler, family: "Kangaroo Bot", respect: RespectUnclear},
		{token: "google-cloudvertexbot", category: CategoryTrainingCrawler, family: "Google Inc.", respect: RespectUnclear},
		{token: "duckassistbot", category: CategoryAssistant, family: "DuckDuckGo", respect: RespectUnclear},
		{token: "operator-agent/", category: CategoryAgent, family: "OpenAI", respect: RespectUnclear},
		{token: "mistralai-user/", category: CategoryOtherAI, family: "MistralAI-User", respect: RespectUnclear},
		{token: "ai2bot-dolma", category: CategoryOtherAI, family: "Ai2Bot-Dolma", respect: RespectUnclear},
	}
	for _, tc := range tests {
		agent := findAgent(t, data, tc.token)
		if agent.Category != tc.category {
			t.Errorf("agent %q category = %q, want %q", tc.token, agent.Category, tc.category)
		}
		if agent.Family != tc.family {
			t.Errorf("agent %q family = %q, want %q", tc.token, agent.Family, tc.family)
		}
		if agent.Respect != tc.respect {
			t.Errorf("agent %q respect = %q, want %q", tc.token, agent.Respect, tc.respect)
		}
	}

	// URLs come from the operator markdown link (robots.json), the entry or
	// producer URL (device detector), or the entry URL (crawler-user-agents),
	// so the dashboard can derive favicon hosts without curation.
	urlTests := map[string]string{
		"devin":                 "https://www.cognition.ai",
		"google-cloudvertexbot": "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers",
		"duckassistbot":         "https://duckduckgo.com/",
		"mistralai-user/":       "https://docs.mistral.ai/robots/",
	}
	for token, wantURL := range urlTests {
		if agent := findAgent(t, data, token); agent.URL != wantURL {
			t.Errorf("agent %q URL = %q, want %q", token, agent.URL, wantURL)
		}
	}
	// A curated entry without a URL adopts the upstream URL for the same token.
	if agent := findAgent(t, data, "gptbot"); agent.URL == "" {
		t.Error("curated gptbot should keep or adopt a URL")
	}

	// Non-AI entries, non-literal regexes, and denylisted tokens are dropped.
	for _, token := range []string{"monitoring360bot", "googlebot/", "bot", "anthropic[- ]ai(?:/[0-9])?"} {
		if hasAgent(data, token) {
			t.Errorf("token %q must not be admitted", token)
		}
	}

	// Referrers ship from the curated set.
	if len(data.AIReferrers) < len(requiredReferrerNames) {
		t.Fatalf("expected at least %d referrers, got %d", len(requiredReferrerNames), len(data.AIReferrers))
	}
}

func TestFetchAIAgentDataToleratesSecondaryFailures(t *testing.T) {
	client := newFakeFeedClient()
	client.failures[deviceDetectorURL] = true
	client.failures[crawlerUserAgentsURL] = true

	data, err := FetchAIAgentData(context.Background(), client, testAIAgentLogger())
	if err != nil {
		t.Fatalf("secondary feed failures must not abort the fetch: %v", err)
	}
	if !hasAgent(data, "devin") {
		t.Fatal("primary source data missing")
	}
	if hasAgent(data, "duckassistbot") {
		t.Fatal("failed feed data should be absent")
	}
	if !hasAgent(data, "claudebot") {
		t.Fatal("curated agents must always be present")
	}
}

func TestFetchAIAgentDataDoesNotLogRawFailureDetails(t *testing.T) {
	const rawFailure = "provider response password=do-not-log"
	client := newFakeFeedClient()
	client.providerErrors[deviceDetectorURL] = fmt.Errorf("upstream returned %s", rawFailure)
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if _, err := FetchAIAgentData(context.Background(), client, logger); err != nil {
		t.Fatalf("partial failure should not return error: %v", err)
	}
	if strings.Contains(logs.String(), rawFailure) {
		t.Fatalf("raw upstream failure appeared in logs: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "source=device_detector_bots") || !strings.Contains(logs.String(), "error_kind=invalid_response") {
		t.Fatalf("expected safe source and error kind in logs: %s", logs.String())
	}
}

func TestFetchAIAgentDataFailsWhenPrimarySourceIsEmpty(t *testing.T) {
	const rawFailure = "provider response password=do-not-return"
	client := newFakeFeedClient()
	client.providerErrors[aiRobotsTxtURL] = fmt.Errorf("upstream returned %s", rawFailure)

	_, err := FetchAIAgentData(context.Background(), client, testAIAgentLogger())
	if err == nil {
		t.Fatal("expected error when the primary source fails")
	}
	if strings.Contains(err.Error(), rawFailure) {
		t.Fatalf("raw upstream failure appeared in returned error: %v", err)
	}
}

func TestLiteralRegexToken(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		ok      bool
	}{
		{pattern: "GPTBot", want: "GPTBot", ok: true},
		{pattern: "Googlebot\\/", want: "Googlebot/", ok: true},
		{pattern: "AddThis\\.com", want: "AddThis.com", ok: true},
		{pattern: "Ai2Bot\\-Dolma", want: "Ai2Bot-Dolma", ok: true},
		{pattern: "(360Spider(?:-Image|-Video)?)", ok: false},
		{pattern: "anthropic[- ]ai", ok: false},
		{pattern: "bot|crawler", ok: false},
		{pattern: "^curl", ok: false},
		{pattern: "trailing\\", ok: false},
	}
	for _, tc := range tests {
		got, ok := literalRegexToken(tc.pattern)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("literalRegexToken(%q) = %q, %v; want %q, %v", tc.pattern, got, ok, tc.want, tc.ok)
		}
	}
}

package aianalytics

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

const maxAgentFeedResponseBytes = 10 << 20 // 10 MB

// Upstream lists merged into the embedded bundle. ai.robots.txt is the
// primary source; the others extend coverage and cross-validate it.
// device-detector data is LGPL-3.0-or-later and attributed in NOTICE.
const (
	aiRobotsTxtURL       = "https://raw.githubusercontent.com/ai-robots-txt/ai.robots.txt/main/robots.json"
	deviceDetectorURL    = "https://raw.githubusercontent.com/matomo-org/device-detector/master/regexes/bots.yml"
	crawlerUserAgentsURL = "https://raw.githubusercontent.com/monperrus/crawler-user-agents/master/crawler-user-agents.json"
)

// Merge priority per source: lower rank wins field conflicts for the same
// token. Curated entries protect HitKeep's established names and categories.
var sourceRank = map[string]int{
	sourceHitkeepCurated:    0,
	sourceAIRobotsTxt:       1,
	sourceDeviceDetector:    2,
	sourceCrawlerUserAgents: 3,
}

type agentHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type aiFeedWarning struct {
	source string
	err    error
}

// FetchAIAgentData assembles the AI agent master list from the upstream
// sources plus the curated overlay. Secondary source failures degrade to
// warnings; only a failing primary source aborts the fetch.
func FetchAIAgentData(ctx context.Context, client agentHTTPDoer, logger *slog.Logger) (AIAgentData, error) {
	if logger == nil {
		panic("aianalytics: logger is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var warnings []aiFeedWarning

	primary, err := fetchAIRobotsTxtAgents(ctx, client)
	if err != nil {
		return AIAgentData{}, fmt.Errorf("primary AI agent source failed: %s", aiFeedErrorKind(err))
	}
	if len(primary) == 0 {
		return AIAgentData{}, fmt.Errorf("primary AI agent source %s returned no usable entries", aiRobotsTxtURL)
	}

	deviceDetector, err := fetchDeviceDetectorAgents(ctx, client)
	if err != nil {
		warnings = append(warnings, aiFeedWarning{source: "device_detector_bots", err: err})
	}
	crawlerAgents, err := fetchCrawlerUserAgents(ctx, client)
	if err != nil {
		warnings = append(warnings, aiFeedWarning{source: "crawler_user_agents", err: err})
	}
	for _, warning := range warnings {
		logger.Warn("Partial AI agent feed failure, continuing with available data", "source", warning.source, "error_kind", aiFeedErrorKind(warning.err))
	}

	merged := mergeAgentEntries(slices.Concat(curatedAgents, primary, deviceDetector, crawlerAgents))

	referrers := make([]AIReferrerEntry, len(curatedReferrers))
	copy(referrers, curatedReferrers)

	data := AIAgentData{
		GeneratedAt: time.Now().UTC(),
		Sources: map[string]string{
			sourceHitkeepCurated:    "embedded",
			sourceAIRobotsTxt:       aiRobotsTxtURL,
			sourceDeviceDetector:    deviceDetectorURL,
			sourceCrawlerUserAgents: crawlerUserAgentsURL,
		},
		SourceMetadata: map[string]AIAgentSourceMetadata{
			sourceAIRobotsTxt: {
				License: "MIT",
				URL:     "https://github.com/ai-robots-txt/ai.robots.txt",
			},
			sourceDeviceDetector: {
				License:     "LGPL-3.0-or-later",
				URL:         "https://github.com/matomo-org/device-detector",
				Attribution: "Bot list data derived from Device Detector, (c) Matomo (https://matomo.org)",
			},
			sourceCrawlerUserAgents: {
				License: "MIT",
				URL:     "https://github.com/monperrus/crawler-user-agents",
			},
		},
		Agents:      merged,
		AIReferrers: referrers,
	}
	data.normalize()
	return data, nil
}

func aiFeedErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		var networkErr net.Error
		if errors.As(err, &networkErr) {
			if networkErr.Timeout() {
				return "timeout"
			}
			return "network"
		}
		return "invalid_response"
	}
}

func mergeAgentEntries(entries []AIAgentEntry) []AIAgentEntry {
	byToken := make(map[string]AIAgentEntry, len(entries))
	for _, entry := range entries {
		entry.Token = strings.ToLower(strings.TrimSpace(entry.Token))
		existing, ok := byToken[entry.Token]
		if !ok {
			byToken[entry.Token] = entry
			continue
		}
		merged := existing
		other := entry
		if rankOf(entry) < rankOf(existing) {
			merged = entry
			other = existing
		}
		// URL merges field-level: any source's URL beats none, so favicon
		// hosts stay derivable even when the winning source lacks a URL.
		if merged.URL == "" {
			merged.URL = other.URL
		}
		merged.Sources = normalizeTokenList(append(append([]string(nil), existing.Sources...), entry.Sources...), false)
		byToken[entry.Token] = merged
	}

	out := make([]AIAgentEntry, 0, len(byToken))
	for _, entry := range byToken {
		out = append(out, entry)
	}
	return out
}

func rankOf(entry AIAgentEntry) int {
	best := len(sourceRank)
	for _, source := range entry.Sources {
		if rank, ok := sourceRank[source]; ok && rank < best {
			best = rank
		}
	}
	return best
}

// admitToken applies the same hygiene rules as the embedded-data validation so
// rejected upstream entries never reach the bundle in the first place.
func admitToken(token string) bool {
	return validateToken(token) == nil
}

type aiRobotsTxtEntry struct {
	Operator string `json:"operator"`
	Respect  string `json:"respect"`
	Function string `json:"function"`
}

func fetchAIRobotsTxtAgents(ctx context.Context, client agentHTTPDoer) ([]AIAgentEntry, error) {
	body, err := fetchAgentFeed(ctx, client, aiRobotsTxtURL)
	if err != nil {
		return nil, fmt.Errorf("fetch ai.robots.txt list: %w", err)
	}
	defer body.Close()

	var raw map[string]aiRobotsTxtEntry
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode ai.robots.txt list: %w", err)
	}

	out := make([]AIAgentEntry, 0, len(raw))
	for name, entry := range raw {
		token := strings.ToLower(strings.TrimSpace(name))
		if !admitToken(token) {
			continue
		}
		family := markdownLinkText(entry.Operator)
		if family == "" {
			family = strings.TrimSpace(name)
		}
		out = append(out, AIAgentEntry{
			Token:    token,
			Name:     strings.TrimSpace(name),
			Family:   family,
			Category: categoryFromAIRobotsTxtFunction(entry.Function),
			Respect:  respectFromUpstream(entry.Respect),
			URL:      markdownLinkURL(entry.Operator),
			Sources:  []string{sourceAIRobotsTxt},
		})
	}
	return out, nil
}

func categoryFromAIRobotsTxtFunction(function string) string {
	normalized := strings.ToLower(strings.TrimSpace(function))
	switch {
	case strings.Contains(normalized, "coding"):
		return CategoryCodingAgent
	case strings.Contains(normalized, "ai agents"):
		return CategoryAgent
	case strings.Contains(normalized, "ai assistants"):
		return CategoryAssistant
	case strings.Contains(normalized, "search"):
		return CategorySearchIndexer
	case strings.Contains(normalized, "scraper"), strings.Contains(normalized, "scrapes"),
		strings.Contains(normalized, "training"), strings.Contains(normalized, "train"):
		return CategoryTrainingCrawler
	default:
		return CategoryOtherAI
	}
}

// markdownLinkURL extracts the link target from values like
// "[OpenAI](https://openai.com)"; non-link and placeholder values yield "".
func markdownLinkURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "[") {
		return ""
	}
	start := strings.Index(trimmed, "](")
	if start < 0 || !strings.HasSuffix(trimmed, ")") {
		return ""
	}
	target := strings.TrimSpace(trimmed[start+2 : len(trimmed)-1])
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return ""
	}
	return target
}

// markdownLinkText extracts the link text from values like
// "[OpenAI](https://openai.com)" and blanks placeholder values like
// "Unclear at this time.".
func markdownLinkText(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.Index(trimmed, "]("); end > 1 {
			trimmed = trimmed[1:end]
		}
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "unclear") {
		return ""
	}
	return strings.TrimSpace(trimmed)
}

func respectFromUpstream(value string) string {
	switch strings.ToLower(markdownLinkText(value)) {
	case "yes":
		return RespectYes
	case "no":
		return RespectNo
	default:
		return RespectUnclear
	}
}

var deviceDetectorCategories = map[string]string{
	"AI Data Scraper":   CategoryTrainingCrawler,
	"AI Search Crawler": CategorySearchIndexer,
	"AI Assistant":      CategoryAssistant,
	"AI Agent":          CategoryAgent,
}

type deviceDetectorBot struct {
	regex        string
	name         string
	category     string
	url          string
	producerName string
	producerURL  string
}

// fetchDeviceDetectorAgents parses device-detector's machine-generated
// bots.yml with a purpose-built reader instead of adding a YAML dependency to
// the production binary. Only entries in AI categories with literal regexes
// are admitted, so unexpected formatting degrades to fewer entries, which the
// embedded-data validation then catches.
func fetchDeviceDetectorAgents(ctx context.Context, client agentHTTPDoer) ([]AIAgentEntry, error) {
	body, err := fetchAgentFeed(ctx, client, deviceDetectorURL)
	if err != nil {
		return nil, fmt.Errorf("fetch device detector bots: %w", err)
	}
	defer body.Close()

	var (
		bots       []deviceDetectorBot
		current    deviceDetectorBot
		inEntry    bool
		inProducer bool
	)
	flush := func() {
		if inEntry {
			bots = append(bots, current)
		}
		current = deviceDetectorBot{}
		inEntry = false
		inProducer = false
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#") || trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "- regex:"):
			flush()
			inEntry = true
			current.regex = unquoteYAMLScalar(strings.TrimPrefix(trimmed, "- regex:"))
		case !inEntry:
			continue
		case strings.HasPrefix(trimmed, "producer:"):
			inProducer = true
		case strings.HasPrefix(trimmed, "name:"):
			value := unquoteYAMLScalar(strings.TrimPrefix(trimmed, "name:"))
			if inProducer {
				current.producerName = value
			} else {
				current.name = value
			}
		case strings.HasPrefix(trimmed, "url:"):
			value := unquoteYAMLScalar(strings.TrimPrefix(trimmed, "url:"))
			if inProducer {
				current.producerURL = value
			} else {
				current.url = value
			}
		case strings.HasPrefix(trimmed, "category:"):
			current.category = unquoteYAMLScalar(strings.TrimPrefix(trimmed, "category:"))
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan device detector bots: %w", err)
	}
	if len(bots) == 0 {
		return nil, fmt.Errorf("device detector bots list contained no entries")
	}

	out := make([]AIAgentEntry, 0, len(bots))
	for _, bot := range bots {
		category, isAI := deviceDetectorCategories[bot.category]
		if !isAI {
			continue
		}
		literal, ok := literalRegexToken(bot.regex)
		if !ok {
			continue
		}
		token := strings.ToLower(literal)
		if !admitToken(token) {
			continue
		}
		name := strings.TrimSpace(bot.name)
		if name == "" || strings.Contains(name, "$") {
			continue
		}
		family := strings.TrimSpace(bot.producerName)
		if family == "" {
			family = name
		}
		botURL := strings.TrimSpace(bot.url)
		if botURL == "" {
			botURL = strings.TrimSpace(bot.producerURL)
		}
		out = append(out, AIAgentEntry{
			Token:    token,
			Name:     name,
			Family:   family,
			Category: category,
			Respect:  RespectUnclear,
			URL:      botURL,
			Sources:  []string{sourceDeviceDetector},
		})
	}
	return out, nil
}

func unquoteYAMLScalar(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'' {
		return strings.ReplaceAll(trimmed[1:len(trimmed)-1], "''", "'")
	}
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		inner := trimmed[1 : len(trimmed)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		return strings.ReplaceAll(inner, `\\`, `\`)
	}
	return trimmed
}

type crawlerUserAgentEntry struct {
	Pattern string   `json:"pattern"`
	URL     string   `json:"url"`
	Tags    []string `json:"tags"`
}

func fetchCrawlerUserAgents(ctx context.Context, client agentHTTPDoer) ([]AIAgentEntry, error) {
	body, err := fetchAgentFeed(ctx, client, crawlerUserAgentsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch crawler user agents: %w", err)
	}
	defer body.Close()

	var raw []crawlerUserAgentEntry
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode crawler user agents: %w", err)
	}

	out := make([]AIAgentEntry, 0, len(raw))
	for _, entry := range raw {
		if !slices.Contains(entry.Tags, "ai-crawler") {
			continue
		}
		literal, ok := literalRegexToken(entry.Pattern)
		if !ok {
			continue
		}
		token := strings.ToLower(literal)
		if !admitToken(token) {
			continue
		}
		name := strings.TrimRight(literal, "/-")
		if name == "" {
			continue
		}
		out = append(out, AIAgentEntry{
			Token:    token,
			Name:     name,
			Family:   name,
			Category: CategoryOtherAI,
			Respect:  RespectUnclear,
			URL:      strings.TrimSpace(entry.URL),
			Sources:  []string{sourceCrawlerUserAgents},
		})
	}
	return out, nil
}

// literalRegexToken reports whether the pattern is a literal string once
// simple backslash escapes are resolved, and returns that literal. Patterns
// with real regex syntax are rejected; substring tokens are the only matching
// semantic implementable identically in Go and SQL.
func literalRegexToken(pattern string) (string, bool) {
	var builder strings.Builder
	escaped := false
	for _, r := range pattern {
		if escaped {
			builder.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if strings.ContainsRune(`.+*?()[]{}|^$`, r) {
			return "", false
		}
		builder.WriteRune(r)
	}
	if escaped {
		return "", false
	}
	return builder.String(), true
}

func fetchAgentFeed(ctx context.Context, client agentHTTPDoer, sourceURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	return io.NopCloser(io.LimitReader(resp.Body, maxAgentFeedResponseBytes)), nil
}

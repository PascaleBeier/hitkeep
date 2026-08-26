package aianalytics

import (
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	json "hitkeep/jsonapi"
)

//go:embed default_ai_agents.json
var embeddedAIAgentDataFS embed.FS

// AI agent categories exposed as analytics dimensions. Values are stable API
// identifiers; user-facing labels are translated in the dashboard.
const (
	CategoryTrainingCrawler = "ai_training_crawler"
	CategorySearchIndexer   = "ai_search_indexer"
	CategoryAssistant       = "ai_assistant"
	CategoryAgent           = "ai_agent"
	CategoryCodingAgent     = "ai_coding_agent"
	CategoryOtherAI         = "other_ai"
)

// Robots.txt compliance as reported by the upstream lists.
const (
	RespectYes     = "yes"
	RespectNo      = "no"
	RespectUnclear = "unclear"
)

const (
	sourceHitkeepCurated    = "hitkeep_curated"
	sourceAIRobotsTxt       = "ai_robots_txt"
	sourceDeviceDetector    = "device_detector_bots"
	sourceCrawlerUserAgents = "crawler_user_agents"
)

// minEmbeddedAgents guards against shipping a degraded bundle when an upstream
// list shrinks or a parser silently breaks.
const minEmbeddedAgents = 100

// minAgentsPerSource refuses embedded bundles in which an upstream feed
// failed or a parser silently stopped admitting entries. Runtime fetches stay
// tolerant of partial failures; the committed bundle must be complete.
const minAgentsPerSource = 10

const minTokenLength = 4

var knownCategories = map[string]struct{}{
	CategoryTrainingCrawler: {},
	CategorySearchIndexer:   {},
	CategoryAssistant:       {},
	CategoryAgent:           {},
	CategoryCodingAgent:     {},
	CategoryOtherAI:         {},
}

// tokenDenylist rejects upstream tokens that would classify ordinary browser or
// tooling traffic as AI agents, plus dual-use agents whose dominant real-world
// traffic is not AI even though upstream lists include them for robots.txt
// purposes. Matched against the exact token.
var tokenDenylist = map[string]struct{}{
	"agent": {}, "agents": {}, "assistant": {}, "browser": {}, "chat": {},
	"client": {}, "code": {}, "crawl": {}, "crawler": {}, "curl": {},
	"fetch": {}, "fetcher": {}, "http": {}, "https": {}, "index": {},
	"java": {}, "mozilla": {}, "node": {}, "python": {}, "robot": {},
	"scan": {}, "scraper": {}, "search": {}, "spider": {}, "wget": {},
	// facebookexternalhit is mostly Facebook/WhatsApp/Messenger link previews;
	// classifying those shares as AI traffic would skew the AI metrics.
	"facebookexternalhit": {},
}

// requiredReferrerNames pins the AI referrer surfaces that the previous
// hand-written hk_ai_source macro covered; a refresh may add names but must
// never lose one.
var requiredReferrerNames = []string{
	"Arc Search", "ChatGPT", "Claude", "Copilot", "DeepSeek", "Gemini",
	"HuggingChat", "Kagi", "Mistral", "Perplexity", "Phind", "Poe", "You.com",
}

type AIAgentEntry struct {
	Token    string   `json:"token"`
	Name     string   `json:"name"`
	Family   string   `json:"family"`
	Category string   `json:"category"`
	Respect  string   `json:"respects_robots_txt,omitempty"`
	URL      string   `json:"url,omitempty"`
	Sources  []string `json:"sources"`
}

type AIReferrerEntry struct {
	Name         string   `json:"name"`
	Hosts        []string `json:"hosts"`
	PathContains string   `json:"path_contains,omitempty"`
}

type AIAgentSourceMetadata struct {
	License     string `json:"license,omitempty"`
	URL         string `json:"url,omitempty"`
	Attribution string `json:"attribution,omitempty"`
}

type AIAgentData struct {
	GeneratedAt    time.Time                        `json:"generated_at"`
	Sources        map[string]string                `json:"sources"`
	SourceMetadata map[string]AIAgentSourceMetadata `json:"source_metadata,omitempty"`
	Agents         []AIAgentEntry                   `json:"agents"`
	AIReferrers    []AIReferrerEntry                `json:"ai_referrers"`
}

// MustEmbeddedAIAgentData decodes the embedded master list once per process and
// hands the same value to every derived shape (matchers, macros, catalog). The
// bundle is embedded and covered by tests; failing to decode it is a build
// defect, not a runtime condition, so a decode error panics.
var MustEmbeddedAIAgentData = sync.OnceValue(func() AIAgentData {
	data, err := LoadEmbeddedAIAgentData()
	if err != nil {
		panic("aianalytics: invalid embedded AI agent data: " + err.Error())
	}
	return data
})

func LoadEmbeddedAIAgentData() (AIAgentData, error) {
	raw, err := embeddedAIAgentDataFS.ReadFile("default_ai_agents.json")
	if err != nil {
		return AIAgentData{}, fmt.Errorf("read embedded AI agent data: %w", err)
	}
	return decodeAIAgentData(raw)
}

func LoadAIAgentData(path string) (AIAgentData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AIAgentData{}, err
	}
	return decodeAIAgentData(raw)
}

func SaveAIAgentData(path string, data AIAgentData) error {
	data.normalize()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create AI agent data dir: %w", err)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal AI agent data: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write AI agent data: %w", err)
	}
	return nil
}

func decodeAIAgentData(raw []byte) (AIAgentData, error) {
	var data AIAgentData
	if err := json.Unmarshal(raw, &data); err != nil {
		return AIAgentData{}, fmt.Errorf("decode AI agent data: %w", err)
	}
	data.normalize()
	return data, nil
}

// ValidateEmbeddedAIAgentData verifies the stricter completeness contract for
// the generated bundle shipped with HitKeep. The Go matcher and the generated
// DuckDB macros both derive from this data, so a degraded bundle must never be
// committed.
func ValidateEmbeddedAIAgentData(data AIAgentData) error {
	if data.GeneratedAt.IsZero() {
		return fmt.Errorf("embedded AI agent data is missing its generation timestamp")
	}
	for _, source := range []string{sourceHitkeepCurated, sourceAIRobotsTxt, sourceDeviceDetector, sourceCrawlerUserAgents} {
		if strings.TrimSpace(data.Sources[source]) == "" {
			return fmt.Errorf("embedded AI agent data is missing source %q", source)
		}
	}
	if len(data.Agents) < minEmbeddedAgents {
		return fmt.Errorf("embedded AI agent data has %d agents, expected at least %d", len(data.Agents), minEmbeddedAgents)
	}

	tokens := make(map[string]struct{}, len(data.Agents))
	for _, agent := range data.Agents {
		if err := validateAgentEntry(agent); err != nil {
			return err
		}
		if _, dup := tokens[agent.Token]; dup {
			return fmt.Errorf("embedded AI agent data contains duplicate token %q", agent.Token)
		}
		tokens[agent.Token] = struct{}{}
	}
	for _, curated := range curatedAgents {
		if _, ok := tokens[curated.Token]; !ok {
			return fmt.Errorf("embedded AI agent data lost curated token %q", curated.Token)
		}
	}

	perSource := make(map[string]int, 4)
	for _, agent := range data.Agents {
		for _, source := range agent.Sources {
			perSource[source]++
		}
	}
	for _, source := range []string{sourceAIRobotsTxt, sourceDeviceDetector, sourceCrawlerUserAgents} {
		if perSource[source] < minAgentsPerSource {
			return fmt.Errorf("embedded AI agent data has only %d agents from source %q, expected at least %d — upstream feed likely failed", perSource[source], source, minAgentsPerSource)
		}
	}

	referrers := make(map[string]struct{}, len(data.AIReferrers))
	for _, ref := range data.AIReferrers {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("embedded AI agent data contains a referrer without a name")
		}
		if len(ref.Hosts) == 0 {
			return fmt.Errorf("embedded AI referrer %q has no hosts", ref.Name)
		}
		for _, host := range ref.Hosts {
			if strings.ContainsAny(host, "'%_ ") {
				return fmt.Errorf("embedded AI referrer %q has host %q with unsupported character", ref.Name, host)
			}
		}
		referrers[ref.Name] = struct{}{}
	}
	for _, name := range requiredReferrerNames {
		if _, ok := referrers[name]; !ok {
			return fmt.Errorf("embedded AI agent data lost required referrer %q", name)
		}
	}
	return nil
}

// validateToken applies the token hygiene rules shared by the embedded-data
// validation and the updater, so an upstream entry the updater rejects is
// exactly one the committed bundle would refuse too.
func validateToken(token string) error {
	if len(token) < minTokenLength {
		return fmt.Errorf("AI agent token %q is too short", token)
	}
	if _, denied := tokenDenylist[token]; denied {
		return fmt.Errorf("AI agent token %q is a generic term and would misclassify regular traffic", token)
	}
	// Tokens become both Go substring matchers and SQL LIKE patterns; LIKE
	// wildcards and quotes would make the two disagree.
	if strings.ContainsAny(token, "'%\\\"") {
		return fmt.Errorf("AI agent token %q contains an unsupported character", token)
	}
	for _, r := range token {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("AI agent token %q contains an unsupported character", token)
		}
	}
	return nil
}

func validateAgentEntry(agent AIAgentEntry) error {
	if err := validateToken(agent.Token); err != nil {
		return err
	}
	if strings.TrimSpace(agent.Name) == "" {
		return fmt.Errorf("AI agent token %q is missing a name", agent.Token)
	}
	if strings.TrimSpace(agent.Family) == "" {
		return fmt.Errorf("AI agent %q is missing a family", agent.Name)
	}
	if _, ok := knownCategories[agent.Category]; !ok {
		return fmt.Errorf("AI agent %q has unknown category %q", agent.Name, agent.Category)
	}
	switch agent.Respect {
	case "", RespectYes, RespectNo, RespectUnclear:
	default:
		return fmt.Errorf("AI agent %q has unknown respects_robots_txt value %q", agent.Name, agent.Respect)
	}
	if len(agent.Sources) == 0 {
		return fmt.Errorf("AI agent %q has no sources", agent.Name)
	}
	return nil
}

// IconHostFromURL derives a favicon-lookup host from an agent's documentation
// URL. Returns "" when no usable host can be derived.
func IconHostFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
}

func (d *AIAgentData) normalize() {
	if d.Sources == nil {
		d.Sources = make(map[string]string)
	}

	for i := range d.Agents {
		d.Agents[i].Token = strings.ToLower(strings.TrimSpace(d.Agents[i].Token))
		d.Agents[i].Sources = normalizeTokenList(d.Agents[i].Sources, false)
	}
	slices.SortStableFunc(d.Agents, func(a, b AIAgentEntry) int {
		return strings.Compare(a.Token, b.Token)
	})
	deduped := d.Agents[:0]
	for _, agent := range d.Agents {
		if agent.Token == "" {
			continue
		}
		if n := len(deduped); n > 0 && deduped[n-1].Token == agent.Token {
			deduped[n-1].Sources = normalizeTokenList(append(deduped[n-1].Sources, agent.Sources...), false)
			continue
		}
		deduped = append(deduped, agent)
	}
	d.Agents = deduped

	for i := range d.AIReferrers {
		d.AIReferrers[i].Name = strings.TrimSpace(d.AIReferrers[i].Name)
		d.AIReferrers[i].Hosts = normalizeTokenList(d.AIReferrers[i].Hosts, true)
	}
	slices.SortStableFunc(d.AIReferrers, func(a, b AIReferrerEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func normalizeTokenList(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	slices.Sort(out)
	return out
}

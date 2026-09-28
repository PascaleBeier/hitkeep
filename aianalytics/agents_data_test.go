package aianalytics

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validTestData() AIAgentData {
	agents := make([]AIAgentEntry, 0, minEmbeddedAgents+len(curatedAgents))
	agents = append(agents, curatedAgents...)
	syntheticSources := []string{sourceAIRobotsTxt, sourceDeviceDetector, sourceCrawlerUserAgents}
	for i := 0; len(agents) < minEmbeddedAgents+5; i++ {
		agents = append(agents, AIAgentEntry{
			Token:    "syntheticbot-" + strings.Repeat("x", 1) + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
			Name:     "SyntheticBot",
			Family:   "Synthetic",
			Category: CategoryOtherAI,
			Respect:  RespectUnclear,
			Sources:  []string{syntheticSources[i%len(syntheticSources)]},
		})
	}

	referrers := make([]AIReferrerEntry, 0, len(requiredReferrerNames))
	hosts := map[string][]string{
		"ChatGPT":     {"chatgpt.com", "chat.openai.com"},
		"Perplexity":  {"perplexity.ai"},
		"Claude":      {"claude.ai"},
		"Gemini":      {"gemini.google.com"},
		"Copilot":     {"copilot.microsoft.com"},
		"You.com":     {"you.com"},
		"Phind":       {"phind.com"},
		"Kagi":        {"kagi.com"},
		"DeepSeek":    {"chat.deepseek.com"},
		"Mistral":     {"chat.mistral.ai"},
		"HuggingChat": {"huggingface.co"},
		"Poe":         {"poe.com"},
		"Arc Search":  {"arc.net"},
	}
	for _, name := range requiredReferrerNames {
		entry := AIReferrerEntry{Name: name, Hosts: hosts[name]}
		if name == "HuggingChat" {
			entry.PathContains = "/chat"
		}
		referrers = append(referrers, entry)
	}

	return AIAgentData{
		GeneratedAt: time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
		Sources: map[string]string{
			sourceHitkeepCurated:    "embedded",
			sourceAIRobotsTxt:       "https://example.test/robots.json",
			sourceDeviceDetector:    "https://example.test/bots.yml",
			sourceCrawlerUserAgents: "https://example.test/crawler-user-agents.json",
		},
		SourceMetadata: map[string]AIAgentSourceMetadata{
			sourceDeviceDetector: {License: "LGPL-3.0-or-later", URL: "https://github.com/matomo-org/device-detector"},
		},
		Agents:      agents,
		AIReferrers: referrers,
	}
}

func TestNormalizeDedupesSortsAndLowercasesTokens(t *testing.T) {
	data := AIAgentData{
		Agents: []AIAgentEntry{
			{Token: "  ZetaBot ", Name: "ZetaBot", Family: "Zeta", Category: CategoryOtherAI, Sources: []string{"b_src", "a_src", "a_src"}},
			{Token: "alphabot", Name: "AlphaBot", Family: "Alpha", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}},
			{Token: "ALPHABOT", Name: "AlphaBot Duplicate", Family: "Alpha", Category: CategoryOtherAI, Sources: []string{sourceDeviceDetector}},
		},
		AIReferrers: []AIReferrerEntry{
			{Name: "Zeta", Hosts: []string{"B.example.com", "a.example.com", "a.example.com"}},
			{Name: "Alpha", Hosts: []string{"alpha.example.com"}},
		},
	}
	data.normalize()

	if len(data.Agents) != 2 {
		t.Fatalf("expected 2 agents after dedupe, got %d", len(data.Agents))
	}
	if data.Agents[0].Token != "alphabot" || data.Agents[1].Token != "zetabot" {
		t.Fatalf("unexpected token order: %q, %q", data.Agents[0].Token, data.Agents[1].Token)
	}
	if data.Agents[0].Name != "AlphaBot" {
		t.Fatalf("dedupe should keep the first sorted entry, got %q", data.Agents[0].Name)
	}
	wantSources := []string{sourceAIRobotsTxt, sourceDeviceDetector}
	if len(data.Agents[0].Sources) != len(wantSources) {
		t.Fatalf("duplicate tokens should union sources, got %v", data.Agents[0].Sources)
	}
	for i, source := range wantSources {
		if data.Agents[0].Sources[i] != source {
			t.Fatalf("unexpected merged sources %v, want %v", data.Agents[0].Sources, wantSources)
		}
	}
	if data.Agents[1].Sources[0] != "a_src" || data.Agents[1].Sources[1] != "b_src" || len(data.Agents[1].Sources) != 2 {
		t.Fatalf("sources should be deduped and sorted, got %v", data.Agents[1].Sources)
	}
	if data.AIReferrers[0].Name != "Alpha" {
		t.Fatalf("referrers should be sorted by name, got %q first", data.AIReferrers[0].Name)
	}
	if len(data.AIReferrers[1].Hosts) != 2 || data.AIReferrers[1].Hosts[0] != "a.example.com" {
		t.Fatalf("hosts should be lowercased, deduped, sorted, got %v", data.AIReferrers[1].Hosts)
	}
}

func TestSaveLoadRoundTripIsDeterministic(t *testing.T) {
	data := validTestData()
	path := filepath.Join(t.TempDir(), "ai-agents.json")

	if err := SaveAIAgentData(path, data); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadAIAgentData(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Agents) != len(data.Agents) {
		t.Fatalf("agent count mismatch: got %d, want %d", len(loaded.Agents), len(data.Agents))
	}

	secondPath := filepath.Join(t.TempDir(), "ai-agents-2.json")
	if err := SaveAIAgentData(secondPath, loaded); err != nil {
		t.Fatalf("second save: %v", err)
	}
	first, err := LoadAIAgentData(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	second, err := LoadAIAgentData(secondPath)
	if err != nil {
		t.Fatalf("reload second: %v", err)
	}
	if len(first.Agents) != len(second.Agents) {
		t.Fatal("save/load round trip must be stable")
	}
}

func TestValidateEmbeddedAIAgentData(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AIAgentData)
		wantErr string
	}{
		{name: "valid", mutate: func(*AIAgentData) {}},
		{
			name:    "missing generated_at",
			mutate:  func(d *AIAgentData) { d.GeneratedAt = time.Time{} },
			wantErr: "generation timestamp",
		},
		{
			name:    "too few agents",
			mutate:  func(d *AIAgentData) { d.Agents = d.Agents[:20] },
			wantErr: "agents",
		},
		{
			name: "unknown category",
			mutate: func(d *AIAgentData) {
				d.Agents[len(d.Agents)-1].Category = "robot_overlord"
			},
			wantErr: "category",
		},
		{
			name: "token too short",
			mutate: func(d *AIAgentData) {
				d.Agents = append(d.Agents, AIAgentEntry{Token: "abc", Name: "Abc", Family: "Abc", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}})
			},
			wantErr: "too short",
		},
		{
			name: "denylisted generic token",
			mutate: func(d *AIAgentData) {
				d.Agents = append(d.Agents, AIAgentEntry{Token: "crawler", Name: "Crawler", Family: "Crawler", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}})
			},
			wantErr: "generic",
		},
		{
			name: "token with LIKE wildcard",
			mutate: func(d *AIAgentData) {
				d.Agents = append(d.Agents, AIAgentEntry{Token: "evil%bot", Name: "EvilBot", Family: "Evil", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}})
			},
			wantErr: "unsupported character",
		},
		{
			name: "token with quote",
			mutate: func(d *AIAgentData) {
				d.Agents = append(d.Agents, AIAgentEntry{Token: "evil'bot", Name: "EvilBot", Family: "Evil", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}})
			},
			wantErr: "unsupported character",
		},
		{
			name: "missing curated token",
			mutate: func(d *AIAgentData) {
				kept := d.Agents[:0]
				for _, agent := range d.Agents {
					if agent.Token != "gptbot" {
						kept = append(kept, agent)
					}
				}
				d.Agents = kept
			},
			wantErr: "curated token",
		},
		{
			name: "starved upstream source",
			mutate: func(d *AIAgentData) {
				for i := range d.Agents {
					if len(d.Agents[i].Sources) == 1 && d.Agents[i].Sources[0] == sourceCrawlerUserAgents {
						d.Agents[i].Sources = []string{sourceAIRobotsTxt}
					}
				}
			},
			wantErr: "upstream feed likely failed",
		},
		{
			name: "dual-use token",
			mutate: func(d *AIAgentData) {
				d.Agents = append(d.Agents, AIAgentEntry{Token: "facebookexternalhit", Name: "facebookexternalhit", Family: "Meta", Category: CategoryOtherAI, Sources: []string{sourceAIRobotsTxt}})
			},
			wantErr: "generic",
		},
		{
			name: "missing agent name",
			mutate: func(d *AIAgentData) {
				d.Agents[len(d.Agents)-1].Name = ""
			},
			wantErr: "name",
		},
		{
			name: "missing agent sources",
			mutate: func(d *AIAgentData) {
				d.Agents[len(d.Agents)-1].Sources = nil
			},
			wantErr: "sources",
		},
		{
			name:    "missing source url",
			mutate:  func(d *AIAgentData) { delete(d.Sources, sourceAIRobotsTxt) },
			wantErr: "missing source",
		},
		{
			name: "missing required referrer",
			mutate: func(d *AIAgentData) {
				kept := d.AIReferrers[:0]
				for _, ref := range d.AIReferrers {
					if ref.Name != "ChatGPT" {
						kept = append(kept, ref)
					}
				}
				d.AIReferrers = kept
			},
			wantErr: "referrer",
		},
		{
			name: "referrer without hosts",
			mutate: func(d *AIAgentData) {
				d.AIReferrers[0].Hosts = nil
			},
			wantErr: "hosts",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := validTestData()
			tc.mutate(&data)
			data.normalize()
			err := ValidateEmbeddedAIAgentData(data)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid data, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestEmbeddedAIAgentDataIsValid(t *testing.T) {
	data, err := LoadEmbeddedAIAgentData()
	if err != nil {
		t.Fatalf("load embedded data: %v", err)
	}
	if err := ValidateEmbeddedAIAgentData(data); err != nil {
		t.Fatalf("embedded data is invalid: %v", err)
	}
}

func TestCuratedAgentsAreWellFormed(t *testing.T) {
	seen := make(map[string]struct{}, len(curatedAgents))
	for _, agent := range curatedAgents {
		if agent.Token != strings.ToLower(strings.TrimSpace(agent.Token)) {
			t.Fatalf("curated token %q must be lowercase and trimmed", agent.Token)
		}
		if _, dup := seen[agent.Token]; dup {
			t.Fatalf("duplicate curated token %q", agent.Token)
		}
		seen[agent.Token] = struct{}{}
		if err := validateAgentEntry(agent); err != nil {
			t.Fatalf("curated agent %q invalid: %v", agent.Token, err)
		}
		if len(agent.Sources) != 1 || agent.Sources[0] != sourceHitkeepCurated {
			t.Fatalf("curated agent %q must declare only the curated source, got %v", agent.Token, agent.Sources)
		}
	}
}

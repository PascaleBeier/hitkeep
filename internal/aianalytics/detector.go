package aianalytics

import (
	"strings"
	"sync"
)

type BotIdentity struct {
	Name     string
	Family   string
	Category string
}

type botMatcher struct {
	token    string
	identity BotIdentity
}

// botMatchers derives the matcher table from the embedded master list.
// Longest token first makes overlapping tokens ("chatgpt-user" vs "gptbot",
// "ai2bot-dolma" vs "ai2bot") deterministic; the generated DuckDB macros use
// the same order so Go and SQL always agree.
var botMatchers = sync.OnceValue(func() []botMatcher {
	ordered := matchOrderedAgents(MustEmbeddedAIAgentData())
	matchers := make([]botMatcher, 0, len(ordered))
	for _, agent := range ordered {
		matchers = append(matchers, botMatcher{
			token: agent.Token,
			identity: BotIdentity{
				Name:     agent.Name,
				Family:   agent.Family,
				Category: agent.Category,
			},
		})
	}
	return matchers
})

func ClassifyBot(userAgent string) *BotIdentity {
	normalized := strings.ToLower(strings.TrimSpace(userAgent))
	if normalized == "" {
		return nil
	}

	for _, matcher := range botMatchers() {
		if strings.Contains(normalized, matcher.token) {
			identity := matcher.identity
			return &identity
		}
	}

	return nil
}

func ClassifyResourceType(contentType string) string {
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(normalized, ";"); idx >= 0 {
		normalized = normalized[:idx]
	}

	switch {
	case normalized == "", normalized == "text/html", normalized == "application/xhtml+xml":
		return "html"
	case strings.HasPrefix(normalized, "image/"):
		return "image"
	case normalized == "application/pdf",
		normalized == "application/msword",
		normalized == "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		normalized == "application/vnd.ms-powerpoint",
		normalized == "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		normalized == "application/vnd.ms-excel",
		normalized == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		normalized == "text/plain",
		normalized == "text/markdown":
		return "document"
	default:
		return "other"
	}
}

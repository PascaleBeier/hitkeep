package aianalytics

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// matchOrderedAgents returns agents in matching order: longest token first,
// then token ascending. ClassifyBot and the generated macros both use this
// order, so the Go and SQL classifiers always agree on overlapping tokens.
func matchOrderedAgents(data AIAgentData) []AIAgentEntry {
	agents := make([]AIAgentEntry, len(data.Agents))
	copy(agents, data.Agents)
	slices.SortStableFunc(agents, func(a, b AIAgentEntry) int {
		if diff := len(b.Token) - len(a.Token); diff != 0 {
			return diff
		}
		return strings.Compare(a.Token, b.Token)
	})
	return agents
}

// EmbeddedAIClassificationMacroStatements returns the CREATE OR REPLACE MACRO
// statements derived from the embedded master list. The database layer runs
// them at every store open, so historical rows are reclassified whenever a
// release ships a fresher list.
var EmbeddedAIClassificationMacroStatements = sync.OnceValue(func() []string {
	return AIClassificationMacroStatements(MustEmbeddedAIAgentData())
})

// AIClassificationMacroStatements generates hk_ai_bot, hk_ai_bot_category,
// hk_ai_bot_category_from_name, and hk_ai_source macro definitions from the
// master list. Output is deterministic for a given data set.
func AIClassificationMacroStatements(data AIAgentData) []string {
	agents := matchOrderedAgents(data)

	var bot strings.Builder
	bot.WriteString("CREATE OR REPLACE MACRO hk_ai_bot_lc(lua) AS\n    CASE\n")
	var category strings.Builder
	category.WriteString("CREATE OR REPLACE MACRO hk_ai_bot_category_lc(lua) AS\n    CASE\n")
	for _, agent := range agents {
		pattern := likePattern(agent.Token)
		fmt.Fprintf(&bot, "        WHEN lua LIKE %s ESCAPE '|' THEN %s\n", pattern, sqlString(agent.Name))
		fmt.Fprintf(&category, "        WHEN lua LIKE %s ESCAPE '|' THEN %s\n", pattern, sqlString(agent.Category))
	}
	bot.WriteString("        ELSE NULL\n    END;")
	category.WriteString("        ELSE NULL\n    END;")

	return []string{
		bot.String(),
		"CREATE OR REPLACE MACRO hk_ai_bot(ua) AS\n" +
			"    CASE WHEN ua IS NULL OR TRIM(ua) = '' THEN NULL ELSE hk_ai_bot_lc(lower(ua)) END;",
		category.String(),
		"CREATE OR REPLACE MACRO hk_ai_bot_category(ua) AS\n" +
			"    CASE WHEN ua IS NULL OR TRIM(ua) = '' THEN NULL ELSE hk_ai_bot_category_lc(lower(ua)) END;",
		aiBotCategoryFromNameMacro(agents),
		aiSourceHostMacro(data.AIReferrers),
		"CREATE OR REPLACE MACRO hk_ai_source(ref) AS\n" +
			"    CASE\n" +
			"        WHEN ref IS NULL OR TRIM(ref) = '' THEN NULL\n" +
			"        ELSE hk_ai_source_from_host(\n" +
			"            CASE\n" +
			"                WHEN lower(TRIM(ref)) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(TRIM(ref)), 'https?://([^/:?#]+)', 1), '^www\\.', '')\n" +
			"                ELSE regexp_replace(lower(TRIM(ref)), '^www\\.', '')\n" +
			"            END,\n" +
			"            lower(TRIM(ref)))\n" +
			"    END;",
	}
}

// aiBotCategoryFromNameMacro maps an already-classified agent name to its
// category with an equality CASE. Queries that need both the name and the
// category can then scan the user agent once via hk_ai_bot and derive the
// category from its result instead of walking every token pattern twice.
// Agents are expected in matching order, so when two tokens share a name but
// disagree on the category the first match wins — exactly like hk_ai_bot and
// hk_ai_bot_category resolve the same overlap.
func aiBotCategoryFromNameMacro(agents []AIAgentEntry) string {
	var builder strings.Builder
	builder.WriteString("CREATE OR REPLACE MACRO hk_ai_bot_category_from_name(name) AS\n    CASE\n")
	seen := make(map[string]struct{}, len(agents))
	for _, agent := range agents {
		if _, dup := seen[agent.Name]; dup {
			continue
		}
		seen[agent.Name] = struct{}{}
		fmt.Fprintf(&builder, "        WHEN name = %s THEN %s\n", sqlString(agent.Name), sqlString(agent.Category))
	}
	builder.WriteString("        ELSE NULL\n    END;")
	return builder.String()
}

func aiSourceHostMacro(referrers []AIReferrerEntry) string {
	var builder strings.Builder
	builder.WriteString("CREATE OR REPLACE MACRO hk_ai_source_from_host(h, lref) AS\n    CASE\n")
	// Path-conditional entries first so a plain host rule for the same host
	// can never shadow them.
	for _, withPath := range []bool{true, false} {
		for _, ref := range referrers {
			if (ref.PathContains != "") != withPath {
				continue
			}
			hosts := make([]string, 0, len(ref.Hosts))
			for _, host := range ref.Hosts {
				hosts = append(hosts, sqlString(host))
			}
			fmt.Fprintf(&builder, "        WHEN h IN (%s)", strings.Join(hosts, ", "))
			if ref.PathContains != "" {
				fmt.Fprintf(&builder, " AND lref LIKE %s", sqlString("%"+strings.ToLower(ref.PathContains)+"%"))
			}
			fmt.Fprintf(&builder, " THEN %s\n", sqlString(ref.Name))
		}
	}
	builder.WriteString("        ELSE NULL\n    END;")
	return builder.String()
}

// likePattern wraps a validated token in LIKE wildcards. '_' is a LIKE
// single-character wildcard, so it is escaped with '|'; '%', quotes, and
// backslashes are rejected by the data validation before they get here.
func likePattern(token string) string {
	escaped := strings.ReplaceAll(token, "|", "||")
	escaped = strings.ReplaceAll(escaped, "_", "|_")
	return sqlString("%" + escaped + "%")
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

package skills

import (
	"embed"
	"strings"
)

// Keep the embedded files explicit. These are transport-neutral analytics
// procedures, not external MCP adapters or contributor instructions.
//
//go:embed hitkeep-analytics/references/procedure.md hitkeep-traffic-diagnosis/references/procedure.md hitkeep-ai-visibility-analyst/references/procedure.md hitkeep-ecommerce-analyst/references/procedure.md hitkeep-tracking-verifier/references/procedure.md
var skillFS embed.FS

var embeddedAnalyticsProcedureFiles = []string{
	"hitkeep-analytics/references/procedure.md",
	"hitkeep-traffic-diagnosis/references/procedure.md",
	"hitkeep-ai-visibility-analyst/references/procedure.md",
	"hitkeep-ecommerce-analyst/references/procedure.md",
	"hitkeep-tracking-verifier/references/procedure.md",
}

// EmbeddedAnalyticsProcedurePack returns the transport-neutral analytics
// procedures used to ground HitKeep Ask AI.
func EmbeddedAnalyticsProcedurePack() string {
	var builder strings.Builder
	for _, path := range embeddedAnalyticsProcedureFiles {
		data, err := skillFS.ReadFile(path)
		if err != nil {
			return ""
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n---\n\n")
		}
		builder.Write(data)
	}
	return builder.String()
}

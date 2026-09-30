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

// Procedure is one transport-neutral analytics procedure.
type Procedure struct {
	// Name is the product skill identity, such as hitkeep-traffic-diagnosis.
	Name     string
	Title    string
	Markdown string
}

// AnalyticsProcedures returns each embedded procedure, titled by its heading.
func AnalyticsProcedures() []Procedure {
	out := make([]Procedure, 0, len(embeddedAnalyticsProcedureFiles))
	for _, path := range embeddedAnalyticsProcedureFiles {
		data, err := skillFS.ReadFile(path)
		if err != nil {
			continue
		}
		markdown := string(data)
		heading, _, _ := strings.Cut(markdown, "\n")
		name, _, _ := strings.Cut(path, "/")
		out = append(out, Procedure{Name: name, Title: strings.TrimSpace(strings.TrimPrefix(heading, "#")), Markdown: markdown})
	}
	return out
}

// EmbeddedAnalyticsProcedurePack returns the transport-neutral analytics
// procedures used to ground HitKeep Ask AI.
func EmbeddedAnalyticsProcedurePack() string {
	procedures := AnalyticsProcedures()
	parts := make([]string, 0, len(procedures))
	for _, procedure := range procedures {
		parts = append(parts, procedure.Markdown)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

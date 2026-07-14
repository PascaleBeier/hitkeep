package skills

import (
	"strings"
	"testing"
)

func TestEmbeddedAnalyticsProcedurePackUsesExactTransportNeutralFiles(t *testing.T) {
	want := []string{
		"hitkeep-analytics/references/procedure.md",
		"hitkeep-traffic-diagnosis/references/procedure.md",
		"hitkeep-ai-visibility-analyst/references/procedure.md",
		"hitkeep-ecommerce-analyst/references/procedure.md",
		"hitkeep-tracking-verifier/references/procedure.md",
	}
	if len(embeddedAnalyticsProcedureFiles) != len(want) {
		t.Fatalf("embedded procedure count = %d, want %d", len(embeddedAnalyticsProcedureFiles), len(want))
	}
	pack := EmbeddedAnalyticsProcedurePack()
	for index, path := range want {
		if embeddedAnalyticsProcedureFiles[index] != path {
			t.Errorf("embedded procedure %d = %q, want %q", index, embeddedAnalyticsProcedureFiles[index], path)
		}
		raw, err := skillFS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(pack, string(raw)) {
			t.Errorf("analytics procedure %q is missing", path)
		}
	}
	for _, marker := range []string{
		"name: hitkeep-",
		"production HitKeep MCP",
		"API client token",
		"hk_doctor",
		"hk_workspace_status",
		"hk_qa_plan",
		"TranslocoPipe",
		"./hk",
	} {
		if strings.Contains(pack, marker) {
			t.Errorf("adapter or contributor instruction %q leaked into Ask AI context", marker)
		}
	}
}

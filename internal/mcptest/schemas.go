// Package mcptest holds assertions shared by the MCP server test suites. It is
// test-support code only: nothing outside a _test.go file may import it.
package mcptest

import (
	"testing"

	json "hitkeep/jsonapi"
)

// RequireObjectFormPropertySchemas guards a tool contract against boolean-form
// property schemas, which jsonschema-go emits for `any`-typed fields and some MCP
// clients (Claude Code) reject. Call it for both the input and the output schema
// of every exposed tool; kind names which one is under test ("input"/"output").
//
// A nil schema is accepted: not every tool declares both halves.
func RequireObjectFormPropertySchemas(t *testing.T, toolName, kind string, toolSchema any) {
	t.Helper()

	if toolSchema == nil {
		return
	}
	raw, err := json.Marshal(toolSchema)
	if err != nil {
		t.Fatalf("tool %s: marshal %s schema: %v", toolName, kind, err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("tool %s: %s schema is not an object: %v", toolName, kind, err)
	}
	for property, propertySchema := range schema.Properties {
		if len(propertySchema) == 0 || propertySchema[0] != '{' {
			t.Fatalf("tool %s: %s schema property %q is not object-form: %s", toolName, kind, property, propertySchema)
		}
	}
}

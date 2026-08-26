package config

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestRenderExampleYAML(t *testing.T) {
	rendered := string(RenderExampleYAML())
	if !bytes.Equal(RenderExampleYAML(), RenderExampleYAML()) {
		t.Fatal("example YAML is not deterministic")
	}

	for _, setting := range Catalog().Settings {
		key := setting.ConfigFileKey + ":"
		if setting.CloudOnly {
			if keyOccurrences(rendered, setting.ConfigFileKey) != 0 {
				t.Errorf("cloud-only %s appears in self-hosted example", setting.Field)
			}
			continue
		}
		if got := keyOccurrences(rendered, setting.ConfigFileKey); got != 1 {
			t.Errorf("%s appears %d times, want once", setting.Field, got)
		}
		if !strings.Contains(rendered, "# "+setting.Description) {
			t.Errorf("%s description is missing", setting.Field)
		}
		if !strings.Contains(rendered, "type: "+setting.Type) {
			t.Errorf("%s type metadata is missing", setting.Field)
		}
		if setting.Sensitive != "" || setting.DisplayDefault != "" {
			if strings.Contains(rendered, "\n"+key) {
				t.Errorf("%s should remain unset", setting.Field)
			}
			if !strings.Contains(rendered, "\n# "+key) {
				t.Errorf("%s should be a commented key", setting.Field)
			}
			continue
		}
		if !strings.Contains(rendered, "\n"+key) {
			t.Errorf("%s ordinary default is not active", setting.Field)
		}
		if !strings.Contains(rendered, "default: "+strconv.Quote(setting.Default)) {
			t.Errorf("%s default metadata is missing", setting.Field)
		}
	}
}

func TestRenderExampleYAMLSuppressesSensitiveAndDerivedValues(t *testing.T) {
	rendered := string(renderExampleYAML(ConfigurationCatalog{Settings: []ConfigurationSetting{
		{
			ConfigFileKey: "api-key",
			Description:   "API key",
			Type:          "string",
			Default:       "secret-value",
			Sensitive:     "redact",
		},
		{
			ConfigFileKey:  "node-name",
			Description:    "Node name",
			Type:           "string",
			Default:        "runtime-value",
			DisplayDefault: "generated",
		},
	}}))

	for _, value := range []string{"secret-value", "runtime-value"} {
		if strings.Contains(rendered, value) {
			t.Errorf("rendered example leaks inactive value %q", value)
		}
	}
	for _, key := range []string{"api-key", "node-name"} {
		if !strings.Contains(rendered, "\n# "+key+":") || strings.Contains(rendered, "\n"+key+":") {
			t.Errorf("%s is not commented and unset: %q", key, rendered)
		}
	}
}

func keyOccurrences(contents, key string) int {
	return strings.Count(contents, "\n"+key+":") + strings.Count(contents, "\n# "+key+":")
}

func TestExampleYAMLMatchesCheckedInFile(t *testing.T) {
	got, err := os.ReadFile("../../hitkeep.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if want := RenderExampleYAML(); !bytes.Equal(got, want) {
		for index := range min(len(got), len(want)) {
			if got[index] != want[index] {
				t.Fatalf("example mismatch at byte %d: got %q, want %q", index, got[index:], want[index:])
			}
		}
		t.Fatalf("example length = %d, want %d", len(got), len(want))
	}
}

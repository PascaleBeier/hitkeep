package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestViperRejectsAmbiguousYAMLShapes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "duplicate key", content: "data-path: /first\ndata-path: /second\n", want: "duplicate configuration key"},
		{name: "multiple documents", content: "data-path: /first\n---\ndata-path: /second\n", want: "exactly one YAML document"},
		{name: "alias", content: "data-path: &path /data\npublic-url: *path\n", want: "aliases are not supported"},
		{name: "merge key", content: "data-path: /data\n<<: {public-url: https://example.com}\n", want: "merge keys are not supported"},
		{name: "null", content: "data-path: null\n", want: "must be a scalar"},
		{name: "sequence", content: "data-path: [/first, /second]\n", want: "must be a scalar"},
		{name: "mapping", content: "data-path:\n  path: /data\n", want: "must be a scalar"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if err := afero.WriteFile(fs, "/config.yaml", []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadViper(nil, func(_ string, fallback string) string { return fallback }, fs, "/config.yaml")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadViper() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

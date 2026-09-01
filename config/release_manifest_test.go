package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderConfigurationReleaseManifestDeterministic(t *testing.T) {
	catalog := []byte("{\"schema_version\":\"hitkeep.config/v2\"}\n")
	example := []byte("data-path: /var/lib/hitkeep/data\n")

	first := RenderConfigurationReleaseManifest(catalog, example)
	second := RenderConfigurationReleaseManifest(catalog, example)
	if !bytes.Equal(first, second) {
		t.Fatalf("manifest output differs between identical inputs\nfirst: %s\nsecond: %s", first, second)
	}
	if err := ValidateConfigurationReleaseManifest(first, catalog, example); err != nil {
		t.Fatalf("ValidateConfigurationReleaseManifest() error = %v", err)
	}

	var manifest ConfigurationReleaseManifest
	if err := json.Unmarshal(first, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.SchemaVersion != ConfigurationReleaseManifestSchemaVersion {
		t.Errorf("schema version = %q, want %q", manifest.SchemaVersion, ConfigurationReleaseManifestSchemaVersion)
	}
	if len(manifest.Subjects) != 2 || manifest.Subjects[0].Name != ConfigurationCatalogFilename || manifest.Subjects[1].Name != ConfigurationExampleFilename {
		t.Fatalf("subjects = %#v, want catalog then example", manifest.Subjects)
	}
}

func TestValidateConfigurationReleaseManifestRejectsMissingStaleAndWrongSubjects(t *testing.T) {
	catalog := []byte("catalog\n")
	example := []byte("example\n")
	valid := RenderConfigurationReleaseManifest(catalog, example)
	mutate := func(change func(*ConfigurationReleaseManifest)) []byte {
		t.Helper()
		var manifest ConfigurationReleaseManifest
		if err := json.Unmarshal(valid, &manifest); err != nil {
			t.Fatalf("unmarshal valid manifest: %v", err)
		}
		change(&manifest)
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		return raw
	}

	tests := []struct {
		name     string
		manifest []byte
		catalog  []byte
		example  []byte
		want     string
	}{
		{
			name:     "missing catalog subject",
			manifest: mutate(func(manifest *ConfigurationReleaseManifest) { manifest.Subjects[0].Name = "missing-catalog" }),
			catalog:  catalog,
			example:  example,
			want:     ConfigurationCatalogFilename,
		},
		{
			name:     "missing example subject",
			manifest: mutate(func(manifest *ConfigurationReleaseManifest) { manifest.Subjects[1].Name = "missing-example" }),
			catalog:  catalog,
			example:  example,
			want:     ConfigurationExampleFilename,
		},
		{
			name:     "stale catalog digest",
			manifest: valid,
			catalog:  append(append([]byte(nil), catalog...), 'x'),
			example:  example,
			want:     ConfigurationCatalogFilename,
		},
		{
			name:     "stale example digest",
			manifest: valid,
			catalog:  catalog,
			example:  append(append([]byte(nil), example...), 'x'),
			want:     ConfigurationExampleFilename,
		},
		{
			name:     "wrong catalog digest",
			manifest: mutate(func(manifest *ConfigurationReleaseManifest) { manifest.Subjects[0].SHA256 = strings.Repeat("0", 64) }),
			catalog:  catalog,
			example:  example,
			want:     ConfigurationCatalogFilename,
		},
		{
			name:     "wrong example digest",
			manifest: mutate(func(manifest *ConfigurationReleaseManifest) { manifest.Subjects[1].SHA256 = strings.Repeat("0", 64) }),
			catalog:  catalog,
			example:  example,
			want:     ConfigurationExampleFilename,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateConfigurationReleaseManifest(test.manifest, test.catalog, test.example)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateConfigurationReleaseManifest() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

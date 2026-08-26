package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestConfigurationCatalogCoversRuntimeAnnotations(t *testing.T) {
	catalog := Catalog()
	if catalog.SchemaVersion != ConfigurationCatalogSchemaVersion {
		t.Fatalf("schema version = %q", catalog.SchemaVersion)
	}

	wantSettings := 0
	typeOfConfig := reflect.TypeFor[Config]()
	for field := range typeOfConfig.Fields() {
		if field.IsExported() && (field.Tag.Get("env") != "" || field.Tag.Get("flag") != "") {
			wantSettings++
		}
	}
	if len(catalog.Settings) != wantSettings {
		t.Fatalf("settings = %d, want %d", len(catalog.Settings), wantSettings)
	}

	environments := map[string]bool{}
	flags := map[string]bool{}
	for _, setting := range catalog.Settings {
		if setting.Category == "uncategorized" {
			t.Errorf("%s has no documentation category", setting.Field)
		}
		if setting.Type == "unsupported" || setting.Description == "" || setting.Flag == "" {
			t.Errorf("incomplete catalog setting: %+v", setting)
		}
		if setting.Environment != "" {
			if environments[setting.Environment] {
				t.Errorf("duplicate environment variable %s", setting.Environment)
			}
			environments[setting.Environment] = true
		}
		if flags[setting.Flag] {
			t.Errorf("duplicate flag --%s", setting.Flag)
		}
		flags[setting.Flag] = true
	}
}

func TestConfigurationPublicationRequirementsCoverPersistentDataPath(t *testing.T) {
	requirements := PublicationRequirements()
	if len(requirements) != 1 {
		t.Fatalf("publication requirements = %d, want exactly the persistent data path", len(requirements))
	}

	requirement := requirements[0]
	if requirement.Environment != "HITKEEP_DATA_PATH" {
		t.Fatalf("publication requirement environment = %q, want HITKEEP_DATA_PATH", requirement.Environment)
	}
	for _, surface := range []ConfigurationPublicationSurface{
		ConfigurationPublicationDocker,
		ConfigurationPublicationCompose,
		ConfigurationPublicationHelm,
		ConfigurationPublicationExample,
		ConfigurationPublicationCanonicalExample,
	} {
		declared := slices.Contains(requirement.Surfaces, surface)
		if !declared {
			t.Errorf("publication requirement does not require %s", surface)
		}
		if requirement.Defaults[surface] == "" {
			t.Errorf("publication requirement has no default for %s", surface)
		}
		if len(requirement.Paths[surface]) == 0 {
			t.Errorf("publication requirement has no paths for %s", surface)
		}
	}
}

func TestConfigurationCatalogConfigFileKeys(t *testing.T) {
	catalog := Catalog()
	if catalog.SchemaVersion != "hitkeep.config/v2" {
		t.Fatalf("schema version = %q, want hitkeep.config/v2", catalog.SchemaVersion)
	}
	seen := make(map[string]bool, len(catalog.Settings))
	second := Catalog()
	for index, setting := range catalog.Settings {
		if setting.ConfigFileKey == "" {
			t.Errorf("%s has no config-file key", setting.Field)
		}
		if seen[setting.ConfigFileKey] {
			t.Errorf("duplicate config-file key %q", setting.ConfigFileKey)
		}
		seen[setting.ConfigFileKey] = true

		want := setting.Flag
		if setting.Environment != "" {
			want = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(setting.Environment, "HITKEEP_")), "_", "-")
		}
		if setting.ConfigFileKey != want {
			t.Errorf("%s config-file key = %q, want %q", setting.Field, setting.ConfigFileKey, want)
		}
		if strings.Contains(setting.ConfigFileKey, "_") || setting.ConfigFileKey != strings.ToLower(setting.ConfigFileKey) {
			t.Errorf("%s config-file key is not lowercase kebab-case: %q", setting.Field, setting.ConfigFileKey)
		}
		if setting.ConfigFileKey != second.Settings[index].ConfigFileKey {
			t.Errorf("%s config-file key is not deterministic: %q then %q", setting.Field, setting.ConfigFileKey, second.Settings[index].ConfigFileKey)
		}
	}
}

func TestConfigurationCatalogPreservesDerivedAndSensitiveMetadata(t *testing.T) {
	settings := map[string]ConfigurationSetting{}
	for _, setting := range Catalog().Settings {
		settings[setting.Environment] = setting
	}
	if settings["HITKEEP_JWT_SECRET"].DisplayDefault != "randomly generated" || settings["HITKEEP_JWT_SECRET"].Sensitive != "redact" {
		t.Fatalf("JWT metadata = %+v", settings["HITKEEP_JWT_SECRET"])
	}
	if settings["HITKEEP_DUCKDB_THREADS"].Default != "0" || settings["HITKEEP_DUCKDB_THREADS"].DisplayDefault != "GOMAXPROCS" {
		t.Fatalf("DuckDB thread metadata = %+v", settings["HITKEEP_DUCKDB_THREADS"])
	}
	if !settings["HITKEEP_CLOUD_HOSTED"].CloudOnly {
		t.Fatalf("cloud metadata = %+v", settings["HITKEEP_CLOUD_HOSTED"])
	}
}

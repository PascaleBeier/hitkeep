package config

import (
	"reflect"
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

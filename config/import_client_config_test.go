package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportClientConfigurationUsesCatalogViperPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(path, []byte("public-url: http://public-file.example\napi-url: http://api-file.example\napi-token: file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		env     map[string]string
		wantURL string
		wantTok string
	}{
		{name: "config file", wantURL: "http://api-file.example", wantTok: "file-token"},
		{name: "primary environment", env: map[string]string{"HITKEEP_API_URL": "http://api-env.example", "HITKEEP_API_TOKEN": "env-token"}, wantURL: "http://api-env.example", wantTok: "env-token"},
		{name: "legacy URL environment", env: map[string]string{"HITKEEP_URL": "http://legacy-env.example"}, wantURL: "http://legacy-env.example", wantTok: "file-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			conf, err := LoadArgs(nil, path)
			if err != nil {
				t.Fatal(err)
			}
			if conf.ImportAPIURL != tt.wantURL {
				t.Errorf("ImportAPIURL = %q, want %q", conf.ImportAPIURL, tt.wantURL)
			}
			if conf.ImportAPIToken != tt.wantTok {
				t.Errorf("ImportAPIToken = %q, want %q", conf.ImportAPIToken, tt.wantTok)
			}
		})
	}
}

func TestImportClientConfigurationCatalogsSecretAndLegacyEnvironment(t *testing.T) {
	var apiURL, token *ConfigurationSetting
	for index := range Catalog().Settings {
		setting := &Catalog().Settings[index]
		switch setting.Field {
		case "ImportAPIURL":
			apiURL = setting
		case "ImportAPIToken":
			token = setting
		}
	}
	if apiURL == nil || apiURL.Environment != "HITKEEP_API_URL" || len(apiURL.DeprecatedEnvironments) != 1 || apiURL.DeprecatedEnvironments[0] != "HITKEEP_URL" {
		t.Fatalf("ImportAPIURL catalog = %#v", apiURL)
	}
	if token == nil || token.Environment != "HITKEEP_API_TOKEN" || token.Sensitive == "" {
		t.Fatalf("ImportAPIToken catalog = %#v", token)
	}
}

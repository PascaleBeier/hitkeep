package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestLoadUsesViperWithLegacyParity(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"hitkeep", "--healthcheck"}

	getEnv := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}
	if got, want := Load(), load(os.Args[1:], getEnv); !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() differs from legacy loader:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestLoadArgsReadsExplicitOSFile(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(configFile, []byte("http-addr: ':7070'\nhealthcheck: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf, err := LoadArgs(nil, configFile)
	if err != nil {
		t.Fatal(err)
	}
	if conf.HTTPAddr != ":7070" || !conf.Healthcheck {
		t.Fatalf("explicit config was not loaded: %#v", conf)
	}
}

func TestViperShadowMatchesLegacyLoader(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "defaults", args: []string{"--healthcheck"}},
		{
			name: "environment",
			args: []string{"--healthcheck"},
			env: map[string]string{
				"HITKEEP_HTTP_ADDR":      ":8181",
				"HITKEEP_MAIL_PORT":      "2525",
				"HITKEEP_S3_USE_SSL":     "false",
				"HITKEEP_API_RATE_LIMIT": "12.5",
			},
		},
		{
			name: "flags override environment",
			args: []string{"--healthcheck", "--http-addr=:9191", "--mail-port=3535"},
			env:  map[string]string{"HITKEEP_HTTP_ADDR": ":8181", "HITKEEP_MAIL_PORT": "2525"},
		},
		{
			name: "deprecated then canonical",
			args: []string{"--healthcheck", "--http=:8181", "--http-addr=:9191"},
		},
		{
			name: "canonical then deprecated",
			args: []string{"--healthcheck", "--http-addr=:9191", "--http=:8181"},
		},
		{
			name: "invalid environment",
			args: []string{"--healthcheck"},
			env: map[string]string{
				"HITKEEP_MAIL_PORT":  "invalid",
				"HITKEEP_S3_USE_SSL": "invalid",
			},
		},
		{
			name: "runtime normalization",
			env: map[string]string{
				"HITKEEP_JWT_SECRET":          "fixed-secret",
				"HITKEEP_NODE_NAME":           "fixed-node",
				"HITKEEP_DUCKDB_MEMORY_LIMIT": "none",
				"HITKEEP_DUCKDB_THREADS":      "2",
				"HITKEEP_TRUSTED_PROXIES":     "127.0.0.1/32",
				"HITKEEP_MCP_DOCS_URL":        "https://example.com/",
				"HITKEEP_DB_RECOVERY_PATH":    "/recovery",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getEnv := mapEnv(tt.env)
			var legacyLog, shadowLog bytes.Buffer
			logOptions := &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
				if attr.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return attr
			}}
			legacy := load(tt.args, getEnv, slog.New(slog.NewTextHandler(&legacyLog, logOptions)))
			shadow, err := loadViper(tt.args, getEnv, afero.NewMemMapFs(), "", slog.New(slog.NewTextHandler(&shadowLog, logOptions)))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(shadow, legacy) {
				t.Fatalf("shadow config differs from legacy\nshadow: %#v\nlegacy: %#v", shadow, legacy)
			}
			if shadowLog.String() != legacyLog.String() {
				t.Fatalf("shadow logs differ from legacy\nshadow: %q\nlegacy: %q", shadowLog.String(), legacyLog.String())
			}
		})
	}
}

func TestViperShadowCatalogEnvironmentAndFlagParity(t *testing.T) {
	for _, setting := range Catalog().Settings {
		if setting.CloudOnly && !includeCloudConfigFields() {
			continue
		}
		t.Run(setting.Field, func(t *testing.T) {
			args, baseEnv := catalogParityInputs(setting)
			absent, _ := loadViperShadowParity(t, args, baseEnv)
			if setting.Environment == "" {
				flagValue := alternateConfigValue(t, setting, absent)
				flagged, _ := loadViperShadowParity(t, append(args, "--"+canonicalFlag(t, setting)+"="+flagValue), baseEnv)
				assertConfigSetting(t, setting, flagged, flagValue)
				return
			}

			emptyEnv := cloneEnv(baseEnv)
			emptyEnv[setting.Environment] = ""
			var empty *Config
			if hasGeneratedValue(setting) {
				empty, _ = loadViperShadowParity(t, args, emptyEnv, setting.Field)
			} else {
				empty, _ = loadViperShadowParity(t, args, emptyEnv)
				assertSameConfigSetting(t, setting, empty, absent)
			}

			envValue := alternateConfigValue(t, setting, absent)
			configuredEnv := cloneEnv(baseEnv)
			configuredEnv[setting.Environment] = envValue
			configured, _ := loadViperShadowParity(t, args, configuredEnv)
			assertConfigSetting(t, setting, configured, envValue)

			flagValue := alternateConfigValue(t, setting, configured)
			flagged, _ := loadViperShadowParity(t, append(args, "--"+canonicalFlag(t, setting)+"="+flagValue), configuredEnv)
			assertConfigSetting(t, setting, flagged, flagValue)

			if setting.Type == "integer" || setting.Type == "number" || setting.Type == "boolean" {
				invalidValue := "not-a-number"
				if setting.Type == "boolean" {
					invalidValue = "not-a-bool"
				}
				invalidEnv := cloneEnv(baseEnv)
				invalidEnv[setting.Environment] = invalidValue
				invalid, log := loadViperShadowParity(t, args, invalidEnv)
				assertSameConfigSetting(t, setting, invalid, absent)
				if !strings.Contains(log, setting.Environment) {
					t.Errorf("invalid %s warning does not identify %s: %q", setting.Type, setting.Environment, log)
				}
				if strings.Contains(log, invalidValue) {
					t.Errorf("invalid %s warning exposes configured value %q: %q", setting.Type, invalidValue, log)
				}
			}

			for _, alias := range setting.DeprecatedFlags {
				aliasValue := alternateConfigValue(t, setting, absent)
				aliased, _ := loadViperShadowParity(t, append(args, "--"+alias+"="+aliasValue), baseEnv)
				assertConfigSetting(t, setting, aliased, aliasValue)

				canonicalValue := alternateConfigValue(t, setting, aliased)
				deprecatedThenCanonical, _ := loadViperShadowParity(t, append(args, "--"+alias+"="+aliasValue, "--"+canonicalFlag(t, setting)+"="+canonicalValue), baseEnv)
				assertConfigSetting(t, setting, deprecatedThenCanonical, canonicalValue)

				canonicalThenDeprecated, _ := loadViperShadowParity(t, append(args, "--"+canonicalFlag(t, setting)+"="+canonicalValue, "--"+alias+"="+aliasValue), baseEnv)
				assertConfigSetting(t, setting, canonicalThenDeprecated, aliasValue)
			}
		})
	}
}

func TestViperShadowRedactsInvalidMCPDocsURL(t *testing.T) {
	const invalidURL = "not-a-valid-url"
	_, env := catalogParityInputs(ConfigurationSetting{})
	env["HITKEEP_MCP_DOCS_URL"] = invalidURL
	conf, log := loadViperShadowParity(t, nil, env)
	if conf.MCPDocsURL != "https://hitkeep.com" {
		t.Fatalf("invalid MCP docs URL = %q, want default", conf.MCPDocsURL)
	}
	if !strings.Contains(log, "Invalid MCP docs URL, using default") {
		t.Fatalf("missing invalid MCP docs URL warning: %q", log)
	}
	if strings.Contains(log, invalidURL) {
		t.Fatalf("invalid MCP docs URL leaked into warning: %q", log)
	}
}

func TestViperShadowExplicitFilePrecedence(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeShadowConfig(t, fs, "config.yaml", "http-addr: ':7070'\nmail-port: 2020\napi-rate-limit: 7.5\nhealthcheck: true\n")
	conf, err := loadViper(
		[]string{"--http-addr=:9090"},
		mapEnv(map[string]string{"HITKEEP_MAIL_PORT": "3030"}),
		fs,
		"config.yaml",
	)
	if err != nil {
		t.Fatal(err)
	}
	if conf.HTTPAddr != ":9090" || conf.MailPort != 3030 || conf.ApiRateLimit != 7.5 || !conf.Healthcheck {
		t.Fatalf("precedence mismatch: %#v", conf)
	}
}

func TestViperShadowRejectsInvalidExplicitFilesWithoutValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "malformed", content: "mail-port: [\n"},
		{name: "unknown key", content: "unknown-setting: true\n"},
		{name: "invalid type", content: "jwt-secret: top-secret\nmail-port: not-a-number\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			writeShadowConfig(t, fs, "config.yaml", tt.content)
			_, err := loadViper(nil, mapEnv(nil), fs, "config.yaml")
			if err == nil {
				t.Fatal("expected configuration error")
			}
			for _, value := range []string{"top-secret", "not-a-number"} {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error exposes configured value %q: %v", value, err)
				}
			}
		})
	}
}

func TestViperShadowRejectsNonCanonicalTopLevelKeys(t *testing.T) {
	for _, key := range []string{"mail_port", "MAIL-PORT", "Mail-Port"} {
		t.Run(key, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			writeShadowConfig(t, fs, "config.yaml", key+": 2525\n")
			_, err := loadViper(nil, mapEnv(nil), fs, "config.yaml")
			if err == nil || !strings.Contains(err.Error(), `unknown configuration key "`+key+`"`) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestViperShadowDoesNotDiscoverFilesImplicitly(t *testing.T) {
	fs := afero.NewMemMapFs()
	writeShadowConfig(t, fs, "hitkeep.yaml", "http-addr: ':6060'\nhealthcheck: true\n")
	conf, err := loadViper([]string{"--healthcheck"}, mapEnv(nil), fs, "")
	if err != nil {
		t.Fatal(err)
	}
	if conf.HTTPAddr != ":8080" {
		t.Fatalf("implicit file changed HTTP address to %q", conf.HTTPAddr)
	}
}

func TestViperShadowLoadsEveryCatalogType(t *testing.T) {
	var contents strings.Builder
	for _, setting := range Catalog().Settings {
		if setting.CloudOnly && !includeCloudConfigFields() {
			continue
		}
		contents.WriteString(setting.ConfigFileKey)
		contents.WriteString(": ")
		switch setting.Type {
		case "string":
			contents.WriteString(strconv.Quote("configured"))
		case "integer":
			contents.WriteString("42")
		case "boolean":
			contents.WriteString("true")
		case "number":
			contents.WriteString("12.5")
		default:
			t.Fatalf("unsupported catalog type %q", setting.Type)
		}
		contents.WriteByte('\n')
	}

	fs := afero.NewMemMapFs()
	writeShadowConfig(t, fs, "all.yaml", contents.String())
	conf, err := loadViper(nil, mapEnv(nil), fs, "all.yaml")
	if err != nil {
		t.Fatal(err)
	}
	value := reflect.ValueOf(conf).Elem()
	for _, setting := range Catalog().Settings {
		if setting.CloudOnly && !includeCloudConfigFields() {
			continue
		}
		field := value.FieldByName(setting.Field)
		switch setting.Type {
		case "string":
			if field.String() != "configured" {
				t.Errorf("%s = %q", setting.Field, field.String())
			}
		case "integer":
			if field.Int() != 42 {
				t.Errorf("%s = %d", setting.Field, field.Int())
			}
		case "boolean":
			if !field.Bool() {
				t.Errorf("%s = false", setting.Field)
			}
		case "number":
			if field.Float() != 12.5 {
				t.Errorf("%s = %f", setting.Field, field.Float())
			}
		}
	}
}

func loadViperShadowParity(t *testing.T, args []string, env map[string]string, generatedFields ...string) (*Config, string) {
	t.Helper()
	getEnv := mapEnv(env)
	var legacyLog, shadowLog bytes.Buffer
	logOptions := &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return attr
	}}
	legacy := load(args, getEnv, slog.New(slog.NewTextHandler(&legacyLog, logOptions)))
	shadow, err := loadViper(args, getEnv, afero.NewMemMapFs(), "", slog.New(slog.NewTextHandler(&shadowLog, logOptions)))
	if err != nil {
		t.Fatal(err)
	}
	legacyComparable, shadowComparable := *legacy, *shadow
	for _, name := range generatedFields {
		legacyField := configField(t, name, &legacyComparable)
		shadowField := configField(t, name, &shadowComparable)
		if legacyField.String() == "" || shadowField.String() == "" {
			t.Fatalf("generated %s is empty", name)
		}
		legacyField.SetZero()
		shadowField.SetZero()
	}
	if !reflect.DeepEqual(&shadowComparable, &legacyComparable) {
		t.Fatalf("shadow config differs from legacy\nshadow: %#v\nlegacy: %#v", shadow, legacy)
	}
	if shadowLog.String() != legacyLog.String() {
		t.Fatalf("shadow logs differ from legacy\nshadow: %q\nlegacy: %q", shadowLog.String(), legacyLog.String())
	}
	return shadow, shadowLog.String()
}

func catalogParityInputs(setting ConfigurationSetting) ([]string, map[string]string) {
	env := map[string]string{
		"HITKEEP_JWT_SECRET":          "fixed-secret",
		"HITKEEP_NODE_NAME":           "fixed-node",
		"HITKEEP_DUCKDB_MEMORY_LIMIT": "none",
		"HITKEEP_DUCKDB_THREADS":      "2",
		"HITKEEP_TRUSTED_PROXIES":     "127.0.0.1/32",
		"HITKEEP_MCP_DOCS_URL":        "https://example.com",
		"HITKEEP_DB_RECOVERY_PATH":    "/recovery",
	}
	if setting.Field != "JWTSecret" && setting.Field != "NodeName" {
		delete(env, setting.Environment)
	}
	return nil, env
}

func cloneEnv(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values)+1)
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func canonicalFlag(t *testing.T, setting ConfigurationSetting) string {
	t.Helper()
	if setting.Flag == "" {
		t.Fatalf("catalog field %q has no canonical flag", setting.Field)
	}
	return setting.Flag
}

var exceptionalViperParityStringValues = map[string][]string{
	"HTTPAddr":                       {":8181", ":8282"},
	"BindAddr":                       {"127.0.0.1:7946", "127.0.0.1:7947"},
	"JoinAddr":                       {"127.0.0.1:7948", "127.0.0.1:7949"},
	"NodeName":                       {"parity-node", "parity-node-next"},
	"DuckDBMemoryLimit":              {"1024MiB", "2048MiB"},
	"DBRecoveryPath":                 {"/parity-recovery", "/parity-recovery-next"},
	"DataPath":                       {"parity-data", "parity-data-next"},
	"ArchivePath":                    {"parity-archive", "parity-archive-next"},
	"PublicURL":                      {"https://parity.example", "https://next.parity.example"},
	"LogLevel":                       {"debug", "warn"},
	"TrustedProxies":                 {"127.0.0.1/32", "10.0.0.0/8"},
	"NSQTCPAddress":                  {"127.0.0.1:4150", "127.0.0.1:4250"},
	"NSQHTTPAddress":                 {"127.0.0.1:4151", "127.0.0.1:4251"},
	"SocialMicrosoftTenant":          {"organizations", "consumers"},
	"SpamFilterPath":                 {"parity-spam-filter.json", "parity-spam-filter-next.json"},
	"GoogleSearchConsoleRedirectURL": {"https://parity.example/callback", "https://next.parity.example/callback"},
	"BackupPath":                     {"parity-backups", "parity-backups-next"},
	"S3Endpoint":                     {"https://s3.parity.example", "https://s3-next.parity.example"},
	"S3URLStyle":                     {"path", "vhost"},
	"MCPPath":                        {"/mcp-parity", "/mcp-parity-next"},
	"MCPDocsURL":                     {"https://docs.parity.example", "https://docs-next.parity.example"},
	"CustomTrackingDNSTarget":        {"tracking.parity.example", "tracking-next.parity.example"},
	"CustomTrackingTLSMode":          {"external", "caddy-on-demand"},
	"AIBaseURL":                      {"https://ai.parity.example", "https://ai-next.parity.example"},
}

func alternateConfigValue(t *testing.T, setting ConfigurationSetting, conf *Config) string {
	t.Helper()
	field := configSettingValue(t, setting, conf)
	switch setting.Type {
	case "string":
		values := exceptionalViperParityStringValues[setting.Field]
		if len(values) == 0 {
			values = []string{"configured-viper-parity", "configured-viper-parity-next"}
		}
		for _, value := range values {
			if field.String() != value {
				return value
			}
		}
		t.Fatalf("no alternate test value for %s", setting.Field)
	case "integer":
		if field.Int() == 42 {
			return "43"
		}
		return "42"
	case "boolean":
		return strconv.FormatBool(!field.Bool())
	case "number":
		if field.Float() == 12.5 {
			return "13.5"
		}
		return "12.5"
	default:
		t.Fatalf("unsupported catalog type %q", setting.Type)
	}
	return ""
}

func assertConfigSetting(t *testing.T, setting ConfigurationSetting, conf *Config, want string) {
	t.Helper()
	field := configSettingValue(t, setting, conf)
	switch setting.Type {
	case "string":
		if field.String() != want {
			t.Errorf("%s = %q, want %q", setting.Field, field.String(), want)
		}
	case "integer":
		wantValue, err := strconv.ParseInt(want, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if field.Int() != wantValue {
			t.Errorf("%s = %d, want %d", setting.Field, field.Int(), wantValue)
		}
	case "boolean":
		wantValue, err := strconv.ParseBool(want)
		if err != nil {
			t.Fatal(err)
		}
		if field.Bool() != wantValue {
			t.Errorf("%s = %t, want %t", setting.Field, field.Bool(), wantValue)
		}
	case "number":
		wantValue, err := strconv.ParseFloat(want, 64)
		if err != nil {
			t.Fatal(err)
		}
		if field.Float() != wantValue {
			t.Errorf("%s = %f, want %f", setting.Field, field.Float(), wantValue)
		}
	default:
		t.Fatalf("unsupported catalog type %q", setting.Type)
	}
}

func assertSameConfigSetting(t *testing.T, setting ConfigurationSetting, got, want *Config) {
	t.Helper()
	if gotValue, wantValue := configSettingValue(t, setting, got).Interface(), configSettingValue(t, setting, want).Interface(); !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s = %#v, want %#v", setting.Field, gotValue, wantValue)
	}
}

func hasGeneratedValue(setting ConfigurationSetting) bool {
	return setting.Field == "JWTSecret" || setting.Field == "NodeName"
}

func configSettingValue(t *testing.T, setting ConfigurationSetting, conf *Config) reflect.Value {
	t.Helper()
	return configField(t, setting.Field, conf)
}

func configField(t *testing.T, name string, conf *Config) reflect.Value {
	t.Helper()
	field := reflect.ValueOf(conf).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("config field %q does not exist", name)
	}
	return field
}

func mapEnv(values map[string]string) func(string, string) string {
	return func(key, fallback string) string {
		if value := values[key]; value != "" {
			return value
		}
		return fallback
	}
}

func writeShadowConfig(t *testing.T, fs afero.Fs, name, contents string) {
	t.Helper()
	if err := afero.WriteFile(fs, name, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

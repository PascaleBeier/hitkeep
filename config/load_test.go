package config

import (
	"bytes"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// TestLoadCatalogSettingSources characterizes every catalog setting through the
// 2.x sources: default, empty/invalid/deprecated environment, file, canonical
// and single-dash flags, and deprecated flag aliases in argument order.
func TestLoadCatalogSettingSources(t *testing.T) {
	cloudSettings := 0
	for _, setting := range Catalog().Settings {
		t.Run(setting.Field, func(t *testing.T) {
			env := fixedGeneratedEnv(setting)
			absent, _ := loadForTest(t, nil, env, "")
			if setting.CloudOnly {
				cloudSettings++
				if !includeCloudConfigFields() {
					value := alternateConfigValue(t, setting, absent)
					ignored, _ := loadForTest(t, []string{"--" + setting.Flag + "=" + value}, with(env, setting.Environment, value), "")
					assertSameConfigSetting(t, setting, ignored, absent)
					return
				}
			}
			flag := func(value string) []string { return []string{"--" + setting.Flag + "=" + value} }
			if setting.Environment == "" {
				value := alternateConfigValue(t, setting, absent)
				flagged, _ := loadForTest(t, flag(value), env, "")
				assertConfigSetting(t, setting, flagged, value)
				return
			}
			if !hasGeneratedValue(setting) {
				empty, _ := loadForTest(t, nil, with(env, setting.Environment, ""), "")
				assertSameConfigSetting(t, setting, empty, absent)
			}

			envValue := alternateConfigValue(t, setting, absent)
			configuredEnv := with(env, setting.Environment, envValue)
			configured, _ := loadForTest(t, nil, configuredEnv, "")
			assertConfigSetting(t, setting, configured, envValue)
			for _, deprecated := range setting.DeprecatedEnvironments {
				fromDeprecated, _ := loadForTest(t, nil, with(env, deprecated, envValue), "")
				assertConfigSetting(t, setting, fromDeprecated, envValue)
			}

			fileValue := alternateConfigValue(t, setting, configured)
			file := setting.ConfigFileKey + ": " + yamlScalar(setting, fileValue) + "\n"
			fromFile, _ := loadForTest(t, nil, with(env, setting.Environment, ""), file)
			assertConfigSetting(t, setting, fromFile, fileValue)
			envOverFile, _ := loadForTest(t, nil, configuredEnv, file)
			assertConfigSetting(t, setting, envOverFile, envValue)

			flagValue := alternateConfigValue(t, setting, configured)
			flagged, _ := loadForTest(t, flag(flagValue), configuredEnv, file)
			assertConfigSetting(t, setting, flagged, flagValue)
			singleDash, _ := loadForTest(t, []string{"-" + setting.Flag + "=" + flagValue}, configuredEnv, "")
			assertConfigSetting(t, setting, singleDash, flagValue)

			if setting.Type != "string" {
				const invalid = "not-a-value"
				fromInvalid, log := loadForTest(t, nil, with(env, setting.Environment, invalid), file)
				assertConfigSetting(t, setting, fromInvalid, fileValue)
				if !strings.Contains(log, setting.Environment) || strings.Contains(log, invalid) {
					t.Errorf("invalid %s warning = %q, want key without value", setting.Environment, log)
				}
			}

			for _, alias := range setting.DeprecatedFlags {
				aliasValue := alternateConfigValue(t, setting, absent)
				aliased, _ := loadForTest(t, []string{"-" + alias + "=" + aliasValue}, env, "")
				assertConfigSetting(t, setting, aliased, aliasValue)
				canonicalValue := alternateConfigValue(t, setting, aliased)
				canonicalLast, _ := loadForTest(t, []string{"--" + alias + "=" + aliasValue, "--" + setting.Flag + "=" + canonicalValue}, env, "")
				assertConfigSetting(t, setting, canonicalLast, canonicalValue)
				aliasLast, _ := loadForTest(t, []string{"--" + setting.Flag + "=" + canonicalValue, "--" + alias + "=" + aliasValue}, env, "")
				assertConfigSetting(t, setting, aliasLast, aliasValue)
			}
		})
	}
	if includeCloudConfigFields() && cloudSettings == 0 {
		t.Fatal("cloud build has no cloud-only configuration descriptors")
	}
}

// TestLoadKeepsLegacyFlagGrammar pins the stdlib-flag grammar that 2.x accepted.
func TestLoadKeepsLegacyFlagGrammar(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{name: "single dash with separate values", args: []string{"-http-addr", ":9191", "-mail-port=3535", "-db", "single.db"}, want: map[string]any{"HTTPAddr": ":9191", "MailPort": 3535, "DBPath": "single.db"}},
		{name: "unknown flag stops parsing", args: []string{"-http-addr", ":9191", "--nope", "-mail-port=3535"}, want: map[string]any{"HTTPAddr": ":9191", "MailPort": 587}},
		{name: "positional stops parsing", args: []string{"extra", "-http-addr", ":9191"}, want: map[string]any{"HTTPAddr": ":8080"}},
		{name: "terminator stops parsing", args: []string{"--", "-http-addr", ":9191"}, want: map[string]any{"HTTPAddr": ":8080"}},
		{name: "help stops parsing", args: []string{"-h", "-http-addr", ":9191"}, want: map[string]any{"HTTPAddr": ":8080"}},
		{name: "long help stops parsing", args: []string{"-help", "-http-addr", ":9191"}, want: map[string]any{"HTTPAddr": ":8080"}},
		{name: "flag-like value", args: []string{"-join-addr", "-db", "-http-addr=:9191"}, want: map[string]any{"JoinAddr": "-db", "HTTPAddr": ":9191"}},
		{name: "base prefixed integers", args: []string{"-mail-port", "0x1f", "--api-burst=0o17"}, want: map[string]any{"MailPort": 31, "ApiBurst": 15}},
		{name: "explicit bool values", args: []string{"-s3-use-ssl=false", "--mcp-enabled=1"}, want: map[string]any{"S3UseSSL": false, "MCPEnabled": true}},
		{name: "bool does not consume next value", args: []string{"-mcp-enabled", "false", "-http-addr", ":9191"}, want: map[string]any{"MCPEnabled": true, "HTTPAddr": ":8080"}},
		{name: "missing value keeps earlier flags", args: []string{"-http-addr=:9191", "-db"}, want: map[string]any{"HTTPAddr": ":9191", "DBPath": "hitkeep.db"}},
		{name: "triple dash is rejected", args: []string{"---http-addr=:9191"}, want: map[string]any{"HTTPAddr": ":8080"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf, _ := loadForTest(t, append([]string{"-healthcheck"}, test.args...), nil, "")
			for field, want := range test.want {
				if got := configField(t, field, conf).Interface(); got != want {
					t.Errorf("%s = %#v, want %#v", field, got, want)
				}
			}
		})
	}
}

func TestLoadArgsReadsExplicitOSFile(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(configFile, []byte("http-addr: ':7070'\nmail-port: 2020\napi-rate-limit: 7.5\nhealthcheck: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HITKEEP_MAIL_PORT", "3030")
	t.Setenv("HITKEEP_MCP_DOCS_URL", "")

	conf, err := LoadArgs(afero.NewOsFs(), []string{"--http-addr=:9090"}, configFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if conf.HTTPAddr != ":9090" || conf.MailPort != 3030 || conf.ApiRateLimit != 7.5 || !conf.Healthcheck || conf.MCPDocsURL != "https://hitkeep.com" {
		t.Fatalf("configuration precedence mismatch: %#v", conf)
	}
}

func TestLoadRedactsInvalidMCPDocsURL(t *testing.T) {
	const invalidURL = "not-a-valid-url"
	conf, log := loadForTest(t, nil, with(fixedGeneratedEnv(ConfigurationSetting{}), "HITKEEP_MCP_DOCS_URL", invalidURL), "")
	if conf.MCPDocsURL != "https://hitkeep.com" {
		t.Fatalf("invalid MCP docs URL = %q, want default", conf.MCPDocsURL)
	}
	if !strings.Contains(log, "Invalid MCP docs URL, using default") || strings.Contains(log, invalidURL) {
		t.Fatalf("invalid MCP docs URL warning = %q", log)
	}
}

func TestLoadRejectsInvalidExplicitFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "malformed", content: "mail-port: [\n", want: "invalid YAML"},
		{name: "unknown key", content: "unknown-setting: true\n", want: `unknown configuration key "unknown-setting"`},
		{name: "invalid type", content: "jwt-secret: top-secret\nmail-port: not-a-number\n", want: "invalid type"},
		{name: "underscore key", content: "mail_port: 2525\n", want: `unknown configuration key "mail_port"`},
		{name: "uppercase key", content: "MAIL-PORT: 2525\n", want: `unknown configuration key "MAIL-PORT"`},
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
			if err := afero.WriteFile(fs, "config.yaml", []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadArgs(fs, nil, "config.yaml", nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadArgs() error = %v, want substring %q", err, test.want)
			}
			for _, value := range []string{"top-secret", "not-a-number"} {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error exposes configured value %q: %v", value, err)
				}
			}
		})
	}
}

func TestLoadDoesNotDiscoverFilesImplicitly(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "hitkeep.yaml", []byte("http-addr: ':6060'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf, err := LoadArgs(fs, []string{"--healthcheck"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if conf.HTTPAddr != ":8080" {
		t.Fatalf("implicit file changed HTTP address to %q", conf.HTTPAddr)
	}
}

func TestFlagSetHidesDeprecatedAliases(t *testing.T) {
	var conf Config
	flags := newFlagSet(reflect.ValueOf(&conf).Elem(), activeSettings())
	if healthcheck := flags.Lookup("healthcheck"); healthcheck == nil || healthcheck.DefValue != "false" {
		t.Fatalf("healthcheck flag = %+v", healthcheck)
	}
	if http := flags.Lookup("http"); http == nil || http.Name != "http-addr" {
		t.Fatalf("deprecated --http resolves to %+v, want http-addr", http)
	}
	var usage bytes.Buffer
	flags.SetOutput(&usage)
	flags.PrintDefaults()
	if strings.Contains(usage.String(), "--http ") {
		t.Fatalf("usage lists deprecated alias:\n%s", usage.String())
	}
}

// loadForTest replaces every catalog environment variable, loads an optional
// in-memory file, and returns the time-free log output.
func loadForTest(t *testing.T, args []string, env map[string]string, file string) (*Config, string) {
	t.Helper()
	fs := afero.NewMemMapFs()
	configFile := ""
	if file != "" {
		configFile = "config.yaml"
		if err := afero.WriteFile(fs, configFile, []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return attr
	}}))
	conf, err := loadWithEnv(t, args, mapEnv(env), fs, configFile, logger)
	if err != nil {
		t.Fatal(err)
	}
	return conf, log.String()
}

// fixedGeneratedEnv pins generated and host-derived values so loads compare equal.
func fixedGeneratedEnv(setting ConfigurationSetting) map[string]string {
	env := map[string]string{
		"HITKEEP_JWT_SECRET":          "fixed-secret",
		"HITKEEP_NODE_NAME":           "fixed-node",
		"HITKEEP_DUCKDB_MEMORY_LIMIT": "none",
		"HITKEEP_DUCKDB_THREADS":      "2",
		"HITKEEP_TRUSTED_PROXIES":     "127.0.0.1/32",
		"HITKEEP_MCP_DOCS_URL":        "https://example.com",
		"HITKEEP_DB_RECOVERY_PATH":    "/recovery",
	}
	if !hasGeneratedValue(setting) {
		delete(env, setting.Environment)
	}
	return env
}

func with(env map[string]string, key, value string) map[string]string {
	clone := maps.Clone(env)
	if clone == nil {
		clone = map[string]string{}
	}
	clone[key] = value
	return clone
}

func yamlScalar(setting ConfigurationSetting, value string) string {
	if setting.Type == "string" {
		return strconv.Quote(value)
	}
	return value
}

var alternateStringValues = map[string][]string{
	"HTTPAddr":                       {":8181", ":8282", ":8383"},
	"BindAddr":                       {"127.0.0.1:7946", "127.0.0.1:7947", "127.0.0.1:7950"},
	"JoinAddr":                       {"127.0.0.1:7948", "127.0.0.1:7949", "127.0.0.1:7951"},
	"NodeName":                       {"parity-node", "parity-node-next", "parity-node-last"},
	"DuckDBMemoryLimit":              {"1024MiB", "2048MiB", "3072MiB"},
	"DBRecoveryPath":                 {"/parity-recovery", "/parity-recovery-next", "/parity-recovery-last"},
	"DataPath":                       {"parity-data", "parity-data-next", "parity-data-last"},
	"ArchivePath":                    {"parity-archive", "parity-archive-next", "parity-archive-last"},
	"PublicURL":                      {"https://parity.example", "https://next.parity.example", "https://last.parity.example"},
	"LogLevel":                       {"debug", "warn", "error"},
	"TrustedProxies":                 {"127.0.0.1/32", "10.0.0.0/8", "192.168.0.0/16"},
	"NSQTCPAddress":                  {"127.0.0.1:4150", "127.0.0.1:4250", "127.0.0.1:4350"},
	"NSQHTTPAddress":                 {"127.0.0.1:4151", "127.0.0.1:4251", "127.0.0.1:4351"},
	"SocialMicrosoftTenant":          {"organizations", "consumers", "common"},
	"SpamFilterPath":                 {"parity-spam-filter.json", "parity-spam-filter-next.json", "parity-spam-filter-last.json"},
	"GoogleSearchConsoleRedirectURL": {"https://parity.example/callback", "https://next.parity.example/callback", "https://last.parity.example/callback"},
	"BackupPath":                     {"parity-backups", "parity-backups-next", "parity-backups-last"},
	"S3Endpoint":                     {"https://s3.parity.example", "https://s3-next.parity.example", "https://s3-last.parity.example"},
	"S3URLStyle":                     {"path", "vhost", ""},
	"MCPPath":                        {"/mcp-parity", "/mcp-parity-next", "/mcp-parity-last"},
	"MCPDocsURL":                     {"https://docs.parity.example", "https://docs-next.parity.example", "https://docs-last.parity.example"},
	"CustomTrackingDNSTarget":        {"tracking.parity.example", "tracking-next.parity.example", "tracking-last.parity.example"},
	"CustomTrackingTLSMode":          {"external", "caddy-on-demand"},
	"AIBaseURL":                      {"https://ai.parity.example", "https://ai-next.parity.example", "https://ai-last.parity.example"},
}

// alternateConfigValue returns a valid value that differs from the setting's current value.
func alternateConfigValue(t *testing.T, setting ConfigurationSetting, conf *Config) string {
	t.Helper()
	field := configField(t, setting.Field, conf)
	switch setting.Type {
	case "string":
		values := alternateStringValues[setting.Field]
		if len(values) == 0 {
			values = []string{"configured", "configured-next"}
		}
		for _, value := range values {
			if field.String() != value {
				return value
			}
		}
		t.Fatalf("no alternate test value for %s", setting.Field)
	case "integer":
		return strconv.FormatInt(field.Int()%1000+42, 10)
	case "boolean":
		return strconv.FormatBool(!field.Bool())
	case "number":
		return strconv.FormatFloat(field.Float()+1.5, 'f', -1, 64)
	default:
		t.Fatalf("unsupported catalog type %q", setting.Type)
	}
	return ""
}

func assertConfigSetting(t *testing.T, setting ConfigurationSetting, conf *Config, want string) {
	t.Helper()
	wantConf := reflect.New(reflect.TypeFor[Config]()).Elem()
	if !setField(wantConf.FieldByName(setting.Field), want) {
		t.Fatalf("invalid expected %s value %q", setting.Field, want)
	}
	if got, want := configField(t, setting.Field, conf).Interface(), wantConf.FieldByName(setting.Field).Interface(); got != want {
		t.Errorf("%s = %#v, want %#v", setting.Field, got, want)
	}
}

func assertSameConfigSetting(t *testing.T, setting ConfigurationSetting, got, want *Config) {
	t.Helper()
	if gotValue, wantValue := configField(t, setting.Field, got).Interface(), configField(t, setting.Field, want).Interface(); gotValue != wantValue {
		t.Errorf("%s = %#v, want %#v", setting.Field, gotValue, wantValue)
	}
}

func hasGeneratedValue(setting ConfigurationSetting) bool {
	return setting.Field == "JWTSecret" || setting.Field == "NodeName"
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

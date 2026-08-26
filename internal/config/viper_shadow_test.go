package config

import (
	"bytes"
	"log/slog"
	"os"
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

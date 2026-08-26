package config

import (
	"bytes"
	"log/slog"
	"reflect"
	"testing"
)

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
			env:  map[string]string{"HITKEEP_MAIL_PORT": "invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getEnv := func(key, fallback string) string {
				if value := tt.env[key]; value != "" {
					return value
				}
				return fallback
			}
			var legacyLog, shadowLog bytes.Buffer
			legacy := load(tt.args, getEnv, slog.New(slog.NewTextHandler(&legacyLog, nil)))
			shadow := loadViperShadow(tt.args, getEnv, slog.New(slog.NewTextHandler(&shadowLog, nil)))
			if !reflect.DeepEqual(shadow, legacy) {
				t.Fatalf("shadow config differs from legacy\nshadow: %#v\nlegacy: %#v", shadow, legacy)
			}
			if shadowLog.String() != legacyLog.String() {
				t.Fatalf("shadow logs differ from legacy\nshadow: %q\nlegacy: %q", shadowLog.String(), legacyLog.String())
			}
		})
	}
}

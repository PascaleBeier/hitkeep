package config

import (
	"net/netip"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		check      func(*Config) bool
		errMessage string
	}{
		{
			name: "Defaults",
			args: []string{},
			env:  map[string]string{},
			check: func(c *Config) bool {
				return c.HTTPAddr == ":8080" &&
					c.MailDriver == "smtp" &&
					c.IngestBurst == 40 &&
					c.WebhookRateLimit == 30.0 &&
					c.WebhookBurst == 60 &&
					c.AuthRememberMeDays == 30 &&
					c.AuthSessionMinutes == 15 &&
					c.AuthSessionWarningSeconds == 120 &&
					len(c.JWTSecret) >= 32
			},
			errMessage: "Defaults failed",
		},
		{
			name: "Environment Variables Override Defaults",
			args: []string{},
			env: map[string]string{
				"HITKEEP_HTTP_ADDR":                    ":9000",
				"HITKEEP_MAIL_PORT":                    "25",
				"HITKEEP_INGEST_RATE_LIMIT":            "100.5",
				"HITKEEP_MAIL_DRIVER":                  "log",
				"HITKEEP_WEBHOOK_RATE_LIMIT":           "55.5",
				"HITKEEP_WEBHOOK_BURST":                "80",
				"HITKEEP_AUTH_REMEMBER_ME_DAYS":        "14",
				"HITKEEP_AUTH_SESSION_MINUTES":         "45",
				"HITKEEP_AUTH_SESSION_WARNING_SECONDS": "180",
			},
			check: func(c *Config) bool {
				return c.HTTPAddr == ":9000" &&
					c.MailPort == 25 &&
					c.IngestRateLimit == 100.5 &&
					c.MailDriver == "log" &&
					c.WebhookRateLimit == 55.5 &&
					c.WebhookBurst == 80 &&
					c.AuthRememberMeDays == 14 &&
					c.AuthSessionMinutes == 45 &&
					c.AuthSessionWarningSeconds == 180
			},
			errMessage: "Environment variables did not override defaults",
		},
		{
			name: "Flags Override Environment Variables",
			args: []string{"-http", ":9999", "-mail-port", "1025", "-auth-session-minutes", "30", "-auth-remember-me-days", "7"},
			env: map[string]string{
				"HITKEEP_HTTP_ADDR":             ":8080",
				"HITKEEP_MAIL_PORT":             "587",
				"HITKEEP_AUTH_SESSION_MINUTES":  "45",
				"HITKEEP_AUTH_REMEMBER_ME_DAYS": "30",
			},
			check: func(c *Config) bool {
				return c.HTTPAddr == ":9999" && c.MailPort == 1025 && c.AuthSessionMinutes == 30 && c.AuthRememberMeDays == 7
			},
			errMessage: "Flags did not override environment variables",
		},
		{
			name: "Boolean Flags and Env",
			args: []string{"-healthcheck"},
			env: map[string]string{
				"HITKEEP_MAIL_INSECURE_SKIP_VERIFY": "true",
			},
			check: func(c *Config) bool {
				return c.Healthcheck == true && c.MailInsecureSkipVerify == true
			},
			errMessage: "Boolean logic failed",
		},
		{
			name: "JWT Secret Generation",
			args: []string{},
			env:  map[string]string{},
			check: func(c *Config) bool {
				return c.JWTSecret != ""
			},
			errMessage: "JWT Secret was not generated",
		},
		{
			name: "JWT Secret Supplied",
			args: []string{},
			env: map[string]string{
				"HITKEEP_JWT_SECRET": "super-secret-key",
			},
			check: func(c *Config) bool {
				return c.JWTSecret == "super-secret-key"
			},
			errMessage: "Supplied JWT secret was ignored",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock Env Lookup
			mockEnv := func(key, fallback string) string {
				if val, ok := tc.env[key]; ok {
					return val
				}
				return fallback
			}

			// Run Logic
			conf := load(tc.args, mockEnv)

			if !tc.check(conf) {
				t.Errorf("%s: %s", tc.name, tc.errMessage)
			}
		})
	}
}

func TestNormalizeAuthSessionConfig(t *testing.T) {
	conf := &Config{AuthSessionMinutes: -1, AuthRememberMeDays: -2, AuthSessionWarningSeconds: 5}
	NormalizeAuthSessionConfig(conf)
	if conf.AuthSessionMinutes != 15 {
		t.Fatalf("expected default session minutes, got %d", conf.AuthSessionMinutes)
	}
	if conf.AuthRememberMeDays != 30 {
		t.Fatalf("expected default remember-me days, got %d", conf.AuthRememberMeDays)
	}
	if conf.AuthSessionWarningSeconds != 20 {
		t.Fatalf("expected warning to normalize to WCAG-safe minimum, got %d", conf.AuthSessionWarningSeconds)
	}

	conf = &Config{AuthSessionMinutes: 10, AuthSessionWarningSeconds: 900}
	NormalizeAuthSessionConfig(conf)
	if conf.AuthSessionWarningSeconds != 300 {
		t.Fatalf("expected warning to stay before expiry, got %d", conf.AuthSessionWarningSeconds)
	}
}

func TestTrustedProxiesDefaultIsWildcard(t *testing.T) {
	conf := load([]string{}, func(key, fallback string) string {
		return fallback
	})

	if conf.TrustedProxies != "*" {
		t.Fatalf("expected default trusted proxies to be '*', got %q", conf.TrustedProxies)
	}
	if len(conf.GetTrustedProxyNetworks()) == 0 {
		t.Fatalf("expected trust-all proxy networks to be loaded by default")
	}
	if !conf.IsTrustedProxy(netip.MustParseAddr("203.0.113.10")) {
		t.Fatalf("expected default trusted proxies to trust public ipv4")
	}
}

func TestParseTrustedProxiesWildcard(t *testing.T) {
	networks := parseTrustedProxies("*")
	if len(networks) == 0 {
		t.Fatalf("expected wildcard to parse into trust-all proxy networks")
	}

	if !isIPInNetworksForTest(netip.MustParseAddr("198.51.100.20"), networks) {
		t.Fatalf("expected wildcard trusted proxies to include public ipv4")
	}
	if !isIPInNetworksForTest(netip.MustParseAddr("2001:db8::1"), networks) {
		t.Fatalf("expected wildcard trusted proxies to include ipv6")
	}
}

func TestLoadS3ConfigDefaults(t *testing.T) {
	conf := load([]string{}, func(key, fallback string) string {
		return fallback
	})

	if conf.S3AccessKeyID != "" {
		t.Fatalf("expected empty S3AccessKeyID by default, got %q", conf.S3AccessKeyID)
	}
	if conf.S3SecretAccessKey != "" {
		t.Fatalf("expected empty S3SecretAccessKey by default, got %q", conf.S3SecretAccessKey)
	}
	if conf.S3Region != "us-east-1" {
		t.Fatalf("expected S3Region default us-east-1, got %q", conf.S3Region)
	}
	if conf.S3Endpoint != "" {
		t.Fatalf("expected empty S3Endpoint by default, got %q", conf.S3Endpoint)
	}
	if conf.S3URLStyle != "" {
		t.Fatalf("expected empty S3URLStyle by default, got %q", conf.S3URLStyle)
	}
	if !conf.S3UseSSL {
		t.Fatalf("expected S3UseSSL default true, got false")
	}
}

func TestLoadS3ConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"HITKEEP_S3_ACCESS_KEY_ID":     "AKIAEXAMPLE",
		"HITKEEP_S3_SECRET_ACCESS_KEY": "secretkey123",
		"HITKEEP_S3_SESSION_TOKEN":     "tokenXYZ",
		"HITKEEP_S3_REGION":            "eu-central-1",
		"HITKEEP_S3_ENDPOINT":          "minio.local:9000",
		"HITKEEP_S3_URL_STYLE":         "path",
		"HITKEEP_S3_USE_SSL":           "false",
	}

	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if conf.S3AccessKeyID != "AKIAEXAMPLE" {
		t.Fatalf("expected S3AccessKeyID=AKIAEXAMPLE, got %q", conf.S3AccessKeyID)
	}
	if conf.S3SecretAccessKey != "secretkey123" {
		t.Fatalf("expected S3SecretAccessKey=secretkey123, got %q", conf.S3SecretAccessKey)
	}
	if conf.S3SessionToken != "tokenXYZ" {
		t.Fatalf("expected S3SessionToken=tokenXYZ, got %q", conf.S3SessionToken)
	}
	if conf.S3Region != "eu-central-1" {
		t.Fatalf("expected S3Region=eu-central-1, got %q", conf.S3Region)
	}
	if conf.S3Endpoint != "minio.local:9000" {
		t.Fatalf("expected S3Endpoint=minio.local:9000, got %q", conf.S3Endpoint)
	}
	if conf.S3URLStyle != "path" {
		t.Fatalf("expected S3URLStyle=path, got %q", conf.S3URLStyle)
	}
	if conf.S3UseSSL {
		t.Fatalf("expected S3UseSSL=false, got true")
	}
}

func TestLoadBackupConfigDefaults(t *testing.T) {
	conf := load([]string{}, func(key, fallback string) string {
		return fallback
	})

	if conf.BackupPath != "" {
		t.Fatalf("expected empty BackupPath by default, got %q", conf.BackupPath)
	}
	if conf.BackupIntervalMinutes != 60 {
		t.Fatalf("expected BackupIntervalMinutes default 60, got %d", conf.BackupIntervalMinutes)
	}
	if conf.BackupRetentionCount != 24 {
		t.Fatalf("expected BackupRetentionCount default 24, got %d", conf.BackupRetentionCount)
	}
}

func TestLoadBackupConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"HITKEEP_BACKUP_PATH":      "/tmp/backups",
		"HITKEEP_BACKUP_INTERVAL":  "30",
		"HITKEEP_BACKUP_RETENTION": "48",
	}

	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if conf.BackupPath != "/tmp/backups" {
		t.Fatalf("expected BackupPath=/tmp/backups, got %q", conf.BackupPath)
	}
	if conf.BackupIntervalMinutes != 30 {
		t.Fatalf("expected BackupIntervalMinutes=30, got %d", conf.BackupIntervalMinutes)
	}
	if conf.BackupRetentionCount != 48 {
		t.Fatalf("expected BackupRetentionCount=48, got %d", conf.BackupRetentionCount)
	}
}

func TestLoadMCPConfigDefaults(t *testing.T) {
	conf := load([]string{}, func(key, fallback string) string {
		return fallback
	})

	if conf.MCPEnabled {
		t.Fatalf("expected MCP disabled by default")
	}
	if conf.MCPPath != "/mcp" {
		t.Fatalf("expected default MCPPath /mcp, got %q", conf.MCPPath)
	}
	if conf.MCPMaxRangeDays != 366 {
		t.Fatalf("expected default MCPMaxRangeDays 366, got %d", conf.MCPMaxRangeDays)
	}
	if !conf.MCPDocsEnabled {
		t.Fatalf("expected MCP docs enabled by default")
	}
	if conf.MCPDocsURL != "https://hitkeep.com" {
		t.Fatalf("expected default MCPDocsURL, got %q", conf.MCPDocsURL)
	}
	if conf.MCPDocsCacheMinutes != 60 {
		t.Fatalf("expected default MCPDocsCacheMinutes 60, got %d", conf.MCPDocsCacheMinutes)
	}
}

func TestLoadMCPConfigFromEnvAndFlags(t *testing.T) {
	env := map[string]string{
		"HITKEEP_MCP_ENABLED":            "true",
		"HITKEEP_MCP_PATH":               "agent",
		"HITKEEP_MCP_MAX_RANGE_DAYS":     "90",
		"HITKEEP_MCP_DOCS_ENABLED":       "false",
		"HITKEEP_MCP_DOCS_URL":           "https://docs.example.com/",
		"HITKEEP_MCP_DOCS_CACHE_MINUTES": "15",
	}

	conf := load([]string{"-mcp-path", "/custom-mcp"}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if !conf.MCPEnabled {
		t.Fatalf("expected MCP enabled from env")
	}
	if conf.MCPPath != "/custom-mcp" {
		t.Fatalf("expected flag MCPPath to win, got %q", conf.MCPPath)
	}
	if conf.MCPMaxRangeDays != 90 {
		t.Fatalf("expected MCPMaxRangeDays 90, got %d", conf.MCPMaxRangeDays)
	}
	if conf.MCPDocsEnabled {
		t.Fatalf("expected MCP docs disabled from env")
	}
	if conf.MCPDocsURL != "https://docs.example.com" {
		t.Fatalf("expected trimmed MCPDocsURL, got %q", conf.MCPDocsURL)
	}
	if conf.MCPDocsCacheMinutes != 15 {
		t.Fatalf("expected MCPDocsCacheMinutes 15, got %d", conf.MCPDocsCacheMinutes)
	}
}

func TestLoadMCPConfigNormalizesInvalidValues(t *testing.T) {
	env := map[string]string{
		"HITKEEP_MCP_PATH":               "",
		"HITKEEP_MCP_MAX_RANGE_DAYS":     "-2",
		"HITKEEP_MCP_DOCS_URL":           "://bad",
		"HITKEEP_MCP_DOCS_CACHE_MINUTES": "0",
	}

	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if conf.MCPPath != "/mcp" {
		t.Fatalf("expected normalized MCPPath, got %q", conf.MCPPath)
	}
	if conf.MCPMaxRangeDays != 366 {
		t.Fatalf("expected normalized MCPMaxRangeDays, got %d", conf.MCPMaxRangeDays)
	}
	if conf.MCPDocsURL != "https://hitkeep.com" {
		t.Fatalf("expected normalized MCPDocsURL, got %q", conf.MCPDocsURL)
	}
	if conf.MCPDocsCacheMinutes != 60 {
		t.Fatalf("expected normalized MCPDocsCacheMinutes, got %d", conf.MCPDocsCacheMinutes)
	}
}

func TestLoadMCPConfigRejectsRootPath(t *testing.T) {
	env := map[string]string{
		"HITKEEP_MCP_PATH": "/",
	}

	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})

	if conf.MCPPath != "/mcp" {
		t.Fatalf("expected root MCPPath to normalize to /mcp, got %q", conf.MCPPath)
	}
}

func isIPInNetworksForTest(ip netip.Addr, networks []netip.Prefix) bool {
	for _, network := range networks {
		if network.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

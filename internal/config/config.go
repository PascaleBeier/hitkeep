package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ApiBurst                  int
	ApiRateLimit              float64
	AuthBurst                 int
	AuthRateLimit             float64
	AuthRememberMeDays        int
	AuthSessionMinutes        int
	AuthSessionWarningSeconds int
	WebhookBurst              int
	WebhookRateLimit          float64
	ArchivePath               string
	BindAddr                  string
	DataPath                  string
	DBPath                    string
	Healthcheck               bool
	HTTPAddr                  string
	IngestBurst               int
	IngestRateLimit           float64
	JoinAddr                  string
	//nolint:gosec // runtime configuration intentionally carries the JWT signing secret.
	JWTSecret                   string
	LogLevel                    string
	MailDriver                  string
	MailEncryption              string
	MailInsecureSkipVerify      bool
	MailFromAddress             string
	MailFromName                string
	MailHost                    string
	MailPassword                string
	MailPort                    int
	MailUsername                string
	MCPDocsCacheMinutes         int
	MCPDocsEnabled              bool
	MCPDocsURL                  string
	MCPEnabled                  bool
	MCPMaxRangeDays             int
	MCPPath                     string
	NodeName                    string
	NSQHTTPAddress              string
	NSQTCPAddress               string
	PublicURL                   string
	SpamFilterAutoUpdate        bool
	SpamFilterPath              string
	SpamFilterUpdateIntervalMin int
	Version                     string
	DataRetentionDays           int
	BackupPath                  string
	BackupIntervalMinutes       int
	BackupRetentionCount        int
	S3AccessKeyID               string
	S3SecretAccessKey           string
	S3SessionToken              string
	S3Region                    string
	S3Endpoint                  string
	S3URLStyle                  string
	S3UseSSL                    bool
	CloudHosted                 bool
	CloudSignupEnabled          bool
	CloudJurisdiction           string
	CloudRegion                 string
	CloudUpgradeURL             string
	CloudSupportURL             string
	CloudPlanCode               string
	CloudPlanName               string
	CloudMaxTeams               int
	CloudMaxSitesPerTeam        int
	CloudMaxRetentionDays       int
	CloudMaxTeamMembers         int
	CloudAllowSSO               bool
	CloudAllowCustomBranding    bool
	StripeSecretKey             string
	StripePublishableKey        string
	StripeWebhookSecret         string
	StripePortalConfigurationID string
	StripePriceProMonthly       string
	StripePriceBusinessMonthly  string
	CloudCheckoutSuccessURL     string
	CloudCheckoutCancelURL      string
	TrustedProxies              string
	trustedProxyNets            []netip.Prefix
}

// GetTrustedProxyNetworks returns the parsed trusted proxy networks.
func (c *Config) GetTrustedProxyNetworks() []netip.Prefix {
	return c.trustedProxyNets
}

// IsTrustedProxy checks if an IP is in the trusted proxy list.
func (c *Config) IsTrustedProxy(ip netip.Addr) bool {
	if len(c.trustedProxyNets) == 0 {
		return false
	}

	for _, network := range c.trustedProxyNets {
		if network.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

func Load() *Config {
	return load(os.Args[1:], func(key, fallback string) string {
		if val := os.Getenv(key); val != "" {
			return val
		}
		return fallback
	})
}

// load internal
func load(args []string, getEnv func(string, string) string) *Config {
	var conf Config

	fs := flag.NewFlagSet("hitkeep", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	getInt := func(key string, fallback int) int {
		val := getEnv(key, "")
		if val != "" {
			if i, err := strconv.Atoi(val); err == nil {
				return i
			}
			slog.Warn("Invalid integer in env var, using default", "key", key, "val", val, "default", fallback)
		}
		return fallback
	}

	getFloat := func(key string, fallback float64) float64 {
		val := getEnv(key, "")
		if val != "" {
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f
			}
			slog.Warn("Invalid float in env var, using default", "key", key, "val", val, "default", fallback)
		}
		return fallback
	}

	getBool := func(key string, fallback bool) bool {
		val := getEnv(key, "")
		if val != "" {
			if b, err := strconv.ParseBool(val); err == nil {
				return b
			}
			slog.Warn("Invalid boolean in env var, using default", "key", key, "val", val, "default", fallback)
		}
		return fallback
	}

	getInt64 := func(key string, fallback int64) int64 {
		val := getEnv(key, "")
		if val != "" {
			if i, err := strconv.ParseInt(val, 10, 64); err == nil {
				return i
			}
			slog.Warn("Invalid int64 in env var, using default", "key", key, "val", val, "default", fallback)
		}
		return fallback
	}

	defDB := getEnv("HITKEEP_DB_PATH", "hitkeep.db")
	defDataPath := getEnv("HITKEEP_DATA_PATH", "data")
	defArchive := getEnv("HITKEEP_ARCHIVE_PATH", "archive")
	defHTTP := getEnv("HITKEEP_HTTP_ADDR", ":8080")
	defBind := getEnv("HITKEEP_BIND_ADDR", "0.0.0.0:7946")
	defJoin := getEnv("HITKEEP_JOIN_ADDR", "")
	defPublicURL := getEnv("HITKEEP_PUBLIC_URL", "http://localhost:8080")
	defLogLevel := getEnv("HITKEEP_LOG_LEVEL", "info")
	defJWT := getEnv("HITKEEP_JWT_SECRET", "")
	defSpamFilterPath := getEnv("HITKEEP_SPAM_FILTER_PATH", "")
	defSpamFilterAutoUpdate := getBool("HITKEEP_SPAM_FILTER_AUTO_UPDATE", false)
	defSpamFilterUpdateInterval := getInt("HITKEEP_SPAM_FILTER_UPDATE_INTERVAL", 1440)

	defNSQTCP := getEnv("HITKEEP_NSQ_TCP_ADDRESS", "127.0.0.1:4150")
	defNSQHTTP := getEnv("HITKEEP_NSQ_HTTP_ADDRESS", "127.0.0.1:4151")

	defIngestRate := getFloat("HITKEEP_INGEST_RATE_LIMIT", 20.0)
	defIngestBurst := getInt("HITKEEP_INGEST_BURST", 40)
	defApiRate := getFloat("HITKEEP_API_RATE_LIMIT", 10.0)
	defApiBurst := getInt("HITKEEP_API_BURST", 20)
	defAuthRate := getFloat("HITKEEP_AUTH_RATE_LIMIT", 2.0)
	defAuthBurst := getInt("HITKEEP_AUTH_BURST", 5)
	defAuthRememberMeDays := getInt("HITKEEP_AUTH_REMEMBER_ME_DAYS", 30)
	defAuthSessionMinutes := getInt("HITKEEP_AUTH_SESSION_MINUTES", 15)
	defAuthSessionWarningSeconds := getInt("HITKEEP_AUTH_SESSION_WARNING_SECONDS", 120)
	defWebhookRate := getFloat("HITKEEP_WEBHOOK_RATE_LIMIT", 30.0)
	defWebhookBurst := getInt("HITKEEP_WEBHOOK_BURST", 60)

	defMailDriver := getEnv("HITKEEP_MAIL_DRIVER", "smtp")
	defMailEnc := getEnv("HITKEEP_MAIL_ENCRYPTION", "tls")
	defMailSkipVerify := getBool("HITKEEP_MAIL_INSECURE_SKIP_VERIFY", false)
	defMailHost := getEnv("HITKEEP_MAIL_HOST", "")
	defMailPort := getInt("HITKEEP_MAIL_PORT", 587)
	defMailUser := getEnv("HITKEEP_MAIL_USERNAME", "")
	defMailPass := getEnv("HITKEEP_MAIL_PASSWORD", "")
	defMailFrom := getEnv("HITKEEP_MAIL_FROM_ADDRESS", "hitkeep@localhost")
	defMailName := getEnv("HITKEEP_MAIL_FROM_NAME", "HitKeep")

	defRetention := getInt("HITKEEP_DATA_RETENTION_DAYS", 365)

	defBackupPath := getEnv("HITKEEP_BACKUP_PATH", "")
	defBackupInterval := getInt("HITKEEP_BACKUP_INTERVAL", 60)
	defBackupRetention := getInt("HITKEEP_BACKUP_RETENTION", 24)

	defS3AccessKeyID := getEnv("HITKEEP_S3_ACCESS_KEY_ID", "")
	defS3SecretAccessKey := getEnv("HITKEEP_S3_SECRET_ACCESS_KEY", "")
	defS3SessionToken := getEnv("HITKEEP_S3_SESSION_TOKEN", "")
	defS3Region := getEnv("HITKEEP_S3_REGION", "us-east-1")
	defS3Endpoint := getEnv("HITKEEP_S3_ENDPOINT", "")
	defS3URLStyle := getEnv("HITKEEP_S3_URL_STYLE", "")
	defS3UseSSL := getBool("HITKEEP_S3_USE_SSL", true)

	defTrustedProxies := getEnv("HITKEEP_TRUSTED_PROXIES", "*")
	defMCPEnabled := getBool("HITKEEP_MCP_ENABLED", false)
	defMCPPath := getEnv("HITKEEP_MCP_PATH", "/mcp")
	defMCPMaxRangeDays := getInt("HITKEEP_MCP_MAX_RANGE_DAYS", 366)
	defMCPDocsEnabled := getBool("HITKEEP_MCP_DOCS_ENABLED", true)
	defMCPDocsURL := getEnv("HITKEEP_MCP_DOCS_URL", "https://hitkeep.com")
	defMCPDocsCacheMinutes := getInt("HITKEEP_MCP_DOCS_CACHE_MINUTES", 60)

	hostname, _ := os.Hostname()
	defNodeName := getEnv("HITKEEP_NODE_NAME", fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano()))

	fs.StringVar(&conf.HTTPAddr, "http", defHTTP, "HTTP listen address")
	fs.StringVar(&conf.BindAddr, "bind", defBind, "Address for cluster gossip")
	fs.StringVar(&conf.JoinAddr, "join", defJoin, "Address of a peer to join")
	fs.StringVar(&conf.PublicURL, "public-url", defPublicURL, "Public URL")
	fs.StringVar(&conf.SpamFilterPath, "spam-filter-path", defSpamFilterPath, "Path to cached spam filter data (defaults to <data-path>/spam-filter.json)")
	fs.BoolVar(&conf.SpamFilterAutoUpdate, "spam-filter-auto-update", defSpamFilterAutoUpdate, "Automatically refresh OSS spam filter feeds on the leader node (disabled by default for airgapped/offline installs)")
	fs.IntVar(&conf.SpamFilterUpdateIntervalMin, "spam-filter-update-interval", defSpamFilterUpdateInterval, "Minutes between OSS spam filter feed refreshes")

	fs.StringVar(&conf.DBPath, "db", defDB, "Database file path")
	fs.StringVar(&conf.DataPath, "data-path", defDataPath, "Base directory for per-tenant data files")
	fs.StringVar(&conf.ArchivePath, "archive-path", defArchive, "Data archive path")
	fs.StringVar(&conf.LogLevel, "log-level", defLogLevel, "Log level")
	fs.StringVar(&conf.JWTSecret, "jwt-secret", defJWT, "Secret key for JWT")

	fs.StringVar(&conf.NSQTCPAddress, "nsq-tcp-address", defNSQTCP, "Internal NSQ TCP")
	fs.StringVar(&conf.NSQHTTPAddress, "nsq-http-address", defNSQHTTP, "Internal NSQ HTTP")

	fs.BoolVar(&conf.Healthcheck, "healthcheck", false, "Run as healthcheck client")

	fs.Float64Var(&conf.IngestRateLimit, "ingest-rate", defIngestRate, "Ingest rate limit")
	fs.IntVar(&conf.IngestBurst, "ingest-burst", defIngestBurst, "Ingest burst")
	fs.Float64Var(&conf.ApiRateLimit, "api-rate", defApiRate, "API rate limit")
	fs.IntVar(&conf.ApiBurst, "api-burst", defApiBurst, "API burst")
	fs.Float64Var(&conf.AuthRateLimit, "auth-rate", defAuthRate, "Auth rate limit")
	fs.IntVar(&conf.AuthBurst, "auth-burst", defAuthBurst, "Auth burst")
	fs.IntVar(&conf.AuthRememberMeDays, "auth-remember-me-days", defAuthRememberMeDays, "Remember-me session lifetime in days")
	fs.IntVar(&conf.AuthSessionMinutes, "auth-session-minutes", defAuthSessionMinutes, "Authenticated dashboard session lifetime in minutes")
	fs.IntVar(&conf.AuthSessionWarningSeconds, "auth-session-warning-seconds", defAuthSessionWarningSeconds, "Seconds before session expiry to warn users")
	fs.Float64Var(&conf.WebhookRateLimit, "webhook-rate", defWebhookRate, "Webhook rate limit")
	fs.IntVar(&conf.WebhookBurst, "webhook-burst", defWebhookBurst, "Webhook burst")

	fs.StringVar(&conf.MailDriver, "mail-driver", defMailDriver, "Mail driver")
	fs.StringVar(&conf.MailEncryption, "mail-encryption", defMailEnc, "Mail encryption")
	fs.BoolVar(&conf.MailInsecureSkipVerify, "mail-insecure-skip-verify", defMailSkipVerify, "Disable Cert validation")
	fs.StringVar(&conf.MailHost, "mail-host", defMailHost, "SMTP Host")
	fs.IntVar(&conf.MailPort, "mail-port", defMailPort, "SMTP Port")
	fs.StringVar(&conf.MailUsername, "mail-username", defMailUser, "SMTP Username")
	fs.StringVar(&conf.MailPassword, "mail-password", defMailPass, "SMTP Password")
	fs.StringVar(&conf.MailFromAddress, "mail-from-address", defMailFrom, "From Email")
	fs.StringVar(&conf.MailFromName, "mail-from-name", defMailName, "From Name")

	fs.IntVar(&conf.DataRetentionDays, "retention-days", defRetention, "Default data retention in days")

	fs.StringVar(&conf.BackupPath, "backup-path", defBackupPath, "Backup destination path (local dir or s3://)")
	fs.IntVar(&conf.BackupIntervalMinutes, "backup-interval", defBackupInterval, "Minutes between backups")
	fs.IntVar(&conf.BackupRetentionCount, "backup-retention", defBackupRetention, "Number of backup snapshots to keep")

	fs.StringVar(&conf.S3AccessKeyID, "s3-access-key-id", defS3AccessKeyID, "S3 access key ID (static credentials)")
	fs.StringVar(&conf.S3SecretAccessKey, "s3-secret-access-key", defS3SecretAccessKey, "S3 secret access key (static credentials)")
	fs.StringVar(&conf.S3SessionToken, "s3-session-token", defS3SessionToken, "S3 session token (STS temporary credentials)")
	fs.StringVar(&conf.S3Region, "s3-region", defS3Region, "S3 region")
	fs.StringVar(&conf.S3Endpoint, "s3-endpoint", defS3Endpoint, "S3 custom endpoint (MinIO, R2, Spaces)")
	fs.StringVar(&conf.S3URLStyle, "s3-url-style", defS3URLStyle, "S3 URL style: path or vhost")
	fs.BoolVar(&conf.S3UseSSL, "s3-use-ssl", defS3UseSSL, "S3 use SSL (set false for local MinIO over HTTP)")

	fs.BoolVar(&conf.MCPEnabled, "mcp-enabled", defMCPEnabled, "Enable the optional leader-only MCP server")
	fs.StringVar(&conf.MCPPath, "mcp-path", defMCPPath, "MCP server HTTP path on the main HitKeep HTTP server")
	fs.IntVar(&conf.MCPMaxRangeDays, "mcp-max-range-days", defMCPMaxRangeDays, "Maximum analytics date range in days for MCP tools")
	fs.BoolVar(&conf.MCPDocsEnabled, "mcp-docs-enabled", defMCPDocsEnabled, "Enable MCP tools and resources that read official HitKeep docs")
	fs.StringVar(&conf.MCPDocsURL, "mcp-docs-url", defMCPDocsURL, "Base URL for official HitKeep docs used by MCP docs tools")
	fs.IntVar(&conf.MCPDocsCacheMinutes, "mcp-docs-cache-minutes", defMCPDocsCacheMinutes, "Minutes to cache fetched docs for MCP tools")

	registerCloudFlags(fs, &conf, getEnv, getInt, getInt64, getBool)

	fs.StringVar(&conf.TrustedProxies, "trusted-proxies", defTrustedProxies, "Trusted proxy CIDRs (comma-separated) or '*' to trust all")

	fs.StringVar(&conf.NodeName, "name", defNodeName, "Unique node name")

	_ = fs.Parse(args)

	if conf.JWTSecret == "" {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err == nil {
			conf.JWTSecret = hex.EncodeToString(bytes)
		} else {
			conf.JWTSecret = fmt.Sprintf("fallback-secret-%d", time.Now().UnixNano())
		}
	}

	// Parse trusted proxies
	if conf.TrustedProxies != "" {
		conf.trustedProxyNets = parseTrustedProxies(conf.TrustedProxies)
		if len(conf.trustedProxyNets) > 0 {
			slog.Info("Loaded trusted proxy networks", "count", len(conf.trustedProxyNets))
		}
	}
	NormalizeMCPConfig(&conf)
	NormalizeAuthSessionConfig(&conf)

	return &conf
}

func (c *Config) AuthSessionDuration() time.Duration {
	if c.AuthSessionMinutes <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(c.AuthSessionMinutes) * time.Minute
}

func (c *Config) AuthSessionWarningDuration() time.Duration {
	if c.AuthSessionWarningSeconds <= 0 {
		return 2 * time.Minute
	}
	return time.Duration(c.AuthSessionWarningSeconds) * time.Second
}

func (c *Config) AuthRememberMeDuration() time.Duration {
	if c.AuthRememberMeDays <= 0 {
		return 30 * 24 * time.Hour
	}
	return time.Duration(c.AuthRememberMeDays) * 24 * time.Hour
}

func NormalizeAuthSessionConfig(conf *Config) {
	if conf.AuthSessionMinutes <= 0 {
		conf.AuthSessionMinutes = 15
	}
	if conf.AuthRememberMeDays <= 0 {
		conf.AuthRememberMeDays = 30
	}
	if conf.AuthSessionWarningSeconds < 20 {
		conf.AuthSessionWarningSeconds = 20
	}

	maxWarningSeconds := int((time.Duration(conf.AuthSessionMinutes) * time.Minute / 2).Seconds())
	if maxWarningSeconds < 20 {
		maxWarningSeconds = 20
	}
	if conf.AuthSessionWarningSeconds >= conf.AuthSessionMinutes*60 {
		conf.AuthSessionWarningSeconds = maxWarningSeconds
	}
}

func NormalizeMCPConfig(conf *Config) {
	conf.MCPPath = strings.TrimSpace(conf.MCPPath)
	if conf.MCPPath == "" || conf.MCPPath == "/" {
		conf.MCPPath = "/mcp"
	}
	if !strings.HasPrefix(conf.MCPPath, "/") {
		conf.MCPPath = "/" + conf.MCPPath
	}

	if conf.MCPMaxRangeDays <= 0 {
		conf.MCPMaxRangeDays = 366
	}
	if conf.MCPDocsCacheMinutes <= 0 {
		conf.MCPDocsCacheMinutes = 60
	}

	docsURL := strings.TrimRight(strings.TrimSpace(conf.MCPDocsURL), "/")
	parsed, err := url.Parse(docsURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		slog.Warn("Invalid MCP docs URL, using default", "value", conf.MCPDocsURL)
		docsURL = "https://hitkeep.com"
	}
	conf.MCPDocsURL = docsURL
}

// parseTrustedProxies parses a comma-separated list of CIDR ranges.
// The wildcard "*" expands to both IPv4 and IPv6 all-network CIDRs.
func parseTrustedProxies(cidrs string) []netip.Prefix {
	if cidrs == "" {
		return nil
	}

	parts := strings.Split(cidrs, ",")
	networks := make([]netip.Prefix, 0, len(parts))

	for _, cidr := range parts {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if cidr == "*" {
			return trustAllProxyNetworks()
		}

		network, err := netip.ParsePrefix(cidr)
		if err != nil {
			slog.Warn("Invalid trusted proxy CIDR, skipping", "cidr", cidr, "error", err)
			continue
		}

		networks = append(networks, network.Masked())
	}

	return networks
}

func trustAllProxyNetworks() []netip.Prefix {
	allV4, errV4 := netip.ParsePrefix("0.0.0.0/0")
	allV6, errV6 := netip.ParsePrefix("::/0")
	if errV4 != nil || errV6 != nil {
		slog.Warn("Failed to parse trust-all proxy CIDRs", "ipv4_error", errV4, "ipv6_error", errV6)
		return nil
	}
	return []netip.Prefix{allV4.Masked(), allV6.Masked()}
}

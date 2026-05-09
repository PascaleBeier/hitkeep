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
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string  `env:"HITKEEP_HTTP_ADDR"        default:":8080"                               desc:"HTTP listen address"                                    deprecated:"http"`
	DBPath            string  `env:"HITKEEP_DB_PATH"          default:"hitkeep.db"                          desc:"Database file path"                                     deprecated:"db"`
	BindAddr          string  `env:"HITKEEP_BIND_ADDR"        default:"0.0.0.0:7946"                       desc:"Address for cluster gossip"                             deprecated:"bind"`
	JoinAddr          string  `env:"HITKEEP_JOIN_ADDR"        default:""                                   desc:"Address of a peer to join"                              deprecated:"join"`
	IngestRateLimit   float64 `env:"HITKEEP_INGEST_RATE_LIMIT" default:"20"                                desc:"Ingest rate limit"                                      deprecated:"ingest-rate"`
	ApiRateLimit      float64 `env:"HITKEEP_API_RATE_LIMIT"    default:"10"                                desc:"API rate limit"                                         deprecated:"api-rate"`
	AuthRateLimit     float64 `env:"HITKEEP_AUTH_RATE_LIMIT"   default:"2"                                 desc:"Auth rate limit"                                        deprecated:"auth-rate"`
	WebhookRateLimit  float64 `env:"HITKEEP_WEBHOOK_RATE_LIMIT" default:"30"                               desc:"Webhook rate limit"                                     deprecated:"webhook-rate"`
	DataRetentionDays int     `env:"HITKEEP_DATA_RETENTION_DAYS" default:"365"                             desc:"Default data retention in days"                         deprecated:"retention-days"`
	NodeName          string  `env:"HITKEEP_NODE_NAME"         default:""                                  desc:"Unique node name"                                       deprecated:"name"`

	DataPath       string `env:"HITKEEP_DATA_PATH"          default:"data"           desc:"Base directory for per-tenant data files"`
	ArchivePath    string `env:"HITKEEP_ARCHIVE_PATH"       default:"archive"        desc:"Data archive path"`
	PublicURL      string `env:"HITKEEP_PUBLIC_URL"         default:"http://localhost:8080" desc:"Public URL"`
	LogLevel       string `env:"HITKEEP_LOG_LEVEL"          default:"info"           desc:"Log level (debug/info/warn/error)"`
	JWTSecret      string `env:"HITKEEP_JWT_SECRET"         default:""               desc:"Secret key for JWT"                                                     sensitive:"redact"`
	Healthcheck    bool   `flag:"healthcheck"               default:"false"          desc:"Run as healthcheck client"`
	TrustedProxies string `env:"HITKEEP_TRUSTED_PROXIES"    default:"*"              desc:"Trusted proxy CIDRs (comma-separated) or '*' to trust all"`

	NSQTCPAddress  string `env:"HITKEEP_NSQ_TCP_ADDRESS"  default:"127.0.0.1:4150" desc:"Internal NSQ TCP"`
	NSQHTTPAddress string `env:"HITKEEP_NSQ_HTTP_ADDRESS" default:"127.0.0.1:4151" desc:"Internal NSQ HTTP"`

	ApiBurst                  int `env:"HITKEEP_API_BURST"                  default:"20"    desc:"API burst"`
	AuthBurst                 int `env:"HITKEEP_AUTH_BURST"                 default:"5"     desc:"Auth burst"`
	IngestBurst               int `env:"HITKEEP_INGEST_BURST"               default:"40"    desc:"Ingest burst"`
	WebhookBurst              int `env:"HITKEEP_WEBHOOK_BURST"              default:"60"    desc:"Webhook burst"`
	AuthRememberMeDays        int `env:"HITKEEP_AUTH_REMEMBER_ME_DAYS"      default:"30"    desc:"Remember-me session lifetime in days"`
	AuthSessionMinutes        int `env:"HITKEEP_AUTH_SESSION_MINUTES"       default:"15"    desc:"Authenticated dashboard session lifetime in minutes"`
	AuthSessionWarningSeconds int `env:"HITKEEP_AUTH_SESSION_WARNING_SECONDS" default:"120" desc:"Seconds before session expiry to warn users"`

	MailDriver             string `env:"HITKEEP_MAIL_DRIVER"              default:"smtp"     desc:"Mail driver"`
	MailEncryption         string `env:"HITKEEP_MAIL_ENCRYPTION"          default:"tls"      desc:"Mail encryption"`
	MailInsecureSkipVerify bool   `env:"HITKEEP_MAIL_INSECURE_SKIP_VERIFY" default:"false"   desc:"Disable Cert validation"`
	MailHost               string `env:"HITKEEP_MAIL_HOST"                default:""         desc:"SMTP Host"`
	MailPort               int    `env:"HITKEEP_MAIL_PORT"                default:"587"      desc:"SMTP Port"`
	MailUsername           string `env:"HITKEEP_MAIL_USERNAME"            default:""         desc:"SMTP Username"`
	MailPassword           string `env:"HITKEEP_MAIL_PASSWORD"            default:""         desc:"SMTP Password"                                            sensitive:"redact"`
	MailFromAddress        string `env:"HITKEEP_MAIL_FROM_ADDRESS"        default:"hitkeep@localhost" desc:"From Email"`
	MailFromName           string `env:"HITKEEP_MAIL_FROM_NAME"           default:"HitKeep"  desc:"From Name"`

	SpamFilterAutoUpdate        bool   `env:"HITKEEP_SPAM_FILTER_AUTO_UPDATE"         default:"false" desc:"Automatically refresh OSS spam filter feeds on the leader node (disabled by default for airgapped/offline installs)"`
	SpamFilterPath              string `env:"HITKEEP_SPAM_FILTER_PATH"                 default:""     desc:"Path to cached spam filter data (defaults to <data-path>/spam-filter.json)"`
	SpamFilterUpdateIntervalMin int    `env:"HITKEEP_SPAM_FILTER_UPDATE_INTERVAL"      default:"1440" desc:"Minutes between OSS spam filter feed refreshes"`

	ImportMaxStageBytes      int `env:"HITKEEP_IMPORT_MAX_STAGE_BYTES"        default:"107374182400" desc:"Maximum staged import upload size in bytes"`
	ImportStageRetentionDays int `env:"HITKEEP_IMPORT_STAGE_RETENTION_DAYS" default:"7"            desc:"Days to keep stale staged import upload files; 0 disables import staging cleanup"`

	GoogleSearchConsoleClientID     string `env:"HITKEEP_GOOGLE_SEARCH_CONSOLE_CLIENT_ID"     default:"" desc:"Google Search Console OAuth client ID"`
	GoogleSearchConsoleClientSecret string `env:"HITKEEP_GOOGLE_SEARCH_CONSOLE_CLIENT_SECRET" default:"" desc:"Google Search Console OAuth client secret" sensitive:"redact"`
	GoogleSearchConsoleRedirectURL  string `env:"HITKEEP_GOOGLE_SEARCH_CONSOLE_REDIRECT_URL"  default:"" desc:"Google Search Console OAuth callback URL override"`

	BackupPath            string `env:"HITKEEP_BACKUP_PATH"      default:""   desc:"Backup destination path (local dir or s3://)"`
	BackupIntervalMinutes int    `env:"HITKEEP_BACKUP_INTERVAL"  default:"60" desc:"Minutes between backups"`
	BackupRetentionCount  int    `env:"HITKEEP_BACKUP_RETENTION" default:"24" desc:"Number of backup snapshots to keep"`

	S3AccessKeyID     string `env:"HITKEEP_S3_ACCESS_KEY_ID"     default:""         desc:"S3 access key ID (static credentials)"                  sensitive:"mask"`
	S3SecretAccessKey string `env:"HITKEEP_S3_SECRET_ACCESS_KEY" default:""         desc:"S3 secret access key (static credentials)"              sensitive:"redact"`
	S3SessionToken    string `env:"HITKEEP_S3_SESSION_TOKEN"     default:""         desc:"S3 session token (STS temporary credentials)"           sensitive:"redact"`
	S3Region          string `env:"HITKEEP_S3_REGION"            default:"us-east-1" desc:"S3 region"`
	S3Endpoint        string `env:"HITKEEP_S3_ENDPOINT"          default:""         desc:"S3 custom endpoint (MinIO, R2, Spaces)"`
	S3URLStyle        string `env:"HITKEEP_S3_URL_STYLE"         default:""         desc:"S3 URL style: path or vhost"`
	S3UseSSL          bool   `env:"HITKEEP_S3_USE_SSL"           default:"true"     desc:"S3 use SSL (set false for local MinIO over HTTP)"`

	MCPEnabled          bool   `env:"HITKEEP_MCP_ENABLED"            default:"false"  desc:"Enable the optional leader-only MCP server"`
	MCPPath             string `env:"HITKEEP_MCP_PATH"               default:"/mcp"   desc:"MCP server HTTP path on the main HitKeep HTTP server"`
	MCPMaxRangeDays     int    `env:"HITKEEP_MCP_MAX_RANGE_DAYS"     default:"366"    desc:"Maximum analytics date range in days for MCP tools"`
	MCPDocsEnabled      bool   `env:"HITKEEP_MCP_DOCS_ENABLED"       default:"true"   desc:"Enable MCP tools and resources that read official HitKeep docs"`
	MCPDocsURL          string `env:"HITKEEP_MCP_DOCS_URL"           default:"https://hitkeep.com" desc:"Base URL for official HitKeep docs used by MCP docs tools"`
	MCPDocsCacheMinutes int    `env:"HITKEEP_MCP_DOCS_CACHE_MINUTES" default:"60"     desc:"Minutes to cache fetched docs for MCP tools"`

	AIEnabled             bool   `env:"HITKEEP_AI_ENABLED"             default:"false" desc:"Enable optional AI-powered product features"`
	AIProvider            string `env:"HITKEEP_AI_PROVIDER"            default:""      desc:"AI provider key supported by GoAI (openai, openai-compatible, bedrock, anthropic, google, mistral, ollama, openrouter)"`
	AIModel               string `env:"HITKEEP_AI_MODEL"               default:""      desc:"AI model identifier for the configured provider"`
	AIBaseURL             string `env:"HITKEEP_AI_BASE_URL"            default:""      desc:"Optional AI provider or gateway base URL"`
	AIRegion              string `env:"HITKEEP_AI_REGION"              default:""      desc:"Optional AI provider region"`
	AIAPIKey              string `env:"HITKEEP_AI_API_KEY"             default:""      desc:"AI provider API key, bearer token, or gateway key" sensitive:"redact"`
	AITimeoutSeconds      int    `env:"HITKEEP_AI_TIMEOUT_SECONDS"     default:"30"    desc:"AI provider request timeout in seconds"`
	AIRequestLimit        int    `env:"HITKEEP_AI_REQUEST_LIMIT"       default:"100"   desc:"Maximum AI requests per budget window; 0 disables local request cap"`
	AITokenLimit          int    `env:"HITKEEP_AI_TOKEN_LIMIT"         default:"100000" desc:"Maximum AI tokens per budget window; 0 disables local token cap"`
	AIBudgetWindowMinutes int    `env:"HITKEEP_AI_BUDGET_WINDOW"       default:"1440"  desc:"AI local budget window in minutes"`

	CloudHosted                 bool   `env:"HITKEEP_CLOUD_HOSTED"                   default:"true"  desc:"Enable managed cloud runtime surfaces"                              cloud:"true"`
	CloudSignupEnabled          bool   `env:"HITKEEP_CLOUD_SIGNUP_ENABLED"           default:"false" desc:"Enable hosted self-serve onboarding surfaces"                      cloud:"true"`
	CloudJurisdiction           string `env:"HITKEEP_CLOUD_JURISDICTION"             default:""      desc:"Managed cloud jurisdiction label"                                   cloud:"true"`
	CloudRegion                 string `env:"HITKEEP_CLOUD_REGION"                   default:""      desc:"Managed cloud region label"                                        cloud:"true"`
	CloudUpgradeURL             string `env:"HITKEEP_CLOUD_UPGRADE_URL"              default:""      desc:"Managed cloud upgrade URL"                                         cloud:"true"`
	CloudSupportURL             string `env:"HITKEEP_CLOUD_SUPPORT_URL"              default:""      desc:"Managed cloud support URL"                                         cloud:"true"`
	CloudPlanCode               string `env:"HITKEEP_CLOUD_PLAN_CODE"                default:"free"  desc:"Managed cloud plan code"                                           cloud:"true"`
	CloudPlanName               string `env:"HITKEEP_CLOUD_PLAN_NAME"                default:"Free"  desc:"Managed cloud plan name"                                           cloud:"true"`
	CloudMaxTeams               int    `env:"HITKEEP_CLOUD_MAX_TEAMS"                default:"1"     desc:"Managed cloud max teams per user"                                  cloud:"true"`
	CloudMaxSitesPerTeam        int    `env:"HITKEEP_CLOUD_MAX_SITES_PER_TEAM"       default:"3"     desc:"Managed cloud max sites per team"                                  cloud:"true"`
	CloudMaxRetentionDays       int    `env:"HITKEEP_CLOUD_MAX_RETENTION_DAYS"       default:"60"    desc:"Managed cloud max retention days"                                  cloud:"true"`
	CloudMaxTeamMembers         int    `env:"HITKEEP_CLOUD_MAX_TEAM_MEMBERS"         default:"3"     desc:"Managed cloud max members per team"                                cloud:"true"`
	CloudAllowSSO               bool   `env:"HITKEEP_CLOUD_ALLOW_SSO"                default:"false" desc:"Managed cloud SSO entitlement"                                     cloud:"true"`
	CloudAllowCustomBranding    bool   `env:"HITKEEP_CLOUD_ALLOW_CUSTOM_BRANDING"    default:"false" desc:"Managed cloud custom branding entitlement"                          cloud:"true"`
	StripeSecretKey             string `env:"HITKEEP_STRIPE_SECRET_KEY"              default:""      desc:"Stripe secret key for managed cloud billing"                       cloud:"true" sensitive:"redact"`
	StripePublishableKey        string `env:"HITKEEP_STRIPE_PUBLISHABLE_KEY"         default:""      desc:"Stripe publishable key for managed cloud billing"                  cloud:"true" sensitive:"redact"`
	StripeWebhookSecret         string `env:"HITKEEP_STRIPE_WEBHOOK_SECRET"          default:""      desc:"Stripe webhook signing secret for managed cloud billing"           cloud:"true" sensitive:"redact"`
	StripePortalConfigurationID string `env:"HITKEEP_STRIPE_PORTAL_CONFIGURATION_ID" default:""   desc:"Stripe customer portal configuration ID for managed cloud billing" cloud:"true"`
	StripePriceProMonthly       string `env:"HITKEEP_STRIPE_PRICE_PRO_MONTHLY"       default:""      desc:"Stripe monthly recurring price ID for the Pro plan"               cloud:"true"`
	StripePriceBusinessMonthly  string `env:"HITKEEP_STRIPE_PRICE_BUSINESS_MONTHLY" default:""     desc:"Stripe monthly recurring price ID for the Business plan"          cloud:"true"`
	CloudCheckoutSuccessURL     string `env:"HITKEEP_CLOUD_CHECKOUT_SUCCESS_URL"     default:""      desc:"Managed cloud checkout success URL override"                      cloud:"true"`
	CloudCheckoutCancelURL      string `env:"HITKEEP_CLOUD_CHECKOUT_CANCEL_URL"      default:""      desc:"Managed cloud checkout cancel URL override"                       cloud:"true"`

	Version          string
	trustedProxyNets []netip.Prefix
}

func (c *Config) GetTrustedProxyNetworks() []netip.Prefix { return c.trustedProxyNets }

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

func Load() *Config {
	return load(os.Args[1:], func(key, fallback string) string {
		if val := os.Getenv(key); val != "" {
			return val
		}
		return fallback
	})
}

func load(args []string, getEnv func(string, string) string) *Config {
	var conf Config
	loadEnvDefaults(&conf)
	loadEnvOverrides(&conf, getEnv)

	fs := flag.NewFlagSet("hitkeep", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerFlags(fs, &conf)
	registerCloudFlags(fs, &conf)
	_ = fs.Parse(args)

	if conf.Healthcheck {
		return &conf
	}

	normalizeConfig(&conf)
	return &conf
}

func flagName(envKey string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(envKey, "HITKEEP_")), "_", "-")
}

func loadEnvDefaults(conf *Config) {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !shouldLoadEnvField(f) {
			continue
		}
		def := f.Tag.Get("default")
		if def == "" {
			continue
		}
		fv := v.Field(i)
		setDefault(fv, def)
	}
}

func setDefault(fv reflect.Value, def string) {
	switch fv.Kind() { //nolint:exhaustive // only handling config-relevant kinds
	case reflect.String:
		fv.SetString(def)
	case reflect.Int:
		if n, err := strconv.Atoi(def); err == nil {
			fv.SetInt(int64(n))
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(def); err == nil {
			fv.SetBool(b)
		}
	case reflect.Float64:
		if f, err := strconv.ParseFloat(def, 64); err == nil {
			fv.SetFloat(f)
		}
	}
}

func loadEnvOverrides(conf *Config, getEnv func(string, string) string) {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !shouldLoadEnvField(f) {
			continue
		}
		env := f.Tag.Get("env")
		if env == "" {
			continue
		}
		val := getEnv(env, "")
		if val == "" {
			continue
		}
		fv := v.Field(i)
		if !setEnvValue(fv, val) {
			slog.Warn("Invalid value in env var, using default", "key", env, "val", val)
		}
	}
}

func shouldLoadEnvField(f reflect.StructField) bool {
	if !f.IsExported() {
		return false
	}
	if f.Tag.Get("cloud") == "true" {
		return includeCloudConfigFields()
	}
	return true
}

func setEnvValue(fv reflect.Value, val string) bool {
	switch fv.Kind() { //nolint:exhaustive // only handling config-relevant kinds
	case reflect.String:
		fv.SetString(val)
		return true
	case reflect.Int:
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		fv.SetInt(int64(n))
		return true
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}
		fv.SetBool(b)
		return true
	case reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		fv.SetFloat(f)
		return true
	}
	return false
}

func registerFlags(fs *flag.FlagSet, conf *Config) {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("cloud") == "true" {
			continue
		}
		registerOneField(fs, v.Field(i), f)
	}
}

func registerOneField(fs *flag.FlagSet, fv reflect.Value, sf reflect.StructField) {
	fname := sf.Tag.Get("flag")
	if fname == "" {
		env := sf.Tag.Get("env")
		if env == "" {
			return
		}
		fname = flagName(env)
	}
	desc := sf.Tag.Get("desc")

	if dep := sf.Tag.Get("deprecated"); dep != "" {
		registerFlagVar(fs, fv, dep, "(deprecated, use --"+fname+")")
	}
	registerFlagVar(fs, fv, fname, desc)
}

func registerFlagVar(fs *flag.FlagSet, fv reflect.Value, name, desc string) {
	switch fv.Kind() { //nolint:exhaustive // only handling config-relevant kinds
	case reflect.String:
		fs.StringVar(fv.Addr().Interface().(*string), name, fv.String(), desc)
	case reflect.Int:
		fs.IntVar(fv.Addr().Interface().(*int), name, int(fv.Int()), desc)
	case reflect.Bool:
		fs.BoolVar(fv.Addr().Interface().(*bool), name, fv.Bool(), desc)
	case reflect.Float64:
		fs.Float64Var(fv.Addr().Interface().(*float64), name, fv.Float(), desc)
	}
}

func normalizeConfig(conf *Config) {
	if conf.JWTSecret == "" {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err == nil {
			conf.JWTSecret = hex.EncodeToString(bytes)
		} else {
			conf.JWTSecret = fmt.Sprintf("fallback-secret-%d", time.Now().UnixNano())
		}
	}

	if conf.NodeName == "" {
		hostname, _ := os.Hostname()
		conf.NodeName = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	}

	if conf.TrustedProxies != "" {
		conf.trustedProxyNets = parseTrustedProxies(conf.TrustedProxies)
		if len(conf.trustedProxyNets) > 0 {
			slog.Debug("Loaded trusted proxy networks", "count", len(conf.trustedProxyNets))
		}
	}

	NormalizeMCPConfig(conf)
	NormalizeAuthSessionConfig(conf)
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

	maxWarningSeconds := max(int((time.Duration(conf.AuthSessionMinutes) * time.Minute / 2).Seconds()), 20)
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

func maskKey(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

func (c *Config) LogValue() slog.Value {
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	attrs := make([]slog.Attr, 0, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Tag.Get("cloud") == "true" {
			continue
		}
		kind := f.Tag.Get("sensitive")
		name := f.Name
		fv := v.Field(i)
		switch fv.Kind() { //nolint:exhaustive // only handling config-relevant kinds
		case reflect.String:
			s := fv.String()
			switch kind {
			case "redact":
				if s == "" {
					attrs = append(attrs, slog.String(name, ""))
				} else {
					attrs = append(attrs, slog.String(name, "[redacted]"))
				}
			case "mask":
				attrs = append(attrs, slog.String(name, maskKey(s)))
			default:
				attrs = append(attrs, slog.String(name, s))
			}
		case reflect.Int:
			attrs = append(attrs, slog.Int64(name, fv.Int()))
		case reflect.Bool:
			attrs = append(attrs, slog.Bool(name, fv.Bool()))
		case reflect.Float64:
			attrs = append(attrs, slog.Float64(name, fv.Float()))
		}
	}
	return slog.GroupValue(attrs...)
}

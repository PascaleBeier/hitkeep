package config

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
)

const ConfigurationCatalogSchemaVersion = "hitkeep.config/v2"

type ConfigurationCategory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ConfigurationSetting struct {
	Field                  string                        `json:"field"`
	Category               string                        `json:"category"`
	ConfigFileKey          string                        `json:"config_file_key"`
	Environment            string                        `json:"environment,omitempty"`
	Flag                   string                        `json:"flag"`
	DeprecatedFlags        []string                      `json:"deprecated_flags,omitempty"`
	DeprecatedEnvironments []string                      `json:"deprecated_environments,omitempty"`
	Type                   string                        `json:"type"`
	Default                string                        `json:"default"`
	DisplayDefault         string                        `json:"display_default,omitempty"`
	Description            string                        `json:"description"`
	Sensitive              string                        `json:"sensitive,omitempty"`
	CloudOnly              bool                          `json:"cloud_only,omitempty"`
	Publication            ConfigurationPublicationClass `json:"publication"`
}

type ConfigurationCatalog struct {
	SchemaVersion string                  `json:"schema_version"`
	Categories    []ConfigurationCategory `json:"categories"`
	Settings      []ConfigurationSetting  `json:"settings"`
}

// Catalog returns the operator configuration contract directly from the same
// Config annotations used to load environment variables and register flags.
// It intentionally contains metadata only; configured values are never read.
func Catalog() ConfigurationCatalog {
	typeOfConfig := reflect.TypeFor[Config]()
	settings := make([]ConfigurationSetting, 0, typeOfConfig.NumField())
	for field := range typeOfConfig.Fields() {
		if !field.IsExported() {
			continue
		}
		environment := field.Tag.Get("env")
		flag := field.Tag.Get("flag")
		if flag == "" && environment != "" {
			flag = flagName(environment)
		}
		if flag == "" {
			continue
		}
		setting := ConfigurationSetting{
			Field:          field.Name,
			Category:       configurationCategory(field),
			ConfigFileKey:  configFileKey(environment, flag),
			Environment:    environment,
			Flag:           flag,
			Type:           configurationType(field.Type.Kind()),
			Default:        field.Tag.Get("default"),
			DisplayDefault: field.Tag.Get("docdefault"),
			Description:    field.Tag.Get("desc"),
			Sensitive:      field.Tag.Get("sensitive"),
			CloudOnly:      field.Tag.Get("cloud") == "true",
			Publication:    configurationPublication(field),
		}
		if deprecated := field.Tag.Get("deprecated"); deprecated != "" {
			setting.DeprecatedFlags = []string{deprecated}
		}
		if deprecated := field.Tag.Get("deprecatedenv"); deprecated != "" {
			setting.DeprecatedEnvironments = strings.Split(deprecated, ",")
		}
		settings = append(settings, setting)
	}
	slices.SortFunc(settings, func(left, right ConfigurationSetting) int {
		if left.Category != right.Category {
			return cmp.Compare(configurationCategoryIndex(left.Category), configurationCategoryIndex(right.Category))
		}
		return cmp.Compare(left.Flag, right.Flag)
	})
	return ConfigurationCatalog{
		SchemaVersion: ConfigurationCatalogSchemaVersion,
		Categories:    configurationCategories(),
		Settings:      settings,
	}
}

// ConfigurationPublicationSurface is a published configuration surface whose
// default must remain aligned with the runtime catalog.
type ConfigurationPublicationSurface string

const (
	ConfigurationPublicationDocker           ConfigurationPublicationSurface = "docker"
	ConfigurationPublicationCompose          ConfigurationPublicationSurface = "compose"
	ConfigurationPublicationHelm             ConfigurationPublicationSurface = "helm"
	ConfigurationPublicationExample          ConfigurationPublicationSurface = "example"
	ConfigurationPublicationCanonicalExample ConfigurationPublicationSurface = "canonical-example"
)

// ConfigurationPublication declares where a persistence-sensitive setting must
// be published and the default required by each surface.
type ConfigurationPublication struct {
	Environment   string
	ConfigFileKey string
	Surfaces      []ConfigurationPublicationSurface
	Paths         map[ConfigurationPublicationSurface][]string
	Defaults      map[ConfigurationPublicationSurface]string
}

// PublicationRequirements returns the delivery contract for state that must
// survive container replacement. Credentials and ephemeral paths stay out of
// this list deliberately.
func PublicationRequirements() []ConfigurationPublication {
	canonicalDefault := ""
	for _, setting := range Catalog().Settings {
		if setting.Environment == "HITKEEP_DATA_PATH" {
			canonicalDefault = setting.Default
			break
		}
	}

	return []ConfigurationPublication{{
		Environment:   "HITKEEP_DATA_PATH",
		ConfigFileKey: "data-path",
		Surfaces: []ConfigurationPublicationSurface{
			ConfigurationPublicationDocker,
			ConfigurationPublicationCompose,
			ConfigurationPublicationHelm,
			ConfigurationPublicationExample,
			ConfigurationPublicationCanonicalExample,
		},
		Paths: map[ConfigurationPublicationSurface][]string{
			ConfigurationPublicationDocker:  {"Dockerfile"},
			ConfigurationPublicationCompose: {"compose.yaml", "compose.cluster.yaml", "compose.dev.yaml"},
			ConfigurationPublicationHelm:    {"charts/hitkeep/templates/statefulset.yaml"},
			ConfigurationPublicationExample: {
				"examples/compose.yml",
				"examples/compose.caddy-on-demand.yml",
				"examples/compose.caddy.yml",
				"examples/compose.nginx-custom-tracking.yml",
				"examples/compose.traefik-custom-tracking.yml",
			},
			ConfigurationPublicationCanonicalExample: {"config.example.yaml"},
		},
		Defaults: map[ConfigurationPublicationSurface]string{
			ConfigurationPublicationDocker:           "/var/lib/hitkeep/data",
			ConfigurationPublicationCompose:          "/var/lib/hitkeep/data",
			ConfigurationPublicationHelm:             "/var/lib/hitkeep/data",
			ConfigurationPublicationExample:          "/var/lib/hitkeep/data",
			ConfigurationPublicationCanonicalExample: canonicalDefault,
		},
	}}
}

// ConfigurationPublicationClass declares how an active setting reaches an
// operator. Every catalog setting must choose one explicitly.
type ConfigurationPublicationClass string

const (
	ConfigurationPublicationUnclassified ConfigurationPublicationClass = ""
	ConfigurationPublicationOperator     ConfigurationPublicationClass = "operator"
	ConfigurationPublicationPersistent   ConfigurationPublicationClass = "persistent-data"
	ConfigurationPublicationCommand      ConfigurationPublicationClass = "command"
	ConfigurationPublicationCloud        ConfigurationPublicationClass = "managed-cloud"
)

func configurationPublication(field reflect.StructField) ConfigurationPublicationClass {
	switch field.Name {
	case "DataPath":
		return ConfigurationPublicationPersistent
	case "Healthcheck":
		return ConfigurationPublicationCommand
	case "CloudHosted", "CloudSignupEnabled", "CloudJurisdiction", "CloudRegion", "CloudUpgradeURL", "CloudSupportURL", "CloudPlanCode", "CloudPlanName", "CloudMaxTeams", "CloudMaxSitesPerTeam", "CloudMaxRetentionDays", "CloudMaxTeamMembers", "CloudAllowSSO", "CloudAllowCustomBranding", "StripeSecretKey", "StripePublishableKey", "StripeWebhookSecret", "StripePortalConfigurationID", "StripePriceProMonthly", "StripePriceBusinessMonthly", "StripePriceProAnnual", "StripePriceBusinessAnnual", "CloudCheckoutSuccessURL", "CloudCheckoutCancelURL":
		return ConfigurationPublicationCloud
	case "HTTPAddr", "DBPath", "BindAddr", "JoinAddr", "IngestRateLimit", "ApiRateLimit", "AuthRateLimit", "WebhookRateLimit", "DataRetentionDays", "NodeName", "DuckDBMemoryLimit", "DuckDBThreads", "DBCompactOnStart", "DBAutoRecover", "DBAutoRecoverWAL", "DBCheckpointIntervalMinutes", "DBRecoveryPath", "ArchivePath", "PublicURL", "LogLevel", "JWTSecret", "TrustedProxies", "NSQTCPAddress", "NSQHTTPAddress", "ApiBurst", "AuthBurst", "IngestBurst", "WebhookBurst", "AuthRememberMeDays", "AuthSessionMinutes", "AuthSessionWarningSeconds", "SocialGoogleClientID", "SocialGoogleClientSecret", "SocialGitHubClientID", "SocialGitHubClientSecret", "SocialMicrosoftClientID", "SocialMicrosoftClientSecret", "SocialMicrosoftTenant", "SocialSignupEnabled", "MailDriver", "MailEncryption", "MailInsecureSkipVerify", "MailHost", "MailPort", "MailUsername", "MailPassword", "MailFromAddress", "MailFromName", "SpamFilterAutoUpdate", "SpamFilterPath", "SpamFilterUpdateIntervalMin", "ImportMaxStageBytes", "ImportStageRetentionDays", "ImportAPIURL", "ImportAPIToken", "WebhookAllowDevelopmentTargets", "WebhookDeliveryTimeoutSeconds", "WebhookDeliveryConcurrency", "WebhookPerEndpointConcurrency", "WebhookMaxAttempts", "WebhookRetryBaseSeconds", "WebhookRetryMaxSeconds", "WebhookRetentionDays", "WebhookSweepSeconds", "GoogleSearchConsoleClientID", "GoogleSearchConsoleClientSecret", "GoogleSearchConsoleRedirectURL", "BackupPath", "BackupIntervalMinutes", "BackupRetentionCount", "S3AccessKeyID", "S3SecretAccessKey", "S3SessionToken", "S3Region", "S3Endpoint", "S3URLStyle", "S3UseSSL", "MCPEnabled", "MCPPath", "MCPMaxRangeDays", "MCPDocsEnabled", "MCPDocsURL", "MCPDocsCacheMinutes", "CustomTrackingDNSTarget", "CustomTrackingTLSMode", "CaddyTLSAskToken", "AIEnabled", "AskAIEnabled", "AIProvider", "AIModel", "AIBaseURL", "AIRegion", "AIAPIKey", "AITimeoutSeconds", "AIRequestLimit", "AITokenLimit", "AIBudgetWindowMinutes":
		return ConfigurationPublicationOperator
	default:
		return ConfigurationPublicationUnclassified
	}
}

func configFileKey(environment, flag string) string {
	if environment == "" {
		return flag
	}
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(environment, "HITKEEP_")), "_", "-")
}

func configurationCategories() []ConfigurationCategory {
	return []ConfigurationCategory{
		{ID: "general", Title: "General settings"},
		{ID: "data", Title: "Data management"},
		{ID: "custom-tracking", Title: "Custom tracking domains"},
		{ID: "spam-filtering", Title: "Spam filtering"},
		{ID: "backups", Title: "Database backups"},
		{ID: "s3", Title: "S3 archive storage"},
		{ID: "clustering", Title: "Server and clustering"},
		{ID: "mcp", Title: "Optional MCP route"},
		{ID: "ai", Title: "Optional AI model configuration"},
		{ID: "webhooks", Title: "Outbound webhook delivery"},
		{ID: "search-console", Title: "Google Search Console"},
		{ID: "social-sign-in", Title: "Social sign-in"},
		{ID: "email", Title: "Email (SMTP)"},
		{ID: "rate-limits", Title: "Rate limiting"},
		{ID: "trusted-proxies", Title: "Trusted proxies"},
		{ID: "internals", Title: "Internals (advanced)"},
		{ID: "managed-cloud", Title: "Managed cloud runtime"},
	}
}

func configurationCategory(field reflect.StructField) string {
	if field.Tag.Get("cloud") == "true" {
		return "managed-cloud"
	}
	name := field.Name
	switch {
	case name == "HTTPAddr" || name == "PublicURL" || name == "LogLevel" || name == "JWTSecret" || name == "Healthcheck" || strings.HasPrefix(name, "AuthRemember") || strings.HasPrefix(name, "AuthSession"):
		return "general"
	case name == "DBPath" || name == "DataPath" || name == "ArchivePath" || name == "DataRetentionDays" || strings.HasPrefix(name, "DuckDB") || strings.HasPrefix(name, "DB") || strings.HasPrefix(name, "Import"):
		return "data"
	case strings.HasPrefix(name, "CustomTracking") || name == "CaddyTLSAskToken":
		return "custom-tracking"
	case strings.HasPrefix(name, "SpamFilter"):
		return "spam-filtering"
	case strings.HasPrefix(name, "Backup"):
		return "backups"
	case strings.HasPrefix(name, "S3"):
		return "s3"
	case name == "BindAddr" || name == "JoinAddr" || name == "NodeName":
		return "clustering"
	case strings.HasPrefix(name, "MCP"):
		return "mcp"
	case strings.HasPrefix(name, "AI") || name == "AskAIEnabled":
		return "ai"
	case strings.HasPrefix(name, "Webhook") && name != "WebhookRateLimit" && name != "WebhookBurst":
		return "webhooks"
	case strings.HasPrefix(name, "GoogleSearchConsole"):
		return "search-console"
	case strings.HasPrefix(name, "Social"):
		return "social-sign-in"
	case strings.HasPrefix(name, "Mail"):
		return "email"
	case strings.HasSuffix(name, "RateLimit") || strings.HasSuffix(name, "Burst"):
		return "rate-limits"
	case name == "TrustedProxies":
		return "trusted-proxies"
	case strings.HasPrefix(name, "NSQ"):
		return "internals"
	default:
		return "uncategorized"
	}
}

func configurationCategoryIndex(category string) int {
	for index, candidate := range configurationCategories() {
		if candidate.ID == category {
			return index
		}
	}
	return len(configurationCategories())
}

func configurationType(kind reflect.Kind) string {
	switch kind { //nolint:exhaustive // Config supports only these scalar kinds
	case reflect.String:
		return "string"
	case reflect.Int:
		return "integer"
	case reflect.Bool:
		return "boolean"
	case reflect.Float64:
		return "number"
	default:
		return "unsupported"
	}
}

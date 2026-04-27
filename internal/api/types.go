package api

import (
	"time"

	"github.com/google/uuid"
)

type Hit struct {
	ID             uuid.UUID `json:"id"`
	SiteID         uuid.UUID `json:"site_id"`
	SessionID      uuid.UUID `json:"session_id"`
	PageID         uuid.UUID `json:"page_id"`
	Timestamp      time.Time `json:"timestamp"`
	Path           string    `json:"path"`
	Hostname       *string   `json:"hostname"`
	Referrer       *string   `json:"referrer"`
	UserAgent      *string   `json:"user_agent"`
	ViewportWidth  *int      `json:"viewport_width"`
	ViewportHeight *int      `json:"viewport_height"`
	ScreenWidth    *int      `json:"screen_width"`
	ScreenHeight   *int      `json:"screen_height"`
	Language       *string   `json:"language"`
	CountryCode    *string   `json:"country_code"`
	UTMSource      *string   `json:"utm_source"`
	UTMMedium      *string   `json:"utm_medium"`
	UTMCampaign    *string   `json:"utm_campaign"`
	UTMTerm        *string   `json:"utm_term"`
	UTMContent     *string   `json:"utm_content"`
	IsUnique       *bool     `json:"is_unique"`
}

type Site struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
	Domain            string    `json:"domain"`
	OwnerEmail        string    `json:"owner_email,omitempty"`
	DataRetentionDays int       `json:"data_retention_days"`
	CreatedAt         time.Time `json:"created_at"`
}

type ShareLink struct {
	ID        uuid.UUID `json:"id"`
	SiteID    uuid.UUID `json:"site_id"`
	TokenHint string    `json:"token_hint"`
	CreatedAt time.Time `json:"created_at"`
}

type APIClient struct {
	ID           uuid.UUID           `json:"id"`
	UserID       *uuid.UUID          `json:"user_id,omitempty"`
	TenantID     *uuid.UUID          `json:"tenant_id,omitempty"`
	OwnerType    string              `json:"owner_type"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	InstanceRole string              `json:"instance_role"`
	ExpiresAt    *time.Time          `json:"expires_at,omitempty"`
	LastUsedAt   *time.Time          `json:"last_used_at,omitempty"`
	RevokedAt    *time.Time          `json:"revoked_at,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	SiteRoles    []APIClientSiteRole `json:"site_roles"`
}

type APIClientSiteRole struct {
	SiteID uuid.UUID `json:"site_id"`
	Role   string    `json:"role"`
}

type Event struct {
	ID         uuid.UUID      `json:"id"`
	SiteID     uuid.UUID      `json:"site_id"`
	SessionID  uuid.UUID      `json:"session_id"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties"`
	Timestamp  time.Time      `json:"timestamp"`
}

type Goal struct {
	ID        uuid.UUID `json:"id"`
	SiteID    uuid.UUID `json:"site_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "event" or "path"
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type FunnelStep struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Funnel struct {
	ID        uuid.UUID    `json:"id"`
	SiteID    uuid.UUID    `json:"site_id"`
	Name      string       `json:"name"`
	Steps     []FunnelStep `json:"steps"`
	CreatedAt time.Time    `json:"created_at"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	GivenName    string    `json:"given_name,omitempty"`
	LastName     string    `json:"last_name,omitempty"`
	InstanceRole string    `json:"instance_role,omitempty"`
	Password     string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserProfile struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	GivenName   string    `json:"given_name,omitempty"`
	LastName    string    `json:"last_name,omitempty"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
}

type UserPreferences struct {
	DefaultLocale string `json:"default_locale"`
}

type AuthSession struct {
	ExpiresAt              time.Time  `json:"expires_at"`
	IssuedAt               time.Time  `json:"issued_at"`
	DurationSeconds        int        `json:"duration_seconds"`
	WarningSeconds         int        `json:"warning_seconds"`
	Extendable             bool       `json:"extendable"`
	TimingAdjustable       bool       `json:"timing_adjustable"`
	Remembered             bool       `json:"remembered"`
	RememberExpiresAt      *time.Time `json:"remember_expires_at,omitempty"`
	RememberMeDurationDays int        `json:"remember_me_duration_days"`
}

type UserPasskey struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSecurityStatus struct {
	TOTPEnabled            bool          `json:"totp_enabled"`
	TOTPPending            bool          `json:"totp_pending"`
	Passkeys               []UserPasskey `json:"passkeys"`
	RecoveryCodesGenerated bool          `json:"recovery_codes_generated"`
	RecoveryCodesRemaining int           `json:"recovery_codes_remaining"`
}

type UserTOTPSetup struct {
	//nolint:gosec // TOTP bootstrap secret is intentionally returned to the authenticated user during setup.
	Secret     string    `json:"secret"`
	OTPAuthURL string    `json:"otpauth_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type UserRecoveryCodesResponse struct {
	Codes     []string `json:"codes"`
	Remaining int      `json:"remaining"`
}

type AnalyticsParams struct {
	SiteID       uuid.UUID
	UserID       uuid.UUID
	Start        time.Time
	End          time.Time
	Filters      []Filter
	GoalIDs      []uuid.UUID
	FunnelIDs    []uuid.UUID
	CompareStart time.Time
	CompareEnd   time.Time
}

type ComparisonStats struct {
	TotalPageviews     int              `json:"total_pageviews"`
	UniqueSessions     int              `json:"unique_sessions"`
	BounceRate         float64          `json:"bounce_rate"`
	AvgSessionDuration float64          `json:"avg_session_duration"`
	PagesPerSession    float64          `json:"pages_per_session"`
	ChartData          []ChartDataPoint `json:"chart_data"`
	UTMCampaignHits    int              `json:"utm_campaign_hits"`
	UTMContentHits     int              `json:"utm_content_hits"`
	UTMMediumHits      int              `json:"utm_medium_hits"`
	UTMSourceHits      int              `json:"utm_source_hits"`
	UTMTermHits        int              `json:"utm_term_hits"`
	Goals              []GoalStats      `json:"goals"`
	TotalConversions   int              `json:"total_conversions"`
}

type ChartDataPoint struct {
	Time      time.Time `json:"time"`
	Pageviews int       `json:"pageviews"`
	Visitors  int       `json:"visitors"`
}

type GoalSeriesPoint struct {
	Time        time.Time `json:"time"`
	Conversions int       `json:"conversions"`
}

type FunnelSeriesPoint struct {
	Time        time.Time `json:"time"`
	Entries     int       `json:"entries"`
	Completions int       `json:"completions"`
}

type MetricStat struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type SiteStats struct {
	LiveVisitors int `json:"live_visitors"`

	TotalPageviews     int              `json:"total_pageviews"`
	UniqueSessions     int              `json:"unique_sessions"`
	BounceRate         float64          `json:"bounce_rate"`
	AvgSessionDuration float64          `json:"avg_session_duration"`
	PagesPerSession    float64          `json:"pages_per_session"`
	ChartData          []ChartDataPoint `json:"chart_data"`
	TopPages           []MetricStat     `json:"top_pages"`
	TopLandingPages    []MetricStat     `json:"top_landing_pages"`
	TopExitPages       []MetricStat     `json:"top_exit_pages"`
	TopReferrers       []MetricStat     `json:"top_referrers"`
	TopDevices         []MetricStat     `json:"top_devices"`
	TopCountries       []MetricStat     `json:"top_countries"`
	TopBrowsers        []MetricStat     `json:"top_browsers"`
	TopAIBots          []MetricStat     `json:"top_ai_bots"`
	TopAISources       []MetricStat     `json:"top_ai_sources"`
	TopLanguages       []MetricStat     `json:"top_languages"`
	TopUTMCampaigns    []MetricStat     `json:"top_utm_campaigns"`
	TopUTMContents     []MetricStat     `json:"top_utm_contents"`
	TopUTMMediums      []MetricStat     `json:"top_utm_mediums"`
	TopUTMSources      []MetricStat     `json:"top_utm_sources"`
	TopUTMTerms        []MetricStat     `json:"top_utm_terms"`
	AIBotHits          int              `json:"ai_bot_hits"`
	AISourceVisits     int              `json:"ai_source_visits"`
	UTMCampaignHits    int              `json:"utm_campaign_hits"`
	UTMContentHits     int              `json:"utm_content_hits"`
	UTMMediumHits      int              `json:"utm_medium_hits"`
	UTMSourceHits      int              `json:"utm_source_hits"`
	UTMTermHits        int              `json:"utm_term_hits"`
	Goals              []GoalStats      `json:"goals"`
	Comparison         *ComparisonStats `json:"comparison,omitempty"`
}

type EventNamesParams struct {
	SiteID uuid.UUID
	Start  time.Time
	End    time.Time
}

type EventBreakdownParams struct {
	SiteID      uuid.UUID
	Start       time.Time
	End         time.Time
	EventName   string
	PropertyKey string
}

type EventTimeseriesParams struct {
	SiteID         uuid.UUID
	Start          time.Time
	End            time.Time
	EventName      string
	PropertyKey    string // optional — filter to events with this property key
	PropertyValue  string // optional — requires PropertyKey; filter to events where key == value
	DimensionKey   string // optional: "path" | "referrer" | "device" | "country"
	DimensionValue string // optional — restrict to sessions where hits match this pre-processed dimension value
}

type EventAudienceParams struct {
	SiteID         uuid.UUID
	Start          time.Time
	End            time.Time
	EventName      string
	PropertyKey    string // optional — filter sessions by property
	PropertyValue  string // optional — requires PropertyKey
	DimensionKey   string // optional: "path" | "referrer" | "device" | "country"
	DimensionValue string // optional — filter hits to this pre-processed dimension value
}

type ChatbotExportParams struct {
	SiteID     uuid.UUID
	Start      time.Time
	End        time.Time
	ScopeKey   string // optional: "provider" | "bot_id" | "surface" | "model"
	ScopeValue string // optional — requires ScopeKey
}

type EventAudience struct {
	TopPages     []MetricStat `json:"top_pages"`
	TopReferrers []MetricStat `json:"top_referrers"`
	TopDevices   []MetricStat `json:"top_devices"`
	TopCountries []MetricStat `json:"top_countries"`
}

type EventSeriesPoint struct {
	Time  time.Time `json:"time"`
	Count int       `json:"count"`
}

type AIFetch struct {
	ID              uuid.UUID `json:"id"`
	SiteID          uuid.UUID `json:"site_id"`
	Timestamp       time.Time `json:"timestamp"`
	AssistantName   string    `json:"assistant_name"`
	AssistantFamily string    `json:"assistant_family"`
	Path            string    `json:"path"`
	Hostname        *string   `json:"hostname,omitempty"`
	StatusCode      int       `json:"status_code"`
	ContentType     *string   `json:"content_type,omitempty"`
	ResourceType    string    `json:"resource_type"`
	ResponseMs      *int      `json:"response_ms,omitempty"`
	BytesServed     *int64    `json:"bytes_served,omitempty"`
	UserAgent       *string   `json:"user_agent,omitempty"`
}

type AIFetchQueryParams struct {
	SiteID          uuid.UUID
	Start           time.Time
	End             time.Time
	AssistantName   string
	AssistantFamily string
	ResourceType    string
}

type AIFetchCorrelationParams struct {
	SiteID          uuid.UUID
	Start           time.Time
	End             time.Time
	AssistantName   string
	AssistantFamily string
	ResourceType    string
	WindowDays      int
}

type AIFetchOverview struct {
	TotalRequests     int64        `json:"total_requests"`
	UniquePaths       int64        `json:"unique_paths"`
	UniqueAssistants  int64        `json:"unique_assistants"`
	ErrorRate4xx      float64      `json:"error_rate_4xx"`
	ErrorRate5xx      float64      `json:"error_rate_5xx"`
	MedianResponseMs  int          `json:"median_response_ms"`
	TotalBytes        int64        `json:"total_bytes"`
	TopAssistants     []MetricStat `json:"top_assistants"`
	TopFamilies       []MetricStat `json:"top_families"`
	TopPaths          []MetricStat `json:"top_paths"`
	TopErrorPaths     []MetricStat `json:"top_error_paths"`
	ResourceTypeSplit []MetricStat `json:"resource_type_split"`
}

type AIFetchSeriesPoint struct {
	Time  time.Time `json:"time"`
	Count int       `json:"count"`
}

type AIFetchCorrelationSummary struct {
	TotalFetches        int64 `json:"total_fetches"`
	FetchedPaths        int64 `json:"fetched_paths"`
	CorrelatedPaths     int64 `json:"correlated_paths"`
	AIReferredVisits    int64 `json:"ai_referred_visits"`
	UncorrelatedFetches int64 `json:"uncorrelated_fetches"`
}

type AIFetchCitationYieldRow struct {
	Path             string  `json:"path"`
	AssistantName    string  `json:"assistant_name"`
	FetchCount       int64   `json:"fetch_count"`
	AIReferredVisits int64   `json:"ai_referred_visits"`
	CitationYieldPct float64 `json:"citation_yield_pct"`
}

type AIFetchOpportunityRow struct {
	Path             string  `json:"path"`
	FetchCount       int64   `json:"fetch_count"`
	AIReferredVisits int64   `json:"ai_referred_visits"`
	ErrorRequests    int64   `json:"error_requests"`
	ErrorRatePct     float64 `json:"error_rate_pct"`
}

type AIFetchFailureHotspot struct {
	AssistantName string  `json:"assistant_name"`
	PathPrefix    string  `json:"path_prefix"`
	TotalRequests int64   `json:"total_requests"`
	ErrorRequests int64   `json:"error_requests"`
	ErrorRatePct  float64 `json:"error_rate_pct"`
}

type AIFetchCorrelationReport struct {
	Summary          AIFetchCorrelationSummary `json:"summary"`
	CitationYield    []AIFetchCitationYieldRow `json:"citation_yield"`
	OpportunityPages []AIFetchOpportunityRow   `json:"opportunity_pages"`
	FailureHotspots  []AIFetchFailureHotspot   `json:"failure_hotspots"`
}

type EcommerceParams struct {
	SiteID   uuid.UUID
	Start    time.Time
	End      time.Time
	Filters  []Filter
	ItemID   string
	ItemName string
	Limit    int
}

type EcommerceSummary struct {
	Revenue                float64 `json:"revenue"`
	Orders                 int     `json:"orders"`
	AverageOrderValue      float64 `json:"average_order_value"`
	CheckoutStarts         int     `json:"checkout_starts"`
	CheckoutConversionRate float64 `json:"checkout_conversion_rate"`
	Currency               string  `json:"currency"`
}

type EcommerceSeriesPoint struct {
	Time    time.Time `json:"time"`
	Revenue float64   `json:"revenue"`
	Orders  int       `json:"orders"`
}

type EcommerceProductStat struct {
	ItemID   string  `json:"item_id"`
	ItemName string  `json:"item_name"`
	Revenue  float64 `json:"revenue"`
	Orders   int     `json:"orders"`
	Quantity int     `json:"quantity"`
}

type EcommerceSourceStat struct {
	UTMSource   string  `json:"utm_source"`
	UTMMedium   string  `json:"utm_medium"`
	UTMCampaign string  `json:"utm_campaign"`
	Referrer    string  `json:"referrer"`
	Revenue     float64 `json:"revenue"`
	Orders      int     `json:"orders"`
}

type GoalStats struct {
	GoalID         uuid.UUID `json:"goal_id"`
	Name           string    `json:"name"`
	Conversions    int       `json:"conversions"`
	ConversionRate float64   `json:"conversion_rate"`
}

type FunnelStepStats struct {
	StepIndex      int     `json:"step_index"`
	Name           string  `json:"name"`
	Visitors       int     `json:"visitors"`
	Dropoff        int     `json:"dropoff"`
	ConversionRate float64 `json:"conversion_rate"`
}

type FunnelStats struct {
	FunnelID              uuid.UUID         `json:"funnel_id"`
	Name                  string            `json:"name"`
	Steps                 []FunnelStepStats `json:"steps"`
	TotalEntries          int               `json:"total_entries"`
	TotalCompletions      int               `json:"total_completions"`
	OverallConversionRate float64           `json:"overall_conversion_rate"`
}

type HitQueryParams struct {
	SiteID    uuid.UUID
	UserID    uuid.UUID
	Start     time.Time
	End       time.Time
	Query     string
	SortField string
	SortOrder string
	Limit     int
	Offset    int
	Filters   []Filter
}

type Filter struct {
	Type  string
	Value string
}

type PaginatedHits struct {
	Data  []Hit `json:"data"`
	Total int   `json:"total"`
}

type SiteMember struct {
	ID      uuid.UUID `json:"id"`
	UserID  uuid.UUID `json:"user_id"`
	Email   string    `json:"email"`
	Role    string    `json:"role"`
	AddedAt time.Time `json:"added_at"`
}

type Team struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	LogoURL      string            `json:"logo_url"`
	Role         string            `json:"role"`
	CreatedAt    time.Time         `json:"created_at"`
	Usage        *TeamUsageSummary `json:"usage,omitempty"`
	Entitlements *TeamEntitlements `json:"entitlements,omitempty"`
	Plan         *TeamPlan         `json:"plan,omitempty"`
}

type TeamUsageSummary struct {
	CurrentSites          int `json:"current_sites"`
	CurrentMembers        int `json:"current_members"`
	CurrentPendingInvites int `json:"current_pending_invites"`
}

type TeamEntitlements struct {
	MaxSitesPerTeam     int  `json:"max_sites_per_team"`
	MaxTeamMembers      int  `json:"max_team_members"`
	MaxRetentionDays    int  `json:"max_retention_days"`
	AllowSSO            bool `json:"allow_sso"`
	AllowCustomBranding bool `json:"allow_custom_branding"`
}

type TeamPlan struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	UpgradeURL string `json:"upgrade_url,omitempty"`
	SupportURL string `json:"support_url,omitempty"`
}

type CloudPlanTier struct {
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Entitlements TeamEntitlements `json:"entitlements"`
}

type CloudStatus struct {
	Hosted        bool   `json:"hosted"`
	SignupEnabled bool   `json:"signup_enabled"`
	Jurisdiction  string `json:"jurisdiction,omitempty"`
	Region        string `json:"region,omitempty"`
	UpgradeURL    string `json:"upgrade_url,omitempty"`
	SupportURL    string `json:"support_url,omitempty"`
}

type TeamMember struct {
	ID      uuid.UUID `json:"id"`
	UserID  uuid.UUID `json:"user_id"`
	Email   string    `json:"email"`
	Role    string    `json:"role"`
	AddedAt time.Time `json:"added_at"`
}

type TeamInvite struct {
	ID            uuid.UUID  `json:"id"`
	TeamID        uuid.UUID  `json:"team_id"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	InvitedUserID *uuid.UUID `json:"invited_user_id,omitempty"`
	Status        string     `json:"status"`
	CreatedBy     *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	AcceptedAt    *time.Time `json:"accepted_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type TeamAuditEntry struct {
	ID           uuid.UUID  `json:"id"`
	TeamID       uuid.UUID  `json:"team_id"`
	Action       string     `json:"action"`
	Details      string     `json:"details"`
	ActorUserID  *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorEmail   string     `json:"actor_email,omitempty"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty"`
	TargetEmail  string     `json:"target_email,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type TeamAuditListResponse struct {
	Entries []TeamAuditEntry `json:"entries"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	HasMore bool             `json:"has_more"`
	Action  string           `json:"action,omitempty"`
}

type AdminTeam struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	IsDefault   bool      `json:"is_default"`
	IsArchived  bool      `json:"is_archived"`
	MemberCount int       `json:"member_count"`
	SiteCount   int       `json:"site_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdminDeleteUserBlockedResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Teams   []Team `json:"teams"`
}

type AdminDeleteTeamResponse struct {
	Status string    `json:"status"`
	TeamID uuid.UUID `json:"team_id"`
	Name   string    `json:"name"`
}

type AdminDisableUserMFAResponse struct {
	Status              string `json:"status"`
	TOTPDisabled        bool   `json:"totp_disabled"`
	PasskeysDeleted     int    `json:"passkeys_deleted"`
	SessionsInvalidated int    `json:"sessions_invalidated"`
}

type IPExclusion struct {
	ID          uuid.UUID  `json:"id"`
	SiteID      *uuid.UUID `json:"site_id,omitempty"`
	CIDR        string     `json:"cidr"`
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
}

// ReportFrequency is the cadence for scheduled analytics emails.
type ReportFrequency string

const (
	ReportFrequencyDaily   ReportFrequency = "daily"
	ReportFrequencyWeekly  ReportFrequency = "weekly"
	ReportFrequencyMonthly ReportFrequency = "monthly"
)

// SiteReportSubscription is one site's subscription state for the current user.
// Domain is read-only, populated server-side.
type SiteReportSubscription struct {
	SiteID  uuid.UUID `json:"site_id"`
	Domain  string    `json:"domain"`
	Daily   bool      `json:"daily"`
	Weekly  bool      `json:"weekly"`
	Monthly bool      `json:"monthly"`
}

// DigestSubscription holds the per-frequency enabled flags for a consolidated digest.
type DigestSubscription struct {
	Daily   bool `json:"daily"`
	Weekly  bool `json:"weekly"`
	Monthly bool `json:"monthly"`
}

// ReportSubscriptions is the full subscription state for a user.
type ReportSubscriptions struct {
	Sites  []SiteReportSubscription `json:"sites"`
	Digest DigestSubscription       `json:"digest"`
}

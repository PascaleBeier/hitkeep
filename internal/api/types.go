package api

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

type DateOnly time.Time

func NewDateOnly(value time.Time) DateOnly {
	return DateOnly(dateOnlyTime(value))
}

func NewDateOnlyPtr(value *time.Time) *DateOnly {
	if value == nil {
		return nil
	}
	date := NewDateOnly(*value)
	return &date
}

func (d DateOnly) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(time.Time(d).UTC().Format(time.DateOnly))), nil
}

func (d *DateOnly) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	value, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return err
	}
	*d = DateOnly(parsed)
	return nil
}

func dateOnlyTime(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

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
	TrackerSource  string    `json:"-"`
	TrackerVersion string    `json:"-"`
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

type GoogleSearchConsoleStatus struct {
	Status                 string     `json:"status"`
	Configured             bool       `json:"configured"`
	Connected              bool       `json:"connected"`
	CredentialStatus       string     `json:"credential_status"`
	ConnectedAccountLabel  string     `json:"connected_account_label,omitempty"`
	LastConnectedAt        *time.Time `json:"last_connected_at,omitempty"`
	LastDisconnectedAt     *time.Time `json:"last_disconnected_at,omitempty"`
	NeedsAdminAction       bool       `json:"needs_admin_action"`
	CanManage              bool       `json:"can_manage"`
	ManagedCredentialsMode string     `json:"managed_credentials_mode"`
}

type GoogleSearchConsoleConnectResponse struct {
	AuthURL string `json:"auth_url"`
}

type GoogleSearchConsoleProperty struct {
	URI             string `json:"uri"`
	PermissionLevel string `json:"permission_level"`
}

type GoogleSearchConsolePropertiesResponse struct {
	Properties []GoogleSearchConsoleProperty `json:"properties"`
}

type GoogleSearchConsoleSiteMappingResponse struct {
	SiteID                  uuid.UUID                      `json:"site_id"`
	TeamID                  uuid.UUID                      `json:"team_id"`
	Mapped                  bool                           `json:"mapped"`
	PropertyURI             string                         `json:"property_uri,omitempty"`
	PropertyPermissionLevel string                         `json:"property_permission_level,omitempty"`
	MappedAt                *time.Time                     `json:"mapped_at,omitempty"`
	CanManage               bool                           `json:"can_manage"`
	SyncStatus              *GoogleSearchConsoleSyncStatus `json:"sync_status,omitempty"`
}

type GoogleSearchConsoleMapPropertyRequest struct {
	PropertyURI string `json:"property_uri"`
}

type GoogleSearchConsoleSyncStatus struct {
	State             string     `json:"state"`
	ImportedStartDate *DateOnly  `json:"imported_start_date,omitempty"`
	ImportedEndDate   *DateOnly  `json:"imported_end_date,omitempty"`
	LastSuccessAt     *time.Time `json:"last_success_at,omitempty"`
	LastAttemptAt     *time.Time `json:"last_attempt_at,omitempty"`
	LastErrorCategory string     `json:"last_error_category,omitempty"`
	NextRetryAt       *time.Time `json:"next_retry_at,omitempty"`
	Manual            bool       `json:"manual"`
}

type SearchConsoleReportParams struct {
	SiteID      uuid.UUID
	PropertyURI string
	Start       time.Time
	End         time.Time
	Page        string
	Path        string
	Country     string
	Device      string
	Limit       int
}

type SearchConsoleOverview struct {
	DataSource      string  `json:"data_source"`
	Clicks          int     `json:"clicks"`
	Impressions     int     `json:"impressions"`
	CTR             float64 `json:"ctr"`
	AveragePosition float64 `json:"average_position"`
}

type SearchConsoleMetricPoint struct {
	Date            DateOnly `json:"date"`
	Clicks          int      `json:"clicks"`
	Impressions     int      `json:"impressions"`
	CTR             float64  `json:"ctr"`
	AveragePosition float64  `json:"average_position"`
}

type SearchConsoleSeriesResponse struct {
	DataSource string                     `json:"data_source"`
	Series     []SearchConsoleMetricPoint `json:"series"`
}

type SearchConsoleDimensionRow struct {
	Value           string  `json:"value"`
	Clicks          int     `json:"clicks"`
	Impressions     int     `json:"impressions"`
	CTR             float64 `json:"ctr"`
	AveragePosition float64 `json:"average_position"`
}

type SearchConsoleDimensionResponse struct {
	DataSource string                      `json:"data_source"`
	Dimension  string                      `json:"dimension"`
	Rows       []SearchConsoleDimensionRow `json:"rows"`
}

type Event struct {
	ID             uuid.UUID      `json:"id"`
	SiteID         uuid.UUID      `json:"site_id"`
	SessionID      uuid.UUID      `json:"session_id"`
	Name           string         `json:"name"`
	Properties     map[string]any `json:"properties"`
	Timestamp      time.Time      `json:"timestamp"`
	TrackerSource  string         `json:"-"`
	TrackerVersion string         `json:"-"`
}

type ServerPageviewIngestRequest struct {
	URL       string    `json:"url"`
	Timestamp string    `json:"timestamp"`
	VisitorIP string    `json:"visitor_ip"`
	UserAgent string    `json:"user_agent"`
	Referrer  *string   `json:"referrer"`
	VPWidth   *int      `json:"viewport_width"`
	VPHeight  *int      `json:"viewport_height"`
	SCWidth   *int      `json:"screen_width"`
	SCHeight  *int      `json:"screen_height"`
	Language  *string   `json:"language"`
	DNT       bool      `json:"dnt"`
	SessionID uuid.UUID `json:"session_id"`
	PageID    uuid.UUID `json:"page_id"`
}

type ServerEventIngestRequest struct {
	URL        string         `json:"url"`
	Timestamp  string         `json:"timestamp"`
	VisitorIP  string         `json:"visitor_ip"`
	UserAgent  string         `json:"user_agent"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties"`
	Referrer   *string        `json:"referrer"`
	DNT        bool           `json:"dnt"`
	SessionID  uuid.UUID      `json:"session_id"`
}

type TrackingStatus string

const (
	TrackingStatusWaiting        TrackingStatus = "waiting"
	TrackingStatusLive           TrackingStatus = "live"
	TrackingStatusDormant        TrackingStatus = "dormant"
	TrackingStatusDomainMismatch TrackingStatus = "domain_mismatch"
)

type SiteTrackingStatus struct {
	SiteID                 uuid.UUID      `json:"site_id"`
	TenantID               uuid.UUID      `json:"tenant_id"`
	Status                 TrackingStatus `json:"status"`
	FirstHitAt             *time.Time     `json:"first_hit_at,omitempty"`
	LastHitAt              *time.Time     `json:"last_hit_at,omitempty"`
	LastEventAt            *time.Time     `json:"last_event_at,omitempty"`
	LastHostname           string         `json:"last_hostname,omitempty"`
	LastEventName          string         `json:"last_event_name,omitempty"`
	LastAutomaticEventAt   *time.Time     `json:"last_automatic_event_at,omitempty"`
	LastAutomaticEventName string         `json:"last_automatic_event_name,omitempty"`
	TrackerSource          string         `json:"tracker_source,omitempty"`
	TrackerVersion         string         `json:"tracker_version,omitempty"`
	ConfiguredDomain       string         `json:"configured_domain"`
	UpdatedAt              *time.Time     `json:"updated_at,omitempty"`
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
	DefaultLocale         string     `json:"default_locale"`
	DismissedOnboardingAt *time.Time `json:"dismissed_onboarding_at,omitempty"`
}

type UserTeamsResponse struct {
	ActiveTeamID  uuid.UUID   `json:"active_team_id"`
	RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
	Teams         []Team      `json:"teams"`
}

type PermissionContext struct {
	InstanceRole        string            `json:"instance_role"`
	Permissions         map[string]string `json:"permissions"`
	InstancePermissions []string          `json:"instance_permissions"`
}

type SystemStatus struct {
	NeedsSetup bool         `json:"needs_setup"`
	Version    string       `json:"version"`
	Cloud      *CloudStatus `json:"cloud,omitempty"`
}

type UserBootstrap struct {
	Session     AuthSession       `json:"session"`
	Profile     UserProfile       `json:"profile"`
	Preferences UserPreferences   `json:"preferences"`
	Teams       UserTeamsResponse `json:"teams"`
	Permissions PermissionContext `json:"permissions"`
	Sites       []Site            `json:"sites"`
	Status      SystemStatus      `json:"status"`
}

type OnboardingStep struct {
	Key        string `json:"key"`
	Complete   bool   `json:"complete"`
	Current    int    `json:"current,omitempty"`
	Target     int    `json:"target,omitempty"`
	SiteID     string `json:"site_id,omitempty"`
	SiteDomain string `json:"site_domain,omitempty"`
}

type UserOnboarding struct {
	Dismissed bool             `json:"dismissed"`
	Complete  bool             `json:"complete"`
	Steps     []OnboardingStep `json:"steps"`
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

type ImportExclusionReason struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

type SiteStats struct {
	LiveVisitors int `json:"live_visitors"`

	TotalPageviews     int                     `json:"total_pageviews"`
	UniqueSessions     int                     `json:"unique_sessions"`
	BounceRate         float64                 `json:"bounce_rate"`
	AvgSessionDuration float64                 `json:"avg_session_duration"`
	PagesPerSession    float64                 `json:"pages_per_session"`
	ChartData          []ChartDataPoint        `json:"chart_data"`
	TopPages           []MetricStat            `json:"top_pages"`
	TopLandingPages    []MetricStat            `json:"top_landing_pages"`
	TopExitPages       []MetricStat            `json:"top_exit_pages"`
	TopReferrers       []MetricStat            `json:"top_referrers"`
	TopDevices         []MetricStat            `json:"top_devices"`
	TopCountries       []MetricStat            `json:"top_countries"`
	TopBrowsers        []MetricStat            `json:"top_browsers"`
	TopAIBots          []MetricStat            `json:"top_ai_bots"`
	TopAISources       []MetricStat            `json:"top_ai_sources"`
	TopLanguages       []MetricStat            `json:"top_languages"`
	TopUTMCampaigns    []MetricStat            `json:"top_utm_campaigns"`
	TopUTMContents     []MetricStat            `json:"top_utm_contents"`
	TopUTMMediums      []MetricStat            `json:"top_utm_mediums"`
	TopUTMSources      []MetricStat            `json:"top_utm_sources"`
	TopUTMTerms        []MetricStat            `json:"top_utm_terms"`
	AIBotHits          int                     `json:"ai_bot_hits"`
	AISourceVisits     int                     `json:"ai_source_visits"`
	UTMCampaignHits    int                     `json:"utm_campaign_hits"`
	UTMContentHits     int                     `json:"utm_content_hits"`
	UTMMediumHits      int                     `json:"utm_medium_hits"`
	UTMSourceHits      int                     `json:"utm_source_hits"`
	UTMTermHits        int                     `json:"utm_term_hits"`
	Goals              []GoalStats             `json:"goals"`
	Comparison         *ComparisonStats        `json:"comparison,omitempty"`
	ImportedExcluded   []ImportExclusionReason `json:"imported_excluded,omitempty"`
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
	Filters        []Filter
	DimensionKey   string // deprecated optional single dimension filter
	DimensionValue string // optional — restrict to sessions where hits match this pre-processed dimension value
}

type EventAudienceParams struct {
	SiteID         uuid.UUID
	Start          time.Time
	End            time.Time
	EventName      string
	PropertyKey    string // optional — filter sessions by property
	PropertyValue  string // optional — requires PropertyKey
	Filters        []Filter
	DimensionKey   string // deprecated optional single dimension filter
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
	TopPages         []MetricStat            `json:"top_pages"`
	TopReferrers     []MetricStat            `json:"top_referrers"`
	TopDevices       []MetricStat            `json:"top_devices"`
	TopCountries     []MetricStat            `json:"top_countries"`
	ImportedExcluded []ImportExclusionReason `json:"imported_excluded,omitempty"`
}

type EventSeriesPoint struct {
	Time  time.Time `json:"time"`
	Count int       `json:"count"`
}

type ImportProviderDescriptor struct {
	Key                string   `json:"key"`
	Name               string   `json:"name"`
	AcceptedExtensions []string `json:"accepted_extensions"`
	Capabilities       []string `json:"capabilities"`
}

type ImportUploadFileInput struct {
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256,omitempty"`
}

type ImportUploadCreateRequest struct {
	Files []ImportUploadFileInput `json:"files"`
}

type ImportUploadFile struct {
	ID            uuid.UUID `json:"id"`
	Filename      string    `json:"filename"`
	SizeBytes     int64     `json:"size_bytes"`
	BytesReceived int64     `json:"bytes_received"`
	SHA256        string    `json:"sha256,omitempty"`
	Status        string    `json:"status"`
}

type ImportUploadCreateResponse struct {
	ImportID  uuid.UUID          `json:"import_id"`
	Provider  string             `json:"provider"`
	Status    string             `json:"status"`
	ChunkSize int64              `json:"chunk_size"`
	Files     []ImportUploadFile `json:"files"`
}

type ImportChunkResponse struct {
	ImportID      uuid.UUID `json:"import_id"`
	FileID        uuid.UUID `json:"file_id"`
	BytesReceived int64     `json:"bytes_received"`
	Complete      bool      `json:"complete"`
}

type ImportWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
}

type ImportDatasetSummary struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Files        []string `json:"files"`
	RowsScanned  int64    `json:"rows_scanned"`
	RowsAccepted int64    `json:"rows_accepted"`
	RowsSkipped  int64    `json:"rows_skipped"`
	Visitors     int64    `json:"visitors,omitempty"`
	Visits       int64    `json:"visits,omitempty"`
	Pageviews    int64    `json:"pageviews,omitempty"`
	Events       int64    `json:"events,omitempty"`
}

type ImportEventCoverage struct {
	RowsScanned  int64    `json:"rows_scanned"`
	RowsAccepted int64    `json:"rows_accepted"`
	Events       int64    `json:"events"`
	Visitors     int64    `json:"visitors"`
	EventNames   []string `json:"event_names"`
	PropertyKeys []string `json:"property_keys"`
}

type ImportEventPropertyCoverage struct {
	AttributedRows             int64    `json:"attributed_rows"`
	AttributedEvents           int64    `json:"attributed_events"`
	AttributedVisitors         int64    `json:"attributed_visitors"`
	AttributedPropertyKeys     []string `json:"attributed_property_keys"`
	UnattributedRows           int64    `json:"unattributed_rows"`
	UnattributedEvents         int64    `json:"unattributed_events"`
	UnattributedVisitors       int64    `json:"unattributed_visitors"`
	UnattributedPropertyKeys   []string `json:"unattributed_property_keys"`
	UnattributedRelationship   string   `json:"unattributed_relationship,omitempty"`
	UnavailableRelationshipMsg string   `json:"unavailable_relationship_message,omitempty"`
}

type ImportEventDimensionCoverage struct {
	Available   []string `json:"available"`
	Unavailable []string `json:"unavailable"`
	Reason      string   `json:"reason,omitempty"`
}

type ImportOverlapSummary struct {
	Policy                    string `json:"policy"`
	NativeTrafficDays         int    `json:"native_traffic_days"`
	NativeEventDays           int    `json:"native_event_days"`
	NativeEventKeys           int    `json:"native_event_keys"`
	EstimatedSkippedRows      int64  `json:"estimated_skipped_rows"`
	EstimatedSkippedPageviews int64  `json:"estimated_skipped_pageviews"`
	EstimatedSkippedEvents    int64  `json:"estimated_skipped_events"`
}

type ImportOverlapMetrics struct {
	Rows      int64
	Pageviews int64
	Events    int64
}

type ImportOverlapCandidates struct {
	TrafficByDate            map[string]ImportOverlapMetrics
	DimensionByDate          map[string]ImportOverlapMetrics
	EventByDateName          map[string]ImportOverlapMetrics
	EventDimensionByDateName map[string]ImportOverlapMetrics
	EventPropertyByDateName  map[string]ImportOverlapMetrics
}

type ImportManifest struct {
	Provider               string                       `json:"provider"`
	SourceHash             string                       `json:"source_hash"`
	DateStart              *time.Time                   `json:"date_start,omitempty"`
	DateEnd                *time.Time                   `json:"date_end,omitempty"`
	Files                  []string                     `json:"files"`
	IgnoredFiles           []string                     `json:"ignored_files"`
	MissingFiles           []string                     `json:"missing_files"`
	Datasets               []ImportDatasetSummary       `json:"datasets"`
	EventCoverage          ImportEventCoverage          `json:"event_coverage"`
	EventPropertyCoverage  ImportEventPropertyCoverage  `json:"event_property_coverage"`
	EventDimensionCoverage ImportEventDimensionCoverage `json:"event_dimension_coverage"`
	Overlap                ImportOverlapSummary         `json:"overlap"`
	Warnings               []ImportWarning              `json:"warnings"`
	RowsScanned            int64                        `json:"rows_scanned"`
	RowsAccepted           int64                        `json:"rows_accepted"`
	RowsSkipped            int64                        `json:"rows_skipped"`
	OverlapCandidates      *ImportOverlapCandidates     `json:"-"`
}

type ImportJob struct {
	ID            uuid.UUID          `json:"id"`
	SiteID        uuid.UUID          `json:"site_id"`
	Provider      string             `json:"provider"`
	Status        string             `json:"status"`
	SourceHash    string             `json:"source_hash,omitempty"`
	BytesTotal    int64              `json:"bytes_total"`
	BytesReceived int64              `json:"bytes_received"`
	RowsScanned   int64              `json:"rows_scanned"`
	RowsImported  int64              `json:"rows_imported"`
	Error         string             `json:"error,omitempty"`
	Manifest      *ImportManifest    `json:"manifest,omitempty"`
	Files         []ImportUploadFile `json:"files,omitempty"`
	CreatedBy     *uuid.UUID         `json:"-"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	ValidatedAt   *time.Time         `json:"validated_at,omitempty"`
	StartedAt     *time.Time         `json:"started_at,omitempty"`
	FinishedAt    *time.Time         `json:"finished_at,omitempty"`
}

type ImportListResponse struct {
	Imports []ImportJob `json:"imports"`
}

type ImportStageCleanupEstimate struct {
	Imports int   `json:"imports"`
	Files   int   `json:"files"`
	Bytes   int64 `json:"bytes"`
}

type ImportStageCleanupRunResult struct {
	ImportsCleaned      int      `json:"imports_cleaned"`
	FilesCleaned        int      `json:"files_cleaned"`
	BytesCleaned        int64    `json:"bytes_cleaned"`
	ImportsMarkedFailed int      `json:"imports_marked_failed"`
	Errors              []string `json:"errors,omitempty"`
}

type SystemImportStageCleanupStatus struct {
	Enabled            bool       `json:"enabled"`
	RetentionDays      int        `json:"retention_days"`
	StaleImports       int        `json:"stale_imports"`
	StaleFiles         int        `json:"stale_files"`
	StaleBytes         int64      `json:"stale_bytes"`
	LastRun            *time.Time `json:"last_run,omitempty"`
	LastFailedAt       *time.Time `json:"last_failed_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	RecentFailures     int        `json:"recent_failures"`
	LastCleanedImports int        `json:"last_cleaned_imports"`
	LastCleanedFiles   int        `json:"last_cleaned_files"`
	LastCleanedBytes   int64      `json:"last_cleaned_bytes"`
	LastMarkedFailed   int        `json:"last_marked_failed"`
}

type SystemImportStageCleanupRunResponse struct {
	Status  string                      `json:"status"`
	Message string                      `json:"message,omitempty"`
	Result  ImportStageCleanupRunResult `json:"result"`
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
	ID                 uuid.UUID  `json:"id"`
	TeamID             uuid.UUID  `json:"team_id"`
	Action             string     `json:"action"`
	Details            string     `json:"details"`
	ActorUserID        *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorEmail         string     `json:"actor_email,omitempty"`
	ActorEmailSnapshot string     `json:"actor_email_snapshot,omitempty"`
	ActorRoleSnapshot  string     `json:"actor_role_snapshot,omitempty"`
	TargetUserID       *uuid.UUID `json:"target_user_id,omitempty"`
	TargetEmail        string     `json:"target_email,omitempty"`
	TargetType         string     `json:"target_type,omitempty"`
	TargetID           string     `json:"target_id,omitempty"`
	TargetLabel        string     `json:"target_label,omitempty"`
	Outcome            string     `json:"outcome,omitempty"`
	IPAddress          string     `json:"ip_address,omitempty"`
	IPCountryCode      string     `json:"ip_country_code,omitempty"`
	UserAgent          string     `json:"user_agent,omitempty"`
	RequestID          string     `json:"request_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
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

// Instance audit log types
type InstanceAuditEntry struct {
	ID                 uuid.UUID  `json:"id"`
	CreatedAt          time.Time  `json:"created_at"`
	ActorID            *uuid.UUID `json:"actor_id,omitempty"`
	TeamID             *uuid.UUID `json:"team_id,omitempty"`
	TargetUserID       *uuid.UUID `json:"target_user_id,omitempty"`
	ActorEmailSnapshot string     `json:"actor_email_snapshot"`
	ActorRoleSnapshot  string     `json:"actor_role_snapshot"`
	Action             string     `json:"action"`
	TargetType         string     `json:"target_type,omitempty"`
	TargetID           string     `json:"target_id,omitempty"`
	TargetLabel        string     `json:"target_label,omitempty"`
	Outcome            string     `json:"outcome"`
	IPAddress          string     `json:"ip_address,omitempty"`
	IPCountryCode      string     `json:"ip_country_code,omitempty"`
	UserAgent          string     `json:"user_agent,omitempty"`
	RequestID          string     `json:"request_id,omitempty"`
	Details            string     `json:"details,omitempty"`
}

type InstanceAuditListResponse struct {
	Entries []InstanceAuditEntry `json:"entries"`
	Total   int                  `json:"total"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
	HasMore bool                 `json:"has_more"`
}

// System health types
type SystemFeatureStatus struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Detail  string `json:"detail,omitempty"`
}

type SystemInfo struct {
	Version         string                `json:"version"`
	RuntimeMode     string                `json:"runtime_mode"`
	Uptime          string                `json:"uptime"`
	PublicURL       string                `json:"public_url"`
	EnabledFeatures []SystemFeatureStatus `json:"enabled_features"`
	ConfigFlags     map[string]any        `json:"config_flags"`
}

type SystemHealth struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Workers  string `json:"workers"`
	IsLeader bool   `json:"is_leader"`
}

type SystemSearchConsoleStatus struct {
	Status              string     `json:"status"`
	CredentialsStatus   string     `json:"credentials_status"`
	WorkerStatus        string     `json:"worker_status"`
	SyncStatus          string     `json:"sync_status"`
	ConnectedTeams      int        `json:"connected_teams"`
	MappedSites         int        `json:"mapped_sites"`
	PendingSyncs        int        `json:"pending_syncs"`
	RunningSyncs        int        `json:"running_syncs"`
	FailedSyncs         int        `json:"failed_syncs"`
	NeedsAttentionSyncs int        `json:"needs_attention_syncs"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastAttemptAt       *time.Time `json:"last_attempt_at,omitempty"`
	NextRetryAt         *time.Time `json:"next_retry_at,omitempty"`
}

type TenantDBInfo struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Name     string    `json:"name"`
	Bytes    int64     `json:"bytes"`
	Path     string    `json:"path"`
}

type SystemStorage struct {
	SharedDBPath  string         `json:"shared_db_path"`
	SharedDBBytes int64          `json:"shared_db_bytes"`
	DataPath      string         `json:"data_path"`
	TenantDBCount int            `json:"tenant_db_count"`
	TenantDBs     []TenantDBInfo `json:"tenant_dbs,omitempty"`
	SpamCachePath string         `json:"spam_cache_path"`
	BackupPath    string         `json:"backup_path"`
	DiskAvailable int64          `json:"disk_available_bytes"`
	DiskTotal     int64          `json:"disk_total_bytes"`
}

type SystemIngestStats struct {
	RecentHits       int     `json:"recent_hits"`
	RecentEvents     int     `json:"recent_events"`
	RecentRejections int     `json:"recent_rejections"`
	RecentSpam       int     `json:"recent_spam"`
	HitsPerSecond    float64 `json:"hits_per_second"`
}

type SystemActivationRow struct {
	TeamID         uuid.UUID      `json:"team_id"`
	TeamName       string         `json:"team_name"`
	OwnerEmail     string         `json:"owner_email"`
	PlanCode       string         `json:"plan_code,omitempty"`
	PlanName       string         `json:"plan_name,omitempty"`
	CloudRegion    string         `json:"cloud_region,omitempty"`
	SiteID         uuid.UUID      `json:"site_id"`
	SiteDomain     string         `json:"site_domain"`
	SitesCount     int            `json:"sites_count"`
	ActiveSites    int            `json:"active_sites"`
	Status         TrackingStatus `json:"status"`
	FirstHitAt     *time.Time     `json:"first_hit_at,omitempty"`
	LastHitAt      *time.Time     `json:"last_hit_at,omitempty"`
	LastEventAt    *time.Time     `json:"last_event_at,omitempty"`
	LastEventName  string         `json:"last_event_name,omitempty"`
	HitsLast24h    int            `json:"hits_last_24h"`
	HitsLast7d     int            `json:"hits_last_7d"`
	EventsLast7d   int            `json:"events_last_7d"`
	TrackerSource  string         `json:"tracker_source,omitempty"`
	TrackerVersion string         `json:"tracker_version,omitempty"`
}

type SystemActivationResponse struct {
	Rows    []SystemActivationRow `json:"rows"`
	Total   int                   `json:"total"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

type SystemBackupStatus struct {
	Enabled        bool       `json:"enabled"`
	ConfigPath     string     `json:"config_path"`
	IntervalMin    int        `json:"interval_min"`
	Retention      int        `json:"retention"`
	LastBackup     *time.Time `json:"last_backup,omitempty"`
	NextBackup     *time.Time `json:"next_backup,omitempty"`
	LastFailedAt   *time.Time `json:"last_failed_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	RecentFailures int        `json:"recent_failures"`
}

type SystemSpamStatus struct {
	DBPath      string     `json:"db_path"`
	LastRefresh *time.Time `json:"last_refresh,omitempty"`
	RuleCount   int        `json:"rule_count"`
	AutoUpdate  bool       `json:"auto_update"`
	LastError   string     `json:"last_error,omitempty"`
}

type SystemCacheEntry struct {
	Size    int    `json:"size"`
	MaxSize int    `json:"max_size"`
	TTL     string `json:"ttl"`
}

type SystemCacheStatus struct {
	PermissionsCache SystemCacheEntry `json:"permissions_cache"`
	APIClientCache   SystemCacheEntry `json:"api_client_cache"`
	RateLimiterCache SystemCacheEntry `json:"rate_limiter_cache"`
	Status           string           `json:"status"`
}

type SystemMailStatus struct {
	Configured  bool       `json:"configured"`
	Driver      string     `json:"driver"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Encryption  string     `json:"encryption"`
	FromAddress string     `json:"from_address"`
	FromName    string     `json:"from_name"`
	Username    string     `json:"username"`
	PasswordSet bool       `json:"password_set"`
	LastTestAt  *time.Time `json:"last_test_at,omitempty"`
	LastTestOK  *bool      `json:"last_test_ok,omitempty"`
}

type SystemActionResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

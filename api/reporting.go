package api

import (
	"time"

	"github.com/google/uuid"
)

type ReportScope string

const (
	ReportScopePersonal ReportScope = "personal"
	ReportScopeTeam     ReportScope = "team"
)

type ReportPreset string

const (
	ReportPresetSiteSummary      ReportPreset = "site_summary"
	ReportPresetPortfolioDigest  ReportPreset = "portfolio_digest"
	ReportPresetOpportunityBrief ReportPreset = "opportunity_brief"
)

type ReportStatus string

const (
	ReportStatusDraft  ReportStatus = "draft"
	ReportStatusActive ReportStatus = "active"
	ReportStatusPaused ReportStatus = "paused"
)

type ReportSiteMode string

const (
	ReportSiteModeSelected      ReportSiteMode = "selected"
	ReportSiteModeAllAccessible ReportSiteMode = "all_accessible"
)

type ReportSchedule struct {
	Frequency  ReportFrequency `json:"frequency"`
	Timezone   string          `json:"timezone"`
	LocalTime  string          `json:"local_time"`
	WeeklyDay  *int            `json:"weekly_day,omitempty"`
	MonthlyDay *int            `json:"monthly_day,omitempty"`
}

type ReportSite struct {
	ID     uuid.UUID `json:"id"`
	Domain string    `json:"domain"`
}

type ReportRecipientKind string

const (
	ReportRecipientKindMember   ReportRecipientKind = "member"
	ReportRecipientKindExternal ReportRecipientKind = "external"
)

type ReportRecipientStatus string

const (
	ReportRecipientStatusPending   ReportRecipientStatus = "pending_confirmation"
	ReportRecipientStatusConfirmed ReportRecipientStatus = "confirmed"
	ReportRecipientStatusOptedOut  ReportRecipientStatus = "opted_out"
)

type ReportRecipient struct {
	ID                    uuid.UUID             `json:"id"`
	Kind                  ReportRecipientKind   `json:"kind"`
	UserID                *uuid.UUID            `json:"user_id,omitempty"`
	Email                 string                `json:"email"`
	Status                ReportRecipientStatus `json:"status"`
	ConfirmedAt           *time.Time            `json:"confirmed_at,omitempty"`
	ConfirmationExpiresAt *time.Time            `json:"confirmation_expires_at,omitempty"`
	InvitationState       string                `json:"invitation_state,omitempty"`
	OptedOutAt            *time.Time            `json:"opted_out_at,omitempty"`
}

type ReportLastOutcome struct {
	RunID       uuid.UUID  `json:"run_id"`
	Status      string     `json:"status"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type ReportDefinition struct {
	ID             uuid.UUID          `json:"id"`
	TenantID       *uuid.UUID         `json:"tenant_id,omitempty"`
	OwnerUserID    *uuid.UUID         `json:"owner_user_id,omitempty"`
	CreatedBy      *uuid.UUID         `json:"created_by,omitempty"`
	Name           string             `json:"name"`
	Scope          ReportScope        `json:"scope"`
	Preset         ReportPreset       `json:"preset"`
	SiteMode       ReportSiteMode     `json:"site_mode"`
	Sites          []ReportSite       `json:"sites"`
	Recipients     []ReportRecipient  `json:"recipients"`
	Schedule       ReportSchedule     `json:"schedule"`
	Status         ReportStatus       `json:"status"`
	ConsentVersion int                `json:"consent_version"`
	NextRunAt      *time.Time         `json:"next_run_at,omitempty"`
	LastOutcome    *ReportLastOutcome `json:"last_outcome,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

type CreateReportRequest struct {
	Name                    string         `json:"name"`
	Scope                   ReportScope    `json:"scope"`
	TenantID                *uuid.UUID     `json:"tenant_id,omitempty"`
	Preset                  ReportPreset   `json:"preset"`
	SiteMode                ReportSiteMode `json:"site_mode"`
	SiteIDs                 []uuid.UUID    `json:"site_ids"`
	RecipientUserIDs        []uuid.UUID    `json:"recipient_user_ids"`
	ExternalRecipientEmails []string       `json:"external_recipient_emails"`
	Schedule                ReportSchedule `json:"schedule"`
	Status                  ReportStatus   `json:"status"`
}

type UpdateReportRequest struct {
	Name                    *string         `json:"name,omitempty"`
	Preset                  *ReportPreset   `json:"preset,omitempty"`
	SiteMode                *ReportSiteMode `json:"site_mode,omitempty"`
	SiteIDs                 *[]uuid.UUID    `json:"site_ids,omitempty"`
	RecipientUserIDs        *[]uuid.UUID    `json:"recipient_user_ids,omitempty"`
	ExternalRecipientEmails *[]string       `json:"external_recipient_emails,omitempty"`
	Schedule                *ReportSchedule `json:"schedule,omitempty"`
	Status                  *ReportStatus   `json:"status,omitempty"`
}

type ReportPreviewRequest struct {
	Definition CreateReportRequest `json:"definition"`
	ReportID   *uuid.UUID          `json:"report_id,omitempty"`
}

type ReportRecipientConfirmationRequest struct {
	Action string `json:"action"`
}

type ReportPreview struct {
	Subject               string         `json:"subject"`
	Preset                ReportPreset   `json:"preset"`
	Schedule              ReportSchedule `json:"schedule"`
	SiteCount             int            `json:"site_count"`
	RecipientCount        int            `json:"recipient_count"`
	PendingRecipientCount int            `json:"pending_recipient_count"`
	PeriodStart           time.Time      `json:"period_start"`
	PeriodEnd             time.Time      `json:"period_end"`
	Suppressed            bool           `json:"suppressed"`
}

type ReportDelivery struct {
	ID              uuid.UUID           `json:"id"`
	RecipientID     uuid.UUID           `json:"recipient_id"`
	RecipientKind   ReportRecipientKind `json:"recipient_kind"`
	RecipientUserID *uuid.UUID          `json:"recipient_user_id,omitempty"`
	RecipientEmail  string              `json:"recipient_email,omitempty"`
	Status          string              `json:"status"`
	AttemptCount    int                 `json:"attempt_count"`
	NextAttemptAt   *time.Time          `json:"next_attempt_at,omitempty"`
	SafeErrorCode   string              `json:"safe_error_code,omitempty"`
	SMTPAcceptedAt  *time.Time          `json:"smtp_accepted_at,omitempty"`
}

type ReportRecipientConfirmation struct {
	ReportName string         `json:"report_name"`
	TeamName   string         `json:"team_name"`
	Preset     ReportPreset   `json:"preset"`
	Schedule   ReportSchedule `json:"schedule"`
	Sites      []ReportSite   `json:"sites"`
	ExpiresAt  time.Time      `json:"expires_at"`
}

type ReportRun struct {
	ID            uuid.UUID        `json:"id"`
	ReportID      uuid.UUID        `json:"report_id"`
	ScheduledFor  time.Time        `json:"scheduled_for"`
	PeriodStart   time.Time        `json:"period_start"`
	PeriodEnd     time.Time        `json:"period_end"`
	Status        string           `json:"status"`
	SafeErrorCode string           `json:"safe_error_code,omitempty"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
	Deliveries    []ReportDelivery `json:"deliveries"`
}

type ReportTestSendResponse struct {
	Status    string    `json:"status"`
	MessageID string    `json:"message_id"`
	SentAt    time.Time `json:"sent_at"`
}

type MailDeliveryStatus struct {
	Available bool   `json:"available"`
	Status    string `json:"status"`
}

package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/server/shared"
	"hitkeep/internal/sso"
)

type ssoAuditFlow string

const (
	ssoAuditFlowAvailability ssoAuditFlow = "availability"
	ssoAuditFlowStart        ssoAuditFlow = "start"
	ssoAuditFlowCallback     ssoAuditFlow = "callback"
)

type ssoAuditReason string

const (
	ssoReasonAvailabilityEnabled     ssoAuditReason = "availability_enabled"
	ssoReasonAvailabilityUnavailable ssoAuditReason = "availability_unavailable"
	ssoReasonProviderUnresolved      ssoAuditReason = "provider_unresolved"
	ssoReasonEntitlementDenied       ssoAuditReason = "entitlement_denied"
	ssoReasonAccessDenied            ssoAuditReason = "access_denied"
	ssoReasonCapacityDenied          ssoAuditReason = "capacity_denied"
	ssoReasonProviderUnavailable     ssoAuditReason = "provider_unavailable"
	ssoReasonProviderDenied          ssoAuditReason = "provider_denied"
	ssoReasonAuthorizationMissing    ssoAuditReason = "authorization_code_missing"
	ssoReasonConfigurationChanged    ssoAuditReason = "configuration_changed"
	ssoReasonEmailNotAllowed         ssoAuditReason = "email_not_allowed"
	ssoReasonIdentityLinkFailed      ssoAuditReason = "identity_link_failed"
	ssoReasonMembershipFailed        ssoAuditReason = "membership_failed"
	ssoReasonSessionFailed           ssoAuditReason = "session_failed"
	ssoReasonTokenExchangeFailed     ssoAuditReason = "token_exchange_failed"
	ssoReasonIDTokenMissing          ssoAuditReason = "id_token_missing"
	ssoReasonIDTokenInvalid          ssoAuditReason = "id_token_invalid"
	ssoReasonEmailUnverified         ssoAuditReason = "email_unverified"
	ssoReasonInternalError           ssoAuditReason = "internal_error"
	ssoReasonLoginSucceeded          ssoAuditReason = "login_succeeded"
)

type ssoAuditMetadata struct {
	Flow             ssoAuditFlow   `json:"flow"`
	Reason           ssoAuditReason `json:"reason"`
	EmailDomain      string         `json:"email_domain,omitempty"`
	AccessMode       string         `json:"access_mode,omitempty"`
	ConfiguredTeamID string         `json:"configured_team_id,omitempty"`
}

func ssoAccessModeName(mode ssoAccessMode) string {
	switch mode {
	case ssoAccessExistingMember:
		return "existing_member"
	case ssoAccessInvitation:
		return "invitation"
	case ssoAccessAutoProvision:
		return "auto_provision"
	default:
		return ""
	}
}

func (h *handler) appendSSOAudit(r *http.Request, action, outcome string, teamID, targetUserID uuid.UUID, email string, flow ssoAuditFlow, reason ssoAuditReason, mode ssoAccessMode, details string) {
	domain := ""
	if _, normalizedDomain, err := sso.NormalizeEmail(email); err == nil {
		domain = normalizedDomain
	}
	configuredTeamID := ""
	if teamID != uuid.Nil {
		configuredTeamID = teamID.String()
	}
	targetID := ""
	if targetUserID != uuid.Nil {
		targetID = targetUserID.String()
	}
	metadata := marshalSSOAuditMetadata(ssoAuditMetadata{
		Flow:             flow,
		Reason:           reason,
		EmailDomain:      domain,
		AccessMode:       ssoAccessModeName(mode),
		ConfiguredTeamID: configuredTeamID,
	})

	slog.Info("SSO outcome",
		"action", action,
		"outcome", outcome,
		"flow", flow,
		"reason", reason,
		"team_id", teamID,
		"email_domain", domain,
		"access_mode", ssoAccessModeName(mode),
		"request_id", requestID(r),
	)
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		TeamID:       teamID,
		TargetUserID: targetUserID,
		Action:       action,
		TargetType:   "user",
		TargetID:     targetID,
		TargetLabel:  strings.ToLower(strings.TrimSpace(email)),
		Outcome:      outcome,
		Details:      details,
		MetadataJSON: metadata,
	})
}

func (h *handler) appendSSOAuditForUserTeams(r *http.Request, userID uuid.UUID, action, outcome, email string, configuredTeamID uuid.UUID, flow ssoAuditFlow, reason ssoAuditReason, mode ssoAccessMode, details string) {
	domain := ""
	if _, normalizedDomain, err := sso.NormalizeEmail(email); err == nil {
		domain = normalizedDomain
	}
	configuredTeamIDValue := ""
	if configuredTeamID != uuid.Nil {
		configuredTeamIDValue = configuredTeamID.String()
	}
	metadata := marshalSSOAuditMetadata(ssoAuditMetadata{
		Flow:             flow,
		Reason:           reason,
		EmailDomain:      domain,
		AccessMode:       ssoAccessModeName(mode),
		ConfiguredTeamID: configuredTeamIDValue,
	})
	slog.Info("SSO outcome",
		"action", action,
		"outcome", outcome,
		"flow", flow,
		"reason", reason,
		"team_id", configuredTeamID,
		"email_domain", domain,
		"access_mode", ssoAccessModeName(mode),
		"request_id", requestID(r),
	)
	h.ctx.AppendAuditEventForUserTeams(r.Context(), r, userID, shared.AuditEvent{
		TargetUserID: userID,
		Action:       action,
		TargetType:   "user",
		TargetID:     userID.String(),
		TargetLabel:  strings.ToLower(strings.TrimSpace(email)),
		Outcome:      outcome,
		Details:      details,
		MetadataJSON: metadata,
	})
}

func marshalSSOAuditMetadata(value ssoAuditMetadata) string {
	metadata, err := json.Marshal(value)
	if err != nil {
		slog.Error("Failed to encode SSO audit metadata", "error", err)
		return "{}"
	}
	return string(metadata)
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-Request-Id"))
}

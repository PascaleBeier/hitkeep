package shared

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

type AuditEvent struct {
	ActorID       uuid.UUID
	ActorEmail    string
	ActorRole     string
	TeamID        uuid.UUID
	TargetUserID  uuid.UUID
	Action        string
	TargetType    string
	TargetID      string
	TargetLabel   string
	Outcome       string
	IPAddress     string
	IPCountryCode string
	UserAgent     string
	RequestID     string
	Details       string
	MetadataJSON  string
}

func (c *Context) AppendAuditEvent(ctx context.Context, r *http.Request, event AuditEvent) {
	if err := c.AppendAuditEventChecked(ctx, r, event); err != nil {
		slog.Error("Failed to append audit entry", "error", err, "action", event.Action)
	}
}

func (c *Context) AppendAuditEventChecked(ctx context.Context, r *http.Request, event AuditEvent) error {
	params, err := c.BuildAuditEntryParams(ctx, r, event)
	if err != nil {
		return err
	}
	return c.Store.AppendAuditEntry(ctx, params)
}

func (c *Context) AppendAuditEventForUserTeams(ctx context.Context, r *http.Request, userID uuid.UUID, event AuditEvent) {
	if c == nil || c.Store == nil {
		return
	}
	if userID == uuid.Nil {
		c.AppendAuditEvent(ctx, r, event)
		return
	}

	teams, _, err := c.Store.ListUserTeams(ctx, userID)
	if err != nil {
		slog.Error("Failed to list user teams for audit", "error", err, "user_id", userID, "action", event.Action)
		c.AppendAuditEvent(ctx, r, event)
		return
	}
	if len(teams) == 0 {
		c.AppendAuditEvent(ctx, r, event)
		return
	}

	for _, team := range teams {
		teamEvent := event
		if teamEvent.TargetUserID == uuid.Nil {
			teamEvent.TargetUserID = userID
		}
		teamEvent.TeamID = team.ID
		if strings.TrimSpace(teamEvent.ActorRole) == "" {
			teamEvent.ActorRole = team.Role
		}
		c.AppendAuditEvent(ctx, r, teamEvent)
	}
}

func (c *Context) BuildAuditEntryParams(ctx context.Context, r *http.Request, event AuditEvent) (database.AuditEntryParams, error) {
	if c == nil || c.Store == nil {
		return database.AuditEntryParams{}, fmt.Errorf("audit store is not configured")
	}
	if ctx == nil {
		if r != nil {
			ctx = r.Context()
		} else {
			ctx = context.Background()
		}
	}

	if event.ActorID == uuid.Nil && r != nil {
		event.ActorID = GetUserIDFromContext(r)
	}

	if r != nil {
		event = c.withAuditRequestMetadata(r, event)
	}
	event = c.withAuditActorSnapshot(ctx, r, event)

	return database.AuditEntryParams{
		ActorID:       event.ActorID,
		ActorEmail:    event.ActorEmail,
		ActorRole:     event.ActorRole,
		TeamID:        event.TeamID,
		TargetUserID:  event.TargetUserID,
		Action:        event.Action,
		TargetType:    event.TargetType,
		TargetID:      event.TargetID,
		TargetLabel:   event.TargetLabel,
		Outcome:       event.Outcome,
		IPAddress:     event.IPAddress,
		IPCountryCode: event.IPCountryCode,
		UserAgent:     event.UserAgent,
		RequestID:     event.RequestID,
		Details:       event.Details,
		MetadataJSON:  event.MetadataJSON,
	}, nil
}

func (c *Context) withAuditRequestMetadata(r *http.Request, event AuditEvent) AuditEvent {
	var trustedProxies []netip.Prefix
	if c.Config != nil {
		trustedProxies = c.Config.GetTrustedProxyNetworks()
	}
	if strings.TrimSpace(event.IPAddress) == "" {
		event.IPAddress = GetRealIP(r, trustedProxies)
	}
	if strings.TrimSpace(event.IPCountryCode) == "" {
		event.IPCountryCode = CountryCodeFromRequest(r, trustedProxies)
	}
	if strings.TrimSpace(event.UserAgent) == "" {
		event.UserAgent = r.UserAgent()
	}
	if strings.TrimSpace(event.RequestID) == "" {
		event.RequestID = r.Header.Get("X-Request-Id")
	}
	return event
}

func (c *Context) withAuditActorSnapshot(ctx context.Context, r *http.Request, event AuditEvent) AuditEvent {
	if event.ActorID == uuid.Nil {
		return event
	}

	if strings.TrimSpace(event.ActorEmail) == "" {
		if user, err := c.Store.GetUserByID(ctx, event.ActorID); err == nil && user != nil {
			event.ActorEmail = user.Email
		}
	}

	if strings.TrimSpace(event.ActorRole) == "" && event.TeamID != uuid.Nil {
		if role, err := c.Store.GetTenantRole(ctx, event.TeamID, event.ActorID); err == nil {
			event.ActorRole = role
		}
	}
	if strings.TrimSpace(event.ActorRole) == "" && r != nil {
		if permCtx, ok := r.Context().Value(PermissionKey).(PermissionContext); ok {
			event.ActorRole = string(permCtx.InstanceRole)
		}
	}
	if strings.TrimSpace(event.ActorRole) == "" {
		if role, err := c.Store.GetInstanceRole(ctx, event.ActorID); err == nil {
			event.ActorRole = string(role)
		}
	}

	return event
}

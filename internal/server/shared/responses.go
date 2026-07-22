package shared

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
)

func (c *Context) AuthSessionResponse(session AuthSessionContext) api.AuthSession {
	duration := c.Config.AuthSessionDuration()
	warning := c.Config.AuthSessionWarningDuration()
	return api.AuthSession{
		ExpiresAt:              session.ExpiresAt.UTC(),
		IssuedAt:               session.IssuedAt.UTC(),
		DurationSeconds:        int(duration.Seconds()),
		WarningSeconds:         int(warning.Seconds()),
		Extendable:             true,
		TimingAdjustable:       true,
		RememberMeDurationDays: int(c.Config.AuthRememberMeDuration().Hours() / 24),
	}
}

func (c *Context) AuthSessionResponseForRequest(r *http.Request, userID uuid.UUID, session AuthSessionContext) api.AuthSession {
	resp := c.AuthSessionResponse(session)
	if remembered, rememberExpiresAt := c.RememberedSession(r, userID); remembered {
		resp.Remembered = true
		resp.RememberExpiresAt = &rememberExpiresAt
	}
	return resp
}

func (c *Context) RememberedSession(r *http.Request, userID uuid.UUID) (bool, time.Time) {
	if c.Store == nil || userID == uuid.Nil {
		return false, time.Time{}
	}
	cookie, err := r.Cookie(auth.RememberMeCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false, time.Time{}
	}
	rememberedUserID, expiresAt, err := c.Store.ValidateRememberMeSession(r.Context(), cookie.Value)
	if err != nil || rememberedUserID != userID || expiresAt.IsZero() {
		return false, time.Time{}
	}
	return true, expiresAt.UTC()
}

func (c *Context) SystemStatusResponse(ctx context.Context) (api.SystemStatus, error) {
	userCount, err := c.Store.GetUserCount(ctx)
	if err != nil {
		return api.SystemStatus{}, fmt.Errorf("get user count: %w", err)
	}

	mailStatus := "unavailable"
	if c.Mailer != nil {
		mailStatus = "available"
	}
	return api.SystemStatus{
		NeedsSetup: userCount == 0 && !c.Config.CloudHosted,
		Version:    c.Config.Version,
		Cloud:      c.CloudStatus(),
		AskAI:      c.askAIStatus(ctx),
		MailDelivery: &api.MailDeliveryStatus{
			Available: c.Mailer != nil,
			Status:    mailStatus,
		},
	}, nil
}

func (c *Context) askAIStatus(ctx context.Context) *api.AskAIStatus {
	if c == nil || c.Config == nil {
		return &api.AskAIStatus{Status: "disabled"}
	}
	return AskAIStatus(ctx, c.Config, c.Store)
}

func AskAIStatus(ctx context.Context, cfg *config.Config, store aiUsageReader) *api.AskAIStatus {
	status := &api.AskAIStatus{Status: "disabled"}
	if cfg == nil {
		return status
	}
	configured := askAIConfigured(cfg)
	status.Enabled = cfg.AIEnabled && cfg.AskAIEnabled
	status.Provider = strings.TrimSpace(cfg.AIProvider)
	status.Model = strings.TrimSpace(cfg.AIModel)
	switch {
	case !cfg.AIEnabled || !cfg.AskAIEnabled:
		status.Status = "disabled"
	case !configured:
		status.Status = "not_configured"
	default:
		status.Status = "available"
		status.Available = true
	}
	if store != nil {
		window := time.Duration(cfg.AIBudgetWindowMinutes) * time.Minute
		if window <= 0 {
			window = 24 * time.Hour
		}
		if usage, err := store.GetAIUsageSince(ctx, time.Now().UTC().Add(-window)); err == nil {
			status.BudgetExhausted = (cfg.AIRequestLimit > 0 && usage.Requests >= cfg.AIRequestLimit) ||
				(cfg.AITokenLimit > 0 && usage.Tokens >= cfg.AITokenLimit)
			if status.BudgetExhausted && status.Available {
				status.Status = "budget_exhausted"
				status.Available = false
			}
		}
	}
	return status
}

type aiUsageReader interface {
	GetAIUsageSince(context.Context, time.Time) (database.AIUsageSummary, error)
}

func askAIConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return hitai.ValidateConfig(hitai.Config{
		Provider: strings.TrimSpace(cfg.AIProvider),
		Model:    strings.TrimSpace(cfg.AIModel),
		BaseURL:  strings.TrimSpace(cfg.AIBaseURL),
		Region:   strings.TrimSpace(cfg.AIRegion),
		APIKey:   strings.TrimSpace(cfg.AIAPIKey),
	}) == nil
}

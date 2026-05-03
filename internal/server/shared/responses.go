package shared

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
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

	return api.SystemStatus{
		NeedsSetup: userCount == 0 && !c.Config.CloudHosted,
		Version:    c.Config.Version,
		Cloud:      c.CloudStatus(),
	}, nil
}

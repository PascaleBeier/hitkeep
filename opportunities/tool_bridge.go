package opportunities

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/analyticstools"
	"hitkeep/auth"
	"hitkeep/database"
)

type ToolBridgeConfig struct {
	Shared                *database.Store
	Analytics             *database.Store
	TeamID                uuid.UUID
	SiteID                uuid.UUID
	ActorID               uuid.UUID
	ActorType             string
	APIClientAuth         *database.APIClientAuth
	EffectiveUserID       uuid.UUID
	EffectiveInstanceRole auth.InstanceRole
	EffectiveSiteRole     auth.SiteRole
	SchedulerTeamID       uuid.UUID
	SchedulerSiteID       uuid.UUID
	From                  time.Time
	To                    time.Time
}

type ToolBridge struct {
	config ToolBridgeConfig
}

func NewToolBridge(config ToolBridgeConfig) ToolBridge {
	return ToolBridge{config: config}
}

// Tools offers the evidence tools pinned to the candidate's site and window,
// so every citation refers to data from the range the candidate measured.
func (b ToolBridge) Tools() []goaisdk.Tool {
	return analyticstools.GoAI(analyticstools.Scope{
		SiteID:    b.config.SiteID,
		Resolve:   b.resolve,
		From:      b.config.From,
		To:        b.config.To,
		LockRange: true,
	}, analyticstools.Evidence()...)
}

func (b ToolBridge) resolve(ctx context.Context, siteID uuid.UUID) (analyticstools.Site, error) {
	if err := newToolBridgeScope(b.config).authorize(ctx); err != nil {
		return analyticstools.Site{}, err
	}
	if b.config.Analytics == nil {
		return analyticstools.Site{}, errors.New("analytics store unavailable")
	}
	if siteID == uuid.Nil || siteID != b.config.SiteID {
		return analyticstools.Site{}, errors.New("site scope is required")
	}
	if b.config.From.IsZero() || !b.config.From.Before(b.config.To) {
		return analyticstools.Site{}, errors.New("valid date range is required")
	}
	return analyticstools.Site{ID: siteID, Control: b.config.Shared, Analytics: b.config.Analytics}, nil
}

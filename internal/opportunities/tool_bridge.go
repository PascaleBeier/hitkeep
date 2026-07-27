package opportunities

import (
	"context"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/internal/analyticstools"
	"hitkeep/internal/auth"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
)

type ToolBridgeConfig struct {
	Shared                *controlstore.Store
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

func (b ToolBridge) Tools() []goaisdk.Tool {
	return analyticstools.NewBridge(analyticstools.Config{
		Analytics:     b.config.Analytics,
		SiteID:        b.config.SiteID,
		From:          b.config.From,
		To:            b.config.To,
		BeforeExecute: b.authorize,
	}).Tools()
}

func (b ToolBridge) authorize(ctx context.Context) error {
	return newToolBridgeScope(b.config).authorize(ctx)
}

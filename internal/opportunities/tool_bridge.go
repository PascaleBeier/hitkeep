package opportunities

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
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

func (b ToolBridge) Tools() []goaisdk.Tool {
	return []goaisdk.Tool{
		b.tool("hitkeep_get_site_overview", "Read aggregate traffic KPIs for the scoped site.", b.siteOverview),
		b.tool("hitkeep_get_ecommerce", "Read aggregate ecommerce summary for the scoped site.", b.ecommerce),
		b.tool("hitkeep_get_ai_visibility", "Read aggregate AI crawler visibility summary for the scoped site.", b.aiVisibility),
	}
}

func (b ToolBridge) tool(name, description string, execute func(context.Context) (any, error)) goaisdk.Tool {
	return goaisdk.Tool{
		Name:        name,
		Description: description,
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (string, error) {
			if err := b.authorize(ctx); err != nil {
				return "", err
			}
			value, err := execute(ctx)
			if err != nil {
				return "", err
			}
			return safeJSON(value)
		},
	}
}

func (b ToolBridge) authorize(ctx context.Context) error {
	return newToolBridgeScope(b.config).authorize(ctx)
}

func (b ToolBridge) siteOverview(ctx context.Context) (any, error) {
	return b.config.Analytics.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: b.config.SiteID,
		Start:  b.config.From,
		End:    b.config.To,
	})
}

func (b ToolBridge) ecommerce(ctx context.Context) (any, error) {
	return b.config.Analytics.GetEcommerceSummary(ctx, api.EcommerceParams{
		SiteID: b.config.SiteID,
		Start:  b.config.From,
		End:    b.config.To,
	})
}

func (b ToolBridge) aiVisibility(ctx context.Context) (any, error) {
	return b.config.Analytics.GetAIFetchOverview(ctx, api.AIFetchQueryParams{
		SiteID: b.config.SiteID,
		Start:  b.config.From,
		End:    b.config.To,
	})
}

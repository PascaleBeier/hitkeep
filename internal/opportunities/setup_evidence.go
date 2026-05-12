package opportunities

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

const (
	defaultSetupEvidenceGoalLimit       = 10
	defaultSetupEvidenceFunnelLimit     = 10
	defaultSetupEvidenceFunnelStepLimit = 8
	defaultSetupEvidenceEventLimit      = 20
	defaultSetupEvidenceTopPageLimit    = 10
	defaultSetupEvidenceStringLimit     = 160
)

type setupEvidenceSnapshotInput struct {
	SharedStore    *database.Store
	AnalyticsStore *database.Store
	SiteID         uuid.UUID
	From           time.Time
	To             time.Time
}

type SetupEvidenceSnapshot struct {
	SiteID     uuid.UUID              `json:"site_id"`
	From       time.Time              `json:"from"`
	To         time.Time              `json:"to"`
	Goals      []SetupGoalEvidence    `json:"goals"`
	Funnels    []SetupFunnelEvidence  `json:"funnels"`
	EventNames []string               `json:"event_names"`
	Events     []SetupEventEvidence   `json:"events"`
	TopPages   []SetupTopPageEvidence `json:"top_pages"`
	Ecommerce  SetupEcommerceEvidence `json:"ecommerce"`
	SetupState SetupStateEvidence     `json:"setup_state"`
}

type SetupGoalEvidence struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type SetupFunnelEvidence struct {
	Name  string                    `json:"name"`
	Steps []SetupFunnelStepEvidence `json:"steps"`
}

type SetupFunnelStepEvidence struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type SetupEventEvidence struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type SetupTopPageEvidence struct {
	Path      string `json:"path"`
	Pageviews int    `json:"pageviews"`
}

type SetupEcommerceEvidence struct {
	Revenue                float64 `json:"revenue"`
	Orders                 int     `json:"orders"`
	AverageOrderValue      float64 `json:"average_order_value"`
	CheckoutStarts         int     `json:"checkout_starts"`
	CheckoutConversionRate float64 `json:"checkout_conversion_rate"`
	Currency               string  `json:"currency,omitempty"`
}

type SetupStateEvidence struct {
	HasGoals           bool `json:"has_goals"`
	HasFunnels         bool `json:"has_funnels"`
	HasEvents          bool `json:"has_events"`
	HasConversionEvent bool `json:"has_conversion_event"`
	HasCheckoutSignal  bool `json:"has_checkout_signal"`
	HasOrderSignal     bool `json:"has_order_signal"`
	HasTraffic         bool `json:"has_traffic"`
}

func buildSetupEvidenceSnapshot(ctx context.Context, input setupEvidenceSnapshotInput) (*SetupEvidenceSnapshot, error) {
	if input.SharedStore == nil {
		return nil, fmt.Errorf("shared store is required")
	}
	if input.AnalyticsStore == nil {
		return nil, fmt.Errorf("analytics store is required")
	}
	if input.SiteID == uuid.Nil {
		return nil, fmt.Errorf("site id is required")
	}

	goals, err := input.SharedStore.GetGoals(ctx, input.SiteID)
	if err != nil {
		return nil, fmt.Errorf("load setup goals: %w", err)
	}
	funnels, err := input.SharedStore.GetFunnels(ctx, input.SiteID)
	if err != nil {
		return nil, fmt.Errorf("load setup funnels: %w", err)
	}
	stats, err := input.AnalyticsStore.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID: input.SiteID,
		Start:  input.From,
		End:    input.To,
	})
	if err != nil {
		return nil, fmt.Errorf("load setup site stats: %w", err)
	}
	ecommerce, err := input.AnalyticsStore.GetEcommerceSummary(ctx, api.EcommerceParams{
		SiteID: input.SiteID,
		Start:  input.From,
		End:    input.To,
	})
	if err != nil {
		return nil, fmt.Errorf("load setup ecommerce summary: %w", err)
	}
	eventNames, err := input.AnalyticsStore.GetEventNames(ctx, api.EventNamesParams{
		SiteID: input.SiteID,
		Start:  input.From,
		End:    input.To,
	})
	if err != nil {
		return nil, fmt.Errorf("load setup event names: %w", err)
	}
	eventCounts, err := input.AnalyticsStore.GetEventCounts(ctx, api.EventNamesParams{
		SiteID: input.SiteID,
		Start:  input.From,
		End:    input.To,
	})
	if err != nil {
		return nil, fmt.Errorf("load setup event counts: %w", err)
	}

	snapshot := &SetupEvidenceSnapshot{
		SiteID:     input.SiteID,
		From:       input.From,
		To:         input.To,
		Goals:      setupGoalEvidence(goals),
		Funnels:    setupFunnelEvidence(funnels),
		EventNames: setupEventNames(eventNames),
		Events:     setupEventEvidence(eventCounts),
		TopPages:   setupTopPageEvidence(stats.TopPages),
		Ecommerce:  setupEcommerceEvidence(ecommerce),
	}
	snapshot.SetupState = setupStateEvidence(snapshot, stats)
	return snapshot, nil
}

func setupGoalEvidence(goals []api.Goal) []SetupGoalEvidence {
	limit := min(len(goals), defaultSetupEvidenceGoalLimit)
	out := make([]SetupGoalEvidence, 0, limit)
	for i := range limit {
		goal := goals[i]
		out = append(out, SetupGoalEvidence{
			Name:  truncateSetupEvidenceString(goal.Name),
			Type:  truncateSetupEvidenceString(goal.Type),
			Value: truncateSetupEvidenceString(goal.Value),
		})
	}
	return out
}

func setupFunnelEvidence(funnels []api.Funnel) []SetupFunnelEvidence {
	limit := min(len(funnels), defaultSetupEvidenceFunnelLimit)
	out := make([]SetupFunnelEvidence, 0, limit)
	for i := range limit {
		funnel := funnels[i]
		stepsLimit := min(len(funnel.Steps), defaultSetupEvidenceFunnelStepLimit)
		steps := make([]SetupFunnelStepEvidence, 0, stepsLimit)
		for stepIndex := range stepsLimit {
			step := funnel.Steps[stepIndex]
			steps = append(steps, SetupFunnelStepEvidence{
				Type:  truncateSetupEvidenceString(step.Type),
				Value: truncateSetupEvidenceString(step.Value),
			})
		}
		out = append(out, SetupFunnelEvidence{
			Name:  truncateSetupEvidenceString(funnel.Name),
			Steps: steps,
		})
	}
	return out
}

func setupEventNames(eventNames []string) []string {
	limit := min(len(eventNames), defaultSetupEvidenceEventLimit)
	out := make([]string, 0, limit)
	for i := range limit {
		name := strings.TrimSpace(eventNames[i])
		if name == "" {
			continue
		}
		out = append(out, truncateSetupEvidenceString(name))
	}
	return out
}

func setupEventEvidence(events []api.MetricStat) []SetupEventEvidence {
	limit := min(len(events), defaultSetupEvidenceEventLimit)
	out := make([]SetupEventEvidence, 0, limit)
	for i := range limit {
		event := events[i]
		name := strings.TrimSpace(event.Name)
		if name == "" {
			continue
		}
		out = append(out, SetupEventEvidence{
			Name:  truncateSetupEvidenceString(name),
			Count: event.Value,
		})
	}
	return out
}

func setupTopPageEvidence(topPages []api.MetricStat) []SetupTopPageEvidence {
	limit := min(len(topPages), defaultSetupEvidenceTopPageLimit)
	out := make([]SetupTopPageEvidence, 0, limit)
	for i := range limit {
		topPage := topPages[i]
		path := strings.TrimSpace(topPage.Name)
		if path == "" {
			continue
		}
		out = append(out, SetupTopPageEvidence{
			Path:      truncateSetupEvidenceString(path),
			Pageviews: topPage.Value,
		})
	}
	return out
}

func setupEcommerceEvidence(ecommerce *api.EcommerceSummary) SetupEcommerceEvidence {
	if ecommerce == nil {
		return SetupEcommerceEvidence{}
	}
	return SetupEcommerceEvidence{
		Revenue:                ecommerce.Revenue,
		Orders:                 ecommerce.Orders,
		AverageOrderValue:      ecommerce.AverageOrderValue,
		CheckoutStarts:         ecommerce.CheckoutStarts,
		CheckoutConversionRate: ecommerce.CheckoutConversionRate,
		Currency:               truncateSetupEvidenceString(ecommerce.Currency),
	}
}

func setupStateEvidence(snapshot *SetupEvidenceSnapshot, stats *api.SiteStats) SetupStateEvidence {
	return SetupStateEvidence{
		HasGoals:           len(snapshot.Goals) > 0,
		HasFunnels:         len(snapshot.Funnels) > 0,
		HasEvents:          len(snapshot.Events) > 0 || len(snapshot.EventNames) > 0,
		HasConversionEvent: hasKnownConversionEvent(snapshot.EventNames),
		HasCheckoutSignal:  snapshot.Ecommerce.CheckoutStarts > 0,
		HasOrderSignal:     snapshot.Ecommerce.Orders > 0,
		HasTraffic:         stats != nil && stats.TotalPageviews > 0,
	}
}

func truncateSetupEvidenceString(value string) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= defaultSetupEvidenceStringLimit {
		return value
	}
	runes := []rune(value)
	return string(runes[:defaultSetupEvidenceStringLimit])
}

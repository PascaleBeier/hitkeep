package opportunities

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

type WebVitalsEvidenceSnapshot struct {
	SiteID     uuid.UUID
	From       time.Time
	To         time.Time
	Summary    []api.WebVitalSummaryMetric
	Pages      map[api.WebVitalMetric][]api.WebVitalPageRow
	Dimensions map[api.WebVitalMetric]WebVitalsDimensionEvidence
}

type WebVitalsDimensionEvidence struct {
	TopCities    []api.WebVitalDimensionRow
	TopProviders []api.WebVitalDimensionRow
	TopASNs      []api.WebVitalDimensionRow
}

func buildWebVitalsEvidenceSnapshot(ctx context.Context, store *database.Store, siteID uuid.UUID, from, to time.Time) (*WebVitalsEvidenceSnapshot, error) {
	if err := validateWebVitalsEvidenceInput(store, siteID); err != nil {
		return nil, err
	}

	summary, err := store.GetWebVitalsSummary(ctx, api.WebVitalsParams{
		SiteID: siteID,
		Start:  from,
		End:    to,
	})
	if err != nil {
		return nil, fmt.Errorf("load web vitals summary: %w", err)
	}
	pages, err := loadWebVitalsEvidencePages(ctx, store, siteID, from, to, summary)
	if err != nil {
		return nil, err
	}
	dimensions, err := loadWebVitalsEvidenceDimensions(ctx, store, siteID, from, to, summary)
	if err != nil {
		return nil, err
	}

	return &WebVitalsEvidenceSnapshot{
		SiteID:     siteID,
		From:       from,
		To:         to,
		Summary:    append([]api.WebVitalSummaryMetric(nil), summary...),
		Pages:      pages,
		Dimensions: dimensions,
	}, nil
}

func validateWebVitalsEvidenceInput(store *database.Store, siteID uuid.UUID) error {
	if store == nil {
		return fmt.Errorf("analytics store is required")
	}
	if siteID == uuid.Nil {
		return fmt.Errorf("site id is required")
	}
	return nil
}

func loadWebVitalsEvidencePages(ctx context.Context, store *database.Store, siteID uuid.UUID, from, to time.Time, summary []api.WebVitalSummaryMetric) (map[api.WebVitalMetric][]api.WebVitalPageRow, error) {
	pages := make(map[api.WebVitalMetric][]api.WebVitalPageRow, len(summary))
	for _, metric := range summary {
		rows, err := store.GetWebVitalsPages(ctx, api.WebVitalsParams{
			SiteID: siteID,
			Start:  from,
			End:    to,
			Metric: metric.Metric,
			Limit:  5,
		})
		if err != nil {
			return nil, fmt.Errorf("load web vitals pages for %s: %w", metric.Metric, err)
		}
		pages[metric.Metric] = rows
	}
	return pages, nil
}

func loadWebVitalsEvidenceDimensions(ctx context.Context, store *database.Store, siteID uuid.UUID, from, to time.Time, summary []api.WebVitalSummaryMetric) (map[api.WebVitalMetric]WebVitalsDimensionEvidence, error) {
	dimensions := make(map[api.WebVitalMetric]WebVitalsDimensionEvidence, len(summary))
	for _, metric := range summary {
		params := api.WebVitalsParams{
			SiteID: siteID,
			Start:  from,
			End:    to,
			Metric: metric.Metric,
			Limit:  3,
		}
		cities, err := store.GetWebVitalsBreakdown(ctx, params, api.WebVitalDimensionCity)
		if err != nil {
			return nil, fmt.Errorf("load web vitals cities for %s: %w", metric.Metric, err)
		}
		providers, err := store.GetWebVitalsBreakdown(ctx, params, api.WebVitalDimensionProvider)
		if err != nil {
			return nil, fmt.Errorf("load web vitals providers for %s: %w", metric.Metric, err)
		}
		asns, err := store.GetWebVitalsBreakdown(ctx, params, api.WebVitalDimensionASN)
		if err != nil {
			return nil, fmt.Errorf("load web vitals ASNs for %s: %w", metric.Metric, err)
		}
		dimensions[metric.Metric] = WebVitalsDimensionEvidence{
			TopCities:    cities,
			TopProviders: providers,
			TopASNs:      asns,
		}
	}
	return dimensions, nil
}

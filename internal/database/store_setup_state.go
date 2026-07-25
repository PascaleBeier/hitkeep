package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// GetSiteSetupState reports whether a site has ever recorded data for the
// optional dashboard surfaces. The flags are lifetime signals, so the query
// deliberately applies no date range.
//
// Every table it reads (ai_fetches, events, web_vitals) is site-scoped analytics
// data, so this must run against the site's tenant store — not the control-plane
// store, which holds no hits.
func (s *Store) GetSiteSetupState(ctx context.Context, siteID uuid.UUID) (api.SiteSetupState, error) {
	args := make([]any, 0, 5+len(ecommerceSignalEventNames))
	args = append(args, siteID, siteID, siteID, siteID)
	for _, name := range ecommerceSignalEventNames {
		args = append(args, name)
	}
	args = append(args, siteID)

	var state api.SiteSetupState
	//nolint:gosec // Only generated bind markers are interpolated.
	err := s.QueryRowOrNil(ctx, fmt.Sprintf(`
		SELECT
			EXISTS (SELECT 1 FROM ai_fetches WHERE site_id = ?),
			EXISTS (SELECT 1 FROM events WHERE site_id = ? AND name LIKE 'assistant.%%'),
			EXISTS (SELECT 1 FROM events WHERE site_id = ?),
			EXISTS (SELECT 1 FROM events WHERE site_id = ? AND lower(trim(name)) IN (%s)),
			EXISTS (SELECT 1 FROM web_vitals WHERE site_id = ?)
	`, buildPlaceholders(len(ecommerceSignalEventNames))),
		[]any{&state.HasAIFetches, &state.HasChatbotEvents, &state.HasCustomEvents, &state.HasEcommerceEvents, &state.HasWebVitals},
		args...)
	if err != nil {
		return api.SiteSetupState{}, fmt.Errorf("query site setup state: %w", err)
	}
	return state, nil
}

package filterparams

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"hitkeep/internal/api"
)

// LegacyPair describes the older split query-parameter form for a single filter.
type LegacyPair struct {
	TypeParam          string
	ValueParam         string
	MissingMessage     string
	InvalidTypeMessage string
}

// hitFilterTypes is the canonical set of filter types the hit-backed analytics
// surfaces accept. Every other allowlist in the codebase must derive from it
// through AllowedHitFilterTypes instead of hand-copying the entries.
var hitFilterTypes = map[string]struct{}{
	"path":            {},
	"ai_bot":          {},
	"ai_bot_category": {},
	"ai_source":       {},
	"hostname":        {},
	"referrer":        {},
	"referrer_host":   {},
	"device":          {},
	"country":         {},
	"city":            {},
	"provider":        {},
	"asn":             {},
	"browser":         {},
	"language":        {},
	"utm_campaign":    {},
	"utm_content":     {},
	"utm_medium":      {},
	"utm_source":      {},
	"utm_term":        {},
	"qr_code_id":      {},
}

// AllowedHitFilterTypes returns every accepted hit filter type in sorted order.
// Callers that need a narrower allowlist (the MCP surface, for one) should derive
// it from this set and subtract explicit exclusions, so a new filter type never
// has to be added in two places.
func AllowedHitFilterTypes() []string {
	types := make([]string, 0, len(hitFilterTypes))
	for filterType := range hitFilterTypes {
		types = append(types, filterType)
	}
	slices.Sort(types)
	return types
}

// ParseHitFilters parses repeatable filter=type:value params and an optional
// legacy type/value pair into the shared API filter shape.
func ParseHitFilters(q url.Values, legacy LegacyPair) ([]api.Filter, error) {
	var filters []api.Filter

	for _, raw := range q["filter"] {
		filter, ok, err := parseRawHitFilter(raw, legacy)
		if err != nil {
			return nil, err
		}
		if ok {
			filters = append(filters, filter)
		}
	}

	if legacy.TypeParam == "" || legacy.ValueParam == "" {
		return filters, nil
	}

	filterType := strings.ToLower(strings.TrimSpace(q.Get(legacy.TypeParam)))
	filterValue := strings.TrimSpace(q.Get(legacy.ValueParam))
	if filterType == "" && filterValue == "" {
		return filters, nil
	}
	if err := validateHitFilter(filterType, filterValue, legacy); err != nil {
		return nil, err
	}

	return append(filters, api.Filter{Type: filterType, Value: filterValue}), nil
}

func parseRawHitFilter(raw string, legacy LegacyPair) (api.Filter, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return api.Filter{}, false, nil
	}

	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return api.Filter{}, false, errors.New("invalid filter format")
	}

	filterType := strings.ToLower(strings.TrimSpace(parts[0]))
	filterValue := strings.TrimSpace(parts[1])
	if err := validateHitFilter(filterType, filterValue, legacy); err != nil {
		return api.Filter{}, false, err
	}

	return api.Filter{Type: filterType, Value: filterValue}, true, nil
}

func validateHitFilter(filterType, filterValue string, legacy LegacyPair) error {
	if filterType == "" || filterValue == "" {
		return errors.New(messageOrDefault(legacy.MissingMessage, "filter type and value are required together"))
	}
	if _, ok := hitFilterTypes[filterType]; !ok {
		return errors.New(messageOrDefault(legacy.InvalidTypeMessage, "invalid filter type"))
	}
	return nil
}

func messageOrDefault(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}

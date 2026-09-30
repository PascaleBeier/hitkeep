package analyticstools

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"hitkeep/api"
	"hitkeep/server/filterparams"
)

const (
	defaultRangeDays = 30
	defaultMaxDays   = 366
	defaultLimit     = 10
	maxLimit         = 50
)

// FilterInput is one aggregate analytics filter.
type FilterInput struct {
	// Keep this list in step with FilterTypes: struct tags are compile-time
	// constants, so TestFilterInputSchemaDocumentsAllowedFilterTypes is what
	// stops the two from drifting.
	Type  string `json:"type" jsonschema:"Filter type: path, ai_bot, ai_bot_category, ai_source, hostname, referrer, referrer_host, device, country, city, provider, asn, browser, language, utm_campaign, utm_content, utm_medium, utm_source, or utm_term."`
	Value string `json:"value" jsonschema:"Filter value."`
}

// FilterSet is embedded by inputs that accept analytics filters.
type FilterSet struct {
	Filters []FilterInput `json:"filters,omitempty" jsonschema:"Optional analytics filters."`
}

func (f FilterSet) filterInputs() []FilterInput { return f.Filters }

// excludedFilterTypes names the canonical hit filter types the tools
// deliberately withhold:
//
//   - qr_code_id: a dashboard-only drill-down. The values are opaque QR code
//     UUIDs a model cannot discover through any exposed aggregate tool, so
//     accepting them would only invite failed guesses.
//
// Everything else in filterparams' canonical set is accepted, which means a new
// filter type reaches the tools automatically and removing one takes a
// deliberate edit here.
var excludedFilterTypes = map[string]struct{}{
	"qr_code_id": {},
}

// FilterTypes is the sorted tool filter allowlist, derived from the canonical
// REST set minus excludedFilterTypes.
var FilterTypes = deriveFilterTypes()

func deriveFilterTypes() []string {
	canonical := filterparams.AllowedHitFilterTypes()
	allowed := make([]string, 0, len(canonical))
	for _, filterType := range canonical {
		if _, excluded := excludedFilterTypes[filterType]; excluded {
			continue
		}
		allowed = append(allowed, filterType)
	}
	return allowed
}

func parseFilters(inputs []FilterInput) ([]api.Filter, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	filters := make([]api.Filter, 0, len(inputs))
	for _, input := range inputs {
		filterType := strings.ToLower(strings.TrimSpace(input.Type))
		filterValue := strings.TrimSpace(input.Value)
		if filterType == "" || filterValue == "" {
			return nil, errors.New("filter type and value are required together")
		}
		if !slices.Contains(FilterTypes, filterType) {
			return nil, fmt.Errorf("invalid filter type %q", filterType)
		}
		filters = append(filters, api.Filter{Type: filterType, Value: filterValue})
	}
	return filters, nil
}

func parseRange(from, to string, maxDays int) (time.Time, time.Time, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	end := time.Now().UTC()
	if to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid to timestamp, expected RFC3339")
		}
		end = parsed
	}
	start := end.AddDate(0, 0, -defaultRangeDays)
	if from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid from timestamp, expected RFC3339")
		}
		start = parsed
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("to must be after from")
	}
	if maxDays <= 0 {
		maxDays = defaultMaxDays
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("date range exceeds %d days", maxDays)
	}
	return start.UTC(), end.UTC(), nil
}

func parseExplicitRange(from, to string, maxDays int) (time.Time, time.Time, error) {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return time.Time{}, time.Time{}, errors.New("compare_from and compare_to are required together")
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(from))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid compare_from timestamp, expected RFC3339")
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(to))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid compare_to timestamp, expected RFC3339")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("compare_to must be after compare_from")
	}
	if maxDays <= 0 {
		maxDays = defaultMaxDays
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("comparison date range exceeds %d days", maxDays)
	}
	return start.UTC(), end.UTC(), nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	return min(limit, maxLimit)
}

func formatTime(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339)
}

func formatDate(ts time.Time) string {
	return ts.UTC().Format(time.DateOnly)
}

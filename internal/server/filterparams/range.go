package filterparams

import (
	"net/http"
	"time"
)

// defaultAnalyticsRangeDays is the trailing window analytics endpoints fall back
// to when the caller supplies no from/to.
const defaultAnalyticsRangeDays = 30

// ParseAnalyticsRange resolves the from/to pair every hit-backed analytics
// endpoint accepts and writes the 400 response itself when either side is
// malformed, so all of them keep the same defaults and the same strictness.
//
// Both sides must be RFC3339; anything looser (a bare date, for instance) is
// rejected rather than silently widened. The default end is tomorrow so the
// current day is fully covered, and the default start is 30 days before that end.
func ParseAnalyticsRange(w http.ResponseWriter, fromRaw, toRaw string) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -defaultAnalyticsRangeDays)

	if fromRaw != "" {
		parsed, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			http.Error(w, "Invalid from", http.StatusBadRequest)
			return time.Time{}, time.Time{}, false
		}
		start = parsed
	}
	if toRaw != "" {
		parsed, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			http.Error(w, "Invalid to", http.StatusBadRequest)
			return time.Time{}, time.Time{}, false
		}
		end = parsed
	}

	return start, end, true
}

// ParseComparisonRange resolves the optional compare_from/compare_to pair.
// A malformed or absent value yields a zero time rather than a 400: the
// comparison band is additive decoration on top of a report that is still
// perfectly serviceable without it, which is the same leniency
// GET /api/sites/{id}/stats has always applied to these two params.
// Callers treat a zero start as "no comparison requested".
func ParseComparisonRange(fromRaw, toRaw string) (time.Time, time.Time) {
	var start, end time.Time
	if fromRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, fromRaw); err == nil {
			start = parsed
		}
	}
	if toRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, toRaw); err == nil {
			end = parsed
		}
	}
	return start, end
}

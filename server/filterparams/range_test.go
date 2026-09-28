package filterparams

import (
	"net/url"
	"testing"
	"time"
)

func TestParseLenientAnalyticsRange(t *testing.T) {
	for _, values := range []url.Values{
		{},
		{"from": {"invalid"}, "to": {"invalid"}},
		{"from": {"2025-01-01T12:00:00Z"}},
		{"to": {"2025-01-02T12:00:00Z"}},
		{"from": {"0001-01-01T00:00:00Z"}, "to": {"2025-01-02T12:00:00+02:00"}},
		{"from": {"2025-01-01"}, "to": {"2025-01-02"}},
	} {
		t.Run(values.Encode(), func(t *testing.T) {
			before := time.Now().UTC()
			start, end := ParseLenientAnalyticsRange(values)
			after := time.Now().UTC()
			for _, bound := range []struct {
				key         string
				actual      time.Time
				defaultDays int
			}{{"from", start, 1 - defaultAnalyticsRangeDays}, {"to", end, 1}} {
				if want, err := time.Parse(time.RFC3339, values.Get(bound.key)); err == nil {
					if !bound.actual.Equal(want) {
						t.Errorf("%s = %v, want %v", bound.key, bound.actual, want)
					}
				} else if bound.actual.Before(before.AddDate(0, 0, bound.defaultDays)) || bound.actual.After(after.AddDate(0, 0, bound.defaultDays)) {
					t.Errorf("%s = %v, outside default range", bound.key, bound.actual)
				}
			}
		})
	}
}

package filterparams

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"hitkeep/internal/api"
)

func TestParseHitFiltersCombinesRepeatableAndLegacyFilters(t *testing.T) {
	values := url.Values{
		"filter":          {"path:/pricing", "device:Desktop"},
		"dimension_key":   {"country"},
		"dimension_value": {"US"},
	}

	filters, err := ParseHitFilters(values, LegacyPair{
		TypeParam:          "dimension_key",
		ValueParam:         "dimension_value",
		MissingMessage:     "filter type and value are required together",
		InvalidTypeMessage: "invalid filter type",
	})
	if err != nil {
		t.Fatalf("ParseHitFilters: %v", err)
	}

	want := []api.Filter{
		{Type: "path", Value: "/pricing"},
		{Type: "device", Value: "Desktop"},
		{Type: "country", Value: "US"},
	}
	if len(filters) != len(want) {
		t.Fatalf("expected %d filters, got %d: %+v", len(want), len(filters), filters)
	}
	for i := range want {
		if filters[i] != want[i] {
			t.Fatalf("filter %d mismatch: got %+v want %+v", i, filters[i], want[i])
		}
	}
}

func TestParseHitFiltersAllowsGeoNetworkFilters(t *testing.T) {
	values := url.Values{
		"filter": {"city:Mountain View", "provider:Google LLC", "asn:AS15169 Google LLC"},
	}

	filters, err := ParseHitFilters(values, LegacyPair{
		TypeParam:          "filter_type",
		ValueParam:         "filter_value",
		MissingMessage:     "filter_type and filter_value are required together",
		InvalidTypeMessage: "invalid filter_type",
	})
	if err != nil {
		t.Fatalf("ParseHitFilters: %v", err)
	}

	want := []api.Filter{
		{Type: "city", Value: "Mountain View"},
		{Type: "provider", Value: "Google LLC"},
		{Type: "asn", Value: "AS15169 Google LLC"},
	}
	if len(filters) != len(want) {
		t.Fatalf("expected %d filters, got %d: %+v", len(want), len(filters), filters)
	}
	for i := range want {
		if filters[i] != want[i] {
			t.Fatalf("filter %d mismatch: got %+v want %+v", i, filters[i], want[i])
		}
	}
}

func TestParseHitFiltersAllowsAIFilters(t *testing.T) {
	values := url.Values{
		"filter": {"ai_bot:GPTBot", "ai_bot_category:Training", "ai_source:ChatGPT"},
	}

	filters, err := ParseHitFilters(values, LegacyPair{
		TypeParam:          "filter_type",
		ValueParam:         "filter_value",
		MissingMessage:     "filter_type and filter_value are required together",
		InvalidTypeMessage: "invalid filter_type",
	})
	if err != nil {
		t.Fatalf("ParseHitFilters: %v", err)
	}

	want := []api.Filter{
		{Type: "ai_bot", Value: "GPTBot"},
		{Type: "ai_bot_category", Value: "Training"},
		{Type: "ai_source", Value: "ChatGPT"},
	}
	if len(filters) != len(want) {
		t.Fatalf("expected %d filters, got %d: %+v", len(want), len(filters), filters)
	}
	for i := range want {
		if filters[i] != want[i] {
			t.Fatalf("filter %d mismatch: got %+v want %+v", i, filters[i], want[i])
		}
	}
}

func TestParseHitFiltersRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		values  url.Values
		wantErr string
	}{
		{
			name:    "bad repeatable format",
			values:  url.Values{"filter": {"path"}},
			wantErr: "invalid filter format",
		},
		{
			name:    "empty legacy pair",
			values:  url.Values{"filter_type": {"path"}},
			wantErr: "filter_type and filter_value are required together",
		},
		{
			name:    "invalid type",
			values:  url.Values{"filter": {"unknown:value"}},
			wantErr: "invalid filter_type",
		},
		{
			name:    "unknown ai-prefixed type",
			values:  url.Values{"filter": {"ai_bogus:x"}},
			wantErr: "invalid filter_type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseHitFilters(tc.values, LegacyPair{
				TypeParam:          "filter_type",
				ValueParam:         "filter_value",
				MissingMessage:     "filter_type and filter_value are required together",
				InvalidTypeMessage: "invalid filter_type",
			})
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("expected %q error, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAllowedHitFilterTypesIsTheSortedCanonicalSet(t *testing.T) {
	got := AllowedHitFilterTypes()
	want := []string{
		"ai_bot", "ai_bot_category", "ai_source", "asn", "browser", "city", "country", "device",
		"hostname", "language", "path", "provider", "qr_code_id", "referrer", "referrer_host",
		"utm_campaign", "utm_content", "utm_medium", "utm_source", "utm_term",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected canonical filter types\nwant %v\n got %v", want, got)
	}

	// The exported view must not let a caller mutate the validator's own set.
	got[0] = "mutated"
	if AllowedHitFilterTypes()[0] != "ai_bot" {
		t.Fatal("AllowedHitFilterTypes leaked mutable state")
	}
}

func TestParseAnalyticsRangeDefaultsToTheTrailingThirtyDayWindow(t *testing.T) {
	before := time.Now().UTC()
	rec := httptest.NewRecorder()

	start, end, ok := ParseAnalyticsRange(rec, "", "")
	if !ok {
		t.Fatalf("expected empty from/to to be accepted, got %d %q", rec.Code, rec.Body.String())
	}

	// The default end is tomorrow so "today" is always fully covered, and the
	// default start is 30 days before that end.
	wantEnd := before.AddDate(0, 0, 1)
	if end.Before(wantEnd) || end.Sub(wantEnd) > time.Minute {
		t.Fatalf("expected the default end near %s, got %s", wantEnd, end)
	}
	if got := end.Sub(start); got != 30*24*time.Hour {
		t.Fatalf("expected a 30 day default window, got %s", got)
	}
	if start.Location() != time.UTC || end.Location() != time.UTC {
		t.Fatalf("expected UTC defaults, got %s and %s", start.Location(), end.Location())
	}
}

func TestParseAnalyticsRangeUsesSuppliedTimestamps(t *testing.T) {
	rec := httptest.NewRecorder()

	start, end, ok := ParseAnalyticsRange(rec, "2026-01-02T03:04:05Z", "2026-02-03T04:05:06Z")
	if !ok {
		t.Fatalf("expected valid timestamps to be accepted, got %d %q", rec.Code, rec.Body.String())
	}
	if !start.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("unexpected start %s", start)
	}
	if !end.Equal(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)) {
		t.Fatalf("unexpected end %s", end)
	}
}

func TestParseAnalyticsRangeRejectsNonRFC3339Timestamps(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantBody string
	}{
		{name: "date only from", from: "2026-01-02", to: "", wantBody: "Invalid from\n"},
		{name: "date only to", from: "", to: "2026-01-02", wantBody: "Invalid to\n"},
		{name: "garbage from", from: "yesterday", to: "", wantBody: "Invalid from\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			start, end, ok := ParseAnalyticsRange(rec, tc.from, tc.to)
			if ok {
				t.Fatalf("expected rejection, got %s..%s", start, end)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			if rec.Body.String() != tc.wantBody {
				t.Fatalf("expected %q, got %q", tc.wantBody, rec.Body.String())
			}
			if !start.IsZero() || !end.IsZero() {
				t.Fatalf("expected zero times on rejection, got %s..%s", start, end)
			}
		})
	}
}

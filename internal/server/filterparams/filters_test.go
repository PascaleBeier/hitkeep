package filterparams

import (
	"net/url"
	"testing"

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

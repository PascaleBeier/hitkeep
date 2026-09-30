package analyticstools

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"hitkeep/server/filterparams"
)

func TestParseFiltersAllowsGeoNetworkDimensions(t *testing.T) {
	filters, err := parseFilters([]FilterInput{
		{Type: "city", Value: "Mountain View"},
		{Type: "provider", Value: "Google LLC"},
		{Type: "asn", Value: "AS15169 Google LLC"},
	})
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	if len(filters) != 3 {
		t.Fatalf("expected 3 filters, got %d", len(filters))
	}
	if filters[0].Type != "city" || filters[1].Type != "provider" || filters[2].Type != "asn" {
		t.Fatalf("unexpected filters: %+v", filters)
	}
}

func TestParseFiltersAllowsAIVisibilityDimensions(t *testing.T) {
	filters, err := parseFilters([]FilterInput{
		{Type: "ai_bot", Value: "GPTBot"},
		{Type: "ai_bot_category", Value: "ai_training_crawler"},
		{Type: "ai_source", Value: "ChatGPT"},
	})
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	if len(filters) != 3 {
		t.Fatalf("expected 3 filters, got %d", len(filters))
	}
	if filters[0].Type != "ai_bot" || filters[1].Type != "ai_bot_category" || filters[2].Type != "ai_source" {
		t.Fatalf("unexpected filters: %+v", filters)
	}
}

// TestFilterAllowlistDerivesFromCanonicalSet pins the one deliberate
// difference between the REST filter set and the tool one. A new canonical filter
// type reaches the tools automatically; dropping it needs an explicit exclusion.
func TestFilterAllowlistDerivesFromCanonicalSet(t *testing.T) {
	canonical := filterparams.AllowedHitFilterTypes()
	want := make([]string, 0, len(canonical))
	for _, filterType := range canonical {
		if filterType == "qr_code_id" {
			continue
		}
		want = append(want, filterType)
	}

	if !slices.Equal(FilterTypes, want) {
		t.Fatalf("tool filter allowlist drifted\nwant %v\n got %v", want, FilterTypes)
	}
	if slices.Contains(FilterTypes, "qr_code_id") {
		t.Fatal("qr_code_id is a dashboard-only drill-down and must stay off the tool surface")
	}
}

// TestFilterInputSchemaDocumentsAllowedFilterTypes keeps the jsonschema doc
// string honest: struct tags are compile-time constants, so the only way to stop
// them drifting from the derived allowlist is to fail CI when they do.
func TestFilterInputSchemaDocumentsAllowedFilterTypes(t *testing.T) {
	field, ok := reflect.TypeFor[FilterInput]().FieldByName("Type")
	if !ok {
		t.Fatal("FilterInput has no Type field")
	}

	const prefix = "Filter type: "
	doc := field.Tag.Get("jsonschema")
	if !strings.HasPrefix(doc, prefix) {
		t.Fatalf("unexpected filter type doc string %q", doc)
	}

	listed := strings.Split(strings.TrimSuffix(strings.TrimPrefix(doc, prefix), "."), ",")
	for i, entry := range listed {
		listed[i] = strings.TrimPrefix(strings.TrimSpace(entry), "or ")
	}
	slices.Sort(listed)

	want := slices.Clone(FilterTypes)
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Fatalf("FilterInput doc string drifted from the allowlist\nwant %v\n got %v", want, listed)
	}
}

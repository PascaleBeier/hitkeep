package database

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"hitkeep/internal/api"
)

func TestBuildHitFilter(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		value      string
		wantClause string
		wantArgs   []any
	}{
		{
			name:       "utm source",
			field:      "utm_source",
			value:      "newsletter",
			wantClause: " AND COALESCE(NULLIF(TRIM(h.utm_source), ''), '(Unspecified)') = ?",
			wantArgs:   []any{"newsletter"},
		},
		{
			name:       "utm campaign unspecified",
			field:      "utm_campaign",
			value:      "unspecified",
			wantClause: " AND COALESCE(NULLIF(TRIM(h.utm_campaign), ''), '(Unspecified)') = ?",
			wantArgs:   []any{"(Unspecified)"},
		},
		{
			name:       "utm campaign parenthesized unspecified",
			field:      "utm_campaign",
			value:      "(unspecified)",
			wantClause: " AND COALESCE(NULLIF(TRIM(h.utm_campaign), ''), '(Unspecified)') = ?",
			wantArgs:   []any{"(Unspecified)"},
		},
		{
			name:       "language base code",
			field:      "language",
			value:      "de-DE",
			wantClause: " AND CASE WHEN NULLIF(TRIM(h.language), '') IS NULL THEN '(Unspecified)' ELSE lower(split_part(TRIM(h.language), '-', 1)) END = ?",
			wantArgs:   []any{"de"},
		},
		{
			name:       "hostname",
			field:      "hostname",
			value:      "blog.example.com",
			wantClause: " AND COALESCE(NULLIF(TRIM(h.hostname), ''), '(Unknown Host)') = ?",
			wantArgs:   []any{"blog.example.com"},
		},
		{
			name:  "referrer host",
			field: "referrer_host",
			value: "www.Google.com",
			wantClause: ` AND CASE
		WHEN h.referrer IS NULL OR NULLIF(TRIM(h.referrer), '') IS NULL THEN '(Direct)'
		WHEN lower(h.referrer) LIKE 'http%' THEN regexp_replace(regexp_extract(lower(h.referrer), 'https?://([^/:?#]+)', 1), '^www\\.', '')
		ELSE regexp_replace(lower(TRIM(h.referrer)), '^www\\.', '')
	END = ?`,
			wantArgs: []any{"google.com"},
		},
		{
			name:       "AI bot",
			field:      "ai_bot",
			value:      "GPTBot",
			wantClause: " AND hk_ai_bot(h.user_agent) = ?",
			wantArgs:   []any{"GPTBot"},
		},
		{
			name:       "AI source",
			field:      "ai_source",
			value:      "ChatGPT",
			wantClause: " AND hk_ai_source(h.referrer) = ?",
			wantArgs:   []any{"ChatGPT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := buildHitFilter(tt.field, tt.value, "h")
			if clause != tt.wantClause {
				t.Fatalf("unexpected clause: got %q want %q", clause, tt.wantClause)
			}
			if diff := cmp.Diff(tt.wantArgs, args); diff != "" {
				t.Fatalf("unexpected args (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildHitFiltersIncludesUTMClauses(t *testing.T) {
	filters := []api.Filter{
		{Type: "utm_medium", Value: "paid"},
		{Type: "path", Value: "/pricing"},
	}
	clause, args := buildHitFilters(filters, "h")
	wantClause := " AND COALESCE(NULLIF(TRIM(h.utm_medium), ''), '(Unspecified)') = ? AND h.path = ?"
	if clause != wantClause {
		t.Fatalf("unexpected clause: got %q want %q", clause, wantClause)
	}
	if diff := cmp.Diff([]any{"paid", "/pricing"}, args); diff != "" {
		t.Fatalf("unexpected args (-want +got):\n%s", diff)
	}
}

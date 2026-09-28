package database

import (
	"strings"
	"testing"
)

func TestPrepareAIRunOutputJSONEnforcesGoAIStorageBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		feature     string
		output      string
		evidenceIDs []string
		wantOutput  string
		wantErr     string
	}{
		{
			name:       "empty output becomes safe object",
			feature:    "imports",
			output:     "",
			wantOutput: "{}",
		},
		{
			name:        "opportunity output may cite supplied evidence",
			feature:     "opportunities",
			output:      `{"title_key":"opportunities.fixture.title","summary_key":"opportunities.fixture.summary","action_key":"opportunities.fixture.action","digest_key":"opportunities.fixture.digest","copy_params":{"rate":"42%"},"cited_evidence_ids":["checkout-rate"]}`,
			evidenceIDs: []string{"checkout-rate"},
			wantOutput:  `{"title_key":"opportunities.fixture.title","summary_key":"opportunities.fixture.summary","action_key":"opportunities.fixture.action","digest_key":"opportunities.fixture.digest","copy_params":{"rate":"42%"},"cited_evidence_ids":["checkout-rate"]}`,
		},
		{
			name:    "scalar output is rejected",
			feature: "opportunities",
			output:  `"raw provider response body"`,
			wantErr: "object",
		},
		{
			name:    "raw prompt fields are rejected",
			feature: "opportunities",
			output:  `{"title_key":"opportunities.fixture.title","raw_prompt":"prompt text"}`,
			wantErr: "raw prompt",
		},
		{
			name:    "provider payload fields are rejected",
			feature: "opportunities",
			output:  `{"title_key":"opportunities.fixture.title","provider_response":{"raw":"payload"}}`,
			wantErr: "provider payload",
		},
		{
			name:    "credential-shaped values are rejected",
			feature: "opportunities",
			output:  `{"title_key":"opportunities.fixture.title","debug":"Authorization: Bearer placeholder"}`,
			wantErr: "raw payload",
		},
		{
			name:    "customer prose fields are rejected",
			feature: "opportunities",
			output:  `{"title":"Rewrite the checkout page","cited_evidence_ids":["checkout-rate"]}`,
			wantErr: "customer prose",
		},
		{
			name:        "invented cited evidence is rejected",
			feature:     "opportunities",
			output:      `{"title_key":"opportunities.fixture.title","cited_evidence_ids":["checkout-rate","invented"]}`,
			evidenceIDs: []string{"checkout-rate"},
			wantErr:     "invented",
		},
		{
			name:        "malformed cited evidence is rejected",
			feature:     "opportunities",
			output:      `{"title_key":"opportunities.fixture.title","cited_evidence_ids":"checkout-rate"}`,
			evidenceIDs: []string{"checkout-rate"},
			wantErr:     "string array",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := prepareAIRunOutputJSON(tc.feature, tc.output, tc.evidenceIDs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("prepare output json: %v", err)
				}
				if got != tc.wantOutput {
					t.Fatalf("output json = %q, want %q", got, tc.wantOutput)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected %q validation error", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q validation error, got %v", tc.wantErr, err)
			}
		})
	}
}

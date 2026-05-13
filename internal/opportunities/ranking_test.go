package opportunities

import (
	"testing"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestRankOpportunitiesReturnsEmptySliceForNilInput(t *testing.T) {
	ranked := RankOpportunities(nil)
	if ranked == nil {
		t.Fatal("expected empty opportunity slice, got nil")
	}
	if len(ranked) != 0 {
		t.Fatalf("expected no opportunities, got %#v", ranked)
	}
}

func TestRankOpportunityInputsReturnsEmptySliceForNilInput(t *testing.T) {
	ranked := rankOpportunityInputs(nil)
	if ranked == nil {
		t.Fatal("expected empty opportunity input slice, got nil")
	}
	if len(ranked) != 0 {
		t.Fatalf("expected no opportunity inputs, got %#v", ranked)
	}
}

func TestRankOpportunitiesDoesNotAliasInput(t *testing.T) {
	input := []api.Opportunity{
		{Score: 1},
		{
			Score:            9,
			CopyParams:       map[string]any{"copy": "original"},
			RouteParams:      map[string]any{"route": "original"},
			Evidence:         []api.OpportunityEvidence{{ID: "original"}},
			CitedEvidenceIDs: []string{"original"},
		},
	}

	ranked := RankOpportunities(input)
	ranked[0].Score = 100
	ranked[0].CopyParams["copy"] = "changed"
	ranked[0].RouteParams["route"] = "changed"
	ranked[0].Evidence[0].ID = "changed"
	ranked[0].CitedEvidenceIDs[0] = "changed"

	if input[1].Score != 9 {
		t.Fatalf("expected ranked copy not to alias input, got %#v", input)
	}
	if input[1].CopyParams["copy"] != "original" ||
		input[1].RouteParams["route"] != "original" ||
		input[1].Evidence[0].ID != "original" ||
		input[1].CitedEvidenceIDs[0] != "original" {
		t.Fatalf("expected ranked nested collections not to alias input, got %#v", input[1])
	}
}

func TestRankOpportunitiesNormalizesPublicCollections(t *testing.T) {
	ranked := RankOpportunities([]api.Opportunity{{Score: 1}})

	if ranked[0].CopyParams == nil {
		t.Fatal("expected copy params to be an empty object")
	}
	if ranked[0].RouteParams == nil {
		t.Fatal("expected route params to be an empty object")
	}
	if ranked[0].Evidence == nil {
		t.Fatal("expected evidence to be an empty array")
	}
	if ranked[0].CitedEvidenceIDs == nil {
		t.Fatal("expected cited evidence ids to be an empty array")
	}
}

func TestRankOpportunityInputsDoesNotAliasInput(t *testing.T) {
	input := []database.OpportunityInput{
		{Score: 1},
		{
			Score:            9,
			CopyParams:       map[string]any{"copy": "original"},
			RouteParams:      map[string]any{"route": "original"},
			Evidence:         []api.OpportunityEvidence{{ID: "original"}},
			CitedEvidenceIDs: []string{"original"},
		},
	}

	ranked := rankOpportunityInputs(input)
	ranked[0].Score = 100
	ranked[0].CopyParams["copy"] = "changed"
	ranked[0].RouteParams["route"] = "changed"
	ranked[0].Evidence[0].ID = "changed"
	ranked[0].CitedEvidenceIDs[0] = "changed"

	if input[1].Score != 9 {
		t.Fatalf("expected ranked copy not to alias input, got %#v", input)
	}
	if input[1].CopyParams["copy"] != "original" ||
		input[1].RouteParams["route"] != "original" ||
		input[1].Evidence[0].ID != "original" ||
		input[1].CitedEvidenceIDs[0] != "original" {
		t.Fatalf("expected ranked nested collections not to alias input, got %#v", input[1])
	}
}

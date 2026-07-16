package server

import (
	"testing"

	"hitkeep/internal/database"
)

func TestDatabaseUnavailableResponseDistinguishesRecoveryStates(t *testing.T) {
	tests := []struct {
		state   string
		code    string
		message string
	}{
		{
			state:   database.DatabaseStateRecovering,
			code:    "database_recovering",
			message: "Database recovery is in progress",
		},
		{
			state:   database.DatabaseStateNeedsAttention,
			code:    "database_needs_attention",
			message: "Database recovery requires operator attention",
		},
		{
			state:   database.DatabaseStateFailed,
			code:    "database_unavailable",
			message: "Database is unavailable",
		},
	}
	for _, test := range tests {
		code, message := databaseUnavailableResponse(test.state)
		if code != test.code || message != test.message {
			t.Fatalf("state %q: expected %q/%q, got %q/%q", test.state, test.code, test.message, code, message)
		}
	}
}

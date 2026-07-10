package webhooks

import (
	"testing"
)

func TestCatalogSeparatesInstanceAndSiteEvents(t *testing.T) {
	t.Parallel()

	if !EventAllowedForScope(EventSiteCreated, ScopeInstance) {
		t.Fatal("site.created must be available to instance webhooks")
	}
	if EventAllowedForScope(EventSiteCreated, ScopeSite) {
		t.Fatal("site.created must not be available to site webhooks")
	}

	for _, eventType := range []string{
		EventSiteUpdated,
		EventSiteDeleted,
		EventGoalCreated,
		EventGoalUpdated,
		EventGoalDeleted,
		EventGoalConverted,
		EventImportCompleted,
		EventImportFailed,
		EventWebhookTest,
	} {
		if !EventAllowedForScope(eventType, ScopeInstance) {
			t.Errorf("%s must be available to instance webhooks", eventType)
		}
		if !EventAllowedForScope(eventType, ScopeSite) {
			t.Errorf("%s must be available to site webhooks", eventType)
		}
	}
}

func TestValidateEventSelectionRejectsUnknownDuplicateAndWrongScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scope  Scope
		events []string
	}{
		{name: "empty", scope: ScopeSite},
		{name: "unknown", scope: ScopeSite, events: []string{"pageview.created"}},
		{name: "duplicate", scope: ScopeSite, events: []string{EventGoalCreated, EventGoalCreated}},
		{name: "instance only", scope: ScopeSite, events: []string{EventSiteCreated}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEventSelection(tt.scope, tt.events); err == nil {
				t.Fatal("expected invalid event selection")
			}
		})
	}

	if err := ValidateEventSelection(ScopeSite, []string{EventGoalCreated, EventImportCompleted}); err != nil {
		t.Fatalf("expected valid site event selection: %v", err)
	}
}

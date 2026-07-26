package shared

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAnalyticsStoreNeverFallsBackToControlStore(t *testing.T) {
	control := &Context{}

	store, err := control.AnalyticsStore(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected analytics resolution to fail without the tenant data plane")
	}
	if store != nil {
		t.Fatal("analytics resolution returned a control-plane store")
	}
}

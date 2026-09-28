package searchconsole

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hitkeep/jsonapi"
)

func TestGoogleOperationContextPreservesParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := googleOperationContext(parent)
	defer cancel()
	cancelParent()

	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatalf("context error = %v, want %v", ctx.Err(), context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("operation context did not inherit parent cancellation")
	}
}

func TestGoogleClientOperationsHaveDeadline(t *testing.T) {
	oldTimeout := googleOperationTimeout
	googleOperationTimeout = 50 * time.Millisecond
	t.Cleanup(func() { googleOperationTimeout = oldTimeout })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()

	client := NewGoogleClient(OAuthConfig{ClientID: "client-id", ClientSecret: "client-secret", TokenURL: server.URL, APIBaseURL: server.URL})
	token := Token{AccessToken: "access-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
	query := SearchAnalyticsQuery{SiteURL: "sc-domain:example.com", StartDate: time.Now().AddDate(0, 0, -1), EndDate: time.Now(), RowLimit: 1}
	operations := []struct {
		name string
		run  func(context.Context) error
	}{
		{"exchange", func(ctx context.Context) error {
			_, err := client.ExchangeCode(ctx, "code", "https://hitkeep.test/callback")
			return err
		}},
		{"list", func(ctx context.Context) error { _, err := client.ListProperties(ctx, token); return err }},
		{"query", func(ctx context.Context) error { _, err := client.QuerySearchAnalytics(ctx, token, query); return err }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("operation error = %v, want deadline exceeded", err)
			}
		})
	}
}

func TestGoogleClientQuerySearchAnalyticsCeilingsReturnErrors(t *testing.T) {
	oldPages, oldRows := maxSearchAnalyticsPages, maxSearchAnalyticsRows
	maxSearchAnalyticsPages, maxSearchAnalyticsRows = 2, 3
	t.Cleanup(func() { maxSearchAnalyticsPages, maxSearchAnalyticsRows = oldPages, oldRows })

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_ = jsonapi.MarshalWrite(w, map[string]any{"rows": []map[string]any{{"clicks": 1}, {"clicks": 2}}})
	}))
	defer server.Close()

	client := NewGoogleClient(OAuthConfig{ClientID: "client-id", ClientSecret: "client-secret", APIBaseURL: server.URL})
	token := Token{AccessToken: "access-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
	query := SearchAnalyticsQuery{SiteURL: "sc-domain:example.com", StartDate: time.Now().AddDate(0, 0, -1), EndDate: time.Now(), RowLimit: 2}
	_, err := client.QuerySearchAnalytics(context.Background(), token, query)
	if err == nil || !strings.Contains(err.Error(), "exceeds 3 rows") {
		t.Fatalf("query error = %v, want row ceiling", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}

	maxSearchAnalyticsRows = 10
	requests = 0
	_, err = client.QuerySearchAnalytics(context.Background(), token, query)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exceeds %d pages", maxSearchAnalyticsPages)) {
		t.Fatalf("query error = %v, want page ceiling", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

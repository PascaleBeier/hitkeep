package searchconsole

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func TestGoogleClientAuthCodeURLUsesReadOnlyScope(t *testing.T) {
	client := NewGoogleClient(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	})

	authURL, err := client.AuthCodeURL("state-token", "https://hitkeep.test/api/integrations/google-search-console/oauth/callback")
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	if !strings.Contains(authURL, "scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fwebmasters.readonly") {
		t.Fatalf("expected read-only scope in auth URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "access_type=offline") {
		t.Fatalf("expected offline access in auth URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "prompt=consent") {
		t.Fatalf("expected forced consent in auth URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "state=state-token") {
		t.Fatalf("expected state in auth URL, got %q", authURL)
	}
}

func TestGoogleClientExchangeCodeUsesConfiguredTokenEndpoint(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected token path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("code") != "oauth-code" {
			t.Fatalf("expected oauth-code, got %q", r.Form.Get("code"))
		}
		if r.Form.Get("scope") != "" {
			t.Fatalf("token exchange should not request extra scopes, got %q", r.Form.Get("scope"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	client := NewGoogleClient(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		AuthURL:      "https://accounts.example.test/auth",
		TokenURL:     tokenServer.URL + "/token",
	})

	token, err := client.ExchangeCode(context.Background(), "oauth-code", "https://hitkeep.test/callback")
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	if token.AccessToken != "access-token" || token.RefreshToken != "refresh-token" {
		t.Fatalf("expected token material to round-trip, got %+v", token)
	}
	if token.Scope != ReadOnlyScope {
		t.Fatalf("expected scope %q, got %q", ReadOnlyScope, token.Scope)
	}
	if time.Until(token.Expiry) <= 0 {
		t.Fatalf("expected future expiry, got %s", token.Expiry)
	}
}

func TestGoogleClientListPropertiesUsesOfficialSitesEndpoint(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webmasters/v3/sites" {
			t.Fatalf("unexpected sites path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("expected bearer token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"siteEntry": []map[string]string{
				{"siteUrl": "sc-domain:example.com", "permissionLevel": "SITE_OWNER"},
				{"siteUrl": "https://www.example.com/", "permissionLevel": "SITE_FULL_USER"},
			},
		})
	}))
	defer apiServer.Close()

	client := NewGoogleClient(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		APIBaseURL:   apiServer.URL,
	})

	properties, err := client.ListProperties(context.Background(), Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("list properties: %v", err)
	}
	if len(properties) != 2 {
		t.Fatalf("expected two properties, got %+v", properties)
	}
	if properties[0].URI != "sc-domain:example.com" || properties[0].PermissionLevel != "SITE_OWNER" {
		t.Fatalf("unexpected first property: %+v", properties[0])
	}
}

func TestClassifyErrorDetectsDisabledSearchConsoleAPI(t *testing.T) {
	err := &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Google Search Console API has not been used in project 123 before or it is disabled.",
		Errors: []googleapi.ErrorItem{
			{Reason: "accessNotConfigured"},
		},
	}
	if got := ClassifyError(err); got != CategoryAPIDisabled {
		t.Fatalf("expected %s, got %s", CategoryAPIDisabled, got)
	}
}

func TestGoogleClientQuerySearchAnalyticsUsesFinalDataState(t *testing.T) {
	var requestBody string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/webmasters/v3/sites/sc-domain%3Aexample.com/searchAnalytics/query" {
			t.Fatalf("unexpected search analytics path %s", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("expected bearer token, got %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responseAggregationType": "AUTO",
			"rows": []map[string]any{
				{
					"keys":        []string{"2026-05-01", "hitkeep", "https://example.com/", "USA", "DESKTOP"},
					"clicks":      7,
					"impressions": 70,
					"ctr":         0.1,
					"position":    2.4,
				},
			},
		})
	}))
	defer apiServer.Close()

	client := NewGoogleClient(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		APIBaseURL:   apiServer.URL,
	})
	rows, err := client.QuerySearchAnalytics(context.Background(), Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().UTC().Add(time.Hour),
	}, SearchAnalyticsQuery{
		SiteURL:    "sc-domain:example.com",
		StartDate:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Dimensions: []string{"date", "query", "page", "country", "device"},
		DataState:  DataStateFinal,
	})
	if err != nil {
		t.Fatalf("query search analytics: %v", err)
	}
	for _, expected := range []string{`"dataState":"FINAL"`, `"startDate":"2026-05-01"`, `"endDate":"2026-05-03"`, `"dimensions":["DATE","QUERY","PAGE","COUNTRY","DEVICE"]`} {
		if !strings.Contains(requestBody, expected) {
			t.Fatalf("expected request body to contain %s, got %s", expected, requestBody)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %+v", rows)
	}
	if rows[0].Date.Format(time.DateOnly) != "2026-05-01" || rows[0].Query != "hitkeep" || rows[0].Clicks != 7 || rows[0].AggregationType != "auto" || rows[0].DataState != "final" {
		t.Fatalf("unexpected mapped row: %+v", rows[0])
	}
}

func TestGoogleClientQuerySearchAnalyticsPaginatesRows(t *testing.T) {
	var requestBodies []string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBodies = append(requestBodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		rows := []map[string]any{
			{"keys": []string{"2026-05-01"}, "clicks": 1, "impressions": 10},
			{"keys": []string{"2026-05-02"}, "clicks": 2, "impressions": 20},
		}
		if len(requestBodies) > 1 {
			rows = []map[string]any{
				{"keys": []string{"2026-05-03"}, "clicks": 3, "impressions": 30},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responseAggregationType": "AUTO",
			"rows":                    rows,
		})
	}))
	defer apiServer.Close()

	client := NewGoogleClient(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		APIBaseURL:   apiServer.URL,
	})
	rows, err := client.QuerySearchAnalytics(context.Background(), Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().UTC().Add(time.Hour),
	}, SearchAnalyticsQuery{
		SiteURL:    "sc-domain:example.com",
		StartDate:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Dimensions: []string{"date"},
		RowLimit:   2,
	})
	if err != nil {
		t.Fatalf("query search analytics: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected paginated rows, got %+v", rows)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected two paginated requests, got %d: %+v", len(requestBodies), requestBodies)
	}
	if !strings.Contains(requestBodies[0], `"rowLimit":2`) || strings.Contains(requestBodies[0], `"startRow":`) {
		t.Fatalf("expected first request to start at row 0 implicitly, got %s", requestBodies[0])
	}
	if !strings.Contains(requestBodies[1], `"startRow":2`) {
		t.Fatalf("expected second request to continue at startRow 2, got %s", requestBodies[1])
	}
}

func TestClassifyErrorRecognizesOAuthTokenRefreshFailures(t *testing.T) {
	err := fmt.Errorf("oauth2: cannot fetch token: 400 Bad Request\nResponse: {\"error\":\"invalid_grant\"}")
	if got := ClassifyError(err); got != CategoryTokenRefreshFailed {
		t.Fatalf("expected token refresh failure, got %q", got)
	}
}

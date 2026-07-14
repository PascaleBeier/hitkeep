package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func crossOriginTestHandler() http.Handler {
	return newCrossOriginProtection().Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func TestCrossOriginProtectionAllowsSafeCrossSiteRequests(t *testing.T) {
	handler := crossOriginTestHandler()

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/status", nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}

	for _, path := range []string{
		"/api/auth/sso",
		"/api/auth/sso/callback?state=state-token&code=authorization-code",
		"/api/integrations/google-search-console/oauth/callback?state=state-token&code=authorization-code",
		"/api/cloud/signup/verify?token=signup-token",
		"/api/auth/mfa/email-link/verify?token=mfa-token",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}
}

func TestCrossOriginProtectionAllowsNonBrowserUnsafeRequests(t *testing.T) {
	handler := crossOriginTestHandler()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/sites/00000000-0000-0000-0000-000000000001", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}

	for _, path := range []string{
		"/api/ingest/server/pageview",
		"/api/ingest/server/event",
		"/api/cloud/webhooks/stripe",
		"/api/sites/00000000-0000-0000-0000-000000000001/ingest/ai-fetch",
		"/mcp",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}
}

func TestCrossOriginProtectionAllowsExpectedBrowserContexts(t *testing.T) {
	handler := crossOriginTestHandler()

	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "same origin", header: "Sec-Fetch-Site", value: "same-origin"},
		{name: "user initiated", header: "Sec-Fetch-Site", value: "none"},
		{name: "matching origin", header: "Origin", value: "https://hitkeep.example"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
			req.Host = "hitkeep.example"
			req.Header.Set(test.header, test.value)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}
}

func TestCrossOriginProtectionBlocksUnsafeCrossOriginBrowserRequests(t *testing.T) {
	handler := crossOriginTestHandler()

	for _, site := range []string{"cross-site", "same-site"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			t.Run(site+"/"+method, func(t *testing.T) {
				req := httptest.NewRequest(method, "/api/login", nil)
				req.Header.Set("Sec-Fetch-Site", site)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusNotFound {
					t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
				}
			})
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.Host = "hitkeep.example"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mismatched Origin: expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestCrossOriginProtectionBlocksBrowserRequestsToServerEndpoints(t *testing.T) {
	handler := crossOriginTestHandler()

	for _, path := range []string{
		"/api/ingest/server/pageview",
		"/api/ingest/server/event",
		"/api/cloud/webhooks/stripe",
		"/api/sites/00000000-0000-0000-0000-000000000001/ingest/ai-fetch",
		"/mcp",
		"/api/sites",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
			}
		})
	}
}

func TestCrossOriginProtectionBypassesOnlyPublicBrowserIngest(t *testing.T) {
	handler := crossOriginTestHandler()

	for _, path := range []string{"/ingest", "/ingest/event", "/ingest/web-vitals"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
			}
		})
	}

	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/ingest"},
		{method: http.MethodPost, path: "/ingest/other"},
		{method: http.MethodPost, path: "/api/ingest/server/pageview"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
			}
		})
	}
}

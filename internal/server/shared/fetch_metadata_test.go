package shared

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchMetadataMiddleware_AllowsRequestsWithoutHeaders(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_BlocksCrossSiteAPIRequests(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsSameOriginAPIRequests(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_BlocksIngestNavigation(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsIngestCORS(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/ingest/event", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_BlocksStateChangingAPIWithoutOriginFallback(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsStateChangingAPIWithMatchingOriginFallback(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.Header.Set("Origin", "https://hitkeep.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsStripeWebhookWithoutBrowserHeaders(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsServerIngestWithoutBrowserHeaders(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []string{"/api/ingest/server/pageview", "/api/ingest/server/event"} {
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

func TestFetchMetadataMiddleware_AllowsStateChangingAPIWithMatchingRefererFallback(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.Header.Set("Referer", "https://hitkeep.example/login")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestFetchMetadataMiddleware_BlocksStateChangingAPIWithMismatchedOriginFallback(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestFetchMetadataMiddleware_AllowsSignupVerifyFromEmail(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// Browsers navigating from an email link send Sec-Fetch-Site: cross-site or none.
	for _, site := range []string{"cross-site", "none"} {
		t.Run(site, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token=abc123", nil)
			req.Header.Set("Sec-Fetch-Site", site)
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("Sec-Fetch-Site=%s: expected status %d, got %d", site, http.StatusNoContent, rec.Code)
			}
		})
	}
}

func TestFetchMetadataMiddleware_AllowsMFAEmailLinkVerifyFromEmail(t *testing.T) {
	handler := FetchMetadataMiddleware("https://hitkeep.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// Browsers navigating from an email link send Sec-Fetch-Site: cross-site or none.
	for _, site := range []string{"cross-site", "none"} {
		t.Run(site, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/auth/mfa/email-link/verify?token=abc123", nil)
			req.Header.Set("Sec-Fetch-Site", site)
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("Sec-Fetch-Site=%s: expected status %d, got %d", site, http.StatusNoContent, rec.Code)
			}
		})
	}
}

func TestCanonicalOrigin_NormalizesDefaultPortsAndIPv6(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "https default port",
			raw:  "https://hitkeep.example:443/path",
			want: "https://hitkeep.example",
		},
		{
			name: "http default port",
			raw:  "http://hitkeep.example:80/path",
			want: "http://hitkeep.example",
		},
		{
			name: "ipv6 host preserves brackets",
			raw:  "https://[2001:db8::1]:8443/path",
			want: "https://[2001:db8::1]:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
			req.Header.Set("Origin", tt.raw)

			got := originFromURLString(req.Header.Get("Origin"))
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

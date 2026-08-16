package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/internal/server/shared"
)

func TestRequestLoggingMiddlewareAddsRequestFields(t *testing.T) {
	var logs bytes.Buffer
	server := &Server{logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := shared.LoggerFromContext(r.Context()); got == nil {
			t.Fatal("request logger is missing")
		}
		w.WriteHeader(http.StatusCreated)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/sites/123", nil)
	server.requestLoggingMiddleware(next).ServeHTTP(recorder, request)

	requestID := recorder.Header().Get("X-Request-Id")
	if requestID == "" {
		t.Fatal("expected generated request ID")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	for _, want := range []string{
		"request_id=" + requestID,
		"http.method=POST",
		"http.path=/api/sites/123",
		"http.status=201",
		"msg=\"HTTP request completed\"",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs do not contain %q: %s", want, logs.String())
		}
	}
}

func TestRequestLoggingMiddlewareEmitsOneHTTPGroup(t *testing.T) {
	var logs bytes.Buffer
	server := &Server{logger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	request := httptest.NewRequest(http.MethodPut, "/api/sites/123", nil)
	server.requestLoggingMiddleware(next).ServeHTTP(httptest.NewRecorder(), request)

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record); err != nil {
		t.Fatalf("unmarshal request log: %v; logs: %s", err, logs.String())
	}
	httpFields, ok := record["http"].(map[string]any)
	if !ok {
		t.Fatalf("http field = %T, want object; record: %s", record["http"], logs.String())
	}
	for key, want := range map[string]any{
		"method": "PUT",
		"path":   "/api/sites/123",
		"status": float64(http.StatusAccepted),
	} {
		if got := httpFields[key]; got != want {
			t.Errorf("http.%s = %v, want %v", key, got, want)
		}
	}
	if _, ok := httpFields["duration_ms"]; !ok {
		t.Error("http.duration_ms is missing")
	}
}

func TestRequestLoggingMiddlewareReusesSafeRequestID(t *testing.T) {
	var logs bytes.Buffer
	server := &Server{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-Id", "client-request-42")
	server.requestLoggingMiddleware(next).ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Request-Id"); got != "client-request-42" {
		t.Fatalf("request ID = %q, want client-request-42", got)
	}
}

func TestSafeRequestPathRedactsTokenSegments(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/share/raw-secret/sites/1/stats", want: "/api/share/[redacted]/sites/1/stats"},
		{path: "/api/qr-share/raw-secret/qr-code", want: "/api/qr-share/[redacted]/qr-code"},
		{path: "/q/raw-secret", want: "/q/[redacted]"},
		{path: "/internal/caddy/on-demand-tls/raw-secret", want: "/internal/caddy/on-demand-tls/[redacted]"},
		{path: "/api/report-recipient-confirmations/raw-secret", want: "/api/report-recipient-confirmations/[redacted]"},
		{path: "/api/reports/unsubscribe/raw-secret", want: "/api/reports/unsubscribe/[redacted]"},
		{path: "/api/sites/1/stats", want: "/api/sites/1/stats"},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := safeRequestPath(test.path); got != test.want {
				t.Fatalf("safeRequestPath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

package shared

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(t.Context(), rec, http.StatusCreated, map[string]string{"b": "<2>", "a": "1"})

	res := rec.Result() // headers as sent, not the live header map
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusCreated)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	// Deterministic key order and HTML escaping come from the jsonapi policy.
	body := rec.Body.String()
	if !strings.HasPrefix(body, `{"a":"1","b":`) || strings.ContainsAny(body, "<>") {
		t.Fatalf("body = %s, want sorted keys and HTML-escaped values", body)
	}
	if got := res.Header.Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length = %q, want %d", got, len(body))
	}
}

func TestWriteJSONEncodeFailureIsServerError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(t.Context(), rec, http.StatusOK, map[string]string{"title": "bad \xff utf-8"})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("Content-Type"); got == "application/json" {
		t.Fatalf("error response must not claim a JSON body")
	}
}

func TestPathUUID(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name   string
		value  string
		wantOK bool
	}{
		{"valid", id.String(), true},
		{"surrounding whitespace", " " + id.String() + " ", true},
		{"invalid", "not-a-uuid", false},
		{"missing", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("id", tc.value)
			rec := httptest.NewRecorder()

			got, ok := PathUUID(rec, r, "id", "Invalid ID")
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != id {
				t.Fatalf("id = %s, want %s", got, id)
			}
			if !ok && rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

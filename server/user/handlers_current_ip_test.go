package user

import (
	"net/http"
	"net/http/httptest"
	"testing"

	json "hitkeep/jsonapi"
)

func TestGetCurrentIPIPv4(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/current-ip", nil), userID)
	req.RemoteAddr = "203.0.113.44:12345"
	w := httptest.NewRecorder()

	h.handleGetCurrentIP().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		IP   string `json:"ip"`
		CIDR string `json:"cidr"`
	}
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.IP != "203.0.113.44" {
		t.Fatalf("expected ip %q, got %q", "203.0.113.44", resp.IP)
	}
	if resp.CIDR != "203.0.113.44/32" {
		t.Fatalf("expected cidr %q, got %q", "203.0.113.44/32", resp.CIDR)
	}
}

func TestGetCurrentIPIPv6(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	defer store.Close()

	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/current-ip", nil), userID)
	req.RemoteAddr = "[2001:db8::44]:12345"
	w := httptest.NewRecorder()

	h.handleGetCurrentIP().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		IP   string `json:"ip"`
		CIDR string `json:"cidr"`
	}
	if err := json.UnmarshalRead(w.Body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.IP != "2001:db8::44" {
		t.Fatalf("expected ip %q, got %q", "2001:db8::44", resp.IP)
	}
	if resp.CIDR != "2001:db8::44/128" {
		t.Fatalf("expected cidr %q, got %q", "2001:db8::44/128", resp.CIDR)
	}
}

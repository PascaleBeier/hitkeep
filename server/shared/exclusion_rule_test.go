package shared

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeTrafficExclusionRequestSupportsLegacyAndContextRules(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantType string
		want     string
		ok       bool
	}{
		{name: "legacy cidr", body: `{"cidr":"203.0.113.8"}`, wantType: "cidr", want: "203.0.113.8/32", ok: true},
		{name: "country", body: `{"type":"country","country_code":" de "}`, wantType: "country", want: "DE", ok: true},
		{name: "user agent", body: `{"type":"user_agent","user_agent":" HealthCheck/1.0 "}`, wantType: "user_agent", want: "HealthCheck/1.0", ok: true},
		{name: "path", body: `{"type":"path","path":" /admin//users/../?q=1#top "}`, wantType: "path", want: "/admin", ok: true},
		{name: "empty user agent", body: `{"type":"user_agent","user_agent":"  "}`, ok: false},
		{name: "empty path", body: `{"type":"path","path":"?q=1"}`, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/exclusions", strings.NewReader(test.body))
			input, _, _, ok := DecodeTrafficExclusionRequest(req)
			if ok != test.ok {
				t.Fatalf("DecodeTrafficExclusionRequest() ok=%v, want %v", ok, test.ok)
			}
			if !ok {
				return
			}
			if input.Type != test.wantType || input.Label != test.want {
				t.Fatalf("unexpected input: %#v", input)
			}
		})
	}
}

func TestDecodeTrafficExclusionRequestKeepsDescriptionLimit(t *testing.T) {
	req := httptest.NewRequest("POST", "/exclusions", strings.NewReader(`{"cidr":"203.0.113.8","description":"`+strings.Repeat("x", 256)+`"}`))
	_, message, _, ok := DecodeTrafficExclusionRequest(req)
	if ok || message != "Description must be 255 characters or fewer" {
		t.Fatalf("expected description validation, got ok=%v message=%q", ok, message)
	}
}

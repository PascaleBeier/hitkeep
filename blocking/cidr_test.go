package blocking

import "testing"

func TestNormalizeCIDR(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{name: "ipv4", input: "203.0.113.5", expected: "203.0.113.5/32"},
		{name: "ipv6", input: "2001:db8::1", expected: "2001:db8::1/128"},
		{name: "cidr canonicalizes", input: "10.1.2.3/24", expected: "10.1.2.0/24"},
		{name: "invalid", input: "not-an-ip", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normalized, ipNet, err := NormalizeCIDR(tc.input)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize cidr: %v", err)
			}
			if !ipNet.IsValid() {
				t.Fatalf("expected parsed network")
			}
			if normalized != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, normalized)
			}
		})
	}
}

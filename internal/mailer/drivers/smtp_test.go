package drivers

import "testing"

func TestHeloNameFromPublicURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		want      string
	}{
		{
			name:      "https url",
			publicURL: "https://analytics.example.net",
			want:      "analytics.example.net",
		},
		{
			name:      "https url with port and path",
			publicURL: "https://analytics.example.net:8443/app",
			want:      "analytics.example.net",
		},
		{
			name:      "local url",
			publicURL: "http://localhost:8080",
			want:      "localhost",
		},
		{
			name:      "bare hostname",
			publicURL: "analytics.example.net",
			want:      "analytics.example.net",
		},
		{
			name:      "bare host port",
			publicURL: "analytics.example.net:8443",
			want:      "analytics.example.net",
		},
		{
			name:      "empty fallback",
			publicURL: "",
			want:      "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := heloNameFromPublicURL(tt.publicURL); got != tt.want {
				t.Fatalf("heloNameFromPublicURL(%q) = %q, want %q", tt.publicURL, got, tt.want)
			}
		})
	}
}

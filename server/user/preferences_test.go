package user

import "testing"

func TestDefaultPreferencesFromHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "prefers highest quality tag",
			header: "fr-CH, fr;q=0.9, en;q=0.8, de;q=0.7",
			want:   "fr",
		},
		{
			name:   "normalizes region tag to base language",
			header: "EN-us,en;q=0.9",
			want:   "en",
		},
		{
			name:   "falls back for wildcard only",
			header: "*",
			want:   defaultLocaleFallback,
		},
		{
			name:   "falls back for invalid header",
			header: "not a valid header;q=wat",
			want:   defaultLocaleFallback,
		},
		{
			name:   "falls back for empty header",
			header: "",
			want:   defaultLocaleFallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := defaultPreferencesFromHeader(tt.header)
			if got.DefaultLocale != tt.want {
				t.Fatalf("defaultPreferencesFromHeader(%q) = %q, want %q", tt.header, got.DefaultLocale, tt.want)
			}
		})
	}
}

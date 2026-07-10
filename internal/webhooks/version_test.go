package webhooks

import "testing"

func TestMinorAPIVersion(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"2.10.2":        "2.10",
		"v3.4.0":        "3.4",
		"2.11.0-beta.1": "2.11",
		"snapshot":      "0.0",
		"":              "0.0",
	} {
		if got := MinorAPIVersion(input); got != want {
			t.Errorf("MinorAPIVersion(%q)=%q want %q", input, got, want)
		}
	}
}

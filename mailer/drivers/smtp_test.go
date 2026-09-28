package drivers

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestBuildMessageUsesSingleProvidedMessageID(t *testing.T) {
	const messageID = "<report-test.1234@hitkeep>"

	driver := &SMTPDriver{
		from: "noreply@example.com",
		name: "HitKeep",
	}
	msg, err := driver.buildMessage(
		[]string{"recipient@example.com"},
		"Test report",
		"<p>Test report</p>",
		"Test report",
		messageID,
		nil,
	)
	if err != nil {
		t.Fatalf("buildMessage() error = %v", err)
	}

	var serialized bytes.Buffer
	if _, err := msg.WriteTo(&serialized); err != nil {
		t.Fatalf("msg.WriteTo() error = %v", err)
	}

	message := serialized.String()
	messageIDHeaders := regexp.MustCompile(`(?im)^Message-ID:`).FindAllString(message, -1)
	if len(messageIDHeaders) != 1 {
		t.Fatalf("serialized message has %d Message-ID headers, want 1:\n%s", len(messageIDHeaders), message)
	}
	if !strings.Contains(message, "Message-ID: "+messageID) {
		t.Fatalf("serialized message does not contain provided Message-ID %q:\n%s", messageID, message)
	}
}

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

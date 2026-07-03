package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestIsBedrockMantleBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "mantle", url: "https://bedrock-mantle.eu-central-1.api.aws/v1", want: true},
		{name: "mantle without path", url: "https://bedrock-mantle.us-east-1.api.aws", want: true},
		{name: "bedrock runtime", url: "https://bedrock-runtime.eu-central-1.amazonaws.com/openai/v1", want: false},
		{name: "openai", url: "https://api.openai.com/v1", want: false},
		{name: "invalid", url: "://bad-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBedrockMantleBaseURL(tt.url); got != tt.want {
				t.Fatalf("isBedrockMantleBaseURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestBedrockMantleSigV4TransportSignsRequest(t *testing.T) {
	t.Parallel()

	const body = `{"model":"openai.gpt-oss-120b","messages":[{"role":"user","content":"hi"}]}`

	var seenBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		seenBody = string(rawBody)

		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
			t.Fatalf("Authorization = %q, want AWS4-HMAC-SHA256 signature", auth)
		}
		if got := r.Header.Get("X-Amz-Security-Token"); got != "SESSION" {
			t.Fatalf("X-Amz-Security-Token = %q, want SESSION", got)
		}
		if got := r.Header.Get("X-Amz-Date"); got != "20260625T120000Z" {
			t.Fatalf("X-Amz-Date = %q, want fixed signing time", got)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &bedrockMantleSigV4Transport{
		base:   http.DefaultTransport,
		region: "eu-central-1",
		credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			"AKIDEXAMPLE",
			"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
			"SESSION",
		)),
		signer: v4.NewSigner(),
		now: func() time.Time {
			return time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
		},
	}
	client := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_ = resp.Body.Close()

	if seenBody != body {
		t.Fatalf("signed request body = %q, want %q", seenBody, body)
	}
}

func TestBedrockMantleSigV4TransportRequiresRegion(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: &bedrockMantleSigV4Transport{}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://bedrock-mantle.eu-central-1.api.aws/v1/chat/completions", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if _, err := client.Do(req); err == nil || !strings.Contains(err.Error(), "region is not configured") {
		t.Fatalf("error = %v, want region error", err)
	}
}

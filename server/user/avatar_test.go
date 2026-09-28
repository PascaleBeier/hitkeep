package user

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestNewGravatarRequestUsesFixedOrigin(t *testing.T) {
	req, err := newGravatarRequest(context.Background(), "Person@example.com", 128)
	if err != nil {
		t.Fatalf("newGravatarRequest() error = %v", err)
	}

	if req.Method != http.MethodGet {
		t.Fatalf("expected GET method, got %q", req.Method)
	}
	if req.URL.Scheme != "https" {
		t.Fatalf("expected https scheme, got %q", req.URL.Scheme)
	}
	if req.URL.Host != "www.gravatar.com" {
		t.Fatalf("expected gravatar host, got %q", req.URL.Host)
	}
	if req.URL.Path != "/avatar/"+gravatarHash("Person@example.com") {
		t.Fatalf("unexpected avatar path %q", req.URL.Path)
	}
	if got := req.URL.Query().Get("s"); got != "128" {
		t.Fatalf("expected size query to be 128, got %q", got)
	}
	if got := req.URL.Query().Get("d"); got != "mp" {
		t.Fatalf("expected default query to be mp, got %q", got)
	}
}

func TestGravatarErrorKindUsesStableCategories(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "upstream", err: errors.New(`GET "https://www.gravatar.com/avatar/email-hash": provider response email=person@example.com`), want: "upstream_request_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := gravatarErrorKind(test.err); got != test.want {
				t.Fatalf("gravatarErrorKind() = %q, want %q", got, test.want)
			}
		})
	}
}

package mailer

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func TestSendErrorPreservesStageAndCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := wrapSendError(SendStageTransport, cause)

	var sendErr *SendError
	if !errors.As(err, &sendErr) || sendErr.Stage != SendStageTransport {
		t.Fatalf("SendError stage = %#v, want %q", sendErr, SendStageTransport)
	}
	if !errors.Is(err, cause) {
		t.Fatal("SendError did not preserve its cause")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("SendError() = %q, want original cause", err)
	}
}

func TestDescribeErrorRedactsSensitiveDetails(t *testing.T) {
	err := wrapSendError(SendStageTransport, errors.New("smtp 550 rejected user@example.com at https://example.com/reset token=secret-value"))
	details := DescribeError(err)

	if details.Stage != string(SendStageTransport) {
		t.Fatalf("error stage = %q, want %q", details.Stage, SendStageTransport)
	}
	if details.Kind != "permanent_rejection" {
		t.Fatalf("error kind = %q, want permanent_rejection", details.Kind)
	}
	if details.SMTPCode != "550" {
		t.Fatalf("SMTP code = %q, want 550", details.SMTPCode)
	}
	for _, secret := range []string{"user@example.com", "https://example.com/reset", "secret-value"} {
		if strings.Contains(details.Message, secret) {
			t.Fatalf("redacted message contains %q: %q", secret, details.Message)
		}
	}
	for _, marker := range []string{"[redacted-email]", "[redacted-url]", "token=[redacted]"} {
		if !strings.Contains(details.Message, marker) {
			t.Fatalf("redacted message missing %q: %q", marker, details.Message)
		}
	}
}

func TestDescribeErrorClassifiesTimeoutAndDisabled(t *testing.T) {
	timeout := &net.DNSError{Err: "i/o timeout", Name: "smtp.example.com", IsTimeout: true}
	timeoutDetails := DescribeError(wrapSendError(SendStageTransport, timeout))
	if timeoutDetails.Kind != "timeout" {
		t.Fatalf("timeout kind = %q, want timeout", timeoutDetails.Kind)
	}
	if timeoutDetails.Stage != string(SendStageTransport) {
		t.Fatalf("timeout stage = %q, want %q", timeoutDetails.Stage, SendStageTransport)
	}

	disabledDetails := DescribeError(ErrMailerDisabled)
	if disabledDetails.Stage != string(SendStageConfiguration) || disabledDetails.Kind != "disabled" {
		t.Fatalf("disabled details = %+v", disabledDetails)
	}
}

func TestDescribeErrorClassifiesRenderFailure(t *testing.T) {
	details := DescribeError(wrapSendError(SendStageMJMLRender, errors.New("invalid MJML with report body")))
	if details.Stage != string(SendStageMJMLRender) || details.Kind != "render" {
		t.Fatalf("render details = %+v", details)
	}
	if details.Message != "mail content rendering failed" {
		t.Fatalf("render message = %q, want generic safe message", details.Message)
	}
}

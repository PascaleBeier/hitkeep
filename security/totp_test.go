package security

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidateTOTPCode(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error = %v", err)
	}

	now := time.Unix(1_735_000_000, 0).UTC()
	code, err := GenerateCurrentTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateCurrentTOTPCode() error = %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected code length 6, got %d", len(code))
	}

	if !ValidateTOTPCode(secret, code, now) {
		t.Fatalf("expected code to validate")
	}
	if ValidateTOTPCode(secret, "123456", now) && code != "123456" {
		t.Fatalf("unexpected validation for invalid code")
	}
}

func TestValidateTOTPCodeStrictRejectsAdjacentSteps(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_735_000_000, 0).UTC()

	currentCode, err := GenerateCurrentTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateCurrentTOTPCode(current) error = %v", err)
	}
	previousCode, err := GenerateCurrentTOTPCode(secret, now.Add(-totpPeriod*time.Second))
	if err != nil {
		t.Fatalf("GenerateCurrentTOTPCode(previous) error = %v", err)
	}
	nextCode, err := GenerateCurrentTOTPCode(secret, now.Add(totpPeriod*time.Second))
	if err != nil {
		t.Fatalf("GenerateCurrentTOTPCode(next) error = %v", err)
	}

	if !ValidateTOTPCodeStrict(secret, currentCode, now) {
		t.Fatalf("expected strict validation to accept current code")
	}
	if ValidateTOTPCodeStrict(secret, previousCode, now) {
		t.Fatalf("expected strict validation to reject previous-step code")
	}
	if ValidateTOTPCodeStrict(secret, nextCode, now) {
		t.Fatalf("expected strict validation to reject next-step code")
	}

	// Backward compatibility: lenient validation keeps accepting adjacent steps.
	if !ValidateTOTPCode(secret, previousCode, now) {
		t.Fatalf("expected lenient validation to accept previous-step code")
	}
	if !ValidateTOTPCode(secret, nextCode, now) {
		t.Fatalf("expected lenient validation to accept next-step code")
	}
}

func TestBuildOTPAuthURL(t *testing.T) {
	uri := BuildOTPAuthURL("HitKeep", "user@example.com", "ABCDEF")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("expected otpauth prefix, got %q", uri)
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("failed to parse otp auth url: %v", err)
	}
	if parsed.Query().Get("secret") != "ABCDEF" {
		t.Fatalf("expected secret query parameter")
	}
	if parsed.Query().Get("issuer") != "HitKeep" {
		t.Fatalf("expected issuer query parameter")
	}
}

func TestGenerateRandomChallenge(t *testing.T) {
	challenge, err := GenerateRandomChallenge(32)
	if err != nil {
		t.Fatalf("GenerateRandomChallenge() error = %v", err)
	}
	if strings.TrimSpace(challenge) == "" {
		t.Fatalf("expected non-empty challenge")
	}
}

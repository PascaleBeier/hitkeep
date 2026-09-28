package sso

import (
	"errors"
	"strings"
	"testing"
)

func TestSecretBoxRoundTrip(t *testing.T) {
	box, err := NewSecretBox("instance-secret")
	if err != nil {
		t.Fatalf("create secret box: %v", err)
	}

	sealed, err := box.Seal("oidc-client-secret")
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	if sealed == "oidc-client-secret" || strings.Contains(sealed, "oidc-client-secret") {
		t.Fatal("ciphertext exposed plaintext secret")
	}

	opened, err := box.Open(sealed)
	if err != nil {
		t.Fatalf("open secret: %v", err)
	}
	if opened != "oidc-client-secret" {
		t.Fatalf("unexpected plaintext %q", opened)
	}
}

func TestSecretBoxRejectsWrongInstanceSecret(t *testing.T) {
	first, _ := NewSecretBox("first-instance-secret")
	second, _ := NewSecretBox("second-instance-secret")
	sealed, _ := first.Seal("oidc-client-secret")

	if _, err := second.Open(sealed); !errors.Is(err, ErrInvalidSecretCiphertext) {
		t.Fatalf("expected invalid ciphertext error, got %v", err)
	}
}

func TestSecretBoxRequiresInstanceSecret(t *testing.T) {
	if _, err := NewSecretBox("  "); err == nil {
		t.Fatal("expected empty instance secret to fail")
	}
}

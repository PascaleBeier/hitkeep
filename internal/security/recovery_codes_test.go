package security

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}
	if len(codes) != recoveryCodeCount {
		t.Fatalf("expected %d recovery codes, got %d", recoveryCodeCount, len(codes))
	}

	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if len(code) != recoveryCodeRawLength+1 {
			t.Fatalf("unexpected code length for %q", code)
		}
		if code[recoveryCodeGroupLength] != '-' {
			t.Fatalf("expected hyphen in code %q", code)
		}
		if NormalizeRecoveryCode(code) == "" {
			t.Fatalf("expected generated code %q to normalize", code)
		}
		if _, exists := seen[code]; exists {
			t.Fatalf("expected unique recovery code, duplicate %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestNormalizeAndHashRecoveryCode(t *testing.T) {
	const code = "ABCD-EFGH"

	if normalized := NormalizeRecoveryCode(" abcd efgh "); normalized != "ABCDEFGH" {
		t.Fatalf("unexpected normalized code %q", normalized)
	}
	if NormalizeRecoveryCode("1234-5678") != "" {
		t.Fatal("expected invalid recovery code to normalize to empty string")
	}
	hash, err := HashRecoveryCode(code)
	if err != nil {
		t.Fatalf("HashRecoveryCode() error = %v", err)
	}
	if hash == "" {
		t.Fatal("expected hashed recovery code")
	}
	match, err := VerifyRecoveryCode("abcd efgh", hash)
	if err != nil {
		t.Fatalf("VerifyRecoveryCode() error = %v", err)
	}
	if !match {
		t.Fatal("expected equivalent recovery codes to verify")
	}
	mismatch, err := VerifyRecoveryCode("WXYZ-QRST", hash)
	if err != nil {
		t.Fatalf("VerifyRecoveryCode() mismatch error = %v", err)
	}
	if mismatch {
		t.Fatal("expected mismatched recovery code to fail verification")
	}
}

func TestVerifyRecoveryCodeRejectsUnexpectedHashLength(t *testing.T) {
	hash, err := HashRecoveryCode("ABCD-EFGH")
	if err != nil {
		t.Fatalf("HashRecoveryCode() error = %v", err)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected encoded hash with 6 parts, got %d", len(parts))
	}
	parts[5] = parts[5][:len(parts[5])-1]
	invalidHash := fmt.Sprintf("$%s", strings.Join(parts[1:], "$"))
	match, err := VerifyRecoveryCode("ABCD-EFGH", invalidHash)
	if err == nil {
		t.Fatal("expected invalid hash length error")
	}
	if match {
		t.Fatal("expected invalid hash length to fail verification")
	}
}

func TestVerifyRecoveryCodeRejectsNonCanonicalParameters(t *testing.T) {
	hash, err := HashRecoveryCode("ABCD-EFGH")
	if err != nil {
		t.Fatalf("HashRecoveryCode() error = %v", err)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected encoded hash with 6 parts, got %d", len(parts))
	}
	parts[3] = "m=1,t=1,p=4"
	invalidHash := fmt.Sprintf("$%s", strings.Join(parts[1:], "$"))
	match, err := VerifyRecoveryCode("ABCD-EFGH", invalidHash)
	if err == nil {
		t.Fatal("expected non-canonical parameters error")
	}
	if match {
		t.Fatal("expected non-canonical parameters to fail verification")
	}
}

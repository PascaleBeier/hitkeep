package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestVerifyRecoveryCodeRejectsNonCanonicalEncoding(t *testing.T) {
	code, err := generateRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashRecoveryCode(code)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(hash, "$")
	tests := []struct {
		name   string
		mutate func([]string)
	}{
		{"version trailing junk", func(parts []string) { parts[2] += ",junk" }},
		{"parameters trailing junk", func(parts []string) { parts[3] += ",junk" }},
		{"oversized salt", func(parts []string) { parts[4] = strings.Repeat("A", base64.RawStdEncoding.EncodedLen(16)+1) }},
		{"oversized hash", func(parts []string) {
			parts[5] = strings.Repeat("A", base64.RawStdEncoding.EncodedLen(int(recoveryCodeHashKeyLen))+1)
		}},
		{"wrong salt length", func(parts []string) { parts[4] = base64.RawStdEncoding.EncodeToString(make([]byte, 15)) }},
		{"wrong hash length", func(parts []string) {
			parts[5] = base64.RawStdEncoding.EncodeToString(make([]byte, recoveryCodeHashKeyLen-1))
		}},
		{"delimiter-heavy hash", func(parts []string) { parts[4] = strings.Repeat("$", 1024) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := append([]string(nil), parts...)
			test.mutate(candidate)
			if _, err := VerifyRecoveryCode(code, strings.Join(candidate, "$")); err == nil {
				t.Fatal("VerifyRecoveryCode accepted a noncanonical hash")
			}
		})
	}
}

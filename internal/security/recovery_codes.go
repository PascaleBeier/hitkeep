package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	recoveryCodeCount       = 10
	recoveryCodeRawLength   = 8
	recoveryCodeGroupLength = 4
)

var recoveryCodeAlphabet = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

const (
	recoveryCodeHashTime    uint32 = 1
	recoveryCodeHashMemory  uint32 = 64 * 1024
	recoveryCodeHashKeyLen  uint32 = 32
	recoveryCodeHashSaltLen        = 16
	recoveryCodeHashThreads uint8  = 4
)

func GenerateRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	seen := make(map[string]struct{}, recoveryCodeCount)

	for len(codes) < recoveryCodeCount {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, err
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}

	return codes, nil
}

func NormalizeRecoveryCode(code string) string {
	replacer := strings.NewReplacer("-", "", " ", "", "\t", "", "\n", "", "\r", "")
	code = strings.ToUpper(replacer.Replace(strings.TrimSpace(code)))
	if len(code) != recoveryCodeRawLength {
		return ""
	}
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '2' && r <= '9':
		default:
			return ""
		}
	}
	return code
}

func HashRecoveryCode(code string) (string, error) {
	normalized := NormalizeRecoveryCode(code)
	if normalized == "" {
		return "", nil
	}
	salt := make([]byte, recoveryCodeHashSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("could not generate recovery code salt: %w", err)
	}
	hash := argon2.IDKey([]byte(normalized), salt, recoveryCodeHashTime, recoveryCodeHashMemory, recoveryCodeHashThreads, recoveryCodeHashKeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, recoveryCodeHashMemory, recoveryCodeHashTime, recoveryCodeHashThreads, b64Salt, b64Hash), nil
}

func VerifyRecoveryCode(code, encodedHash string) (bool, error) {
	normalized := NormalizeRecoveryCode(code)
	if normalized == "" {
		return false, nil
	}

	// Recovery-code hashes have one exact canonical encoding. Check it before
	// splitting so corrupted persisted input cannot allocate by delimiter count.
	const canonicalEncodedHashLength = 97
	if len(encodedHash) != canonicalEncodedHashLength {
		return false, errors.New("invalid hash format")
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}
	if parts[1] != "argon2id" {
		return false, errors.New("incompatible variant")
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errors.New("incompatible version")
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", recoveryCodeHashMemory, recoveryCodeHashTime, recoveryCodeHashThreads) {
		return false, errors.New("incompatible parameters")
	}

	const saltLength = 16
	if len(parts[4]) != base64.RawStdEncoding.EncodedLen(saltLength) {
		return false, errors.New("invalid salt encoding")
	}
	if len(parts[5]) != base64.RawStdEncoding.EncodedLen(int(recoveryCodeHashKeyLen)) {
		return false, errors.New("invalid hash encoding")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	if len(salt) != saltLength || base64.RawStdEncoding.EncodeToString(salt) != parts[4] {
		return false, errors.New("invalid salt")
	}
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	if len(decodedHash) != int(recoveryCodeHashKeyLen) || base64.RawStdEncoding.EncodeToString(decodedHash) != parts[5] {
		return false, errors.New("invalid hash")
	}

	comparisonHash := argon2.IDKey([]byte(normalized), salt, recoveryCodeHashTime, recoveryCodeHashMemory, recoveryCodeHashThreads, recoveryCodeHashKeyLen)
	return subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1, nil
}

func generateRecoveryCode() (string, error) {
	random := make([]byte, recoveryCodeRawLength)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("could not generate recovery code: %w", err)
	}

	out := make([]byte, 0, recoveryCodeRawLength+1)
	for idx, value := range random {
		if idx == recoveryCodeGroupLength {
			out = append(out, '-')
		}
		out = append(out, recoveryCodeAlphabet[int(value)%len(recoveryCodeAlphabet)])
	}

	return string(out), nil
}

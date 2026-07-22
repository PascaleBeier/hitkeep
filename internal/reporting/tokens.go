package reporting

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

const ConfirmationTokenBytes = 32

func NewConfirmationToken() (string, error) {
	raw := make([]byte, ConfirmationTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ConfirmationTokenHash(token string) string {
	return UnsubscribeTokenHash(token)
}

func UnsubscribeToken(secret string, reportID, recipientID uuid.UUID) string {
	payload := reportID.String() + "." + recipientID.String()
	signature := hmac.New(sha256.New, []byte(secret))
	_, _ = signature.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
}

func UnsubscribeTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func VerifyUnsubscribeToken(secret, token string) (uuid.UUID, uuid.UUID, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return uuid.Nil, uuid.Nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	expected := hmac.New(sha256.New, []byte(secret))
	_, _ = expected.Write(payload)
	if !hmac.Equal(signature, expected.Sum(nil)) {
		return uuid.Nil, uuid.Nil, false
	}
	ids := strings.Split(string(payload), ".")
	if len(ids) != 2 {
		return uuid.Nil, uuid.Nil, false
	}
	reportID, err := uuid.Parse(ids[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	recipientID, err := uuid.Parse(ids[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return reportID, recipientID, true
}

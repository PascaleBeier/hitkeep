package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const secretCiphertextPrefix = "v1."

var ErrInvalidSecretCiphertext = errors.New("invalid SSO secret ciphertext")

// SecretBox encrypts team SSO client secrets with a purpose-specific key
// derived from the instance JWT secret. Ciphertexts are versioned so the
// storage format can be migrated without exposing plaintext credentials.
type SecretBox struct {
	aead cipher.AEAD
}

func NewSecretBox(masterSecret string) (*SecretBox, error) {
	masterSecret = strings.TrimSpace(masterSecret)
	if masterSecret == "" {
		return nil, errors.New("instance secret is required")
	}

	mac := hmac.New(sha256.New, []byte(masterSecret))
	_, _ = mac.Write([]byte("hitkeep/team-sso/client-secret/v1"))
	key := mac.Sum(nil)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create SSO secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create SSO secret AEAD: %w", err)
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Seal(plaintext string) (string, error) {
	if b == nil || b.aead == nil {
		return "", errors.New("SSO secret cipher is unavailable")
	}
	if strings.TrimSpace(plaintext) == "" {
		return "", errors.New("SSO client secret is required")
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate SSO secret nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return secretCiphertextPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (b *SecretBox) Open(ciphertext string) (string, error) {
	if b == nil || b.aead == nil {
		return "", errors.New("SSO secret cipher is unavailable")
	}
	if !strings.HasPrefix(ciphertext, secretCiphertextPrefix) {
		return "", ErrInvalidSecretCiphertext
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, secretCiphertextPrefix))
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", ErrInvalidSecretCiphertext
	}
	nonce, sealed := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", ErrInvalidSecretCiphertext
	}
	return string(plaintext), nil
}

package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestValidateTokenClaimsAcceptsOnlyHS256(t *testing.T) {
	const (
		secret = "test-secret"
		issuer = "https://example.test"
	)

	userID := uuid.New()
	hs256Token, _, err := GenerateTokenWithDuration(secret, issuer, userID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateTokenClaims(hs256Token, secret, issuer)
	if err != nil {
		t.Fatalf("ValidateTokenClaims() error = %v", err)
	}
	if claims == nil || claims.UserID != userID {
		t.Fatalf("ValidateTokenClaims() claims = %#v, want user %s", claims, userID)
	}

	for _, method := range []*jwt.SigningMethodHMAC{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		t.Run(method.Alg(), func(t *testing.T) {
			now := time.Now()
			token := jwt.NewWithClaims(method, Claims{
				UserID:    userID,
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(now),
				Issuer:    issuer,
				Audience:  jwt.ClaimStrings{issuer},
			})
			tokenString, err := token.SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}

			claims, err := ValidateTokenClaims(tokenString, secret, issuer)
			if err == nil {
				t.Fatal("ValidateTokenClaims() error = nil, want rejected signing method")
			}
			if claims != nil {
				t.Fatalf("ValidateTokenClaims() claims = %#v, want nil", claims)
			}
			if strings.Contains(err.Error(), tokenString) {
				t.Fatal("ValidateTokenClaims() error leaked token")
			}
		})
	}
}

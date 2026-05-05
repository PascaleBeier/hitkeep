package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	CookieName           = "hk_token"
	RememberMeCookieName = "hk_remember_me"
	TokenDuration        = 15 * time.Minute
	RememberMeDuration   = 30 * 24 * time.Hour
)

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the given user ID.
// issuer is used for both the 'iss' and 'aud' claims.
func GenerateToken(secret string, issuer string, userID uuid.UUID) (string, error) {
	token, _, err := GenerateTokenWithDuration(secret, issuer, userID, TokenDuration)
	return token, err
}

func GenerateTokenWithDuration(secret string, issuer string, userID uuid.UUID, duration time.Duration) (string, time.Time, error) {
	if duration <= 0 {
		duration = TokenDuration
	}
	now := time.Now()
	expiresAt := now.Add(duration)
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{issuer},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func ValidateToken(tokenString string, secret string, issuer string) (uuid.UUID, error) {
	claims, err := ValidateTokenClaims(tokenString, secret, issuer)
	if err != nil {
		return uuid.Nil, err
	}
	return claims.UserID, nil
}

func ValidateTokenClaims(tokenString string, secret string, issuer string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// Validate the algorithm is what we expect
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	},
		// Validate Audience and Issuer match our Public URL
		jwt.WithAudience(issuer),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// SetTokenCookie attaches the JWT to the response as an HTTP-only cookie.
func SetTokenCookie(w http.ResponseWriter, token string, secure bool) {
	SetTokenCookieWithDuration(w, token, secure, TokenDuration)
}

func SetTokenCookieWithDuration(w http.ResponseWriter, token string, secure bool, duration time.Duration) {
	if duration <= 0 {
		duration = TokenDuration
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set from the configured public URL; local HTTP dev intentionally uses insecure cookies.
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(duration),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetRememberMeCookie attaches the remember me token to the response as an HTTP-only cookie.
func SetRememberMeCookie(w http.ResponseWriter, token string, secure bool) {
	SetRememberMeCookieWithDuration(w, token, secure, RememberMeDuration)
}

func SetRememberMeCookieWithDuration(w http.ResponseWriter, token string, secure bool, duration time.Duration) {
	if duration <= 0 {
		duration = RememberMeDuration
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set from the configured public URL; local HTTP dev intentionally uses insecure cookies.
		Name:     RememberMeCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(duration),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookies removes both auth and remember me cookies.
func ClearCookies(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set from the configured public URL; local HTTP dev intentionally uses insecure cookies.
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set from the configured public URL; local HTTP dev intentionally uses insecure cookies.
		Name:     RememberMeCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

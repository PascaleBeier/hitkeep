package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp"
	otptotp "github.com/pquerna/otp/totp"
)

const (
	totpDigits             = 6
	totpPeriod             = 30
	totpSecretBytes        = 20
	defaultTOTPIssuer      = "HitKeep"
	defaultTOTPAccountName = "account"
)

func GenerateTOTPSecret() (string, error) {
	key, err := otptotp.Generate(otptotp.GenerateOpts{
		Issuer:      defaultTOTPIssuer,
		AccountName: defaultTOTPAccountName,
		Period:      uint(totpPeriod),
		SecretSize:  uint(totpSecretBytes),
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
		Rand:        rand.Reader,
	})
	if err != nil {
		return "", fmt.Errorf("could not generate totp secret: %w", err)
	}
	return key.Secret(), nil
}

func BuildOTPAuthURL(issuer string, account string, secret string) string {
	label := strings.TrimSpace(account)
	if label == "" {
		label = defaultTOTPAccountName
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = defaultTOTPIssuer
	}

	encodedLabel := url.PathEscape(fmt.Sprintf("%s:%s", issuer, label))
	values := url.Values{
		"secret":    {strings.TrimSpace(secret)},
		"issuer":    {issuer},
		"algorithm": {"SHA1"},
		"digits":    {strconv.Itoa(totpDigits)},
		"period":    {strconv.Itoa(totpPeriod)},
	}

	return "otpauth://totp/" + encodedLabel + "?" + values.Encode()
}

func ValidateTOTPCode(secret string, code string, now time.Time) bool {
	return ValidateTOTPCodeWithWindow(secret, code, now, 1, 1)
}

func ValidateTOTPCodeStrict(secret string, code string, now time.Time) bool {
	return ValidateTOTPCodeWithWindow(secret, code, now, 0, 0)
}

func ValidateTOTPCodeWithWindow(secret string, code string, now time.Time, pastWindow int, futureWindow int) bool {
	normalizedCode := normalizeTOTPCode(code)
	if normalizedCode == "" {
		return false
	}

	normalizedSecret := strings.TrimSpace(strings.ToUpper(secret))
	if normalizedSecret == "" {
		return false
	}

	if pastWindow < 0 {
		pastWindow = 0
	}
	if futureWindow < 0 {
		futureWindow = 0
	}

	now = now.UTC()
	if pastWindow == futureWindow {
		valid, err := otptotp.ValidateCustom(normalizedCode, normalizedSecret, now, totpValidateOptions(uint(pastWindow)))
		return err == nil && valid
	}

	opts := totpValidateOptions(0)
	for delta := -pastWindow; delta <= futureWindow; delta++ {
		valid, err := otptotp.ValidateCustom(
			normalizedCode,
			normalizedSecret,
			now.Add(time.Duration(delta*totpPeriod)*time.Second),
			opts,
		)
		if err != nil {
			return false
		}
		if valid {
			return true
		}
	}
	return false
}

func GenerateCurrentTOTPCode(secret string, now time.Time) (string, error) {
	normalizedSecret := strings.TrimSpace(strings.ToUpper(secret))
	if normalizedSecret == "" {
		return "", fmt.Errorf("invalid totp secret")
	}
	if now.UTC().Unix() < 0 {
		return "", fmt.Errorf("invalid time for totp generation")
	}

	code, err := otptotp.GenerateCodeCustom(normalizedSecret, now.UTC(), totpValidateOptions(0))
	if err != nil {
		return "", fmt.Errorf("invalid totp secret")
	}
	return code, nil
}

func GenerateRandomChallenge(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("could not generate random challenge: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeTOTPCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return ""
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return code
}

func totpValidateOptions(skew uint) otptotp.ValidateOpts {
	return otptotp.ValidateOpts{
		Period:    uint(totpPeriod),
		Skew:      skew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
}

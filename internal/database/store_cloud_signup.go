//go:build billing

package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	pendingSignupTTL                        = 24 * time.Hour
	PendingSignupVerificationResendCooldown = 5 * time.Minute
)

var (
	ErrPendingSignupInvalid           = errors.New("invalid or unknown signup verification token")
	ErrPendingSignupExpired           = errors.New("signup verification token has expired")
	ErrPendingSignupResendUnavailable = errors.New("pending signup verification resend is unavailable")
	pendingSignupResendMu             sync.Mutex
)

type PreparedPendingSignupVerification struct {
	Token    string
	Email    string
	TeamName string
	Locale   string
}

func (s *Store) CreatePendingSignup(ctx context.Context, entry PendingSignupEntry) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)
	now := time.Now().UTC()
	entry.ExpiresAt = now.Add(pendingSignupTTL)
	entry.PlanCode = strings.TrimSpace(strings.ToLower(entry.PlanCode))
	if entry.PlanCode == "" {
		entry.PlanCode = CloudPlanFree
	}
	entry.BillingInterval = normalizeCloudBillingInterval(entry.BillingInterval)

	// Remove any existing pending signup for this email.
	normalizedEmail := strings.TrimSpace(strings.ToLower(entry.Email))
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM pending_signups WHERE LOWER(TRIM(email)) = ?",
		normalizedEmail,
	); err != nil {
		return "", fmt.Errorf("clear previous pending signup: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO pending_signups (token, email, hashed_password, given_name, last_name, team_name, jurisdiction, locale, plan_code, billing_interval, accepted_tos_at, expires_at, verification_sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token, entry.Email, entry.HashedPassword, entry.GivenName, entry.LastName,
		entry.TeamName, entry.Jurisdiction, entry.Locale, entry.PlanCode, entry.BillingInterval,
		sql.NullTime{Time: entry.AcceptedTosAt, Valid: !entry.AcceptedTosAt.IsZero()}, entry.ExpiresAt, now,
	); err != nil {
		return "", fmt.Errorf("insert pending signup: %w", err)
	}

	return token, nil
}

func (s *Store) PreparePendingSignupVerificationResend(ctx context.Context, email string, now time.Time) (*PreparedPendingSignupVerification, error) {
	pendingSignupResendMu.Lock()
	defer pendingSignupResendMu.Unlock()

	normalizedEmail := strings.TrimSpace(strings.ToLower(email))
	if normalizedEmail == "" {
		return nil, ErrPendingSignupResendUnavailable
	}
	now = now.UTC()

	var prepared PreparedPendingSignupVerification
	err := s.db.QueryRowContext(ctx, `
		UPDATE pending_signups
		SET verification_sent_at = ?
		WHERE token = (
			SELECT token
			FROM pending_signups
			WHERE LOWER(TRIM(email)) = ?
			  AND expires_at > ?
			  AND COALESCE(verification_sent_at, created_at) <= ?
			ORDER BY created_at DESC
			LIMIT 1
		)
		RETURNING token, email, team_name, locale
	`, now, normalizedEmail, now, now.Add(-PendingSignupVerificationResendCooldown)).Scan(
		&prepared.Token,
		&prepared.Email,
		&prepared.TeamName,
		&prepared.Locale,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPendingSignupResendUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("prepare pending signup verification resend: %w", err)
	}
	return &prepared, nil
}

func (s *Store) CompletePendingSignup(ctx context.Context, token string) (*PendingSignupEntry, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrPendingSignupInvalid
	}

	var entry PendingSignupEntry
	var acceptedTosAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT email, hashed_password, given_name, last_name, team_name, jurisdiction, locale, plan_code, billing_interval, accepted_tos_at, expires_at
		FROM pending_signups WHERE token = ?`, token,
	).Scan(&entry.Email, &entry.HashedPassword, &entry.GivenName, &entry.LastName,
		&entry.TeamName, &entry.Jurisdiction, &entry.Locale, &entry.PlanCode, &entry.BillingInterval, &acceptedTosAt, &entry.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPendingSignupInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("query pending signup: %w", err)
	}
	if acceptedTosAt.Valid {
		entry.AcceptedTosAt = acceptedTosAt.Time
	}

	if !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt.UTC()) {
		// Clean up expired token.
		_, _ = s.db.ExecContext(ctx, "DELETE FROM pending_signups WHERE token = ?", token)
		return nil, ErrPendingSignupExpired
	}

	// Consume the token.
	if _, err := s.db.ExecContext(ctx, "DELETE FROM pending_signups WHERE token = ?", token); err != nil {
		return nil, fmt.Errorf("consume pending signup token: %w", err)
	}

	return &entry, nil
}

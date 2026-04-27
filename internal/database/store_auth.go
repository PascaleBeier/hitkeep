package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrPasswordResetInvalid = errors.New("invalid or expired token")
var ErrPasswordResetExpired = errors.New("token expired")

// CreatePasswordResetToken generates a secure token, saves it, and returns it.
func (s *Store) CreatePasswordResetToken(ctx context.Context, email string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)
	expiresAt := time.Now().UTC().Add(passwordResetTTL)
	s.storePasswordResetToken(email, token, expiresAt)
	return token, nil
}

// CompletePasswordReset verifies a token and updates the password in a single transaction.
func (s *Store) CompletePasswordReset(ctx context.Context, token string, newHashedPassword string) error {
	entry, found, err := s.lookupPasswordResetToken(token, true)
	if err != nil {
		return err
	}
	if !found {
		return ErrPasswordResetInvalid
	}
	email := entry.Email

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET password = ? WHERE email = ?", newHashedPassword, email)
		if err != nil {
			return err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return fmt.Errorf("user not found")
		}

		_, err = tx.ExecContext(ctx, "DELETE FROM remember_me_tokens WHERE user_id = (SELECT id FROM users WHERE email = ?)", email)
		return err
	})
	if err != nil {
		if time.Now().UTC().Before(entry.ExpiresAt.UTC()) {
			s.storePasswordResetToken(entry.Email, strings.TrimSpace(token), entry.ExpiresAt)
		}
		return err
	}
	return nil
}

func (s *Store) ResolvePasswordResetEmail(ctx context.Context, token string) (string, error) {
	entry, found, err := s.lookupPasswordResetToken(token, false)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrPasswordResetInvalid
	}
	return entry.Email, nil
}

func (s *Store) UpdatePasswordByID(ctx context.Context, userID string, newHashedPassword string) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET password = ? WHERE id = ?", newHashedPassword, userID); err != nil {
			return err
		}
		// Wipe active sessions
		if _, err := tx.ExecContext(ctx, "DELETE FROM remember_me_tokens WHERE user_id = ?", userID); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) CreateRememberMeToken(ctx context.Context, userID uuid.UUID) (string, error) {
	token, _, err := s.CreateRememberMeSession(ctx, userID)
	return token, err
}

func (s *Store) CreateRememberMeSession(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	return s.CreateRememberMeSessionWithDuration(ctx, userID, 30*24*time.Hour)
}

func (s *Store) CreateRememberMeTokenWithDuration(ctx context.Context, userID uuid.UUID, duration time.Duration) (string, error) {
	token, _, err := s.CreateRememberMeSessionWithDuration(ctx, userID, duration)
	return token, err
}

func (s *Store) CreateRememberMeSessionWithDuration(ctx context.Context, userID uuid.UUID, duration time.Duration) (string, time.Time, error) {
	if duration <= 0 {
		duration = 30 * 24 * time.Hour
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(bytes)
	now := time.Now().UTC()
	expiresAt := now.Add(duration)

	err := s.Exec(ctx,
		"INSERT INTO remember_me_tokens (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		token, userID, now, expiresAt,
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to insert remember me token: %w", err)
	}
	return token, expiresAt.UTC(), nil
}

func (s *Store) ValidateRememberMeToken(ctx context.Context, token string) (uuid.UUID, error) {
	userID, _, err := s.ValidateRememberMeSession(ctx, token)
	return userID, err
}

func (s *Store) ValidateRememberMeSession(ctx context.Context, token string) (uuid.UUID, time.Time, error) {
	var userID uuid.UUID
	var expiresAt time.Time

	err := s.QueryRowOrNil(ctx,
		"SELECT user_id, expires_at FROM remember_me_tokens WHERE token = ?",
		[]any{&userID, &expiresAt},
		token,
	)
	if err != nil {
		return uuid.Nil, time.Time{}, err
	}
	if userID == uuid.Nil {
		return uuid.Nil, time.Time{}, nil
	}

	if time.Now().After(expiresAt) {
		_ = s.DeleteRememberMeToken(ctx, token)
		return uuid.Nil, time.Time{}, nil
	}
	return userID, expiresAt.UTC(), nil
}

func (s *Store) DeleteRememberMeToken(ctx context.Context, token string) error {
	return s.Exec(ctx, "DELETE FROM remember_me_tokens WHERE token = ?", token)
}

func (s *Store) DeleteAllRememberMeTokensForUser(ctx context.Context, userID uuid.UUID) error {
	return s.Exec(ctx, "DELETE FROM remember_me_tokens WHERE user_id = ?", userID)
}

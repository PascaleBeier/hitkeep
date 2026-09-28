//go:build billing

package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPreparePendingSignupVerificationResendReusesEligibleSignup(t *testing.T) {
	store := newSharedTestFixtureStore(t)
	ctx := context.Background()
	token, err := store.CreatePendingSignup(ctx, PendingSignupEntry{
		Email:          "Mixed.Case@Example.com",
		HashedPassword: "hashed",
		TeamName:       "Test Team",
		Locale:         "de",
	})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}

	var sentAt, expiresAt time.Time
	if err := store.DB().QueryRowContext(ctx, `
		SELECT verification_sent_at, expires_at
		FROM pending_signups
		WHERE token = ?
	`, token).Scan(&sentAt, &expiresAt); err != nil {
		t.Fatalf("read pending signup timestamps: %v", err)
	}

	now := sentAt.UTC().Add(PendingSignupVerificationResendCooldown)
	prepared, err := store.PreparePendingSignupVerificationResend(ctx, "  mixed.case@example.com  ", now)
	if err != nil {
		t.Fatalf("prepare verification resend: %v", err)
	}
	if prepared.Token != token || prepared.Email != "Mixed.Case@Example.com" {
		t.Fatalf("prepared signup = %+v, want original token and email", prepared)
	}
	if prepared.TeamName != "Test Team" || prepared.Locale != "de" {
		t.Fatalf("prepared signup metadata = %+v", prepared)
	}

	var gotSentAt, gotExpiresAt time.Time
	if err := store.DB().QueryRowContext(ctx, `
		SELECT verification_sent_at, expires_at
		FROM pending_signups
		WHERE token = ?
	`, token).Scan(&gotSentAt, &gotExpiresAt); err != nil {
		t.Fatalf("read updated pending signup timestamps: %v", err)
	}
	if !gotSentAt.Equal(now) {
		t.Fatalf("verification_sent_at = %s, want %s", gotSentAt, now)
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at changed from %s to %s", expiresAt, gotExpiresAt)
	}
}

func TestPreparePendingSignupVerificationResendUsesCreatedAtForLegacyRows(t *testing.T) {
	store := newSharedTestFixtureStore(t)
	ctx := context.Background()
	token, err := store.CreatePendingSignup(ctx, PendingSignupEntry{Email: "legacy@example.com", HashedPassword: "hashed"})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}

	createdAt := time.Now().UTC().Add(-PendingSignupVerificationResendCooldown)
	if _, err := store.DB().ExecContext(ctx, `
		UPDATE pending_signups
		SET created_at = ?, verification_sent_at = NULL
		WHERE token = ?
	`, createdAt, token); err != nil {
		t.Fatalf("prepare legacy pending signup: %v", err)
	}

	prepared, err := store.PreparePendingSignupVerificationResend(ctx, "legacy@example.com", createdAt.Add(PendingSignupVerificationResendCooldown))
	if err != nil {
		t.Fatalf("prepare legacy verification resend: %v", err)
	}
	if prepared.Token != token {
		t.Fatalf("token = %q, want %q", prepared.Token, token)
	}
}

func TestPreparePendingSignupVerificationResendRejectsUnavailableSignup(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, store *Store, token string, now time.Time)
		email   string
	}{
		{name: "unknown", email: "unknown@example.com"},
		{
			name:  "cooldown",
			email: "pending@example.com",
			prepare: func(t *testing.T, store *Store, token string, now time.Time) {
				t.Helper()
				if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET verification_sent_at = ? WHERE token = ?`, now.Add(-PendingSignupVerificationResendCooldown).Add(time.Second), token); err != nil {
					t.Fatalf("set cooldown timestamp: %v", err)
				}
			},
		},
		{
			name:  "expired",
			email: "pending@example.com",
			prepare: func(t *testing.T, store *Store, token string, now time.Time) {
				t.Helper()
				if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET expires_at = ? WHERE token = ?`, now.Add(-time.Second), token); err != nil {
					t.Fatalf("expire pending signup: %v", err)
				}
			},
		},
		{
			name:  "consumed",
			email: "pending@example.com",
			prepare: func(t *testing.T, store *Store, token string, _ time.Time) {
				t.Helper()
				if _, err := store.CompletePendingSignup(context.Background(), token); err != nil {
					t.Fatalf("consume pending signup: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newSharedTestFixtureStore(t)
			ctx := context.Background()
			token, err := store.CreatePendingSignup(ctx, PendingSignupEntry{Email: "pending@example.com", HashedPassword: "hashed"})
			if err != nil {
				t.Fatalf("create pending signup: %v", err)
			}
			now := time.Now().UTC()
			if test.prepare != nil {
				test.prepare(t, store, token, now)
			}

			prepared, err := store.PreparePendingSignupVerificationResend(ctx, test.email, now)
			if prepared != nil || !errors.Is(err, ErrPendingSignupResendUnavailable) {
				t.Fatalf("prepared = %+v, error = %v, want unavailable", prepared, err)
			}
		})
	}
}

func TestPreparePendingSignupVerificationResendAllowsOneConcurrentRequest(t *testing.T) {
	store := newSharedTestFixtureStoreWithOptions(t, WithThreads(2))
	ctx := context.Background()
	token, err := store.CreatePendingSignup(ctx, PendingSignupEntry{Email: "race@example.com", HashedPassword: "hashed"})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.DB().ExecContext(ctx, `UPDATE pending_signups SET verification_sent_at = ? WHERE token = ?`, now.Add(-PendingSignupVerificationResendCooldown), token); err != nil {
		t.Fatalf("make pending signup eligible: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.PreparePendingSignupVerificationResend(ctx, "race@example.com", now)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, unavailable int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrPendingSignupResendUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if succeeded != 1 || unavailable != 1 {
		t.Fatalf("concurrent results: succeeded=%d unavailable=%d", succeeded, unavailable)
	}
}

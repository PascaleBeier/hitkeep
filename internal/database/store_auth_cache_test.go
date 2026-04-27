package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func setupAuthStore(t *testing.T) *Store {
	t.Helper()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}

func TestPasswordResetTokensAreReplacedAndConsumedFromCache(t *testing.T) {
	store := setupAuthStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "reset@example.com", "old-hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	firstToken, err := store.CreatePasswordResetToken(ctx, "reset@example.com")
	if err != nil {
		t.Fatalf("create first reset token: %v", err)
	}
	if _, err := store.ResolvePasswordResetEmail(ctx, firstToken); err != nil {
		t.Fatalf("resolve first reset token: %v", err)
	}

	secondToken, err := store.CreatePasswordResetToken(ctx, "reset@example.com")
	if err != nil {
		t.Fatalf("create second reset token: %v", err)
	}
	if firstToken == secondToken {
		t.Fatal("expected distinct reset tokens")
	}

	if _, err := store.ResolvePasswordResetEmail(ctx, firstToken); !errors.Is(err, ErrPasswordResetInvalid) {
		t.Fatalf("expected first token to be invalidated, got %v", err)
	}

	rememberToken, err := store.CreateRememberMeToken(ctx, userID)
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	if err := store.CompletePasswordReset(ctx, secondToken, "new-hash"); err != nil {
		t.Fatalf("complete password reset: %v", err)
	}

	if _, err := store.ResolvePasswordResetEmail(ctx, secondToken); !errors.Is(err, ErrPasswordResetInvalid) {
		t.Fatalf("expected consumed token to be invalid, got %v", err)
	}

	user, err := store.GetUserByEmail(ctx, "reset@example.com")
	if err != nil {
		t.Fatalf("get user by email: %v", err)
	}
	if user == nil || user.Password != "new-hash" {
		t.Fatalf("expected password to be updated, got %+v", user)
	}

	resolvedUserID, err := store.ValidateRememberMeToken(ctx, rememberToken)
	if err != nil {
		t.Fatalf("validate remember me token: %v", err)
	}
	if resolvedUserID != uuid.Nil {
		t.Fatalf("expected remember me token to be invalidated, got %s", resolvedUserID)
	}
}

func TestValidateRememberMeSessionReturnsExpiry(t *testing.T) {
	store := setupAuthStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "remember@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := store.CreateRememberMeToken(ctx, userID)
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	resolvedUserID, expiresAt, err := store.ValidateRememberMeSession(ctx, token)
	if err != nil {
		t.Fatalf("validate remember me session: %v", err)
	}
	if resolvedUserID != userID {
		t.Fatalf("expected user %s, got %s", userID, resolvedUserID)
	}
	if expiresAt.IsZero() {
		t.Fatal("expected remember me expiry")
	}
}

func TestCreateRememberMeSessionWithDuration(t *testing.T) {
	store := setupAuthStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "remember-duration@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, expiresAt, err := store.CreateRememberMeSessionWithDuration(ctx, userID, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("create remember me session: %v", err)
	}
	if token == "" {
		t.Fatal("expected remember me token")
	}
	if time.Until(expiresAt) < 6*24*time.Hour || time.Until(expiresAt) > 8*24*time.Hour {
		t.Fatalf("expected remember me expiry around 7 days, got %s", expiresAt)
	}
}

package database

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/security"
)

func TestDisableUserMFAClearsSecurityFactorsAndSessions(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "mfa-user@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	otherUserID, err := store.CreateUser(ctx, "other-mfa-user@example.com", "hash")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	if err := store.EnableUserTOTP(ctx, userID, "totp-secret"); err != nil {
		t.Fatalf("enable user totp: %v", err)
	}
	if _, err := store.CreateUserPasskey(ctx, userID, "Recovery key", "cred-1", "public-key", nil); err != nil {
		t.Fatalf("create user passkey: %v", err)
	}
	codes, err := security.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := security.HashRecoveryCode(code)
		if err != nil {
			t.Fatalf("hash recovery code: %v", err)
		}
		hashes = append(hashes, hash)
	}
	if err := store.ReplaceUserRecoveryCodes(ctx, userID, hashes); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	rememberToken, err := store.CreateRememberMeToken(ctx, userID)
	if err != nil {
		t.Fatalf("create remember me token: %v", err)
	}

	if _, err := store.CreateUserPasskey(ctx, otherUserID, "Other key", "cred-2", "public-key", nil); err != nil {
		t.Fatalf("create unaffected passkey: %v", err)
	}
	otherRememberToken, err := store.CreateRememberMeToken(ctx, otherUserID)
	if err != nil {
		t.Fatalf("create unaffected remember me token: %v", err)
	}

	result, err := store.DisableUserMFA(ctx, userID)
	if err != nil {
		t.Fatalf("disable user mfa: %v", err)
	}
	if !result.TOTPDisabled {
		t.Fatal("expected totp to be disabled")
	}
	if result.PasskeysDeleted != 1 {
		t.Fatalf("expected 1 deleted passkey, got %d", result.PasskeysDeleted)
	}
	if result.SessionsInvalidated != 1 {
		t.Fatalf("expected 1 invalidated session, got %d", result.SessionsInvalidated)
	}

	hasTOTP, err := store.HasEnabledTOTP(ctx, userID)
	if err != nil {
		t.Fatalf("check totp status: %v", err)
	}
	if hasTOTP {
		t.Fatal("expected totp to be removed")
	}

	passkeys, err := store.ListUserPasskeys(ctx, userID)
	if err != nil {
		t.Fatalf("list passkeys: %v", err)
	}
	if len(passkeys) != 0 {
		t.Fatalf("expected passkeys to be removed, got %d", len(passkeys))
	}

	recoveryStatus, err := store.GetRecoveryCodeStatus(ctx, userID)
	if err != nil {
		t.Fatalf("get recovery code status: %v", err)
	}
	if recoveryStatus.Generated || recoveryStatus.Remaining != 0 {
		t.Fatalf("expected recovery codes to be removed, got %+v", recoveryStatus)
	}

	rememberedUserID, err := store.ValidateRememberMeToken(ctx, rememberToken)
	if err != nil {
		t.Fatalf("validate remember me token: %v", err)
	}
	if rememberedUserID != uuid.Nil {
		t.Fatalf("expected token to be invalidated, got %s", rememberedUserID)
	}

	unaffectedPasskeys, err := store.ListUserPasskeys(ctx, otherUserID)
	if err != nil {
		t.Fatalf("list unaffected passkeys: %v", err)
	}
	if len(unaffectedPasskeys) != 1 {
		t.Fatalf("expected unaffected passkey to remain, got %d", len(unaffectedPasskeys))
	}

	unaffectedRememberedUserID, err := store.ValidateRememberMeToken(ctx, otherRememberToken)
	if err != nil {
		t.Fatalf("validate unaffected remember me token: %v", err)
	}
	if unaffectedRememberedUserID != otherUserID {
		t.Fatalf("expected unaffected token to remain valid, got %s", unaffectedRememberedUserID)
	}
}

func TestRecoveryCodeLifecycle(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "recovery-user@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	codes, err := security.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}

	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := security.HashRecoveryCode(code)
		if err != nil {
			t.Fatalf("hash recovery code: %v", err)
		}
		hashes = append(hashes, hash)
	}
	// Persisted recovery-code hashes are untrusted at verification time. The store
	// must surface a non-canonical Argon2 parameter set rather than deriving it.
	validHash := hashes[0]
	hashes[0] = "$argon2id$v=19$m=1,t=1,p=4$invalid$invalid"
	if err := store.ReplaceUserRecoveryCodes(ctx, userID, hashes); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}

	status, err := store.GetRecoveryCodeStatus(ctx, userID)
	if err != nil {
		t.Fatalf("get recovery code status: %v", err)
	}
	if !status.Generated {
		t.Fatal("expected recovery codes to be marked as generated")
	}
	if status.Remaining != len(codes) {
		t.Fatalf("expected %d remaining codes, got %d", len(codes), status.Remaining)
	}

	if _, _, err := store.ConsumeRecoveryCode(ctx, userID, codes[0]); err == nil {
		t.Fatal("expected persisted non-canonical parameters to be rejected")
	}
	status, err = store.GetRecoveryCodeStatus(ctx, userID)
	if err != nil {
		t.Fatalf("get recovery code status after rejected consume: %v", err)
	}
	if status.Remaining != len(codes) {
		t.Fatalf("expected %d remaining codes after rejected consume, got %d", len(codes), status.Remaining)
	}

	hashes[0] = validHash
	if err := store.ReplaceUserRecoveryCodes(ctx, userID, hashes); err != nil {
		t.Fatalf("restore valid recovery codes: %v", err)
	}

	remaining, consumed, err := store.ConsumeRecoveryCode(ctx, userID, codes[0])
	if err != nil {
		t.Fatalf("consume recovery code: %v", err)
	}
	if !consumed {
		t.Fatal("expected recovery code to be consumed")
	}
	if remaining != len(codes)-1 {
		t.Fatalf("expected %d remaining codes after consume, got %d", len(codes)-1, remaining)
	}

	if _, consumed, err := store.ConsumeRecoveryCode(ctx, userID, codes[0]); err != nil {
		t.Fatalf("consume used recovery code: %v", err)
	} else if consumed {
		t.Fatal("expected used recovery code to be rejected")
	}
}

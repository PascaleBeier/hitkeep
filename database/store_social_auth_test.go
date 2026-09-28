package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSocialIdentityUsesImmutableSubjectAndRejectsCrossAccountConflicts(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	firstID, err := store.CreateUser(ctx, "first@social.test", "hash")
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	secondID, err := store.CreateUser(ctx, "second@social.test", "hash")
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	identity, err := store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{
		UserID: firstID, Provider: "google", Subject: "immutable-subject", ObservedEmail: "First@Social.Test", MarkUsed: true,
	})
	if err != nil {
		t.Fatalf("link social identity: %v", err)
	}
	if identity.ObservedEmail != "first@social.test" || identity.LastUsedAt == nil {
		t.Fatalf("unexpected linked identity: %+v", identity)
	}

	updated, err := store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{
		UserID: firstID, Provider: "google", Subject: "immutable-subject", ObservedEmail: "renamed@social.test", MarkUsed: true,
	})
	if err != nil {
		t.Fatalf("update observed metadata: %v", err)
	}
	if updated.UserID != firstID || updated.Subject != "immutable-subject" || updated.ObservedEmail != "renamed@social.test" {
		t.Fatalf("provider email change moved immutable identity: %+v", updated)
	}

	_, err = store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{
		UserID: secondID, Provider: "google", Subject: "immutable-subject", ObservedEmail: "second@social.test",
	})
	if !errors.Is(err, ErrSocialIdentityConflict) {
		t.Fatalf("expected subject collision, got %v", err)
	}
	_, err = store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{
		UserID: firstID, Provider: "google", Subject: "different-subject", ObservedEmail: "first@social.test",
	})
	if !errors.Is(err, ErrSocialProviderAlreadyLinked) {
		t.Fatalf("expected per-user provider collision, got %v", err)
	}
}

func TestPendingSocialConfirmationIsHashedExpiringAndOneTime(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	token, err := store.CreatePendingSocialConfirmation(ctx, PendingSocialConfirmation{
		Provider: "microsoft", Subject: "tenant:object", ObservedEmail: "claim@example.com",
		TargetEmail: "owner@example.com", TeamName: "Social Team", Jurisdiction: "EU", Locale: "en",
		PlanCode: "pro", BillingInterval: "annual", AcceptedTosAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create confirmation: %v", err)
	}
	var storedHash string
	if err := store.db.QueryRowContext(ctx, "SELECT token_hash FROM pending_social_confirmations").Scan(&storedHash); err != nil {
		t.Fatalf("load stored confirmation hash: %v", err)
	}
	if storedHash == token || storedHash != socialConfirmationTokenHash(token) {
		t.Fatalf("confirmation token was not stored as its SHA-256 hash")
	}

	entry, err := store.ConsumePendingSocialConfirmation(ctx, token)
	if err != nil {
		t.Fatalf("consume confirmation: %v", err)
	}
	if entry.Subject != "tenant:object" || entry.TargetEmail != "owner@example.com" || entry.PlanCode != "pro" {
		t.Fatalf("unexpected confirmation intent: %+v", entry)
	}
	if _, err := store.ConsumePendingSocialConfirmation(ctx, token); !errors.Is(err, ErrSocialConfirmationInvalid) {
		t.Fatalf("expected replay rejection, got %v", err)
	}

	expiredToken, err := store.CreatePendingSocialConfirmation(ctx, PendingSocialConfirmation{
		Provider: "microsoft", Subject: "tenant:expired", TargetEmail: "expired@example.com",
	})
	if err != nil {
		t.Fatalf("create expiring confirmation: %v", err)
	}
	if err := store.Exec(ctx, "UPDATE pending_social_confirmations SET expires_at = ? WHERE token_hash = ?", time.Now().UTC().Add(-time.Minute), socialConfirmationTokenHash(expiredToken)); err != nil {
		t.Fatalf("expire confirmation: %v", err)
	}
	if _, err := store.ConsumePendingSocialConfirmation(ctx, expiredToken); !errors.Is(err, ErrSocialConfirmationExpired) {
		t.Fatalf("expected expired token rejection, got %v", err)
	}
}

func TestPendingSocialConfirmationConcurrentConsumeHasOneWinner(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	token, err := store.CreatePendingSocialConfirmation(ctx, PendingSocialConfirmation{
		Provider: "microsoft", Subject: "tenant:concurrent", TargetEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatalf("create pending confirmation: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, consumeErr := store.ConsumePendingSocialConfirmation(ctx, token)
			results <- consumeErr
		})
	}
	wg.Wait()
	close(results)

	successes := 0
	invalid := 0
	for consumeErr := range results {
		switch {
		case consumeErr == nil:
			successes++
		case errors.Is(consumeErr, ErrSocialConfirmationInvalid):
			invalid++
		default:
			t.Fatalf("unexpected concurrent consume error: %v", consumeErr)
		}
	}
	if successes != 1 || invalid != 1 {
		t.Fatalf("expected one successful consume and one replay rejection, got success=%d invalid=%d", successes, invalid)
	}
}

func TestManagedSocialAccountStartsPasswordDisabledAndCountsSoleProvider(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	account, err := store.CreateManagedSocialAccount(ctx, CreateManagedSocialAccountInput{
		Email: "new@social.test", HashedPassword: "inaccessible-placeholder", TeamName: "New Social Team", Locale: "en",
		Provider: "github", Subject: "12345", ObservedEmail: "new@social.test", PlanCode: "business", BillingInterval: "annual",
	})
	if err != nil {
		t.Fatalf("create managed social account: %v", err)
	}
	user, err := store.GetUserByID(ctx, account.UserID)
	if err != nil || user == nil {
		t.Fatalf("load social user: user=%v err=%v", user, err)
	}
	if user.PasswordLoginEnabled {
		t.Fatal("new social user should not have password login enabled")
	}
	count, err := store.CountSoleSocialProviderUsers(ctx, "github")
	if err != nil || count != 1 {
		t.Fatalf("expected one sole GitHub user, count=%d err=%v", count, err)
	}
	if err := store.SetPasswordLoginEnabled(ctx, account.UserID, true); err != nil {
		t.Fatalf("enable password login: %v", err)
	}
	count, err = store.CountSoleSocialProviderUsers(ctx, "github")
	if err != nil || count != 0 {
		t.Fatalf("enabled password should remove sole-provider risk, count=%d err=%v", count, err)
	}
}

func TestConcurrentSocialUnlinksPreserveOnePrimaryMethod(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "concurrent-unlink@social.test", "inaccessible-placeholder")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.SetPasswordLoginEnabled(ctx, userID, false); err != nil {
		t.Fatalf("disable password login: %v", err)
	}
	for _, identity := range []LinkSocialIdentityInput{
		{UserID: userID, Provider: "google", Subject: "google-subject"},
		{UserID: userID, Provider: "github", Subject: "github-subject"},
	} {
		if _, err := store.LinkSocialIdentity(ctx, identity); err != nil {
			t.Fatalf("link %s identity: %v", identity.Provider, err)
		}
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, provider := range []string{"google", "github"} {
		wg.Go(func() {
			results <- store.DeleteSocialIdentityWithGuard(ctx, userID, provider, false)
		})
	}
	wg.Wait()
	close(results)

	successes := 0
	guards := 0
	for unlinkErr := range results {
		switch {
		case unlinkErr == nil:
			successes++
		case errors.Is(unlinkErr, ErrSocialLastLoginMethod):
			guards++
		default:
			t.Fatalf("unexpected unlink error: %v", unlinkErr)
		}
	}
	if successes != 1 || guards != 1 {
		t.Fatalf("expected one unlink and one last-method guard, got success=%d guard=%d", successes, guards)
	}
	identities, err := store.ListUserSocialIdentities(ctx, userID)
	if err != nil || len(identities) != 1 {
		t.Fatalf("expected one usable social identity to remain, identities=%+v err=%v", identities, err)
	}
}

func TestConcurrentSocialAndPasskeyRemovalPreservesOnePrimaryMethod(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "concurrent-passkey-unlink@social.test", "inaccessible-placeholder")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.SetPasswordLoginEnabled(ctx, userID, false); err != nil {
		t.Fatalf("disable password login: %v", err)
	}
	if _, err := store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{
		UserID: userID, Provider: "google", Subject: "google-passkey-race",
	}); err != nil {
		t.Fatalf("link Google identity: %v", err)
	}
	passkeyID, err := store.CreateUserPasskey(ctx, userID, "Primary passkey", "race-credential", "public-key", nil)
	if err != nil {
		t.Fatalf("create passkey: %v", err)
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		results <- store.DeleteSocialIdentityWithGuard(ctx, userID, "google", false)
	})
	wg.Go(func() {
		results <- store.DeleteUserPasskey(ctx, userID, passkeyID)
	})
	wg.Wait()
	close(results)

	successes := 0
	guards := 0
	for removalErr := range results {
		switch {
		case removalErr == nil:
			successes++
		case errors.Is(removalErr, ErrLastPrimaryLoginMethod):
			guards++
		default:
			t.Fatalf("unexpected primary-method removal error: %v", removalErr)
		}
	}
	if successes != 1 || guards != 1 {
		t.Fatalf("expected one removal and one primary-method guard, got success=%d guard=%d", successes, guards)
	}
	identities, err := store.ListUserSocialIdentities(ctx, userID)
	if err != nil {
		t.Fatalf("list remaining social identities: %v", err)
	}
	passkeys, err := store.ListUserPasskeys(ctx, userID)
	if err != nil {
		t.Fatalf("list remaining passkeys: %v", err)
	}
	if len(identities)+len(passkeys) != 1 {
		t.Fatalf("expected exactly one primary method to remain, identities=%+v passkeys=%+v", identities, passkeys)
	}
}

func TestInvitePlaceholderPasswordStaysDisabledUntilPasswordSetup(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	userID, err := store.CreatePlaceholderUserWithoutDefaultTenant(ctx, "invited@social.test", "inaccessible-placeholder")
	if err != nil {
		t.Fatalf("create invite placeholder: %v", err)
	}
	user, err := store.GetUserByID(ctx, userID)
	if err != nil || user == nil || user.PasswordLoginEnabled {
		t.Fatalf("invite placeholder unexpectedly had password login: user=%+v err=%v", user, err)
	}
	if err := store.UpdatePasswordByID(ctx, userID.String(), "real-password-hash"); err != nil {
		t.Fatalf("complete password setup: %v", err)
	}
	user, err = store.GetUserByID(ctx, userID)
	if err != nil || user == nil || !user.PasswordLoginEnabled {
		t.Fatalf("password setup did not enable login: user=%+v err=%v", user, err)
	}
}

func TestDeleteUserRemovesSocialIdentityAndPendingConfirmation(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	ownerID, err := store.CreateUser(ctx, "delete-social-owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	coOwnerID, err := store.CreateUser(ctx, "delete-social-coowner@example.com", "hash")
	if err != nil {
		t.Fatalf("create co-owner: %v", err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default team: %v", err)
	}
	if err := store.AddTeamMember(ctx, tenantID, coOwnerID, TenantRoleOwner, ownerID); err != nil {
		t.Fatalf("promote co-owner: %v", err)
	}
	if _, err := store.LinkSocialIdentity(ctx, LinkSocialIdentityInput{UserID: ownerID, Provider: "google", Subject: "delete-subject"}); err != nil {
		t.Fatalf("link identity: %v", err)
	}
	if _, err := store.CreatePendingSocialConfirmation(ctx, PendingSocialConfirmation{
		Provider: "microsoft", Subject: "delete-tenant:object", TargetEmail: "delete-social-owner@example.com", TargetUserID: &ownerID,
	}); err != nil {
		t.Fatalf("create pending confirmation: %v", err)
	}

	if err := store.DeleteUser(ctx, ownerID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	for table := range map[string]struct{}{"social_identities": {}, "pending_social_confirmations": {}} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expected %s cleanup, found %d row(s)", table, count)
		}
	}
}

package shared

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

func TestAuthStateClearUserRemovesOnlyIndexedLoginChallenges(t *testing.T) {
	state := NewAuthStateStore()

	targetUserID := uuid.New()
	otherUserID := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Minute)

	targetChallengeID := state.CreatePasskeyLoginChallenge("target", database.CreateLoginChallengeInput{
		UserID: &targetUserID,
		Flow:   "mfa",
	}, expiresAt, nil)
	otherChallengeID := state.CreatePasskeyLoginChallenge("other", database.CreateLoginChallengeInput{
		UserID: &otherUserID,
		Flow:   "mfa",
	}, expiresAt, nil)
	anonymousChallengeID := state.CreatePasskeyLoginChallenge("anonymous", database.CreateLoginChallengeInput{
		Flow: "passwordless",
	}, expiresAt, nil)

	state.ClearUser(targetUserID)

	if _, found := state.GetPasskeyLoginChallenge(targetChallengeID); found {
		t.Fatal("expected target user's challenge to be cleared")
	}
	if _, found := state.GetPasskeyLoginChallenge(otherChallengeID); !found {
		t.Fatal("expected other user's challenge to remain")
	}
	if _, found := state.GetPasskeyLoginChallenge(anonymousChallengeID); !found {
		t.Fatal("expected anonymous challenge to remain")
	}
}

func TestDeletePasskeyLoginChallengeRemovesIndexedChallenge(t *testing.T) {
	state := NewAuthStateStore()

	userID := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Minute)
	challengeID := state.CreatePasskeyLoginChallenge("challenge", database.CreateLoginChallengeInput{
		UserID: &userID,
		Flow:   "mfa",
	}, expiresAt, nil)

	state.DeletePasskeyLoginChallenge(challengeID)

	if _, found := state.GetPasskeyLoginChallenge(challengeID); found {
		t.Fatal("expected deleted challenge to be removed")
	}

	state.ClearUser(userID)
}

func TestSSOOAuthStateIsOneTimeAndExpires(t *testing.T) {
	stateStore := NewAuthStateStore()
	stateID := stateStore.CreateSSOOAuthState(SSOOAuthState{
		TeamID:       uuid.New(),
		IssuerURL:    "https://id.example.com",
		ClientID:     "hitkeep",
		Email:        "user@example.com",
		InviteToken:  "invite-token",
		Nonce:        "nonce",
		CodeVerifier: "verifier",
		ExpiresAt:    time.Now().UTC().Add(time.Minute),
	})

	state, ok := stateStore.ConsumeSSOOAuthState(stateID)
	if !ok || state.Email != "user@example.com" || state.InviteToken != "invite-token" || state.CodeVerifier != "verifier" {
		t.Fatalf("unexpected SSO state: state=%+v ok=%v", state, ok)
	}
	if _, ok := stateStore.ConsumeSSOOAuthState(stateID); ok {
		t.Fatal("expected SSO state to be one-time")
	}

	expiredID := stateStore.CreateSSOOAuthState(SSOOAuthState{
		TeamID:    uuid.New(),
		ExpiresAt: time.Now().UTC().Add(-time.Second),
	})
	if _, ok := stateStore.ConsumeSSOOAuthState(expiredID); ok {
		t.Fatal("expected expired SSO state to be rejected")
	}
}

func TestSocialOAuthStateAndCompletionAreBoundExpiringAndOneTime(t *testing.T) {
	stateStore := NewAuthStateStore()
	stateID := stateStore.CreateSocialOAuthState(SocialOAuthState{
		Provider: "google", Flow: "login", ReturnPath: "/dashboard", Nonce: "nonce", CodeVerifier: "verifier",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	state, ok := stateStore.ConsumeSocialOAuthState(stateID)
	if !ok || state.Provider != "google" || state.Nonce != "nonce" || state.CodeVerifier != "verifier" {
		t.Fatalf("unexpected social OAuth state: state=%+v ok=%v", state, ok)
	}
	if _, ok := stateStore.ConsumeSocialOAuthState(stateID); ok {
		t.Fatal("social OAuth state should reject replay")
	}
	expiredState := stateStore.CreateSocialOAuthState(SocialOAuthState{ExpiresAt: time.Now().Add(-time.Second)})
	if _, ok := stateStore.ConsumeSocialOAuthState(expiredState); ok {
		t.Fatal("expired social OAuth state should be rejected")
	}

	completionToken := stateStore.CreateSocialCompletion(SocialCompletion{
		Provider: "github", Subject: "immutable-subject", Flow: "login", ExpiresAt: time.Now().Add(time.Minute),
	})
	if completion, ok := stateStore.GetSocialCompletion(completionToken); !ok || completion.Subject != "immutable-subject" {
		t.Fatalf("unexpected social completion preview: completion=%+v ok=%v", completion, ok)
	}
	if _, ok := stateStore.ConsumeSocialCompletion(completionToken); !ok {
		t.Fatal("expected social completion consumption")
	}
	if _, ok := stateStore.GetSocialCompletion(completionToken); ok {
		t.Fatal("social completion should reject replay")
	}
	expiredCompletion := stateStore.CreateSocialCompletion(SocialCompletion{ExpiresAt: time.Now().Add(-time.Second)})
	if _, ok := stateStore.GetSocialCompletion(expiredCompletion); ok {
		t.Fatal("expired social completion should be rejected")
	}

	concurrentToken := stateStore.CreateSocialCompletion(SocialCompletion{ExpiresAt: time.Now().Add(time.Minute)})
	results := make(chan bool, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			_, consumed := stateStore.ConsumeSocialCompletion(concurrentToken)
			results <- consumed
		}()
	}
	close(start)
	consumedCount := 0
	for range 2 {
		if <-results {
			consumedCount++
		}
	}
	if consumedCount != 1 {
		t.Fatalf("expected exactly one concurrent completion consumer, got %d", consumedCount)
	}
}

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/socialauth"
)

func TestSocialProvidersExposeOnlyCompleteConfigurationAndGateSignup(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.SocialGoogleClientID = "google-id"
	h.ctx.Config.SocialGoogleClientSecret = "google-secret"
	h.ctx.Config.SocialGitHubClientID = "partial-id"
	h.ctx.Config.SocialMicrosoftClientID = "microsoft-id"
	h.ctx.Config.SocialMicrosoftClientSecret = "microsoft-secret"
	h.ctx.Config.CloudHosted = true
	h.ctx.Config.CloudSignupEnabled = true
	h.ctx.Config.SocialSignupEnabled = true

	w := httptest.NewRecorder()
	h.handleSocialProviders().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/social/providers", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Providers []struct {
			ID string `json:"id"`
		} `json:"providers"`
		SignupEnabled bool `json:"signup_enabled"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if !response.SignupEnabled || len(response.Providers) != 2 || response.Providers[0].ID != "google" || response.Providers[1].ID != "microsoft" {
		t.Fatalf("unexpected provider response: %+v", response)
	}

	h.ctx.Config.SocialSignupEnabled = false
	w = httptest.NewRecorder()
	h.handleSocialProviders().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/social/providers", nil))
	if !strings.Contains(w.Body.String(), `"signup_enabled":false`) {
		t.Fatalf("expected login providers to remain visible while signup is disabled: %s", w.Body.String())
	}
}

func TestSocialEndpointsReturnStableCodesForInvalidRequestsAndDisabledSignup(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.SocialGoogleClientID = "google-id"
	h.ctx.Config.SocialGoogleClientSecret = "google-secret"
	h.ctx.SocialAuth = socialauth.NewClient(nil)

	r := httptest.NewRequest(http.MethodPost, "/api/auth/social/google/start", strings.NewReader(`{"flow":"login","unexpected":true}`))
	r.SetPathValue("provider", "google")
	w := httptest.NewRecorder()
	h.handleSocialStart(false).ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"social_request_invalid"`) {
		t.Fatalf("expected stable invalid-request code, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"code":"social_signup_disabled"`) {
		t.Fatalf("expected stable disabled-signup code, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSocialOAuthStateIsBoundToTheBrowserThatStartedAuthorization(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "https://hitkeep.example/tools/hitkeep/"
	h.ctx.Config.SocialGitHubClientID = "github-id"
	h.ctx.Config.SocialGitHubClientSecret = "github-secret"
	h.ctx.SocialAuth = socialauth.NewClient(nil)

	startRequest := httptest.NewRequest(http.MethodPost, "/api/auth/social/github/start", strings.NewReader(`{"flow":"login"}`))
	startRequest.SetPathValue("provider", "github")
	startResponse := httptest.NewRecorder()
	h.handleSocialStart(false).ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("expected social start 200, got %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var started struct {
		AuthURL string `json:"auth_url"`
	}
	if err := json.NewDecoder(startResponse.Body).Decode(&started); err != nil {
		t.Fatalf("decode social start: %v", err)
	}
	authURL, err := url.Parse(started.AuthURL)
	if err != nil || authURL.Query().Get("state") == "" {
		t.Fatalf("expected authorization state in %q: %v", started.AuthURL, err)
	}
	state := authURL.Query().Get("state")

	var bindingCookie *http.Cookie
	for _, cookie := range startResponse.Result().Cookies() {
		if strings.HasPrefix(cookie.Name, "hk_social_state_") {
			bindingCookie = cookie
			break
		}
	}
	if bindingCookie == nil || !bindingCookie.HttpOnly || bindingCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("social state was not bound to a short-lived HttpOnly browser cookie: %+v", bindingCookie)
	}
	if bindingCookie.Path != "/tools/hitkeep/api/auth/social/" || !bindingCookie.Secure {
		t.Fatalf("social state binding did not follow the public URL path and scheme: %+v", bindingCookie)
	}

	foreignRequest := httptest.NewRequest(http.MethodGet, "/api/auth/social/github/callback?state="+url.QueryEscape(state)+"&error=access_denied", nil)
	foreignRequest.SetPathValue("provider", "github")
	foreignResponse := httptest.NewRecorder()
	h.handleSocialCallback().ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusSeeOther || !strings.Contains(foreignResponse.Header().Get("Location"), "social_state_invalid") {
		t.Fatalf("foreign browser callback should be rejected without consuming state, got %d location=%q", foreignResponse.Code, foreignResponse.Header().Get("Location"))
	}

	originalRequest := httptest.NewRequest(http.MethodGet, "/api/auth/social/github/callback?state="+url.QueryEscape(state)+"&error=access_denied", nil)
	originalRequest.SetPathValue("provider", "github")
	originalRequest.AddCookie(bindingCookie)
	originalResponse := httptest.NewRecorder()
	h.handleSocialCallback().ServeHTTP(originalResponse, originalRequest)
	if originalResponse.Code != http.StatusSeeOther || !strings.Contains(originalResponse.Header().Get("Location"), "social_provider_cancelled") {
		t.Fatalf("original browser should retain the usable one-time state, got %d location=%q", originalResponse.Code, originalResponse.Header().Get("Location"))
	}
	cleared := false
	for _, cookie := range originalResponse.Result().Cookies() {
		if cookie.Name == bindingCookie.Name && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("successful state validation did not clear the browser-binding cookie")
	}
}

func TestSocialCloudSignupCreatesFreeAndPaidAccounts(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		plan             string
		billing          string
		expectedRedirect string
	}{
		{name: "free", plan: "free", billing: "monthly", expectedRedirect: "/dashboard"},
		{name: "paid", plan: "pro", billing: "annual", expectedRedirect: "/signup/verified?billing=annual&plan=pro"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			h.ctx.Config.CloudHosted = true
			h.ctx.Config.CloudSignupEnabled = true
			h.ctx.Config.SocialSignupEnabled = true
			h.ctx.Config.CloudJurisdiction = "EU"

			email := testCase.name + "@example.com"
			completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
				Provider: "google", Subject: "google-" + testCase.name, ObservedEmail: email, EmailVerified: true,
				Flow: "signup", ExpiresAt: time.Now().Add(time.Minute),
			})
			body, _ := json.Marshal(map[string]any{
				"completion_token": completionToken,
				"team_name":        "Fast Team",
				"plan_code":        testCase.plan,
				"billing":          testCase.billing,
				"jurisdiction":     "EU",
				"locale":           "en",
				"accepted_tos":     true,
			})
			w := httptest.NewRecorder()
			h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", bytes.NewReader(body)))
			if w.Code != http.StatusCreated {
				t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
			}
			var response struct {
				Status      string `json:"status"`
				RedirectURL string `json:"redirect_url"`
			}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("decode social signup response: %v", err)
			}
			if response.Status != "ok" || response.RedirectURL != testCase.expectedRedirect {
				t.Fatalf("unexpected social signup response: %+v", response)
			}
			if len(w.Result().Cookies()) == 0 {
				t.Fatal("successful social signup did not issue a session")
			}

			user, err := store.GetUserByEmail(context.Background(), email)
			if err != nil || user == nil {
				t.Fatalf("load social signup user: user=%+v err=%v", user, err)
			}
			if user.PasswordLoginEnabled {
				t.Fatal("new social account unexpectedly enabled password login")
			}
			identity, err := store.GetSocialIdentity(context.Background(), "google", "google-"+testCase.name)
			if err != nil || identity == nil || identity.UserID != user.ID {
				t.Fatalf("social identity not attached to new account: identity=%+v err=%v", identity, err)
			}
			tenantID, err := store.GetPrimaryTenantID(context.Background(), user.ID)
			if err != nil {
				t.Fatalf("load social signup tenant: %v", err)
			}
			events, err := store.ListCloudConversionEvents(context.Background(), tenantID)
			if err != nil || len(events) != 1 || events[0].EventName != database.CloudConversionSignupVerified || events[0].PlanCode != testCase.plan || events[0].BillingInterval != testCase.billing {
				t.Fatalf("verified signup milestone mismatch: events=%+v err=%v", events, err)
			}
		})
	}
}

func TestSocialCloudSignupRejectsValuesOutsideThePublicContract(t *testing.T) {
	testCases := []struct {
		name         string
		billing      string
		jurisdiction string
		expectedCode string
	}{
		{name: "missing billing", jurisdiction: "EU", expectedCode: "social_signup_invalid"},
		{name: "unknown billing", billing: "weekly", jurisdiction: "EU", expectedCode: "social_signup_invalid"},
		{name: "missing jurisdiction", billing: "monthly", expectedCode: "social_signup_invalid"},
		{name: "wrong jurisdiction", billing: "monthly", jurisdiction: "US", expectedCode: "jurisdiction_mismatch"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			h.ctx.Config.CloudHosted = true
			h.ctx.Config.CloudSignupEnabled = true
			h.ctx.Config.SocialSignupEnabled = true
			h.ctx.Config.CloudJurisdiction = "EU"

			completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
				Provider: "google", Subject: "google-" + testCase.name, ObservedEmail: strings.ReplaceAll(testCase.name, " ", "-") + "@example.com",
				EmailVerified: true, Flow: "signup", ExpiresAt: time.Now().Add(time.Minute),
			})
			body, _ := json.Marshal(map[string]any{
				"completion_token": completionToken,
				"team_name":        "Contract Team",
				"plan_code":        "free",
				"billing":          testCase.billing,
				"jurisdiction":     testCase.jurisdiction,
				"locale":           "en",
				"accepted_tos":     true,
			})
			w := httptest.NewRecorder()
			h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", bytes.NewReader(body)))
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), testCase.expectedCode) {
				t.Fatalf("expected %s, got %d: %s", testCase.expectedCode, w.Code, w.Body.String())
			}
			if _, ok := h.ctx.AuthState.GetSocialCompletion(completionToken); !ok {
				t.Fatal("invalid form values consumed the one-time social completion")
			}
		})
	}
}

func TestSocialLoginAutoLinksVerifiedEmailAndRequiresExistingMFA(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "User@Example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.EnableUserTOTP(context.Background(), userID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatalf("enable TOTP: %v", err)
	}
	completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
		Provider: "google", Subject: "google-subject", ObservedEmail: "user@example.com", EmailVerified: true,
		Flow: "login", ReturnPath: "/dashboard", RememberMe: true, ExpiresAt: time.Now().Add(time.Minute),
	})
	body, _ := json.Marshal(map[string]any{"completion_token": completionToken})
	w := httptest.NewRecorder()
	h.handleSocialComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/social/complete", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var response socialCompleteResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "mfa_required" || response.ChallengeToken == "" || len(response.Factors) != 1 || response.Factors[0] != "totp" {
		t.Fatalf("expected existing local MFA, got %+v", response)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("remember-me session must not be issued before MFA succeeds")
	}
	challengeID, err := uuid.Parse(response.ChallengeToken)
	if err != nil {
		t.Fatalf("parse MFA challenge: %v", err)
	}
	challenge, ok := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID)
	if !ok || challenge.AuthProvider != "google" || !challenge.RememberMe {
		t.Fatalf("social provider and remember-me intent were not preserved for MFA audit/session: %+v", challenge)
	}
	identity, err := store.GetSocialIdentity(context.Background(), "google", "google-subject")
	if err != nil || identity == nil || identity.UserID != userID {
		t.Fatalf("verified normalized email was not auto-linked: identity=%+v err=%v", identity, err)
	}
	if _, ok := h.ctx.AuthState.GetSocialCompletion(completionToken); ok {
		t.Fatal("social completion should be consumed once identity resolution succeeds")
	}
}

func TestSocialInviteRequiresVerifiedMatchingGoogleEmailAndLetsInviteProveMicrosoftTarget(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	ownerID, err := store.CreateUser(context.Background(), "owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	teamID, err := store.GetPrimaryTenantID(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("get owner team: %v", err)
	}
	inviteeID, err := store.CreatePlaceholderUserWithoutDefaultTenant(context.Background(), "invited@example.com", "placeholder")
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	if _, err := store.CreateTeamInvite(context.Background(), teamID, "invited@example.com", database.TenantRoleMember, &inviteeID, ownerID, true); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	inviteToken, err := store.CreatePasswordResetToken(context.Background(), "invited@example.com")
	if err != nil {
		t.Fatalf("create invite token: %v", err)
	}

	complete := func(completion shared.SocialCompletion) *httptest.ResponseRecorder {
		completion.Flow = "invite"
		completion.InviteToken = inviteToken
		completion.ReturnPath = "/dashboard"
		completion.ExpiresAt = time.Now().Add(time.Minute)
		token := h.ctx.AuthState.CreateSocialCompletion(completion)
		body, _ := json.Marshal(map[string]string{"completion_token": token})
		w := httptest.NewRecorder()
		h.handleSocialComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/social/complete", bytes.NewReader(body)))
		return w
	}

	mismatch := complete(shared.SocialCompletion{
		Provider: "google", Subject: "wrong-google", ObservedEmail: "someone-else@example.com", EmailVerified: true,
	})
	if mismatch.Code != http.StatusForbidden || !strings.Contains(mismatch.Body.String(), `"social_invite_email_mismatch"`) {
		t.Fatalf("expected Google invite-email mismatch, got %d: %s", mismatch.Code, mismatch.Body.String())
	}
	if _, err := store.ResolvePasswordResetEmail(context.Background(), inviteToken); err != nil {
		t.Fatalf("email mismatch consumed the invite token: %v", err)
	}

	microsoftSubject := uuid.NewString() + ":" + uuid.NewString()
	microsoft := complete(shared.SocialCompletion{
		Provider: "microsoft", Subject: microsoftSubject, ObservedEmail: "mutable@example.net",
	})
	if microsoft.Code != http.StatusOK || !strings.Contains(microsoft.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected valid invite to prove Microsoft target, got %d: %s", microsoft.Code, microsoft.Body.String())
	}
	if _, err := store.ResolvePasswordResetEmail(context.Background(), inviteToken); err == nil {
		t.Fatal("successful Microsoft invitation did not consume the invite token")
	}
	linkedUser, identity, err := store.GetUserBySocialIdentity(context.Background(), "microsoft", microsoftSubject)
	if err != nil || linkedUser == nil || identity == nil || linkedUser.ID != inviteeID || identity.UserID != inviteeID {
		t.Fatalf("Microsoft invite linked the wrong target: user=%+v identity=%+v err=%v", linkedUser, identity, err)
	}
}

func TestMicrosoftFirstAccountLinkSendsHitKeepConfirmation(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	if _, err := store.CreateUser(context.Background(), "owner@example.com", "hash"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	driver := &authTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)
	completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
		Provider: "microsoft", Subject: "11111111-2222-3333-4444-555555555555:object", ObservedEmail: "mutable@example.net",
		Flow: "login", ExpiresAt: time.Now().Add(time.Minute),
	})
	body, _ := json.Marshal(map[string]any{"completion_token": completionToken, "email": "owner@example.com"})
	w := httptest.NewRecorder()
	h.handleSocialComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/social/complete", bytes.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"verification_sent"`) {
		t.Fatalf("expected Microsoft confirmation response, got %d: %s", w.Code, w.Body.String())
	}
	const marker = "/api/auth/social/confirm?token="
	index := strings.Index(driver.textBody, marker)
	if index == -1 {
		t.Fatalf("expected social confirmation URL in mail: %s", driver.textBody)
	}
	raw := driver.textBody[index+len(marker):]
	raw = strings.Fields(raw)[0]
	token, err := url.QueryUnescape(raw)
	if err != nil || token == "" {
		t.Fatalf("decode social confirmation token %q: %v", raw, err)
	}
	entry, err := store.ConsumePendingSocialConfirmation(context.Background(), token)
	if err != nil {
		t.Fatalf("consume pending Microsoft confirmation: %v", err)
	}
	if entry.TargetUserID == nil || entry.TargetEmail != "owner@example.com" || entry.Subject == "" {
		t.Fatalf("unexpected Microsoft confirmation intent: %+v", entry)
	}
	if _, ok := h.ctx.AuthState.GetSocialCompletion(completionToken); ok {
		t.Fatal("emailed social completion should no longer be reusable")
	}
}

func TestSocialPreviewRequiresMicrosoftEmailConfirmationOnlyBeforeFirstLink(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.SocialMicrosoftClientID = "microsoft-id"
	h.ctx.Config.SocialMicrosoftClientSecret = "microsoft-secret"

	userID, err := store.CreateUser(context.Background(), "linked@example.com", "hash")
	if err != nil {
		t.Fatalf("create linked user: %v", err)
	}
	linkedSubject := uuid.NewString() + ":" + uuid.NewString()
	if _, err := store.LinkSocialIdentity(context.Background(), database.LinkSocialIdentityInput{
		UserID: userID, Provider: "microsoft", Subject: linkedSubject,
	}); err != nil {
		t.Fatalf("link Microsoft identity: %v", err)
	}

	preview := func(subject string) bool {
		t.Helper()
		completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
			Provider: "microsoft", Subject: subject, ObservedEmail: "mutable@example.net",
			Flow: "login", ExpiresAt: time.Now().Add(time.Minute),
		})
		body, _ := json.Marshal(map[string]string{"completion_token": completionToken})
		w := httptest.NewRecorder()
		h.handleSocialPreview().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/social/preview", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("preview Microsoft identity: %d %s", w.Code, w.Body.String())
		}
		var response map[string]any
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("decode Microsoft preview: %v", err)
		}
		required, ok := response["email_confirmation_required"].(bool)
		if !ok {
			t.Fatalf("Microsoft preview omitted email confirmation requirement: %+v", response)
		}
		return required
	}

	if preview(linkedSubject) {
		t.Fatal("already-linked immutable Microsoft identity should not ask for HitKeep email again")
	}
	if !preview(uuid.NewString() + ":" + uuid.NewString()) {
		t.Fatal("first unauthenticated Microsoft link should require HitKeep email confirmation")
	}
}

func TestMicrosoftSocialSignupCreatesAccountOnlyAfterHitKeepConfirmation(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	h.ctx.Config.CloudHosted = true
	h.ctx.Config.CloudSignupEnabled = true
	h.ctx.Config.SocialSignupEnabled = true
	h.ctx.Config.CloudJurisdiction = "US"
	driver := &authTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(driver, h.ctx.Config)

	const subject = "11111111-2222-3333-4444-555555555555:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
		Provider: "microsoft", Subject: subject, ObservedEmail: "mutable@example.net",
		Flow: "signup", ExpiresAt: time.Now().Add(time.Minute),
	})
	body, _ := json.Marshal(map[string]any{
		"completion_token": completionToken,
		"email":            "new-owner@example.com",
		"team_name":        "Confirmed Team",
		"plan_code":        "business",
		"billing":          "annual",
		"jurisdiction":     "US",
		"locale":           "en",
		"accepted_tos":     true,
	})
	w := httptest.NewRecorder()
	h.handleSocialCloudSignupComplete().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cloud/signup/social/complete", bytes.NewReader(body)))
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"verification_sent"`) {
		t.Fatalf("expected Microsoft signup confirmation, got %d: %s", w.Code, w.Body.String())
	}
	if user, err := store.GetUserByEmail(context.Background(), "new-owner@example.com"); err != nil || user != nil {
		t.Fatalf("Microsoft account was created before confirmation: user=%+v err=%v", user, err)
	}

	const marker = "/api/auth/social/confirm?token="
	index := strings.Index(driver.textBody, marker)
	if index == -1 {
		t.Fatalf("expected social confirmation URL in mail: %s", driver.textBody)
	}
	raw := strings.Fields(driver.textBody[index+len(marker):])[0]
	token, err := url.QueryUnescape(raw)
	if err != nil || token == "" {
		t.Fatalf("decode social confirmation token %q: %v", raw, err)
	}
	w = httptest.NewRecorder()
	h.handleSocialConfirmation().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/social/confirm?token="+url.QueryEscape(token), nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "http://localhost:8080/signup/verified?billing=annual&plan=business" {
		t.Fatalf("expected paid signup redirect after confirmation, got %d location=%q", w.Code, w.Header().Get("Location"))
	}
	if len(w.Result().Cookies()) == 0 {
		t.Fatal("confirmed Microsoft signup did not issue a session")
	}
	user, err := store.GetUserByEmail(context.Background(), "new-owner@example.com")
	if err != nil || user == nil || user.PasswordLoginEnabled {
		t.Fatalf("unexpected confirmed Microsoft user: user=%+v err=%v", user, err)
	}
	identity, err := store.GetSocialIdentity(context.Background(), "microsoft", subject)
	if err != nil || identity == nil || identity.UserID != user.ID {
		t.Fatalf("Microsoft identity not attached after confirmation: identity=%+v err=%v", identity, err)
	}
}

func TestMicrosoftConfirmationResumesBoundLoginWithOneTimeCompletion(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	userID, err := store.CreateUser(context.Background(), "owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := store.CreatePendingSocialConfirmation(context.Background(), database.PendingSocialConfirmation{
		Provider: "microsoft", Subject: "11111111-2222-3333-4444-555555555555:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ObservedEmail: "mutable@example.net", TargetEmail: "owner@example.com", TargetUserID: &userID,
		ReturnPath: "/events?range=30d", RememberMe: true,
	})
	if err != nil {
		t.Fatalf("create pending confirmation: %v", err)
	}

	w := httptest.NewRecorder()
	h.handleSocialConfirmation().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/social/confirm?token="+url.QueryEscape(token), nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected confirmation redirect, got %d: %s", w.Code, w.Body.String())
	}
	redirect, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse confirmation redirect: %v", err)
	}
	completionToken := new(url.Values)
	*completionToken, err = url.ParseQuery(strings.TrimPrefix(redirect.Fragment, "?"))
	if err != nil || completionToken.Get("social_token") == "" {
		t.Fatalf("expected fragment completion token in %q: %v", redirect.String(), err)
	}
	completion, ok := h.ctx.AuthState.ConsumeSocialCompletion(completionToken.Get("social_token"))
	if !ok || completion.Provider != "microsoft" || completion.ReturnPath != "/events?range=30d" || !completion.RememberMe {
		t.Fatalf("unexpected resumed completion: %+v found=%v", completion, ok)
	}
	identity, err := store.GetSocialIdentity(context.Background(), "microsoft", "11111111-2222-3333-4444-555555555555:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err != nil || identity == nil || identity.UserID != userID {
		t.Fatalf("confirmed identity was not linked: identity=%+v err=%v", identity, err)
	}
}

func TestAuthenticatedMicrosoftLinkDoesNotAuthorizeByMutableEmail(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	firstID, err := store.CreateUser(context.Background(), "first@example.com", "hash")
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if _, err := store.CreateUser(context.Background(), "claimed@example.com", "hash"); err != nil {
		t.Fatalf("create claimed-email user: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/callback", nil)
	identity := socialauth.Identity{
		Provider: "microsoft", Subject: uuid.NewString() + ":" + uuid.NewString(), Email: "claimed@example.com", EmailVerified: false,
	}
	if err := h.completeAuthenticatedSocialLink(r, firstID, identity); err != nil {
		t.Fatalf("authenticated Microsoft link should ignore mutable email ownership: %v", err)
	}
	linked, err := store.GetSocialIdentity(context.Background(), identity.Provider, identity.Subject)
	if err != nil || linked == nil || linked.UserID != firstID {
		t.Fatalf("Microsoft identity linked to wrong account: identity=%+v err=%v", linked, err)
	}
}

func TestSanitizeAuthReturnPathRejectsExternalAndAmbiguousURLs(t *testing.T) {
	for _, unsafe := range []string{"https://evil.example/phish", "//evil.example/phish", "/\\\\evil.example/phish", "/%2f%2fevil.example/phish", "/%5cevil.example/phish", "/login?next=/dashboard", "/setup", "/events#fragment", "/events\r\nLocation: https://evil.example"} {
		if got := sanitizeAuthReturnPath(unsafe); got != "/dashboard" {
			t.Fatalf("unsafe return path %q resolved to %q", unsafe, got)
		}
	}
	if got := sanitizeAuthReturnPath("/events?range=30d"); got != "/events?range=30d" {
		t.Fatalf("safe local return path changed to %q", got)
	}
}

func TestSocialUnlinkRequiresAnotherUsablePrimaryMethod(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "unlink@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.SetPasswordLoginEnabled(context.Background(), userID, false); err != nil {
		t.Fatalf("disable password login: %v", err)
	}
	if _, err := store.LinkSocialIdentity(context.Background(), database.LinkSocialIdentityInput{UserID: userID, Provider: "github", Subject: "42"}); err != nil {
		t.Fatalf("link identity: %v", err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/user/security/social/github", bytes.NewReader([]byte(`{}`)))
	request.SetPathValue("provider", "github")
	request = request.WithContext(context.WithValue(request.Context(), shared.UserIDKey, userID))
	w := httptest.NewRecorder()
	h.handleSocialUnlink().ServeHTTP(w, request)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "social_last_login_method") {
		t.Fatalf("expected last-method guard, got %d: %s", w.Code, w.Body.String())
	}

	if err := store.SetPasswordLoginEnabled(context.Background(), userID, true); err != nil {
		t.Fatalf("enable password login: %v", err)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/user/security/social/github", bytes.NewReader([]byte(`{"current_password":"password123"}`)))
	request.SetPathValue("provider", "github")
	request = request.WithContext(context.WithValue(request.Context(), shared.UserIDKey, userID))
	w = httptest.NewRecorder()
	h.handleSocialUnlink().ServeHTTP(w, request)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected confirmed password unlink, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDisabledPasswordLoginUsesGenericFailure(t *testing.T) {
	h, store := setupAuthTestEnv(t)
	defer store.Close()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "social-only@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.SetPasswordLoginEnabled(context.Background(), userID, false); err != nil {
		t.Fatalf("disable password login: %v", err)
	}

	login := func(email, password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"email": email, "password": password})
		w := httptest.NewRecorder()
		h.handleLogin().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body)))
		return w
	}
	disabled := login("social-only@example.com", "password123")
	incorrect := login("social-only@example.com", "incorrect")
	if disabled.Code != http.StatusUnauthorized || incorrect.Code != http.StatusUnauthorized || disabled.Body.String() != incorrect.Body.String() {
		t.Fatalf("disabled password disclosed a different failure: disabled=%d %q incorrect=%d %q", disabled.Code, disabled.Body.String(), incorrect.Code, incorrect.Body.String())
	}
}

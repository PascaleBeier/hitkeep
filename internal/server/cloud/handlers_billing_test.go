//go:build billing

package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
	"golang.org/x/time/rate"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
)

type fakeStripeClient struct {
	lastCustomerInput *createCustomerInput
	lastCheckoutInput *createCheckoutSessionInput
	lastPortalInput   *createPortalSessionInput
	lastChargeID      string
	chargeCustomerID  string
	createCustomerErr error
	createCheckoutErr error
	createPortalErr   error
}

func (f *fakeStripeClient) CreateCustomer(_ context.Context, input createCustomerInput) (string, error) {
	f.lastCustomerInput = &input
	if f.createCustomerErr != nil {
		return "", f.createCustomerErr
	}
	return "cus_test", nil
}

func (f *fakeStripeClient) CreateCheckoutSession(_ context.Context, input createCheckoutSessionInput) (*checkoutSessionOutput, error) {
	f.lastCheckoutInput = &input
	if f.createCheckoutErr != nil {
		return nil, f.createCheckoutErr
	}
	return &checkoutSessionOutput{
		ID:  "cs_test",
		URL: "https://checkout.stripe.test/session",
	}, nil
}

func (f *fakeStripeClient) CreatePortalSession(_ context.Context, input createPortalSessionInput) (*portalSessionOutput, error) {
	f.lastPortalInput = &input
	if f.createPortalErr != nil {
		return nil, f.createPortalErr
	}
	return &portalSessionOutput{
		ID:  "bps_test",
		URL: "https://billing.stripe.test/session",
	}, nil
}

func (f *fakeStripeClient) GetCharge(_ context.Context, chargeID string) (*stripeChargeOutput, error) {
	f.lastChargeID = chargeID
	customerID := f.chargeCustomerID
	if customerID == "" {
		customerID = "cus_dispute"
	}
	return &stripeChargeOutput{
		ID:         chargeID,
		CustomerID: customerID,
	}, nil
}

type noopMailDriver struct{}

func (noopMailDriver) Send(_ []string, _ string, _ string, _ string) error { return nil }
func (noopMailDriver) Close() error                                        { return nil }

type captureMailDriver struct {
	recipients []string
	subject    string
	htmlBody   string
	textBody   string
	sendCount  int
	err        error
}

func (d *captureMailDriver) Send(recipients []string, subject, htmlBody, textBody string) error {
	d.recipients = append([]string(nil), recipients...)
	d.subject = subject
	d.htmlBody = htmlBody
	d.textBody = textBody
	d.sendCount++
	return d.err
}

func (d *captureMailDriver) Close() error { return nil }

type fakeWebhookVerifier struct {
	event stripe.Event
	err   error
}

func (f fakeWebhookVerifier) ConstructEvent(_ []byte, _ string, _ string) (stripe.Event, error) {
	return f.event, f.err
}

func setupCloudTestHandler(t *testing.T) (*handler, *database.Store) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	testConf := &config.Config{
		PublicURL:                   "https://cloud.hitkeep.eu",
		JWTSecret:                   "test-secret",
		CloudHosted:                 true,
		CloudSignupEnabled:          true,
		CloudJurisdiction:           "EU",
		StripeSecretKey:             "sk_test_123",
		StripePortalConfigurationID: "bpc_test_123",
		StripeWebhookSecret:         "whsec_test_123",
		StripePriceProMonthly:       "price_pro",
		StripePriceBusinessMonthly:  "price_business",
		StripePriceProAnnual:        "price_pro_annual",
		StripePriceBusinessAnnual:   "price_business_annual",
	}

	h := &handler{
		ctx: &shared.Context{
			Store:  store,
			Config: testConf,
			Mailer: mailer.NewWithDriver(noopMailDriver{}, testConf),
		},
		stripe:   &fakeStripeClient{},
		webhooks: fakeWebhookVerifier{},
	}

	return h, store
}

func setupSignedCloudWebhookTestHandler(t *testing.T) (*handler, *database.Store, *fakeStripeClient) {
	t.Helper()

	h, store := setupCloudTestHandler(t)
	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok {
		t.Fatal("expected fake stripe client")
	}
	h.webhooks = stripeWebhookSDK{}
	return h, store, stripeClient
}

func signedStripeEventPayload(t *testing.T, eventID string, eventType string, object any) (payload []byte, signature string) {
	t.Helper()

	envelope, err := json.Marshal(map[string]any{
		"id":          eventID,
		"object":      "event",
		"api_version": stripeAPIVersion,
		"type":        eventType,
		"livemode":    false,
		"data": map[string]any{
			"object": object,
		},
	})
	if err != nil {
		t.Fatalf("marshal signed stripe event payload: %v", err)
	}

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: envelope,
		Secret:  "whsec_test_123",
	})
	return signed.Payload, signed.Header
}

func postSignedStripeWebhook(t *testing.T, h *handler, eventID string, eventType string, object any) *httptest.ResponseRecorder {
	t.Helper()

	payload, signature := signedStripeEventPayload(t, eventID, eventType, object)
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signature)
	w := httptest.NewRecorder()
	h.handleStripeWebhook().ServeHTTP(w, req)
	return w
}

func TestRegisterUsesDedicatedWebhookLimiter(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	apiLimiter := shared.NewIPRateLimiter(0, 0)
	defer apiLimiter.Stop()
	webhookLimiter := shared.NewIPRateLimiter(rate.Limit(10), 10)
	defer webhookLimiter.Stop()

	ctx := &shared.Context{
		Store: store,
		Config: &config.Config{
			CloudHosted:         true,
			StripeSecretKey:     "sk_test_123",
			StripeWebhookSecret: "whsec_test_123",
		},
		ApiLimiter:     apiLimiter,
		WebhookLimiter: webhookLimiter,
	}

	mux := http.NewServeMux()
	Register(mux, ctx)

	payload, signature := signedStripeEventPayload(t, "evt_webhook_limit_ok", "billing.test", map[string]any{
		"id": "obj_test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signature)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d when webhook limiter allows request, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestRegisterUsesAuthLimiterForSignupVerificationResend(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	authLimiter := shared.NewIPRateLimiter(0, 0)
	defer authLimiter.Stop()
	apiLimiter := shared.NewIPRateLimiter(rate.Limit(10), 10)
	defer apiLimiter.Stop()
	webhookLimiter := shared.NewIPRateLimiter(rate.Limit(10), 10)
	defer webhookLimiter.Stop()

	ctx := &shared.Context{
		Store: store,
		Config: &config.Config{
			CloudHosted:        true,
			CloudSignupEnabled: true,
			StripeSecretKey:    "sk_test_123",
		},
		AuthLimiter:    authLimiter,
		ApiLimiter:     apiLimiter,
		WebhookLimiter: webhookLimiter,
	}
	mux := http.NewServeMux()
	Register(mux, ctx)

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup/resend-verification", strings.NewReader(`{"email":"user@example.com"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRegisterWebhookLimiterCanThrottleStripeWebhook(t *testing.T) {
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	apiLimiter := shared.NewIPRateLimiter(rate.Inf, 1)
	defer apiLimiter.Stop()
	webhookLimiter := shared.NewIPRateLimiter(0, 0)
	defer webhookLimiter.Stop()

	ctx := &shared.Context{
		Store: store,
		Config: &config.Config{
			CloudHosted:         true,
			StripeSecretKey:     "sk_test_123",
			StripeWebhookSecret: "whsec_test_123",
		},
		ApiLimiter:     apiLimiter,
		WebhookLimiter: webhookLimiter,
	}

	mux := http.NewServeMux()
	Register(mux, ctx)

	payload, signature := signedStripeEventPayload(t, "evt_webhook_limit_blocked", "billing.test", map[string]any{
		"id": "obj_test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signature)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d when webhook limiter blocks request, got %d: %s", http.StatusTooManyRequests, w.Code, w.Body.String())
	}
}

func TestHandleSignupSendsVerificationEmail(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	body, err := json.Marshal(signupRequest{
		Email:        "free@example.com",
		Password:     "password123",
		GivenName:    "Free",
		LastName:     "User",
		TeamName:     "Free Team",
		PlanCode:     database.CloudPlanFree,
		Jurisdiction: "EU",
		AcceptedTos:  true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSignup().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp signupResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "verification_sent" {
		t.Fatalf("expected status verification_sent, got %q", resp.Status)
	}
	if resp.RetryAfterSeconds != int(database.PendingSignupVerificationResendCooldown/time.Second) {
		t.Fatalf("retry_after_seconds = %d, want %d", resp.RetryAfterSeconds, int(database.PendingSignupVerificationResendCooldown/time.Second))
	}
	if resp.RedirectURL != "" {
		t.Fatalf("expected no redirect_url before verification, got %q", resp.RedirectURL)
	}

	// User should NOT exist yet — account is pending verification
	user, _ := store.GetUserByEmail(context.Background(), "free@example.com")
	if user != nil {
		t.Fatal("expected user NOT to exist before email verification")
	}
}

func TestHandleSignupDoesNotLogRawVerificationMailError(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	rawMailError := "provider response password=super-secret token=top-secret https://mail.example.test/reject"
	h.ctx.Mailer = mailer.NewWithDriver(&captureMailDriver{err: errors.New(rawMailError)}, h.ctx.Config)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	body, err := json.Marshal(signupRequest{
		Email:        "verification-mail-error@example.com",
		Password:     "password123",
		GivenName:    "Verification",
		LastName:     "Failure",
		TeamName:     "Verification Team",
		PlanCode:     database.CloudPlanFree,
		Jurisdiction: "EU",
		AcceptedTos:  true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup", bytes.NewReader(body))
	req = req.WithContext(shared.WithLogger(req.Context(), logger))
	w := httptest.NewRecorder()
	h.handleSignup().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
	if strings.Contains(logs.String(), rawMailError) || strings.Contains(w.Body.String(), rawMailError) {
		t.Fatalf("raw verification mail error leaked into logs or response: logs=%q body=%q", logs.String(), w.Body.String())
	}
	if !strings.Contains(logs.String(), "error_stage=transport") || !strings.Contains(logs.String(), "error_kind=transport") {
		t.Fatalf("expected safe verification mail diagnostics, got %q", logs.String())
	}
}

func TestHandleResendSignupVerificationSendsExistingLocalizedLink(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	mailDriver := &captureMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(mailDriver, h.ctx.Config)

	token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{
		Email:          "Mixed.Case@Example.com",
		HashedPassword: "hashed",
		TeamName:       "Beispielteam",
		Locale:         "de",
	})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}
	var expiresAt time.Time
	if err := store.DB().QueryRowContext(context.Background(), `SELECT expires_at FROM pending_signups WHERE token = ?`, token).Scan(&expiresAt); err != nil {
		t.Fatalf("read pending signup expiry: %v", err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET verification_sent_at = ? WHERE token = ?`, time.Now().UTC().Add(-database.PendingSignupVerificationResendCooldown), token); err != nil {
		t.Fatalf("make pending signup eligible: %v", err)
	}
	var usersBefore, tenantsBefore, billingBefore, conversionsBefore int
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM tenants),
			(SELECT COUNT(*) FROM cloud_billing_accounts),
			(SELECT COUNT(*) FROM cloud_conversion_events)
	`).Scan(&usersBefore, &tenantsBefore, &billingBefore, &conversionsBefore); err != nil {
		t.Fatalf("snapshot account state before resend: %v", err)
	}

	w := postSignupVerificationResend(t, h, "  mixed.case@example.com  ")
	assertSignupVerificationResendAccepted(t, w)
	if mailDriver.sendCount != 1 || len(mailDriver.recipients) != 1 || mailDriver.recipients[0] != "Mixed.Case@Example.com" {
		t.Fatalf("mail delivery = count %d recipients %v", mailDriver.sendCount, mailDriver.recipients)
	}
	if got := extractCloudSignupToken(t, mailDriver.htmlBody+"\n"+mailDriver.textBody); got != token {
		t.Fatalf("resent token = %q, want %q", got, token)
	}
	if !strings.Contains(mailDriver.htmlBody+mailDriver.textBody, "Beispielteam") {
		t.Fatalf("expected stored localized team name in resent email")
	}
	var gotExpiresAt time.Time
	if err := store.DB().QueryRowContext(context.Background(), `SELECT expires_at FROM pending_signups WHERE token = ?`, token).Scan(&gotExpiresAt); err != nil {
		t.Fatalf("read pending signup expiry after resend: %v", err)
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("resend changed expiry from %s to %s", expiresAt, gotExpiresAt)
	}
	if user, _ := store.GetUserByEmail(context.Background(), "mixed.case@example.com"); user != nil {
		t.Fatal("resend created a user")
	}
	var usersAfter, tenantsAfter, billingAfter, conversionsAfter int
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM tenants),
			(SELECT COUNT(*) FROM cloud_billing_accounts),
			(SELECT COUNT(*) FROM cloud_conversion_events)
	`).Scan(&usersAfter, &tenantsAfter, &billingAfter, &conversionsAfter); err != nil {
		t.Fatalf("snapshot account state after resend: %v", err)
	}
	if usersAfter != usersBefore || tenantsAfter != tenantsBefore || billingAfter != billingBefore || conversionsAfter != conversionsBefore {
		t.Fatalf("resend changed account state: users %d→%d tenants %d→%d billing %d→%d conversions %d→%d", usersBefore, usersAfter, tenantsBefore, tenantsAfter, billingBefore, billingAfter, conversionsBefore, conversionsAfter)
	}
}

func TestHandleResendSignupVerificationHidesPendingSignupState(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, h *handler, store *database.Store, token string)
		email   string
	}{
		{name: "unknown", email: "unknown@example.com"},
		{name: "cooldown", email: "pending@example.com"},
		{
			name:  "expired",
			email: "pending@example.com",
			prepare: func(t *testing.T, _ *handler, store *database.Store, token string) {
				t.Helper()
				if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET expires_at = ? WHERE token = ?`, time.Now().UTC().Add(-time.Second), token); err != nil {
					t.Fatalf("expire pending signup: %v", err)
				}
			},
		},
		{
			name:  "consumed",
			email: "pending@example.com",
			prepare: func(t *testing.T, _ *handler, store *database.Store, token string) {
				t.Helper()
				if _, err := store.CompletePendingSignup(context.Background(), token); err != nil {
					t.Fatalf("consume pending signup: %v", err)
				}
			},
		},
		{
			name:  "mailer unavailable",
			email: "pending@example.com",
			prepare: func(t *testing.T, h *handler, store *database.Store, token string) {
				t.Helper()
				h.ctx.Mailer = nil
				if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET verification_sent_at = ? WHERE token = ?`, time.Now().UTC().Add(-database.PendingSignupVerificationResendCooldown), token); err != nil {
					t.Fatalf("make pending signup eligible: %v", err)
				}
			},
		},
	}

	var wantBody string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			defer store.Close()
			mailDriver := &captureMailDriver{}
			h.ctx.Mailer = mailer.NewWithDriver(mailDriver, h.ctx.Config)
			token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{Email: "pending@example.com", HashedPassword: "hashed"})
			if err != nil {
				t.Fatalf("create pending signup: %v", err)
			}
			if test.prepare != nil {
				test.prepare(t, h, store, token)
			}

			w := postSignupVerificationResend(t, h, test.email)
			assertSignupVerificationResendAccepted(t, w)
			if wantBody == "" {
				wantBody = w.Body.String()
			} else if w.Body.String() != wantBody {
				t.Fatalf("response body = %q, want indistinguishable %q", w.Body.String(), wantBody)
			}
			if mailDriver.sendCount != 0 {
				t.Fatalf("unexpected mail deliveries: %d", mailDriver.sendCount)
			}
		})
	}
}

func TestHandleResendSignupVerificationHidesMailFailure(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	mailDriver := &captureMailDriver{err: errors.New("smtp rejected message")}
	h.ctx.Mailer = mailer.NewWithDriver(mailDriver, h.ctx.Config)

	token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{Email: "failure@example.com", HashedPassword: "hashed"})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `UPDATE pending_signups SET verification_sent_at = ? WHERE token = ?`, time.Now().UTC().Add(-database.PendingSignupVerificationResendCooldown), token); err != nil {
		t.Fatalf("make pending signup eligible: %v", err)
	}

	w := postSignupVerificationResend(t, h, "failure@example.com")
	assertSignupVerificationResendAccepted(t, w)
	if mailDriver.sendCount != 1 {
		t.Fatalf("mail attempts = %d, want 1", mailDriver.sendCount)
	}
	if !strings.Contains(logs.String(), "error_code=mail_send_failed") {
		t.Fatalf("expected safe mail failure category, got %q", logs.String())
	}
	if strings.Contains(logs.String(), "failure@example.com") || strings.Contains(logs.String(), token) || strings.Contains(logs.String(), "smtp rejected message") {
		t.Fatalf("resend failure log exposed request or provider details: %q", logs.String())
	}
}

func TestHandleResendSignupVerificationRejectsMalformedJSON(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup/resend-verification", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.handleResendSignupVerification().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleResendSignupVerificationIsCloudOnly(t *testing.T) {
	tests := []struct {
		name          string
		cloudHosted   bool
		signupEnabled bool
	}{
		{name: "self hosted", signupEnabled: true},
		{name: "signup disabled", cloudHosted: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			defer store.Close()
			h.ctx.Config.CloudHosted = test.cloudHosted
			h.ctx.Config.CloudSignupEnabled = test.signupEnabled

			w := postSignupVerificationResend(t, h, "user@example.com")
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
			}
		})
	}
}

func postSignupVerificationResend(t *testing.T, h *handler, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(resendSignupVerificationRequest{Email: email})
	if err != nil {
		t.Fatalf("marshal resend request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup/resend-verification", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleResendSignupVerification().ServeHTTP(w, req)
	return w
}

func assertSignupVerificationResendAccepted(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusAccepted, w.Body.String())
	}
	var response resendSignupVerificationResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode resend response: %v", err)
	}
	if response.Status != "accepted" || response.RetryAfterSeconds != int(database.PendingSignupVerificationResendCooldown/time.Second) {
		t.Fatalf("resend response = %+v", response)
	}
}

func TestHandleSignupDefaultsLocalizedTeamNameWhenBlank(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	mailDriver := &captureMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(mailDriver, h.ctx.Config)

	body, err := json.Marshal(signupRequest{
		Email:        "maria@example.com",
		Password:     "password123",
		GivenName:    "María",
		LastName:     "García",
		TeamName:     "   ",
		Jurisdiction: "EU",
		Locale:       "es-ES",
		AcceptedTos:  true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSignup().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
	if !strings.Contains(mailDriver.htmlBody+mailDriver.textBody, "Equipo de María") {
		t.Fatalf("expected verification email to use localized team name, got html:\n%s\ntext:\n%s", mailDriver.htmlBody, mailDriver.textBody)
	}

	token := extractCloudSignupToken(t, mailDriver.htmlBody+"\n"+mailDriver.textBody)
	verifySignupToken(t, h, token)
	userID := requireCloudSignupUser(t, store, "maria@example.com")

	teams, activeTenantID, err := store.ListUserTeams(context.Background(), userID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected one team, got %d", len(teams))
	}
	if teams[0].Name != "Equipo de María" {
		t.Fatalf("expected localized team name %q, got %q", "Equipo de María", teams[0].Name)
	}
	if activeTenantID != teams[0].ID {
		t.Fatalf("expected active team %s, got %s", teams[0].ID, activeTenantID)
	}

	prefs, err := store.GetUserPreferences(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user preferences: %v", err)
	}
	if prefs == nil || prefs.DefaultLocale != "es" {
		t.Fatalf("expected default locale es, got %+v", prefs)
	}
}

func extractCloudSignupToken(t *testing.T, body string) string {
	t.Helper()

	const prefix = "/api/cloud/signup/verify?token="
	idx := strings.Index(body, prefix)
	if idx == -1 {
		t.Fatalf("expected cloud signup verification link in body, got:\n%s", body)
	}

	start := idx + len(prefix)
	end := start
	for end < len(body) {
		c := body[end]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			break
		}
		end++
	}
	token := body[start:end]
	if len(token) != 64 {
		t.Fatalf("expected 64-character verification token, got %q", token)
	}
	return token
}

func TestHandleSignupRejectsWithoutTos(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	body, err := json.Marshal(signupRequest{
		Email:        "notos@example.com",
		Password:     "password123",
		TeamName:     "No ToS Team",
		Jurisdiction: "EU",
		AcceptedTos:  false,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSignup().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleVerifySignupCreatesAccount(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	// Pre-create a pending signup token directly
	token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{
		Email:          "verify@example.com",
		HashedPassword: "$2a$10$testhashedpassword",
		GivenName:      "Verify",
		LastName:       "User",
		TeamName:       "Verify Team",
		Jurisdiction:   "EU",
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token="+token, nil)
	w := httptest.NewRecorder()
	h.handleVerifySignup().ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect status %d, got %d: %s", http.StatusFound, w.Code, w.Body.String())
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "/dashboard") {
		t.Fatalf("expected redirect to dashboard, got %q", location)
	}

	// User and billing account should now exist
	user, err := store.GetUserByEmail(context.Background(), "verify@example.com")
	if err != nil {
		t.Fatalf("get created user: %v", err)
	}
	if user == nil {
		t.Fatal("expected created user after verification")
	}

	teams, _, err := store.ListUserTeams(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected one team, got %d", len(teams))
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), teams[0].ID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if billingAccount.PlanCode != database.CloudPlanFree || billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusFree {
		t.Fatalf("unexpected billing account: %+v", billingAccount)
	}

	var verifiedEvents int
	if err := store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM cloud_conversion_events WHERE event_name = ?`, database.CloudConversionSignupVerified).Scan(&verifiedEvents); err != nil {
		t.Fatalf("count signup verification conversions: %v", err)
	}
	if verifiedEvents != 1 {
		t.Fatalf("signup verification conversions = %d, want 1", verifiedEvents)
	}

	repeatReq := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token="+token, nil)
	repeatResponse := httptest.NewRecorder()
	h.handleVerifySignup().ServeHTTP(repeatResponse, repeatReq)
	if err := store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM cloud_conversion_events WHERE event_name = ?`, database.CloudConversionSignupVerified).Scan(&verifiedEvents); err != nil {
		t.Fatalf("count repeated signup verification conversions: %v", err)
	}
	if verifiedEvents != 1 {
		t.Fatalf("repeated verification recorded %d signup verification conversions, want 1", verifiedEvents)
	}
}

func TestHandleVerifySignupWithExistingDefaultTeamDoesNotJoinDefaultTeam(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	defaultOwnerID, defaultTenantID := createDefaultOwner(t, store)
	requireTenantMembership(t, store, defaultTenantID, defaultOwnerID, true, "seeded owner")

	token := createPendingPublicSignupToken(t, store, "public-signup@example.com")
	verifySignupToken(t, h, token)
	userID := requireCloudSignupUser(t, store, "public-signup@example.com")

	requireTenantMembership(t, store, defaultTenantID, userID, false, "public signup user")
	requireSingleNonDefaultActiveTeam(t, store, userID, defaultTenantID)
}

func createDefaultOwner(t *testing.T, store *database.Store) (uuid.UUID, uuid.UUID) {
	t.Helper()
	defaultOwnerID, err := store.CreateUser(context.Background(), "default-owner@example.com", "hashed")
	if err != nil {
		t.Fatalf("create default owner: %v", err)
	}
	defaultTenantID, err := store.GetDefaultTenantID(context.Background())
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	return defaultOwnerID, defaultTenantID
}

func createPendingPublicSignupToken(t *testing.T, store *database.Store, email string) string {
	t.Helper()
	token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{
		Email:          email,
		HashedPassword: "$2a$10$testhashedpassword",
		TeamName:       "Public Signup Team",
		Jurisdiction:   "EU",
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}
	return token
}

func verifySignupToken(t *testing.T, h *handler, token string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token="+token, nil)
	w := httptest.NewRecorder()
	h.handleVerifySignup().ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect status %d, got %d: %s", http.StatusFound, w.Code, w.Body.String())
	}
}

func requireCloudSignupUser(t *testing.T, store *database.Store, email string) uuid.UUID {
	t.Helper()
	user, err := store.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("get created user: %v", err)
	}
	if user == nil {
		t.Fatal("expected public signup user to be created")
	}
	return user.ID
}

func requireTenantMembership(t *testing.T, store *database.Store, tenantID, userID uuid.UUID, want bool, label string) {
	t.Helper()
	isMember, err := store.IsTenantMember(context.Background(), tenantID, userID)
	if err != nil {
		t.Fatalf("check %s tenant membership: %v", label, err)
	}
	if isMember != want {
		t.Fatalf("expected %s default membership %v, got %v", label, want, isMember)
	}
}

func requireSingleNonDefaultActiveTeam(t *testing.T, store *database.Store, userID, defaultTenantID uuid.UUID) {
	t.Helper()
	teams, activeTenantID, err := store.ListUserTeams(context.Background(), userID)
	if err != nil {
		t.Fatalf("list public signup teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected one public signup team, got %d", len(teams))
	}
	if teams[0].ID == defaultTenantID {
		t.Fatalf("expected public signup team not to be default team %s", defaultTenantID)
	}
	if activeTenantID != teams[0].ID {
		t.Fatalf("expected active team %s, got %s", teams[0].ID, activeTenantID)
	}
}

func TestHandleVerifySignupInvalidToken(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token=invalid", nil)
	w := httptest.NewRecorder()
	h.handleVerifySignup().ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect status %d, got %d", http.StatusFound, w.Code)
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "error=expired") {
		t.Fatalf("expected redirect to signup with error=expired, got %q", location)
	}
}

func TestHandleSignupPreservesPaidAnnualIntentWithoutStartingCheckout(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	body, err := json.Marshal(signupRequest{
		Email:           "pro@example.com",
		Password:        "password123",
		GivenName:       "Pro",
		LastName:        "User",
		TeamName:        "Pro Team",
		PlanCode:        database.CloudPlanPro,
		BillingInterval: database.CloudBillingIntervalAnnual,
		Jurisdiction:    "EU",
		Locale:          "de-DE",
		AcceptedTos:     true,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/signup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleSignup().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp signupResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "verification_sent" {
		t.Fatalf("expected status verification_sent, got %q", resp.Status)
	}
	if resp.PlanCode != database.CloudPlanPro {
		t.Fatalf("expected pro plan code, got %q", resp.PlanCode)
	}
	if resp.BillingInterval != database.CloudBillingIntervalAnnual {
		t.Fatalf("expected annual billing interval, got %q", resp.BillingInterval)
	}

	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok {
		t.Fatal("expected fake stripe client")
	}
	if stripeClient.lastCheckoutInput != nil {
		t.Fatal("expected no checkout session to be created during signup")
	}
	if stripeClient.lastCustomerInput != nil {
		t.Fatal("expected no stripe customer to be created during signup")
	}
}

func TestHandleVerifySignupRedirectsPaidIntentToCheckoutBridge(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	token, err := store.CreatePendingSignup(context.Background(), database.PendingSignupEntry{
		Email:           "annual-pro@example.com",
		HashedPassword:  "$2a$10$testhashedpassword",
		TeamName:        "Annual Pro Team",
		Jurisdiction:    "EU",
		Locale:          "en",
		PlanCode:        database.CloudPlanPro,
		BillingInterval: database.CloudBillingIntervalAnnual,
	})
	if err != nil {
		t.Fatalf("create pending signup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/signup/verify?token="+token, nil)
	w := httptest.NewRecorder()
	h.handleVerifySignup().ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect status %d, got %d: %s", http.StatusFound, w.Code, w.Body.String())
	}
	location := w.Header().Get("Location")
	if location != "https://cloud.hitkeep.eu/signup/verified?billing=annual&plan=pro" {
		t.Fatalf("expected paid checkout bridge redirect, got %q", location)
	}
}

func TestHandleStripeEventUpdatesBillingAccount(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "webhook@example.com",
		HashedPassword: "hashed",
		TeamName:       "Webhook Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	event := stripe.Event{
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"status":"complete"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if billingAccount.StripeCustomerID != "cus_live" || billingAccount.StripeSubscriptionID != "sub_live" {
		t.Fatalf("unexpected billing account: %+v", billingAccount)
	}

	storedEvent, err := store.GetCloudBillingEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("get stored cloud billing event: %v", err)
	}
	if storedEvent.ProcessingStatus != database.CloudBillingEventStatusDone {
		t.Fatalf("expected processed billing event, got %+v", storedEvent)
	}
}

func TestHandleStripeEventCheckoutSessionWithoutExpandedRefsDoesNotPanic(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "webhook-minimal@example.com",
		HashedPassword: "hashed",
		TeamName:       "Webhook Minimal Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_checkout_minimal",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},
				"customer":null,
				"subscription":null,
				"status":"complete"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != "complete" {
		t.Fatalf("expected complete status, got %+v", billingAccount)
	}
	if billingAccount.StripeCustomerID != "" || billingAccount.StripeSubscriptionID != "" {
		t.Fatalf("expected empty stripe refs for minimal session, got %+v", billingAccount)
	}
}

func TestHandleStripeEventIsIdempotent(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "webhook-duplicate@example.com",
		HashedPassword: "hashed",
		TeamName:       "Webhook Duplicate Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_duplicate",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"status":"complete"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("first handle stripe event: %v", err)
	}
	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("second handle stripe event: %v", err)
	}

	storedEvent, err := store.GetCloudBillingEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("get stored cloud billing event: %v", err)
	}
	if storedEvent.ProcessingStatus != database.CloudBillingEventStatusDone {
		t.Fatalf("expected processed billing event, got %+v", storedEvent)
	}
}

func TestHandleStripeEventIgnoresOtherJurisdictions(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "webhook-us@example.com",
		HashedPassword: "hashed",
		TeamName:       "Webhook US Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_us_only",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro","jurisdiction":"US"},
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"status":"complete"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	if _, err := store.GetCloudBillingEvent(context.Background(), event.ID); !errors.Is(err, database.ErrCloudBillingEventNotFound) {
		t.Fatalf("expected foreign-jurisdiction event to be ignored, got %v", err)
	}
}

func TestHandleStripeEventMarksPaymentFailuresPastDue(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "invoice-failed@example.com",
		HashedPassword: "hashed",
		TeamName:       "Invoice Failed Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:             account.TenantID,
		PlanCode:             database.CloudPlanPro,
		PlanName:             "Pro",
		SubscriptionStatus:   database.CloudSubscriptionStatusActive,
		StripeCustomerID:     "cus_failed",
		StripeSubscriptionID: "sub_failed",
		StripePriceID:        "price_pro",
	}); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_invoice_failed",
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"customer":{"id":"cus_failed"},
				"subscription":{"id":"sub_failed"}
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle invoice.payment_failed: %v", err)
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusPastDue {
		t.Fatalf("expected past_due status, got %+v", billingAccount)
	}
}

func TestHandleStripeEventMarksDisputes(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "dispute@example.com",
		HashedPassword: "hashed",
		TeamName:       "Dispute Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:             account.TenantID,
		PlanCode:             database.CloudPlanBusiness,
		PlanName:             "Business",
		SubscriptionStatus:   database.CloudSubscriptionStatusActive,
		StripeCustomerID:     "cus_dispute",
		StripeSubscriptionID: "sub_dispute",
		StripePriceID:        "price_business",
	}); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}

	created := stripe.Event{
		ID:   "evt_dispute_created",
		Type: "charge.dispute.created",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"charge":{"id":"ch_dispute"},
				"status":"needs_response"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), created); err != nil {
		t.Fatalf("handle charge.dispute.created: %v", err)
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get billing account after dispute create: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusDisputed {
		t.Fatalf("expected disputed status, got %+v", billingAccount)
	}

	closed := stripe.Event{
		ID:   "evt_dispute_closed",
		Type: "charge.dispute.closed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"charge":{"id":"ch_dispute"},
				"status":"lost"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), closed); err != nil {
		t.Fatalf("handle charge.dispute.closed: %v", err)
	}

	billingAccount, err = store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get billing account after dispute close: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusChargebackLost {
		t.Fatalf("expected chargeback_lost status, got %+v", billingAccount)
	}

	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok {
		t.Fatal("expected fake stripe client")
	}
	if stripeClient.lastChargeID != "ch_dispute" {
		t.Fatalf("expected disputed charge lookup, got %q", stripeClient.lastChargeID)
	}
}

func TestHandleStripeWebhookSignedSubscriptionLifecycle(t *testing.T) {
	h, store, _ := setupSignedCloudWebhookTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "signed-webhook@example.com",
		HashedPassword: "hashed",
		TeamName:       "Signed Webhook Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: subscriptionStatusPending,
		StripeCustomerID:   "cus_live",
		StripePriceID:      "price_pro",
	}); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}

	activate := postSignedStripeWebhook(t, h, "evt_sub_created", "customer.subscription.created", map[string]any{
		"id": "sub_live",
		"metadata": map[string]string{
			"tenant_id":    account.TenantID.String(),
			"plan_code":    database.CloudPlanPro,
			"plan_name":    "Pro",
			"jurisdiction": "EU",
		},
		"status":   subscriptionStatusActive,
		"customer": map[string]string{"id": "cus_live"},
		"items": map[string]any{
			"data": []map[string]any{
				{"price": map[string]string{"id": "price_pro"}},
			},
		},
	})
	if activate.Code != http.StatusOK {
		t.Fatalf("expected activation webhook status %d, got %d: %s", http.StatusOK, activate.Code, activate.Body.String())
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get activated billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != subscriptionStatusActive {
		t.Fatalf("expected active subscription status, got %+v", billingAccount)
	}
	if billingAccount.StripeSubscriptionID != "sub_live" || billingAccount.StripePriceID != "price_pro" {
		t.Fatalf("expected subscription identifiers to be stored, got %+v", billingAccount)
	}
	conversionEvents, err := store.ListCloudConversionEvents(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("list subscription conversion events: %v", err)
	}
	if len(conversionEvents) != 1 || conversionEvents[0].EventName != database.CloudConversionSubscriptionActivated || conversionEvents[0].PlanCode != database.CloudPlanPro || conversionEvents[0].BillingInterval != database.CloudBillingIntervalMonthly {
		t.Fatalf("expected one Pro monthly subscription activation, got %+v", conversionEvents)
	}

	replayed := postSignedStripeWebhook(t, h, "evt_sub_created", "customer.subscription.created", map[string]any{
		"id": "sub_live",
		"metadata": map[string]string{
			"tenant_id": account.TenantID.String(),
			"plan_code": database.CloudPlanPro,
			"plan_name": "Pro",
		},
		"status":   subscriptionStatusActive,
		"customer": map[string]string{"id": "cus_live"},
		"items": map[string]any{
			"data": []map[string]any{
				{"price": map[string]string{"id": "price_pro"}},
			},
		},
	})
	if replayed.Code != http.StatusOK {
		t.Fatalf("expected replayed webhook status %d, got %d: %s", http.StatusOK, replayed.Code, replayed.Body.String())
	}

	failedPayment := postSignedStripeWebhook(t, h, "evt_invoice_failed_signed", "invoice.payment_failed", map[string]any{
		"customer":     map[string]string{"id": "cus_live"},
		"subscription": map[string]string{"id": "sub_live"},
		"metadata": map[string]string{
			"tenant_id": account.TenantID.String(),
		},
	})
	if failedPayment.Code != http.StatusOK {
		t.Fatalf("expected invoice.payment_failed webhook status %d, got %d: %s", http.StatusOK, failedPayment.Code, failedPayment.Body.String())
	}

	billingAccount, err = store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get past-due billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusPastDue {
		t.Fatalf("expected past_due status, got %+v", billingAccount)
	}

	storedEvent, err := store.GetCloudBillingEvent(context.Background(), "evt_sub_created")
	if err != nil {
		t.Fatalf("get replayed billing event: %v", err)
	}
	if storedEvent.ProcessingStatus != database.CloudBillingEventStatusDone {
		t.Fatalf("expected replayed billing event to remain processed, got %+v", storedEvent)
	}
}

func TestHandleStripeWebhookSignedChargeDisputeLifecycle(t *testing.T) {
	h, store, stripeClient := setupSignedCloudWebhookTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "signed-dispute@example.com",
		HashedPassword: "hashed",
		TeamName:       "Signed Dispute Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	stripeClient.chargeCustomerID = "cus_dispute_signed"
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:             account.TenantID,
		PlanCode:             database.CloudPlanBusiness,
		PlanName:             "Business",
		SubscriptionStatus:   database.CloudSubscriptionStatusActive,
		StripeCustomerID:     "cus_dispute_signed",
		StripeSubscriptionID: "sub_dispute_signed",
		StripePriceID:        "price_business",
	}); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}

	disputed := postSignedStripeWebhook(t, h, "evt_dispute_created_signed", "charge.dispute.created", map[string]any{
		"charge": map[string]string{"id": "ch_dispute_signed"},
		"status": "needs_response",
	})
	if disputed.Code != http.StatusOK {
		t.Fatalf("expected dispute.created webhook status %d, got %d: %s", http.StatusOK, disputed.Code, disputed.Body.String())
	}

	billingAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get disputed billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusDisputed {
		t.Fatalf("expected disputed status, got %+v", billingAccount)
	}

	lost := postSignedStripeWebhook(t, h, "evt_dispute_closed_lost_signed", "charge.dispute.closed", map[string]any{
		"charge": map[string]string{"id": "ch_dispute_signed"},
		"status": "lost",
	})
	if lost.Code != http.StatusOK {
		t.Fatalf("expected dispute.closed lost webhook status %d, got %d: %s", http.StatusOK, lost.Code, lost.Body.String())
	}

	billingAccount, err = store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get lost-dispute billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != database.CloudSubscriptionStatusChargebackLost {
		t.Fatalf("expected chargeback_lost status, got %+v", billingAccount)
	}

	won := postSignedStripeWebhook(t, h, "evt_dispute_closed_won_signed", "charge.dispute.closed", map[string]any{
		"charge": map[string]string{"id": "ch_dispute_signed"},
		"status": "won",
	})
	if won.Code != http.StatusOK {
		t.Fatalf("expected dispute.closed won webhook status %d, got %d: %s", http.StatusOK, won.Code, won.Body.String())
	}

	billingAccount, err = store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get won-dispute billing account: %v", err)
	}
	if billingAccount.SubscriptionStatus != subscriptionStatusActive {
		t.Fatalf("expected active status after won dispute, got %+v", billingAccount)
	}

	if stripeClient.lastChargeID != "ch_dispute_signed" {
		t.Fatalf("expected signed dispute charge lookup, got %q", stripeClient.lastChargeID)
	}
}

func TestStripeWebhookSDKUsesConfiguredAPIVersion(t *testing.T) {
	t.Parallel()

	payload := []byte(fmt.Sprintf(`{
		"id":"evt_test_configured_api",
		"object":"event",
		"api_version":%q,
		"type":"invoice.payment_failed",
		"data":{"object":{"customer":"cus_test","subscription":"sub_test"}}
	}`, stripeAPIVersion))

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("whsec_test_123"))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	signature := fmt.Sprintf("t=%s,v1=%x", timestamp, mac.Sum(nil))

	event, err := stripeWebhookSDK{}.ConstructEvent(payload, signature, "whsec_test_123")
	if err != nil {
		t.Fatalf("construct event: %v", err)
	}
	if event.Type != "invoice.payment_failed" {
		t.Fatalf("expected invoice.payment_failed, got %q", event.Type)
	}
	if !strings.Contains(string(event.Data.Raw), `"customer":"cus_test"`) {
		t.Fatalf("expected raw invoice payload to be preserved, got %s", string(event.Data.Raw))
	}
	if event.APIVersion != stripeAPIVersion {
		t.Fatalf("expected event api version %q, got %q", stripeAPIVersion, event.APIVersion)
	}
}

func TestStripeCustomerIDHandlesNil(t *testing.T) {
	t.Parallel()

	if got := stripeCustomerID(nil); got != "" {
		t.Fatalf("expected empty customer id for nil customer, got %q", got)
	}
	if got := stripeCustomerID(&stripe.Customer{ID: "cus_test"}); got != "cus_test" {
		t.Fatalf("expected cus_test, got %q", got)
	}
}

func TestSetStripeVersionHeader(t *testing.T) {
	t.Parallel()

	params := &stripe.Params{}
	setStripeVersionHeader(params)

	if got := params.Headers.Get("Stripe-Version"); got != stripeAPIVersion {
		t.Fatalf("expected stripe version header %q, got %q", stripeAPIVersion, got)
	}
}

func TestSetStripeIdempotencyKey(t *testing.T) {
	t.Parallel()

	params := &stripe.Params{}
	setStripeIdempotencyKey(params, " hitkeep:test:key ")

	if params.IdempotencyKey == nil {
		t.Fatal("expected idempotency key to be set")
	}
	if got := *params.IdempotencyKey; got != "hitkeep:test:key" {
		t.Fatalf("expected trimmed idempotency key, got %q", got)
	}
}

func TestHandleCreateBillingPortalSession(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "https://cloud.hitkeep.eu/hitkeep/"

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "portal@example.com",
		HashedPassword: "hashed",
		TeamName:       "Portal Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: "active",
		StripeCustomerID:   "cus_portal",
	}); err != nil {
		t.Fatalf("upsert cloud billing account: %v", err)
	}

	body, err := json.Marshal(billingPortalSessionRequest{Locale: "fr-FR"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/portal", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, account.UserID))
	w := httptest.NewRecorder()

	h.handleCreateBillingPortalSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp billingPortalSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.URL != "https://billing.stripe.test/session" {
		t.Fatalf("unexpected billing portal url %q", resp.URL)
	}

	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok || stripeClient.lastPortalInput == nil {
		t.Fatal("expected billing portal input to be captured")
	}
	if stripeClient.lastPortalInput.Locale != "fr" {
		t.Fatalf("expected billing portal locale fr, got %q", stripeClient.lastPortalInput.Locale)
	}
	if stripeClient.lastPortalInput.ReturnURL != "https://cloud.hitkeep.eu/hitkeep/admin/team" {
		t.Fatalf("expected prefixed billing portal return URL, got %q", stripeClient.lastPortalInput.ReturnURL)
	}
}

func TestHandleCreateBillingCheckoutSession(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "https://cloud.hitkeep.eu/hitkeep/"

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "upgrade@example.com",
		HashedPassword: "hashed",
		GivenName:      "Ada",
		LastName:       "Lovelace",
		TeamName:       "Upgrade Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanFree,
		PlanName:           "Free",
		SubscriptionStatus: database.CloudSubscriptionStatusFree,
	}); err != nil {
		t.Fatalf("upsert cloud billing account: %v", err)
	}

	body, err := json.Marshal(billingCheckoutSessionRequest{
		PlanCode:        database.CloudPlanPro,
		BillingInterval: database.CloudBillingIntervalAnnual,
		Locale:          "de-DE",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/checkout", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, account.UserID))
	w := httptest.NewRecorder()

	h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp billingCheckoutSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.URL != "https://checkout.stripe.test/session" {
		t.Fatalf("unexpected billing checkout url %q", resp.URL)
	}

	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok {
		t.Fatal("expected fake stripe client")
	}
	if stripeClient.lastCustomerInput == nil {
		t.Fatal("expected stripe customer create input to be captured")
	}
	if stripeClient.lastCustomerInput.Email != "upgrade@example.com" {
		t.Fatalf("expected customer email upgrade@example.com, got %q", stripeClient.lastCustomerInput.Email)
	}
	if stripeClient.lastCheckoutInput == nil {
		t.Fatal("expected stripe checkout input to be captured")
	}
	if stripeClient.lastCheckoutInput.Locale != "de" {
		t.Fatalf("expected checkout locale de, got %q", stripeClient.lastCheckoutInput.Locale)
	}
	if stripeClient.lastCheckoutInput.PlanCode != database.CloudPlanPro {
		t.Fatalf("expected checkout plan %q, got %q", database.CloudPlanPro, stripeClient.lastCheckoutInput.PlanCode)
	}
	if stripeClient.lastCheckoutInput.BillingInterval != database.CloudBillingIntervalAnnual {
		t.Fatalf("expected annual checkout interval, got %q", stripeClient.lastCheckoutInput.BillingInterval)
	}
	if stripeClient.lastCheckoutInput.PriceID != "price_pro_annual" {
		t.Fatalf("expected annual Pro price, got %q", stripeClient.lastCheckoutInput.PriceID)
	}
	if stripeClient.lastCheckoutInput.SuccessURL != "https://cloud.hitkeep.eu/hitkeep/admin/team?checkout=success" {
		t.Fatalf("expected prefixed checkout success URL, got %q", stripeClient.lastCheckoutInput.SuccessURL)
	}
	if stripeClient.lastCheckoutInput.CancelURL != "https://cloud.hitkeep.eu/hitkeep/admin/team?checkout=canceled" {
		t.Fatalf("expected prefixed checkout cancel URL, got %q", stripeClient.lastCheckoutInput.CancelURL)
	}

	storedAccount, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get cloud billing account: %v", err)
	}
	if storedAccount.SubscriptionStatus != subscriptionStatusPending {
		t.Fatalf("expected subscription status %q, got %q", subscriptionStatusPending, storedAccount.SubscriptionStatus)
	}
	if storedAccount.PlanCode != database.CloudPlanFree {
		t.Fatalf("expected persisted plan code %q, got %q", database.CloudPlanFree, storedAccount.PlanCode)
	}
	if storedAccount.StripeCustomerID != "cus_test" {
		t.Fatalf("expected persisted customer id cus_test, got %q", storedAccount.StripeCustomerID)
	}
	if storedAccount.StripePriceID != "price_pro_annual" {
		t.Fatalf("expected persisted price id price_pro_annual, got %q", storedAccount.StripePriceID)
	}
	if storedAccount.BillingInterval != database.CloudBillingIntervalAnnual {
		t.Fatalf("expected persisted annual interval, got %q", storedAccount.BillingInterval)
	}
}

func TestHandleCreateBillingCheckoutSessionDoesNotLogRawStripeError(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "stripe-failure@example.com",
		HashedPassword: "hashed",
		GivenName:      "Ada",
		LastName:       "Lovelace",
		TeamName:       "Stripe Failure Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanFree,
		PlanName:           "Free",
		SubscriptionStatus: database.CloudSubscriptionStatusFree,
		StripeCustomerID:   "cus_existing",
	}); err != nil {
		t.Fatalf("upsert cloud billing account: %v", err)
	}

	const rawStripeError = "stripe response contains sk_test_checkout_secret and https://billing.example.test/secret"
	stripeClient, ok := h.stripe.(*fakeStripeClient)
	if !ok {
		t.Fatal("expected fake stripe client")
	}
	stripeClient.createCheckoutErr = fmt.Errorf("checkout failed: %w", &stripe.Error{
		Type:           stripe.ErrorTypeAPI,
		Code:           stripe.ErrorCodeCustomerSessionExpired,
		Msg:            rawStripeError,
		HTTPStatusCode: http.StatusBadGateway,
	})

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	body := strings.NewReader(`{"plan_code":"pro","billing":"monthly","locale":"en-US"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/checkout", body)
	ctx := shared.WithLogger(req.Context(), logger)
	ctx = context.WithValue(ctx, shared.UserIDKey, account.UserID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if strings.Contains(logs.String(), rawStripeError) {
		t.Fatalf("raw Stripe error leaked into logs: %q", logs.String())
	}
	if !strings.Contains(logs.String(), "stripe.error_code=stripe_request_failed") ||
		!strings.Contains(logs.String(), "stripe.error_kind=provider") ||
		!strings.Contains(logs.String(), "stripe.stripe_error_type=api_error") ||
		!strings.Contains(logs.String(), "stripe.stripe_error_code=customer_session_expired") ||
		!strings.Contains(logs.String(), "stripe.stripe_http_status=502") {
		t.Fatalf("expected safe Stripe diagnostics, got %q", logs.String())
	}
}

func TestHandleCreateBillingCheckoutSessionRejectsPaidTeams(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "paid@example.com",
		HashedPassword: "hashed",
		TeamName:       "Paid Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: subscriptionStatusActive,
		StripeCustomerID:   "cus_existing",
	}); err != nil {
		t.Fatalf("upsert cloud billing account: %v", err)
	}

	body, err := json.Marshal(billingCheckoutSessionRequest{PlanCode: database.CloudPlanBusiness})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/billing/checkout", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), shared.UserIDKey, account.UserID))
	w := httptest.NewRecorder()

	h.handleCreateBillingCheckoutSession().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestNormalizeStripeLocale(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty falls back to auto", in: "", want: "auto"},
		{name: "base locale passes through", in: "de", want: "de"},
		{name: "region locale maps to base", in: "fr-FR", want: "fr"},
		{name: "supported regional locale preserved", in: "pt-BR", want: "pt-BR"},
		{name: "unsupported locale falls back to auto", in: "ga-IE", want: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeStripeLocale(tt.in); got != tt.want {
				t.Fatalf("normalizeStripeLocale(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

//go:build billing

package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v84"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	serverauth "hitkeep/internal/server/auth"
	"hitkeep/internal/server/shared"
)

const (
	checkoutModeSubscription   = "subscription"
	subscriptionStatusActive   = "active"
	subscriptionStatusPending  = "pending_checkout"
	subscriptionStatusCanceled = "canceled"
	stripeAPIVersion           = "2026-02-25.clover"
)

type stripeClient interface {
	CreateCustomer(context.Context, createCustomerInput) (string, error)
	CreateCheckoutSession(context.Context, createCheckoutSessionInput) (*checkoutSessionOutput, error)
	CreatePortalSession(context.Context, createPortalSessionInput) (*portalSessionOutput, error)
	GetCharge(context.Context, string) (*stripeChargeOutput, error)
}

type stripeWebhookVerifier interface {
	ConstructEvent(payload []byte, header string, secret string) (stripe.Event, error)
}

type handler struct {
	ctx      *shared.Context
	stripe   stripeClient
	webhooks stripeWebhookVerifier
}

type createCustomerInput struct {
	Email          string
	Name           string
	UserID         uuid.UUID
	TenantID       uuid.UUID
	PlanCode       string
	Jurisdiction   string
	IdempotencyKey string
}

type createCheckoutSessionInput struct {
	CustomerID   string
	PriceID      string
	SuccessURL   string
	CancelURL    string
	Locale       string
	UserID       uuid.UUID
	TenantID     uuid.UUID
	PlanCode     string
	PlanName     string
	Jurisdiction string
	Email        string
}

type checkoutSessionOutput struct {
	ID  string
	URL string
}

type createPortalSessionInput struct {
	CustomerID      string
	ConfigurationID string
	ReturnURL       string
	Locale          string
}

type portalSessionOutput struct {
	ID  string
	URL string
}

type stripeChargeOutput struct {
	ID         string
	CustomerID string
}

type stripeSDKClient struct {
	client *stripe.Client
}

type stripeWebhookSDK struct{}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{
		ctx:      ctx,
		stripe:   newStripeSDKClient(ctx.Config.StripeSecretKey),
		webhooks: stripeWebhookSDK{},
	}

	mux.HandleFunc("POST /api/cloud/signup", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleSignup()))
	mux.HandleFunc("GET /api/cloud/signup/verify", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.AuthLimiter,
	}, h.handleVerifySignup()))
	mux.HandleFunc("POST /api/cloud/billing/portal", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateBillingPortalSession()))
	mux.HandleFunc("POST /api/cloud/billing/checkout", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateBillingCheckoutSession()))
	mux.HandleFunc("GET /api/cloud/plans", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListCloudPlans()))
	mux.HandleFunc("POST /api/cloud/webhooks/stripe", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.WebhookLimiter,
	}, h.handleStripeWebhook()))
}

type signupRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	GivenName    string `json:"given_name"`
	LastName     string `json:"last_name"`
	TeamName     string `json:"team_name"`
	PlanCode     string `json:"plan_code"`
	Jurisdiction string `json:"jurisdiction"`
	Locale       string `json:"locale"`
	AcceptedTos  bool   `json:"accepted_tos"`
}

type signupResponse struct {
	Status      string `json:"status"`
	PlanCode    string `json:"plan_code"`
	RedirectURL string `json:"redirect_url,omitempty"`
	CheckoutURL string `json:"checkout_url,omitempty"`
}

type billingPortalSessionResponse struct {
	URL string `json:"url"`
}

type billingPortalSessionRequest struct {
	Locale string `json:"locale"`
}

type billingCheckoutSessionRequest struct {
	PlanCode string `json:"plan_code"`
	Locale   string `json:"locale"`
}

type billingCheckoutSessionResponse struct {
	URL string `json:"url"`
}

func (h *handler) handleSignup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted || !h.ctx.Config.CloudSignupEnabled {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		var req signupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.GivenName = strings.TrimSpace(req.GivenName)
		req.LastName = strings.TrimSpace(req.LastName)
		req.TeamName = strings.TrimSpace(req.TeamName)
		req.PlanCode = database.CloudPlanFree // signup always starts on free
		req.Jurisdiction = strings.TrimSpace(strings.ToUpper(req.Jurisdiction))
		req.Locale = normalizeStripeLocale(req.Locale)

		if req.Email == "" || len(req.Password) < 8 {
			http.Error(w, "Email required; Password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		if req.TeamName == "" {
			http.Error(w, "Team name is required", http.StatusBadRequest)
			return
		}
		if !req.AcceptedTos {
			http.Error(w, "You must accept the Terms of Service and Privacy Policy", http.StatusBadRequest)
			return
		}
		if configuredJurisdiction := normalizeJurisdiction(h.ctx.Config.CloudJurisdiction); configuredJurisdiction != "" && req.Jurisdiction != "" && normalizeJurisdiction(req.Jurisdiction) != configuredJurisdiction {
			http.Error(w, "Jurisdiction mismatch", http.StatusBadRequest)
			return
		}

		hashedPassword, err := serverauth.HashPassword(req.Password)
		if err != nil {
			slog.Error("Failed to hash cloud signup password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		existing, _ := h.ctx.Store.GetUserByEmail(r.Context(), req.Email)
		if existing != nil {
			http.Error(w, "Email already exists", http.StatusConflict)
			return
		}

		token, err := h.ctx.Store.CreatePendingSignup(r.Context(), database.PendingSignupEntry{
			Email:          req.Email,
			HashedPassword: hashedPassword,
			GivenName:      req.GivenName,
			LastName:       req.LastName,
			TeamName:       req.TeamName,
			Jurisdiction:   req.Jurisdiction,
			Locale:         req.Locale,
			AcceptedTosAt:  time.Now().UTC(),
		})
		if err != nil {
			slog.Error("Failed to create pending signup token", "error", err, "email", req.Email)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		verifyLink := strings.TrimRight(h.ctx.Config.PublicURL, "/") + "/api/cloud/signup/verify?token=" + token
		if err := h.ctx.Mailer.Send(req.Email, mailables.NewEmailVerification(verifyLink, req.TeamName, req.Locale)); err != nil {
			slog.Error("Failed to send verification email", "error", err, "email", req.Email)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resp := signupResponse{
			Status:   "verification_sent",
			PlanCode: database.CloudPlanFree,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode cloud signup response", "error", err)
		}
	}
}

func (h *handler) handleVerifySignup() http.HandlerFunc {
	signupURL := func(errorCode string) string {
		return strings.TrimRight(h.ctx.Config.PublicURL, "/") + "/signup?error=" + errorCode
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted || !h.ctx.Config.CloudSignupEnabled {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			http.Redirect(w, r, signupURL("expired"), http.StatusFound)
			return
		}

		entry, err := h.ctx.Store.CompletePendingSignup(r.Context(), token)
		if err != nil {
			slog.Warn("Signup verification failed", "error", err, "token_prefix", safeTokenPrefix(token))
			http.Redirect(w, r, signupURL("expired"), http.StatusFound)
			return
		}

		account, err := h.ctx.Store.CreateManagedCloudAccount(r.Context(), database.CreateManagedCloudAccountInput{
			Email:          entry.Email,
			HashedPassword: entry.HashedPassword,
			GivenName:      entry.GivenName,
			LastName:       entry.LastName,
			TeamName:       entry.TeamName,
		})
		if errors.Is(err, database.ErrUserEmailAlreadyExists) {
			http.Redirect(w, r, signupURL("exists"), http.StatusFound)
			return
		}
		if err != nil {
			slog.Error("Failed to create managed cloud account during verification", "error", err, "email", entry.Email)
			http.Redirect(w, r, signupURL("expired"), http.StatusFound)
			return
		}

		if err := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), database.CloudBillingAccount{
			TenantID:           account.TenantID,
			PlanCode:           database.CloudPlanFree,
			PlanName:           planNameForCode(database.CloudPlanFree),
			SubscriptionStatus: database.CloudSubscriptionStatusFree,
		}); err != nil {
			slog.Error("Failed to initialize cloud billing account", "error", err, "team_id", account.TenantID)
			http.Redirect(w, r, signupURL("expired"), http.StatusFound)
			return
		}

		if err := issueLoginSession(w, h.ctx.Config, account.UserID); err != nil {
			slog.Error("Failed to issue cloud signup login session", "error", err, "user_id", account.UserID)
			http.Redirect(w, r, signupURL("expired"), http.StatusFound)
			return
		}

		dashboardURL := strings.TrimRight(h.ctx.Config.PublicURL, "/") + "/dashboard"
		http.Redirect(w, r, dashboardURL, http.StatusFound)
	}
}

func (h *handler) handleListCloudPlans() http.HandlerFunc {
	plans := []api.CloudPlanTier{
		{
			Code: database.CloudPlanFree,
			Name: planNameForCode(database.CloudPlanFree),
			Entitlements: api.TeamEntitlements{
				MaxSitesPerTeam:     3,
				MaxTeamMembers:      3,
				MaxRetentionDays:    60,
				AllowSSO:            false,
				AllowCustomBranding: false,
			},
		},
		{
			Code: database.CloudPlanPro,
			Name: planNameForCode(database.CloudPlanPro),
			Entitlements: api.TeamEntitlements{
				MaxSitesPerTeam:     10,
				MaxTeamMembers:      5,
				MaxRetentionDays:    365,
				AllowSSO:            false,
				AllowCustomBranding: false,
			},
		},
		{
			Code: database.CloudPlanBusiness,
			Name: planNameForCode(database.CloudPlanBusiness),
			Entitlements: api.TeamEntitlements{
				MaxSitesPerTeam:     50,
				MaxTeamMembers:      20,
				MaxRetentionDays:    1095,
				AllowSSO:            true,
				AllowCustomBranding: true,
			},
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(plans); err != nil {
			slog.Error("Failed to encode cloud plans response", "error", err)
		}
	}
}

func safeTokenPrefix(token string) string {
	if len(token) > 8 {
		return token[:8] + "..."
	}
	return "***"
}

func (h *handler) createCheckout(ctx context.Context, account *database.ManagedCloudAccount, req signupRequest) (checkoutURL string, customerID string, sessionID string, priceID string, err error) {
	planName := planNameForCode(req.PlanCode)
	priceID = priceIDForPlan(h.ctx.Config, req.PlanCode)
	if priceID == "" {
		return "", "", "", "", fmt.Errorf("plan %s is not configured for checkout", req.PlanCode)
	}
	if strings.TrimSpace(h.ctx.Config.StripeSecretKey) == "" {
		return "", "", "", "", fmt.Errorf("stripe secret key is not configured")
	}

	displayName := strings.TrimSpace(strings.Join([]string{req.GivenName, req.LastName}, " "))
	customerID, err = h.stripe.CreateCustomer(ctx, createCustomerInput{
		Email:          req.Email,
		Name:           displayName,
		UserID:         account.UserID,
		TenantID:       account.TenantID,
		PlanCode:       req.PlanCode,
		Jurisdiction:   effectiveJurisdiction(h.ctx.Config, req.Jurisdiction),
		IdempotencyKey: stripeCustomerCreateIdempotencyKey(account.UserID, account.TenantID, req.Email),
	})
	if err != nil {
		return "", "", "", "", err
	}

	session, err := h.stripe.CreateCheckoutSession(ctx, createCheckoutSessionInput{
		CustomerID:   customerID,
		PriceID:      priceID,
		SuccessURL:   checkoutSuccessURL(h.ctx.Config),
		CancelURL:    checkoutCancelURL(h.ctx.Config),
		Locale:       normalizeStripeLocale(req.Locale),
		UserID:       account.UserID,
		TenantID:     account.TenantID,
		PlanCode:     req.PlanCode,
		PlanName:     planName,
		Jurisdiction: effectiveJurisdiction(h.ctx.Config, req.Jurisdiction),
		Email:        req.Email,
	})
	if err != nil {
		return "", "", "", "", err
	}

	return session.URL, customerID, session.ID, priceID, nil
}

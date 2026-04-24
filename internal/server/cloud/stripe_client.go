//go:build billing

package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
)

func (c *stripeSDKClient) CreateCustomer(ctx context.Context, input createCustomerInput) (string, error) {
	params := &stripe.CustomerCreateParams{
		Email: stripe.String(input.Email),
		Name:  stripe.String(strings.TrimSpace(input.Name)),
	}
	setStripeVersionHeader(&params.Params)
	setStripeIdempotencyKey(&params.Params, input.IdempotencyKey)
	params.AddMetadata("user_id", input.UserID.String())
	params.AddMetadata("tenant_id", input.TenantID.String())
	params.AddMetadata("plan_code", input.PlanCode)
	params.AddMetadata("jurisdiction", input.Jurisdiction)

	created, err := c.client.V1Customers.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create stripe customer: %w", err)
	}
	return created.ID, nil
}

func (c *stripeSDKClient) CreateCheckoutSession(ctx context.Context, input createCheckoutSessionInput) (*checkoutSessionOutput, error) {
	params := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String(checkoutModeSubscription),
		Customer:   stripe.String(input.CustomerID),
		SuccessURL: stripe.String(input.SuccessURL),
		CancelURL:  stripe.String(input.CancelURL),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Price:    stripe.String(input.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
		SubscriptionData: &stripe.CheckoutSessionCreateSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id":      input.UserID.String(),
				"tenant_id":    input.TenantID.String(),
				"plan_code":    input.PlanCode,
				"plan_name":    input.PlanName,
				"jurisdiction": input.Jurisdiction,
			},
		},
		CustomerUpdate: &stripe.CheckoutSessionCreateCustomerUpdateParams{
			Name:    stripe.String("auto"),
			Address: stripe.String("auto"),
		},
	}
	setStripeVersionHeader(&params.Params)
	params.Locale = stripe.String(input.Locale)
	params.AddMetadata("user_id", input.UserID.String())
	params.AddMetadata("tenant_id", input.TenantID.String())
	params.AddMetadata("plan_code", input.PlanCode)
	params.AddMetadata("plan_name", input.PlanName)
	params.AddMetadata("jurisdiction", input.Jurisdiction)
	params.AddMetadata("email", input.Email)

	created, err := c.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create checkout session: %w", err)
	}
	return &checkoutSessionOutput{
		ID:  created.ID,
		URL: created.URL,
	}, nil
}

func (c *stripeSDKClient) CreatePortalSession(ctx context.Context, input createPortalSessionInput) (*portalSessionOutput, error) {
	params := &stripe.BillingPortalSessionCreateParams{
		Customer:  stripe.String(input.CustomerID),
		ReturnURL: stripe.String(input.ReturnURL),
	}
	setStripeVersionHeader(&params.Params)
	params.Locale = stripe.String(input.Locale)
	if strings.TrimSpace(input.ConfigurationID) != "" {
		params.Configuration = stripe.String(strings.TrimSpace(input.ConfigurationID))
	}

	created, err := c.client.V1BillingPortalSessions.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create stripe billing portal session: %w", err)
	}
	return &portalSessionOutput{
		ID:  created.ID,
		URL: created.URL,
	}, nil
}

func (c *stripeSDKClient) GetCharge(ctx context.Context, chargeID string) (*stripeChargeOutput, error) {
	params := &stripe.ChargeRetrieveParams{}
	setStripeVersionHeader(&params.Params)

	loaded, err := c.client.V1Charges.Retrieve(ctx, strings.TrimSpace(chargeID), params)
	if err != nil {
		return nil, fmt.Errorf("get stripe charge: %w", err)
	}

	return &stripeChargeOutput{
		ID:         strings.TrimSpace(loaded.ID),
		CustomerID: stripeCustomerID(loaded.Customer),
	}, nil
}

func (stripeWebhookSDK) ConstructEvent(payload []byte, header string, secret string) (stripe.Event, error) {
	event, err := webhook.ConstructEventWithOptions(payload, header, secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return event, err
	}
	if strings.TrimSpace(event.APIVersion) != stripeAPIVersion {
		return event, fmt.Errorf("unexpected stripe event api version %s", strings.TrimSpace(event.APIVersion))
	}
	return event, nil
}

func stripeCustomerID(customer *stripe.Customer) string {
	if customer == nil {
		return ""
	}
	return strings.TrimSpace(customer.ID)
}

func newStripeSDKClient(secretKey string) *stripeSDKClient {
	return &stripeSDKClient{
		client: stripe.NewClient(strings.TrimSpace(secretKey)),
	}
}

func setStripeVersionHeader(params *stripe.Params) {
	if params == nil {
		return
	}
	if params.Headers == nil {
		params.Headers = make(http.Header)
	}
	params.Headers.Set("Stripe-Version", stripeAPIVersion)
}

func setStripeIdempotencyKey(params *stripe.Params, key string) {
	if params == nil {
		return
	}
	if trimmed := strings.TrimSpace(key); trimmed != "" {
		params.SetIdempotencyKey(trimmed)
	}
}

func stripeCustomerCreateIdempotencyKey(userID uuid.UUID, tenantID uuid.UUID, email string) string {
	return stripeIdempotencyKey("customer-create", tenantID.String(), userID.String(), strings.ToLower(strings.TrimSpace(email)))
}

func stripeIdempotencyKey(operation string, parts ...string) string {
	normalized := make([]string, 0, len(parts)+2)
	normalized = append(normalized, "hitkeep", strings.TrimSpace(operation))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return strings.Join(normalized, ":")
}

func supportedStripeLocale(tag string) (string, bool) {
	switch tag {
	case "auto", "bg", "cs", "da", "de", "el", "en", "en-GB", "es", "es-419", "et", "fi", "fil", "fr", "fr-CA", "hr", "hu", "id", "it", "ja", "ko", "lt", "lv", "ms", "mt", "nb", "nl", "pl", "pt", "pt-BR", "ro", "ru", "sk", "sl", "sv", "th", "tr", "vi", "zh", "zh-HK", "zh-TW":
		return tag, true
	default:
		return "", false
	}
}

func jurisdictionFromStripeEvent(event stripe.Event) (string, error) {
	switch string(event.Type) {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return "", fmt.Errorf("decode checkout.session.completed: %w", err)
		}
		return strings.TrimSpace(strings.ToUpper(session.Metadata["jurisdiction"])), nil
	case "customer.subscription.updated", "customer.subscription.deleted":
		var subscription stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
			return "", fmt.Errorf("decode subscription event: %w", err)
		}
		return strings.TrimSpace(strings.ToUpper(subscription.Metadata["jurisdiction"])), nil
	case "invoice.payment_failed":
		invoice, err := parseStripeInvoiceEvent(event.Data.Raw)
		if err != nil {
			return "", fmt.Errorf("decode invoice.payment_failed: %w", err)
		}
		return strings.TrimSpace(strings.ToUpper(invoice.Metadata["jurisdiction"])), nil
	default:
		return "", nil
	}
}

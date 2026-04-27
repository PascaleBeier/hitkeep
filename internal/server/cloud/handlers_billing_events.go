//go:build billing

package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v84"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
)

func (h *handler) handleStripeEvent(ctx context.Context, event stripe.Event) error {
	eventJurisdiction, err := jurisdictionFromStripeEvent(event)
	if err != nil {
		return err
	}
	configuredJurisdiction := strings.TrimSpace(strings.ToUpper(h.ctx.Config.CloudJurisdiction))
	if configuredJurisdiction != "" && eventJurisdiction != "" && eventJurisdiction != configuredJurisdiction {
		return nil
	}

	tenantID, _ := tenantIDFromStripeEvent(event)
	inserted, err := h.ctx.Store.CreateCloudBillingEvent(ctx, database.CloudBillingEvent{
		StripeEventID: event.ID,
		TenantID:      tenantID,
		EventType:     string(event.Type),
		Livemode:      event.Livemode,
		Payload:       string(event.Data.Raw),
	})
	if err != nil {
		return fmt.Errorf("record stripe event: %w", err)
	}
	if !inserted {
		storedEvent, lookupErr := h.ctx.Store.GetCloudBillingEvent(ctx, event.ID)
		if lookupErr != nil {
			return fmt.Errorf("load existing stripe event: %w", lookupErr)
		}
		if storedEvent.ProcessingStatus == database.CloudBillingEventStatusDone {
			return nil
		}
	}

	updateEventStatus := func(status string, processedTenantID uuid.UUID, processErr error) error {
		billingEvent := database.CloudBillingEvent{
			StripeEventID:    event.ID,
			TenantID:         processedTenantID,
			ProcessingStatus: status,
		}
		if processErr != nil {
			billingEvent.ProcessingError = processErr.Error()
		} else {
			processedAt := stripeEventProcessedAt()
			billingEvent.ProcessedAt = &processedAt
		}
		return h.ctx.Store.UpdateCloudBillingEventStatus(ctx, billingEvent)
	}

	var handleErr error
	switch string(event.Type) {
	case "checkout.session.completed":
		session, err := parseStripeCheckoutSessionEvent(event.Data.Raw)
		if err != nil {
			handleErr = fmt.Errorf("decode checkout.session.completed: %w", err)
			break
		}
		tenantID, err := uuid.Parse(strings.TrimSpace(session.Metadata["tenant_id"]))
		if err != nil {
			handleErr = fmt.Errorf("parse checkout tenant_id: %w", err)
			break
		}
		handleErr = h.ctx.Store.UpsertCloudBillingAccount(ctx, database.CloudBillingAccount{
			TenantID:             tenantID,
			PlanCode:             normalizePlanCode(session.Metadata["plan_code"]),
			PlanName:             strings.TrimSpace(session.Metadata["plan_name"]),
			SubscriptionStatus:   session.Status,
			StripeCustomerID:     session.Customer.ID,
			StripeSubscriptionID: session.Subscription.ID,
			StripePriceID:        session.FirstPriceID(),
		})
	case "customer.subscription.updated", "customer.subscription.deleted":
		subscription, err := parseStripeSubscriptionEvent(event.Data.Raw)
		if err != nil {
			handleErr = fmt.Errorf("decode subscription event: %w", err)
			break
		}
		tenantID, err := tenantIDFromStripeMetadata(subscription.Metadata)
		if err != nil {
			handleErr = err
			break
		}
		handleErr = h.ctx.Store.UpsertCloudBillingAccount(ctx, database.CloudBillingAccount{
			TenantID:             tenantID,
			PlanCode:             normalizePlanCode(subscription.Metadata["plan_code"]),
			PlanName:             strings.TrimSpace(subscription.Metadata["plan_name"]),
			SubscriptionStatus:   subscription.Status,
			StripeCustomerID:     subscription.Customer.ID,
			StripeSubscriptionID: subscription.ID,
			StripePriceID:        subscription.FirstPriceID(),
		})
	case "invoice.payment_failed":
		tenantID, handleErr = h.handleInvoicePaymentFailed(ctx, event)
	case "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed":
		tenantID, handleErr = h.handleChargeDisputeEvent(ctx, event)
	default:
		handleErr = nil
	}

	if handleErr != nil {
		if updateErr := updateEventStatus(database.CloudBillingEventStatusErrored, tenantID, handleErr); updateErr != nil {
			return fmt.Errorf("mark stripe event failed: %w", updateErr)
		}
		return handleErr
	}

	if err := updateEventStatus(database.CloudBillingEventStatusDone, tenantID, nil); err != nil {
		return fmt.Errorf("mark stripe event processed: %w", err)
	}

	return nil
}

func tenantIDFromStripeMetadata(metadata map[string]string) (uuid.UUID, error) {
	tenantID, err := uuid.Parse(strings.TrimSpace(metadata["tenant_id"]))
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse subscription tenant_id: %w", err)
	}
	return tenantID, nil
}

func normalizeJurisdiction(value string) string {
	v := strings.TrimSpace(strings.ToUpper(value))
	if strings.HasPrefix(v, "EU") {
		return "EU"
	}
	if strings.HasPrefix(v, "US") {
		return "US"
	}
	return v
}

func normalizePlanCode(planCode string) string {
	switch strings.TrimSpace(strings.ToLower(planCode)) {
	case database.CloudPlanFree:
		return database.CloudPlanFree
	case database.CloudPlanBusiness:
		return database.CloudPlanBusiness
	case database.CloudPlanPro:
		return database.CloudPlanPro
	default:
		return ""
	}
}

func planNameForCode(planCode string) string {
	switch normalizePlanCode(planCode) {
	case database.CloudPlanBusiness:
		return "Business"
	case database.CloudPlanPro:
		return "Pro"
	default:
		return "Free"
	}
}

func effectivePlanCode(account *database.CloudBillingAccount) (string, string) {
	if account == nil {
		return "", ""
	}

	switch strings.TrimSpace(account.SubscriptionStatus) {
	case "", database.CloudSubscriptionStatusFree, subscriptionStatusPending, subscriptionStatusCanceled, database.CloudSubscriptionStatusChargebackLost:
		return database.CloudPlanFree, planNameForCode(database.CloudPlanFree)
	default:
		return strings.TrimSpace(account.PlanCode), strings.TrimSpace(account.PlanName)
	}
}

func stripeCustomerName(user *api.User, teamName string) string {
	if user != nil {
		if name := strings.TrimSpace(strings.TrimSpace(user.GivenName + " " + user.LastName)); name != "" {
			return name
		}
	}
	return strings.TrimSpace(teamName)
}

func priceIDForPlan(conf *config.Config, planCode string) string {
	switch normalizePlanCode(planCode) {
	case database.CloudPlanBusiness:
		return strings.TrimSpace(conf.StripePriceBusinessMonthly)
	case database.CloudPlanPro:
		return strings.TrimSpace(conf.StripePriceProMonthly)
	default:
		return ""
	}
}

func effectiveJurisdiction(conf *config.Config, requested string) string {
	if trimmed := strings.TrimSpace(strings.ToUpper(requested)); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(strings.ToUpper(conf.CloudJurisdiction))
}

func checkoutSuccessURL(conf *config.Config) string {
	if override := strings.TrimSpace(conf.CloudCheckoutSuccessURL); override != "" {
		return override
	}
	return strings.TrimRight(conf.PublicURL, "/") + "/admin/team?checkout=success"
}

func checkoutCancelURL(conf *config.Config) string {
	if override := strings.TrimSpace(conf.CloudCheckoutCancelURL); override != "" {
		return override
	}
	return strings.TrimRight(conf.PublicURL, "/") + "/admin/team?checkout=canceled"
}

func issueLoginSession(w http.ResponseWriter, conf *config.Config, userID uuid.UUID) error {
	duration := conf.AuthSessionDuration()
	token, _, err := authcore.GenerateTokenWithDuration(conf.JWTSecret, conf.PublicURL, userID, duration)
	if err != nil {
		return fmt.Errorf("generate auth token: %w", err)
	}

	isSecure := strings.HasPrefix(conf.PublicURL, "https://")
	authcore.SetTokenCookieWithDuration(w, token, isSecure, duration)
	return nil
}

func (h *handler) handleInvoicePaymentFailed(ctx context.Context, event stripe.Event) (uuid.UUID, error) {
	invoice, err := parseStripeInvoiceEvent(event.Data.Raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("decode invoice.payment_failed: %w", err)
	}

	account, err := h.resolveCloudBillingAccount(ctx, invoice.TenantID(), invoice.Customer.ID, invoice.Subscription.ID)
	if err != nil {
		if errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}

	account.SubscriptionStatus = database.CloudSubscriptionStatusPastDue
	if invoice.Subscription.ID != "" {
		account.StripeSubscriptionID = invoice.Subscription.ID
	}
	if invoice.Customer.ID != "" {
		account.StripeCustomerID = invoice.Customer.ID
	}

	return account.TenantID, h.ctx.Store.UpsertCloudBillingAccount(ctx, *account)
}

func (h *handler) handleChargeDisputeEvent(ctx context.Context, event stripe.Event) (uuid.UUID, error) {
	dispute, err := parseStripeDisputeEvent(event.Data.Raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("decode %s: %w", event.Type, err)
	}
	if strings.TrimSpace(dispute.Charge.ID) == "" {
		return uuid.Nil, fmt.Errorf("dispute charge id missing")
	}

	charge, err := h.stripe.GetCharge(ctx, dispute.Charge.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("load disputed charge: %w", err)
	}

	account, err := h.resolveCloudBillingAccount(ctx, uuid.Nil, charge.CustomerID, "")
	if err != nil {
		if errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}

	account.SubscriptionStatus = disputeSubscriptionStatus(string(event.Type), dispute.Status)
	return account.TenantID, h.ctx.Store.UpsertCloudBillingAccount(ctx, *account)
}

func (h *handler) resolveCloudBillingAccount(ctx context.Context, tenantID uuid.UUID, customerID string, subscriptionID string) (*database.CloudBillingAccount, error) {
	if tenantID != uuid.Nil {
		account, err := h.ctx.Store.GetCloudBillingAccount(ctx, tenantID)
		if err == nil {
			return account, nil
		}
		if !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return nil, err
		}
	}

	if strings.TrimSpace(subscriptionID) != "" {
		account, err := h.ctx.Store.GetCloudBillingAccountByStripeSubscriptionID(ctx, subscriptionID)
		if err == nil {
			return account, nil
		}
		if !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return nil, err
		}
	}

	if strings.TrimSpace(customerID) != "" {
		account, err := h.ctx.Store.GetCloudBillingAccountByStripeCustomerID(ctx, customerID)
		if err == nil {
			return account, nil
		}
		if !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			return nil, err
		}
	}

	return nil, database.ErrCloudBillingAccountNotFound
}

func tenantIDFromStripeEvent(event stripe.Event) (uuid.UUID, error) {
	switch string(event.Type) {
	case "checkout.session.completed":
		session, err := parseStripeCheckoutSessionEvent(event.Data.Raw)
		if err != nil {
			return uuid.Nil, fmt.Errorf("decode checkout.session.completed: %w", err)
		}
		return uuid.Parse(strings.TrimSpace(session.Metadata["tenant_id"]))
	case "customer.subscription.updated", "customer.subscription.deleted":
		subscription, err := parseStripeSubscriptionEvent(event.Data.Raw)
		if err != nil {
			return uuid.Nil, fmt.Errorf("decode subscription event: %w", err)
		}
		return tenantIDFromStripeMetadata(subscription.Metadata)
	case "invoice.payment_failed":
		invoice, err := parseStripeInvoiceEvent(event.Data.Raw)
		if err != nil {
			return uuid.Nil, fmt.Errorf("decode invoice.payment_failed: %w", err)
		}
		return invoice.TenantID(), nil
	default:
		return uuid.Nil, nil
	}
}

func stripeEventProcessedAt() time.Time {
	return time.Now().UTC()
}

type stripeExpandableRef struct {
	ID string
}

func (r *stripeExpandableRef) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		r.ID = ""
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		return json.Unmarshal(data, &r.ID)
	}

	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	r.ID = strings.TrimSpace(obj.ID)
	return nil
}

type stripeCheckoutSessionLineItem struct {
	Price stripeExpandableRef `json:"price"`
}

type stripeCheckoutSessionEventPayload struct {
	Metadata     map[string]string               `json:"metadata"`
	Status       string                          `json:"status"`
	Customer     stripeExpandableRef             `json:"customer"`
	Subscription stripeExpandableRef             `json:"subscription"`
	LineItems    []stripeCheckoutSessionLineItem `json:"line_items"`
}

func (p *stripeCheckoutSessionEventPayload) FirstPriceID() string {
	if p == nil || len(p.LineItems) == 0 {
		return ""
	}
	return strings.TrimSpace(p.LineItems[0].Price.ID)
}

type stripeSubscriptionItem struct {
	Price stripeExpandableRef `json:"price"`
}

type stripeSubscriptionItems struct {
	Data []stripeSubscriptionItem `json:"data"`
}

type stripeSubscriptionEventPayload struct {
	ID       string                  `json:"id"`
	Metadata map[string]string       `json:"metadata"`
	Status   string                  `json:"status"`
	Customer stripeExpandableRef     `json:"customer"`
	Items    stripeSubscriptionItems `json:"items"`
}

func (p *stripeSubscriptionEventPayload) FirstPriceID() string {
	if p == nil || len(p.Items.Data) == 0 {
		return ""
	}
	return strings.TrimSpace(p.Items.Data[0].Price.ID)
}

type stripeInvoiceEventPayload struct {
	Customer     stripeExpandableRef `json:"customer"`
	Subscription stripeExpandableRef `json:"subscription"`
	Metadata     map[string]string   `json:"metadata"`
}

func (p *stripeInvoiceEventPayload) TenantID() uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	tenantID, err := tenantIDFromStripeMetadata(p.Metadata)
	if err != nil {
		return uuid.Nil
	}
	return tenantID
}

type stripeDisputeEventPayload struct {
	Charge stripeExpandableRef `json:"charge"`
	Status string              `json:"status"`
}

func parseStripeInvoiceEvent(raw []byte) (*stripeInvoiceEventPayload, error) {
	var invoice stripeInvoiceEventPayload
	if err := json.Unmarshal(raw, &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}

func parseStripeCheckoutSessionEvent(raw []byte) (*stripeCheckoutSessionEventPayload, error) {
	var session stripeCheckoutSessionEventPayload
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func parseStripeSubscriptionEvent(raw []byte) (*stripeSubscriptionEventPayload, error) {
	var subscription stripeSubscriptionEventPayload
	if err := json.Unmarshal(raw, &subscription); err != nil {
		return nil, err
	}
	return &subscription, nil
}

func parseStripeDisputeEvent(raw []byte) (*stripeDisputeEventPayload, error) {
	var dispute stripeDisputeEventPayload
	if err := json.Unmarshal(raw, &dispute); err != nil {
		return nil, err
	}
	return &dispute, nil
}

func disputeSubscriptionStatus(eventType string, disputeStatus string) string {
	normalizedStatus := strings.TrimSpace(strings.ToLower(disputeStatus))
	if eventType == "charge.dispute.closed" {
		switch normalizedStatus {
		case "won":
			return database.CloudSubscriptionStatusActive
		case "lost":
			return database.CloudSubscriptionStatusChargebackLost
		}
	}
	return database.CloudSubscriptionStatusDisputed
}

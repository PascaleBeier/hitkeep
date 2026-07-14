//go:build billing

package database

import (
	"context"
	"testing"
	"time"
)

func TestCreateManagedCloudAccountDoesNotJoinDefaultTenant(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	account, err := store.CreateManagedCloudAccount(context.Background(), CreateManagedCloudAccountInput{
		Email:          "cloud-owner@example.com",
		HashedPassword: "hashed",
		GivenName:      "Cloud",
		LastName:       "Owner",
		TeamName:       "Cloud Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	teams, activeTenantID, err := store.ListUserTeams(context.Background(), account.UserID)
	if err != nil {
		t.Fatalf("list user teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected exactly one visible team, got %d", len(teams))
	}
	if teams[0].ID != account.TenantID {
		t.Fatalf("expected created tenant %s, got %s", account.TenantID, teams[0].ID)
	}
	if activeTenantID != account.TenantID {
		t.Fatalf("expected active tenant %s, got %s", account.TenantID, activeTenantID)
	}
}

func TestUpsertCloudBillingAccountRoundTrips(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	account, err := store.CreateManagedCloudAccount(context.Background(), CreateManagedCloudAccountInput{
		Email:          "billing-owner@example.com",
		HashedPassword: "hashed",
		TeamName:       "Billing Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	err = store.UpsertCloudBillingAccount(context.Background(), CloudBillingAccount{
		TenantID:             account.TenantID,
		PlanCode:             CloudPlanPro,
		PlanName:             "Pro",
		SubscriptionStatus:   "trialing",
		StripeCustomerID:     "cus_123",
		StripeSubscriptionID: "sub_123",
		StripePriceID:        "price_123",
	})
	if err != nil {
		t.Fatalf("upsert cloud billing account: %v", err)
	}

	got, err := store.GetCloudBillingAccount(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("get cloud billing account: %v", err)
	}
	if got.PlanCode != CloudPlanPro || got.PlanName != "Pro" {
		t.Fatalf("unexpected plan payload: %+v", got)
	}
	if got.StripeCustomerID != "cus_123" || got.StripeSubscriptionID != "sub_123" || got.StripePriceID != "price_123" {
		t.Fatalf("unexpected stripe payload: %+v", got)
	}

	byCustomer, err := store.GetCloudBillingAccountByStripeCustomerID(context.Background(), "cus_123")
	if err != nil {
		t.Fatalf("get cloud billing account by customer id: %v", err)
	}
	if byCustomer.TenantID != account.TenantID {
		t.Fatalf("expected customer lookup tenant %s, got %s", account.TenantID, byCustomer.TenantID)
	}

	bySubscription, err := store.GetCloudBillingAccountByStripeSubscriptionID(context.Background(), "sub_123")
	if err != nil {
		t.Fatalf("get cloud billing account by subscription id: %v", err)
	}
	if bySubscription.TenantID != account.TenantID {
		t.Fatalf("expected subscription lookup tenant %s, got %s", account.TenantID, bySubscription.TenantID)
	}
}

func TestRecordCloudConversionEventsPreservesIntentAndDeduplicatesMilestones(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	account, err := store.CreateManagedCloudAccount(context.Background(), CreateManagedCloudAccountInput{
		Email:          "conversion-owner@example.com",
		HashedPassword: "hashed",
		TeamName:       "Conversion Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           CloudPlanFree,
		PlanName:           "Free",
		BillingInterval:    CloudBillingIntervalAnnual,
		SubscriptionStatus: CloudSubscriptionStatusFree,
	}); err != nil {
		t.Fatalf("upsert billing account: %v", err)
	}

	inserted, err := store.RecordCloudConversionEvent(context.Background(), CloudConversionEvent{
		TenantID:        account.TenantID,
		EventName:       CloudConversionSignupVerified,
		PlanCode:        CloudPlanPro,
		BillingInterval: CloudBillingIntervalAnnual,
	})
	if err != nil || !inserted {
		t.Fatalf("record signup conversion: inserted=%v err=%v", inserted, err)
	}
	inserted, err = store.RecordCloudConversionEvent(context.Background(), CloudConversionEvent{
		TenantID:  account.TenantID,
		EventName: CloudConversionSignupVerified,
	})
	if err != nil {
		t.Fatalf("repeat signup conversion: %v", err)
	}
	if inserted {
		t.Fatal("expected repeated one-time milestone to be deduplicated")
	}

	events, err := store.ListCloudConversionEvents(context.Background(), account.TenantID)
	if err != nil {
		t.Fatalf("list conversion events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one conversion event, got %d", len(events))
	}
	if events[0].PlanCode != CloudPlanPro || events[0].BillingInterval != CloudBillingIntervalAnnual {
		t.Fatalf("expected pro annual intent, got %+v", events[0])
	}
}

func TestCloudBillingEventsAreIdempotentAndTrackStatus(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	account, err := store.CreateManagedCloudAccount(context.Background(), CreateManagedCloudAccountInput{
		Email:          "events-owner@example.com",
		HashedPassword: "hashed",
		TeamName:       "Events Team",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}

	created, err := store.CreateCloudBillingEvent(context.Background(), CloudBillingEvent{
		StripeEventID: "evt_123",
		TenantID:      account.TenantID,
		EventType:     "checkout.session.completed",
		Livemode:      true,
		Payload:       `{"id":"evt_123"}`,
	})
	if err != nil {
		t.Fatalf("create cloud billing event: %v", err)
	}
	if !created {
		t.Fatal("expected first cloud billing event insert to succeed")
	}

	created, err = store.CreateCloudBillingEvent(context.Background(), CloudBillingEvent{
		StripeEventID: "evt_123",
		TenantID:      account.TenantID,
		EventType:     "checkout.session.completed",
	})
	if err != nil {
		t.Fatalf("create duplicate cloud billing event: %v", err)
	}
	if created {
		t.Fatal("expected duplicate cloud billing event insert to be ignored")
	}

	processedAt := time.Now().UTC().Round(time.Second)
	if err := store.UpdateCloudBillingEventStatus(context.Background(), CloudBillingEvent{
		StripeEventID:    "evt_123",
		TenantID:         account.TenantID,
		ProcessingStatus: CloudBillingEventStatusDone,
		ProcessedAt:      &processedAt,
	}); err != nil {
		t.Fatalf("update cloud billing event status: %v", err)
	}

	event, err := store.GetCloudBillingEvent(context.Background(), "evt_123")
	if err != nil {
		t.Fatalf("get cloud billing event: %v", err)
	}
	if event.ProcessingStatus != CloudBillingEventStatusDone {
		t.Fatalf("expected processed status, got %+v", event)
	}
	if event.ProcessedAt == nil || !event.ProcessedAt.Equal(processedAt) {
		t.Fatalf("expected processed timestamp %v, got %+v", processedAt, event.ProcessedAt)
	}
}

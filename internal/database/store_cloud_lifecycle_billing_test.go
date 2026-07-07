//go:build billing

package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestListEligibleCloudLifecycleRecipientsForWelcome(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	activated := seedCloudLifecycleTeam(t, store, "activated@example.com", "activated.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.Add(-time.Hour)))
	seedCloudLifecycleTeam(t, store, "no-hit@example.com", "waiting.example", CloudPlanFree, CloudSubscriptionStatusFree, nil)
	seedCloudLifecycleTeamWithoutSite(t, store, "no-site@example.com", CloudPlanFree, CloudSubscriptionStatusFree)
	seedCloudLifecycleTeamWithoutBilling(t, store, "no-billing@example.com", "self-hosted.example", ptrTime(now.Add(-time.Hour)))

	recipients, err := store.ListEligibleCloudLifecycleRecipients(ctx, CloudLifecycleMessageWelcome, now, 100)
	if err != nil {
		t.Fatalf("list welcome recipients: %v", err)
	}
	if len(recipients) != 1 {
		t.Fatalf("expected one welcome recipient, got %d: %+v", len(recipients), recipients)
	}
	got := recipients[0]
	if got.TenantID != activated.TenantID || got.UserID != activated.UserID || got.SiteDomain != "activated.example" {
		t.Fatalf("unexpected welcome recipient: %+v", got)
	}
	if got.PlanCode != CloudPlanFree || got.SubscriptionStatus != CloudSubscriptionStatusFree {
		t.Fatalf("expected free plan metadata, got %+v", got)
	}

	if err := store.MarkCloudLifecycleMessageSent(ctx, CloudLifecycleMessageUpdate{
		TenantID: activated.TenantID,
		UserID:   activated.UserID,
		Kind:     CloudLifecycleMessageWelcome,
		Now:      now,
	}); err != nil {
		t.Fatalf("mark welcome sent: %v", err)
	}

	recipients, err = store.ListEligibleCloudLifecycleRecipients(ctx, CloudLifecycleMessageWelcome, now, 100)
	if err != nil {
		t.Fatalf("list welcome recipients after sent: %v", err)
	}
	if len(recipients) != 0 {
		t.Fatalf("expected sent welcome recipient to be excluded, got %+v", recipients)
	}
}

func TestListEligibleCloudLifecycleRecipientsForFreeRetentionReminder(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	oldFree := seedCloudLifecycleTeam(t, store, "old-free@example.com", "old-free.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.AddDate(0, 0, -20)))
	seedCloudLifecycleTeam(t, store, "young-free@example.com", "young-free.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.AddDate(0, 0, -7)))
	seedCloudLifecycleTeam(t, store, "paid@example.com", "paid.example", CloudPlanPro, CloudSubscriptionStatusActive, ptrTime(now.AddDate(0, 0, -20)))
	seedCloudLifecycleTeam(t, store, "no-hit-reminder@example.com", "no-hit-reminder.example", CloudPlanFree, CloudSubscriptionStatusFree, nil)

	recipients, err := store.ListEligibleCloudLifecycleRecipients(ctx, CloudLifecycleMessageFreeRetentionReminder, now, 100)
	if err != nil {
		t.Fatalf("list reminder recipients: %v", err)
	}
	if len(recipients) != 1 {
		t.Fatalf("expected one reminder recipient, got %d: %+v", len(recipients), recipients)
	}
	if recipients[0].TenantID != oldFree.TenantID || recipients[0].Email != "old-free@example.com" {
		t.Fatalf("unexpected reminder recipient: %+v", recipients[0])
	}
}

func TestCloudLifecycleMessageFailureRetriesAndCapsAttempts(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	team := seedCloudLifecycleTeam(t, store, "retry@example.com", "retry.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.AddDate(0, 0, -20)))

	for attempt := 1; attempt <= CloudLifecycleMessageMaxAttempts; attempt++ {
		if err := store.MarkCloudLifecycleMessageFailed(ctx, CloudLifecycleMessageUpdate{
			TenantID: team.TenantID,
			UserID:   team.UserID,
			Kind:     CloudLifecycleMessageFreeRetentionReminder,
			Error:    strings.Repeat("x", 1200),
			Now:      now.Add(time.Duration(attempt) * time.Minute),
		}); err != nil {
			t.Fatalf("mark reminder failed attempt %d: %v", attempt, err)
		}

		message, err := store.GetCloudLifecycleMessage(ctx, team.TenantID, team.UserID, CloudLifecycleMessageFreeRetentionReminder)
		if err != nil {
			t.Fatalf("get failed lifecycle message: %v", err)
		}
		if message.Attempts != attempt {
			t.Fatalf("expected attempts %d, got %+v", attempt, message)
		}
		if len(message.ProcessingError) != 1000 {
			t.Fatalf("expected truncated processing error, got length %d", len(message.ProcessingError))
		}
	}

	recipients, err := store.ListEligibleCloudLifecycleRecipients(ctx, CloudLifecycleMessageFreeRetentionReminder, now, 100)
	if err != nil {
		t.Fatalf("list reminder recipients after failures: %v", err)
	}
	if len(recipients) != 0 {
		t.Fatalf("expected max-attempt failed recipient to be excluded, got %+v", recipients)
	}
}

func TestMarkCloudLifecycleMessageSentRecordsSentState(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	team := seedCloudLifecycleTeam(t, store, "sent@example.com", "sent.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.AddDate(0, 0, -20)))

	if err := store.MarkCloudLifecycleMessageFailed(ctx, CloudLifecycleMessageUpdate{
		TenantID: team.TenantID,
		UserID:   team.UserID,
		Kind:     CloudLifecycleMessageWelcome,
		Error:    "temporary smtp error",
		Now:      now,
	}); err != nil {
		t.Fatalf("mark welcome failed: %v", err)
	}
	if err := store.MarkCloudLifecycleMessageSent(ctx, CloudLifecycleMessageUpdate{
		TenantID: team.TenantID,
		UserID:   team.UserID,
		Kind:     CloudLifecycleMessageWelcome,
		Now:      now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("mark welcome sent: %v", err)
	}

	message, err := store.GetCloudLifecycleMessage(ctx, team.TenantID, team.UserID, CloudLifecycleMessageWelcome)
	if err != nil {
		t.Fatalf("get sent lifecycle message: %v", err)
	}
	if message.Status != CloudLifecycleMessageStatusSent || message.Attempts != 2 || message.SentAt == nil || message.ProcessingError != "" {
		t.Fatalf("unexpected sent lifecycle message: %+v", message)
	}

	_, err = store.GetCloudLifecycleMessage(ctx, uuid.New(), uuid.New(), CloudLifecycleMessageWelcome)
	if !errors.Is(err, ErrCloudLifecycleMessageNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func seedCloudLifecycleTeam(t *testing.T, store *Store, email string, domain string, planCode string, status string, firstHitAt *time.Time) *ManagedCloudAccount {
	t.Helper()
	ctx := context.Background()
	account := seedCloudLifecycleTeamWithoutSite(t, store, email, planCode, status)
	site, err := store.CreateSite(ctx, account.UserID, domain)
	if err != nil {
		t.Fatalf("create site for %s: %v", email, err)
	}
	if firstHitAt != nil {
		if err := store.RecordHitActivity(ctx, []*api.Hit{{
			SiteID:    site.ID,
			Timestamp: firstHitAt.UTC(),
			Path:      "/",
		}}); err != nil {
			t.Fatalf("record hit activity for %s: %v", email, err)
		}
	}
	return account
}

func seedCloudLifecycleTeamWithoutSite(t *testing.T, store *Store, email string, planCode string, status string) *ManagedCloudAccount {
	t.Helper()
	ctx := context.Background()
	account, err := store.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{
		Email:          email,
		HashedPassword: "hashed",
		TeamName:       email,
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create managed cloud account for %s: %v", email, err)
	}
	if planCode == "" {
		planCode = CloudPlanFree
	}
	if status == "" {
		status = CloudSubscriptionStatusFree
	}
	if err := store.UpsertCloudBillingAccount(ctx, CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           planCode,
		PlanName:           planCode,
		SubscriptionStatus: status,
	}); err != nil {
		t.Fatalf("upsert cloud billing account for %s: %v", email, err)
	}
	return account
}

func seedCloudLifecycleTeamWithoutBilling(t *testing.T, store *Store, email string, domain string, firstHitAt *time.Time) *ManagedCloudAccount {
	t.Helper()
	ctx := context.Background()
	account, err := store.CreateManagedCloudAccount(ctx, CreateManagedCloudAccountInput{
		Email:          email,
		HashedPassword: "hashed",
		TeamName:       email,
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create unmanaged account for %s: %v", email, err)
	}
	site, err := store.CreateSite(ctx, account.UserID, domain)
	if err != nil {
		t.Fatalf("create unmanaged site for %s: %v", email, err)
	}
	if firstHitAt != nil {
		if err := store.RecordHitActivity(ctx, []*api.Hit{{
			SiteID:    site.ID,
			Timestamp: firstHitAt.UTC(),
			Path:      "/",
		}}); err != nil {
			t.Fatalf("record unmanaged hit activity for %s: %v", email, err)
		}
	}
	return account
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestListEligibleCloudLifecycleRecipientsFreeLimitReminder(t *testing.T) {
	store := setupTenantStore(t)
	now := time.Now().UTC()

	// One activated site and one member: well below the free plan limits.
	relaxed := seedCloudLifecycleTeam(t, store, "relaxed@example.com", "relaxed.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.Add(-time.Hour)))

	// A team that has filled every free plan site slot.
	crowded := seedCloudLifecycleTeam(t, store, "crowded@example.com", "crowded.example", CloudPlanFree, CloudSubscriptionStatusFree, ptrTime(now.Add(-time.Hour)))
	ctx := context.Background()
	for _, domain := range []string{"crowded-two.example", "crowded-three.example"} {
		if _, err := store.CreateSite(ctx, crowded.UserID, domain); err != nil {
			t.Fatalf("create extra site %s: %v", domain, err)
		}
	}

	recipients, err := store.ListEligibleCloudLifecycleRecipients(ctx, CloudLifecycleMessageFreeLimitReminder, now, 100)
	if err != nil {
		t.Fatalf("list free limit recipients: %v", err)
	}

	if len(recipients) != 1 {
		t.Fatalf("expected exactly one recipient at the free plan limit, got %d", len(recipients))
	}
	if recipients[0].TenantID != crowded.TenantID {
		t.Fatalf("expected crowded tenant %s, got %s", crowded.TenantID, recipients[0].TenantID)
	}
	if recipients[0].TenantID == relaxed.TenantID {
		t.Fatalf("relaxed tenant must not be eligible")
	}
}

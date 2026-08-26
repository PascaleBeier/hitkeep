//go:build billing

package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"hitkeep/hklog"
	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/testutil/testdb"
)

type cloudLifecycleWorkerMailDriver struct {
	sendErr error
	sends   []cloudLifecycleWorkerSend
}

type cloudLifecycleWorkerSend struct {
	to       []string
	subject  string
	htmlBody string
	textBody string
}

func (d *cloudLifecycleWorkerMailDriver) Send(to []string, subject, htmlBody, textBody string) error {
	d.sends = append(d.sends, cloudLifecycleWorkerSend{
		to:       append([]string(nil), to...),
		subject:  subject,
		htmlBody: htmlBody,
		textBody: textBody,
	})
	return d.sendErr
}

func (d *cloudLifecycleWorkerMailDriver) Close() error { return nil }

func TestCloudLifecycleWorkerSendsWelcomeAndReminderOnce(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	team := seedWorkerCloudLifecycleTeam(t, store, "owner@example.com", "example.com", database.CloudPlanFree, database.CloudSubscriptionStatusFree, ptrWorkerTime(now.AddDate(0, 0, -20)))
	driver := &cloudLifecycleWorkerMailDriver{}
	worker := NewCloudLifecycleWorker(mgr, mailer.NewWithDriver(driver, nil), cloudLifecycleWorkerConfig())

	worker.RunAt(context.Background(), now)

	if len(driver.sends) != 2 {
		t.Fatalf("expected welcome and reminder sends, got %d: %+v", len(driver.sends), driver.sends)
	}
	for _, kind := range []string{database.CloudLifecycleMessageWelcome, database.CloudLifecycleMessageFreeRetentionReminder} {
		message, err := store.GetCloudLifecycleMessage(context.Background(), team.TenantID, team.UserID, kind)
		if err != nil {
			t.Fatalf("get lifecycle message %s: %v", kind, err)
		}
		if message.Status != database.CloudLifecycleMessageStatusSent || message.Attempts != 1 || message.SentAt == nil {
			t.Fatalf("unexpected lifecycle message %s: %+v", kind, message)
		}
	}

	worker.RunAt(context.Background(), now.Add(24*time.Hour))
	if len(driver.sends) != 2 {
		t.Fatalf("expected no duplicate sends, got %d", len(driver.sends))
	}
}

func TestCloudLifecycleWorkerSendsPreTrimWarningBeforeFirstRollOff(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	firstHit := now.AddDate(0, 0, -(database.CloudFreePlanRetentionDays - database.CloudRetentionPreTrimLeadDays + 1))
	team := seedWorkerCloudLifecycleTeam(t, store, "pretrim-worker@example.com", "pretrim-worker.example", database.CloudPlanFree, database.CloudSubscriptionStatusFree, ptrWorkerTime(firstHit))
	driver := &cloudLifecycleWorkerMailDriver{}
	worker := NewCloudLifecycleWorker(mgr, mailer.NewWithDriver(driver, nil), cloudLifecycleWorkerConfig())

	worker.RunAt(context.Background(), now)

	message, err := store.GetCloudLifecycleMessage(context.Background(), team.TenantID, team.UserID, database.CloudLifecycleMessageFreeRetentionPreTrim)
	if err != nil {
		t.Fatalf("get pre-trim message: %v", err)
	}
	if message.Status != database.CloudLifecycleMessageStatusSent || message.Attempts != 1 || message.SentAt == nil {
		t.Fatalf("unexpected pre-trim message: %+v", message)
	}

	rollOffDate := firstHit.UTC().AddDate(0, 0, database.CloudFreePlanRetentionDays).Format("2006-01-02")
	found := false
	for _, send := range driver.sends {
		if strings.Contains(send.textBody, rollOffDate) && strings.Contains(send.textBody, "pretrim-worker.example") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a pre-trim send containing roll-off date %s, got %+v", rollOffDate, driver.sends)
	}

	sendsAfterFirstRun := len(driver.sends)
	worker.RunAt(context.Background(), now.Add(24*time.Hour))
	if len(driver.sends) != sendsAfterFirstRun {
		t.Fatalf("expected no duplicate pre-trim sends, got %d after %d", len(driver.sends), sendsAfterFirstRun)
	}
}

func TestCloudLifecycleWorkerSkipsWhenMailerMissing(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	team := seedWorkerCloudLifecycleTeam(t, store, "missing-mailer@example.com", "missing-mailer.example", database.CloudPlanFree, database.CloudSubscriptionStatusFree, ptrWorkerTime(now.Add(-time.Hour)))
	worker := NewCloudLifecycleWorker(mgr, nil, cloudLifecycleWorkerConfig())

	worker.RunAt(context.Background(), now)

	_, err := store.GetCloudLifecycleMessage(context.Background(), team.TenantID, team.UserID, database.CloudLifecycleMessageWelcome)
	if !errors.Is(err, database.ErrCloudLifecycleMessageNotFound) {
		t.Fatalf("expected no lifecycle message when mailer is missing, got %v", err)
	}
}

func TestCloudLifecycleWorkerRetriesFailedSend(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	team := seedWorkerCloudLifecycleTeam(t, store, "retry-worker@example.com", "retry-worker.example", database.CloudPlanFree, database.CloudSubscriptionStatusFree, ptrWorkerTime(now.Add(-time.Hour)))
	rawMailError := "provider response password=super-secret token=top-secret https://mail.example.test/reject"
	driver := &cloudLifecycleWorkerMailDriver{sendErr: errors.New(rawMailError)}
	worker := NewCloudLifecycleWorker(mgr, mailer.NewWithDriver(driver, nil), cloudLifecycleWorkerConfig())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := hklog.WithLogger(context.Background(), logger)

	worker.RunAt(ctx, now)

	message, err := store.GetCloudLifecycleMessage(context.Background(), team.TenantID, team.UserID, database.CloudLifecycleMessageWelcome)
	if err != nil {
		t.Fatalf("get failed welcome message: %v", err)
	}
	if message.Status != database.CloudLifecycleMessageStatusFailed || message.Attempts != 1 || message.ProcessingError != "mail transport failed" {
		t.Fatalf("unexpected failed lifecycle message: %+v", message)
	}
	if strings.Contains(logs.String(), rawMailError) || strings.Contains(message.ProcessingError, rawMailError) {
		t.Fatalf("raw mail error leaked into logs or persisted state: logs=%q message=%q", logs.String(), message.ProcessingError)
	}
	if !strings.Contains(logs.String(), "mail.error_stage=transport") || !strings.Contains(logs.String(), "mail.error_kind=transport") || !strings.Contains(logs.String(), "mail.error_message=\"mail transport failed\"") {
		t.Fatalf("expected redacted mail diagnostics in logs, got %q", logs.String())
	}

	driver.sendErr = nil
	worker.RunAt(ctx, now.Add(24*time.Hour))

	message, err = store.GetCloudLifecycleMessage(context.Background(), team.TenantID, team.UserID, database.CloudLifecycleMessageWelcome)
	if err != nil {
		t.Fatalf("get retried welcome message: %v", err)
	}
	if message.Status != database.CloudLifecycleMessageStatusSent || message.Attempts != 2 || message.SentAt == nil || message.ProcessingError != "" {
		t.Fatalf("unexpected retried lifecycle message: %+v", message)
	}
}

func TestCloudLifecycleWorkerUsesConfiguredLinks(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	seedWorkerCloudLifecycleTeam(t, store, "links@example.com", "links.example", database.CloudPlanPro, database.CloudSubscriptionStatusActive, ptrWorkerTime(now.Add(-time.Hour)))
	driver := &cloudLifecycleWorkerMailDriver{}
	conf := cloudLifecycleWorkerConfig()
	conf.PublicURL = "https://cloud.hitkeep.eu"
	conf.MCPDocsURL = "https://docs.hitkeep.test"
	conf.CloudSupportURL = "https://support.hitkeep.test"
	worker := NewCloudLifecycleWorker(mgr, mailer.NewWithDriver(driver, nil), conf)

	worker.RunAt(context.Background(), now)

	if len(driver.sends) != 1 {
		t.Fatalf("expected one welcome send for paid team, got %d", len(driver.sends))
	}
	body := driver.sends[0].textBody
	for _, want := range []string{
		"https://cloud.hitkeep.eu/admin/team",
		"https://docs.hitkeep.test/guides/introduction/",
		"https://docs.hitkeep.test/guides/integrations/wordpress/",
		"https://support.hitkeep.test",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected text body to contain %q, got %q", want, body)
		}
	}
}

func setupCloudLifecycleWorkerStore(t *testing.T) (*database.Store, *database.TenantStoreManager) {
	t.Helper()
	store := testdb.Shared(t)
	return store, database.NewTenantStoreManager(store, t.TempDir())
}

func seedWorkerCloudLifecycleTeam(t *testing.T, store *database.Store, email string, domain string, planCode string, status string, firstHitAt *time.Time) *database.ManagedCloudAccount {
	t.Helper()
	ctx := context.Background()
	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          email,
		HashedPassword: "hashed",
		TeamName:       email,
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(ctx, database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           planCode,
		PlanName:           planCode,
		SubscriptionStatus: status,
	}); err != nil {
		t.Fatalf("upsert billing account: %v", err)
	}
	site, err := store.CreateSite(ctx, account.UserID, domain)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if firstHitAt != nil {
		if err := store.RecordHitActivity(ctx, []*api.Hit{{
			SiteID:    site.ID,
			Timestamp: firstHitAt.UTC(),
			Path:      "/",
		}}); err != nil {
			t.Fatalf("record hit activity: %v", err)
		}
	}
	return account
}

func cloudLifecycleWorkerConfig() *config.Config {
	return &config.Config{
		PublicURL:       "https://cloud.hitkeep.test",
		MCPDocsURL:      "https://hitkeep.com",
		CloudHosted:     true,
		CloudSupportURL: "",
	}
}

func ptrWorkerTime(value time.Time) *time.Time {
	return &value
}

func TestCloudLifecycleIsFreePlanTreatsLapsedStatusesAsFree(t *testing.T) {
	for _, status := range []string{
		database.CloudSubscriptionStatusUnpaid,
		database.CloudSubscriptionStatusPaused,
		database.CloudSubscriptionStatusCanceled,
		database.CloudSubscriptionStatusIncompleteExpired,
	} {
		recipient := database.CloudLifecycleRecipient{PlanCode: database.CloudPlanPro, SubscriptionStatus: status}
		if !cloudLifecycleIsFreePlan(recipient) {
			t.Errorf("status %q: expected lapsed pro subscription to count as free", status)
		}
	}

	active := database.CloudLifecycleRecipient{PlanCode: database.CloudPlanPro, SubscriptionStatus: database.CloudSubscriptionStatusActive}
	if cloudLifecycleIsFreePlan(active) {
		t.Error("expected active pro subscription to count as paid")
	}
}

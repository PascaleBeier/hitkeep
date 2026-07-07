//go:build billing

package worker

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"hitkeep/internal/appurl"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
)

const cloudLifecycleFreeRetentionDays = 60

type CloudLifecycleWorker struct {
	tenantMgr *database.TenantStoreManager
	mailer    *mailer.Mailer
	conf      *config.Config
}

func NewCloudLifecycleWorker(tenantMgr *database.TenantStoreManager, m *mailer.Mailer, conf *config.Config) *CloudLifecycleWorker {
	return &CloudLifecycleWorker{
		tenantMgr: tenantMgr,
		mailer:    m,
		conf:      conf,
	}
}

func (w *CloudLifecycleWorker) Start(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("CloudLifecycleWorker panicked", "error", r)
		}
	}()

	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}

	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	w.Run(ctx)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Run(ctx)
		}
	}
}

func (w *CloudLifecycleWorker) Run(ctx context.Context) {
	w.RunAt(ctx, time.Now().UTC())
}

func (w *CloudLifecycleWorker) RunAt(ctx context.Context, now time.Time) {
	if w == nil || w.mailer == nil || w.tenantMgr == nil || w.tenantMgr.Shared() == nil || w.conf == nil || !w.conf.CloudHosted {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	w.processKind(ctx, database.CloudLifecycleMessageWelcome, now.UTC())
	w.processKind(ctx, database.CloudLifecycleMessageFreeRetentionReminder, now.UTC())
}

func (w *CloudLifecycleWorker) processKind(ctx context.Context, kind string, now time.Time) {
	store := w.tenantMgr.Shared()
	recipients, err := store.ListEligibleCloudLifecycleRecipients(ctx, kind, now, 100)
	if err != nil {
		slog.Error("CloudLifecycleWorker: failed to load recipients", "kind", kind, "error", err)
		return
	}

	links := cloudLifecycleLinks(w.conf)
	for _, recipient := range recipients {
		if ctx.Err() != nil {
			slog.Warn("CloudLifecycleWorker: context cancelled, halting sends", "kind", kind)
			return
		}

		email := cloudLifecycleMailable(kind, recipient, links)
		if email == nil {
			continue
		}

		if err := w.mailer.Send(recipient.Email, email); err != nil {
			slog.Error("CloudLifecycleWorker: failed to send email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, "error", err)
			if markErr := store.MarkCloudLifecycleMessageFailed(ctx, database.CloudLifecycleMessageUpdate{
				TenantID: recipient.TenantID,
				UserID:   recipient.UserID,
				Kind:     kind,
				Error:    err.Error(),
				Now:      now,
			}); markErr != nil {
				slog.Error("CloudLifecycleWorker: failed to record send failure", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, "error", markErr)
			}
			continue
		}

		if err := store.MarkCloudLifecycleMessageSent(ctx, database.CloudLifecycleMessageUpdate{
			TenantID: recipient.TenantID,
			UserID:   recipient.UserID,
			Kind:     kind,
			Now:      now,
		}); err != nil {
			slog.Error("CloudLifecycleWorker: failed to record sent email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, "error", err)
			continue
		}
		slog.Info("CloudLifecycleWorker: sent email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID)
	}
}

func cloudLifecycleMailable(kind string, recipient database.CloudLifecycleRecipient, links mailables.CloudLifecycleLinks) mailer.Mailable {
	switch kind {
	case database.CloudLifecycleMessageWelcome:
		return mailables.NewCloudWelcome(
			recipient.Locale,
			cloudLifecycleTeamName(recipient),
			cloudLifecycleSiteDomain(recipient),
			cloudLifecycleIsFreePlan(recipient),
			cloudLifecycleFreeRetentionDays,
			links,
		)
	case database.CloudLifecycleMessageFreeRetentionReminder:
		return mailables.NewCloudFreeRetentionReminder(
			recipient.Locale,
			cloudLifecycleTeamName(recipient),
			cloudLifecycleSiteDomain(recipient),
			cloudLifecycleFreeRetentionDays,
			links,
		)
	default:
		return nil
	}
}

func cloudLifecycleLinks(conf *config.Config) mailables.CloudLifecycleLinks {
	docsBase := strings.TrimRight(strings.TrimSpace(conf.MCPDocsURL), "/")
	if docsBase == "" {
		docsBase = "https://hitkeep.com"
	}

	feedbackURL := strings.TrimSpace(conf.CloudSupportURL)
	if feedbackURL == "" {
		feedbackURL = appurl.Path(docsBase, "/support/help/")
	}

	return mailables.CloudLifecycleLinks{
		DashboardURL: appurl.Path(conf.PublicURL, "/admin/team"),
		UpgradeURL:   appurl.Path(conf.PublicURL, "/admin/team/overview"),
		FundingURL:   appurl.Path(docsBase, "/support/funding/"),
		DocsURL:      appurl.Path(docsBase, "/guides/introduction/"),
		WordPressURL: appurl.Path(docsBase, "/guides/integrations/wordpress/"),
		FeedbackURL:  feedbackURL,
	}
}

func cloudLifecycleIsFreePlan(recipient database.CloudLifecycleRecipient) bool {
	switch strings.TrimSpace(recipient.SubscriptionStatus) {
	case "", database.CloudSubscriptionStatusFree, "pending_checkout", "canceled", database.CloudSubscriptionStatusChargebackLost:
		return true
	default:
		return strings.TrimSpace(recipient.PlanCode) == database.CloudPlanFree
	}
}

func cloudLifecycleTeamName(recipient database.CloudLifecycleRecipient) string {
	if name := strings.TrimSpace(recipient.TenantName); name != "" {
		return name
	}
	return "HitKeep"
}

func cloudLifecycleSiteDomain(recipient database.CloudLifecycleRecipient) string {
	if domain := strings.TrimSpace(recipient.SiteDomain); domain != "" {
		return domain
	}
	return "your site"
}

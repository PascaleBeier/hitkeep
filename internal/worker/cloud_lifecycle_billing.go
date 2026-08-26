//go:build billing

package worker

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"hitkeep/appurl"
	"hitkeep/hklog"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
)

const cloudLifecycleFreeRetentionDays = database.CloudFreePlanRetentionDays

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

// Start waits until 09:00 UTC, then ticks every 24 hours.
func (w *CloudLifecycleWorker) Start(ctx context.Context) {
	runDailyAtUTC(ctx, "CloudLifecycleWorker", 9, w.Run)
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
	w.processKind(ctx, database.CloudLifecycleMessageFreeRetentionPreTrim, now.UTC())
	w.processKind(ctx, database.CloudLifecycleMessageFreeLimitReminder, now.UTC())
}

func (w *CloudLifecycleWorker) processKind(ctx context.Context, kind string, now time.Time) {
	store := w.tenantMgr.Control()
	var recipients []database.CloudLifecycleRecipient
	var err error
	if w.tenantMgr.TenantDataPlaneEnabled() {
		recipients, err = w.tenantMgr.ListEligibleCloudLifecycleRecipients(ctx, kind, now, 100)
	} else {
		recipients, err = store.ListEligibleCloudLifecycleRecipients(ctx, kind, now, 100)
	}
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("CloudLifecycleWorker: failed to load recipients", "kind", kind, "error", err)
		return
	}

	links := cloudLifecycleLinks(w.conf)
	for _, recipient := range recipients {
		if ctx.Err() != nil {
			hklog.LoggerFromContext(ctx).Warn("CloudLifecycleWorker: context cancelled, halting sends", "kind", kind)
			return
		}

		email := cloudLifecycleMailable(kind, recipient, links)
		if email == nil {
			continue
		}

		if err := w.mailer.Send(recipient.Email, email); err != nil {
			details := mailer.DescribeError(err)
			hklog.LoggerFromContext(ctx).Error("CloudLifecycleWorker: failed to send email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, slog.Group("mail",
				"error_code", "smtp_send_failed",
				"error_stage", details.Stage,
				"error_kind", details.Kind,
				"error_message", details.Message,
				"smtp_code", details.SMTPCode,
			))
			if markErr := store.MarkCloudLifecycleMessageFailed(ctx, database.CloudLifecycleMessageUpdate{
				TenantID: recipient.TenantID,
				UserID:   recipient.UserID,
				Kind:     kind,
				Error:    details.Message,
				Now:      now,
			}); markErr != nil {
				hklog.LoggerFromContext(ctx).Error("CloudLifecycleWorker: failed to record send failure", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, "error", markErr)
			}
			continue
		}

		if err := store.MarkCloudLifecycleMessageSent(ctx, database.CloudLifecycleMessageUpdate{
			TenantID: recipient.TenantID,
			UserID:   recipient.UserID,
			Kind:     kind,
			Now:      now,
		}); err != nil {
			hklog.LoggerFromContext(ctx).Error("CloudLifecycleWorker: failed to record sent email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID, "error", err)
			continue
		}
		hklog.LoggerFromContext(ctx).Debug("CloudLifecycleWorker: sent email", "kind", kind, "tenant_id", recipient.TenantID, "user_id", recipient.UserID)
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
	case database.CloudLifecycleMessageFreeRetentionPreTrim:
		return mailables.NewCloudFreeRetentionPreTrim(
			recipient.Locale,
			cloudLifecycleTeamName(recipient),
			cloudLifecycleSiteDomain(recipient),
			cloudLifecycleFreeRetentionDays,
			recipient.FirstHitAt.UTC().AddDate(0, 0, cloudLifecycleFreeRetentionDays).Format("2006-01-02"),
			links,
		)
	case database.CloudLifecycleMessageFreeLimitReminder:
		return mailables.NewCloudFreeLimitReminder(
			recipient.Locale,
			cloudLifecycleTeamName(recipient),
			database.CloudFreePlanSiteLimit,
			database.CloudFreePlanMemberLimit,
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
	if database.CloudSubscriptionStatusIsFree(recipient.SubscriptionStatus) {
		return true
	}
	return strings.TrimSpace(recipient.PlanCode) == database.CloudPlanFree
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

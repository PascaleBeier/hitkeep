package worker

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/hklog"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	opportunitysvc "hitkeep/internal/opportunities"
	"hitkeep/internal/reporting"
)

// ReportContentRequest contains the recipient- and occurrence-specific inputs
// needed to build the same report content for scheduled delivery, preview, and
// test sends.
type ReportContentRequest struct {
	Report          *api.ReportDefinition
	RecipientUserID uuid.UUID
	RecipientLocale string
	SelfContained   bool
	ScheduledFor    time.Time
	PeriodStart     time.Time
	PeriodEnd       time.Time
}

// ReportContentBuilder produces report mailables without sending or persisting
// them. Keeping content generation separate from transport prevents previews
// and test sends from drifting away from scheduled delivery.
type ReportContentBuilder struct {
	shared                *database.Store
	resolveAnalyticsStore func(context.Context, uuid.UUID) (*database.Store, error)
	pubURL                string
}

func NewReportContentBuilder(
	shared *database.Store,
	resolveAnalyticsStore func(context.Context, uuid.UUID) (*database.Store, error),
	pubURL string,
) *ReportContentBuilder {
	return &ReportContentBuilder{shared: shared, resolveAnalyticsStore: resolveAnalyticsStore, pubURL: pubURL}
}

func (w *ReportWorker) runScheduleScanner(ctx context.Context) {
	w.Run(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.RunAt(ctx, now.UTC())
		}
	}
}

func (w *ReportWorker) RunAt(ctx context.Context, now time.Time) {
	if w.mailer == nil {
		hklog.LoggerFromContext(ctx).Debug("ReportWorker: mail delivery unavailable")
		return
	}
	shared := w.tenantMgr.Shared()
	if err := shared.RecoverStaleReportDeliveries(ctx, now); err != nil {
		hklog.LoggerFromContext(ctx).Error("ReportWorker: failed to recover interrupted deliveries", "error_code", "delivery_recovery_failed")
		return
	}
	if _, err := shared.ClaimDueReportRuns(ctx, now); err != nil {
		hklog.LoggerFromContext(ctx).Error("ReportWorker: failed to claim due schedules", "error_code", "schedule_claim_failed")
		return
	}
	deliveryIDs, err := shared.ListDueReportDeliveries(ctx, now, 100)
	if err != nil {
		hklog.LoggerFromContext(ctx).Error("ReportWorker: failed to list due deliveries", "error_code", "delivery_list_failed")
		return
	}
	for _, deliveryID := range deliveryIDs {
		if ctx.Err() != nil {
			return
		}
		claimed, err := shared.ClaimReportDelivery(ctx, deliveryID, now)
		if err != nil || !claimed {
			continue
		}
		w.processNamedDelivery(ctx, deliveryID, now)
	}
}

func (w *ReportWorker) processNamedDelivery(ctx context.Context, deliveryID uuid.UUID, now time.Time) {
	shared := w.tenantMgr.Shared()
	delivery, err := shared.GetPendingReportDelivery(ctx, deliveryID)
	if err != nil {
		_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "delivery_state_unavailable", now)
		return
	}
	canAccess := false
	if delivery.RecipientKind == api.ReportRecipientKindMember && delivery.RecipientUserID != nil {
		canAccess, err = shared.RecipientCanAccessReportSites(ctx, delivery.Report, *delivery.RecipientUserID)
	} else if delivery.RecipientKind == api.ReportRecipientKindExternal {
		if !w.allowsExternalReportRecipient(ctx, delivery.Report) {
			_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "plan_upgrade_required", now)
			_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
			hklog.LoggerFromContext(ctx).Debug("ReportWorker: skipped external recipient without plan entitlement", "report_id", delivery.ReportID, "delivery_id", deliveryID, "recipient_id", delivery.RecipientID)
			return
		}
		canAccess, err = shared.ExternalRecipientCanAccessReportSites(ctx, delivery.Report)
	}
	if err != nil || !canAccess {
		_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "site_access_lost", now)
		_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
		hklog.LoggerFromContext(ctx).Debug("ReportWorker: skipped ineligible recipient", "report_id", delivery.ReportID, "delivery_id", deliveryID, "recipient_id", delivery.RecipientID)
		return
	}

	recipientUserID := uuid.Nil
	if delivery.RecipientUserID != nil {
		recipientUserID = *delivery.RecipientUserID
	}
	email, shouldSend, err := w.contentBuilder().Build(ctx, ReportContentRequest{
		Report: delivery.Report, RecipientUserID: recipientUserID, RecipientLocale: delivery.RecipientLocale,
		SelfContained: delivery.RecipientKind == api.ReportRecipientKindExternal,
		ScheduledFor:  delivery.ScheduledFor, PeriodStart: delivery.PeriodStart, PeriodEnd: delivery.PeriodEnd,
	})
	if err != nil {
		_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "content_unavailable", now)
		_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
		hklog.LoggerFromContext(ctx).Warn("ReportWorker: report content unavailable", "report_id", delivery.ReportID, "delivery_id", deliveryID, "error_code", "content_unavailable")
		return
	}
	if !shouldSend {
		_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "empty_report_suppressed", now)
		_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
		hklog.LoggerFromContext(ctx).Debug("ReportWorker: suppressed empty report", "report_id", delivery.ReportID, "delivery_id", deliveryID)
		return
	}

	token := reporting.UnsubscribeToken(w.tokenSecret, delivery.ReportID, delivery.RecipientID)
	tokenHash := reporting.UnsubscribeTokenHash(token)
	if err := shared.SetReportRecipientTokenHash(ctx, delivery.RecipientID, tokenHash); err != nil {
		_ = shared.MarkReportDeliverySkipped(ctx, deliveryID, "unsubscribe_unavailable", now)
		_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
		return
	}
	unsubscribeURL := appurl.Path(w.pubURL, "/api/reports/unsubscribe/"+url.PathEscape(token))
	email = mailables.WithReportUnsubscribe(email, unsubscribeURL)
	options := mailer.SendOptions{
		MessageID: delivery.MessageID,
		Headers: map[string]string{
			"List-Unsubscribe":      "<" + unsubscribeURL + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			"Auto-Submitted":        "auto-generated",
		},
	}
	if err := w.mailer.SendWithOptions(delivery.RecipientEmail, email, options); err != nil {
		code := "smtp_send_failed"
		if errors.Is(err, mailer.ErrMailerDisabled) {
			code = "smtp_unavailable"
		}
		details := mailer.DescribeError(err)
		_ = shared.MarkReportDeliveryFailed(ctx, deliveryID, delivery.AttemptCount, code, now)
		_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
		hklog.LoggerFromContext(ctx).Warn("ReportWorker: mail delivery failed", "report_id", delivery.ReportID, "run_id", delivery.RunID, "delivery_id", deliveryID, "recipient_id", delivery.RecipientID, reportMailFailureLogAttr(delivery.RecipientKind, delivery.MessageID, delivery.AttemptCount, code, details))
		return
	}
	_ = shared.MarkReportDeliveryAccepted(ctx, deliveryID, now)
	_ = shared.FinalizeReportRun(ctx, delivery.RunID, now)
	hklog.LoggerFromContext(ctx).Debug("ReportWorker: delivery accepted by mail server", "report_id", delivery.ReportID, "run_id", delivery.RunID, "delivery_id", deliveryID, "recipient_id", delivery.RecipientID)
}

func reportMailFailureLogAttr(recipientKind api.ReportRecipientKind, messageID string, attempt int, code string, details mailer.ErrorDetails) slog.Attr {
	return slog.Group("mail",
		"recipient_kind", recipientKind,
		"message_id", messageID,
		"attempt", attempt,
		"error_code", code,
		"error_stage", details.Stage,
		"error_kind", details.Kind,
		"error_message", details.Message,
		"smtp_code", details.SMTPCode,
	)
}

func (w *ReportWorker) allowsExternalReportRecipient(ctx context.Context, report *api.ReportDefinition) bool {
	if report == nil || report.TenantID == nil {
		return false
	}
	return w.limits == nil || w.limits.AllowsExternalReportRecipients(ctx, uuid.Nil, *report.TenantID)
}

func (w *ReportWorker) contentBuilder() *ReportContentBuilder {
	return NewReportContentBuilder(w.tenantMgr.Shared(), w.resolveAnalyticsStore, w.pubURL)
}

func (b *ReportContentBuilder) Build(ctx context.Context, request ReportContentRequest) (mailer.Mailable, bool, error) {
	report := request.Report
	if report == nil || b == nil || b.shared == nil || b.resolveAnalyticsStore == nil {
		return nil, false, errors.New("report content dependencies unavailable")
	}
	sites, err := b.shared.ResolveReportSites(ctx, report, request.RecipientUserID)
	if err != nil {
		return nil, false, err
	}
	if len(sites) == 0 {
		return nil, false, nil
	}
	freqLabel := mailables.LocalizedReportFrequencyLabel(request.RecipientLocale, string(report.Schedule.Frequency))
	periodLabel := localizedScheduledPeriodLabel(request.RecipientLocale, report.Schedule, request.PeriodStart, request.PeriodEnd)
	settingsURL := appurl.Path(b.pubURL, "/settings/reports") + "?report=" + report.ID.String()
	if request.SelfContained {
		settingsURL = ""
	}

	switch report.Preset {
	case api.ReportPresetSiteSummary:
		site := sites[0]
		current, previous, trend, err := b.siteReportStats(ctx, site.ID, request.RecipientUserID, report.Schedule, request.ScheduledFor)
		if err != nil {
			return nil, false, err
		}
		dashboardURL := b.reportDashboardURL(site.ID, report.Schedule, request.PeriodStart, request.PeriodEnd)
		if request.SelfContained {
			dashboardURL = ""
		}
		return mailables.NewSiteAnalyticsReport(request.RecipientLocale, site.Domain, periodLabel, freqLabel,
			dashboardURL, settingsURL, current, previous, trend), true, nil

	case api.ReportPresetPortfolioDigest:
		entries := make([]mailables.DigestSiteEntry, 0, len(sites))
		for _, site := range sites {
			current, previous, _, err := b.siteReportStats(ctx, site.ID, request.RecipientUserID, report.Schedule, request.ScheduledFor)
			if err != nil {
				continue
			}
			dashURL := b.reportDashboardURL(site.ID, report.Schedule, request.PeriodStart, request.PeriodEnd)
			if request.SelfContained {
				dashURL = ""
			}
			entries = append(entries, mailables.DigestSiteEntry{
				Domain: site.Domain, DashURL: dashURL,
				Pageviews: current.Pageviews, PrevPageviews: previous.Pageviews,
				Visitors: current.Visitors, PrevVisitors: previous.Visitors,
				Goals: goalCompletions(current.Goals), PrevGoals: goalCompletions(previous.Goals),
			})
		}
		if len(entries) == 0 {
			return nil, false, nil
		}
		digestFreq := mailables.LocalizedDigestFrequencyLabel(request.RecipientLocale, string(report.Schedule.Frequency))
		subjectFreq := mailables.LocalizedDigestSubjectFrequencyLabel(request.RecipientLocale, string(report.Schedule.Frequency))
		dashboardURL := appurl.Path(b.pubURL, "/dashboard")
		if request.SelfContained {
			dashboardURL = ""
		}
		return mailables.NewAnalyticsDigestWithSubjectLabel(request.RecipientLocale, periodLabel, digestFreq,
			subjectFreq, dashboardURL, settingsURL, entries), true, nil

	case api.ReportPresetOpportunityBrief:
		site := sites[0]
		preview, err := opportunitysvc.SelectDigestPreviewForSite(ctx, b.shared, opportunitysvc.DigestPreviewForSiteInput{
			SiteID: site.ID, Frequency: report.Schedule.Frequency,
		})
		if err != nil {
			return nil, false, err
		}
		if !preview.ShouldSend {
			return nil, false, nil
		}
		opportunitiesURL := b.reportPageURL("/opportunities", site.ID, report.Schedule, request.PeriodStart, request.PeriodEnd)
		if request.SelfContained {
			opportunitiesURL = ""
		}
		return mailables.NewOpportunityDigestWithSubjectLabel(request.RecipientLocale, site.Domain, periodLabel,
			freqLabel, freqLabel, opportunitiesURL, settingsURL, preview), true, nil
	default:
		return nil, false, nil
	}
}

func (b *ReportContentBuilder) siteReportStats(ctx context.Context, siteID, userID uuid.UUID, schedule api.ReportSchedule, scheduledFor time.Time) (mailables.ReportStats, mailables.ReportStats, []int, error) {
	start, end, previousStart, previousEnd, err := reporting.PeriodBounds(schedule, scheduledFor)
	if err != nil {
		return mailables.ReportStats{}, mailables.ReportStats{}, nil, err
	}
	store, err := b.resolveAnalyticsStore(ctx, siteID)
	if err != nil {
		return mailables.ReportStats{}, mailables.ReportStats{}, nil, err
	}
	currentStats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: siteID, UserID: userID, Start: start, End: inclusivePeriodEnd(start, end)})
	if err != nil {
		return mailables.ReportStats{}, mailables.ReportStats{}, nil, err
	}
	previousStats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: siteID, UserID: userID, Start: previousStart, End: inclusivePeriodEnd(previousStart, previousEnd)})
	if err != nil {
		return mailables.ReportStats{}, mailables.ReportStats{}, nil, err
	}
	current := reportStats(currentStats)
	previous := reportStats(previousStats)
	trend, err := store.GetDailyPageviewsForPeriod(ctx, siteID, start, end)
	if err != nil {
		trend = nil
	}
	return current, previous, trend, nil
}

func reportStats(stats *api.SiteStats) mailables.ReportStats {
	topPages := stats.TopPages
	if len(topPages) > 5 {
		topPages = topPages[:5]
	}
	topReferrers := stats.TopReferrers
	if len(topReferrers) > 5 {
		topReferrers = topReferrers[:5]
	}
	return mailables.ReportStats{
		Pageviews: stats.TotalPageviews, Visitors: stats.UniqueSessions,
		BounceRate: stats.BounceRate, AvgSessionDuration: stats.AvgSessionDuration,
		TopPages: topPages, TopReferrers: topReferrers, Goals: stats.Goals,
	}
}

func goalCompletions(goals []api.GoalStats) int {
	total := 0
	for _, goal := range goals {
		total += goal.Conversions
	}
	return total
}

func localizedScheduledPeriodLabel(locale string, schedule api.ReportSchedule, start, end time.Time) string {
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		loc = time.UTC
	}
	start = start.In(loc)
	end = end.In(loc)
	return reportPeriodLabel(locale, schedule.Frequency, start, end)
}

func (b *ReportContentBuilder) reportDashboardURL(siteID uuid.UUID, schedule api.ReportSchedule, start, end time.Time) string {
	return b.reportPageURL("/dashboard", siteID, schedule, start, end)
}

func (b *ReportContentBuilder) reportPageURL(path string, siteID uuid.UUID, schedule api.ReportSchedule, start, end time.Time) string {
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		loc = time.UTC
	}
	query := url.Values{}
	query.Set("site", siteID.String())
	query.Set("from", start.In(loc).Format(time.DateOnly))
	query.Set("to", end.Add(-time.Nanosecond).In(loc).Format(time.DateOnly))
	query.Set("report", strconv.FormatBool(true))
	return appurl.Path(b.pubURL, path) + "?" + query.Encode()
}

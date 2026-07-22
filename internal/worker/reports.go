package worker

import (
	"context"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
)

// ReportWorker sends scheduled analytics emails (daily / weekly / monthly).
type ReportWorker struct {
	tenantMgr   *database.TenantStoreManager
	mailer      *mailer.Mailer
	pubURL      string
	tokenSecret string
	limits      *entitlements.Service
}

// WithEntitlements applies managed-cloud plan policy to scheduled delivery.
// It remains optional so focused worker tests can construct the worker without
// a cloud billing fixture.
func (w *ReportWorker) WithEntitlements(limits *entitlements.Service) *ReportWorker {
	w.limits = limits
	return w
}

// NewReportWorker creates a ReportWorker. pubURL is used to build dashboard deep-links.
func NewReportWorker(tenantMgr *database.TenantStoreManager, m *mailer.Mailer, pubURL string, tokenSecrets ...string) *ReportWorker {
	worker := &ReportWorker{
		tenantMgr: tenantMgr,
		mailer:    m,
		pubURL:    pubURL,
	}
	if len(tokenSecrets) > 0 {
		worker.tokenSecret = tokenSecrets[0]
	}
	return worker
}

// Start scans due named schedules once per minute.
func (w *ReportWorker) Start(ctx context.Context) {
	w.runScheduleScanner(ctx)
}

// Run claims and processes named reports that are due now.
func (w *ReportWorker) Run(ctx context.Context) {
	w.RunAt(ctx, time.Now().UTC())
}

func (w *ReportWorker) resolveAnalyticsStore(ctx context.Context, siteID uuid.UUID) (*database.Store, error) {
	store, _, err := w.tenantMgr.ResolveSiteStore(ctx, siteID)
	return store, err
}

func reportPeriodLabel(locale string, freq api.ReportFrequency, start time.Time, endExclusive time.Time) string {
	if !endExclusive.After(start) {
		return mailables.FormatPeriodLabelForLocale(locale, start, start)
	}

	endInclusive := endExclusive.Add(-time.Second)
	if freq == api.ReportFrequencyDaily {
		return mailables.FormatSingleDayLabel(locale, endInclusive)
	}
	if freq == api.ReportFrequencyMonthly {
		return mailables.FormatMonthYearLabel(locale, endInclusive)
	}

	return mailables.FormatPeriodLabelForLocale(locale, start, endInclusive)
}

func inclusivePeriodEnd(start time.Time, endExclusive time.Time) time.Time {
	if !endExclusive.After(start) {
		return endExclusive
	}
	return endExclusive.Add(-time.Nanosecond)
}

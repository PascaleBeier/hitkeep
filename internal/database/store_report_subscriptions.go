package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/reporting"
)

var ErrSiteAccessRequired = errors.New("site access required")

// PendingSiteReport holds the data needed to send a per-site report email.
type PendingSiteReport struct {
	UserID     uuid.UUID
	UserEmail  string
	UserLocale string
	SiteID     uuid.UUID
	Domain     string
}

// DigestSite is a single site entry within a PendingDigest.
type DigestSite struct {
	SiteID uuid.UUID
	Domain string
}

// PendingDigest holds the data needed to send a consolidated digest email.
type PendingDigest struct {
	UserID     uuid.UUID
	UserEmail  string
	UserLocale string
	Sites      []DigestSite
}

func (s *Store) listReportAccessibleSites(ctx context.Context, userID uuid.UUID) ([]DigestSite, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT s.id, s.domain
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = ?
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
		WHERE ta.tenant_id IS NULL
		  AND (s.user_id = ? OR sm.user_id IS NOT NULL)
		ORDER BY s.domain ASC
	`, defaultTenantID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := make([]DigestSite, 0)
	for rows.Next() {
		var site DigestSite
		if err := rows.Scan(&site.SiteID, &site.Domain); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sites, nil
}

func (s *Store) CanAccessSiteForReports(ctx context.Context, userID, siteID uuid.UUID) (bool, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return false, err
	}

	var count int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = ?
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
		WHERE s.id = ?
		  AND ta.tenant_id IS NULL
		  AND (s.user_id = ? OR sm.user_id IS NOT NULL)
	`, defaultTenantID, userID, userID, siteID, userID).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetReportSubscriptions returns the full subscription state for a user across all their accessible sites.
func (s *Store) GetReportSubscriptions(ctx context.Context, userID uuid.UUID) (*api.ReportSubscriptions, error) {
	sites, err := s.listReportAccessibleSites(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Read legacy-managed definitions so this endpoint remains a compatibility
	// façade over the named-report model.
	subRows, err := s.db.QueryContext(ctx, `
		SELECT rds.site_id, rd.frequency, rd.status = 'active'
		FROM report_definitions rd
		JOIN report_definition_sites rds ON rds.report_id = rd.id
		WHERE rd.owner_user_id = ?
		  AND rd.source = 'legacy'
		  AND rd.preset = 'site_summary'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer subRows.Close()

	type subKey struct {
		siteID uuid.UUID
		freq   string
	}
	subMap := make(map[subKey]bool)
	for subRows.Next() {
		var siteID uuid.UUID
		var freq string
		var enabled bool
		if err := subRows.Scan(&siteID, &freq, &enabled); err != nil {
			return nil, err
		}
		subMap[subKey{siteID: siteID, freq: freq}] = enabled
	}
	if err := subRows.Err(); err != nil {
		return nil, err
	}
	subRows.Close()

	siteSubscriptions := make([]api.SiteReportSubscription, 0, len(sites))
	for _, site := range sites {
		siteSubscriptions = append(siteSubscriptions, api.SiteReportSubscription{
			SiteID:  site.SiteID,
			Domain:  site.Domain,
			Daily:   subMap[subKey{siteID: site.SiteID, freq: "daily"}],
			Weekly:  subMap[subKey{siteID: site.SiteID, freq: "weekly"}],
			Monthly: subMap[subKey{siteID: site.SiteID, freq: "monthly"}],
		})
	}

	// Fetch legacy-managed digest definitions.
	digestRows, err := s.db.QueryContext(ctx, `
		SELECT frequency, status = 'active'
		FROM report_definitions
		WHERE owner_user_id = ?
		  AND source = 'legacy'
		  AND preset = 'portfolio_digest'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer digestRows.Close()

	digestMap := make(map[string]bool)
	for digestRows.Next() {
		var freq string
		var enabled bool
		if err := digestRows.Scan(&freq, &enabled); err != nil {
			return nil, err
		}
		digestMap[freq] = enabled
	}
	if err := digestRows.Err(); err != nil {
		return nil, err
	}

	return &api.ReportSubscriptions{
		Sites: siteSubscriptions,
		Digest: api.DigestSubscription{
			Daily:   digestMap["daily"],
			Weekly:  digestMap["weekly"],
			Monthly: digestMap["monthly"],
		},
	}, nil
}

// UpsertSiteReportSubscription inserts or updates a per-site subscription for the given frequency.
func (s *Store) UpsertSiteReportSubscription(ctx context.Context, userID, siteID uuid.UUID, freq api.ReportFrequency, enabled bool) error {
	hasAccess, err := s.CanAccessSiteForReports(ctx, userID, siteID)
	if err != nil {
		return err
	}
	if !hasAccess {
		return ErrSiteAccessRequired
	}

	return s.upsertLegacyReportDefinition(ctx, userID, &siteID, freq, enabled)
}

// UpsertDigestSubscription inserts or updates a digest subscription for the given frequency.
func (s *Store) UpsertDigestSubscription(ctx context.Context, userID uuid.UUID, freq api.ReportFrequency, enabled bool) error {
	return s.upsertLegacyReportDefinition(ctx, userID, nil, freq, enabled)
}

func (s *Store) upsertLegacyReportDefinition(ctx context.Context, userID uuid.UUID, siteID *uuid.UUID, freq api.ReportFrequency, enabled bool) error {
	schedule := api.ReportSchedule{Frequency: freq, Timezone: "UTC", LocalTime: "08:00"}
	if freq == api.ReportFrequencyWeekly {
		monday := int(time.Monday)
		schedule.WeeklyDay = &monday
	}
	if freq == api.ReportFrequencyMonthly {
		first := 1
		schedule.MonthlyDay = &first
	}
	if err := reporting.ValidateSchedule(schedule); err != nil {
		return err
	}
	now := time.Now().UTC()
	nextRun, err := reporting.NextOccurrence(schedule, now)
	if err != nil {
		return err
	}
	legacyKey := fmt.Sprintf("digest:%s:%s", userID, freq)
	preset := api.ReportPresetPortfolioDigest
	siteMode := api.ReportSiteModeAllAccessible
	name := fmt.Sprintf("Portfolio Digest · %s", freq)
	var siteTenantID uuid.UUID
	if siteID != nil {
		legacyKey = fmt.Sprintf("site:%s:%s:%s", userID, *siteID, freq)
		preset = api.ReportPresetSiteSummary
		siteMode = api.ReportSiteModeSelected
		var domain string
		if err := s.db.QueryRowContext(ctx, "SELECT domain FROM sites WHERE id = ?", *siteID).Scan(&domain); err != nil {
			return err
		}
		name = "Site Summary · " + domain
		siteTenantID, err = s.GetSiteTenantID(ctx, *siteID)
		if err != nil {
			return err
		}
	}
	status := api.ReportStatusPaused
	var defaultNextRun any
	if enabled {
		status = api.ReportStatusActive
		defaultNextRun = nextRun
	}

	return s.Transact(ctx, func(tx *sql.Tx) error {
		if siteID == nil {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO digest_subscriptions (id, user_id, frequency, enabled, created_at, updated_at)
				VALUES (uuidv7(), ?, ?, ?, ?, ?)
				ON CONFLICT (user_id, frequency) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at
			`, userID, freq, enabled, now, now); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `
			INSERT INTO site_report_subscriptions (id, user_id, site_id, frequency, enabled, created_at, updated_at)
			VALUES (uuidv7(), ?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, site_id, frequency) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at
		`, userID, *siteID, freq, enabled, now, now); err != nil {
			return err
		}

		var reportID uuid.UUID
		var storedFrequency, storedTimezone, storedLocalTime string
		var storedWeeklyDay, storedMonthlyDay sql.NullInt64
		err := tx.QueryRowContext(ctx, `
			SELECT id, frequency, timezone, local_time, weekly_day, monthly_day
			FROM report_definitions WHERE legacy_key = ?
		`, legacyKey).Scan(&reportID, &storedFrequency, &storedTimezone, &storedLocalTime, &storedWeeklyDay, &storedMonthlyDay)
		if err == nil {
			var storedNextRun any
			if enabled {
				storedSchedule := api.ReportSchedule{
					Frequency: api.ReportFrequency(storedFrequency), Timezone: storedTimezone, LocalTime: storedLocalTime,
				}
				if storedWeeklyDay.Valid {
					value := int(storedWeeklyDay.Int64)
					storedSchedule.WeeklyDay = &value
				}
				if storedMonthlyDay.Valid {
					value := int(storedMonthlyDay.Int64)
					storedSchedule.MonthlyDay = &value
				}
				value, nextErr := reporting.NextOccurrence(storedSchedule, now)
				if nextErr != nil {
					return nextErr
				}
				storedNextRun = value
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE report_definitions SET status = ?, next_run_at = ?, updated_at = ? WHERE id = ?
			`, status, storedNextRun, now, reportID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !enabled {
			return nil
		}
		reportID = uuid.New()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO report_definitions (
				id, owner_user_id, created_by, name, scope, preset, site_mode, frequency,
				timezone, local_time, weekly_day, monthly_day, status, source, legacy_key,
				next_run_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, 'personal', ?, ?, ?, 'UTC', '08:00', ?, ?, ?, 'legacy', ?, ?, ?, ?)
		`, reportID, userID, userID, name, preset, siteMode, freq, nullableInt(schedule.WeeklyDay),
			nullableInt(schedule.MonthlyDay), status, legacyKey, defaultNextRun, now, now); err != nil {
			return err
		}
		if siteID != nil {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO report_definition_sites (id, report_id, site_id, tenant_id, created_at)
				VALUES (?, ?, ?, ?, ?)
			`, uuid.New(), reportID, *siteID, siteTenantID, now); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO report_recipients (id, report_id, user_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, uuid.New(), reportID, userID, now, now)
		return err
	})
}

// GetPendingSiteReports returns all active per-site report subscriptions for the given frequency,
// verifying that the user still has access to each site.
func (s *Store) GetPendingSiteReports(ctx context.Context, freq api.ReportFrequency) ([]PendingSiteReport, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT srs.user_id, u.email, COALESCE(up.default_locale, 'en'), srs.site_id, si.domain
		FROM site_report_subscriptions srs
		JOIN users u ON u.id = srs.user_id
		LEFT JOIN user_preferences up ON up.user_id = srs.user_id
		JOIN sites si ON si.id = srs.site_id
		LEFT JOIN site_tenants st ON st.site_id = si.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = srs.user_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		LEFT JOIN site_members sm ON sm.site_id = srs.site_id AND sm.user_id = srs.user_id
		WHERE srs.frequency = ?
		  AND srs.enabled = true
		  AND ta.tenant_id IS NULL
		  AND (si.user_id = srs.user_id OR sm.user_id IS NOT NULL)
	`, defaultTenantID, string(freq))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PendingSiteReport
	for rows.Next() {
		var r PendingSiteReport
		if err := rows.Scan(&r.UserID, &r.UserEmail, &r.UserLocale, &r.SiteID, &r.Domain); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetDailyPageviewsForPeriod returns daily pageview counts from hit_rollups_daily
// for the given site over [start, end), ordered oldest-first.
func (s *Store) GetDailyPageviewsForPeriod(ctx context.Context, siteID uuid.UUID, start, end time.Time) ([]int, error) {
	refreshEnd := end
	if end.After(start) {
		refreshEnd = end.Add(-time.Nanosecond)
	}
	if err := s.refreshDirtyRollupsInRange(ctx, siteID, dirtyRollupHit, rollupDaily, start, refreshEnd); err != nil {
		return nil, fmt.Errorf("refresh daily hit rollups: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT pageviews
		FROM hit_rollups_daily
		WHERE site_id = ?
		  AND bucket >= ? AND bucket < ?
		ORDER BY bucket ASC
	`, siteID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("query daily pageviews: %w", err)
	}
	defer rows.Close()

	var result []int
	for rows.Next() {
		var pv int
		if err := rows.Scan(&pv); err != nil {
			return nil, fmt.Errorf("scan daily pageviews: %w", err)
		}
		result = append(result, pv)
	}
	return result, rows.Err()
}

// GetPendingDigests returns all users with an active digest subscription for the given frequency,
// along with all sites they have access to.
func (s *Store) GetPendingDigests(ctx context.Context, freq api.ReportFrequency) ([]PendingDigest, error) {
	// Get subscribed users.
	userRows, err := s.db.QueryContext(ctx, `
		SELECT ds.user_id, u.email, COALESCE(up.default_locale, 'en')
		FROM digest_subscriptions ds
		JOIN users u ON u.id = ds.user_id
		LEFT JOIN user_preferences up ON up.user_id = ds.user_id
		WHERE ds.frequency = ? AND ds.enabled = true
	`, string(freq))
	if err != nil {
		return nil, err
	}
	defer userRows.Close()

	type subscribedUser struct {
		userID uuid.UUID
		email  string
		locale string
	}
	var users []subscribedUser
	for userRows.Next() {
		var su subscribedUser
		if err := userRows.Scan(&su.userID, &su.email, &su.locale); err != nil {
			return nil, err
		}
		users = append(users, su)
	}
	if err := userRows.Err(); err != nil {
		return nil, err
	}
	userRows.Close()

	result := make([]PendingDigest, 0, len(users))
	for _, u := range users {
		sites, err := s.listReportAccessibleSites(ctx, u.userID)
		if err != nil {
			return nil, err
		}

		if len(sites) > 0 {
			result = append(result, PendingDigest{
				UserID:     u.userID,
				UserEmail:  u.email,
				UserLocale: u.locale,
				Sites:      sites,
			})
		}
	}
	return result, nil
}

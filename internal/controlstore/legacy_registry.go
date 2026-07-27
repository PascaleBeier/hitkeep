package controlstore

import (
	"fmt"
	"sort"
)

// LegacyTableDisposition describes the only three legal outcomes for a base
// table in the post-split DuckDB control database.
type LegacyTableDisposition uint8

const (
	LegacyTableCopy LegacyTableDisposition = iota + 1
	LegacyTableDiscardEmptyAnalytics
	LegacyTableReplaceMetadata
)

// legacyControlTables is deliberately parents-first. The importer compares
// this closed registry with the live DuckDB catalog before copying any row.
var legacyControlTables = []string{
	"data_migrations",
	"users",
	"tenants",
	"sites",
	"site_tenants",
	"tenant_members",
	"site_members",
	"instance_roles",
	"user_preferences",
	"remember_me_tokens",
	"user_totp_factors",
	"user_recovery_codes",
	"user_passkeys",
	"pending_signups",
	"social_identities",
	"pending_social_confirmations",
	"api_clients",
	"api_client_site_roles",
	"tenant_archives",
	"team_invites",
	"team_sso_configs",
	"team_sso_domains",
	"sso_identities",
	"instance_audit_log",
	"share_links",
	"qr_codes",
	"qr_code_assets",
	"qr_code_share_links",
	"site_imports",
	"site_import_files",
	"google_search_console_connections",
	"google_search_console_properties",
	"google_search_console_site_mappings",
	"google_search_console_sync_state",
	"ai_runs",
	"opportunities",
	"custom_tracking_domains",
	"traffic_exclusions",
	"cloud_billing_accounts",
	"cloud_billing_events",
	"cloud_conversion_events",
	"cloud_lifecycle_messages",
	"report_definitions",
	"report_definition_sites",
	"report_recipients",
	"report_runs",
	"report_deliveries",
	"webhooks",
	"webhook_event_subscriptions",
	"webhook_events",
	"webhook_deliveries",
	"webhook_delivery_attempts",
	"site_deletion_webhook_outbox",
}

var legacyEmptyAnalyticsTables = []string{
	"ai_fetches",
	"events",
	"funnel_rollups_daily",
	"funnel_rollups_hourly",
	"funnel_rollups_monthly",
	"funnels",
	"goal_rollups_daily",
	"goal_rollups_hourly",
	"goal_rollups_monthly",
	"goals",
	"hit_rollups_daily",
	"hit_rollups_hourly",
	"hit_rollups_monthly",
	"hits",
	"imported_dimension_daily",
	"imported_event_daily",
	"imported_event_dimensions_daily",
	"imported_event_properties_daily",
	"imported_traffic_daily",
	"qr_code_opens",
	"rollup_dirty_buckets",
	"search_console_facts",
	"session_rollups_daily",
	"session_rollups_hourly",
	"session_rollups_monthly",
	"site_activity_hourly_counts",
	"site_activity_summary",
	"web_vitals",
}

var legacyReplacedMetadataTables = []string{
	"hitkeep_migration_checkpoints",
	"migrations",
}

// LegacyControlTables returns a defensive copy in foreign-key-safe copy order.
func LegacyControlTables() []string {
	return append([]string(nil), legacyControlTables...)
}

// ClassifyLegacyTables requires every live source table to be classified once
// and every registered table to exist. This turns schema drift into a startup
// failure instead of silently losing a future table.
func ClassifyLegacyTables(live []string) (map[string]LegacyTableDisposition, error) {
	registry := make(map[string]LegacyTableDisposition, len(legacyControlTables)+len(legacyEmptyAnalyticsTables)+len(legacyReplacedMetadataTables))
	add := func(names []string, disposition LegacyTableDisposition) error {
		for _, name := range names {
			if previous, exists := registry[name]; exists {
				return fmt.Errorf("legacy control table %q has multiple classifications (%d and %d)", name, previous, disposition)
			}
			registry[name] = disposition
		}
		return nil
	}
	if err := add(legacyControlTables, LegacyTableCopy); err != nil {
		return nil, err
	}
	if err := add(legacyEmptyAnalyticsTables, LegacyTableDiscardEmptyAnalytics); err != nil {
		return nil, err
	}
	if err := add(legacyReplacedMetadataTables, LegacyTableReplaceMetadata); err != nil {
		return nil, err
	}

	liveSet := make(map[string]struct{}, len(live))
	for _, name := range live {
		if _, duplicate := liveSet[name]; duplicate {
			return nil, fmt.Errorf("legacy source catalog listed table %q more than once", name)
		}
		liveSet[name] = struct{}{}
		if _, known := registry[name]; !known {
			return nil, fmt.Errorf("legacy source contains unclassified base table %q", name)
		}
	}

	var missing []string
	for name := range registry {
		if _, exists := liveSet[name]; !exists {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("legacy source is missing registered base tables: %v", missing)
	}
	return registry, nil
}

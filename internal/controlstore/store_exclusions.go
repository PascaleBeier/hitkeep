package controlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	TrafficExclusionScopeInstance = "instance"
	TrafficExclusionScopeTeam     = "team"
	TrafficExclusionScopeSite     = "site"
)

type TrafficExclusionValues struct {
	Type        string
	CIDR        string
	CountryCode string
	UserAgent   string
	Path        string
	Description string
}

type SiteExclusionCIDR struct {
	SiteID uuid.UUID
	CIDR   string
}

type SiteExclusionCountry struct {
	SiteID      uuid.UUID
	CountryCode string
}

type SiteTeamMapping struct {
	SiteID uuid.UUID
	TeamID uuid.UUID
}

const (
	trafficExclusionSelect = `
		SELECT id, scope, tenant_id, site_id, rule_type, cidr, country_code,
		       user_agent, path, description, created_at, created_by
		FROM traffic_exclusions`
	listInstanceTrafficExclusionsQuery = trafficExclusionSelect + `
		WHERE scope = 'instance'
		ORDER BY created_at DESC`
	listTeamTrafficExclusionsQuery = trafficExclusionSelect + `
		WHERE scope = 'team' AND tenant_id = ?
		ORDER BY created_at DESC`
	listSiteTrafficExclusionsQuery = trafficExclusionSelect + `
		WHERE scope = 'site' AND site_id = ?
		ORDER BY created_at DESC`
	listEffectiveTeamTrafficExclusionsQuery = trafficExclusionSelect + `
		WHERE scope = 'instance' OR (scope = 'team' AND tenant_id = ?)
		ORDER BY CASE scope WHEN 'instance' THEN 0 WHEN 'team' THEN 1 ELSE 2 END, created_at DESC`
	listEffectiveSiteTrafficExclusionsQuery = trafficExclusionSelect + `
		WHERE scope = 'instance' OR (scope = 'team' AND tenant_id = ?) OR (scope = 'site' AND site_id = ?)
		ORDER BY CASE scope WHEN 'instance' THEN 0 WHEN 'team' THEN 1 ELSE 2 END, created_at DESC`
	listAllTrafficExclusionsQuery = trafficExclusionSelect + `
		ORDER BY created_at DESC`
)

func (s *Store) ListInstanceExclusions(ctx context.Context) ([]api.IPExclusion, error) {
	return s.listTrafficExclusions(ctx, listInstanceTrafficExclusionsQuery)
}

func (s *Store) ListTeamExclusions(ctx context.Context, teamID uuid.UUID) ([]api.IPExclusion, error) {
	return s.listTrafficExclusions(ctx, listTeamTrafficExclusionsQuery, teamID)
}

func (s *Store) ListSiteExclusions(ctx context.Context, siteID uuid.UUID) ([]api.IPExclusion, error) {
	return s.listTrafficExclusions(ctx, listSiteTrafficExclusionsQuery, siteID)
}

func (s *Store) ListEffectiveTeamExclusions(ctx context.Context, teamID uuid.UUID) ([]api.IPExclusion, error) {
	rules, err := s.listTrafficExclusions(ctx, listEffectiveTeamTrafficExclusionsQuery, teamID)
	if err != nil {
		return nil, err
	}
	markInheritedExclusions(rules, TrafficExclusionScopeTeam)
	return rules, nil
}

func (s *Store) ListEffectiveSiteExclusions(ctx context.Context, teamID, siteID uuid.UUID) ([]api.IPExclusion, error) {
	rules, err := s.listTrafficExclusions(ctx, listEffectiveSiteTrafficExclusionsQuery, teamID, siteID)
	if err != nil {
		return nil, err
	}
	markInheritedExclusions(rules, TrafficExclusionScopeSite)
	return rules, nil
}

func (s *Store) ListAllTrafficExclusions(ctx context.Context) ([]api.IPExclusion, error) {
	return s.listTrafficExclusions(ctx, listAllTrafficExclusionsQuery)
}

func (s *Store) listTrafficExclusions(ctx context.Context, query string, args ...any) ([]api.IPExclusion, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list traffic exclusions: %w", err)
	}
	defer rows.Close()

	rules := make([]api.IPExclusion, 0)
	for rows.Next() {
		rule, scanErr := scanTrafficExclusion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate traffic exclusions: %w", err)
	}
	return rules, nil
}

func markInheritedExclusions(rules []api.IPExclusion, ownedScope string) {
	for i := range rules {
		rules[i].Inherited = rules[i].Scope != ownedScope
		if rules[i].Inherited {
			rules[i].CreatedBy = nil
		}
	}
}

func (s *Store) CreateInstanceTrafficExclusion(ctx context.Context, values TrafficExclusionValues, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.createTrafficExclusion(ctx, TrafficExclusionScopeInstance, uuid.Nil, values, createdBy)
}

func (s *Store) CreateTeamTrafficExclusion(ctx context.Context, teamID uuid.UUID, values TrafficExclusionValues, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.createTrafficExclusion(ctx, TrafficExclusionScopeTeam, teamID, values, createdBy)
}

func (s *Store) CreateSiteTrafficExclusion(ctx context.Context, siteID uuid.UUID, values TrafficExclusionValues, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.createTrafficExclusion(ctx, TrafficExclusionScopeSite, siteID, values, createdBy)
}

func (s *Store) createTrafficExclusion(ctx context.Context, scope string, ownerID uuid.UUID, values TrafficExclusionValues, createdBy uuid.UUID) (*api.IPExclusion, error) {
	rule := &api.IPExclusion{
		ID:          uuid.New(),
		Scope:       scope,
		Type:        strings.TrimSpace(values.Type),
		CIDR:        strings.TrimSpace(values.CIDR),
		CountryCode: normalizeCountryCode(values.CountryCode),
		UserAgent:   strings.TrimSpace(values.UserAgent),
		Path:        strings.TrimSpace(values.Path),
		Description: strings.TrimSpace(values.Description),
		CreatedAt:   time.Now().UTC(),
	}

	var teamIDArg any
	var siteIDArg any
	switch scope {
	case TrafficExclusionScopeInstance:
	case TrafficExclusionScopeTeam:
		if ownerID == uuid.Nil {
			return nil, fmt.Errorf("team exclusion requires team id")
		}
		teamID := ownerID
		rule.TeamID = &teamID
		teamIDArg = teamID
	case TrafficExclusionScopeSite:
		if ownerID == uuid.Nil {
			return nil, fmt.Errorf("site exclusion requires site id")
		}
		siteID := ownerID
		rule.SiteID = &siteID
		siteIDArg = siteID
	default:
		return nil, fmt.Errorf("invalid traffic exclusion scope %q", scope)
	}

	createdByArg := nullableUUID(createdBy)
	if createdByArg != nil {
		id := createdBy
		rule.CreatedBy = &id
	}

	if err := s.exec(ctx, `
		INSERT INTO traffic_exclusions (
			id, scope, tenant_id, site_id, rule_type, cidr, country_code,
			user_agent, path, description, created_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rule.ID,
		rule.Scope,
		teamIDArg,
		siteIDArg,
		rule.Type,
		nullableString(rule.CIDR),
		nullableString(rule.CountryCode),
		nullableString(rule.UserAgent),
		nullableString(rule.Path),
		nullableString(rule.Description),
		rule.CreatedAt,
		createdByArg,
	); err != nil {
		return nil, fmt.Errorf("failed to create traffic exclusion: %w", err)
	}

	return rule, nil
}

// Compatibility wrappers keep existing Go callers aligned with the public IPExclusion contract.
func (s *Store) CreateInstanceExclusion(ctx context.Context, cidr, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.CreateInstanceTrafficExclusion(ctx, TrafficExclusionValues{Type: "cidr", CIDR: cidr, Description: description}, createdBy)
}

func (s *Store) CreateSiteExclusion(ctx context.Context, siteID uuid.UUID, cidr, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.CreateSiteTrafficExclusion(ctx, siteID, TrafficExclusionValues{Type: "cidr", CIDR: cidr, Description: description}, createdBy)
}

func (s *Store) CreateInstanceCountryExclusion(ctx context.Context, countryCode, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.CreateInstanceTrafficExclusion(ctx, TrafficExclusionValues{Type: "country", CountryCode: countryCode, Description: description}, createdBy)
}

func (s *Store) CreateSiteCountryExclusion(ctx context.Context, siteID uuid.UUID, countryCode, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	return s.CreateSiteTrafficExclusion(ctx, siteID, TrafficExclusionValues{Type: "country", CountryCode: countryCode, Description: description}, createdBy)
}

func (s *Store) DeleteInstanceExclusion(ctx context.Context, ruleID uuid.UUID) (bool, error) {
	return s.deleteTrafficExclusion(ctx, "DELETE FROM traffic_exclusions WHERE id = ? AND scope = 'instance'", ruleID)
}

func (s *Store) DeleteTeamExclusion(ctx context.Context, teamID, ruleID uuid.UUID) (bool, error) {
	return s.deleteTrafficExclusion(ctx, "DELETE FROM traffic_exclusions WHERE id = ? AND scope = 'team' AND tenant_id = ?", ruleID, teamID)
}

func (s *Store) DeleteSiteExclusion(ctx context.Context, siteID, ruleID uuid.UUID) (bool, error) {
	return s.deleteTrafficExclusion(ctx, "DELETE FROM traffic_exclusions WHERE id = ? AND scope = 'site' AND site_id = ?", ruleID, siteID)
}

func (s *Store) deleteTrafficExclusion(ctx context.Context, query string, args ...any) (bool, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to delete traffic exclusion: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine deleted traffic exclusion rows: %w", err)
	}
	return rowsAffected > 0, nil
}

func (s *Store) ListInstanceExclusionCIDRs(ctx context.Context) ([]string, error) {
	rules, err := s.ListInstanceExclusions(ctx)
	if err != nil {
		return nil, err
	}
	cidrs := make([]string, 0)
	for _, rule := range rules {
		if rule.Type == "cidr" {
			cidrs = append(cidrs, rule.CIDR)
		}
	}
	return cidrs, nil
}

func (s *Store) ListSiteExclusionCIDRs(ctx context.Context) ([]SiteExclusionCIDR, error) {
	rules, err := s.ListAllTrafficExclusions(ctx)
	if err != nil {
		return nil, err
	}
	cidrs := make([]SiteExclusionCIDR, 0)
	for _, rule := range rules {
		if rule.Scope == TrafficExclusionScopeSite && rule.Type == "cidr" && rule.SiteID != nil {
			cidrs = append(cidrs, SiteExclusionCIDR{SiteID: *rule.SiteID, CIDR: rule.CIDR})
		}
	}
	return cidrs, nil
}

func (s *Store) ListInstanceExclusionCountries(ctx context.Context) ([]string, error) {
	rules, err := s.ListInstanceExclusions(ctx)
	if err != nil {
		return nil, err
	}
	countries := make([]string, 0)
	for _, rule := range rules {
		if rule.Type == "country" {
			countries = append(countries, rule.CountryCode)
		}
	}
	return countries, nil
}

func (s *Store) ListSiteExclusionCountries(ctx context.Context) ([]SiteExclusionCountry, error) {
	rules, err := s.ListAllTrafficExclusions(ctx)
	if err != nil {
		return nil, err
	}
	countries := make([]SiteExclusionCountry, 0)
	for _, rule := range rules {
		if rule.Scope == TrafficExclusionScopeSite && rule.Type == "country" && rule.SiteID != nil {
			countries = append(countries, SiteExclusionCountry{SiteID: *rule.SiteID, CountryCode: rule.CountryCode})
		}
	}
	return countries, nil
}

func (s *Store) ListSiteTeamMappings(ctx context.Context) ([]SiteTeamMapping, error) {
	defaultTeamID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(st.tenant_id, ?)
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
	`, defaultTeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list site team mappings: %w", err)
	}
	defer rows.Close()

	mappings := make([]SiteTeamMapping, 0)
	for rows.Next() {
		var mapping SiteTeamMapping
		if err := rows.Scan(&mapping.SiteID, &mapping.TeamID); err != nil {
			return nil, fmt.Errorf("failed to scan site team mapping: %w", err)
		}
		mappings = append(mappings, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site team mappings: %w", err)
	}
	return mappings, nil
}

func scanTrafficExclusion(scanner interface{ Scan(dest ...any) error }) (api.IPExclusion, error) {
	var rule api.IPExclusion
	var teamID uuid.NullUUID
	var siteID uuid.NullUUID
	var cidr sql.NullString
	var countryCode sql.NullString
	var userAgent sql.NullString
	var exclusionPath sql.NullString
	var description sql.NullString
	var createdBy uuid.NullUUID

	if err := scanner.Scan(
		&rule.ID,
		&rule.Scope,
		&teamID,
		&siteID,
		&rule.Type,
		&cidr,
		&countryCode,
		&userAgent,
		&exclusionPath,
		&description,
		&rule.CreatedAt,
		&createdBy,
	); err != nil {
		return api.IPExclusion{}, fmt.Errorf("failed to scan traffic exclusion: %w", err)
	}

	rule.Scope = strings.TrimSpace(rule.Scope)
	rule.Type = strings.TrimSpace(rule.Type)
	rule.CIDR = strings.TrimSpace(cidr.String)
	rule.CountryCode = normalizeCountryCode(countryCode.String)
	rule.UserAgent = strings.TrimSpace(userAgent.String)
	rule.Path = strings.TrimSpace(exclusionPath.String)
	rule.Description = strings.TrimSpace(description.String)
	if teamID.Valid {
		id := teamID.UUID
		rule.TeamID = &id
	}
	if siteID.Valid {
		id := siteID.UUID
		rule.SiteID = &id
	}
	if createdBy.Valid {
		id := createdBy.UUID
		rule.CreatedBy = &id
	}
	return rule, nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func normalizeCountryCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

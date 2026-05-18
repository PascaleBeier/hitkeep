package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type SiteExclusionCIDR struct {
	SiteID uuid.UUID
	CIDR   string
}

type SiteExclusionCountry struct {
	SiteID      uuid.UUID
	CountryCode string
}

func (s *Store) ListInstanceExclusions(ctx context.Context) ([]api.IPExclusion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_type, cidr, country_code, description, created_at, created_by
		FROM (
			SELECT id, 'cidr' AS rule_type, cidr, NULL AS country_code, description, created_at, created_by
			FROM instance_exclusions
			UNION ALL
			SELECT id, 'country' AS rule_type, NULL AS cidr, country_code, description, created_at, created_by
			FROM instance_country_exclusions
		)
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list instance exclusions: %w", err)
	}
	defer rows.Close()

	rules := make([]api.IPExclusion, 0)
	for rows.Next() {
		rule, err := scanInstanceExclusion(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate instance exclusions: %w", err)
	}

	return rules, nil
}

func (s *Store) ListSiteExclusions(ctx context.Context, siteID uuid.UUID) ([]api.IPExclusion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_type, cidr, country_code, description, created_at, created_by
		FROM (
			SELECT id, 'cidr' AS rule_type, cidr, NULL AS country_code, description, created_at, created_by
			FROM site_exclusions
			WHERE site_id = ?
			UNION ALL
			SELECT id, 'country' AS rule_type, NULL AS cidr, country_code, description, created_at, created_by
			FROM site_country_exclusions
			WHERE site_id = ?
		)
		ORDER BY created_at DESC
	`, siteID, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list site exclusions: %w", err)
	}
	defer rows.Close()

	rules := make([]api.IPExclusion, 0)
	for rows.Next() {
		rule, err := scanSiteExclusion(rows, siteID)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site exclusions: %w", err)
	}

	return rules, nil
}

func (s *Store) CreateInstanceExclusion(ctx context.Context, cidr string, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	rule := &api.IPExclusion{
		ID:          uuid.New(),
		Type:        "cidr",
		CIDR:        cidr,
		Description: strings.TrimSpace(description),
		CreatedAt:   time.Now().UTC(),
	}

	createdByArg := nullableUUID(createdBy)
	if createdByArg != nil {
		id := createdBy
		rule.CreatedBy = &id
	}

	if err := s.Exec(ctx,
		"INSERT INTO instance_exclusions (id, cidr, description, created_at, created_by) VALUES (?, ?, ?, ?, ?)",
		rule.ID,
		rule.CIDR,
		nullableString(rule.Description),
		rule.CreatedAt,
		createdByArg,
	); err != nil {
		return nil, fmt.Errorf("failed to create instance exclusion: %w", err)
	}

	return rule, nil
}

func (s *Store) CreateSiteExclusion(ctx context.Context, siteID uuid.UUID, cidr string, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	rule := &api.IPExclusion{
		ID:          uuid.New(),
		Type:        "cidr",
		CIDR:        cidr,
		Description: strings.TrimSpace(description),
		CreatedAt:   time.Now().UTC(),
	}
	rule.SiteID = &siteID

	createdByArg := nullableUUID(createdBy)
	if createdByArg != nil {
		id := createdBy
		rule.CreatedBy = &id
	}

	if err := s.Exec(ctx,
		"INSERT INTO site_exclusions (id, site_id, cidr, description, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)",
		rule.ID,
		siteID,
		rule.CIDR,
		nullableString(rule.Description),
		rule.CreatedAt,
		createdByArg,
	); err != nil {
		return nil, fmt.Errorf("failed to create site exclusion: %w", err)
	}

	return rule, nil
}

func (s *Store) CreateInstanceCountryExclusion(ctx context.Context, countryCode string, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	normalizedCountry := normalizeCountryCode(countryCode)
	rule := &api.IPExclusion{
		ID:          uuid.New(),
		Type:        "country",
		CountryCode: normalizedCountry,
		Description: strings.TrimSpace(description),
		CreatedAt:   time.Now().UTC(),
	}

	createdByArg := nullableUUID(createdBy)
	if createdByArg != nil {
		id := createdBy
		rule.CreatedBy = &id
	}

	if err := s.Exec(ctx,
		"INSERT INTO instance_country_exclusions (id, country_code, description, created_at, created_by) VALUES (?, ?, ?, ?, ?)",
		rule.ID,
		rule.CountryCode,
		nullableString(rule.Description),
		rule.CreatedAt,
		createdByArg,
	); err != nil {
		return nil, fmt.Errorf("failed to create instance country exclusion: %w", err)
	}

	return rule, nil
}

func (s *Store) CreateSiteCountryExclusion(ctx context.Context, siteID uuid.UUID, countryCode string, description string, createdBy uuid.UUID) (*api.IPExclusion, error) {
	normalizedCountry := normalizeCountryCode(countryCode)
	rule := &api.IPExclusion{
		ID:          uuid.New(),
		Type:        "country",
		CountryCode: normalizedCountry,
		Description: strings.TrimSpace(description),
		CreatedAt:   time.Now().UTC(),
	}
	rule.SiteID = &siteID

	createdByArg := nullableUUID(createdBy)
	if createdByArg != nil {
		id := createdBy
		rule.CreatedBy = &id
	}

	if err := s.Exec(ctx,
		"INSERT INTO site_country_exclusions (id, site_id, country_code, description, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)",
		rule.ID,
		siteID,
		rule.CountryCode,
		nullableString(rule.Description),
		rule.CreatedAt,
		createdByArg,
	); err != nil {
		return nil, fmt.Errorf("failed to create site country exclusion: %w", err)
	}

	return rule, nil
}

func (s *Store) DeleteInstanceExclusion(ctx context.Context, ruleID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM instance_exclusions WHERE id = ?", ruleID)
	if err != nil {
		return false, fmt.Errorf("failed to delete instance exclusion: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine deleted instance exclusion rows: %w", err)
	}
	if rowsAffected > 0 {
		return true, nil
	}

	result, err = s.db.ExecContext(ctx, "DELETE FROM instance_country_exclusions WHERE id = ?", ruleID)
	if err != nil {
		return false, fmt.Errorf("failed to delete instance country exclusion: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine deleted instance country exclusion rows: %w", err)
	}
	return rowsAffected > 0, nil
}

func (s *Store) DeleteSiteExclusion(ctx context.Context, siteID uuid.UUID, ruleID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM site_exclusions WHERE id = ? AND site_id = ?", ruleID, siteID)
	if err != nil {
		return false, fmt.Errorf("failed to delete site exclusion: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine deleted site exclusion rows: %w", err)
	}
	if rowsAffected > 0 {
		return true, nil
	}

	result, err = s.db.ExecContext(ctx, "DELETE FROM site_country_exclusions WHERE id = ? AND site_id = ?", ruleID, siteID)
	if err != nil {
		return false, fmt.Errorf("failed to delete site country exclusion: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine deleted site country exclusion rows: %w", err)
	}
	return rowsAffected > 0, nil
}

func (s *Store) ListInstanceExclusionCIDRs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT cidr FROM instance_exclusions")
	if err != nil {
		return nil, fmt.Errorf("failed to list instance exclusion cidrs: %w", err)
	}
	defer rows.Close()

	cidrs := make([]string, 0)
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return nil, fmt.Errorf("failed to scan instance exclusion cidr: %w", err)
		}
		cidrs = append(cidrs, strings.TrimSpace(cidr))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate instance exclusion cidrs: %w", err)
	}

	return cidrs, nil
}

func (s *Store) ListSiteExclusionCIDRs(ctx context.Context) ([]SiteExclusionCIDR, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT site_id, cidr FROM site_exclusions")
	if err != nil {
		return nil, fmt.Errorf("failed to list site exclusion cidrs: %w", err)
	}
	defer rows.Close()

	rules := make([]SiteExclusionCIDR, 0)
	for rows.Next() {
		var rule SiteExclusionCIDR
		if err := rows.Scan(&rule.SiteID, &rule.CIDR); err != nil {
			return nil, fmt.Errorf("failed to scan site exclusion cidr: %w", err)
		}
		rule.CIDR = strings.TrimSpace(rule.CIDR)
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site exclusion cidrs: %w", err)
	}

	return rules, nil
}

func (s *Store) ListInstanceExclusionCountries(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT country_code FROM instance_country_exclusions")
	if err != nil {
		return nil, fmt.Errorf("failed to list instance exclusion countries: %w", err)
	}
	defer rows.Close()

	countries := make([]string, 0)
	for rows.Next() {
		var countryCode string
		if err := rows.Scan(&countryCode); err != nil {
			return nil, fmt.Errorf("failed to scan instance exclusion country: %w", err)
		}
		countries = append(countries, normalizeCountryCode(countryCode))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate instance exclusion countries: %w", err)
	}

	return countries, nil
}

func (s *Store) ListSiteExclusionCountries(ctx context.Context) ([]SiteExclusionCountry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT site_id, country_code FROM site_country_exclusions")
	if err != nil {
		return nil, fmt.Errorf("failed to list site exclusion countries: %w", err)
	}
	defer rows.Close()

	rules := make([]SiteExclusionCountry, 0)
	for rows.Next() {
		var rule SiteExclusionCountry
		if err := rows.Scan(&rule.SiteID, &rule.CountryCode); err != nil {
			return nil, fmt.Errorf("failed to scan site exclusion country: %w", err)
		}
		rule.CountryCode = normalizeCountryCode(rule.CountryCode)
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site exclusion countries: %w", err)
	}

	return rules, nil
}

func scanInstanceExclusion(scanner interface{ Scan(dest ...any) error }) (api.IPExclusion, error) {
	var rule api.IPExclusion
	var cidr sql.NullString
	var countryCode sql.NullString
	var description sql.NullString
	var createdBy uuid.NullUUID

	if err := scanner.Scan(&rule.ID, &rule.Type, &cidr, &countryCode, &description, &rule.CreatedAt, &createdBy); err != nil {
		return api.IPExclusion{}, fmt.Errorf("failed to scan instance exclusion: %w", err)
	}

	rule.Type = strings.TrimSpace(rule.Type)
	rule.CIDR = strings.TrimSpace(cidr.String)
	rule.CountryCode = normalizeCountryCode(countryCode.String)
	rule.Description = strings.TrimSpace(description.String)
	if createdBy.Valid {
		id := createdBy.UUID
		rule.CreatedBy = &id
	}

	return rule, nil
}

func scanSiteExclusion(scanner interface{ Scan(dest ...any) error }, siteID uuid.UUID) (api.IPExclusion, error) {
	rule, err := scanInstanceExclusion(scanner)
	if err != nil {
		return api.IPExclusion{}, err
	}

	rule.SiteID = &siteID
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

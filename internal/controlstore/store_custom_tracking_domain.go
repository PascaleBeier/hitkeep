package controlstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	CustomTrackingVerificationPending  = string(api.CustomTrackingDomainStatusPending)
	CustomTrackingVerificationVerified = string(api.CustomTrackingDomainStatusVerified)
	CustomTrackingVerificationFailed   = string(api.CustomTrackingDomainStatusFailed)
)

var ErrCustomTrackingDomainNotFound = errors.New("custom tracking domain not found")

type CustomTrackingDomainInput struct {
	TeamID  uuid.UUID
	Host    string
	TLSMode string
}

type CustomTrackingDomainVerificationResult struct {
	VerificationStatus string
	TargetStatus       string
	TLSStatus          string
	LastError          string
	VerifiedAt         *time.Time
	LastCheckedAt      time.Time
}

func NewCustomTrackingDomainVerificationToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate custom tracking verification token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func CustomTrackingDNSTXTName(hostname string) string {
	hostname = NormalizeCustomTrackingHostname(hostname)
	if hostname == "" {
		return ""
	}
	return "_hitkeep-tracking." + hostname
}

func CustomTrackingDNSTXTValue(token string) string {
	return "hitkeep-domain-verification=" + strings.TrimSpace(token)
}

func NormalizeCustomTrackingHostname(hostname string) string {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	hostname = strings.TrimSuffix(hostname, ".")
	return hostname
}

func CustomTrackingDomainIsActive(domain api.CustomTrackingDomain) bool {
	return domain.Enabled &&
		domain.VerificationStatus == api.CustomTrackingDomainStatusVerified &&
		domain.TargetStatus == api.CustomTrackingDomainStatusVerified &&
		domain.TLSStatus == api.CustomTrackingDomainStatusVerified
}

func (s *Store) CreateCustomTrackingDomain(ctx context.Context, input CustomTrackingDomainInput) (*api.CustomTrackingDomain, error) {
	hostname := NormalizeCustomTrackingHostname(input.Host)
	if hostname == "" {
		return nil, fmt.Errorf("custom tracking hostname is required")
	}
	token, err := NewCustomTrackingDomainVerificationToken()
	if err != nil {
		return nil, err
	}
	tlsMode := normalizeCustomTrackingTLSMode(input.TLSMode)
	now := time.Now().UTC()
	id := uuid.New()

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO custom_tracking_domains (
			id, tenant_id, hostname, verification_token,
			verification_status, target_status, tls_mode, tls_status,
			enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE, ?, ?)
	`,
		id,
		input.TeamID,
		hostname,
		token,
		CustomTrackingVerificationPending,
		CustomTrackingVerificationPending,
		tlsMode,
		CustomTrackingVerificationPending,
		now,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("create custom tracking domain: %w", err)
	}
	s.invalidateCustomTrackingDomainHost(hostname)
	return s.GetCustomTrackingDomain(ctx, id)
}

func (s *Store) ListCustomTrackingDomains(ctx context.Context, teamID uuid.UUID) ([]api.CustomTrackingDomain, error) {
	rows, err := s.db.QueryContext(ctx, customTrackingDomainSelect()+`
		WHERE tenant_id = ?
		ORDER BY hostname ASC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list custom tracking domains: %w", err)
	}
	defer rows.Close()

	var domains []api.CustomTrackingDomain
	for rows.Next() {
		domain, err := scanCustomTrackingDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read custom tracking domains: %w", err)
	}
	return domains, nil
}

func (s *Store) GetCustomTrackingDomain(ctx context.Context, id uuid.UUID) (*api.CustomTrackingDomain, error) {
	row := s.db.QueryRowContext(ctx, customTrackingDomainSelect()+" WHERE id = ?", id)
	domain, err := scanCustomTrackingDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get custom tracking domain: %w", err)
	}
	return &domain, nil
}

func (s *Store) GetCustomTrackingDomainForTeam(ctx context.Context, teamID, id uuid.UUID) (*api.CustomTrackingDomain, error) {
	row := s.db.QueryRowContext(ctx, customTrackingDomainSelect()+" WHERE tenant_id = ? AND id = ?", teamID, id)
	domain, err := scanCustomTrackingDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get team custom tracking domain: %w", err)
	}
	return &domain, nil
}

// FindCustomTrackingDomainByHostname resolves a custom tracking domain by
// hostname. It runs on every ingest request that carries a custom tracking
// host, so results — including misses — are held in a short-TTL cache with
// singleflight collapsing concurrent lookups; mutations must call
// invalidateCustomTrackingDomainHost.
func (s *Store) FindCustomTrackingDomainByHostname(ctx context.Context, hostname string) (*api.CustomTrackingDomain, error) {
	hostname = NormalizeCustomTrackingHostname(hostname)
	if hostname == "" {
		return nil, nil
	}
	if domain, ok := s.getCachedCustomTrackingDomain(hostname); ok {
		return domain, nil
	}
	if s.runtime == nil {
		return s.queryCustomTrackingDomainByHostname(ctx, hostname)
	}

	result, err, _ := s.runtime.customTrackingSF.Do(hostname, func() (any, error) {
		domain, err := s.queryCustomTrackingDomainByHostname(ctx, hostname)
		if err != nil {
			return nil, err
		}
		s.cacheCustomTrackingDomain(hostname, domain)
		return domain, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneCustomTrackingDomain(result.(*api.CustomTrackingDomain)), nil
}

func (s *Store) queryCustomTrackingDomainByHostname(ctx context.Context, hostname string) (*api.CustomTrackingDomain, error) {
	row := s.db.QueryRowContext(ctx, customTrackingDomainSelect()+" WHERE hostname = ?", hostname)
	domain, err := scanCustomTrackingDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find custom tracking domain by hostname: %w", err)
	}
	return &domain, nil
}

func (s *Store) UpdateCustomTrackingDomainVerification(ctx context.Context, id uuid.UUID, result CustomTrackingDomainVerificationResult) (*api.CustomTrackingDomain, error) {
	verificationStatus := normalizeCustomTrackingStatus(result.VerificationStatus)
	targetStatus := normalizeCustomTrackingStatus(result.TargetStatus)
	tlsStatus := normalizeCustomTrackingStatus(result.TLSStatus)
	lastCheckedAt := result.LastCheckedAt.UTC()
	if lastCheckedAt.IsZero() {
		lastCheckedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE custom_tracking_domains
		SET verification_status = ?,
			target_status = ?,
			tls_status = ?,
			last_error = ?,
			verified_at = ?,
			last_checked_at = ?,
			updated_at = ?
		WHERE id = ?
	`,
		verificationStatus,
		targetStatus,
		tlsStatus,
		strings.TrimSpace(result.LastError),
		nullableCustomTrackingTime(result.VerifiedAt),
		lastCheckedAt,
		lastCheckedAt,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("update custom tracking domain verification: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrCustomTrackingDomainNotFound
	}
	updated, err := s.GetCustomTrackingDomain(ctx, id)
	if err == nil && updated != nil {
		s.invalidateCustomTrackingDomainHost(updated.Hostname)
	}
	return updated, err
}

func (s *Store) UpdateCustomTrackingDomainEnabled(ctx context.Context, teamID, id uuid.UUID, enabled bool) (*api.CustomTrackingDomain, error) {
	now := time.Now().UTC()
	if !enabled {
		existing, err := s.GetCustomTrackingDomainForTeam(ctx, teamID, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrCustomTrackingDomainNotFound
		}
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE custom_tracking_domains
		SET enabled = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ?
	`, enabled, now, teamID, id)
	if err != nil {
		return nil, fmt.Errorf("update custom tracking domain enabled: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrCustomTrackingDomainNotFound
	}
	updated, err := s.GetCustomTrackingDomain(ctx, id)
	if err == nil && updated != nil {
		s.invalidateCustomTrackingDomainHost(updated.Hostname)
	}
	return updated, err
}

func (s *Store) DeleteCustomTrackingDomain(ctx context.Context, teamID, id uuid.UUID) error {
	existing, err := s.GetCustomTrackingDomainForTeam(ctx, teamID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrCustomTrackingDomainNotFound
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM custom_tracking_domains WHERE tenant_id = ? AND id = ?", teamID, id)
	if err != nil {
		return fmt.Errorf("delete custom tracking domain: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrCustomTrackingDomainNotFound
	}
	s.invalidateCustomTrackingDomainHost(existing.Hostname)
	return nil
}

func (s *Store) RecordCustomTrackingDomainTLSAsk(ctx context.Context, hostname string, at time.Time) error {
	hostname = NormalizeCustomTrackingHostname(hostname)
	if hostname == "" {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if err := s.exec(ctx, `
		UPDATE custom_tracking_domains
		SET last_tls_ask_at = ?, updated_at = ?
		WHERE hostname = ?
	`, at.UTC(), at.UTC(), hostname); err != nil {
		return err
	}
	s.invalidateCustomTrackingDomainHost(hostname)
	return nil
}

func customTrackingDomainSelect() string {
	return `
		SELECT
			id,
			tenant_id,
			hostname,
			verification_token,
			verification_status,
			target_status,
			tls_mode,
			tls_status,
			enabled,
			last_error,
			verified_at,
			last_checked_at,
			last_tls_ask_at,
			created_at,
			updated_at
		FROM custom_tracking_domains
	`
}

type customTrackingDomainScanner interface {
	Scan(dest ...any) error
}

func scanCustomTrackingDomain(scanner customTrackingDomainScanner) (api.CustomTrackingDomain, error) {
	var (
		domain        api.CustomTrackingDomain
		token         string
		verification  string
		target        string
		tlsMode       string
		tlsStatus     string
		lastError     sql.NullString
		verifiedAt    sql.NullTime
		lastCheckedAt sql.NullTime
		lastTLSAskAt  sql.NullTime
	)
	if err := scanner.Scan(
		&domain.ID,
		&domain.TeamID,
		&domain.Hostname,
		&token,
		&verification,
		&target,
		&tlsMode,
		&tlsStatus,
		&domain.Enabled,
		&lastError,
		&verifiedAt,
		&lastCheckedAt,
		&lastTLSAskAt,
		&domain.CreatedAt,
		&domain.UpdatedAt,
	); err != nil {
		return api.CustomTrackingDomain{}, err
	}
	domain.VerificationStatus = api.CustomTrackingDomainStatus(normalizeCustomTrackingStatus(verification))
	domain.TargetStatus = api.CustomTrackingDomainStatus(normalizeCustomTrackingStatus(target))
	domain.TLSMode = api.CustomTrackingTLSMode(normalizeCustomTrackingTLSMode(tlsMode))
	domain.TLSStatus = api.CustomTrackingDomainStatus(normalizeCustomTrackingStatus(tlsStatus))
	domain.Active = CustomTrackingDomainIsActive(domain)
	domain.DNSTXTName = CustomTrackingDNSTXTName(domain.Hostname)
	domain.DNSTXTValue = CustomTrackingDNSTXTValue(token)
	domain.LastError = nullStringValue(lastError)
	domain.VerifiedAt = nullTimePtr(verifiedAt)
	domain.LastCheckedAt = nullTimePtr(lastCheckedAt)
	domain.LastTLSAskAt = nullTimePtr(lastTLSAskAt)
	return domain, nil
}

func normalizeCustomTrackingStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CustomTrackingVerificationVerified:
		return CustomTrackingVerificationVerified
	case CustomTrackingVerificationFailed:
		return CustomTrackingVerificationFailed
	default:
		return CustomTrackingVerificationPending
	}
}

func normalizeCustomTrackingTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(api.CustomTrackingTLSModeCaddyOnDemand):
		return string(api.CustomTrackingTLSModeCaddyOnDemand)
	default:
		return string(api.CustomTrackingTLSModeExternal)
	}
}

func nullableCustomTrackingTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTeamSSONotFound       = errors.New("team SSO configuration not found")
	ErrTeamSSODomainConflict = errors.New("SSO domain is already configured for another team")
)

type TeamSSOConfig struct {
	TeamID                uuid.UUID
	ProviderType          string
	IssuerURL             string
	ClientID              string
	ClientSecretEncrypted string
	AllowedDomains        []string
	EmailClaim            string
	DisplayNameClaim      string
	Enabled               bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ResolveSSOUserInput struct {
	TeamID       uuid.UUID
	IssuerURL    string
	Subject      string
	Email        string
	GivenName    string
	LastName     string
	PasswordHash string
}

type ResolveSSOUserResult struct {
	UserID  uuid.UUID
	Created bool
}

func (s *Store) GetTeamSSOConfig(ctx context.Context, teamID uuid.UUID) (*TeamSSOConfig, error) {
	config, err := scanTeamSSOConfig(s.db.QueryRowContext(ctx, `
		SELECT tenant_id, provider_type, issuer_url, client_id, client_secret_encrypted,
		       email_claim, display_name_claim, enabled, created_at, updated_at
		FROM team_sso_configs
		WHERE tenant_id = ?
	`, teamID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not query team SSO configuration: %w", err)
	}
	domains, err := s.listTeamSSODomains(ctx, teamID)
	if err != nil {
		return nil, err
	}
	config.AllowedDomains = domains
	return config, nil
}

func (s *Store) GetEnabledTeamSSOConfigByDomain(ctx context.Context, domain string) (*TeamSSOConfig, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, nil
	}
	config, err := scanTeamSSOConfig(s.db.QueryRowContext(ctx, `
		SELECT c.tenant_id, c.provider_type, c.issuer_url, c.client_id, c.client_secret_encrypted,
		       c.email_claim, c.display_name_claim, c.enabled, c.created_at, c.updated_at
		FROM team_sso_configs c
		JOIN team_sso_domains d ON d.tenant_id = c.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE d.domain = ? AND c.enabled = TRUE AND ta.tenant_id IS NULL
	`, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not resolve team SSO domain: %w", err)
	}
	domains, err := s.listTeamSSODomains(ctx, config.TeamID)
	if err != nil {
		return nil, err
	}
	config.AllowedDomains = domains
	return config, nil
}

func (s *Store) HasEnabledTeamSSO(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_sso_configs c
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE c.enabled = TRUE AND ta.tenant_id IS NULL
	`).Scan(&count); err != nil {
		return false, fmt.Errorf("could not query SSO availability: %w", err)
	}
	return count > 0, nil
}

// ListEnabledTeamSSOTeamIDs returns the active teams whose SSO configuration
// is enabled. Callers apply live entitlements separately so plan downgrades do
// not require mutating or discarding the saved identity-provider settings.
func (s *Store) ListEnabledTeamSSOTeamIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.tenant_id
		FROM team_sso_configs c
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE c.enabled = TRUE AND ta.tenant_id IS NULL
		ORDER BY c.tenant_id
	`)
	if err != nil {
		return nil, fmt.Errorf("could not query enabled SSO teams: %w", err)
	}
	defer rows.Close()

	teamIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var teamID uuid.UUID
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("could not scan enabled SSO team: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate enabled SSO teams: %w", err)
	}
	return teamIDs, nil
}

func (s *Store) UpsertTeamSSOConfig(ctx context.Context, config TeamSSOConfig) error {
	if config.TeamID == uuid.Nil {
		return errors.New("team ID is required")
	}
	if len(config.AllowedDomains) == 0 {
		return errors.New("at least one SSO domain is required")
	}
	now := time.Now().UTC()
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO team_sso_configs (
				tenant_id, provider_type, issuer_url, client_id, client_secret_encrypted,
				email_claim, display_name_claim, enabled, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id) DO UPDATE SET
				provider_type = excluded.provider_type,
				issuer_url = excluded.issuer_url,
				client_id = excluded.client_id,
				client_secret_encrypted = excluded.client_secret_encrypted,
				email_claim = excluded.email_claim,
				display_name_claim = excluded.display_name_claim,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at
		`, config.TeamID, config.ProviderType, config.IssuerURL, config.ClientID,
			config.ClientSecretEncrypted, config.EmailClaim, config.DisplayNameClaim,
			config.Enabled, now, now); err != nil {
			return fmt.Errorf("could not save team SSO configuration: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM team_sso_domains WHERE tenant_id = ?", config.TeamID); err != nil {
			return fmt.Errorf("could not replace team SSO domains: %w", err)
		}
		for _, domain := range config.AllowedDomains {
			if _, err := tx.ExecContext(ctx, "INSERT INTO team_sso_domains (tenant_id, domain, created_at) VALUES (?, ?, ?)", config.TeamID, domain, now); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
					return fmt.Errorf("%w: %s", ErrTeamSSODomainConflict, domain)
				}
				return fmt.Errorf("could not save team SSO domain: %w", err)
			}
		}
		return nil
	})
	return err
}

func (s *Store) DeleteTeamSSOConfig(ctx context.Context, teamID uuid.UUID) error {
	// DuckDB checks the child reference against the transaction's original
	// snapshot, so the child and parent deletes must be committed separately.
	domains, err := s.listTeamSSODomains(ctx, teamID)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM team_sso_domains WHERE tenant_id = ?", teamID); err != nil {
		return fmt.Errorf("could not delete team SSO domains: %w", err)
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM team_sso_configs WHERE tenant_id = ?", teamID)
	if err != nil {
		s.restoreTeamSSODomains(ctx, teamID, domains)
		return fmt.Errorf("could not delete team SSO configuration: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrTeamSSONotFound
	}
	return nil
}

func (s *Store) ResolveSSOUser(ctx context.Context, input ResolveSSOUserInput) (ResolveSSOUserResult, error) {
	input.IssuerURL = strings.TrimSpace(input.IssuerURL)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.GivenName = strings.TrimSpace(input.GivenName)
	input.LastName = strings.TrimSpace(input.LastName)
	if input.TeamID == uuid.Nil || input.IssuerURL == "" || input.Subject == "" || input.Email == "" || input.PasswordHash == "" {
		return ResolveSSOUserResult{}, errors.New("complete SSO identity input is required")
	}

	result := ResolveSSOUserResult{}
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		var linkedUserID uuid.UUID
		err := tx.QueryRowContext(ctx, `
			SELECT user_id
			FROM sso_identities
			WHERE tenant_id = ? AND issuer_url = ? AND subject = ?
		`, input.TeamID, input.IssuerURL, input.Subject).Scan(&linkedUserID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("could not query SSO identity: %w", err)
		}

		if linkedUserID != uuid.Nil {
			result.UserID = linkedUserID
			if _, err := tx.ExecContext(ctx, `
				UPDATE sso_identities
				SET email = ?, updated_at = ?
				WHERE tenant_id = ? AND issuer_url = ? AND subject = ?
			`, input.Email, time.Now().UTC(), input.TeamID, input.IssuerURL, input.Subject); err != nil {
				return fmt.Errorf("could not update SSO identity: %w", err)
			}
		} else {
			err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE lower(email) = lower(?)", input.Email).Scan(&result.UserID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("could not resolve SSO user by email: %w", err)
			}
			if errors.Is(err, sql.ErrNoRows) {
				result.UserID = uuid.New()
				result.Created = true
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO users (id, email, password, given_name, last_name, created_at)
					VALUES (?, ?, ?, ?, ?, ?)
				`, result.UserID, input.Email, input.PasswordHash, nullableProfileName(input.GivenName), nullableProfileName(input.LastName), time.Now().UTC()); err != nil {
					return fmt.Errorf("could not create SSO user: %w", err)
				}
			}

			if _, err := tx.ExecContext(ctx, `
				INSERT INTO sso_identities (id, tenant_id, user_id, issuer_url, subject, email, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, uuid.New(), input.TeamID, result.UserID, input.IssuerURL, input.Subject, input.Email, time.Now().UTC(), time.Now().UTC()); err != nil {
				return fmt.Errorf("could not link SSO identity: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenant_members (id, tenant_id, user_id, role, added_at, added_by)
			VALUES (?, ?, ?, ?, ?, NULL)
			ON CONFLICT (tenant_id, user_id) DO NOTHING
		`, uuid.New(), input.TeamID, result.UserID, TenantRoleMember, time.Now().UTC()); err != nil {
			return fmt.Errorf("could not grant SSO team membership: %w", err)
		}
		if input.GivenName != "" || input.LastName != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE users
				SET given_name = CASE WHEN COALESCE(TRIM(given_name), '') = '' THEN ? ELSE given_name END,
				    last_name = CASE WHEN COALESCE(TRIM(last_name), '') = '' THEN ? ELSE last_name END
				WHERE id = ?
			`, nullableProfileName(input.GivenName), nullableProfileName(input.LastName), result.UserID); err != nil {
				return fmt.Errorf("could not apply SSO display name: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET
				active_tenant_id = excluded.active_tenant_id,
				updated_at = excluded.updated_at
		`, result.UserID, defaultLocaleCode, time.Now().UTC(), input.TeamID); err != nil {
			return fmt.Errorf("could not activate SSO team: %w", err)
		}
		return nil
	})
	if err != nil {
		return ResolveSSOUserResult{}, err
	}
	s.invalidateAllSiteRolesForUser(result.UserID)
	return result, nil
}

func (s *Store) restoreTeamSSODomains(ctx context.Context, teamID uuid.UUID, domains []string) {
	for _, domain := range domains {
		_, _ = s.db.ExecContext(ctx, "INSERT INTO team_sso_domains (tenant_id, domain, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", teamID, domain, time.Now().UTC())
	}
}

func (s *Store) listTeamSSODomains(ctx context.Context, teamID uuid.UUID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT domain FROM team_sso_domains WHERE tenant_id = ? ORDER BY domain", teamID)
	if err != nil {
		return nil, fmt.Errorf("could not list team SSO domains: %w", err)
	}
	defer rows.Close()
	domains := make([]string, 0)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("could not scan team SSO domain: %w", err)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read team SSO domains: %w", err)
	}
	return domains, nil
}

func scanTeamSSOConfig(row *sql.Row) (*TeamSSOConfig, error) {
	var config TeamSSOConfig
	err := row.Scan(
		&config.TeamID,
		&config.ProviderType,
		&config.IssuerURL,
		&config.ClientID,
		&config.ClientSecretEncrypted,
		&config.EmailClaim,
		&config.DisplayNameClaim,
		&config.Enabled,
		&config.CreatedAt,
		&config.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

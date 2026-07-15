package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const pendingSocialConfirmationTTL = 30 * time.Minute

var (
	ErrSocialIdentityConflict      = errors.New("social identity belongs to another user")
	ErrSocialProviderAlreadyLinked = errors.New("another identity from this provider is already linked")
	ErrSocialIdentityNotFound      = errors.New("social identity not found")
	ErrLastPrimaryLoginMethod      = errors.New("login method is the last usable primary method")
	ErrSocialLastLoginMethod       = ErrLastPrimaryLoginMethod
	ErrSocialConfirmationInvalid   = errors.New("invalid or unknown social confirmation token")
	ErrSocialConfirmationExpired   = errors.New("social confirmation token has expired")
)

type SocialIdentity struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Provider      string
	Subject       string
	ObservedEmail string
	LinkedAt      time.Time
	UpdatedAt     time.Time
	LastUsedAt    *time.Time
}

type LinkSocialIdentityInput struct {
	UserID        uuid.UUID
	Provider      string
	Subject       string
	ObservedEmail string
	MarkUsed      bool
}

type PendingSocialConfirmation struct {
	Provider        string
	Subject         string
	ObservedEmail   string
	TargetEmail     string
	TargetUserID    *uuid.UUID
	TeamName        string
	Jurisdiction    string
	Locale          string
	PlanCode        string
	BillingInterval string
	AcceptedTosAt   time.Time
	ReturnPath      string
	RememberMe      bool
	ExpiresAt       time.Time
}

type CreateManagedSocialAccountInput struct {
	Email           string
	HashedPassword  string
	TeamName        string
	Locale          string
	Provider        string
	Subject         string
	ObservedEmail   string
	PlanCode        string
	PlanName        string
	BillingInterval string
}

type ManagedSocialAccount struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
}

func normalizeSocialProvider(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "google", "github", "microsoft":
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported social provider %q", provider)
	}
}

func (s *Store) GetSocialIdentity(ctx context.Context, provider, subject string) (*SocialIdentity, error) {
	provider, err := normalizeSocialProvider(provider)
	if err != nil {
		return nil, err
	}
	var identity SocialIdentity
	var lastUsed sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, subject, observed_email, linked_at, updated_at, last_used_at
		FROM social_identities
		WHERE provider = ? AND subject = ?
	`, provider, strings.TrimSpace(subject)).Scan(
		&identity.ID, &identity.UserID, &identity.Provider, &identity.Subject, &identity.ObservedEmail,
		&identity.LinkedAt, &identity.UpdatedAt, &lastUsed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load social identity: %w", err)
	}
	if lastUsed.Valid {
		last := lastUsed.Time.UTC()
		identity.LastUsedAt = &last
	}
	return &identity, nil
}

func (s *Store) GetUserBySocialIdentity(ctx context.Context, provider, subject string) (*api.User, *SocialIdentity, error) {
	identity, err := s.GetSocialIdentity(ctx, provider, subject)
	if err != nil || identity == nil {
		return nil, identity, err
	}
	user, err := s.GetUserByID(ctx, identity.UserID)
	return user, identity, err
}

func (s *Store) LinkSocialIdentity(ctx context.Context, input LinkSocialIdentityInput) (*SocialIdentity, error) {
	provider, err := normalizeSocialProvider(input.Provider)
	if err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(input.Subject)
	if input.UserID == uuid.Nil || subject == "" {
		return nil, errors.New("social identity user and subject are required")
	}
	observedEmail := strings.ToLower(strings.TrimSpace(input.ObservedEmail))
	now := time.Now().UTC()

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		var subjectUserID uuid.UUID
		err := tx.QueryRowContext(ctx,
			"SELECT user_id FROM social_identities WHERE provider = ? AND subject = ?",
			provider, subject,
		).Scan(&subjectUserID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check social identity subject: %w", err)
		}
		if err == nil && subjectUserID != input.UserID {
			return ErrSocialIdentityConflict
		}

		var linkedSubject string
		err = tx.QueryRowContext(ctx,
			"SELECT subject FROM social_identities WHERE user_id = ? AND provider = ?",
			input.UserID, provider,
		).Scan(&linkedSubject)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check linked social provider: %w", err)
		}
		if err == nil && linkedSubject != subject {
			return ErrSocialProviderAlreadyLinked
		}

		if subjectUserID == input.UserID {
			if input.MarkUsed {
				_, err = tx.ExecContext(ctx, `
					UPDATE social_identities
					SET observed_email = ?, updated_at = ?, last_used_at = ?
					WHERE provider = ? AND subject = ?
				`, observedEmail, now, now, provider, subject)
			} else {
				_, err = tx.ExecContext(ctx, `
					UPDATE social_identities
					SET observed_email = ?, updated_at = ?
					WHERE provider = ? AND subject = ?
				`, observedEmail, now, provider, subject)
			}
			if err != nil {
				return fmt.Errorf("update social identity: %w", err)
			}
			return nil
		}

		var lastUsed any
		if input.MarkUsed {
			lastUsed = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO social_identities (
				id, user_id, provider, subject, observed_email, linked_at, updated_at, last_used_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.New(), input.UserID, provider, subject, observedEmail, now, now, lastUsed); err != nil {
			return fmt.Errorf("insert social identity: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetSocialIdentity(ctx, provider, subject)
}

func (s *Store) ListUserSocialIdentities(ctx context.Context, userID uuid.UUID) ([]api.UserSocialIdentity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, observed_email, linked_at, last_used_at
		FROM social_identities
		WHERE user_id = ?
		ORDER BY CASE provider WHEN 'google' THEN 1 WHEN 'github' THEN 2 ELSE 3 END
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list social identities: %w", err)
	}
	defer rows.Close()

	identities := make([]api.UserSocialIdentity, 0)
	for rows.Next() {
		var identity api.UserSocialIdentity
		var lastUsed sql.NullTime
		if err := rows.Scan(&identity.Provider, &identity.ObservedEmail, &identity.LinkedAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("scan social identity: %w", err)
		}
		if lastUsed.Valid {
			last := lastUsed.Time.UTC()
			identity.LastUsedAt = &last
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read social identities: %w", err)
	}
	return identities, nil
}

func (s *Store) DeleteSocialIdentityWithGuard(ctx context.Context, userID uuid.UUID, provider string, passwordConfirmed bool) error {
	provider, err := normalizeSocialProvider(provider)
	if err != nil {
		return err
	}

	// Serialize primary-method removal so two simultaneous provider unlinks
	// cannot both observe the other provider and lock the user out.
	s.primaryAuthMu.Lock()
	defer s.primaryAuthMu.Unlock()

	return s.Transact(ctx, func(tx *sql.Tx) error {
		var linkedCount int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM social_identities WHERE user_id = ? AND provider = ?",
			userID, provider,
		).Scan(&linkedCount); err != nil {
			return fmt.Errorf("check social identity: %w", err)
		}
		if linkedCount == 0 {
			return ErrSocialIdentityNotFound
		}

		var otherSocialCount, passkeyCount int
		var passwordLoginEnabled bool
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM social_identities WHERE user_id = ? AND provider <> ?",
			userID, provider,
		).Scan(&otherSocialCount); err != nil {
			return fmt.Errorf("count alternative social identities: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_passkeys WHERE user_id = ?",
			userID,
		).Scan(&passkeyCount); err != nil {
			return fmt.Errorf("count alternative passkeys: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			"SELECT COALESCE(password_login_enabled, TRUE) FROM users WHERE id = ?",
			userID,
		).Scan(&passwordLoginEnabled); err != nil {
			return fmt.Errorf("load password login state: %w", err)
		}
		if otherSocialCount == 0 && passkeyCount == 0 && (!passwordLoginEnabled || !passwordConfirmed) {
			return ErrSocialLastLoginMethod
		}

		if _, err := tx.ExecContext(ctx,
			"DELETE FROM social_identities WHERE user_id = ? AND provider = ?",
			userID, provider,
		); err != nil {
			return fmt.Errorf("delete social identity: %w", err)
		}
		return nil
	})
}

func (s *Store) CountSoleSocialProviderUsers(ctx context.Context, provider string) (int, error) {
	provider, err := normalizeSocialProvider(provider)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM social_identities identity
		JOIN users u ON u.id = identity.user_id
		WHERE identity.provider = ?
		  AND COALESCE(u.password_login_enabled, TRUE) = FALSE
		  AND NOT EXISTS (SELECT 1 FROM user_passkeys p WHERE p.user_id = identity.user_id)
		  AND NOT EXISTS (
			SELECT 1 FROM social_identities other
			WHERE other.user_id = identity.user_id AND other.provider <> identity.provider
		  )
	`, provider).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sole social provider users: %w", err)
	}
	return count, nil
}

func (s *Store) CreateManagedSocialAccount(ctx context.Context, input CreateManagedSocialAccountInput) (*ManagedSocialAccount, error) {
	provider, err := normalizeSocialProvider(input.Provider)
	if err != nil {
		return nil, err
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	subject := strings.TrimSpace(input.Subject)
	teamName := strings.TrimSpace(input.TeamName)
	locale := strings.TrimSpace(input.Locale)
	if email == "" || subject == "" || teamName == "" || strings.TrimSpace(input.HashedPassword) == "" {
		return nil, errors.New("complete managed social account input is required")
	}
	if locale == "" {
		locale = defaultLocaleCode
	}
	billingInterval := strings.ToLower(strings.TrimSpace(input.BillingInterval))
	if billingInterval != "annual" {
		billingInterval = "monthly"
	}
	result := &ManagedSocialAccount{UserID: uuid.New(), TenantID: uuid.New()}
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		var duplicateCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE lower(email) = lower(?)", email).Scan(&duplicateCount); err != nil {
			return fmt.Errorf("check managed social account email: %w", err)
		}
		if duplicateCount > 0 {
			return ErrUserEmailAlreadyExists
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM social_identities WHERE provider = ? AND subject = ?", provider, subject).Scan(&duplicateCount); err != nil {
			return fmt.Errorf("check managed social identity: %w", err)
		}
		if duplicateCount > 0 {
			return ErrSocialIdentityConflict
		}
		if err := ensureDefaultTenantTx(ctx, tx, defaultTenantName, false); err != nil {
			return err
		}

		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, email, password, password_login_enabled, created_at)
			VALUES (?, ?, ?, FALSE, ?)
		`, result.UserID, email, input.HashedPassword, now); err != nil {
			return fmt.Errorf("create managed social user: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)", result.TenantID, teamName, now); err != nil {
			return fmt.Errorf("create managed social team: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenant_members (tenant_id, user_id, role, added_by, added_at)
			VALUES (?, ?, ?, ?, ?)
		`, result.TenantID, result.UserID, TenantRoleOwner, result.UserID, now); err != nil {
			return fmt.Errorf("create managed social team owner: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
			VALUES (?, ?, ?, ?)
		`, result.UserID, locale, now, result.TenantID); err != nil {
			return fmt.Errorf("initialize managed social preferences: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO social_identities (
				id, user_id, provider, subject, observed_email, linked_at, updated_at, last_used_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.New(), result.UserID, provider, subject, strings.ToLower(strings.TrimSpace(input.ObservedEmail)), now, now, now); err != nil {
			return fmt.Errorf("link managed social identity: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cloud_billing_accounts (
				tenant_id, plan_code, plan_name, billing_interval, subscription_status, created_at, updated_at
			) VALUES (?, ?, ?, ?, 'free', ?, ?)
		`, result.TenantID, "free", "Free", billingInterval, now, now); err != nil {
			return fmt.Errorf("initialize managed social billing: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) SetPasswordLoginEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error {
	return s.Exec(ctx, "UPDATE users SET password_login_enabled = ? WHERE id = ?", enabled, userID)
}

func (s *Store) CreatePendingSocialConfirmation(ctx context.Context, entry PendingSocialConfirmation) (string, error) {
	provider, err := normalizeSocialProvider(entry.Provider)
	if err != nil {
		return "", err
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", err
	}
	token := hex.EncodeToString(rawToken)
	tokenHash := socialConfirmationTokenHash(token)
	now := time.Now().UTC()
	entry.ExpiresAt = now.Add(pendingSocialConfirmationTTL)

	var targetUserID any
	if entry.TargetUserID != nil && *entry.TargetUserID != uuid.Nil {
		targetUserID = *entry.TargetUserID
	}
	var acceptedTOS any
	if !entry.AcceptedTosAt.IsZero() {
		acceptedTOS = entry.AcceptedTosAt.UTC()
	}
	if strings.TrimSpace(entry.ReturnPath) == "" {
		entry.ReturnPath = "/dashboard"
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM pending_social_confirmations WHERE expires_at <= ? OR (provider = ? AND subject = ?)", now, provider, strings.TrimSpace(entry.Subject)); err != nil {
			return fmt.Errorf("clear pending social confirmations: %w", err)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO pending_social_confirmations (
				id, token_hash, provider, subject, observed_email, target_email, target_user_id,
				team_name, jurisdiction, locale, plan_code, billing_interval, accepted_tos_at,
				return_path, remember_me, expires_at, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.New(), tokenHash, provider, strings.TrimSpace(entry.Subject), strings.ToLower(strings.TrimSpace(entry.ObservedEmail)),
			strings.ToLower(strings.TrimSpace(entry.TargetEmail)), targetUserID, strings.TrimSpace(entry.TeamName),
			strings.ToUpper(strings.TrimSpace(entry.Jurisdiction)), strings.TrimSpace(entry.Locale), strings.ToLower(strings.TrimSpace(entry.PlanCode)),
			strings.ToLower(strings.TrimSpace(entry.BillingInterval)), acceptedTOS, strings.TrimSpace(entry.ReturnPath), entry.RememberMe,
			entry.ExpiresAt, now)
		if err != nil {
			return fmt.Errorf("insert pending social confirmation: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) ConsumePendingSocialConfirmation(ctx context.Context, token string) (*PendingSocialConfirmation, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrSocialConfirmationInvalid
	}
	tokenHash := socialConfirmationTokenHash(token)
	var entry PendingSocialConfirmation
	var targetUserID sql.NullString
	var acceptedTOS sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		DELETE FROM pending_social_confirmations
		WHERE token_hash = ?
		RETURNING provider, subject, observed_email, target_email, CAST(target_user_id AS VARCHAR),
		          team_name, jurisdiction, locale, plan_code, billing_interval, accepted_tos_at,
		          return_path, remember_me, expires_at
	`, tokenHash).Scan(
		&entry.Provider, &entry.Subject, &entry.ObservedEmail, &entry.TargetEmail, &targetUserID,
		&entry.TeamName, &entry.Jurisdiction, &entry.Locale, &entry.PlanCode, &entry.BillingInterval,
		&acceptedTOS, &entry.ReturnPath, &entry.RememberMe, &entry.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSocialConfirmationInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("load pending social confirmation: %w", err)
	}
	if targetUserID.Valid && strings.TrimSpace(targetUserID.String) != "" {
		id, err := uuid.Parse(targetUserID.String)
		if err != nil {
			return nil, fmt.Errorf("parse pending social confirmation user: %w", err)
		}
		entry.TargetUserID = &id
	}
	if acceptedTOS.Valid {
		entry.AcceptedTosAt = acceptedTOS.Time.UTC()
	}
	if time.Now().UTC().After(entry.ExpiresAt.UTC()) {
		return nil, ErrSocialConfirmationExpired
	}
	return &entry, nil
}

func socialConfirmationTokenHash(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

var (
	ErrUserEmailAlreadyExists = errors.New("user email already exists")
	ErrUserNotFound           = errors.New("user not found")
	ErrUserOwnsTeams          = errors.New("user owns teams")
)

type UserOwnsTeamsError struct {
	Teams []api.Team
}

func (e *UserOwnsTeamsError) Error() string {
	if len(e.Teams) == 0 {
		return ErrUserOwnsTeams.Error()
	}
	return fmt.Sprintf("%s: %d blocking team(s)", ErrUserOwnsTeams.Error(), len(e.Teams))
}

func (e *UserOwnsTeamsError) Unwrap() error {
	return ErrUserOwnsTeams
}

// GetUserCount returns the total number of users.
func (s *Store) GetUserCount(ctx context.Context) (int, error) {
	var count int
	// Helper handles ErrNoRows (though COUNT always returns a row)
	err := s.queryRowOrNil(ctx, "SELECT COUNT(*) FROM users", []any{&count})
	if err != nil {
		return 0, fmt.Errorf("could not query user count: %w", err)
	}
	return count, nil
}

// GetUserByEmail finds a user by email address. Returns nil if not found.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*api.User, error) {
	var user api.User
	err := s.queryRowOrNil(ctx,
		"SELECT id, email, password, COALESCE(given_name, ''), COALESCE(last_name, ''), COALESCE(password_login_enabled, TRUE), created_at FROM users WHERE lower(email) = lower(?)",
		[]any{&user.ID, &user.Email, &user.Password, &user.GivenName, &user.LastName, &user.PasswordLoginEnabled, &user.CreatedAt},
		email,
	)

	if err != nil {
		return nil, fmt.Errorf("could not query user: %w", err)
	}
	// QueryRowOrNil returns nil error on no rows, so check if ID was populated
	if user.ID == uuid.Nil {
		return nil, nil
	}
	return &user, nil
}

// GetUserByID finds a user by UUID. Returns nil if not found.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*api.User, error) {
	var user api.User
	err := s.queryRowOrNil(ctx,
		"SELECT id, email, password, COALESCE(given_name, ''), COALESCE(last_name, ''), COALESCE(password_login_enabled, TRUE), created_at FROM users WHERE id = ?",
		[]any{&user.ID, &user.Email, &user.Password, &user.GivenName, &user.LastName, &user.PasswordLoginEnabled, &user.CreatedAt},
		id,
	)

	if err != nil {
		return nil, fmt.Errorf("could not query user by id: %w", err)
	}
	if user.ID == uuid.Nil {
		return nil, nil
	}
	return &user, nil
}

// ListUsers returns all users ordered by creation date.
func (s *Store) ListUsers(ctx context.Context) ([]api.User, error) {
	var users []api.User

	err := s.queryList(ctx,
		`SELECT u.id, u.email, COALESCE(u.given_name, ''), COALESCE(u.last_name, ''), COALESCE(ir.role, 'user') AS instance_role, u.created_at
		 FROM users u
		 LEFT JOIN instance_roles ir ON ir.user_id = u.id
		 ORDER BY u.created_at DESC`,
		func(rows *sql.Rows) error {
			var u api.User
			// Note: password is not selected for listing
			if err := rows.Scan(&u.ID, &u.Email, &u.GivenName, &u.LastName, &u.InstanceRole, &u.CreatedAt); err != nil {
				return fmt.Errorf("could not scan user: %w", err)
			}
			users = append(users, u)
			return nil
		},
	)

	if err != nil {
		return nil, fmt.Errorf("could not list users: %w", err)
	}
	return users, nil
}

// CreateUser creates a new user and assigns the 'owner' role if they are the first user.
// This uses a transaction, so we do not use the helpers here.
func (s *Store) CreateUser(ctx context.Context, email string, hashedPassword string) (uuid.UUID, error) {
	return s.CreateUserWithNames(ctx, email, hashedPassword, "", "")
}

func (s *Store) CreateUserWithoutDefaultTenant(ctx context.Context, email string, hashedPassword string) (uuid.UUID, error) {
	return s.createUserWithoutDefaultTenant(ctx, email, hashedPassword, true)
}

// CreatePlaceholderUserWithoutDefaultTenant creates an invited account whose
// generated hash is intentionally not usable for password login. Completing a
// password setup or reset enables password login later.
func (s *Store) CreatePlaceholderUserWithoutDefaultTenant(ctx context.Context, email string, hashedPassword string) (uuid.UUID, error) {
	return s.createUserWithoutDefaultTenant(ctx, email, hashedPassword, false)
}

func (s *Store) createUserWithoutDefaultTenant(ctx context.Context, email string, hashedPassword string, passwordLoginEnabled bool) (uuid.UUID, error) {
	id := uuid.New()

	if err := s.exec(ctx,
		"INSERT INTO users (id, email, password, password_login_enabled, created_at) VALUES (?, ?, ?, ?, ?)",
		id, email, hashedPassword, passwordLoginEnabled, time.Now().UTC(),
	); err != nil {
		return uuid.Nil, fmt.Errorf("could not create user: %w", err)
	}

	return id, nil
}

// CreateUserWithNames creates a new user and optionally persists profile names.
func (s *Store) CreateUserWithNames(ctx context.Context, email string, hashedPassword string, givenName string, lastName string) (uuid.UUID, error) {
	return s.CreateUserWithNamesAndDefaultTenantName(ctx, email, hashedPassword, givenName, lastName, defaultTenantNameForSetup(givenName))
}

// CreateUserWithNamesAndDefaultTenantName creates a user and uses defaultTenantName when the first-user setup creates the default team.
func (s *Store) CreateUserWithNamesAndDefaultTenantName(ctx context.Context, email string, hashedPassword string, givenName string, lastName string, defaultTenantName string) (uuid.UUID, error) {
	return s.createUserWithNamesAndDefaultTenantName(ctx, email, hashedPassword, givenName, lastName, defaultTenantName, true)
}

// CreatePlaceholderUser creates an invited self-hosted account with an
// inaccessible generated password until the invitee explicitly sets one.
func (s *Store) CreatePlaceholderUser(ctx context.Context, email string, hashedPassword string) (uuid.UUID, error) {
	return s.createUserWithNamesAndDefaultTenantName(ctx, email, hashedPassword, "", "", defaultTenantNameForSetup(""), false)
}

func (s *Store) createUserWithNamesAndDefaultTenantName(ctx context.Context, email string, hashedPassword string, givenName string, lastName string, defaultTenantName string, passwordLoginEnabled bool) (uuid.UUID, error) {
	id := uuid.New()
	givenName = strings.TrimSpace(givenName)
	lastName = strings.TrimSpace(lastName)
	defaultTenantName = strings.TrimSpace(defaultTenantName)
	if defaultTenantName == "" {
		defaultTenantName = defaultTenantNameForSetup(givenName)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		"INSERT INTO users (id, email, password, given_name, last_name, password_login_enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, email, hashedPassword, nullableProfileName(givenName), nullableProfileName(lastName), passwordLoginEnabled, time.Now().UTC(),
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not create user: %w", err)
	}

	// Check if this is the first user
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return uuid.Nil, fmt.Errorf("could not count users: %w", err)
	}

	if err := ensureDefaultTenantTx(ctx, tx, defaultTenantName, count == 1); err != nil {
		return uuid.Nil, err
	}

	tenantRole := TenantRoleMember

	// If first user, make them instance owner
	if count == 1 {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO instance_roles (user_id, role) VALUES (?, 'owner')",
			id,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("could not assign owner role: %w", err)
		}

		tenantRole = TenantRoleOwner
	}

	defaultTenantID, err := getDefaultTenantID(ctx, tx)
	if err != nil {
		return uuid.Nil, err
	}

	if err := ensureTenantMemberTx(ctx, tx, defaultTenantID, id, tenantRole, id); err != nil {
		return uuid.Nil, err
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return id, nil
}

func nullableProfileName(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) UpdateUserProfile(ctx context.Context, userID uuid.UUID, email string, givenName string, lastName string) error {
	email = strings.TrimSpace(email)
	givenName = strings.TrimSpace(givenName)
	lastName = strings.TrimSpace(lastName)

	var duplicateCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE lower(email) = lower(?) AND id <> ?", email, userID).Scan(&duplicateCount); err != nil {
		return fmt.Errorf("could not check duplicate email: %w", err)
	}
	if duplicateCount > 0 {
		return ErrUserEmailAlreadyExists
	}

	err := s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET email = ?, given_name = ?, last_name = ? WHERE id = ?",
			email, nullableProfileName(givenName), nullableProfileName(lastName), userID,
		)
		if err != nil {
			return fmt.Errorf("could not update user profile: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("could not inspect updated user profile: %w", err)
		}
		if affected == 0 {
			return ErrUserNotFound
		}
		return nil
	})
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint failed") &&
			(strings.Contains(message, "users.email") || strings.Contains(message, "users_email_nocase_idx")) {
			return ErrUserEmailAlreadyExists
		}
		return err
	}
	return nil
}

// DeleteUser removes a user by ID.
func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	blockingTeams, err := s.ListSoleOwnerTeams(ctx, id)
	if err != nil {
		return fmt.Errorf("could not verify team ownership before deleting user: %w", err)
	}
	if len(blockingTeams) > 0 {
		return &UserOwnsTeamsError{Teams: blockingTeams}
	}

	siteIDs, err := s.ListUserSiteIDs(ctx, id)
	if err != nil {
		return err
	}

	for _, siteID := range siteIDs {
		if err := s.DeleteSite(ctx, siteID); err != nil {
			return fmt.Errorf("could not delete site %s: %w", siteID, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := cleanupUserRows(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id); err != nil {
		return fmt.Errorf("could not delete user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}

func (s *Store) ListUserSiteIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM sites WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("could not list user sites: %w", err)
	}
	defer rows.Close()

	var siteIDs []uuid.UUID
	for rows.Next() {
		var siteID uuid.UUID
		if err := rows.Scan(&siteID); err != nil {
			return nil, fmt.Errorf("could not scan site id: %w", err)
		}
		siteIDs = append(siteIDs, siteID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read site ids: %w", err)
	}
	return siteIDs, nil
}

func cleanupUserRows(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	tables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}
	execIfTableExists := func(table string, query string, args ...any) error {
		if _, ok := tables[table]; !ok {
			return nil
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		return nil
	}

	if err := execIfTableExists("share_links", "UPDATE share_links SET created_by = NULL WHERE created_by = ?", userID); err != nil {
		return fmt.Errorf("could not null share link owner: %w", err)
	}
	if err := execIfTableExists("traffic_exclusions", "UPDATE traffic_exclusions SET created_by = NULL WHERE created_by = ?", userID); err != nil {
		return fmt.Errorf("could not null traffic exclusion created_by: %w", err)
	}
	if err := execIfTableExists("team_invites", "UPDATE team_invites SET created_by = NULL WHERE created_by = ?", userID); err != nil {
		return fmt.Errorf("could not null team invite created_by: %w", err)
	}
	if err := execIfTableExists("team_invites", "UPDATE team_invites SET invited_user_id = NULL WHERE invited_user_id = ?", userID); err != nil {
		return fmt.Errorf("could not null team invite invited_user_id: %w", err)
	}
	if err := execIfTableExists("instance_audit_log", "UPDATE instance_audit_log SET actor_id = NULL WHERE actor_id = ?", userID); err != nil {
		return fmt.Errorf("could not null audit actor_id: %w", err)
	}
	if err := execIfTableExists("instance_audit_log", "UPDATE instance_audit_log SET target_user_id = NULL WHERE target_user_id = ?", userID); err != nil {
		return fmt.Errorf("could not null audit target_user_id: %w", err)
	}
	if err := execIfTableExists("report_deliveries", "DELETE FROM report_deliveries WHERE recipient_id IN (SELECT id FROM report_recipients WHERE user_id = ?) OR report_id IN (SELECT id FROM report_definitions WHERE owner_user_id = ?)", userID, userID); err != nil {
		return fmt.Errorf("could not delete user report deliveries: %w", err)
	}
	if err := execIfTableExists("report_runs", "DELETE FROM report_runs WHERE report_id IN (SELECT id FROM report_definitions WHERE owner_user_id = ?)", userID); err != nil {
		return fmt.Errorf("could not delete user report runs: %w", err)
	}
	if err := execIfTableExists("report_recipients", "DELETE FROM report_recipients WHERE user_id = ? OR report_id IN (SELECT id FROM report_definitions WHERE owner_user_id = ?)", userID, userID); err != nil {
		return fmt.Errorf("could not delete user report recipients: %w", err)
	}
	if err := execIfTableExists("report_definition_sites", "DELETE FROM report_definition_sites WHERE report_id IN (SELECT id FROM report_definitions WHERE owner_user_id = ?)", userID); err != nil {
		return fmt.Errorf("could not delete user report sites: %w", err)
	}
	if err := execIfTableExists("report_definitions", "UPDATE report_definitions SET created_by = NULL WHERE created_by = ?", userID); err != nil {
		return fmt.Errorf("could not null report creator: %w", err)
	}
	if err := execIfTableExists("report_definitions", "DELETE FROM report_definitions WHERE owner_user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete personal reports: %w", err)
	}
	if err := execIfTableExists("site_members", "DELETE FROM site_members WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user site memberships: %w", err)
	}
	if err := execIfTableExists("tenant_members", "DELETE FROM tenant_members WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user tenant memberships: %w", err)
	}
	if err := execIfTableExists("instance_roles", "DELETE FROM instance_roles WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete instance role: %w", err)
	}
	if err := execIfTableExists("site_members", "UPDATE site_members SET added_by = NULL WHERE added_by = ?", userID); err != nil {
		return fmt.Errorf("could not null site member added_by: %w", err)
	}
	if err := execIfTableExists("tenant_members", "UPDATE tenant_members SET added_by = NULL WHERE added_by = ?", userID); err != nil {
		return fmt.Errorf("could not null tenant member added_by: %w", err)
	}
	if err := execIfTableExists("instance_roles", "UPDATE instance_roles SET granted_by = NULL WHERE granted_by = ?", userID); err != nil {
		return fmt.Errorf("could not null instance role granted_by: %w", err)
	}
	if err := execIfTableExists("api_client_site_roles", "DELETE FROM api_client_site_roles WHERE api_client_id IN (SELECT id FROM api_clients WHERE user_id = ?)", userID); err != nil {
		return fmt.Errorf("could not delete user api client site roles: %w", err)
	}
	if err := execIfTableExists("api_clients", "DELETE FROM api_clients WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user api clients: %w", err)
	}
	if err := execIfTableExists("remember_me_tokens", "DELETE FROM remember_me_tokens WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete remember tokens: %w", err)
	}
	if err := execIfTableExists("social_identities", "DELETE FROM social_identities WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user social identities: %w", err)
	}
	if err := execIfTableExists("pending_social_confirmations", "DELETE FROM pending_social_confirmations WHERE target_user_id = ? OR lower(target_email) = lower((SELECT email FROM users WHERE id = ?))", userID, userID); err != nil {
		return fmt.Errorf("could not delete pending social confirmations: %w", err)
	}
	if err := execIfTableExists("user_passkey_challenges", "DELETE FROM user_passkey_challenges WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete pending user passkey challenges: %w", err)
	}
	if err := execIfTableExists("passkey_login_challenges", "DELETE FROM passkey_login_challenges WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete passkey login challenges: %w", err)
	}
	if err := execIfTableExists("user_passkeys", "DELETE FROM user_passkeys WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user passkeys: %w", err)
	}
	if err := execIfTableExists("user_totp_pending_setup", "DELETE FROM user_totp_pending_setup WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete pending user totp setup: %w", err)
	}
	if err := execIfTableExists("user_totp_factors", "DELETE FROM user_totp_factors WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user totp factors: %w", err)
	}
	if err := execIfTableExists("user_recovery_codes", "DELETE FROM user_recovery_codes WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user recovery codes: %w", err)
	}
	if err := execIfTableExists("user_preferences", "DELETE FROM user_preferences WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not delete user preferences: %w", err)
	}
	return nil
}

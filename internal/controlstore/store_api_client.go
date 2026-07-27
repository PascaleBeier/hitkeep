package controlstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

var (
	ErrAPIClientNotFound = errors.New("api client not found")
	ErrAPIClientInactive = errors.New("api client is revoked or expired")
)

const (
	APIClientOwnerPersonal = "personal"
	APIClientOwnerTeam     = "team"
)

type APIClientAuth struct {
	ClientID     uuid.UUID
	UserID       uuid.UUID
	TenantID     uuid.UUID
	InstanceRole auth.InstanceRole
	SiteRoles    map[uuid.UUID]auth.SiteRole
}

func (s *Store) CreateAPIClient(
	ctx context.Context,
	userID uuid.UUID,
	name string,
	description string,
	instanceRole auth.InstanceRole,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
) (*api.APIClient, string, error) {
	return s.createAPIClient(ctx, &userID, nil, name, description, instanceRole, siteRoles, expiresAt)
}

func (s *Store) CreateTeamAPIClient(
	ctx context.Context,
	tenantID uuid.UUID,
	name string,
	description string,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
) (*api.APIClient, string, error) {
	return s.createAPIClient(ctx, nil, &tenantID, name, description, auth.InstanceUser, siteRoles, expiresAt)
}

func (s *Store) createAPIClient(
	ctx context.Context,
	userID *uuid.UUID,
	tenantID *uuid.UUID,
	name string,
	description string,
	instanceRole auth.InstanceRole,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
) (*api.APIClient, string, error) {
	id := uuid.New()
	now := time.Now().UTC()
	token, tokenHash, err := generateAPIClientToken()
	if err != nil {
		return nil, "", err
	}

	description = strings.TrimSpace(description)
	exp := toUTCPtr(expiresAt)

	err = s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO api_clients (
				id, user_id, tenant_id, name, description, secret_hash, instance_role, expires_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, nullableUUIDPtr(userID), nullableUUIDPtr(tenantID), name, description, tokenHash, instanceRole, exp, now, now)
		if err != nil {
			return fmt.Errorf("failed to create api client: %w", err)
		}

		if err := replaceAPIClientSiteRolesTx(ctx, tx, id, siteRoles, now); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, "", err
	}

	client := buildAPIClient(id, userID, tenantID, name, description, instanceRole, exp, now, now, flattenSiteRoles(siteRoles))
	return &client, token, nil
}

func (s *Store) ListAPIClients(ctx context.Context, userID uuid.UUID) ([]api.APIClient, error) {
	return s.listAPIClients(ctx, "c.user_id = ?", userID)
}

func (s *Store) ListTeamAPIClients(ctx context.Context, tenantID uuid.UUID) ([]api.APIClient, error) {
	return s.listAPIClients(ctx, "c.tenant_id = ? AND ta.tenant_id IS NULL", tenantID)
}

func (s *Store) listAPIClients(ctx context.Context, where string, args ...any) ([]api.APIClient, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			c.id,
			CAST(c.user_id AS VARCHAR),
			CAST(c.tenant_id AS VARCHAR),
			c.name,
			c.description,
			c.instance_role,
			c.expires_at,
			c.last_used_at,
			c.revoked_at,
			c.created_at,
			c.updated_at
		FROM api_clients c
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE %s
		ORDER BY c.created_at DESC
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list api clients: %w", err)
	}
	defer rows.Close()

	clients := make([]api.APIClient, 0)
	indexByID := make(map[uuid.UUID]int)
	for rows.Next() {
		client, err := scanAPIClient(rows)
		if err != nil {
			return nil, err
		}
		indexByID[client.ID] = len(clients)
		clients = append(clients, client)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api clients: %w", err)
	}

	if len(clients) == 0 {
		return clients, nil
	}

	roleRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT r.api_client_id, r.site_id, r.role
		FROM api_client_site_roles r
		JOIN api_clients c ON c.id = r.api_client_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE %s
		ORDER BY r.api_client_id, r.site_id
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list api client site roles: %w", err)
	}
	defer roleRows.Close()

	for roleRows.Next() {
		var clientID uuid.UUID
		var siteID uuid.UUID
		var role string
		if err := roleRows.Scan(&clientID, &siteID, &role); err != nil {
			return nil, fmt.Errorf("failed to scan api client site role: %w", err)
		}
		idx, ok := indexByID[clientID]
		if !ok {
			continue
		}
		clients[idx].SiteRoles = append(clients[idx].SiteRoles, api.APIClientSiteRole{
			SiteID: siteID,
			Role:   role,
		})
	}
	if err := roleRows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api client site roles: %w", err)
	}

	return clients, nil
}

func (s *Store) GetAPIClient(ctx context.Context, userID uuid.UUID, clientID uuid.UUID) (*api.APIClient, error) {
	return s.getAPIClient(ctx, clientID, "c.user_id = ?", userID)
}

func (s *Store) GetTeamAPIClient(ctx context.Context, tenantID, clientID uuid.UUID) (*api.APIClient, error) {
	return s.getAPIClient(ctx, clientID, "c.tenant_id = ? AND ta.tenant_id IS NULL", tenantID)
}

func (s *Store) getAPIClient(ctx context.Context, clientID uuid.UUID, where string, args ...any) (*api.APIClient, error) {
	queryArgs := append([]any{clientID}, args...)
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT
			c.id,
			CAST(c.user_id AS VARCHAR),
			CAST(c.tenant_id AS VARCHAR),
			c.name,
			c.description,
			c.instance_role,
			c.expires_at,
			c.last_used_at,
			c.revoked_at,
			c.created_at,
			c.updated_at
		FROM api_clients c
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE c.id = ? AND %s
	`, where), queryArgs...)

	client, err := scanAPIClientRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT site_id, role
		FROM api_client_site_roles
		WHERE api_client_id = ?
		ORDER BY site_id
	`, client.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get api client site roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var siteID uuid.UUID
		var role string
		if err := rows.Scan(&siteID, &role); err != nil {
			return nil, fmt.Errorf("failed to scan api client site role: %w", err)
		}
		client.SiteRoles = append(client.SiteRoles, api.APIClientSiteRole{
			SiteID: siteID,
			Role:   role,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api client site roles: %w", err)
	}

	return client, nil
}

func (s *Store) UpdateAPIClient(
	ctx context.Context,
	userID uuid.UUID,
	clientID uuid.UUID,
	name string,
	description string,
	instanceRole auth.InstanceRole,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
	revoked bool,
) (*api.APIClient, error) {
	return s.updateAPIClient(ctx, clientID, "user_id = ?", []any{userID}, name, description, instanceRole, siteRoles, expiresAt, revoked, false)
}

func (s *Store) UpdateTeamAPIClient(
	ctx context.Context,
	tenantID uuid.UUID,
	clientID uuid.UUID,
	name string,
	description string,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
	revoked bool,
) (*api.APIClient, error) {
	return s.updateAPIClient(ctx, clientID, "tenant_id = ?", []any{tenantID}, name, description, auth.InstanceUser, siteRoles, expiresAt, revoked, true)
}

func (s *Store) RotateAPIClient(ctx context.Context, userID uuid.UUID, clientID uuid.UUID) (*api.APIClient, string, error) {
	return s.rotateAPIClient(ctx, clientID, "user_id = ?", []any{userID}, false)
}

func (s *Store) RotateTeamAPIClient(ctx context.Context, tenantID, clientID uuid.UUID) (*api.APIClient, string, error) {
	return s.rotateAPIClient(ctx, clientID, "tenant_id = ?", []any{tenantID}, true)
}

func (s *Store) rotateAPIClient(ctx context.Context, clientID uuid.UUID, ownerWhere string, ownerArgs []any, teamOwned bool) (*api.APIClient, string, error) {
	token, tokenHash, err := generateAPIClientToken()
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	if err := s.ensureAPIClientRotatable(ctx, clientID, ownerWhere, ownerArgs, now); err != nil {
		return nil, "", err
	}

	if err := s.replaceAPIClientToken(ctx, clientID, ownerWhere, ownerArgs, tokenHash, now); err != nil {
		return nil, "", err
	}
	s.invalidateAPIClientAuthCache(clientID)

	if teamOwned {
		tenantID := ownerArgs[0].(uuid.UUID)
		client, err := s.GetTeamAPIClient(ctx, tenantID, clientID)
		if err != nil {
			return nil, "", err
		}
		return client, token, nil
	}

	userID := ownerArgs[0].(uuid.UUID)
	client, err := s.GetAPIClient(ctx, userID, clientID)
	if err != nil {
		return nil, "", err
	}
	return client, token, nil
}

func (s *Store) ensureAPIClientRotatable(ctx context.Context, clientID uuid.UUID, ownerWhere string, ownerArgs []any, now time.Time) error {
	queryArgs := append([]any{clientID}, ownerArgs...)
	var revokedAt sql.NullTime
	var expiresAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT revoked_at, expires_at
			FROM api_clients
			WHERE id = ? AND %s
	`, ownerWhere), queryArgs...).Scan(&revokedAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAPIClientNotFound
		}
		return fmt.Errorf("failed to load api client for rotation: %w", err)
	}
	if revokedAt.Valid || (expiresAt.Valid && now.After(expiresAt.Time.UTC())) {
		return ErrAPIClientInactive
	}
	return nil
}

func (s *Store) replaceAPIClientToken(ctx context.Context, clientID uuid.UUID, ownerWhere string, ownerArgs []any, tokenHash string, now time.Time) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		queryArgs := append([]any{clientID}, ownerArgs...)
		var revokedAt sql.NullTime
		var expiresAt sql.NullTime
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
				SELECT revoked_at, expires_at
				FROM api_clients
				WHERE id = ? AND %s
		`, ownerWhere), queryArgs...).Scan(&revokedAt, &expiresAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAPIClientNotFound
			}
			return fmt.Errorf("failed to load api client for rotation: %w", err)
		}
		if revokedAt.Valid || (expiresAt.Valid && now.After(expiresAt.Time.UTC())) {
			return ErrAPIClientInactive
		}

		siteRoles, err := apiClientSiteRolesTx(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if len(siteRoles) > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM api_client_site_roles WHERE api_client_id = ?", clientID); err != nil {
				return fmt.Errorf("failed to clear api client site roles before rotation: %w", err)
			}
		}

		updateArgs := append([]any{tokenHash, now, clientID}, ownerArgs...)
		res, err := tx.ExecContext(ctx, fmt.Sprintf(`
				UPDATE api_clients
				SET secret_hash = ?, updated_at = ?
				WHERE id = ? AND %s
		`, ownerWhere), updateArgs...)
		if err != nil {
			return fmt.Errorf("failed to rotate api client token: %w", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to determine rows affected: %w", err)
		}
		if rows == 0 {
			return ErrAPIClientNotFound
		}
		if len(siteRoles) > 0 {
			if err := replaceAPIClientSiteRolesTx(ctx, tx, clientID, siteRoles, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) updateAPIClient(
	ctx context.Context,
	clientID uuid.UUID,
	ownerWhere string,
	ownerArgs []any,
	name string,
	description string,
	instanceRole auth.InstanceRole,
	siteRoles map[uuid.UUID]auth.SiteRole,
	expiresAt *time.Time,
	revoked bool,
	teamOwned bool,
) (*api.APIClient, error) {
	now := time.Now().UTC()
	exp := toUTCPtr(expiresAt)

	err := s.transact(ctx, func(tx *sql.Tx) error {
		queryArgs := make([]any, 0, 8+len(ownerArgs))
		queryArgs = append(queryArgs, name, description, instanceRole, exp, revoked, now, now, clientID)
		queryArgs = append(queryArgs, ownerArgs...)
		res, err := tx.ExecContext(ctx, fmt.Sprintf(`
				UPDATE api_clients
				SET name = ?,
					description = ?,
					instance_role = ?,
					expires_at = ?,
					revoked_at = CASE WHEN ? THEN COALESCE(revoked_at, ?) ELSE NULL END,
					updated_at = ?
				WHERE id = ? AND %s
		`, ownerWhere), queryArgs...)
		if err != nil {
			return fmt.Errorf("failed to update api client: %w", err)
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to determine rows affected: %w", err)
		}
		if rows == 0 {
			return ErrAPIClientNotFound
		}

		if err := replaceAPIClientSiteRolesTx(ctx, tx, clientID, siteRoles, now); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAPIClientNotFound) {
			return nil, nil
		}
		return nil, err
	}
	s.invalidateAPIClientAuthCache(clientID)

	if teamOwned {
		tenantID := ownerArgs[0].(uuid.UUID)
		return s.GetTeamAPIClient(ctx, tenantID, clientID)
	}
	userID := ownerArgs[0].(uuid.UUID)
	return s.GetAPIClient(ctx, userID, clientID)
}

func (s *Store) DeleteAPIClient(ctx context.Context, userID uuid.UUID, clientID uuid.UUID) error {
	return s.deleteAPIClient(ctx, clientID, "user_id = ?", userID)
}

func (s *Store) DeleteTeamAPIClient(ctx context.Context, tenantID, clientID uuid.UUID) error {
	return s.deleteAPIClient(ctx, clientID, "tenant_id = ?", tenantID)
}

func (s *Store) deleteAPIClient(ctx context.Context, clientID uuid.UUID, ownerWhere string, ownerArg any) error {
	if err := s.transact(ctx, func(tx *sql.Tx) error {
		scopedArgs := []any{clientID, ownerArg}
		var scopedID uuid.UUID
		if err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM api_clients WHERE id = ? AND %s", ownerWhere), scopedArgs...).Scan(&scopedID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAPIClientNotFound
			}
			return fmt.Errorf("failed to load api client for delete: %w", err)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM api_client_site_roles WHERE api_client_id = ?", scopedID); err != nil {
			return fmt.Errorf("failed to delete api client site roles: %w", err)
		}

		res, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM api_clients WHERE id = ? AND %s", ownerWhere), scopedArgs...)
		if err != nil {
			return fmt.Errorf("failed to delete api client: %w", err)
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to determine rows affected: %w", err)
		}
		if rows == 0 {
			return ErrAPIClientNotFound
		}
		return nil
	}); err != nil {
		return err
	}
	s.invalidateAPIClientAuthCache(clientID)
	return nil
}

func (s *Store) GetAPIClientAuth(ctx context.Context, token string) (*APIClientAuth, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}

	tokenHash := hashAPIClientToken(token)
	if cached, ok := s.getCachedAPIClientAuth(tokenHash); ok {
		return cached, nil
	}

	var authz APIClientAuth
	var userIDRaw sql.NullString
	var tenantIDRaw sql.NullString
	var instanceRole string
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, CAST(c.user_id AS VARCHAR), CAST(c.tenant_id AS VARCHAR), c.instance_role, c.expires_at, c.revoked_at
		FROM api_clients c
		LEFT JOIN tenant_archives ta ON ta.tenant_id = c.tenant_id
		WHERE c.secret_hash = ?
			AND (c.tenant_id IS NULL OR ta.tenant_id IS NULL)
	`, tokenHash).Scan(
		&authz.ClientID,
		&userIDRaw,
		&tenantIDRaw,
		&instanceRole,
		&expiresAt,
		&revokedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get api client auth: %w", err)
	}

	authz.UserID = parseNullUUIDValue(userIDRaw)
	authz.TenantID = parseNullUUIDValue(tenantIDRaw)
	if authz.UserID == uuid.Nil && authz.TenantID == uuid.Nil {
		return nil, nil
	}

	if revokedAt.Valid {
		return nil, nil
	}
	if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time.UTC()) {
		return nil, nil
	}

	authz.InstanceRole = auth.InstanceRole(instanceRole)
	authz.SiteRoles = make(map[uuid.UUID]auth.SiteRole)

	defaultTenantID := uuid.Nil
	if authz.TenantID != uuid.Nil {
		defaultTenantID, err = s.GetDefaultTenantID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve default tenant for api client auth: %w", err)
		}
	}

	roleQuery := `
		SELECT r.site_id, r.role
		FROM api_client_site_roles r
		JOIN api_clients c ON c.id = r.api_client_id
		LEFT JOIN site_tenants st ON st.site_id = r.site_id
		WHERE r.api_client_id = ?
	`
	roleArgs := []any{authz.ClientID}
	if authz.TenantID != uuid.Nil {
		roleQuery += ` AND COALESCE(st.tenant_id, ?) = ?`
		roleArgs = append(roleArgs, defaultTenantID, authz.TenantID)
	}
	rows, err := s.db.QueryContext(ctx, roleQuery, roleArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query api client site roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var siteID uuid.UUID
		var role string
		if err := rows.Scan(&siteID, &role); err != nil {
			return nil, fmt.Errorf("failed to scan api client site role: %w", err)
		}
		authz.SiteRoles[siteID] = auth.SiteRole(role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api client site roles: %w", err)
	}

	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, "UPDATE api_clients SET last_used_at = ?, updated_at = ? WHERE id = ?", now, now, authz.ClientID)
	s.cacheAPIClientAuth(tokenHash, authz, expiresAt.Time)

	return &authz, nil
}

func replaceAPIClientSiteRolesTx(ctx context.Context, tx *sql.Tx, clientID uuid.UUID, siteRoles map[uuid.UUID]auth.SiteRole, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM api_client_site_roles WHERE api_client_id = ?", clientID); err != nil {
		return fmt.Errorf("failed to replace api client site roles: %w", err)
	}

	if len(siteRoles) == 0 {
		return nil
	}

	siteIDs := make([]uuid.UUID, 0, len(siteRoles))
	for siteID := range siteRoles {
		siteIDs = append(siteIDs, siteID)
	}
	sort.Slice(siteIDs, func(i, j int) bool {
		return siteIDs[i].String() < siteIDs[j].String()
	})

	for _, siteID := range siteIDs {
		role := siteRoles[siteID]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO api_client_site_roles (id, api_client_id, site_id, role, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, uuid.New(), clientID, siteID, role, now); err != nil {
			return fmt.Errorf("failed to insert api client site role: %w", err)
		}
	}

	return nil
}

func apiClientSiteRolesTx(ctx context.Context, tx *sql.Tx, clientID uuid.UUID) (map[uuid.UUID]auth.SiteRole, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT site_id, role
		FROM api_client_site_roles
		WHERE api_client_id = ?
	`, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to load api client site roles: %w", err)
	}
	defer rows.Close()

	siteRoles, err := scanAPIClientSiteRoles(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api client site roles: %w", err)
	}
	return siteRoles, nil
}

func scanAPIClientSiteRoles(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) (map[uuid.UUID]auth.SiteRole, error) {
	siteRoles := make(map[uuid.UUID]auth.SiteRole)
	for rows.Next() {
		var siteID uuid.UUID
		var role string
		if err := rows.Scan(&siteID, &role); err != nil {
			return nil, fmt.Errorf("failed to scan api client site role: %w", err)
		}
		siteRoles[siteID] = auth.SiteRole(role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating api client site roles: %w", err)
	}
	return siteRoles, nil
}

func generateAPIClientToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate api client token: %w", err)
	}

	token := hex.EncodeToString(buf)
	return token, hashAPIClientToken(token), nil
}

func hashAPIClientToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func flattenSiteRoles(siteRoles map[uuid.UUID]auth.SiteRole) []api.APIClientSiteRole {
	if len(siteRoles) == 0 {
		return []api.APIClientSiteRole{}
	}

	siteIDs := make([]uuid.UUID, 0, len(siteRoles))
	for siteID := range siteRoles {
		siteIDs = append(siteIDs, siteID)
	}
	sort.Slice(siteIDs, func(i, j int) bool {
		return siteIDs[i].String() < siteIDs[j].String()
	})

	roles := make([]api.APIClientSiteRole, 0, len(siteIDs))
	for _, siteID := range siteIDs {
		roles = append(roles, api.APIClientSiteRole{
			SiteID: siteID,
			Role:   string(siteRoles[siteID]),
		})
	}
	return roles
}

func scanAPIClient(scanner interface {
	Scan(dest ...any) error
}) (api.APIClient, error) {
	var client api.APIClient
	var userIDRaw sql.NullString
	var tenantIDRaw sql.NullString
	var description sql.NullString
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime

	if err := scanner.Scan(
		&client.ID,
		&userIDRaw,
		&tenantIDRaw,
		&client.Name,
		&description,
		&client.InstanceRole,
		&expiresAt,
		&lastUsedAt,
		&revokedAt,
		&client.CreatedAt,
		&client.UpdatedAt,
	); err != nil {
		return api.APIClient{}, fmt.Errorf("failed to scan api client: %w", err)
	}

	client.UserID = parseNullUUIDPointer(userIDRaw)
	client.TenantID = parseNullUUIDPointer(tenantIDRaw)
	if client.TenantID != nil {
		client.OwnerType = APIClientOwnerTeam
	} else {
		client.OwnerType = APIClientOwnerPersonal
	}
	client.Description = strings.TrimSpace(description.String)
	client.ExpiresAt = nullTimePtr(expiresAt)
	client.LastUsedAt = nullTimePtr(lastUsedAt)
	client.RevokedAt = nullTimePtr(revokedAt)
	client.SiteRoles = make([]api.APIClientSiteRole, 0)

	return client, nil
}

func scanAPIClientRow(row interface {
	Scan(dest ...any) error
}) (*api.APIClient, error) {
	client, err := scanAPIClient(row)
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func buildAPIClient(
	id uuid.UUID,
	userID *uuid.UUID,
	tenantID *uuid.UUID,
	name string,
	description string,
	instanceRole auth.InstanceRole,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	siteRoles []api.APIClientSiteRole,
) api.APIClient {
	ownerType := APIClientOwnerPersonal
	if tenantID != nil && *tenantID != uuid.Nil {
		ownerType = APIClientOwnerTeam
	}

	return api.APIClient{
		ID:           id,
		UserID:       normalizedUUIDPointer(userID),
		TenantID:     normalizedUUIDPointer(tenantID),
		OwnerType:    ownerType,
		Name:         name,
		Description:  description,
		InstanceRole: string(instanceRole),
		ExpiresAt:    expiresAt,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		SiteRoles:    siteRoles,
	}
}

func normalizedUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copy := *value
	return &copy
}

func parseNullUUIDPointer(value sql.NullString) *uuid.UUID {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value.String))
	if err != nil || parsed == uuid.Nil {
		return nil
	}
	return &parsed
}

func parseNullUUIDValue(value sql.NullString) uuid.UUID {
	parsed := parseNullUUIDPointer(value)
	if parsed == nil {
		return uuid.Nil
	}
	return *parsed
}

func nullableUUIDPtr(value *uuid.UUID) any {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return *value
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	ts := value.Time.UTC()
	return &ts
}

func toUTCPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	ts := value.UTC()
	return &ts
}

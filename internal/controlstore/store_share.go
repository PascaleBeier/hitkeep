package controlstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) CreateShareLink(ctx context.Context, siteID uuid.UUID, createdBy uuid.UUID) (*api.ShareLink, string, error) {
	token, tokenHash, err := generateShareToken()
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	linkID := uuid.New()

	if err := s.exec(ctx,
		"INSERT INTO share_links (id, site_id, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		linkID, siteID, tokenHash, createdBy, now,
	); err != nil {
		return nil, "", fmt.Errorf("failed to create share link: %w", err)
	}

	return &api.ShareLink{
		ID:        linkID,
		SiteID:    siteID,
		TokenHint: tokenHash[:8],
		CreatedAt: now,
	}, token, nil
}

func (s *Store) ListShareLinks(ctx context.Context, siteID uuid.UUID) ([]api.ShareLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, SUBSTR(token_hash, 1, 8) AS token_hint, created_at
		FROM share_links
		WHERE site_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list share links: %w", err)
	}
	defer rows.Close()

	links := make([]api.ShareLink, 0)
	for rows.Next() {
		var link api.ShareLink
		if err := rows.Scan(&link.ID, &link.SiteID, &link.TokenHint, &link.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan share link: %w", err)
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read share links: %w", err)
	}

	return links, nil
}

func (s *Store) RevokeShareLink(ctx context.Context, siteID uuid.UUID, shareID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		"UPDATE share_links SET revoked_at = ? WHERE id = ? AND site_id = ? AND revoked_at IS NULL",
		time.Now().UTC(), shareID, siteID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to revoke share link: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read revoked rows: %w", err)
	}

	return rowsAffected > 0, nil
}

func (s *Store) GetShareSiteByToken(ctx context.Context, token string) (*api.Site, error) {
	if token == "" {
		return nil, nil
	}

	tokenHash := hashShareToken(token)

	var site api.Site
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.domain, COALESCE(s.data_retention_days, 0), s.created_at
		FROM share_links sl
		JOIN sites s ON sl.site_id = s.id
		WHERE sl.token_hash = ? AND sl.revoked_at IS NULL
	`, tokenHash).Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lookup share link: %w", err)
	}

	return &site, nil
}

func (s *Store) ActiveShareLinkTokenHashExists(ctx context.Context, tokenHash string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM share_links WHERE token_hash = ? AND revoked_at IS NULL)
	`, strings.TrimSpace(tokenHash)).Scan(&exists)
	return exists, err
}

// EnsureShareLinkWithToken creates a deterministic share link when it is not
// already active. It is used by the local demo seeder, which must publish a
// stable token without exposing raw SQL access to the control database.
func (s *Store) EnsureShareLinkWithToken(ctx context.Context, siteID, createdBy uuid.UUID, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if len(token) != 64 {
		return false, fmt.Errorf("share token must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return false, fmt.Errorf("decode share token: %w", err)
	}

	tokenHash := hashShareToken(token)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO share_links (id, site_id, token_hash, created_by, created_at)
		SELECT ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM share_links WHERE token_hash = ? AND revoked_at IS NULL
		)
	`, uuid.New(), siteID, tokenHash, createdBy, time.Now().UTC(), tokenHash)
	if err != nil {
		return false, fmt.Errorf("ensure share link: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read ensured share link count: %w", err)
	}
	return rows > 0, nil
}

func generateShareToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(bytes)
	return token, hashShareToken(token), nil
}

func hashShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

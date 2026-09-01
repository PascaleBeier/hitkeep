package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	json "hitkeep/jsonapi"
)

func (s *Store) CreateQRCode(ctx context.Context, siteID, createdBy uuid.UUID, req api.QRCodeCreateRequest) (*api.QRCode, string, error) {
	token, tokenHash, err := generateShareToken()
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	qrID := uuid.New()
	customJSON, err := marshalNullableJSON(req.CustomParams)
	if err != nil {
		return nil, "", fmt.Errorf("marshal custom params: %w", err)
	}
	styleJSON, err := marshalNullableJSON(req.Style)
	if err != nil {
		return nil, "", fmt.Errorf("marshal style: %w", err)
	}

	if err := s.Exec(ctx, `
		INSERT INTO qr_codes (
			id, site_id, created_by, name, destination_url,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content,
			custom_params_json, style_json, token, token_hash, token_hint, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, qrID, siteID, nullableUUIDValue(createdBy), req.Name, req.DestinationURL,
		emptyStringAsNil(req.UTMSource), emptyStringAsNil(req.UTMMedium), emptyStringAsNil(req.UTMCampaign), emptyStringAsNil(req.UTMTerm), emptyStringAsNil(req.UTMContent),
		customJSON, styleJSON, token, tokenHash, tokenHash[:8], now, now); err != nil {
		return nil, "", fmt.Errorf("create qr code: %w", err)
	}

	qr, err := s.GetQRCode(ctx, siteID, qrID)
	if err != nil {
		return nil, "", err
	}
	return qr, token, nil
}

func (s *Store) ListQRCodes(ctx context.Context, siteID uuid.UUID, includeArchived bool) ([]api.QRCode, error) {
	query := qrCodeSelectSQL + `
		FROM qr_codes q
		LEFT JOIN qr_code_assets a ON a.qr_code_id = q.id
		WHERE q.site_id = ?
	`
	args := []any{siteID}
	if !includeArchived {
		query += " AND q.archived_at IS NULL"
	}
	query += " ORDER BY q.updated_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list qr codes: %w", err)
	}
	defer rows.Close()

	qrs := []api.QRCode{}
	for rows.Next() {
		qr, err := scanQRCode(rows)
		if err != nil {
			return nil, err
		}
		qrs = append(qrs, *qr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read qr codes: %w", err)
	}
	return qrs, nil
}

func (s *Store) GetQRCode(ctx context.Context, siteID, qrID uuid.UUID) (*api.QRCode, error) {
	row := s.db.QueryRowContext(ctx, qrCodeSelectSQL+`
		FROM qr_codes q
		LEFT JOIN qr_code_assets a ON a.qr_code_id = q.id
		WHERE q.site_id = ? AND q.id = ?
	`, siteID, qrID)
	qr, err := scanQRCode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get qr code: %w", err)
	}
	return qr, nil
}

func (s *Store) GetQRCodeByToken(ctx context.Context, token string) (*api.QRCode, error) {
	if token == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, qrCodeSelectSQL+`
		FROM qr_codes q
		LEFT JOIN qr_code_assets a ON a.qr_code_id = q.id
		WHERE q.token_hash = ? OR q.token = ?
	`, hashShareToken(token), token)
	qr, err := scanQRCode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get qr code by token: %w", err)
	}
	return qr, nil
}

func (s *Store) UpdateQRCode(ctx context.Context, siteID, qrID uuid.UUID, req api.QRCodeUpdateRequest) (*api.QRCode, error) {
	customJSON, err := marshalNullableJSON(req.CustomParams)
	if err != nil {
		return nil, fmt.Errorf("marshal custom params: %w", err)
	}
	styleJSON, err := marshalNullableJSON(req.Style)
	if err != nil {
		return nil, fmt.Errorf("marshal style: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE qr_codes
		SET name = ?, destination_url = ?,
			utm_source = ?, utm_medium = ?, utm_campaign = ?, utm_term = ?, utm_content = ?,
			custom_params_json = ?, style_json = ?, updated_at = ?
		WHERE site_id = ? AND id = ? AND archived_at IS NULL
	`, req.Name, req.DestinationURL,
		emptyStringAsNil(req.UTMSource), emptyStringAsNil(req.UTMMedium), emptyStringAsNil(req.UTMCampaign), emptyStringAsNil(req.UTMTerm), emptyStringAsNil(req.UTMContent),
		customJSON, styleJSON, time.Now().UTC(), siteID, qrID)
	if err != nil {
		return nil, fmt.Errorf("update qr code: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read qr update count: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}
	return s.GetQRCode(ctx, siteID, qrID)
}

func (s *Store) ArchiveQRCode(ctx context.Context, siteID, qrID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE qr_codes
		SET archived_at = COALESCE(archived_at, ?), updated_at = ?
		WHERE site_id = ? AND id = ?
	`, time.Now().UTC(), time.Now().UTC(), siteID, qrID)
	if err != nil {
		return false, fmt.Errorf("archive qr code: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read qr archive count: %w", err)
	}
	return affected > 0, nil
}

func (s *Store) UpsertQRCodeAsset(ctx context.Context, asset api.QRCodeAsset) (*api.QRCodeAsset, error) {
	now := time.Now().UTC()
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	asset.UpdatedAt = now
	legacyEmptyBlob := []byte{}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO qr_code_assets (
			qr_code_id, site_id, filename, content_type, byte_size,
			width, height, checksum, storage_key, data, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (qr_code_id) DO UPDATE SET
			filename = excluded.filename,
			content_type = excluded.content_type,
			byte_size = excluded.byte_size,
			width = excluded.width,
			height = excluded.height,
			checksum = excluded.checksum,
			storage_key = excluded.storage_key,
			data = excluded.data,
			updated_at = excluded.updated_at
	`, asset.QRCodeID, asset.SiteID, asset.Filename, asset.ContentType, asset.ByteSize,
		zeroIntAsNil(asset.Width), zeroIntAsNil(asset.Height), asset.Checksum, asset.StorageKey, legacyEmptyBlob, asset.CreatedAt, asset.UpdatedAt); err != nil {
		return nil, fmt.Errorf("upsert qr asset: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE qr_codes SET updated_at = ? WHERE site_id = ? AND id = ?", now, asset.SiteID, asset.QRCodeID); err != nil {
		return nil, fmt.Errorf("touch qr code after asset upsert: %w", err)
	}
	return s.GetQRCodeAsset(ctx, asset.SiteID, asset.QRCodeID)
}

func (s *Store) GetQRCodeAsset(ctx context.Context, siteID, qrID uuid.UUID) (*api.QRCodeAsset, error) {
	var asset api.QRCodeAsset
	var width, height sql.NullInt32
	var storageKey sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT qr_code_id, site_id, filename, content_type, byte_size, width, height, checksum, storage_key, data, created_at, updated_at
		FROM qr_code_assets
		WHERE site_id = ? AND qr_code_id = ?
	`, siteID, qrID).Scan(&asset.QRCodeID, &asset.SiteID, &asset.Filename, &asset.ContentType, &asset.ByteSize, &width, &height, &asset.Checksum, &storageKey, &asset.Data, &asset.CreatedAt, &asset.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get qr asset: %w", err)
	}
	if width.Valid {
		asset.Width = int(width.Int32)
	}
	if height.Valid {
		asset.Height = int(height.Int32)
	}
	if storageKey.Valid {
		asset.StorageKey = storageKey.String
	}
	return &asset, nil
}

func (s *Store) ListArchivedQRCodeAssetsForRetention(ctx context.Context, siteID uuid.UUID, cutoff time.Time) ([]api.QRCodeAsset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.qr_code_id, a.site_id, a.filename, a.content_type, a.byte_size, a.width, a.height, a.checksum, a.storage_key, a.created_at, a.updated_at
		FROM qr_code_assets a
		JOIN qr_codes q ON q.id = a.qr_code_id AND q.site_id = a.site_id
		WHERE a.site_id = ? AND q.archived_at IS NOT NULL AND q.archived_at < ?
	`, siteID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list archived qr assets: %w", err)
	}
	defer rows.Close()

	assets := []api.QRCodeAsset{}
	for rows.Next() {
		var asset api.QRCodeAsset
		var width, height sql.NullInt32
		var storageKey sql.NullString
		if err := rows.Scan(&asset.QRCodeID, &asset.SiteID, &asset.Filename, &asset.ContentType, &asset.ByteSize, &width, &height, &asset.Checksum, &storageKey, &asset.CreatedAt, &asset.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan archived qr asset: %w", err)
		}
		if width.Valid {
			asset.Width = int(width.Int32)
		}
		if height.Valid {
			asset.Height = int(height.Int32)
		}
		if storageKey.Valid {
			asset.StorageKey = storageKey.String
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read archived qr assets: %w", err)
	}
	return assets, nil
}

func (s *Store) DeleteQRCodeAsset(ctx context.Context, siteID, qrID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM qr_code_assets WHERE site_id = ? AND qr_code_id = ?", siteID, qrID)
	if err != nil {
		return false, fmt.Errorf("delete qr asset: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read qr asset delete count: %w", err)
	}
	if affected > 0 {
		_, _ = s.db.ExecContext(ctx, "UPDATE qr_codes SET updated_at = ? WHERE site_id = ? AND id = ?", time.Now().UTC(), siteID, qrID)
	}
	return affected > 0, nil
}

func (s *Store) CreateQRCodeOpen(ctx context.Context, open *api.QRCodeOpen) error {
	if open == nil {
		return fmt.Errorf("qr open is required")
	}
	if open.ID == uuid.Nil {
		open.ID = uuid.New()
	}
	if open.Timestamp.IsZero() {
		open.Timestamp = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO qr_code_opens (
			id, site_id, qr_code_id, timestamp, referrer, user_agent,
			country_code, region, city, provider, asn, asn_org
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, open.ID, open.SiteID, open.QRCodeID, open.Timestamp,
		nullableStringPtr(open.Referrer), nullableStringPtr(open.UserAgent),
		nullableStringPtr(open.CountryCode), nullableStringPtr(open.Region), nullableStringPtr(open.City),
		nullableStringPtr(open.Provider), nullableIntPtr(open.ASN), nullableStringPtr(open.ASNOrg))
	if err != nil {
		return fmt.Errorf("create qr open: %w", err)
	}
	return nil
}

func (s *Store) CountQRCodeOpens(ctx context.Context, siteID, qrID uuid.UUID, start, end time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM qr_code_opens
		WHERE site_id = ? AND qr_code_id = ? AND timestamp >= ? AND timestamp <= ?
	`, siteID, qrID, start, end).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count qr opens: %w", err)
	}
	return count, nil
}

func (s *Store) GetQRCodeOpenSeries(ctx context.Context, siteID, qrID uuid.UUID, start, end time.Time) ([]api.QRCodeOpenSeriesPoint, error) {
	truncUnit := truncUnitForRange(start, end)
	bucketExpr := bucketSQL("timestamp", truncUnit)

	//nolint:gosec // bucketExpr is selected from a fixed allowlist.
	query := fmt.Sprintf(`
			SELECT %s::TIMESTAMP as bucket, COUNT(*) as opens
			FROM qr_code_opens
			WHERE site_id = ? AND qr_code_id = ? AND timestamp >= ? AND timestamp <= ?
			GROUP BY bucket
		ORDER BY bucket
		`, bucketExpr)
	rows, err := s.db.QueryContext(ctx, query, siteID, qrID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query qr open series: %w", err)
	}
	defer rows.Close()

	counts := map[time.Time]int{}
	for rows.Next() {
		var bucket time.Time
		var opens int
		if err := rows.Scan(&bucket, &opens); err != nil {
			return nil, fmt.Errorf("scan qr open series: %w", err)
		}
		counts[truncToUnit(bucket, truncUnit)] = opens
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read qr open series: %w", err)
	}
	buckets := buildSeriesBuckets(start, end, truncUnit)
	points := make([]api.QRCodeOpenSeriesPoint, 0, len(buckets))
	for _, bucket := range buckets {
		points = append(points, api.QRCodeOpenSeriesPoint{Time: bucket, Opens: counts[bucket]})
	}
	return points, nil
}

func (s *Store) CreateQRCodeShareLink(ctx context.Context, siteID, qrID, createdBy uuid.UUID) (*api.QRCodeShareLink, string, error) {
	token, tokenHash, err := generateShareToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.Exec(ctx, `
		INSERT INTO qr_code_share_links (id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, siteID, qrID, tokenHash, tokenHash[:8], nullableUUIDValue(createdBy), now); err != nil {
		return nil, "", fmt.Errorf("create qr share link: %w", err)
	}
	return &api.QRCodeShareLink{ID: id, SiteID: siteID, QRCodeID: qrID, TokenHint: tokenHash[:8], CreatedAt: now}, token, nil
}

func (s *Store) ListQRCodeShareLinks(ctx context.Context, siteID, qrID uuid.UUID) ([]api.QRCodeShareLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, qr_code_id, token_hint, created_at
		FROM qr_code_share_links
		WHERE site_id = ? AND qr_code_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, siteID, qrID)
	if err != nil {
		return nil, fmt.Errorf("list qr share links: %w", err)
	}
	defer rows.Close()

	links := []api.QRCodeShareLink{}
	for rows.Next() {
		var link api.QRCodeShareLink
		if err := rows.Scan(&link.ID, &link.SiteID, &link.QRCodeID, &link.TokenHint, &link.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan qr share link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read qr share links: %w", err)
	}
	return links, nil
}

func (s *Store) RevokeQRCodeShareLink(ctx context.Context, siteID, qrID, shareID uuid.UUID) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE qr_code_share_links
		SET revoked_at = ?
		WHERE site_id = ? AND qr_code_id = ? AND id = ? AND revoked_at IS NULL
	`, time.Now().UTC(), siteID, qrID, shareID)
	if err != nil {
		return false, fmt.Errorf("revoke qr share link: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read qr share revoke count: %w", err)
	}
	return affected > 0, nil
}

func (s *Store) GetQRCodeByShareToken(ctx context.Context, token string) (*api.QRCode, error) {
	if token == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, qrCodeSelectSQL+`
		FROM qr_code_share_links sl
		JOIN qr_codes q ON q.id = sl.qr_code_id AND q.site_id = sl.site_id
		LEFT JOIN qr_code_assets a ON a.qr_code_id = q.id
		WHERE sl.token_hash = ? AND sl.revoked_at IS NULL
	`, hashShareToken(token))
	qr, err := scanQRCode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get qr code by share token: %w", err)
	}
	return qr, nil
}

const qrCodeSelectSQL = `
	SELECT
		q.id, q.site_id, q.created_by, q.name, q.destination_url,
		q.utm_source, q.utm_medium, q.utm_campaign, q.utm_term, q.utm_content,
		q.custom_params_json, q.style_json, q.token, q.token_hint,
		CASE WHEN a.qr_code_id IS NULL THEN FALSE ELSE TRUE END AS has_asset,
		q.created_at, q.updated_at, q.archived_at
`

type qrCodeScanner interface {
	Scan(dest ...any) error
}

func scanQRCode(scanner qrCodeScanner) (*api.QRCode, error) {
	var qr api.QRCode
	var createdBy uuid.NullUUID
	var utmSource, utmMedium, utmCampaign, utmTerm, utmContent sql.NullString
	var customJSON, styleJSON any
	var archivedAt sql.NullTime
	if err := scanner.Scan(
		&qr.ID, &qr.SiteID, &createdBy, &qr.Name, &qr.DestinationURL,
		&utmSource, &utmMedium, &utmCampaign, &utmTerm, &utmContent,
		&customJSON, &styleJSON, &qr.RedirectToken, &qr.TokenHint, &qr.HasAsset, &qr.CreatedAt, &qr.UpdatedAt, &archivedAt,
	); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		qr.CreatedBy = createdBy.UUID
	}
	qr.UTMSource = nullString(utmSource)
	qr.UTMMedium = nullString(utmMedium)
	qr.UTMCampaign = nullString(utmCampaign)
	qr.UTMTerm = nullString(utmTerm)
	qr.UTMContent = nullString(utmContent)
	qr.CustomParams = map[string]string{}
	if err := decodeJSONValue(customJSON, &qr.CustomParams); err != nil {
		return nil, fmt.Errorf("decode qr custom params: %w", err)
	}
	qr.Style = map[string]any{}
	if err := decodeJSONValue(styleJSON, &qr.Style); err != nil {
		return nil, fmt.Errorf("decode qr style: %w", err)
	}
	if archivedAt.Valid {
		qr.ArchivedAt = &archivedAt.Time
	}
	return &qr, nil
}

func decodeJSONValue(value any, target any) error {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return json.Unmarshal([]byte(typed), target)
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		return json.Unmarshal(typed, target)
	default:
		body, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		return json.Unmarshal(body, target)
	}
}

func marshalNullableJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case map[string]string:
		if len(typed) == 0 {
			return nil, nil
		}
	case map[string]any:
		if len(typed) == 0 {
			return nil, nil
		}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(body), nil
}

func emptyStringAsNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func zeroIntAsNil(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableUUIDValue(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

package controlstore

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

// TakeoutScope bounds the control-plane records that may be exported for a
// user, site, or QR-code takeout. Secrets and credential material are never
// selected by the registry below.
type TakeoutScope struct {
	SiteIDs  []uuid.UUID
	UserID   uuid.UUID
	QRCodeID uuid.UUID
}

type takeoutSelect struct {
	recordType string
	query      string
	args       []any
}

// WriteTakeoutNDJSON writes control-plane takeout records without exposing the
// underlying SQLite handle. It returns the number of records written.
func (s *Store) WriteTakeoutNDJSON(ctx context.Context, dst io.Writer, scope TakeoutScope) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("control store is not open")
	}
	if dst == nil {
		return 0, fmt.Errorf("takeout destination is required")
	}
	selects := controlTakeoutSelects(scope)
	buffered := bufio.NewWriter(dst)
	encoder := json.NewEncoder(buffered)
	var count int64
	for _, selection := range selects {
		written, err := s.writeTakeoutSelection(ctx, encoder, selection)
		count += written
		if err != nil {
			return count, err
		}
	}
	if err := buffered.Flush(); err != nil {
		return count, fmt.Errorf("flush control takeout: %w", err)
	}
	return count, nil
}

func (s *Store) writeTakeoutSelection(ctx context.Context, encoder *json.Encoder, selection takeoutSelect) (count int64, err error) {
	rows, err := s.db.QueryContext(ctx, selection.query, selection.args...)
	if err != nil {
		return 0, fmt.Errorf("query %s control takeout records: %w", selection.recordType, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s control takeout rows: %w", selection.recordType, closeErr)
		}
	}()
	return encodeTakeoutRows(encoder, rows, selection.recordType)
}

func controlTakeoutSelects(scope TakeoutScope) []takeoutSelect {
	where, siteArgs := sqliteUUIDScope("site_id", scope.SiteIDs)
	qrWhere := where
	qrChildWhere := where
	qrArgs := cloneArgs(siteArgs)
	if scope.QRCodeID != uuid.Nil {
		qrWhere += " AND id = ?"
		qrChildWhere += " AND qr_code_id = ?"
		qrArgs = append(qrArgs, scope.QRCodeID.String())
	}
	if len(scope.SiteIDs) == 0 {
		where = "0"
		qrWhere = "0"
		qrChildWhere = "0"
		siteArgs = nil
		qrArgs = nil
	}

	selects := []takeoutSelect{
		{recordType: "qr_code", query: "SELECT * FROM qr_codes WHERE " + qrWhere, args: cloneArgs(qrArgs)},
		{recordType: "qr_code_asset", query: `SELECT qr_code_id, site_id, filename, content_type, byte_size, width, height, checksum, created_at, updated_at FROM qr_code_assets WHERE ` + qrChildWhere, args: cloneArgs(qrArgs)},
		{recordType: "qr_code_share_link", query: `SELECT * FROM qr_code_share_links WHERE ` + qrChildWhere, args: cloneArgs(qrArgs)},
	}
	if scope.QRCodeID == uuid.Nil {
		selects = append(selects,
			takeoutSelect{recordType: "opportunity", query: `SELECT id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, digest_key, copy_params_json, impact_value, impact_label_key, confidence, score, score_breakdown_json, status, route_label_key, route_params_json, route_icon, detector_version, evidence_json, cited_evidence_ids_json, ai_run_id, generated_at, created_at, updated_at FROM opportunities WHERE ` + where, args: cloneArgs(siteArgs)},
			takeoutSelect{recordType: "ai_run", query: `SELECT id, team_id, site_id, actor_id, actor_type, feature, provider, model, template_version, evidence_ids_json, input_hash, output_hash, input_tokens, output_tokens, total_tokens, tool_call_count, lifecycle_events_json, status, error_category, latency_ms, created_at FROM ai_runs WHERE ` + where, args: cloneArgs(siteArgs)},
			takeoutSelect{recordType: "webhook", query: `SELECT id, site_id, name, description, enabled, created_at, updated_at FROM webhooks WHERE ` + where, args: cloneArgs(siteArgs)},
			takeoutSelect{recordType: "webhook_event_subscription", query: `SELECT webhook_id, event_type FROM webhook_event_subscriptions WHERE webhook_id IN (SELECT id FROM webhooks WHERE ` + where + `)`, args: cloneArgs(siteArgs)},
			takeoutSelect{recordType: "webhook_delivery", query: `SELECT id, event_id, webhook_id, site_id, event_type, webhook_name, status, attempt_count, next_attempt_at, last_attempt_at, completed_at, response_status, last_error_code, created_at, updated_at FROM webhook_deliveries WHERE webhook_id IN (SELECT id FROM webhooks WHERE ` + where + `)`, args: cloneArgs(siteArgs)},
			takeoutSelect{recordType: "webhook_delivery_attempt", query: `SELECT id, delivery_id, site_id, attempt_number, status, response_status, error_code, started_at, completed_at, next_attempt_at FROM webhook_delivery_attempts WHERE delivery_id IN (SELECT id FROM webhook_deliveries WHERE webhook_id IN (SELECT id FROM webhooks WHERE ` + where + `))`, args: cloneArgs(siteArgs)},
		)
	}
	if scope.UserID != uuid.Nil {
		user := scope.UserID.String()
		selects = append(selects,
			takeoutSelect{recordType: "social_identity", query: `SELECT user_id, provider, subject, observed_email, linked_at, updated_at, last_used_at FROM social_identities WHERE user_id = ?`, args: []any{user}},
			takeoutSelect{recordType: "report_definition", query: `SELECT rd.id, rd.tenant_id, rd.owner_user_id, rd.name, rd.scope, rd.preset, rd.site_mode, rd.frequency, rd.timezone, rd.local_time, rd.weekly_day, rd.monthly_day, rd.status, rd.consent_version, rd.next_run_at, rd.created_at, rd.updated_at FROM report_definitions rd WHERE rd.owner_user_id = ? OR EXISTS (SELECT 1 FROM report_recipients rr WHERE rr.report_id = rd.id AND rr.user_id = ?)`, args: []any{user, user}},
			takeoutSelect{recordType: "report_site", query: `SELECT rds.report_id, rds.site_id, rds.created_at FROM report_definition_sites rds WHERE rds.report_id IN (SELECT rd.id FROM report_definitions rd WHERE rd.owner_user_id = ? OR EXISTS (SELECT 1 FROM report_recipients rr WHERE rr.report_id = rd.id AND rr.user_id = ?))`, args: []any{user, user}},
			takeoutSelect{recordType: "report_recipient", query: `SELECT report_id, user_id, opted_out_at, created_at, updated_at FROM report_recipients WHERE user_id = ?`, args: []any{user}},
			takeoutSelect{recordType: "report_run", query: `SELECT rr.id, rr.report_id, rr.scheduled_for, rr.period_start, rr.period_end, rr.status, rr.safe_error_code, rr.started_at, rr.completed_at, rr.created_at, rr.updated_at FROM report_runs rr WHERE rr.report_id IN (SELECT rd.id FROM report_definitions rd WHERE rd.owner_user_id = ? OR EXISTS (SELECT 1 FROM report_recipients rc WHERE rc.report_id = rd.id AND rc.user_id = ?))`, args: []any{user, user}},
			takeoutSelect{recordType: "report_delivery", query: `SELECT d.id, d.report_id, d.run_id, d.recipient_id, d.recipient_kind, d.status, d.attempt_count, d.next_attempt_at, d.safe_error_code, d.smtp_accepted_at, d.created_at, d.updated_at FROM report_deliveries d JOIN report_recipients rr ON rr.id = d.recipient_id WHERE rr.user_id = ?`, args: []any{user}},
		)
	}
	return selects
}

func sqliteUUIDScope(column string, ids []uuid.UUID) (string, []any) {
	if len(ids) == 0 {
		return "0", nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id.String()
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")", args
}

func cloneArgs(args []any) []any { return append([]any(nil), args...) }

func encodeTakeoutRows(encoder *json.Encoder, rows *sql.Rows, recordType string) (int64, error) {
	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("list %s control takeout columns: %w", recordType, err)
	}
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	var count int64
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return count, fmt.Errorf("scan %s control takeout row: %w", recordType, err)
		}
		record := make(map[string]any, len(columns)+1)
		record["record_type"] = recordType
		for i, column := range columns {
			record[column] = values[i]
		}
		if err := encoder.Encode(record); err != nil {
			return count, fmt.Errorf("encode %s control takeout row: %w", recordType, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("read %s control takeout rows: %w", recordType, err)
	}
	return count, nil
}

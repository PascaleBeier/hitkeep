package database

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

var chatbotExportEventNames = []string{
	"assistant.chat_started",
	"assistant.message_sent",
	"assistant.response_rendered",
	"assistant.citation_clicked",
	"assistant.handoff_requested",
	"assistant.goal_assisted",
}

func (s *Store) ExportChatbotEventsCSV(ctx context.Context, params api.ChatbotExportParams, w io.Writer) error {
	selectQuery, args := buildChatbotExportQuery(params)

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to query chatbot events for export: %w", err)
	}
	defer rows.Close()

	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"id",
		"site_id",
		"session_id",
		"timestamp",
		"name",
		"provider",
		"bot_id",
		"surface",
		"model",
		"intent",
		"properties_json",
	}); err != nil {
		return fmt.Errorf("failed to write chatbot csv header: %w", err)
	}

	for rows.Next() {
		var (
			id             uuid.UUID
			siteID         uuid.UUID
			sessionID      uuid.UUID
			timestamp      time.Time
			name           string
			provider       sql.NullString
			botID          sql.NullString
			surface        sql.NullString
			model          sql.NullString
			intent         sql.NullString
			propertiesJSON sql.NullString
		)
		if err := rows.Scan(
			&id,
			&siteID,
			&sessionID,
			&timestamp,
			&name,
			&provider,
			&botID,
			&surface,
			&model,
			&intent,
			&propertiesJSON,
		); err != nil {
			return fmt.Errorf("failed to scan chatbot export row: %w", err)
		}

		record := []string{
			id.String(),
			siteID.String(),
			sessionID.String(),
			timestamp.Format(time.RFC3339),
			name,
			nullString(provider),
			nullString(botID),
			nullString(surface),
			nullString(model),
			nullString(intent),
			nullString(propertiesJSON),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write chatbot csv record: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read chatbot export rows: %w", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush chatbot csv: %w", err)
	}
	return nil
}

func (s *Store) ExportChatbotEventsFile(ctx context.Context, params api.ChatbotExportParams, format string) (string, error) {
	selectQuery, args := buildChatbotExportQuery(params)
	filename, err := s.exportQueryToTempFile(ctx, "hitkeep_ai_chatbots_", "hitkeep_ai_chatbots_", selectQuery, args, format)
	if err != nil {
		return "", fmt.Errorf("failed to export chatbot events: %w", err)
	}

	return filename, nil
}

func buildChatbotExportQuery(params api.ChatbotExportParams) (string, []any) {
	baseQuery := `
		FROM events
		WHERE site_id = ?
		  AND timestamp >= ?
		  AND timestamp <= ?
		  AND name IN (?, ?, ?, ?, ?, ?)
	`
	args := make([]any, 0, 9)
	args = append(args, params.SiteID, params.Start, params.End)
	for _, eventName := range chatbotExportEventNames {
		args = append(args, eventName)
	}

	if params.ScopeKey != "" && params.ScopeValue != "" {
		baseQuery += " AND json_extract_string(properties, ?) = ?"
		args = append(args, "$."+params.ScopeKey, params.ScopeValue)
	}

	baseQuery += " ORDER BY timestamp DESC"

	//nolint:gosec // baseQuery is composed from a fixed event list and parameterized filter placeholders.
	selectQuery := `
		SELECT
			id,
			site_id,
			session_id,
			timestamp,
			name,
			json_extract_string(properties, '$.provider') AS provider,
			json_extract_string(properties, '$.bot_id') AS bot_id,
			json_extract_string(properties, '$.surface') AS surface,
			json_extract_string(properties, '$.model') AS model,
			json_extract_string(properties, '$.intent') AS intent,
			CAST(properties AS VARCHAR) AS properties_json
	` + baseQuery

	return selectQuery, args
}

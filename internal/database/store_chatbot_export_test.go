package database

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/exportfmt"
	"hitkeep/internal/api"
)

func TestExportChatbotEventsCSVIncludesScopeColumnsAndFilters(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "chatbot-export.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	sessionID := mustUUID(t)
	events := []*api.Event{
		{
			SiteID:    site.ID,
			SessionID: sessionID,
			Name:      "assistant.chat_started",
			Properties: map[string]any{
				"provider": "OpenAI",
				"bot_id":   "support-bot",
				"surface":  "pricing",
				"model":    "gpt-4.1-mini",
			},
			Timestamp: base.Add(-2 * time.Hour),
		},
		{
			SiteID:    site.ID,
			SessionID: sessionID,
			Name:      "assistant.message_sent",
			Properties: map[string]any{
				"provider": "OpenAI",
				"bot_id":   "support-bot",
				"surface":  "pricing",
				"model":    "gpt-4.1-mini",
				"intent":   "pricing",
			},
			Timestamp: base.Add(-90 * time.Minute),
		},
		{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			Name:      "assistant.chat_started",
			Properties: map[string]any{
				"provider": "Anthropic",
				"bot_id":   "sales-bot",
				"surface":  "docs",
				"model":    "claude-sonnet-4-5",
			},
			Timestamp: base.Add(-30 * time.Minute),
		},
	}

	for _, event := range events {
		if err := store.CreateEvent(ctx, event); err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := store.ExportChatbotEventsCSV(ctx, api.ChatbotExportParams{
		SiteID:     site.ID,
		Start:      base.Add(-24 * time.Hour),
		End:        base,
		ScopeKey:   "provider",
		ScopeValue: "OpenAI",
	}, &buf); err != nil {
		t.Fatalf("export chatbot csv: %v", err)
	}

	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("read chatbot csv: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected header plus two rows, got %d", len(rows))
	}

	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}

	for _, col := range []string{"provider", "bot_id", "surface", "model", "intent", "properties_json"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("expected header column %q", col)
		}
	}

	firstDataRow := rows[1]
	if got := firstDataRow[index["provider"]]; got != "OpenAI" {
		t.Fatalf("expected provider OpenAI, got %q", got)
	}
	if got := firstDataRow[index["bot_id"]]; got != "support-bot" {
		t.Fatalf("expected bot_id support-bot, got %q", got)
	}

	var sentRow []string
	for _, row := range rows[1:] {
		if row[index["name"]] == "assistant.message_sent" {
			sentRow = row
			break
		}
	}
	if sentRow == nil {
		t.Fatalf("expected assistant.message_sent row in export, got %v", rows[1:])
	}
	if got := sentRow[index["intent"]]; got != "pricing" {
		t.Fatalf("expected intent pricing, got %q", got)
	}
	if got := sentRow[index["properties_json"]]; !strings.Contains(got, "\"intent\":\"pricing\"") {
		t.Fatalf("expected exported properties json to include intent, got %q", got)
	}
}

func TestExportChatbotEventsFileSupportsAllFormats(t *testing.T) {
	store, userID := setupComparisonStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "chatbot-export-file.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		Name:      "assistant.chat_started",
		Properties: map[string]any{
			"provider": "OpenAI",
			"bot_id":   "support-bot",
			"surface":  "pricing",
			"model":    "gpt-4.1-mini",
		},
		Timestamp: base.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	params := api.ChatbotExportParams{
		SiteID: site.ID,
		Start:  base.Add(-24 * time.Hour),
		End:    base,
	}

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "csv", format: "csv", want: ".csv"},
		{name: "xlsx", format: "xlsx", want: ".xlsx"},
		{name: "parquet", format: "parquet", want: ".parquet"},
		{name: "json", format: "json", want: ".json"},
		{name: "ndjson", format: "ndjson", want: ".ndjson"},
		{name: "unknown defaults to csv", format: "xml", want: ".csv"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename, err := store.ExportChatbotEventsFile(ctx, params, tc.format)
			if err != nil {
				t.Fatalf("export chatbot events file: %v", err)
			}
			t.Cleanup(func() { _ = os.Remove(filename) })

			if got := strings.ToLower(filepath.Ext(filename)); got != tc.want {
				t.Fatalf("expected extension %q, got %q", tc.want, got)
			}

			contentType := exportfmt.ContentType(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."))
			if contentType == "" {
				t.Fatalf("expected content type mapping for %q", filename)
			}

			info, err := os.Stat(filename)
			if err != nil {
				t.Fatalf("stat chatbot export file: %v", err)
			}
			if info.Size() == 0 {
				t.Fatalf("expected non-empty chatbot export file for format %q", tc.format)
			}
		})
	}
}

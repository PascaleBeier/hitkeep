package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrAIBudgetExhausted = errors.New("ai budget exhausted")

type AILifecycleEvent struct {
	Type          string    `json:"type"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	ToolName      string    `json:"tool_name,omitempty"`
	Step          int       `json:"step,omitempty"`
	Status        string    `json:"status,omitempty"`
	StatusCode    int       `json:"status_code,omitempty"`
	ErrorCategory string    `json:"error_category,omitempty"`
	LatencyMS     int64     `json:"latency_ms,omitempty"`
	MessageCount  int       `json:"message_count,omitempty"`
	ToolCount     int       `json:"tool_count,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type AIRunParams struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	SiteID          uuid.UUID
	ActorID         uuid.UUID
	ActorType       string
	Feature         string
	Provider        string
	Model           string
	TemplateVersion string
	EvidenceIDs     []string
	InputHash       string
	OutputHash      string
	OutputJSON      string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	ToolCallCount   int
	LifecycleEvents []AILifecycleEvent
	Status          string
	ErrorCategory   string
	LatencyMS       int64
	CreatedAt       time.Time
}

type AIUsageSummary struct {
	Requests int
	Tokens   int
}

type AIRunSummary struct {
	LastSuccessAt     *time.Time
	LastAttemptAt     *time.Time
	LastErrorCategory string
}

type preparedAIRun struct {
	ID                  uuid.UUID
	CreatedAt           time.Time
	EvidenceJSON        string
	OutputJSON          string
	LifecycleEventsJSON string
	Status              string
	ErrorCategory       string
}

func (s *Store) AppendAIRun(ctx context.Context, params AIRunParams) (uuid.UUID, error) {
	return appendAIRun(ctx, s.db, params)
}

func appendAIRun(ctx context.Context, exec sqlExecContext, params AIRunParams) (uuid.UUID, error) {
	prepared, err := prepareAIRun(params)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = exec.ExecContext(ctx, `
			INSERT INTO ai_runs (
				id, team_id, site_id, actor_id, actor_type, feature, provider, model,
				template_version, evidence_ids_json, input_hash, output_hash, output_json,
			input_tokens, output_tokens, total_tokens, tool_call_count, lifecycle_events_json, status, error_category,
				latency_ms, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				team_id = excluded.team_id,
				site_id = excluded.site_id,
				actor_id = excluded.actor_id,
				actor_type = excluded.actor_type,
				feature = excluded.feature,
				provider = excluded.provider,
				model = excluded.model,
				template_version = excluded.template_version,
				evidence_ids_json = excluded.evidence_ids_json,
				input_hash = excluded.input_hash,
				output_hash = excluded.output_hash,
				output_json = excluded.output_json,
				input_tokens = excluded.input_tokens,
				output_tokens = excluded.output_tokens,
				total_tokens = excluded.total_tokens,
				tool_call_count = excluded.tool_call_count,
				lifecycle_events_json = excluded.lifecycle_events_json,
				status = excluded.status,
				error_category = excluded.error_category,
				latency_ms = excluded.latency_ms
		`,
		prepared.ID,
		nullableUUID(params.TeamID),
		nullableUUID(params.SiteID),
		nullableUUID(params.ActorID),
		strings.TrimSpace(params.ActorType),
		strings.TrimSpace(params.Feature),
		strings.TrimSpace(params.Provider),
		strings.TrimSpace(params.Model),
		strings.TrimSpace(params.TemplateVersion),
		prepared.EvidenceJSON,
		strings.TrimSpace(params.InputHash),
		strings.TrimSpace(params.OutputHash),
		prepared.OutputJSON,
		params.InputTokens,
		params.OutputTokens,
		params.TotalTokens,
		params.ToolCallCount,
		prepared.LifecycleEventsJSON,
		prepared.Status,
		prepared.ErrorCategory,
		params.LatencyMS,
		prepared.CreatedAt,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("append ai run: %w", err)
	}
	return prepared.ID, nil
}

func prepareAIRun(params AIRunParams) (preparedAIRun, error) {
	id, err := prepareAIRunID(params.ID)
	if err != nil {
		return preparedAIRun{}, err
	}
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	evidenceJSON, err := json.Marshal(params.EvidenceIDs)
	if err != nil {
		return preparedAIRun{}, fmt.Errorf("encode ai run evidence ids: %w", err)
	}
	outputJSON, err := prepareAIRunOutputJSON(params.Feature, params.OutputJSON, params.EvidenceIDs)
	if err != nil {
		return preparedAIRun{}, err
	}
	lifecycleEventsJSON, err := prepareAIRunLifecycleEventsJSON(params.LifecycleEvents)
	if err != nil {
		return preparedAIRun{}, fmt.Errorf("encode ai run lifecycle events: %w", err)
	}
	status, err := prepareAIRunStatus(params.Status)
	if err != nil {
		return preparedAIRun{}, err
	}
	errorCategory, err := prepareAIRunErrorCategory(params.ErrorCategory)
	if err != nil {
		return preparedAIRun{}, err
	}
	return preparedAIRun{
		ID:                  id,
		CreatedAt:           createdAt,
		EvidenceJSON:        string(evidenceJSON),
		OutputJSON:          outputJSON,
		LifecycleEventsJSON: string(lifecycleEventsJSON),
		Status:              status,
		ErrorCategory:       errorCategory,
	}, nil
}

func prepareAIRunID(id uuid.UUID) (uuid.UUID, error) {
	if id != uuid.Nil {
		return id, nil
	}
	generatedID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate ai run id: %w", err)
	}
	return generatedID, nil
}

func (s *Store) ReserveAIRun(ctx context.Context, params AIRunParams, since time.Time, requestLimit, tokenLimit int) (uuid.UUID, error) {
	s.aiBudgetMu.Lock()
	defer s.aiBudgetMu.Unlock()

	var id uuid.UUID
	exhausted := false
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		usage, err := queryAIUsageSinceForBudget(ctx, tx, since)
		if err != nil {
			return err
		}
		if aiBudgetExhausted(usage, requestLimit, tokenLimit) {
			exhausted = true
			id, err = appendAIBudgetExhaustedRun(ctx, tx, params)
			return err
		}
		id, err = appendAIReservedRun(ctx, tx, params)
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}
	if exhausted {
		return id, ErrAIBudgetExhausted
	}
	return id, nil
}

func (s *Store) GetAIUsageSince(ctx context.Context, since time.Time) (AIUsageSummary, error) {
	return queryAIUsageSince(ctx, s.db, since)
}

type aiUsageQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryAIUsageSince(ctx context.Context, db aiUsageQuerier, since time.Time) (AIUsageSummary, error) {
	var usage AIUsageSummary
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_tokens), 0)
		FROM ai_runs
		WHERE created_at >= ?
			AND error_category NOT IN ('budget_exhausted', 'disabled', 'not_configured')
	`, since).Scan(&usage.Requests, &usage.Tokens)
	if err != nil {
		return AIUsageSummary{}, fmt.Errorf("query ai usage: %w", err)
	}
	return usage, nil
}

func queryAIUsageSinceForBudget(ctx context.Context, db aiUsageQuerier, since time.Time) (AIUsageSummary, error) {
	return queryAIUsageSince(ctx, db, since)
}

func aiBudgetExhausted(usage AIUsageSummary, requestLimit, tokenLimit int) bool {
	return (requestLimit > 0 && usage.Requests >= requestLimit) ||
		(tokenLimit > 0 && usage.Tokens >= tokenLimit)
}

func appendAIBudgetExhaustedRun(ctx context.Context, exec sqlExecContext, params AIRunParams) (uuid.UUID, error) {
	params.Status = "failure"
	params.ErrorCategory = "budget_exhausted"
	return appendAIRun(ctx, exec, params)
}

func appendAIReservedRun(ctx context.Context, exec sqlExecContext, params AIRunParams) (uuid.UUID, error) {
	params.Status = strings.TrimSpace(params.Status)
	if params.Status == "" {
		params.Status = "reserved"
	}
	return appendAIRun(ctx, exec, params)
}

func (s *Store) GetAIRunSummary(ctx context.Context) (AIRunSummary, error) {
	var summary AIRunSummary
	var lastSuccessRaw, lastAttemptRaw sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT
			MAX(created_at) FILTER (WHERE status = 'success'),
			MAX(created_at),
			COALESCE((
				SELECT error_category
				FROM ai_runs
				WHERE status <> 'success' AND error_category <> ''
				ORDER BY created_at DESC
				LIMIT 1
			), '')
		FROM ai_runs
	`).Scan(&lastSuccessRaw, &lastAttemptRaw, &summary.LastErrorCategory)
	if err != nil {
		return AIRunSummary{}, fmt.Errorf("query ai run summary: %w", err)
	}
	if lastSuccessRaw.Valid {
		summary.LastSuccessAt = &lastSuccessRaw.Time
	}
	if lastAttemptRaw.Valid {
		summary.LastAttemptAt = &lastAttemptRaw.Time
	}
	return summary, nil
}

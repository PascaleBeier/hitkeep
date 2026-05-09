package ai

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

type StoreRecorder struct {
	Store *database.Store
}

func (r StoreRecorder) RecordAIRun(ctx context.Context, run RunRecord) (uuid.UUID, error) {
	if r.Store == nil {
		return uuid.Nil, nil
	}
	lifecycleEvents := make([]database.AILifecycleEvent, 0, len(run.LifecycleEvents))
	for _, event := range run.LifecycleEvents {
		lifecycleEvents = append(lifecycleEvents, database.AILifecycleEvent{
			Type:          event.Type,
			Provider:      event.Provider,
			Model:         event.Model,
			ToolName:      event.ToolName,
			Step:          event.Step,
			Status:        event.Status,
			StatusCode:    event.StatusCode,
			ErrorCategory: event.ErrorCategory,
			LatencyMS:     event.LatencyMS,
			MessageCount:  event.MessageCount,
			ToolCount:     event.ToolCount,
			Timestamp:     event.Timestamp,
		})
	}
	return r.Store.AppendAIRun(ctx, database.AIRunParams{
		ID:              run.ID,
		TeamID:          run.TeamID,
		SiteID:          run.SiteID,
		ActorID:         run.ActorID,
		ActorType:       run.ActorType,
		Feature:         run.Feature,
		Provider:        run.Provider,
		Model:           run.Model,
		TemplateVersion: run.TemplateVersion,
		EvidenceIDs:     run.EvidenceIDs,
		InputHash:       run.InputHash,
		OutputHash:      run.OutputHash,
		OutputJSON:      run.OutputJSON,
		InputTokens:     run.Usage.InputTokens,
		OutputTokens:    run.Usage.OutputTokens,
		TotalTokens:     run.Usage.TotalTokens,
		ToolCallCount:   run.Usage.ToolCallCount,
		LifecycleEvents: lifecycleEvents,
		Status:          run.Status,
		ErrorCategory:   run.ErrorCategory,
		LatencyMS:       run.Latency.Milliseconds(),
		CreatedAt:       run.CreatedAt,
	})
}

func (r StoreRecorder) ReserveAIRun(ctx context.Context, run RunRecord, since time.Time, requestLimit, tokenLimit int) (uuid.UUID, error) {
	if r.Store == nil {
		return uuid.Nil, nil
	}
	id, err := r.Store.ReserveAIRun(ctx, database.AIRunParams{
		ID:              run.ID,
		TeamID:          run.TeamID,
		SiteID:          run.SiteID,
		ActorID:         run.ActorID,
		ActorType:       run.ActorType,
		Feature:         run.Feature,
		Provider:        run.Provider,
		Model:           run.Model,
		TemplateVersion: run.TemplateVersion,
		EvidenceIDs:     run.EvidenceIDs,
		InputHash:       run.InputHash,
		OutputHash:      run.OutputHash,
		OutputJSON:      run.OutputJSON,
		InputTokens:     run.Usage.InputTokens,
		OutputTokens:    run.Usage.OutputTokens,
		TotalTokens:     run.Usage.TotalTokens,
		ToolCallCount:   run.Usage.ToolCallCount,
		Status:          run.Status,
		ErrorCategory:   run.ErrorCategory,
		LatencyMS:       run.Latency.Milliseconds(),
		CreatedAt:       run.CreatedAt,
	}, since, requestLimit, tokenLimit)
	if errors.Is(err, database.ErrAIBudgetExhausted) {
		return uuid.Nil, ErrBudgetExhausted
	}
	return id, err
}

func (r StoreRecorder) GetAIUsageSince(ctx context.Context, since time.Time) (BudgetUsage, error) {
	if r.Store == nil {
		return BudgetUsage{}, nil
	}
	usage, err := r.Store.GetAIUsageSince(ctx, since)
	if err != nil {
		return BudgetUsage{}, err
	}
	return BudgetUsage{Requests: usage.Requests, Tokens: usage.Tokens}, nil
}

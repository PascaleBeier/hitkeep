package ai

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type runLedger struct {
	conf     Config
	recorder RunRecorder
}

func newRunLedger(conf Config, recorder RunRecorder) runLedger {
	return runLedger{conf: conf, recorder: recorder}
}

func (l runLedger) reserve(ctx context.Context, req OpportunityRequest) (uuid.UUID, error) {
	if l.recorder == nil {
		return uuid.Nil, l.checkBudget(ctx)
	}
	window := l.budgetWindow()
	return l.recorder.ReserveAIRun(ctx, l.runRecord(uuid.Nil, req, nil, "reserved", "", 0, Usage{}, nil), time.Now().UTC().Add(-window), l.conf.RequestLimit, l.conf.TokenLimit)
}

func (l runLedger) checkBudget(ctx context.Context) error {
	if l.recorder == nil {
		return nil
	}
	usage, err := l.recorder.GetAIUsageSince(ctx, time.Now().UTC().Add(-l.budgetWindow()))
	if err != nil {
		return err
	}
	if l.conf.RequestLimit > 0 && usage.Requests >= l.conf.RequestLimit {
		return ErrBudgetExhausted
	}
	if l.conf.TokenLimit > 0 && usage.Tokens >= l.conf.TokenLimit {
		return ErrBudgetExhausted
	}
	return nil
}

func (l runLedger) recordNotConfigured(ctx context.Context, req OpportunityRequest) error {
	_, err := l.recordID(ctx, uuid.Nil, req, nil, "failure", "not_configured", 0, Usage{}, nil)
	return err
}

func (l runLedger) recordID(ctx context.Context, runID uuid.UUID, req OpportunityRequest, output *OpportunityCandidateProposal, status, category string, latency time.Duration, usage Usage, lifecycleEvents []LifecycleEvent) (uuid.UUID, error) {
	if l.recorder == nil {
		return uuid.Nil, nil
	}
	return l.recorder.RecordAIRun(ctx, l.runRecord(runID, req, output, status, category, latency, usage, lifecycleEvents))
}

func (l runLedger) finalizeGeneration(ctx context.Context, runID uuid.UUID, req OpportunityRequest, generation opportunityGeneration) (uuid.UUID, error) {
	status, category, outputForAudit := generationAuditFields(generation)
	finalRunID, recordErr := l.recordID(ctx, runID, req, outputForAudit, status, category, generation.Latency, generation.Usage, generation.LifecycleEvents)
	if generation.Err != nil {
		if recordErr != nil {
			return uuid.Nil, errors.Join(generation.Err, recordErr)
		}
		return uuid.Nil, generation.Err
	}
	if recordErr != nil {
		return uuid.Nil, recordErr
	}
	return finalRunID, nil
}

func (l runLedger) budgetWindow() time.Duration {
	window := time.Duration(l.conf.BudgetWindowMinutes) * time.Minute
	if window <= 0 {
		return 24 * time.Hour
	}
	return window
}

func (l runLedger) runRecord(runID uuid.UUID, req OpportunityRequest, output *OpportunityCandidateProposal, status, category string, latency time.Duration, usage Usage, lifecycleEvents []LifecycleEvent) RunRecord {
	outputJSON := "{}"
	if output != nil {
		outputJSON = mustJSON(output)
	}
	auditInput := opportunityAuditInput(req)
	return RunRecord{
		ID:              runID,
		TeamID:          req.TeamID,
		SiteID:          req.SiteID,
		ActorID:         req.ActorID,
		ActorType:       req.ActorType,
		Feature:         "opportunities",
		Provider:        l.conf.Provider,
		Model:           l.conf.Model,
		TemplateVersion: OpportunityTemplateVersion,
		EvidenceIDs:     evidenceIDs(auditInput.Evidence),
		InputHash:       HashAny(auditInput),
		OutputHash:      HashAny(output),
		OutputJSON:      outputJSON,
		Usage:           usage,
		LifecycleEvents: lifecycleEvents,
		Status:          status,
		ErrorCategory:   category,
		Latency:         latency,
		CreatedAt:       time.Now().UTC(),
	}
}

func opportunityAuditInput(req OpportunityRequest) OpportunityEvidenceSnapshot {
	if len(req.EvidenceSnapshot.Evidence) > 0 {
		return req.EvidenceSnapshot
	}
	return OpportunityEvidenceSnapshot{
		SiteDomain: req.DetectorInput.SiteDomain,
		From:       req.DetectorInput.From,
		To:         req.DetectorInput.To,
		Evidence:   req.DetectorInput.Evidence,
	}
}

func generationAuditFields(generation opportunityGeneration) (string, string, *OpportunityCandidateProposal) {
	if generation.Err != nil {
		return "failure", ClassifyError(generation.Err), nil
	}
	return "success", "", &generation.Output
}

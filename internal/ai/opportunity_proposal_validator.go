package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func decodeOpportunityCandidateProposalJSON(raw []byte) (OpportunityCandidateProposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var copy OpportunityCandidateProposal
	if err := decoder.Decode(&copy); err != nil {
		return OpportunityCandidateProposal{}, fmt.Errorf("%w: unsupported output field", ErrInvalidOutput)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return OpportunityCandidateProposal{}, fmt.Errorf("%w: trailing output after JSON object", ErrInvalidOutput)
	} else if !errors.Is(err, io.EOF) {
		return OpportunityCandidateProposal{}, fmt.Errorf("%w: trailing output after JSON object", ErrInvalidOutput)
	}
	return copy, nil
}

func ValidateOpportunityCandidateProposal(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	if err := validateProposalMetadata(proposal, input); err != nil {
		return err
	}
	if !allowedMessageKeys(input.MessageKeys, proposal) {
		return fmt.Errorf("%w: unsupported message key", ErrInvalidOutput)
	}
	if len(proposal.CitedEvidenceIDs) == 0 {
		return fmt.Errorf("%w: missing evidence citations", ErrInvalidOutput)
	}
	if err := validateProposalParams(proposal, input); err != nil {
		return err
	}
	return validateProposalEvidence(proposal, input)
}

func validateProposalMetadata(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	if strings.TrimSpace(proposal.TypeKey) == "" || proposal.TypeKey != input.TypeKey {
		return fmt.Errorf("%w: unsupported type key", ErrInvalidOutput)
	}
	if strings.TrimSpace(proposal.Category) == "" || proposal.Category != input.Category {
		return fmt.Errorf("%w: unsupported category", ErrInvalidOutput)
	}
	if !allowedActionType(proposal.ActionType) {
		return fmt.Errorf("%w: unsupported action type", ErrInvalidOutput)
	}
	if !allowedEffort(proposal.Effort) {
		return fmt.Errorf("%w: unsupported effort", ErrInvalidOutput)
	}
	return nil
}

func validateProposalParams(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	allowedParams := map[string]bool{}
	for _, param := range input.AllowedParams {
		if strings.TrimSpace(param) != "" {
			allowedParams[param] = true
		}
	}
	for param := range proposal.CopyParams {
		if !allowedParams[param] {
			return fmt.Errorf("%w: unsupported param %q", ErrInvalidOutput, param)
		}
	}
	if !sameJSONValue(nonNilCopyParams(proposal.CopyParams), nonNilCopyParams(input.CopyParams)) {
		return fmt.Errorf("%w: changed detector copy params", ErrInvalidOutput)
	}
	return nil
}

func validateProposalEvidence(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	allowed := map[string]bool{}
	for _, item := range input.Evidence {
		if strings.TrimSpace(item.ID) != "" {
			allowed[item.ID] = true
		}
	}
	for _, id := range proposal.CitedEvidenceIDs {
		if !allowed[id] {
			return fmt.Errorf("%w: unknown evidence id %q", ErrInvalidOutput, id)
		}
	}
	return nil
}

func allowedActionType(value string) bool {
	switch strings.TrimSpace(value) {
	case "optimize_checkout", "improve_content", "route_traffic", "fix_tracking", "investigate":
		return true
	default:
		return false
	}
}

func allowedEffort(value string) bool {
	switch strings.TrimSpace(value) {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func nonNilCopyParams(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func sameJSONValue(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
}

func allowedMessageKeys(keys OpportunityMessageKeys, copy OpportunityCandidateProposal) bool {
	return strings.TrimSpace(copy.TitleKey) != "" &&
		copy.TitleKey == keys.Title &&
		copy.SummaryKey == keys.Summary &&
		copy.ActionKey == keys.Action &&
		copy.DigestKey == keys.Digest
}

func opportunityProposalSchema(input OpportunityDetectorInput) json.RawMessage {
	paramProperties := map[string]any{}
	for _, param := range input.AllowedParams {
		if strings.TrimSpace(param) != "" {
			paramProperties[param] = map[string]any{}
		}
	}
	return mustRawJSON(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"type_key":           enumString(input.TypeKey),
			"category":           enumString(input.Category),
			"action_type":        map[string]any{"type": "string", "enum": []string{"optimize_checkout", "improve_content", "route_traffic", "fix_tracking", "investigate"}},
			"effort":             map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"title_key":          enumString(input.MessageKeys.Title),
			"summary_key":        enumString(input.MessageKeys.Summary),
			"action_key":         enumString(input.MessageKeys.Action),
			"digest_key":         enumString(input.MessageKeys.Digest),
			"copy_params":        map[string]any{"type": "object", "additionalProperties": false, "properties": paramProperties},
			"cited_evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"type_key", "category", "action_type", "effort", "title_key", "summary_key", "action_key", "digest_key", "copy_params", "cited_evidence_ids"},
	})
}

func enumString(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}

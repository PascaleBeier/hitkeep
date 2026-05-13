package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

func decodeOpportunityCandidateProposalJSON(raw []byte) (OpportunityCandidateProposal, error) {
	return decodeStrictOpportunityProposalJSON(raw)
}

func decodeOpportunityCatalogCandidateProposalJSON(raw []byte) (OpportunityCandidateProposal, error) {
	return decodeStrictOpportunityProposalJSON(raw)
}

func decodeStrictOpportunityProposalJSON(raw []byte) (OpportunityCandidateProposal, error) {
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

func ValidateOpportunityCatalogCandidateProposal(proposal OpportunityCandidateProposal, catalog OpportunityCandidateCatalog) (OpportunityDetectorInput, error) {
	input, ok := catalogCandidateForType(catalog.Candidates, proposal.TypeKey)
	if !ok {
		return OpportunityDetectorInput{}, fmt.Errorf("%w: unsupported catalog type", ErrInvalidOutput)
	}
	input.Evidence = append([]Evidence(nil), catalog.EvidenceSnapshot.Evidence...)
	if err := validateCatalogCategory(proposal.Category, catalog.AllowedCategories); err != nil {
		return OpportunityDetectorInput{}, err
	}
	if err := validateProposalMetadata(proposal, input); err != nil {
		return OpportunityDetectorInput{}, err
	}
	if !allowedMessageKeys(input.MessageKeys, proposal) {
		return OpportunityDetectorInput{}, fmt.Errorf("%w: unsupported message key", ErrInvalidOutput)
	}
	if len(proposal.CitedEvidenceIDs) == 0 {
		return OpportunityDetectorInput{}, fmt.Errorf("%w: missing evidence citations", ErrInvalidOutput)
	}
	if err := validateCatalogProposalParams(proposal, input); err != nil {
		return OpportunityDetectorInput{}, err
	}
	if err := validateProposalEvidence(proposal, input); err != nil {
		return OpportunityDetectorInput{}, err
	}
	return input, nil
}

func catalogCandidateForType(candidates []OpportunityDetectorInput, typeKey string) (OpportunityDetectorInput, bool) {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.TypeKey) == strings.TrimSpace(typeKey) && strings.TrimSpace(candidate.TypeKey) != "" {
			return candidate, true
		}
	}
	return OpportunityDetectorInput{}, false
}

func validateCatalogCategory(category string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	category = strings.TrimSpace(category)
	for _, candidate := range allowed {
		if category == strings.TrimSpace(candidate) && category != "" {
			return nil
		}
	}
	return fmt.Errorf("%w: unsupported category", ErrInvalidOutput)
}

func validateProposalMetadata(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	if strings.TrimSpace(proposal.TypeKey) == "" || proposal.TypeKey != input.TypeKey {
		return fmt.Errorf("%w: unsupported type key", ErrInvalidOutput)
	}
	if strings.TrimSpace(proposal.Category) == "" || proposal.Category != input.Category {
		return fmt.Errorf("%w: unsupported category", ErrInvalidOutput)
	}
	if !allowedActionType(proposal.ActionType, input.AllowedActionTypes) {
		return fmt.Errorf("%w: unsupported action type", ErrInvalidOutput)
	}
	if !allowedEffort(proposal.Effort) {
		return fmt.Errorf("%w: unsupported effort", ErrInvalidOutput)
	}
	return nil
}

func validateCatalogProposalParams(proposal OpportunityCandidateProposal, input OpportunityDetectorInput) error {
	allowedParams := map[string]bool{}
	for _, param := range input.AllowedParams {
		if strings.TrimSpace(param) != "" {
			allowedParams[param] = true
		}
	}
	for param := range proposal.CopyParams {
		if forbiddenOpportunityProposalParam(param) {
			return fmt.Errorf("%w: removed money/upside param %q", ErrInvalidOutput, param)
		}
		if !allowedParams[param] {
			return fmt.Errorf("%w: unsupported param %q", ErrInvalidOutput, param)
		}
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
		if forbiddenOpportunityProposalParam(param) {
			return fmt.Errorf("%w: removed money/upside param %q", ErrInvalidOutput, param)
		}
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

func forbiddenOpportunityProposalParam(param string) bool {
	switch strings.TrimSpace(param) {
	case "monthly_upside", "currency":
		return true
	default:
		return false
	}
}

func allowedActionType(value string, allowed []string) bool {
	value = strings.TrimSpace(value)
	return slices.Contains(allowedActionTypes(allowed), value)
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
			"action_type":        map[string]any{"type": "string", "enum": allowedActionTypes(input.AllowedActionTypes)},
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

func opportunityCatalogProposalSchema(catalog OpportunityCandidateCatalog) json.RawMessage {
	candidates := catalogSchemaCandidates(catalog)
	typeKeys := make([]string, 0, len(candidates))
	categories := make([]string, 0, len(candidates))
	actionTypes := []string{}
	titleKeys := make([]string, 0, len(candidates))
	summaryKeys := make([]string, 0, len(candidates))
	actionKeys := make([]string, 0, len(candidates))
	digestKeys := make([]string, 0, len(candidates))
	paramProperties := map[string]any{}
	for _, candidate := range candidates {
		typeKeys = appendUniqueString(typeKeys, candidate.TypeKey)
		categories = appendUniqueString(categories, candidate.Category)
		actionTypes = appendUniqueStrings(actionTypes, allowedActionTypes(candidate.AllowedActionTypes)...)
		titleKeys = appendUniqueString(titleKeys, candidate.MessageKeys.Title)
		summaryKeys = appendUniqueString(summaryKeys, candidate.MessageKeys.Summary)
		actionKeys = appendUniqueString(actionKeys, candidate.MessageKeys.Action)
		digestKeys = appendUniqueString(digestKeys, candidate.MessageKeys.Digest)
		for _, param := range candidate.AllowedParams {
			if strings.TrimSpace(param) != "" {
				paramProperties[param] = map[string]any{}
			}
		}
	}
	return mustRawJSON(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"type_key":           map[string]any{"type": "string", "enum": typeKeys},
			"category":           map[string]any{"type": "string", "enum": categories},
			"action_type":        map[string]any{"type": "string", "enum": actionTypes},
			"effort":             map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"title_key":          map[string]any{"type": "string", "enum": titleKeys},
			"summary_key":        map[string]any{"type": "string", "enum": summaryKeys},
			"action_key":         map[string]any{"type": "string", "enum": actionKeys},
			"digest_key":         map[string]any{"type": "string", "enum": digestKeys},
			"copy_params":        map[string]any{"type": "object", "additionalProperties": false, "properties": paramProperties},
			"cited_evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"type_key", "category", "action_type", "effort", "title_key", "summary_key", "action_key", "digest_key", "copy_params", "cited_evidence_ids"},
	})
}

func catalogSchemaCandidates(catalog OpportunityCandidateCatalog) []OpportunityDetectorInput {
	if len(catalog.AllowedCategories) == 0 {
		return catalog.Candidates
	}
	candidates := make([]OpportunityDetectorInput, 0, len(catalog.Candidates))
	for _, candidate := range catalog.Candidates {
		if validateCatalogCategory(candidate.Category, catalog.AllowedCategories) == nil {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func appendUniqueStrings(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		values = appendUniqueString(values, candidate)
	}
	return values
}

func appendUniqueString(values []string, candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return values
	}
	if slices.Contains(values, candidate) {
		return values
	}
	return append(values, candidate)
}

func allowedActionTypes(allowed []string) []string {
	values := make([]string, 0, len(allowed))
	seen := map[string]bool{}
	for _, value := range allowed {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if len(values) > 0 {
		return values
	}
	return []string{"optimize_checkout", "improve_content", "route_traffic", "fix_tracking", "investigate"}
}

func enumString(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}

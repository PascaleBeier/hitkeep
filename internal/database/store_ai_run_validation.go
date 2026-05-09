package database

import (
	"encoding/json"
	"fmt"
	"strings"
)

var rawPayloadPromptFields = map[string]bool{
	"rawprompt":    true,
	"systemprompt": true,
	"userprompt":   true,
}

var rawPayloadProviderFields = map[string]bool{
	"providerresponse":     true,
	"providerpayload":      true,
	"providererrorbody":    true,
	"providererrortext":    true,
	"rawproviderresponse":  true,
	"rawproviderpayload":   true,
	"rawprovidererrorbody": true,
	"externalerrorbody":    true,
	"rawexternalerrorbody": true,
	"rawresponse":          true,
	"rawerrorbody":         true,
	"requestpayload":       true,
	"responsepayload":      true,
}

var rawPayloadCredentialFields = map[string]bool{
	"apikey":        true,
	"authorization": true,
	"bearertoken":   true,
	"clientsecret":  true,
	"credential":    true,
	"credentials":   true,
	"secret":        true,
	"secretkey":     true,
	"token":         true,
	"accesstoken":   true,
	"refreshtoken":  true,
}

var rawPayloadValueMarkers = []string{
	"rawprompt",
	"systemprompt",
	"userprompt",
	"providerresponse",
	"providerpayload",
	"providererrorbody",
	"providererrortext",
	"rawproviderresponse",
	"rawproviderpayload",
	"rawprovidererrorbody",
	"externalerrorbody",
	"rawexternalerrorbody",
	"rawresponse",
	"rawerrorbody",
	"requestpayload",
	"responsepayload",
}

var rawPayloadCredentialValueMarkers = []string{
	"accesstoken",
	"refreshtoken",
	"clientsecret",
	"secretkey",
	"authorizationbearer",
}

func prepareAIRunStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return "success", nil
	}
	switch status {
	case "success", "failure", "reserved":
		return status, nil
	default:
		return "", fmt.Errorf("ai run status must be a stable status code")
	}
}

func prepareAIRunErrorCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return "", nil
	}
	if !isSafeAIRunErrorCategory(category) {
		return "", fmt.Errorf("ai run error category must be a stable error category")
	}
	return category, nil
}

func isSafeAIRunErrorCategory(category string) bool {
	switch category {
	case "disabled", "not_configured", "budget_exhausted", "invalid_output", "access_denied",
		"timeout", "canceled", "auth_failed", "rate_limited", "provider_error":
		return true
	default:
		return false
	}
}

func prepareAIRunLifecycleEventsJSON(events []AILifecycleEvent) ([]byte, error) {
	if events == nil {
		events = []AILifecycleEvent{}
	}
	if err := validateAIRunLifecycleEvents(events); err != nil {
		return nil, err
	}
	return json.Marshal(events)
}

func validateAIRunLifecycleEvents(events []AILifecycleEvent) error {
	for _, event := range events {
		if !isSafeAILifecycleEventType(event.Type) {
			return fmt.Errorf("ai lifecycle event type must be a stable event code")
		}
		if strings.TrimSpace(event.Status) != "" && !isSafeAILifecycleEventStatus(event.Status) {
			return fmt.Errorf("ai lifecycle event status must be a stable status code")
		}
		if strings.TrimSpace(event.ErrorCategory) != "" && !isSafeAIRunErrorCategory(event.ErrorCategory) {
			return fmt.Errorf("ai lifecycle event error category must be a stable error category")
		}
	}
	return nil
}

func isSafeAILifecycleEventType(value string) bool {
	switch strings.TrimSpace(value) {
	case "request_start", "request_finish", "tool_call_start", "tool_call_finish":
		return true
	default:
		return false
	}
}

func isSafeAILifecycleEventStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case "started", "success", "failure":
		return true
	default:
		return false
	}
}

func prepareAIRunOutputJSON(feature, output string, evidenceIDs []string) (string, error) {
	outputJSON := strings.TrimSpace(output)
	if outputJSON == "" {
		outputJSON = "{}"
	}
	if !json.Valid([]byte(outputJSON)) {
		return "", fmt.Errorf("ai run output json must be valid")
	}
	if err := validateAIRunOutputJSON(feature, outputJSON, evidenceIDs); err != nil {
		return "", err
	}
	return outputJSON, nil
}

func validateAIRunOutputJSON(feature, outputJSON string, evidenceIDs []string) error {
	var value any
	if err := json.Unmarshal([]byte(outputJSON), &value); err != nil {
		return fmt.Errorf("ai run output json must be valid")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("ai run output json must be a JSON object")
	}
	if err := rejectRawPayloadFields(value); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(feature), "opportunities") {
		return validateOpportunityAIRunOutput(object, evidenceIDs)
	}
	return nil
}

func validateOpportunityAIRunOutput(output map[string]any, evidenceIDs []string) error {
	if err := rejectOpportunityAIRunProseFields(output); err != nil {
		return err
	}
	return validateOpportunityAIRunOutputCitations(output, evidenceIDs)
}

func validateOpportunityAIRunOutputCitations(output map[string]any, evidenceIDs []string) error {
	raw, ok := output["cited_evidence_ids"]
	if !ok {
		return nil
	}
	cited, err := stringSliceFromJSONValue(raw)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(evidenceIDs))
	for _, id := range evidenceIDs {
		allowed[id] = true
	}
	for _, id := range cited {
		if !allowed[id] {
			return fmt.Errorf("opportunity ai run output cited evidence %q missing from run evidence ids", id)
		}
	}
	return nil
}

func stringSliceFromJSONValue(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("opportunity ai run output cited evidence ids must be a string array")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("opportunity ai run output cited evidence ids must be a string array")
		}
		out = append(out, text)
	}
	return out, nil
}

func rejectRawPayloadFields(value any) error {
	if err := walkAIRunOutputFields(value, func(key string) error {
		field := normalizeAIRunOutputField(key)
		if rawPayloadPromptFields[field] {
			return fmt.Errorf("must not contain raw prompt fields")
		}
		if rawPayloadProviderFields[field] {
			return fmt.Errorf("must not contain raw provider payload fields")
		}
		if rawPayloadCredentialFields[field] {
			return fmt.Errorf("must not contain credential fields")
		}
		return nil
	}); err != nil {
		return err
	}
	return rejectRawPayloadStringValues(value)
}

func rejectRawPayloadStringValues(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := rejectRawPayloadStringValues(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectRawPayloadStringValues(child); err != nil {
				return err
			}
		}
	case string:
		if containsRawPayloadMarker(typed) {
			return fmt.Errorf("must not contain raw payload values")
		}
	}
	return nil
}

func containsRawPayloadMarker(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "sk-") ||
		strings.Contains(lower, "bearer ") ||
		strings.Contains(lower, "authorization:") ||
		containsAnyCredentialAssignment(lower, []string{"api_key", "api-key", "x-api-key"}) ||
		strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "refresh_token") ||
		strings.Contains(lower, "client_secret") {
		return true
	}
	normalized := normalizeAIRunOutputField(value)
	return containsAnyRawPayloadMarker(normalized) || stringContainsAny(normalized, rawPayloadCredentialValueMarkers)
}

func containsAnyCredentialAssignment(value string, names []string) bool {
	for _, name := range names {
		for _, separator := range []string{":", "=", " "} {
			if strings.Contains(value, name+separator) {
				return true
			}
		}
	}
	return false
}

func containsAnyRawPayloadMarker(value string) bool {
	return stringContainsAny(value, rawPayloadValueMarkers)
}

func stringContainsAny(value string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func normalizeAIRunOutputField(value string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(value)))
}

func rejectOpportunityAIRunProseFields(value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for key := range object {
		switch normalizeAIRunOutputField(key) {
		case "title", "summary", "action", "nextaction", "digest":
			return fmt.Errorf("opportunity ai run output json must not contain customer prose fields")
		}
	}
	return nil
}

func walkAIRunOutputFields(value any, visit func(string) error) error {
	switch typed := value.(type) {
	case map[string]any:
		return walkAIRunOutputObject(typed, visit)
	case []any:
		return walkAIRunOutputArray(typed, visit)
	default:
		return nil
	}
}

func walkAIRunOutputObject(object map[string]any, visit func(string) error) error {
	for key, child := range object {
		if err := visit(key); err != nil {
			return err
		}
		if err := walkAIRunOutputFields(child, visit); err != nil {
			return err
		}
	}
	return nil
}

func walkAIRunOutputArray(items []any, visit func(string) error) error {
	for _, item := range items {
		if err := walkAIRunOutputFields(item, visit); err != nil {
			return err
		}
	}
	return nil
}

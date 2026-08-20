package database

import (
	"fmt"
	"math"
	"strings"

	json "hitkeep/internal/jsonapi"
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
	if strings.EqualFold(strings.TrimSpace(feature), "ask_ai") {
		return validateAskAIAIRunOutput(object, evidenceIDs)
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

var askAIAIRunTopLevelFields = map[string]bool{
	"actions":       true,
	"answer_chars":  true,
	"answer_sha256": true,
	"charts":        true,
	"citations":     true,
	"version":       true,
}

var askAIAIRunCitationFields = map[string]bool{
	"label_chars":  true,
	"label_sha256": true,
	"tool_call_id": true,
}

var askAIAIRunChartFields = map[string]bool{
	"row_count":    true,
	"series":       true,
	"series_count": true,
	"title_chars":  true,
	"title_sha256": true,
	"type":         true,
}

var askAIAIRunChartSeriesFields = map[string]bool{
	"key_chars":    true,
	"key_sha256":   true,
	"label_chars":  true,
	"label_sha256": true,
}

var askAIAIRunActionFields = map[string]bool{
	"format":        true,
	"label_chars":   true,
	"label_sha256":  true,
	"target_chars":  true,
	"target_sha256": true,
	"type":          true,
}

func validateAskAIAIRunOutput(output map[string]any, evidenceIDs []string) error {
	if len(output) == 0 {
		return nil
	}
	if err := validateObjectFields(output, askAIAIRunTopLevelFields, "ask ai run output json"); err != nil {
		return err
	}
	version, ok := output["version"].(string)
	if !ok || version != "ask-ai-output-summary-v1" {
		return fmt.Errorf("ask ai run output json must use the safe output summary version")
	}
	if err := summaryStringField(output, "answer_sha256", "ask ai run output json"); err != nil {
		return err
	}
	if err := summaryCountField(output, "answer_chars", "ask ai run output json"); err != nil {
		return err
	}
	if err := validateAskAIAIRunCitations(output, evidenceIDs); err != nil {
		return err
	}
	if err := validateAskAIAIRunCharts(output); err != nil {
		return err
	}
	return validateAskAIAIRunActions(output)
}

func validateAskAIAIRunCitations(output map[string]any, evidenceIDs []string) error {
	citations, err := arrayField(output, "citations", "ask ai run output json citations")
	if err != nil {
		return err
	}
	allowed := map[string]bool{"input_context": true}
	for _, id := range evidenceIDs {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = true
		}
	}
	for _, citation := range citations {
		object, ok := citation.(map[string]any)
		if !ok {
			return fmt.Errorf("ask ai run output json citations must contain objects")
		}
		if err := validateObjectFields(object, askAIAIRunCitationFields, "ask ai run output json citation"); err != nil {
			return err
		}
		if err := summaryStringField(object, "label_sha256", "ask ai run output json citation"); err != nil {
			return err
		}
		if err := summaryCountField(object, "label_chars", "ask ai run output json citation"); err != nil {
			return err
		}
		toolCallID, ok := object["tool_call_id"].(string)
		if !ok || strings.TrimSpace(toolCallID) == "" {
			return fmt.Errorf("ask ai run output json citation tool_call_id must be a string")
		}
		if !allowed[toolCallID] {
			return fmt.Errorf("ask ai run output citation %q missing from run evidence ids", toolCallID)
		}
	}
	return nil
}

func validateAskAIAIRunCharts(output map[string]any) error {
	charts, err := arrayField(output, "charts", "ask ai run output json charts")
	if err != nil {
		return err
	}
	for _, chart := range charts {
		object, ok := chart.(map[string]any)
		if !ok {
			return fmt.Errorf("ask ai run output json charts must contain objects")
		}
		if err := validateObjectFields(object, askAIAIRunChartFields, "ask ai run output json chart"); err != nil {
			return err
		}
		if err := summaryStringField(object, "title_sha256", "ask ai run output json chart"); err != nil {
			return err
		}
		if err := summaryCountField(object, "title_chars", "ask ai run output json chart"); err != nil {
			return err
		}
		if err := summaryCountField(object, "row_count", "ask ai run output json chart"); err != nil {
			return err
		}
		seriesCount, err := summaryCountValue(object, "series_count", "ask ai run output json chart")
		if err != nil {
			return err
		}
		chartType, ok := object["type"].(string)
		if !ok {
			return fmt.Errorf("ask ai run output json chart type must be a string")
		}
		switch strings.TrimSpace(chartType) {
		case "line", "bar", "table":
		default:
			return fmt.Errorf("ask ai run output json chart type must be a stable chart code")
		}
		series, err := arrayField(object, "series", "ask ai run output json chart series")
		if err != nil {
			return err
		}
		if seriesCount != len(series) {
			return fmt.Errorf("ask ai run output json chart series_count must match series length")
		}
		for _, item := range series {
			seriesObject, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("ask ai run output json chart series must contain objects")
			}
			if err := validateObjectFields(seriesObject, askAIAIRunChartSeriesFields, "ask ai run output json chart series"); err != nil {
				return err
			}
			if err := summaryStringField(seriesObject, "key_sha256", "ask ai run output json chart series"); err != nil {
				return err
			}
			if err := summaryCountField(seriesObject, "key_chars", "ask ai run output json chart series"); err != nil {
				return err
			}
			if err := summaryStringField(seriesObject, "label_sha256", "ask ai run output json chart series"); err != nil {
				return err
			}
			if err := summaryCountField(seriesObject, "label_chars", "ask ai run output json chart series"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAskAIAIRunActions(output map[string]any) error {
	actions, err := arrayField(output, "actions", "ask ai run output json actions")
	if err != nil {
		return err
	}
	for _, action := range actions {
		object, ok := action.(map[string]any)
		if !ok {
			return fmt.Errorf("ask ai run output json actions must contain objects")
		}
		if err := validateObjectFields(object, askAIAIRunActionFields, "ask ai run output json action"); err != nil {
			return err
		}
		if err := summaryStringField(object, "label_sha256", "ask ai run output json action"); err != nil {
			return err
		}
		if err := summaryCountField(object, "label_chars", "ask ai run output json action"); err != nil {
			return err
		}
		if err := summaryStringField(object, "target_sha256", "ask ai run output json action"); err != nil {
			return err
		}
		if err := summaryCountField(object, "target_chars", "ask ai run output json action"); err != nil {
			return err
		}
		actionType, ok := object["type"].(string)
		if !ok {
			return fmt.Errorf("ask ai run output json action type must be a string")
		}
		actionType = strings.TrimSpace(actionType)
		format, ok := object["format"].(string)
		if !ok {
			return fmt.Errorf("ask ai run output json action format must be a string")
		}
		format = strings.TrimSpace(format)
		switch actionType {
		case "navigate":
			if format != "" {
				return fmt.Errorf("ask ai run output json navigate action format must be empty")
			}
		case "download_export":
			switch format {
			case "xlsx", "json", "csv", "ndjson":
			default:
				return fmt.Errorf("ask ai run output json export action format must be stable")
			}
		default:
			return fmt.Errorf("ask ai run output json action type must be a stable action code")
		}
	}
	return nil
}

func summaryStringField(object map[string]any, key, label string) error {
	value, ok := object[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s %s must be a non-empty string", label, key)
	}
	return nil
}

func summaryCountField(object map[string]any, key, label string) error {
	_, err := summaryCountValue(object, key, label)
	return err
}

func summaryCountValue(object map[string]any, key, label string) (int, error) {
	value, ok := object[key].(float64)
	if !ok || value < 0 || value != math.Trunc(value) {
		return 0, fmt.Errorf("%s %s must be a non-negative whole number", label, key)
	}
	return int(value), nil
}

func arrayField(object map[string]any, key, label string) ([]any, error) {
	value, ok := object[key]
	if !ok {
		return nil, fmt.Errorf("%s must be an array", label)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", label)
	}
	return items, nil
}

func validateObjectFields(object map[string]any, allowed map[string]bool, label string) error {
	for key := range object {
		if !allowed[key] {
			return fmt.Errorf("%s contains unsupported field %q", label, key)
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
	if containsSecretKeyMarker(lower) ||
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

func containsSecretKeyMarker(value string) bool {
	for offset := 0; ; {
		index := strings.Index(value[offset:], "sk-")
		if index < 0 {
			return false
		}
		index += offset
		if index == 0 || !isASCIIAlphaNumeric(value[index-1]) {
			return true
		}
		offset = index + len("sk-")
	}
}

func isASCIIAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
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

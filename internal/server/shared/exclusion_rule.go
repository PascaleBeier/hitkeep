package shared

import (
	"encoding/json"
	"net/http"
	"strings"

	"hitkeep/internal/blocking"
)

const (
	ExclusionRuleTypeCIDR    = "cidr"
	ExclusionRuleTypeCountry = "country"
)

type TrafficExclusionInput struct {
	Type        string
	CIDR        string
	CountryCode string
	Description string
	Label       string
}

type trafficExclusionRequest struct {
	CIDR        string `json:"cidr"`
	Type        string `json:"type"`
	CountryCode string `json:"country_code"`
	Description string `json:"description"`
}

func DecodeTrafficExclusionRequest(r *http.Request) (TrafficExclusionInput, string, int, bool) {
	req, message, status, ok := decodeTrafficExclusionJSON(r)
	if !ok {
		return TrafficExclusionInput{}, message, status, false
	}

	input, message, status, ok := normalizeTrafficExclusionInput(req)
	if !ok {
		return TrafficExclusionInput{}, message, status, false
	}
	return input, "", 0, true
}

func decodeTrafficExclusionJSON(r *http.Request) (trafficExclusionRequest, string, int, bool) {
	var req trafficExclusionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return trafficExclusionRequest{}, "Invalid request body", http.StatusBadRequest, false
	}
	return req, "", 0, true
}

func normalizeTrafficExclusionInput(req trafficExclusionRequest) (TrafficExclusionInput, string, int, bool) {
	description, message, status, ok := normalizeExclusionDescription(req.Description)
	if !ok {
		return TrafficExclusionInput{}, message, status, false
	}

	ruleType := trafficExclusionRuleType(req.Type, req.CIDR)
	input := TrafficExclusionInput{Type: ruleType, Description: description}
	return normalizeExclusionRuleValue(input, req)
}

func normalizeExclusionDescription(description string) (string, string, int, bool) {
	description = strings.TrimSpace(description)
	if len(description) > 255 {
		return "", "Description must be 255 characters or fewer", http.StatusBadRequest, false
	}
	return description, "", 0, true
}

func trafficExclusionRuleType(ruleType string, cidr string) string {
	ruleType = strings.ToLower(strings.TrimSpace(ruleType))
	if ruleType == "" && strings.TrimSpace(cidr) != "" {
		return ExclusionRuleTypeCIDR
	}
	return ruleType
}

func normalizeExclusionRuleValue(input TrafficExclusionInput, req trafficExclusionRequest) (TrafficExclusionInput, string, int, bool) {
	if input.Type == ExclusionRuleTypeCIDR {
		return normalizeCIDRExclusionInput(input, req.CIDR)
	}
	if input.Type == ExclusionRuleTypeCountry {
		return normalizeCountryExclusionInput(input, req.CountryCode)
	}
	return TrafficExclusionInput{}, "Invalid exclusion type", http.StatusBadRequest, false
}

func normalizeCIDRExclusionInput(input TrafficExclusionInput, cidrValue string) (TrafficExclusionInput, string, int, bool) {
	cidr, _, err := blocking.NormalizeCIDR(cidrValue)
	if err != nil {
		return TrafficExclusionInput{}, "Invalid IP or CIDR", http.StatusBadRequest, false
	}
	input.CIDR = cidr
	input.Label = cidr
	return input, "", 0, true
}

func normalizeCountryExclusionInput(input TrafficExclusionInput, countryCodeValue string) (TrafficExclusionInput, string, int, bool) {
	countryCode := NormalizeCountryCode(countryCodeValue)
	if countryCode == "" {
		return TrafficExclusionInput{}, "Invalid country code", http.StatusBadRequest, false
	}
	input.CountryCode = countryCode
	input.Label = countryCode
	return input, "", 0, true
}

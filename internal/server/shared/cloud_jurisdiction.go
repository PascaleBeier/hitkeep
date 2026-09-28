package shared

import "strings"

// NormalizeCloudJurisdiction maps a configured region to its signup jurisdiction.
func NormalizeCloudJurisdiction(value string) string {
	jurisdiction := strings.TrimSpace(strings.ToUpper(value))
	if strings.HasPrefix(jurisdiction, "EU") {
		return "EU"
	}
	if strings.HasPrefix(jurisdiction, "US") {
		return "US"
	}
	return jurisdiction
}

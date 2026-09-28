package webhooks

import (
	"strconv"
	"strings"
)

func MinorAPIVersion(runtimeVersion string) string {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(runtimeVersion), "v"), ".")
	if len(parts) < 2 {
		return "0.0"
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return "0.0"
	}
	minorPart := parts[1]
	if cut := strings.IndexAny(minorPart, "-+"); cut >= 0 {
		minorPart = minorPart[:cut]
	}
	minor, err := strconv.Atoi(minorPart)
	if err != nil || minor < 0 {
		return "0.0"
	}
	return strconv.Itoa(major) + "." + strconv.Itoa(minor)
}

package opportunities

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func safeJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func stableOpportunityID(siteID uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(siteID, []byte("hitkeep:opportunity:"+key))
}

func confidence(high bool) string {
	if high {
		return "high"
	}
	return "medium"
}

func topMetricName(items []api.MetricStat, fallback string) string {
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			return item.Name
		}
	}
	return fallback
}

func formatRatePercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func clampScore(score int) int {
	if score < 1 {
		return 1
	}
	if score > 99 {
		return 99
	}
	return score
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func copyOpportunityMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = copyOpportunityValue(item)
	}
	return out
}

func copyOpportunityValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyOpportunityMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyOpportunityValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	default:
		return value
	}
}

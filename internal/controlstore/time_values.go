package controlstore

import (
	"fmt"
	"strings"
	"time"
)

// parseSQLiteTime normalizes values returned by expressions and aggregates.
// SQLite loses a column's declared TIMESTAMP type through COALESCE/MAX, so the
// driver may return text even though direct column scans return time.Time.
func parseSQLiteTime(value any) (time.Time, error) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return typed.UTC(), nil
	case string:
		return parseSQLiteTimeString(typed)
	case []byte:
		return parseSQLiteTimeString(string(typed))
	default:
		return time.Time{}, fmt.Errorf("unsupported SQLite timestamp type %T", value)
	}
}

func parseSQLiteTimeString(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid SQLite timestamp %q", value)
}

package database

import "time"

func kindToTruncUnit(kind rollupKind) string {
	switch kind {
	case rollupHourly:
		return "hour"
	case rollupDaily:
		return "day"
	case rollupMonthly:
		return "month"
	default:
		return "day"
	}
}
func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func monthOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, value.Location())
}

func truncUnitForRange(start time.Time, end time.Time) string {
	duration := end.Sub(start)
	switch {
	case duration <= time.Hour:
		return "5minute"
	case duration <= 6*time.Hour:
		return "15minute"
	case duration < 48*time.Hour:
		return "hour"
	case duration >= 365*24*time.Hour:
		return "month"
	default:
		return "day"
	}
}

func addUnit(value time.Time, truncUnit string, delta int) time.Time {
	switch truncUnit {
	case "5minute":
		return value.Add(time.Duration(delta) * 5 * time.Minute)
	case "15minute":
		return value.Add(time.Duration(delta) * 15 * time.Minute)
	case "hour":
		return value.Add(time.Duration(delta) * time.Hour)
	case "6hour":
		return value.Add(time.Duration(delta) * 6 * time.Hour)
	case "day":
		return value.AddDate(0, 0, delta)
	case "month":
		return value.AddDate(0, delta, 0)
	default:
		return value
	}
}

func truncToUnit(value time.Time, truncUnit string) time.Time {
	switch truncUnit {
	case "5minute":
		return value.Truncate(5 * time.Minute)
	case "15minute":
		return value.Truncate(15 * time.Minute)
	case "hour":
		return value.Truncate(time.Hour)
	case "6hour":
		return value.Truncate(6 * time.Hour)
	case "day":
		return dateOnly(value)
	case "month":
		return monthOnly(value)
	default:
		return value
	}
}

func isAlignedToUnit(value time.Time, truncUnit string) bool {
	return value.Equal(truncToUnit(value, truncUnit))
}

type rollupWindow struct {
	FullStart time.Time
	FullEnd   time.Time
	Leading   *time.Time
	Trailing  *time.Time
	UseRollup bool
}

func buildRollupWindow(start time.Time, end time.Time, truncUnit string) rollupWindow {
	if !canUseRollupsForTruncUnit(truncUnit) {
		leadEnd := end
		return rollupWindow{
			Leading:   &leadEnd,
			UseRollup: false,
		}
	}

	startBucket := truncToUnit(start, truncUnit)
	endBucket := truncToUnit(end, truncUnit)

	fullStart := startBucket
	if !isAlignedToUnit(start, truncUnit) {
		fullStart = addUnit(startBucket, truncUnit, 1)
	}

	fullEnd := addUnit(endBucket, truncUnit, -1)

	if fullStart.After(end) || fullEnd.Before(fullStart) {
		leadEnd := end
		return rollupWindow{
			Leading:   &leadEnd,
			UseRollup: false,
		}
	}

	window := rollupWindow{
		FullStart: fullStart,
		FullEnd:   fullEnd,
		UseRollup: true,
	}

	if start.Before(fullStart) {
		leadEnd := fullStart
		window.Leading = &leadEnd
	}

	trailingStart := addUnit(fullEnd, truncUnit, 1)
	if end.After(trailingStart) {
		window.Trailing = &trailingStart
	}

	return window
}

func buildSeriesBuckets(start time.Time, end time.Time, truncUnit string) []time.Time {
	if end.Before(start) {
		return nil
	}

	cursor := truncToUnit(start, truncUnit)
	last := truncToUnit(end, truncUnit)
	var buckets []time.Time
	for !cursor.After(last) {
		buckets = append(buckets, cursor)
		cursor = addUnit(cursor, truncUnit, 1)
	}
	return buckets
}

func canUseRollupsForTruncUnit(truncUnit string) bool {
	return truncUnit == "hour" || truncUnit == "day" || truncUnit == "month"
}

func bucketIntervalSQL(truncUnit string) string {
	switch truncUnit {
	case "5minute":
		return "INTERVAL '5 minutes'"
	case "15minute":
		return "INTERVAL '15 minutes'"
	case "hour":
		return "INTERVAL '1 hour'"
	case "6hour":
		return "INTERVAL '6 hours'"
	case "month":
		return "INTERVAL '1 month'"
	default:
		return "INTERVAL '1 day'"
	}
}

func bucketSQL(column string, truncUnit string) string {
	switch truncUnit {
	case "5minute":
		return "time_bucket(INTERVAL '5 minutes', " + column + ")"
	case "15minute":
		return "time_bucket(INTERVAL '15 minutes', " + column + ")"
	case "6hour":
		return "time_bucket(INTERVAL '6 hours', " + column + ")"
	case "month":
		return "date_trunc('month', " + column + ")"
	case "hour":
		return "date_trunc('hour', " + column + ")"
	default:
		return "date_trunc('day', " + column + ")"
	}
}

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
	if duration < 48*time.Hour {
		return "hour"
	}
	if duration >= 180*24*time.Hour {
		return "month"
	}
	return "day"
}

func addUnit(value time.Time, truncUnit string, delta int) time.Time {
	switch truncUnit {
	case "hour":
		return value.Add(time.Duration(delta) * time.Hour)
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
	case "hour":
		return value.Truncate(time.Hour)
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

package reporting

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/api"
)

var ErrInvalidSchedule = errors.New("invalid report schedule")

func ValidateSchedule(schedule api.ReportSchedule) error {
	if _, err := time.LoadLocation(strings.TrimSpace(schedule.Timezone)); err != nil {
		return fmt.Errorf("%w: timezone", ErrInvalidSchedule)
	}
	hour, minute, err := parseLocalTime(schedule.LocalTime)
	if err != nil || minute%15 != 0 || hour < 0 {
		return fmt.Errorf("%w: local time", ErrInvalidSchedule)
	}
	switch schedule.Frequency {
	case api.ReportFrequencyDaily:
		if schedule.WeeklyDay != nil || schedule.MonthlyDay != nil {
			return fmt.Errorf("%w: daily anchor", ErrInvalidSchedule)
		}
	case api.ReportFrequencyWeekly:
		if schedule.WeeklyDay == nil || *schedule.WeeklyDay < 0 || *schedule.WeeklyDay > 6 || schedule.MonthlyDay != nil {
			return fmt.Errorf("%w: weekly anchor", ErrInvalidSchedule)
		}
	case api.ReportFrequencyMonthly:
		if schedule.MonthlyDay == nil || *schedule.MonthlyDay < 1 || *schedule.MonthlyDay > 28 || schedule.WeeklyDay != nil {
			return fmt.Errorf("%w: monthly anchor", ErrInvalidSchedule)
		}
	default:
		return fmt.Errorf("%w: frequency", ErrInvalidSchedule)
	}
	return nil
}

// NextOccurrence returns the first scheduled instant strictly after after.
// Ambiguous fall-back times use the first occurrence. Non-existent spring-
// forward times move to the first valid local minute after the requested time.
func NextOccurrence(schedule api.ReportSchedule, after time.Time) (time.Time, error) {
	if err := ValidateSchedule(schedule); err != nil {
		return time.Time{}, err
	}
	loc, _ := time.LoadLocation(schedule.Timezone)
	hour, minute, _ := parseLocalTime(schedule.LocalTime)
	localAfter := after.In(loc)
	date := time.Date(localAfter.Year(), localAfter.Month(), localAfter.Day(), 0, 0, 0, 0, loc)

	for range 400 {
		if scheduleMatchesDate(schedule, date) {
			candidate := localOccurrence(date, hour, minute, loc)
			if candidate.After(after) {
				return candidate.UTC(), nil
			}
		}
		date = date.AddDate(0, 0, 1)
	}
	return time.Time{}, fmt.Errorf("%w: no occurrence", ErrInvalidSchedule)
}

func scheduleMatchesDate(schedule api.ReportSchedule, date time.Time) bool {
	switch schedule.Frequency {
	case api.ReportFrequencyDaily:
		return true
	case api.ReportFrequencyWeekly:
		return int(date.Weekday()) == *schedule.WeeklyDay
	case api.ReportFrequencyMonthly:
		return date.Day() == *schedule.MonthlyDay
	default:
		return false
	}
}

func localOccurrence(date time.Time, hour, minute int, loc *time.Location) time.Time {
	desiredMinutes := hour*60 + minute
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc).Add(-3 * time.Hour)
	end := start.Add(30 * time.Hour)
	var firstAfter time.Time
	for instant := start; instant.Before(end); instant = instant.Add(time.Minute) {
		local := instant.In(loc)
		if local.Year() != date.Year() || local.Month() != date.Month() || local.Day() != date.Day() {
			continue
		}
		localMinutes := local.Hour()*60 + local.Minute()
		if localMinutes == desiredMinutes {
			return instant
		}
		if localMinutes > desiredMinutes && firstAfter.IsZero() {
			firstAfter = instant
		}
	}
	return firstAfter
}

// PeriodBounds returns the completed reporting period preceding scheduledFor.
// The boundaries are calculated in the report timezone and returned as UTC.
func PeriodBounds(schedule api.ReportSchedule, scheduledFor time.Time) (start, end, previousStart, previousEnd time.Time, err error) {
	if err = ValidateSchedule(schedule); err != nil {
		return
	}
	loc, _ := time.LoadLocation(schedule.Timezone)
	local := scheduledFor.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)

	switch schedule.Frequency {
	case api.ReportFrequencyDaily:
		end = day
		start = day.AddDate(0, 0, -1)
		previousEnd = start
		previousStart = start.AddDate(0, 0, -1)
	case api.ReportFrequencyWeekly:
		end = day
		start = day.AddDate(0, 0, -7)
		previousEnd = start
		previousStart = start.AddDate(0, 0, -7)
	case api.ReportFrequencyMonthly:
		end = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, loc)
		start = end.AddDate(0, -1, 0)
		previousEnd = start
		previousStart = start.AddDate(0, -1, 0)
	}
	start, end = start.UTC(), end.UTC()
	previousStart, previousEnd = previousStart.UTC(), previousEnd.UTC()
	return
}

func CatchUpWindow(frequency api.ReportFrequency) time.Duration {
	switch frequency {
	case api.ReportFrequencyDaily:
		return 48 * time.Hour
	case api.ReportFrequencyWeekly:
		return 7 * 24 * time.Hour
	case api.ReportFrequencyMonthly:
		return 14 * 24 * time.Hour
	default:
		return 0
	}
}

func parseLocalTime(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, 0, ErrInvalidSchedule
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, ErrInvalidSchedule
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, ErrInvalidSchedule
	}
	return hour, minute, nil
}

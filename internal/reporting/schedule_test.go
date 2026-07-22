package reporting

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestValidateScheduleRequiresQuarterHourAndAnchors(t *testing.T) {
	weeklyDay := 1
	monthlyDay := 1
	tests := []struct {
		name     string
		schedule api.ReportSchedule
		valid    bool
	}{
		{name: "daily default", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:00"}, valid: true},
		{name: "daily quarter hour", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "America/New_York", LocalTime: "17:45"}, valid: true},
		{name: "minute off grid", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "UTC", LocalTime: "08:10"}},
		{name: "unknown timezone", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Mars/Olympus", LocalTime: "08:00"}},
		{name: "weekly anchor", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: "08:00", WeeklyDay: &weeklyDay}, valid: true},
		{name: "weekly without anchor", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: "08:00"}},
		{name: "monthly anchor", schedule: api.ReportSchedule{Frequency: api.ReportFrequencyMonthly, Timezone: "UTC", LocalTime: "08:00", MonthlyDay: &monthlyDay}, valid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSchedule(test.schedule)
			if test.valid && err != nil {
				t.Fatalf("ValidateSchedule() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidSchedule) {
				t.Fatalf("ValidateSchedule() error = %v, want ErrInvalidSchedule", err)
			}
		})
	}
}

func TestNextOccurrencePreservesLocalTimeAcrossDST(t *testing.T) {
	schedule := api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:00"}

	beforeSpring := time.Date(2026, 3, 28, 8, 0, 0, 0, time.UTC)
	spring, err := NextOccurrence(schedule, beforeSpring)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 29, 6, 0, 0, 0, time.UTC); !spring.Equal(want) {
		t.Fatalf("spring occurrence = %s, want %s", spring, want)
	}

	beforeFall := time.Date(2026, 10, 24, 7, 0, 0, 0, time.UTC)
	fall, err := NextOccurrence(schedule, beforeFall)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC); !fall.Equal(want) {
		t.Fatalf("fall occurrence = %s, want %s", fall, want)
	}
}

func TestNextOccurrenceHandlesDSTGapAndOverlap(t *testing.T) {
	gap := api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "02:30"}
	got, err := NextOccurrence(gap, time.Date(2026, 3, 28, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("gap occurrence = %s, want first valid local minute %s", got, want)
	}

	overlap := api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "02:30"}
	got, err = NextOccurrence(overlap, time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("overlap occurrence = %s, want first occurrence %s", got, want)
	}
}

func TestNextOccurrenceWeeklyAndMonthlyAnchors(t *testing.T) {
	monday := 1
	first := 1
	after := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	weekly, err := NextOccurrence(api.ReportSchedule{Frequency: api.ReportFrequencyWeekly, Timezone: "UTC", LocalTime: "08:00", WeeklyDay: &monday}, after)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC); !weekly.Equal(want) {
		t.Fatalf("weekly occurrence = %s, want %s", weekly, want)
	}

	monthly, err := NextOccurrence(api.ReportSchedule{Frequency: api.ReportFrequencyMonthly, Timezone: "UTC", LocalTime: "08:00", MonthlyDay: &first}, after)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC); !monthly.Equal(want) {
		t.Fatalf("monthly occurrence = %s, want %s", monthly, want)
	}
}

func TestPeriodBoundsUseReportTimezone(t *testing.T) {
	schedule := api.ReportSchedule{Frequency: api.ReportFrequencyDaily, Timezone: "Europe/Berlin", LocalTime: "08:00"}
	start, end, previousStart, previousEnd, err := PeriodBounds(schedule, time.Date(2026, 3, 29, 6, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	if want := time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("end = %s, want %s", end, want)
	}
	if !previousEnd.Equal(start) || previousStart.IsZero() {
		t.Fatalf("unexpected previous period %s..%s", previousStart, previousEnd)
	}
}

func TestUnsubscribeTokenRoundTripAndTamperResistance(t *testing.T) {
	reportID := uuid.New()
	userID := uuid.New()
	token := UnsubscribeToken("test-secret", reportID, userID)
	gotReportID, gotUserID, ok := VerifyUnsubscribeToken("test-secret", token)
	if !ok || gotReportID != reportID || gotUserID != userID {
		t.Fatalf("VerifyUnsubscribeToken() = %s, %s, %v", gotReportID, gotUserID, ok)
	}
	if _, _, ok := VerifyUnsubscribeToken("wrong-secret", token); ok {
		t.Fatal("token verified with the wrong secret")
	}
	if UnsubscribeTokenHash(token) == token {
		t.Fatal("stored token hash must not equal the opaque token")
	}
}

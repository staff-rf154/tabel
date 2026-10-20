package repo

import "testing"

func datePtr(value string) *string { return &value }

func TestChooseActiveSchedule(t *testing.T) {
	inputs := []ScheduleInput{
		{ValidFrom: "2026-01-01", ValidTo: datePtr("2026-06-30")},
		{ValidFrom: "2026-07-01", ValidTo: datePtr("2026-12-31")},
		{ValidFrom: "2027-01-01"},
	}
	if got := chooseActiveSchedule(inputs, "2026-10-19"); got != 1 {
		t.Fatalf("expected covering row 1, got %d", got)
	}
	if got := chooseActiveSchedule(inputs, "2025-01-01"); got != 0 {
		t.Fatalf("expected earliest row 0 for past, got %d", got)
	}
	if got := chooseActiveSchedule(inputs, "2028-01-01"); got != 2 {
		t.Fatalf("expected latest past row 2, got %d", got)
	}
	if got := chooseActiveSchedule([]ScheduleInput{{ValidFrom: "2027-01-01"}, {ValidFrom: "2027-06-01"}}, "2026-01-01"); got != 0 {
		t.Fatalf("expected earliest future row 0, got %d", got)
	}
}

func TestChooseActiveSchedulePrefersLatestCovering(t *testing.T) {
	inputs := []ScheduleInput{
		{ValidFrom: "2026-01-01"},
		{ValidFrom: "2026-06-01", ValidTo: datePtr("2026-12-31")},
	}
	if got := chooseActiveSchedule(inputs, "2026-10-19"); got != 1 {
		t.Fatalf("expected latest covering row 1, got %d", got)
	}
}

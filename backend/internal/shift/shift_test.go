package shift

import (
	"testing"
	"time"

	"tabel/internal/repo"
)

func schedule(start, end string, workdays ...int) repo.Schedule {
	return repo.Schedule{
		EmployeeID:  "00000000-0000-0000-0000-000000000001",
		PatternType: "custom",
		Workdays:    workdays,
		StartTime:   start,
		EndTime:     end,
		ValidFrom:   "2026-01-01",
		IsActive:    true,
	}
}

func msk(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, NovosibirskZone)
}

func TestDefaultSchedule(t *testing.T) {
	s := DefaultSchedule("employee", msk(2026, time.October, 19, 12, 0))
	if s.PatternType != "5/2" || s.StartTime != "09:00" || s.EndTime != "18:00" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	if len(s.Workdays) != 5 || s.Workdays[0] != 1 || s.Workdays[4] != 5 {
		t.Fatalf("unexpected workdays: %v", s.Workdays)
	}
}

func TestEvaluateDayShift(t *testing.T) {
	s := schedule("09:00", "18:00", 1, 2, 3, 4, 5)
	at := msk(2026, time.October, 19, 10, 0)
	evaluation, err := Evaluate(s, s, at)
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Within {
		t.Fatal("expected within working window")
	}
	if evaluation.Start != msk(2026, time.October, 19, 9, 0) || evaluation.End != msk(2026, time.October, 19, 18, 0) {
		t.Fatalf("unexpected window: %s - %s", evaluation.Start, evaluation.End)
	}
}

func TestEvaluateAfterShift(t *testing.T) {
	s := schedule("09:00", "18:00", 1, 2, 3, 4, 5)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 19, 20, 0))
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Within {
		t.Fatal("expected outside working window")
	}
}

func TestEvaluateWeekend(t *testing.T) {
	s := schedule("09:00", "18:00", 1, 2, 3, 4, 5)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 17, 10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Within {
		t.Fatal("expected non-workday to be outside")
	}
	if evaluation.Date != msk(2026, time.October, 17, 0, 0) {
		t.Fatalf("unexpected date: %s", evaluation.Date)
	}
}

func TestEvaluateOvernightStart(t *testing.T) {
	s := schedule("22:00", "06:00", 1)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 19, 23, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Within {
		t.Fatal("expected within overnight shift")
	}
	if evaluation.End != msk(2026, time.October, 20, 6, 0) {
		t.Fatalf("unexpected end: %s", evaluation.End)
	}
}

func TestEvaluateOvernightTail(t *testing.T) {
	s := schedule("22:00", "06:00", 1)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 20, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Within {
		t.Fatal("expected within previous day's overnight shift")
	}
	if evaluation.Date != msk(2026, time.October, 19, 0, 0) {
		t.Fatalf("unexpected date: %s", evaluation.Date)
	}
}

func TestEvaluateOvernightTailAfterEnd(t *testing.T) {
	s := schedule("22:00", "06:00", 1)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 20, 7, 0))
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Within {
		t.Fatal("expected outside after overnight end")
	}
}

func TestEvaluateOvernightNonWorkday(t *testing.T) {
	s := schedule("22:00", "06:00", 1)
	evaluation, err := Evaluate(s, s, msk(2026, time.October, 18, 23, 0))
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Within {
		t.Fatal("expected Sunday overnight to be outside")
	}
}

func TestWindowEnd(t *testing.T) {
	end, err := WindowEnd(schedule("22:00", "06:00"), msk(2026, time.October, 19, 12, 0))
	if err != nil {
		t.Fatal(err)
	}
	if end != msk(2026, time.October, 20, 6, 0) {
		t.Fatalf("unexpected end: %s", end)
	}
}

func TestParseClock(t *testing.T) {
	for _, value := range []string{"09:00", "9:05", "23:59", "00:00:30"} {
		if _, err := ParseClock(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"", "24:00", "10:60", "abc", "10"} {
		if _, err := ParseClock(value); err == nil {
			t.Fatalf("%s: expected error", value)
		}
	}
}

func TestNormalizeClock(t *testing.T) {
	value, err := NormalizeClock("9:5")
	if err != nil || value != "09:05" {
		t.Fatalf("unexpected result: %q, %v", value, err)
	}
}

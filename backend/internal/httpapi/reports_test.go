package httpapi

import (
	"testing"
	"time"
)

func TestReportDatesNovosibirsk(t *testing.T) {
	from, _ := time.Parse(time.RFC3339, "2026-10-01T00:00:00+07:00")
	to, _ := time.Parse(time.RFC3339, "2026-10-03T23:59:59+07:00")
	dates := reportDates(from, to)
	if len(dates) != 3 || dates[0] != "2026-10-01" || dates[2] != "2026-10-03" {
		t.Fatalf("unexpected dates: %v", dates)
	}
}

func TestReportHours(t *testing.T) {
	cases := map[int]string{0: "", 480: "8", 510: "8,50", 90: "1,50"}
	for minutes, want := range cases {
		if got := reportHours(minutes); got != want {
			t.Fatalf("reportHours(%d) = %q, want %q", minutes, got, want)
		}
	}
}

package httpapi

import (
	"strings"
	"testing"
)

func TestParseScheduleCSVDefaults(t *testing.T) {
	result := parseScheduleCSV("s.csv", []byte("employee_number\n0001\n"), "2026-10-19")
	if result.err != "" {
		t.Fatal(result.err)
	}
	if len(result.rows) != 1 {
		t.Fatalf("unexpected rows: %d", len(result.rows))
	}
	row := result.rows[0]
	if row.Err != "" {
		t.Fatal(row.Err)
	}
	if row.PatternType != "5/2" || row.StartTime != "09:00" || row.EndTime != "18:00" {
		t.Fatalf("unexpected defaults: %+v", row)
	}
	if len(row.Workdays) != 5 {
		t.Fatalf("unexpected workdays: %v", row.Workdays)
	}
	if row.ValidFrom != "2026-10-19" || row.ValidTo != nil {
		t.Fatalf("unexpected validity: %s %v", row.ValidFrom, row.ValidTo)
	}
}

func TestParseScheduleCSVQuotedWorkdays(t *testing.T) {
	data := "employee_number,workdays,start_time,end_time\n0001,\"1,3,5\",08:30,17:45\n"
	result := parseScheduleCSV("s.csv", []byte(data), "2026-10-19")
	if result.err != "" {
		t.Fatal(result.err)
	}
	row := result.rows[0]
	if row.Err != "" {
		t.Fatal(row.Err)
	}
	if len(row.Workdays) != 3 || row.Workdays[0] != 1 || row.Workdays[2] != 5 {
		t.Fatalf("unexpected workdays: %v", row.Workdays)
	}
	if row.StartTime != "08:30" || row.EndTime != "17:45" {
		t.Fatalf("unexpected times: %s/%s", row.StartTime, row.EndTime)
	}
}

func TestParseScheduleCSVAliasesAndBOM(t *testing.T) {
	data := "\xef\xbb\xbfemployee_number,pattern_type,workdays,date_from,date_to\n0001,custom,1-5,2026-01-01,2026-12-31\n"
	result := parseScheduleCSV("s.csv", []byte(data), "2026-10-19")
	if result.err != "" {
		t.Fatal(result.err)
	}
	row := result.rows[0]
	if row.Err != "" {
		t.Fatal(row.Err)
	}
	if row.ValidFrom != "2026-01-01" || row.ValidTo == nil || *row.ValidTo != "2026-12-31" {
		t.Fatalf("unexpected validity: %s %v", row.ValidFrom, row.ValidTo)
	}
}

func TestParseScheduleCSVInvalidRows(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"workdays", "employee_number,workdays\n0001,8\n", "некорректный день"},
		{"time", "employee_number,start_time\n0001,25:00\n", "некорректный час"},
		{"date", "employee_number,valid_from\n0001,19.10.2026\n", "YYYY-MM-DD"},
		{"pattern", "employee_number,pattern_type\n0001,7/7\n", "неизвестный тип"},
		{"validity", "employee_number,valid_from,valid_to\n0001,2026-10-19,2026-10-18\n", "раньше valid_from"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseScheduleCSV("s.csv", []byte(tc.data), "2026-10-19")
			if result.err != "" {
				t.Fatal(result.err)
			}
			if len(result.rows) != 1 || !strings.Contains(result.rows[0].Err, tc.want) {
				t.Fatalf("expected %q in %q", tc.want, result.rows[0].Err)
			}
		})
	}
}

func TestParseScheduleCSVMissingHeader(t *testing.T) {
	result := parseScheduleCSV("s.csv", []byte("number\n0001\n"), "2026-10-19")
	if !strings.Contains(result.err, "employee_number") {
		t.Fatalf("unexpected error: %q", result.err)
	}
}

func TestMarkScheduleOverlaps(t *testing.T) {
	end := "2026-06-30"
	rows := []scheduleCSVRow{
		{Line: 2, EmployeeNumber: "0001", ValidFrom: "2026-01-01", ValidTo: &end},
		{Line: 3, EmployeeNumber: "0001", ValidFrom: "2026-06-30"},
		{Line: 4, EmployeeNumber: "0002", ValidFrom: "2026-01-01"},
	}
	markScheduleOverlaps(rows)
	if rows[1].Err == "" || !strings.Contains(rows[1].Err, "строкой 2") {
		t.Fatalf("expected overlap error, got %q", rows[1].Err)
	}
	if rows[0].Err != "" || rows[2].Err != "" {
		t.Fatalf("unexpected errors: %q %q", rows[0].Err, rows[2].Err)
	}
}

func TestMarkScheduleOverlapsAllowsAdjacent(t *testing.T) {
	end := "2026-06-30"
	rows := []scheduleCSVRow{
		{Line: 2, EmployeeNumber: "0001", ValidFrom: "2026-01-01", ValidTo: &end},
		{Line: 3, EmployeeNumber: "0001", ValidFrom: "2026-07-01"},
	}
	markScheduleOverlaps(rows)
	if rows[0].Err != "" || rows[1].Err != "" {
		t.Fatalf("unexpected errors: %q %q", rows[0].Err, rows[1].Err)
	}
}

package repo

import "testing"

func TestBuildTimesheetGroupsByEmployee(t *testing.T) {
	position := "Оператор"
	department := "Отдел продаж"
	items := []ReportDaily{
		{EmployeeID: "b", EmployeeNumber: "2", FullName: "Б", Position: &position, Department: &department, WorkDate: "2026-10-02", WorkedMinutes: 480},
		{EmployeeID: "a", EmployeeNumber: "1", FullName: "А", WorkDate: "2026-10-01", WorkedMinutes: 510},
		{EmployeeID: "b", EmployeeNumber: "2", FullName: "Б", WorkDate: "2026-10-03", WorkedMinutes: 0, OvertimeShifts: 1},
	}
	result := BuildTimesheet(items)
	if len(result) != 2 {
		t.Fatalf("unexpected employees: %d", len(result))
	}
	if result[0].EmployeeID != "b" || result[1].EmployeeID != "a" {
		t.Fatalf("employee order changed: %+v", result)
	}
	if result[0].DaysWorked != 2 || result[0].WorkedMinutes != 480 || result[0].OvertimeShifts != 1 {
		t.Fatalf("unexpected totals: %+v", result[0])
	}
	if result[0].Days[0].Code != "Я" || len(result[0].Days) != 2 {
		t.Fatalf("unexpected days: %+v", result[0].Days)
	}
}

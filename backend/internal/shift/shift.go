// Package shift содержит правила определения смены, графика и переработки.
package shift

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tabel/internal/repo"
)

const novosibirskOffset = 7 * 60 * 60

var NovosibirskZone = time.FixedZone("NOV", novosibirskOffset)

// Evaluation — результат сопоставления момента времени с графиком.
type Evaluation struct {
	Schedule repo.Schedule
	Within   bool
	Date     time.Time // локальная дата начала окна смены
	Start    time.Time // начало окна (UTC)
	End      time.Time // конец окна (UTC)
}

// DefaultSchedule возвращает график по умолчанию: 5/2, 09:00–18:00.
func DefaultSchedule(employeeID string, date time.Time) repo.Schedule {
	return repo.Schedule{
		EmployeeID:  employeeID,
		PatternType: "5/2",
		Workdays:    []int{1, 2, 3, 4, 5},
		StartTime:   "09:00",
		EndTime:     "18:00",
		ValidFrom:   date.In(NovosibirskZone).Format("2006-01-02"),
		IsActive:    true,
	}
}

// Evaluate определяет, попадает ли момент ts в рабочее окно. today — график,
// действующий на локальную дату ts; yesterday — на предыдущую дату (нужен для
// ночных смен, переходящих через полночь).
func Evaluate(today, yesterday repo.Schedule, ts time.Time) (Evaluation, error) {
	local := ts.In(NovosibirskZone)
	todayDate := localDate(local)
	yesterdayDate := todayDate.AddDate(0, 0, -1)

	todayStart, todayEnd, todayOvernight, err := window(today, todayDate)
	if err != nil {
		return Evaluation{}, err
	}
	yStart, yEnd, yOvernight, err := window(yesterday, yesterdayDate)
	if err != nil {
		return Evaluation{}, err
	}

	if todayOvernight {
		if !local.Before(todayStart) {
			return Evaluation{
				Schedule: today,
				Within:   isWorkday(today, todayDate),
				Date:     todayDate,
				Start:    todayStart,
				End:      todayEnd,
			}, nil
		}
		if yOvernight && !local.After(yEnd) {
			return Evaluation{
				Schedule: yesterday,
				Within:   isWorkday(yesterday, yesterdayDate),
				Date:     yesterdayDate,
				Start:    yStart,
				End:      yEnd,
			}, nil
		}
		return Evaluation{
			Schedule: today,
			Within:   false,
			Date:     todayDate,
			Start:    todayStart,
			End:      todayEnd,
		}, nil
	}

	if !local.Before(todayStart) && !local.After(todayEnd) {
		return Evaluation{
			Schedule: today,
			Within:   isWorkday(today, todayDate),
			Date:     todayDate,
			Start:    todayStart,
			End:      todayEnd,
		}, nil
	}
	if yOvernight && !local.After(yEnd) {
		return Evaluation{
			Schedule: yesterday,
			Within:   isWorkday(yesterday, yesterdayDate),
			Date:     yesterdayDate,
			Start:    yStart,
			End:      yEnd,
		}, nil
	}
	return Evaluation{
		Schedule: today,
		Within:   false,
		Date:     todayDate,
		Start:    todayStart,
		End:      todayEnd,
	}, nil
}

// WindowEnd возвращает плановое окончание смены для графика и локальной даты
// начала смены. Для ночных смен добавляются сутки.
func WindowEnd(schedule repo.Schedule, startDate time.Time) (time.Time, error) {
	_, end, _, err := window(schedule, localDate(startDate.In(NovosibirskZone)))
	if err != nil {
		return time.Time{}, err
	}
	return end, nil
}

func window(schedule repo.Schedule, date time.Time) (time.Time, time.Time, bool, error) {
	startMin, err := ParseClock(schedule.StartTime)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	endMin, err := ParseClock(schedule.EndTime)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), startMin/60, startMin%60, 0, 0, NovosibirskZone)
	end := time.Date(date.Year(), date.Month(), date.Day(), endMin/60, endMin%60, 0, 0, NovosibirskZone)
	overnight := endMin <= startMin
	if overnight {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, overnight, nil
}

func isWorkday(schedule repo.Schedule, date time.Time) bool {
	weekday := int(date.In(NovosibirskZone).Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for _, day := range schedule.Workdays {
		if day == weekday {
			return true
		}
	}
	return false
}

// ParseClock разбирает время в формате HH:MM или HH:MM:SS и возвращает минуты.
func ParseClock(value string) (int, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, fmt.Errorf("время %q должно быть в формате HH:MM", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("некорректный час в %q", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("некорректные минуты в %q", value)
	}
	if len(parts) == 3 {
		second, err := strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			return 0, fmt.Errorf("некорректные секунды в %q", value)
		}
	}
	return hour*60 + minute, nil
}

// NormalizeClock приводит время к HH:MM.
func NormalizeClock(value string) (string, error) {
	minutes, err := ParseClock(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60), nil
}

func localDate(local time.Time) time.Time {
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, NovosibirskZone)
}
